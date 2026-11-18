package srs

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// VocabRow is the wire shape of one vocab explorer row. Each row is one lemma — either it has a card, or a word_status,
// or both.
type VocabRow struct {
	Lemma       string     `json:"lemma"`
	Status      int        `json:"status"`  // 0=unknown, 1=learning, 2=known, 3=ignored
	CardID      *int64     `json:"card_id"` // null when no card
	Kanji       []string   `json:"kanji"`
	Readings    []string   `json:"readings"`
	Glosses     [][]string `json:"glosses"` // per-sense
	Sentence    *string    `json:"sentence"`
	Translation *string    `json:"translation,omitempty"`
	Explanation *string    `json:"explanation,omitempty"`
	Mnemonic    *Mnemonic  `json:"mnemonic,omitempty"`
	// MnemonicShared: the mnemonic is the server's shared one, not the card's own.
	MnemonicShared bool     `json:"mnemonic_shared,omitempty"`
	SourceURL      *string  `json:"source_url"`
	CreatedAt      *string  `json:"created_at"` // RFC3339 from created_at epoch; null for status-only lemmas
	Tags           []string `json:"tags"`       // always present, may be empty
	HasAudio       bool     `json:"has_audio"`  // true when the card has an audio sidecar
	// Review history, for "what am I struggling with" views (sort "lapses").
	Reps   int `json:"reps,omitempty"`
	Lapses int `json:"lapses,omitempty"`
	// Frequency rank (approximate, lower = more common) and its band, filled
	// by the server layer for sort "frequency"; absent otherwise.
	Rank int    `json:"rank,omitempty"`
	Band string `json:"band,omitempty"`
}

// VocabFilter is the request shape. Every field is optional — empty means
// "no filter on this dimension".
type VocabFilter struct {
	Status     *int     `json:"status,omitempty"`      // exact match on word_status.status (or 0 for unknown)
	HasCard    *bool    `json:"has_card,omitempty"`    // true → only rows with a card, false → only rows without
	Search     string   `json:"search,omitempty"`      // LIKE %s% on lemma / readings / glosses / sentence
	Tags       []string `json:"tags,omitempty"`        // every listed tag must be present (AND)
	Sort       string   `json:"sort,omitempty"`        // "newest" → card created_at DESC; "lapses" → most forgotten first; default lemma A→Z
	NeedsAudio *bool    `json:"needs_audio,omitempty"` // true → has a card but no audio sidecar
	NeedsGloss *bool    `json:"needs_gloss,omitempty"` // true → has a card but empty glosses
	New        *bool    `json:"new,omitempty"`         // true → has a card that has never been reviewed
	Band       string   `json:"band,omitempty"`        // frequency band key; applied by the server layer, which owns ranks
}

// ListVocabResp is the response shape: total matching rows (for paging) +
// the current page of rows.
type ListVocabResp struct {
	Total int        `json:"total"`
	Rows  []VocabRow `json:"rows"`
}

// ListVocabAll returns every matching row, paging through ListVocab, for
// callers that must order or aggregate on data the DB doesn't hold (frequency
// ranks live in the in-memory dictionary).
func (s *Store) ListVocabAll(filter VocabFilter) ([]VocabRow, error) {
	filter.Sort = ""
	var out []VocabRow
	for offset := 0; ; offset += 500 {
		page, err := s.ListVocab(filter, 500, offset)
		if err != nil {
			return nil, err
		}
		out = append(out, page.Rows...)
		if len(out) >= page.Total || len(page.Rows) == 0 {
			return out, nil
		}
	}
}

