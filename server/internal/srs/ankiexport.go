package srs

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Anki .apkg export. Produces a legacy (schema 11) package — a zip holding a
// `collection.anki2` SQLite DB, a `media` manifest, and numbered media files.
// Every current Anki version still imports this format (it's what genanki
// emits), and unlike the CSV export it needs no field-mapping on the Anki
// side and carries things CSV can't:
//
//   - Scheduling: FSRS state maps onto Anki's card columns, so a card due in
//     12 days in Yume is due in 12 days in Anki — no "2000 cards due today"
//     restart shock.
//   - Review history: review_log becomes revlog rows, so Anki's stats graphs
//     and its FSRS optimizer see the real history.
//   - Media: audio/image sidecars ship inside the package and the note
//     templates reference them.
//   - Tags: word_tags become Anki note tags.
//
// Determinism: model/deck ids are fixed constants and note/card ids derive
// from Yume card ids, so exporting twice and importing both into the same
// Anki collection updates notes in place (Anki dedupes on note guid) instead
// of duplicating the deck.

// Fixed identities. Anki uses epoch-milliseconds ints for these; the exact
// values don't matter as long as they're stable across exports and unlikely
// to collide with a user's real ids (real ids encode creation time, so any
// fixed past-date value risks ~nothing).
const (
	ankiModelID    = int64(1748000000001)
	ankiDeckID     = int64(1748000000002)
	ankiNoteIDBase = int64(1748100000000)
	ankiCardIDBase = int64(1748200000000)
)

// ankiFields is the note type's field list, in order. Sort field is Lemma.
var ankiFields = []string{"Lemma", "Reading", "Glosses", "Sentence", "Source", "Audio", "Image"}

const ankiCSS = `.card {
  font-family: "Hiragino Sans", "Yu Gothic", "Noto Sans JP", sans-serif;
  font-size: 22px;
  text-align: center;
  color: #1a1a2e;
  background-color: #fafafa;
}
.lemma { font-size: 40px; font-weight: 600; }
.sentence { margin-top: 14px; font-size: 22px; color: #444; }
.reading { font-size: 26px; color: #6366f1; margin-bottom: 6px; }
.glosses { font-size: 20px; }
.source { margin-top: 16px; font-size: 13px; }
.source a { color: #999; }
`

const ankiQfmt = `<div class="lemma">{{Lemma}}</div>
{{#Sentence}}<div class="sentence">{{Sentence}}</div>{{/Sentence}}`

const ankiAfmt = `{{FrontSide}}
<hr id=answer>
<div class="reading">{{Reading}}</div>
<div class="glosses">{{Glosses}}</div>
{{Audio}}
{{#Image}}<div>{{Image}}</div>{{/Image}}
{{#Source}}<div class="source"><a href="{{Source}}">source</a></div>{{/Source}}`

// exportCard is one cards row plus its tag set, read up front so the export
// runs off a stable snapshot.
type exportCard struct {
	id        int64
	lemma     string
	kanji     []string
	readings  []string
	glosses   [][]string
	sentence  string
	sourceURL string
	createdAt int64
	due       int64
	schedDays float64
	reps      int
	lapses    int
	state     int
	audioFile string
	imageFile string
	tags      []string
}

