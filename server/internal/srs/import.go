package srs

import (
	"database/sql"
	"fmt"
	"html"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Anki CSV import. The frontend parses the CSV in-browser (handling quoting,
// delimiters, header mapping) and sends structured rows here. Safety stack:
//
//  1. Auto-backup — a pre-import snapshot (backups/cards-import-*,
//     restore_backup) before any writes, so a bad import is recoverable while
//     the snapshot is kept. Pre-import snapshots keep their own retention and
//     never prune a manual backup.
//  2. Transactional — every insert in one tx; all-or-nothing.
//  3. Mandatory tag, on created words only — every lemma the import adds
//     gets the caller's tag, so DeleteByTag(tag) undoes the batch. Words
//     already in the deck never get it (the undo would delete the learner's
//     own card); they get "seen-in:<tag>", which DeleteByTag refuses.
//  4. Dedup by lemma against word_status AND cards — default on; an existing
//     word is reported as skipped. With dedup off it may gain an extra card,
//     but its status is never overwritten.
//  5. Default status Learning (1) — avoids the "coverage jumps to 95%
//     overnight" trap. Caller can override. Known (2) and Ignored (3) lists
//     are status only: no cards unless create_cards is true.
//  6. Card row only when content exists — a lemma-only row produces a
//     word_status row but no cards row (nothing to review).
//  7. Lemmas are cleaned (HTML tags and entities, 漢字[かな] furigana) and
//     punctuation rows are rejected, each with a reason in Skipped.

// SeenInTagPrefix marks words that were already in the deck when an import
// listed them. DeleteByTag refuses these tags, and undoing the import clears
// its own marker.
const SeenInTagPrefix = "seen-in:"

// ImportRow is one CSV row mapped to fields.
type ImportRow struct {
	Lemma       string     `json:"lemma"`
	Kanji       []string   `json:"kanji,omitempty"`
	Readings    []string   `json:"readings,omitempty"`
	Glosses     [][]string `json:"glosses,omitempty"`
	Sentence    string     `json:"sentence,omitempty"`
	Translation string     `json:"translation,omitempty"` // English of the sentence
	SourceURL   string     `json:"source_url,omitempty"`
}

// ImportCSVReq is the import_csv payload.
type ImportCSVReq struct {
	Rows          []ImportRow `json:"rows"`
	Tag           string      `json:"tag"`
	DefaultStatus *int        `json:"default_status,omitempty"` // 0..3; defaults to 1 (Learning)
	Dedup         *bool       `json:"dedup,omitempty"`          // defaults true
	// CreateCards defaults to true, except for Known (2) and Ignored (3)
	// imports, which set the status only.
	CreateCards *bool `json:"create_cards,omitempty"`
}

// ImportSkip is one row the import did not add, with the reason.
type ImportSkip struct {
	Lemma  string `json:"lemma"`
	Reason string `json:"reason"`
}

// ImportCSVResp is the import_csv response shape.
type ImportCSVResp struct {
	CreatedWords      int `json:"created_words"`
	CreatedCards      int `json:"created_cards"`
	SkippedDuplicates int `json:"skipped_duplicates"`
	// SkippedCards counts rows with card content that got a status only
	// (a Known or Ignored import without create_cards).
	SkippedCards int          `json:"skipped_cards"`
	Skipped      []ImportSkip `json:"skipped"`
	Tag          string       `json:"tag"`
	SeenInTag    string       `json:"seen_in_tag"`
	// Backup names the snapshot taken just before this batch: restoring it
	// undoes the whole import. BackupPath is the same file's server path.
	Backup     string `json:"backup"`
	BackupPath string `json:"backup_path"`
	// PrunedBackups are older pre-import snapshots this one replaced.
	PrunedBackups []string `json:"pruned_backups"`
}

// ImportCSV runs the full import with the safety stack.
func (s *Store) ImportCSV(req ImportCSVReq) (ImportCSVResp, error) {
	tag := strings.ToLower(strings.TrimSpace(req.Tag))
	if tag == "" {
		return ImportCSVResp{}, clientErrf("tag is required (so the import is undoable via delete_by_tag)")
	}
	// A system tag as the batch tag would break the batch's one-click undo:
	// delete_by_tag refuses system tags, and the rows would pollute tutor triage.
	if IsSystemTag(tag) {
		return ImportCSVResp{}, clientErrf("%q is a reserved system tag; pick another batch tag", tag)
	}
	if strings.HasPrefix(tag, SeenInTagPrefix) {
		return ImportCSVResp{}, clientErrf("%q starts with %q, which marks words already in the deck; pick another batch tag", tag, SeenInTagPrefix)
	}
	if len(req.Rows) == 0 {
		return ImportCSVResp{}, clientErrf("no rows to import")
	}
	for i, row := range req.Rows {
		if err := checkCardText(fmt.Sprintf("row %d ", i+1), row.Lemma, row.Kanji, row.Readings, row.Sentence); err != nil {
			return ImportCSVResp{}, err
		}
	}
	defaultStatus := 1 // Learning
	if req.DefaultStatus != nil {
		defaultStatus = *req.DefaultStatus
		if defaultStatus < 0 || defaultStatus > 3 {
			return ImportCSVResp{}, clientErrf("invalid default_status %d", defaultStatus)
		}
	}
	dedup := true
	if req.Dedup != nil {
		dedup = *req.Dedup
	}
	// A Known list marks what the learner already knows; a card per word
	// would queue them all as new reviews.
	createCards := defaultStatus != 2 && defaultStatus != 3
	if req.CreateCards != nil {
		createCards = *req.CreateCards
	}
	seenTag := SeenInTagPrefix + tag

	// 1. Create a managed, restorable snapshot under backups/.
	info, pruned, err := s.createBackup(BackupPreImport, tag)
	if err != nil {
		return ImportCSVResp{}, fmt.Errorf("backup before import: %w", err)
	}
	if pruned == nil {
		pruned = []string{}
	}

	// 2. Transactional import.
	tx, err := s.q().Begin()
	if err != nil {
		return ImportCSVResp{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().Unix()
	resp := ImportCSVResp{
		Tag: tag, SeenInTag: seenTag, Skipped: []ImportSkip{},
		Backup: info.Name, BackupPath: filepath.Join(s.backupsDir(), info.Name), PrunedBackups: pruned,
	}
	skip := func(lemma, reason string) {
		resp.Skipped = append(resp.Skipped, ImportSkip{Lemma: lemma, Reason: reason})
	}
	// Lemmas this import created; a repeat row is not "already in the deck".
	created := map[string]bool{}

	for _, row := range req.Rows {
		raw := strings.TrimSpace(row.Lemma)
		if raw == "" {
			continue
		}
		// 7. Anki fronts carry markup the tokenizer and dictionary never match.
		lemma := cleanImportLemma(raw)
		if lemma == "" {
			skip(raw, "no word left after removing formatting")
			continue
		}
		if isJunkLemma(lemma) {
			skip(raw, "punctuation, not a word")
			continue
		}

		// 6. Card row only when there's content worth reviewing (a reading,
		//    gloss, or sentence — not a bare lemma).
		hasContent := len(row.Readings) > 0 || len(row.Glosses) > 0 ||
			strings.TrimSpace(row.Sentence) != "" || len(row.Kanji) > 0
		addCard := func() error {
			if !hasContent {
				return nil
			}
			if !createCards {
				resp.SkippedCards++
				return nil
			}
			if err := insertImportCard(tx, s.lang, lemma, row, now); err != nil {
				return err
			}
			resp.CreatedCards++
			return nil
		}

		if created[lemma] {
			if dedup {
				resp.SkippedDuplicates++
				skip(raw, "listed twice in this import")
				continue
			}
			// Still this batch's word, so the undo removes the extra card too.
			if err := addCard(); err != nil {
				return ImportCSVResp{}, err
			}
			continue
		}

		// 4. Dedup check: is this word already in the deck (status or card)?
		existed, err := lemmaInDeck(tx, s.lang, lemma)
		if err != nil {
			return ImportCSVResp{}, fmt.Errorf("dedup check: %w", err)
		}
		if existed {
			// 3. Never the batch tag: undoing the import would take the
			//    learner's own card, notes and history with it.
			if _, err := tx.Exec(
				`INSERT OR IGNORE INTO word_tags (lang, lemma, tag) VALUES (?, ?, ?)`, s.lang, lemma, seenTag,
			); err != nil {
				return ImportCSVResp{}, fmt.Errorf("mark existing lemma: %w", err)
			}
			if dedup {
				resp.SkippedDuplicates++
				skip(raw, "already in the deck")
				continue
			}
			// Dedup off: an extra card beside the learner's own, status
			// untouched. The undo leaves it (the word is not the batch's).
			if err := addCard(); err != nil {
				return ImportCSVResp{}, err
			}
			continue
		}
		created[lemma] = true

		// 5. Word status row at the chosen default (unless 0 = unknown, which
		//    we represent by absence — skip the row).
		if defaultStatus != 0 {
			if _, err := tx.Exec(
				`INSERT INTO word_status (lang, lemma, status, updated_at) VALUES (?, ?, ?, ?)`,
				s.lang, lemma, defaultStatus, now,
			); err != nil {
				return ImportCSVResp{}, fmt.Errorf("insert word_status: %w", err)
			}
		}
		resp.CreatedWords++

		// 3. Mandatory tag.
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO word_tags (lang, lemma, tag) VALUES (?, ?, ?)`, s.lang, lemma, tag,
		); err != nil {
			return ImportCSVResp{}, fmt.Errorf("tag lemma: %w", err)
		}
		if err := addCard(); err != nil {
			return ImportCSVResp{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return ImportCSVResp{}, fmt.Errorf("commit import: %w", err)
	}
	return resp, nil
}

// lemmaInDeck reports whether the word already has a status or a card.
// Cards count on their own: a word saved from the dictionary may have no
// word_status row.
func lemmaInDeck(tx *sql.Tx, lang, lemma string) (bool, error) {
	var n int
	err := tx.QueryRow(`
		SELECT EXISTS (SELECT 1 FROM word_status WHERE lang = ? AND lemma = ?)
		    OR EXISTS (SELECT 1 FROM cards WHERE lang = ? AND lemma = ?)
	`, lang, lemma, lang, lemma).Scan(&n)
	return n == 1, err
}

func insertImportCard(tx *sql.Tx, lang, lemma string, row ImportRow, now int64) error {
	if _, err := tx.Exec(`
		INSERT INTO cards
		  (jmdict_id, lang, lemma, kanji, readings, glosses, sentence, source_url, translation,
		   created_at, due, stability, difficulty, elapsed_days, scheduled_days,
		   reps, lapses, state, last_review, card_type, surface)
		VALUES
		  (NULL, ?, ?, ?, ?, ?, ?, ?, ?,
		   ?, ?, 0, 0, 0, 0,
		   0, 0, 0, NULL, 'vocab', NULL)
	`,
		lang, lemma,
		jsonOrNil(row.Kanji), jsonOrNil(row.Readings), jsonOrNil(row.Glosses),
		nullableStr(row.Sentence), nullableStr(row.SourceURL), nullableStr(strings.TrimSpace(row.Translation)),
		now, now,
	); err != nil {
		return fmt.Errorf("insert card: %w", err)
	}
	return nil
}

var (
	importTagRe      = regexp.MustCompile(`<[^>]*>`)
	importFuriganaRe = regexp.MustCompile(`\[[^\]]*\]`)
)

// cleanImportLemma turns an Anki front into a plain lemma: HTML tags and
// entities go, and furigana (食[た]べる, 日本[にほん] 語[ご]) becomes the base
// text. Anki uses the space before a furigana group as a separator, so
// spaces go with the brackets.
func cleanImportLemma(raw string) string {
	s := importTagRe.ReplaceAllString(raw, "")
	s = html.UnescapeString(s)
	if importFuriganaRe.MatchString(s) {
		s = importFuriganaRe.ReplaceAllString(s, "")
		s = strings.ReplaceAll(s, " ", "")
	}
	return strings.TrimSpace(s)
}