// ListVocab unions lemmas from cards + word_status, applies the filters,
// paginates server-side, and hydrates tags per returned lemma.
//
// Implementation notes:
//   - We union both source tables so a lemma with only a status (no card)
//     and a lemma with only a card (no status) both appear. That's the
//     mining UX users expect.
//   - The query is built dynamically rather than pre-compiled because the
//     tags AND-filter expands to N EXISTS subqueries.
//   - Search hits lemma, readings (JSON text), glosses (JSON text),
//     sentence, and the kanji-array JSON field. Wanakana romaji conversion
//     happens client-side before the request — we just LIKE here.
//   - For lemmas that have multiple cards (rare; happens when one lemma has
//     multiple jmdict_id matches), we pick the lowest card id. This keeps
//     the row count predictable.
func (s *Store) ListVocab(filter VocabFilter, limit, offset int) (ListVocabResp, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		// Cap the page size so a caller can't request an unbounded result set.
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}

	// Build the base query + args. We use the same WHERE for total + page.
	where, args := buildVocabWhere(filter, s.lang)

	// Default order is alphabetical; "newest" surfaces freshly-mined cards
	// first (status-only lemmas have no created_at, so they sort last).
	orderBy := "ORDER BY l.lemma"
	if filter.Sort == "newest" {
		orderBy = "ORDER BY (c.created_at IS NULL), c.created_at DESC, l.lemma"
	}
	// "lapses": the cards the learner keeps forgetting, then the most reviewed.
	if filter.Sort == "lapses" {
		orderBy = "ORDER BY (c.card_id IS NULL), c.lapses DESC, c.reps DESC, l.lemma"
	}

	// `card_per_lemma` collapses cards down to one row per lemma so the
	// outer SELECT has a 1:1 join. We pick the lowest-id card per lemma via
	// a correlated subquery — SQLite doesn't let us refer to MIN() from a
	// scalar subquery inside the same SELECT, so this is the cleanest
	// expression.
	baseQuery := `
WITH all_lemmas AS (
  SELECT DISTINCT lemma FROM cards WHERE lang = ?
  UNION
  SELECT lemma FROM word_status WHERE lang = ?
),
card_per_lemma AS (
  SELECT c.lemma, c.id AS card_id,
         c.kanji, c.readings, c.glosses,
         c.sentence, c.source_url, c.created_at, c.audio_file, c.reps, c.lapses, c.translation, c.explanation, c.mnemonic_parts, c.mnemonic_story
  FROM cards c
  WHERE c.lang = ? AND c.id = (SELECT MIN(c2.id) FROM cards c2 WHERE c2.lang = c.lang AND c2.lemma = c.lemma)
)
SELECT
  l.lemma,
  COALESCE(w.status, 0) AS status,
  c.card_id,
  c.kanji, c.readings, c.glosses, c.sentence, c.source_url, c.created_at, c.audio_file,
  c.reps, c.lapses, c.translation, c.explanation, c.mnemonic_parts, c.mnemonic_story
FROM all_lemmas l
LEFT JOIN card_per_lemma c ON c.lemma = l.lemma
LEFT JOIN word_status w ON w.lang = ? AND w.lemma = l.lemma
WHERE ` + where + `
` + orderBy + `
LIMIT ? OFFSET ?`

	// Count query mirrors the SELECT minus ORDER/LIMIT. It MUST use the same
	// card_per_lemma CTE as the page query — the search WHERE references the
	// card content columns (c.kanji etc.), so a slimmer CTE would break search.
	countQuery := `
WITH all_lemmas AS (
  SELECT DISTINCT lemma FROM cards WHERE lang = ?
  UNION
  SELECT lemma FROM word_status WHERE lang = ?
),
card_per_lemma AS (
  SELECT c.lemma, c.id AS card_id,
         c.kanji, c.readings, c.glosses,
         c.sentence, c.source_url, c.created_at, c.audio_file, c.reps, c.lapses, c.translation, c.explanation, c.mnemonic_parts, c.mnemonic_story
  FROM cards c
  WHERE c.lang = ? AND c.id = (SELECT MIN(c2.id) FROM cards c2 WHERE c2.lang = c.lang AND c2.lemma = c.lemma)
)
SELECT COUNT(*) FROM all_lemmas l
LEFT JOIN card_per_lemma c ON c.lemma = l.lemma
LEFT JOIN word_status w ON w.lang = ? AND w.lemma = l.lemma
WHERE ` + where

	// The CTEs and join take the language before the WHERE args.
	langArgs := []any{s.lang, s.lang, s.lang, s.lang}
	countArgs := append(append([]any{}, langArgs...), args...)
	pageArgs := append(append([]any{}, langArgs...), args...)
	pageArgs = append(pageArgs, limit, offset)

	var resp ListVocabResp
	if err := s.q().QueryRow(countQuery, countArgs...).Scan(&resp.Total); err != nil {
		return resp, fmt.Errorf("count vocab: %w", err)
	}
	rows, err := s.q().Query(baseQuery, pageArgs...)
	if err != nil {
		return resp, fmt.Errorf("query vocab: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var r VocabRow
		var cardID sql.NullInt64
		var kanjiJSON, readingsJSON, glossesJSON sql.NullString
		var sentence, sourceURL, audioFile sql.NullString
		var createdAt, reps, lapses sql.NullInt64
		var translation, explanation, mnemonicParts, mnemonicStory sql.NullString
		if err := rows.Scan(
			&r.Lemma, &r.Status, &cardID,
			&kanjiJSON, &readingsJSON, &glossesJSON,
			&sentence, &sourceURL, &createdAt, &audioFile,
			&reps, &lapses, &translation, &explanation, &mnemonicParts, &mnemonicStory,
		); err != nil {
			return resp, fmt.Errorf("scan vocab: %w", err)
		}
		r.Mnemonic = decodeMnemonic(mnemonicParts, mnemonicStory)
		r.Reps, r.Lapses = int(reps.Int64), int(lapses.Int64)
		if translation.Valid && translation.String != "" {
			x := translation.String
			r.Translation = &x
		}
		if explanation.Valid && explanation.String != "" {
			x := explanation.String
			r.Explanation = &x
		}
		r.HasAudio = audioFile.Valid && strings.TrimSpace(audioFile.String) != ""
		if cardID.Valid {
			id := cardID.Int64
			r.CardID = &id
		}
		r.Kanji = decodeStrArray(kanjiJSON)
		r.Readings = decodeStrArray(readingsJSON)
		r.Glosses = decodeGlosses(glossesJSON)
		if sentence.Valid {
			x := sentence.String
			r.Sentence = &x
		}
		if sourceURL.Valid {
			x := sourceURL.String
			r.SourceURL = &x
		}
		if createdAt.Valid {
			t := time.Unix(createdAt.Int64, 0).UTC().Format(time.RFC3339)
			r.CreatedAt = &t
		}
		r.Tags = []string{} // populated below
		resp.Rows = append(resp.Rows, r)
	}
	if err := rows.Err(); err != nil {
		return resp, err
	}

	// Batch-fetch tags for the page in one extra query.
	if len(resp.Rows) > 0 {
		if err := s.hydrateTags(resp.Rows); err != nil {
			return resp, err
		}
	}
	err = s.fillSharedMnemonics(len(resp.Rows),
		func(i int) (string, *Mnemonic) { return resp.Rows[i].Lemma, resp.Rows[i].Mnemonic },
		func(i int, m *Mnemonic) { resp.Rows[i].Mnemonic, resp.Rows[i].MnemonicShared = m, true })
	if err != nil {
		return resp, err
	}
	known, err := s.knownKanji()
	for i := range resp.Rows {
		markKnown(resp.Rows[i].Mnemonic, known)
	}
	return resp, err
}