// ExportApkg packages every card (plus media + review history) into an Anki
// .apkg and returns the zip bytes. deckName defaults to "Yume".
func (s *Store) ExportApkg(deckName string) ([]byte, error) {
	deckName = strings.TrimSpace(deckName)
	if deckName == "" {
		deckName = "Yume"
	}

	cards, err := s.readExportCards()
	if err != nil {
		return nil, err
	}
	if len(cards) == 0 {
		return nil, errors.New("no cards to export")
	}
	logs, err := s.readReviewLog()
	if err != nil {
		return nil, err
	}

	// Drop references to media files that no longer exist on disk — a stale
	// sidecar reference shouldn't fail the export or produce a note pointing
	// at media the package doesn't contain.
	for i := range cards {
		if f := cards[i].audioFile; f != "" {
			if _, err := os.Stat(filepath.Join(s.MediaDir(), f)); err != nil {
				cards[i].audioFile = ""
			}
		}
		if f := cards[i].imageFile; f != "" {
			if _, err := os.Stat(filepath.Join(s.MediaDir(), f)); err != nil {
				cards[i].imageFile = ""
			}
		}
	}

	// Build collection.anki2 in a temp file (modernc needs a real path).
	tmpDir, err := os.MkdirTemp("", "yume-apkg-*")
	if err != nil {
		return nil, fmt.Errorf("temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	colPath := filepath.Join(tmpDir, "collection.anki2")

	media, err := buildCollectionDB(colPath, deckName, cards, logs)
	if err != nil {
		return nil, err
	}
	colBytes, err := os.ReadFile(colPath)
	if err != nil {
		return nil, fmt.Errorf("read collection: %w", err)
	}

	// Zip it: collection.anki2 + media manifest + numbered media files.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if err := zipWrite(zw, "collection.anki2", colBytes); err != nil {
		return nil, err
	}
	manifest := make(map[string]string, len(media))
	for i, filename := range media {
		n := strconv.Itoa(i)
		manifest[n] = filename
		data, err := os.ReadFile(filepath.Join(s.MediaDir(), filename))
		if err != nil {
			return nil, fmt.Errorf("read media %s: %w", filename, err)
		}
		if err := zipWrite(zw, n, data); err != nil {
			return nil, err
		}
	}
	mjson, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("media manifest: %w", err)
	}
	if err := zipWrite(zw, "media", mjson); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("close zip: %w", err)
	}
	return buf.Bytes(), nil
}

func zipWrite(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("zip entry %s: %w", name, err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("zip write %s: %w", name, err)
	}
	return nil
}

func (s *Store) readExportCards() ([]exportCard, error) {
	rows, err := s.q().Query(`
		SELECT id, lemma, kanji, readings, glosses, sentence, source_url,
		       created_at, due, scheduled_days, reps, lapses, state,
		       audio_file, image_file
		FROM cards WHERE lang = ? ORDER BY id`, s.lang)
	if err != nil {
		return nil, fmt.Errorf("query cards: %w", err)
	}
	defer rows.Close()

	var out []exportCard
	for rows.Next() {
		var c exportCard
		var kanji, readings, glosses, sentence, source, audio, image sql.NullString
		if err := rows.Scan(&c.id, &c.lemma, &kanji, &readings, &glosses, &sentence, &source,
			&c.createdAt, &c.due, &c.schedDays, &c.reps, &c.lapses, &c.state,
			&audio, &image); err != nil {
			return nil, fmt.Errorf("scan card: %w", err)
		}
		c.kanji = decodeStrArray(kanji)
		c.readings = decodeStrArray(readings)
		c.glosses = decodeGlosses(glosses)
		c.sentence = sentence.String
		c.sourceURL = source.String
		c.audioFile = audio.String
		c.imageFile = image.String
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Attach tags (per-lemma, may apply to several cards).
	tagRows, err := s.q().Query(`SELECT lemma, tag FROM word_tags WHERE lang = ?`, s.lang)
	if err != nil {
		return nil, fmt.Errorf("query tags: %w", err)
	}
	defer tagRows.Close()
	byLemma := map[string][]string{}
	for tagRows.Next() {
		var lemma, tag string
		if err := tagRows.Scan(&lemma, &tag); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		byLemma[lemma] = append(byLemma[lemma], tag)
	}
	if err := tagRows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].tags = byLemma[out[i].lemma]
	}
	return out, nil
}

type exportLog struct {
	cardID     int64
	rating     int
	reviewedAt int64
}

func (s *Store) readReviewLog() ([]exportLog, error) {
	rows, err := s.q().Query(`SELECT card_id, rating, reviewed_at FROM review_log ORDER BY reviewed_at, id`)
	if err != nil {
		return nil, fmt.Errorf("query review_log: %w", err)
	}
	defer rows.Close()
	var out []exportLog
	for rows.Next() {
		var l exportLog
		if err := rows.Scan(&l.cardID, &l.rating, &l.reviewedAt); err != nil {
			return nil, fmt.Errorf("scan review_log: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// buildCollectionDB writes a schema-11 Anki collection at path and returns
// the ordered list of media filenames referenced by the notes.
func buildCollectionDB(path, deckName string, cards []exportCard, logs []exportLog) ([]string, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open collection: %w", err)
	}
	defer db.Close()

	for _, stmt := range ankiSchema11 {
		if _, err := db.Exec(stmt); err != nil {
			return nil, fmt.Errorf("anki schema: %w", err)
		}
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now()
	nowSec := now.Unix()
	nowMs := now.UnixMilli()

	// Collection creation time: local midnight of the earliest card, so
	// every due date lands at a non-negative day offset.
	earliest := nowSec
	for _, c := range cards {
		if c.createdAt < earliest {
			earliest = c.createdAt
		}
	}
	crt := time.Unix(earliest, 0)
	crt = time.Date(crt.Year(), crt.Month(), crt.Day(), 4, 0, 0, 0, time.Local) // Anki's 4am day cutoff
	crtSec := crt.Unix()

	colConf, colModels, colDecks, colDconf, err := ankiColJSON(deckName, nowSec)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(
		`INSERT INTO col (id, crt, mod, scm, ver, dty, usn, ls, conf, models, decks, dconf, tags)
		 VALUES (1, ?, ?, ?, 11, 0, 0, 0, ?, ?, ?, ?, '{}')`,
		crtSec, nowMs, nowMs, colConf, colModels, colDecks, colDconf,
	); err != nil {
		return nil, fmt.Errorf("insert col: %w", err)
	}

	var media []string
	newPos := 0
	for _, c := range cards {
		flds, mediaRefs := ankiNoteFields(c)
		media = append(media, mediaRefs...)

		sfld := flds[0]
		joined := strings.Join(flds, "\x1f")
		noteID := ankiNoteIDBase + c.id
		cardID := ankiCardIDBase + c.id
		if _, err := tx.Exec(
			`INSERT INTO notes (id, guid, mid, mod, usn, tags, flds, sfld, csum, flags, data)
			 VALUES (?, ?, ?, ?, -1, ?, ?, ?, ?, 0, '')`,
			noteID, fmt.Sprintf("yume-%d", c.id), ankiModelID, nowSec,
			ankiTagString(c.tags), joined, sfld, ankiChecksum(sfld),
		); err != nil {
			return nil, fmt.Errorf("insert note %d: %w", c.id, err)
		}

		typ, queue, due, ivl, factor := ankiScheduling(c, crtSec, &newPos)
		if _, err := tx.Exec(
			`INSERT INTO cards (id, nid, did, ord, mod, usn, type, queue, due, ivl, factor,
			                    reps, lapses, left, odue, odid, flags, data)
			 VALUES (?, ?, ?, 0, ?, -1, ?, ?, ?, ?, ?, ?, ?, 0, 0, 0, 0, '')`,
			cardID, noteID, ankiDeckID, nowSec, typ, queue, due, ivl, factor, c.reps, c.lapses,
		); err != nil {
			return nil, fmt.Errorf("insert anki card %d: %w", c.id, err)
		}
	}

	// Review history. revlog ids are epoch-ms and must be unique; reviews in
	// the same second get consecutive ms offsets. The first review of a card
	// is a learn (type 0), the rest reviews (type 1) — we don't store Anki's
	// per-review interval/factor, so those are 0 (Anki's stats and the FSRS
	// optimizer key off ease + timestamps, which we have).
	cardIDs := make(map[int64]bool, len(cards))
	for _, c := range cards {
		cardIDs[c.id] = true
	}
	seenMs := make(map[int64]bool, len(logs))
	firstReview := make(map[int64]bool, len(logs))
	for _, l := range logs {
		if !cardIDs[l.cardID] {
			continue // log row for a since-deleted card
		}
		id := l.reviewedAt * 1000
		for seenMs[id] {
			id++
		}
		seenMs[id] = true
		typ := 1
		if !firstReview[l.cardID] {
			firstReview[l.cardID] = true
			typ = 0
		}
		if _, err := tx.Exec(
			`INSERT INTO revlog (id, cid, usn, ease, ivl, lastIvl, factor, time, type)
			 VALUES (?, ?, -1, ?, 0, 0, 0, 0, ?)`,
			id, ankiCardIDBase+l.cardID, l.rating, typ,
		); err != nil {
			return nil, fmt.Errorf("insert revlog: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit collection: %w", err)
	}
	return media, nil
}

// ankiNoteFields renders one card into the model's field list (HTML-escaped)
// and reports which media files the fields reference.
func ankiNoteFields(c exportCard) (flds []string, mediaRefs []string) {
	lemma := c.lemma
	if len(c.kanji) > 0 {
		lemma = c.kanji[0]
	}
	var glossParts []string
	for _, sense := range c.glosses {
		glossParts = append(glossParts, strings.Join(sense, ", "))
	}
	audio, image := "", ""
	if c.audioFile != "" {
		audio = "[sound:" + c.audioFile + "]"
		mediaRefs = append(mediaRefs, c.audioFile)
	}
	if c.imageFile != "" {
		image = `<img src="` + html.EscapeString(c.imageFile) + `">`
		mediaRefs = append(mediaRefs, c.imageFile)
	}
	flds = []string{
		html.EscapeString(lemma),
		html.EscapeString(strings.Join(c.readings, "、")),
		html.EscapeString(strings.Join(glossParts, " | ")),
		html.EscapeString(c.sentence),
		html.EscapeString(c.sourceURL),
		audio,
		image,
	}
	return flds, mediaRefs
}

// ankiScheduling maps FSRS card state onto Anki's (type, queue, due, ivl,
// factor) columns. Review cards keep their due date (as days since collection
// creation) and interval; learning/relearning cards are exported as reviews
// with their current interval (Anki's learning queue encodes step state we
// can't reconstruct); new cards queue up in saved order.
func ankiScheduling(c exportCard, crtSec int64, newPos *int) (typ, queue int, due, ivl, factor int64) {
	switch c.state {
	case 0: // New
		due = int64(*newPos)
		*newPos++
		return 0, 0, due, 0, 0
	default: // Learning / Review / Relearning
		days := (c.due - crtSec) / 86400
		if days < 0 {
			days = 0
		}
		ivl = int64(c.schedDays)
		if ivl < 1 {
			ivl = 1
		}
		return 2, 2, days, ivl, 2500
	}
}

// ankiTagString joins tags in Anki's space-delimited format. Anki tags can't
// contain spaces, so embedded whitespace becomes underscores.
func ankiTagString(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	clean := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.Join(strings.Fields(t), "_")
		if t != "" {
			clean = append(clean, t)
		}
	}
	if len(clean) == 0 {
		return ""
	}
	return " " + strings.Join(clean, " ") + " "
}

// ankiChecksum is Anki's note dedupe checksum: the first 8 hex digits of
// SHA1(first field), as an integer.
func ankiChecksum(sfld string) int64 {
	sum := sha1.Sum([]byte(sfld))
	n, _ := strconv.ParseInt(hex.EncodeToString(sum[:])[:8], 16, 64)
	return n
}

// ankiColJSON builds the four JSON blobs on the col row. Shapes follow what
// genanki writes — known-good against every importer version.
func ankiColJSON(deckName string, nowSec int64) (conf, models, decks, dconf string, err error) {
	confMap := map[string]any{
		"activeDecks": []int64{1}, "addToCur": true, "collapseTime": 1200,
		"curDeck": 1, "curModel": strconv.FormatInt(ankiModelID, 10), "dueCounts": true,
		"estTimes": true, "newBury": true, "newSpread": 0, "nextPos": 1,
		"sortBackwards": false, "sortType": "noteFld", "timeLim": 0,
	}

	flds := make([]map[string]any, len(ankiFields))
	for i, name := range ankiFields {
		flds[i] = map[string]any{
			"name": name, "ord": i, "sticky": false, "rtl": false,
			"font": "Arial", "size": 20, "media": []any{},
		}
	}
	model := map[string]any{
		"id": ankiModelID, "name": "Yume Vocab", "type": 0, "mod": nowSec, "usn": -1,
		"sortf": 0, "did": ankiDeckID, "tags": []any{}, "vers": []any{},
		"css":       ankiCSS,
		"latexPre":  "\\documentclass[12pt]{article}\n\\special{papersize=3in,5in}\n\\usepackage[utf8]{inputenc}\n\\usepackage{amssymb,amsmath}\n\\pagestyle{empty}\n\\setlength{\\parindent}{0in}\n\\begin{document}\n",
		"latexPost": "\\end{document}",
		"flds":      flds,
		"tmpls": []map[string]any{{
			"name": "Card 1", "ord": 0, "qfmt": ankiQfmt, "afmt": ankiAfmt,
			"bqfmt": "", "bafmt": "", "did": nil, "bfont": "", "bsize": 0,
		}},
		"req": []any{[]any{0, "all", []any{0}}},
	}
	modelsMap := map[string]any{strconv.FormatInt(ankiModelID, 10): model}

	mkDeck := func(id int64, name, desc string) map[string]any {
		return map[string]any{
			"id": id, "name": name, "desc": desc, "mod": nowSec, "usn": -1,
			"lrnToday": []int{0, 0}, "revToday": []int{0, 0},
			"newToday": []int{0, 0}, "timeToday": []int{0, 0},
			"collapsed": false, "browserCollapsed": false,
			"dyn": 0, "conf": 1, "extendNew": 10, "extendRev": 50,
		}
	}
	decksMap := map[string]any{
		"1":                               mkDeck(1, "Default", ""),
		strconv.FormatInt(ankiDeckID, 10): mkDeck(ankiDeckID, deckName, "Exported from Yume"),
	}

	dconfMap := map[string]any{
		"1": map[string]any{
			"id": 1, "name": "Default", "autoplay": true, "timer": 0, "replayq": true,
			"mod": 0, "usn": 0, "maxTaken": 60,
			"new": map[string]any{
				"bury": true, "delays": []float64{1, 10}, "initialFactor": 2500,
				"ints": []int{1, 4, 7}, "order": 1, "perDay": 20, "separate": true,
			},
			"rev": map[string]any{
				"bury": true, "ease4": 1.3, "fuzz": 0.05, "ivlFct": 1, "maxIvl": 36500,
				"minSpace": 1, "perDay": 100, "hardFactor": 1.2,
			},
			"lapse": map[string]any{
				"delays": []float64{10}, "leechAction": 0, "leechFails": 8, "minInt": 1, "mult": 0,
			},
		},
	}

	enc := func(v any) (string, error) {
		b, err := json.Marshal(v)
		return string(b), err
	}
	if conf, err = enc(confMap); err != nil {
		return "", "", "", "", fmt.Errorf("conf json: %w", err)
	}
	if models, err = enc(modelsMap); err != nil {
		return "", "", "", "", fmt.Errorf("models json: %w", err)
	}
	if decks, err = enc(decksMap); err != nil {
		return "", "", "", "", fmt.Errorf("decks json: %w", err)
	}
	if dconf, err = enc(dconfMap); err != nil {
		return "", "", "", "", fmt.Errorf("dconf json: %w", err)
	}
	return conf, models, decks, dconf, nil
}

// ankiSchema11 is Anki's legacy collection schema, verbatim (minus the
// sqlite_stat tables Anki adds lazily).
var ankiSchema11 = []string{
	`CREATE TABLE col (
		id integer primary key, crt integer not null, mod integer not null,
		scm integer not null, ver integer not null, dty integer not null,
		usn integer not null, ls integer not null, conf text not null,
		models text not null, decks text not null, dconf text not null,
		tags text not null
	)`,
	`CREATE TABLE notes (
		id integer primary key, guid text not null, mid integer not null,
		mod integer not null, usn integer not null, tags text not null,
		flds text not null, sfld integer not null, csum integer not null,
		flags integer not null, data text not null
	)`,
	`CREATE TABLE cards (
		id integer primary key, nid integer not null, did integer not null,
		ord integer not null, mod integer not null, usn integer not null,
		type integer not null, queue integer not null, due integer not null,
		ivl integer not null, factor integer not null, reps integer not null,
		lapses integer not null, left integer not null, odue integer not null,
		odid integer not null, flags integer not null, data text not null
	)`,
	`CREATE TABLE revlog (
		id integer primary key, cid integer not null, usn integer not null,
		ease integer not null, ivl integer not null, lastIvl integer not null,
		factor integer not null, time integer not null, type integer not null
	)`,
	`CREATE TABLE graves (usn integer not null, oid integer not null, type integer not null)`,
	`CREATE INDEX ix_notes_usn ON notes (usn)`,
	`CREATE INDEX ix_cards_usn ON cards (usn)`,
	`CREATE INDEX ix_revlog_usn ON revlog (usn)`,
	`CREATE INDEX ix_cards_nid ON cards (nid)`,
	`CREATE INDEX ix_cards_sched ON cards (did, queue, due)`,
	`CREATE INDEX ix_revlog_cid ON revlog (cid)`,
	`CREATE INDEX ix_notes_csum ON notes (csum)`,
}