// buildVocabWhere returns a WHERE clause body + the positional args for it.
// Always starts with `1=1` so subsequent ANDs can be appended without
// worrying about whether the first clause exists.
func buildVocabWhere(f VocabFilter, lang string) (string, []any) {
	var parts []string
	var args []any
	parts = append(parts, "1=1")

	if f.Status != nil {
		parts = append(parts, "COALESCE(w.status, 0) = ?")
		args = append(args, *f.Status)
	}
	if f.HasCard != nil {
		if *f.HasCard {
			parts = append(parts, "c.card_id IS NOT NULL")
		} else {
			parts = append(parts, "c.card_id IS NULL")
		}
	}
	if f.New != nil {
		if *f.New {
			parts = append(parts, "c.card_id IS NOT NULL AND c.reps = 0")
		} else {
			parts = append(parts, "c.card_id IS NOT NULL AND c.reps > 0")
		}
	}
	if f.NeedsAudio != nil {
		if *f.NeedsAudio {
			parts = append(parts, "c.card_id IS NOT NULL AND (c.audio_file IS NULL OR c.audio_file = '')")
		} else {
			parts = append(parts, "c.card_id IS NOT NULL AND c.audio_file IS NOT NULL AND c.audio_file != ''")
		}
	}
	if f.NeedsGloss != nil {
		// glosses is JSON text; empty is NULL, "" or "[]".
		if *f.NeedsGloss {
			parts = append(parts, "c.card_id IS NOT NULL AND (c.glosses IS NULL OR c.glosses = '' OR c.glosses = '[]')")
		} else {
			parts = append(parts, "c.card_id IS NOT NULL AND c.glosses IS NOT NULL AND c.glosses != '' AND c.glosses != '[]'")
		}
	}
	if s := strings.TrimSpace(f.Search); s != "" {
		// Single LIKE %s% applied to multiple JSON-text fields. SQLite
		// LIKE is case-insensitive for ASCII; for kana/kanji it matches
		// literally, which is what we want.
		like := "%" + s + "%"
		parts = append(parts, `(
		  l.lemma LIKE ? OR
		  COALESCE(c.kanji,'')    LIKE ? OR
		  COALESCE(c.readings,'') LIKE ? OR
		  COALESCE(c.glosses,'')  LIKE ? OR
		  COALESCE(c.sentence,'') LIKE ?
		)`)
		args = append(args, like, like, like, like, like)
	}
	for _, tag := range f.Tags {
		t := strings.TrimSpace(strings.ToLower(tag))
		if t == "" {
			continue
		}
		parts = append(parts, "EXISTS (SELECT 1 FROM word_tags wt WHERE wt.lang = ? AND wt.lemma = l.lemma AND wt.tag = ?)")
		args = append(args, lang, t)
	}
	return strings.Join(parts, " AND "), args
}

// hydrateTags populates Tags on every row in `rows` in one batched query.
// In-place mutation — callers pass the slice they want enriched.
func (s *Store) hydrateTags(rows []VocabRow) error {
	if len(rows) == 0 {
		return nil
	}
	placeholders := strings.Repeat("?,", len(rows))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(rows))
	for i, r := range rows {
		args[i] = r.Lemma
	}
	q := fmt.Sprintf(`SELECT lemma, tag FROM word_tags WHERE lang = ? AND lemma IN (%s) ORDER BY tag`, placeholders)
	out, err := s.q().Query(q, append([]any{s.lang}, args...)...)
	if err != nil {
		return fmt.Errorf("hydrate tags: %w", err)
	}
	defer out.Close()
	byLemma := make(map[string][]string, len(rows))
	for out.Next() {
		var lemma, tag string
		if err := out.Scan(&lemma, &tag); err != nil {
			return err
		}
		byLemma[lemma] = append(byLemma[lemma], tag)
	}
	if err := out.Err(); err != nil {
		return err
	}
	for i := range rows {
		if tags := byLemma[rows[i].Lemma]; tags != nil {
			rows[i].Tags = tags
		}
	}
	return nil
}

// --- Tag CRUD -------------------------------------------------------------

// SetWordTags replaces the tag set for a lemma. All-or-nothing inside a tx.
// Mirrors /set_word_tags. Tags are normalized (trim + lowercase).
func (s *Store) SetWordTags(lemma string, tags []string) ([]string, error) {
	if lemma == "" {
		return nil, clientErrf("lemma is required")
	}
	tx, err := s.q().Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM word_tags WHERE lang = ? AND lemma = ?`, s.lang, lemma); err != nil {
		return nil, fmt.Errorf("clear tags: %w", err)
	}
	cleaned := make([]string, 0, len(tags))
	seen := make(map[string]bool, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(strings.ToLower(t))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		if _, err := tx.Exec(`INSERT INTO word_tags (lang, lemma, tag, kind) VALUES (?, ?, ?, ?)`, s.lang, lemma, t, tagKind(t)); err != nil {
			return nil, fmt.Errorf("insert tag %q: %w", t, err)
		}
		cleaned = append(cleaned, t)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tags: %w", err)
	}
	return cleaned, nil
}

// AddWordTag is the single-tag convenience version of SetWordTags. Idempotent
// — adding a tag that already exists is a no-op.
func (s *Store) AddWordTag(lemma, tag string) error {
	tag = strings.TrimSpace(strings.ToLower(tag))
	if lemma == "" || tag == "" {
		return clientErrf("lemma and tag are required")
	}
	_, err := s.q().Exec(
		`INSERT OR IGNORE INTO word_tags (lang, lemma, tag, kind) VALUES (?, ?, ?, ?)`,
		s.lang, lemma, tag, tagKind(tag),
	)
	return err
}

// TagCard applies (or removes, when remove is true) a system tag to the lemma
// of the given card. Only registered system tags are accepted here — free-form
// labels go through AddWordTag/SetWordTags. Tags are stored per-lemma, so the
// card's lemma is resolved first; the returned lemma is what was tagged.
func (s *Store) TagCard(cardID int64, tag string, remove bool) (string, error) {
	tag = strings.TrimSpace(strings.ToLower(tag))
	if !IsSystemTag(tag) {
		return "", clientErrf("%q is not a system tag; use add_word_tag for free-form tags", tag)
	}
	var lemma string
	if err := s.q().QueryRow(`SELECT lemma FROM cards WHERE id = ?`, cardID).Scan(&lemma); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", clientErrf("card %d not found", cardID)
		}
		return "", fmt.Errorf("look up card %d: %w", cardID, err)
	}
	if remove {
		_, err := s.q().Exec(`DELETE FROM word_tags WHERE lang = ? AND lemma = ? AND tag = ?`, s.lang, lemma, tag)
		return lemma, err
	}
	_, err := s.q().Exec(
		`INSERT OR IGNORE INTO word_tags (lang, lemma, tag, kind) VALUES (?, ?, ?, 'system')`,
		s.lang, lemma, tag,
	)
	if err == nil && tag == TagFurigana {
		_, err = demoteKnown(s.q(), s.lang, lemma, time.Now())
	}
	return lemma, err
}

// RemoveWordTag drops a single (lemma, tag) pair. Idempotent.
func (s *Store) RemoveWordTag(lemma, tag string) error {
	tag = strings.TrimSpace(strings.ToLower(tag))
	if lemma == "" || tag == "" {
		return clientErrf("lemma and tag are required")
	}
	_, err := s.q().Exec(`DELETE FROM word_tags WHERE lang = ? AND lemma = ? AND tag = ?`, s.lang, lemma, tag)
	return err
}

// FlagWord applies (or clears, when remove is true) a system tag on a lemma
// directly — the by-lemma counterpart to TagCard (which works by card id),
// used by the MCP tutor which curates words, not card ids. Rejects non-system
// tags so this path can't mint free-form labels.
func (s *Store) FlagWord(lemma, tag string, remove bool) error {
	lemma = strings.TrimSpace(lemma)
	tag = strings.TrimSpace(strings.ToLower(tag))
	if lemma == "" {
		return clientErrf("lemma is required")
	}
	if !IsSystemTag(tag) {
		return clientErrf("%q is not a system tag", tag)
	}
	if remove {
		_, err := s.q().Exec(`DELETE FROM word_tags WHERE lang = ? AND lemma = ? AND tag = ?`, s.lang, lemma, tag)
		return err
	}
	_, err := s.q().Exec(
		`INSERT OR IGNORE INTO word_tags (lang, lemma, tag, kind) VALUES (?, ?, ?, 'system')`,
		s.lang, lemma, tag,
	)
	return err
}

// TaggedWord is one (lemma, tag) pair from SystemTaggedWords.
type TaggedWord struct {
	Lemma string `json:"word"`
	Tag   string `json:"tag"`
}

// SystemTaggedWords lists the lemmas carrying system tags, for the tutor to
// triage follow-up work. With tag set, only that system tag; empty returns all
// system-tagged words (keyed off the kind column).
func (s *Store) SystemTaggedWords(tag string) ([]TaggedWord, error) {
	tag = strings.TrimSpace(strings.ToLower(tag))
	var (
		rows *sql.Rows
		err  error
	)
	if tag != "" {
		if !IsSystemTag(tag) {
			return nil, clientErrf("%q is not a system tag", tag)
		}
		rows, err = s.q().Query(`SELECT lemma, tag FROM word_tags WHERE lang = ? AND tag = ? ORDER BY lemma`, s.lang, tag)
	} else {
		rows, err = s.q().Query(`SELECT lemma, tag FROM word_tags WHERE lang = ? AND kind = 'system' ORDER BY tag, lemma`, s.lang)
	}
	if err != nil {
		return nil, fmt.Errorf("list system-tagged: %w", err)
	}
	defer rows.Close()
	out := []TaggedWord{}
	for rows.Next() {
		var tw TaggedWord
		if err := rows.Scan(&tw.Lemma, &tw.Tag); err != nil {
			return nil, err
		}
		out = append(out, tw)
	}
	return out, rows.Err()
}

// TagCount is one row in /list_all_tags — tag + usage count + kind
// ("system" for reserved app tags, "user" for free-form labels).
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
	Kind  string `json:"kind"`
}

// ListAllTags returns every tag in use, sorted by count desc then alpha.
// The vocab explorer surfaces the top N as chips.
func (s *Store) ListAllTags() ([]TagCount, error) {
	rows, err := s.q().Query(`
		SELECT tag, COUNT(*) as n
		FROM word_tags
		WHERE lang = ?
		GROUP BY tag
		ORDER BY n DESC, tag ASC
	`, s.lang)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	defer rows.Close()
	out := []TagCount{}
	for rows.Next() {
		var t TagCount
		if err := rows.Scan(&t.Tag, &t.Count); err != nil {
			return nil, err
		}
		t.Kind = tagKind(t.Tag) // registry is authoritative for system-ness
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteByTag is the safety hatch for Anki imports: drop every lemma carrying
// the given tag (and its cards + word_status rows). Words the learner has
// studied since (a review, review history or a note on any of their cards)
// are kept unless IncludeStudied is set. Media files move to media/.trash,
// where a snapshot restore finds them again. Wrapped in a tx so a partial
// delete is impossible.
type DeleteByTagReq struct {
	Tag            string `json:"tag"`
	IncludeStudied bool   `json:"include_studied,omitempty"`
	// DryRun reports what would be deleted and kept, and changes nothing.
	DryRun bool `json:"dry_run,omitempty"`
}

type DeleteByTagResp struct {
	DeletedLemmas int      `json:"deleted_lemmas"`
	KeptStudied   int      `json:"kept_studied"`
	KeptLemmas    []string `json:"kept_lemmas"`
	DryRun        bool     `json:"dry_run,omitempty"`
}

func (s *Store) DeleteByTag(req DeleteByTagReq) (DeleteByTagResp, error) {
	tag := strings.TrimSpace(strings.ToLower(req.Tag))
	if tag == "" {
		return DeleteByTagResp{}, clientErrf("tag is required")
	}
	// delete_by_tag nukes whole cards — it's the undo hatch for an import
	// batch. System tags mark cards for follow-up, not a batch to delete, so
	// refuse them (use remove_word_tag / tag_card to clear one).
	if IsSystemTag(tag) {
		return DeleteByTagResp{}, clientErrf("%q is a system tag, so its cards are not deleted; use remove_word_tag to clear it", tag)
	}
	// seen-in:<batch> marks words that were in the deck BEFORE that import.
	if strings.HasPrefix(tag, SeenInTagPrefix) {
		return DeleteByTagResp{}, clientErrf("%q marks words that were already in the deck before an import, so they are not deleted; undo the import with %q", tag, strings.TrimPrefix(tag, SeenInTagPrefix))
	}
	tx, err := s.q().Begin()
	if err != nil {
		return DeleteByTagResp{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Pull the lemmas to delete first — we need the count for the response.
	var lemmas []string
	rows, err := tx.Query(`SELECT lemma FROM word_tags WHERE lang = ? AND tag = ?`, s.lang, tag)
	if err != nil {
		return DeleteByTagResp{}, fmt.Errorf("find lemmas: %w", err)
	}
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			rows.Close()
			return DeleteByTagResp{}, err
		}
		lemmas = append(lemmas, l)
	}
	rows.Close()

	// Batch the IN clauses — SQLite's bind-variable limit (and query-string
	// size) make a single statement over a huge tag (e.g. a 100k-row import)
	// fragile. Same transaction, so atomicity is unchanged.
	const batch = 500
	inChunks := func(all []string, fn func(placeholders string, args []any) error) error {
		for start := 0; start < len(all); start += batch {
			end := min(start+batch, len(all))
			chunk := all[start:end]
			placeholders := strings.Repeat("?,", len(chunk))
			placeholders = placeholders[:len(placeholders)-1]
			args := make([]any, len(chunk))
			for i, l := range chunk {
				args[i] = l
			}
			if err := fn(placeholders, args); err != nil {
				return err
			}
		}
		return nil
	}

	// Studied words stay: the tag may be on a word the learner already had
	// (older imports tagged those too), and an undo must not take history.
	resp := DeleteByTagResp{KeptLemmas: []string{}, DryRun: req.DryRun}
	kept := map[string]bool{}
	if !req.IncludeStudied {
		err := inChunks(lemmas, func(placeholders string, args []any) error {
			rows, err := tx.Query(`
				SELECT DISTINCT c.lemma FROM cards c
				 WHERE c.lang = ? AND c.lemma IN (`+placeholders+`)
				   AND (c.reps > 0
				        OR EXISTS (SELECT 1 FROM review_log r WHERE r.card_id = c.id)
				        OR EXISTS (SELECT 1 FROM card_notes n WHERE n.card_id = c.id))`,
				append([]any{s.lang}, args...)...)
			if err != nil {
				return fmt.Errorf("find studied lemmas: %w", err)
			}
			defer rows.Close()
			for rows.Next() {
				var l string
				if err := rows.Scan(&l); err != nil {
					return err
				}
				kept[l] = true
			}
			return rows.Err()
		})
		if err != nil {
			return DeleteByTagResp{}, err
		}
	}
	var doomed []string
	for _, l := range lemmas {
		if kept[l] {
			resp.KeptLemmas = append(resp.KeptLemmas, l)
		} else {
			doomed = append(doomed, l)
		}
	}
	resp.KeptStudied = len(resp.KeptLemmas)
	resp.DeletedLemmas = len(doomed)
	if req.DryRun {
		return resp, nil
	}

	var mediaFiles []string
	err = inChunks(doomed, func(placeholders string, args []any) error {
		// Collect the chunk's media sidecar filenames BEFORE the card rows go:
		// card_media rows cascade with the cards, but their files on disk don't,
		// so we move them to the trash explicitly to avoid leaking them.
		fileRows, err := tx.Query(
			`SELECT COALESCE(c.audio_file,''), COALESCE(c.image_file,''), COALESCE(cm.file,'')
			   FROM cards c LEFT JOIN card_media cm ON cm.card_id = c.id
			  WHERE c.lang = ? AND c.lemma IN (`+placeholders+`)`, append([]any{s.lang}, args...)...)
		if err != nil {
			return fmt.Errorf("collect media: %w", err)
		}
		for fileRows.Next() {
			var a, i, m string
			if err := fileRows.Scan(&a, &i, &m); err != nil {
				fileRows.Close()
				return err
			}
			for _, f := range []string{a, i, m} {
				if f != "" {
					mediaFiles = append(mediaFiles, f)
				}
			}
		}
		fileRows.Close()
		for _, t := range []string{
			`DELETE FROM cards       WHERE lang = ? AND lemma IN (` + placeholders + `)`,
			`DELETE FROM word_status WHERE lang = ? AND lemma IN (` + placeholders + `)`,
			`DELETE FROM word_tags   WHERE lang = ? AND lemma IN (` + placeholders + `)`,
		} {
			if _, err := tx.Exec(t, append([]any{s.lang}, args...)...); err != nil {
				return fmt.Errorf("delete: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return DeleteByTagResp{}, err
	}
	// The import's markers on words that were already in the deck go with it.
	if _, err := tx.Exec(`DELETE FROM word_tags WHERE lang = ? AND tag = ?`, s.lang, SeenInTagPrefix+tag); err != nil {
		return DeleteByTagResp{}, fmt.Errorf("clear seen-in tag: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return DeleteByTagResp{}, fmt.Errorf("commit: %w", err)
	}
	// Best-effort, after the commit — a stale file is not worth failing the
	// call. Trashed rather than removed, so restoring the import's snapshot
	// brings the audio back.
	s.trashMedia(mediaFiles)
	return resp, nil
}
