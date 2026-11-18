package srs

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yumejp/yume/server/internal/dict"
)

// jsonUnmarshal is a thin alias so call sites read clearly. Yomitan
// definition payloads are arbitrary JSON; we decode into `any`.
func jsonUnmarshal(s string, v any) error {
	return json.Unmarshal([]byte(s), v)
}

// This file handles imported Yomitan dictionaries: their term banks (extra
// definitions merged into lookups), pitch-accent data, and frequency data.
//
// Import protocol (driven by the frontend's importYomitanZip):
//   1. create_dict        → row in `dicts`, returns dict_id
//   2. import_dict_chunk   → N rows in `dict_entries`  (repeated)
//   3. import_pitch_chunk  → N rows in `pitches`        (repeated)
//   4. import_freq_chunk   → N rows in `frequencies`    (repeated)
// The frontend parses the .zip and chunks the upload; the server just stores.

// ImportedDict mirrors the /list_dicts response row shape.
type ImportedDict struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Lang string `json:"lang"`
	// GlossLang is the language of the definitions (en, ja, …).
	GlossLang   string `json:"gloss_lang"`
	Revision    string `json:"revision"`
	Author      string `json:"author"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	EntryCount  int    `json:"entry_count"`
	ImportedAt  string `json:"imported_at"`
}

// CreateDictReq is the create_dict payload.
type CreateDictReq struct {
	Name string `json:"name"`
	Lang string `json:"lang,omitempty"` // "" = the store's language
	// GlossLang is the language of the definitions; "" = en.
	GlossLang string `json:"gloss_lang,omitempty"`

	Revision    string `json:"revision"`
	Author      string `json:"author"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

// glossLangOr defaults a dictionary's definition language to English.
func glossLangOr(l string) string {
	if strings.TrimSpace(l) == "" {
		return "en"
	}
	return normLang(l)
}

// langFor resolves a request's optional language against this view's.
func (s *Store) langFor(lang string) string {
	if strings.TrimSpace(lang) == "" {
		return s.lang
	}
	return normLang(lang)
}

// CreateDict inserts a dict row and returns its id. Enabled by default.
func (s *Store) CreateDict(req CreateDictReq) (int64, error) {
	if strings.TrimSpace(req.Name) == "" {
		return 0, clientErrf("dict name is required")
	}
	res, err := s.q().Exec(`
		INSERT INTO dicts (name, lang, gloss_lang, revision, author, url, description, enabled, imported_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?)
	`,
		req.Name, s.langFor(req.Lang), glossLangOr(req.GlossLang),
		nullableStr(req.Revision), nullableStr(req.Author),
		nullableStr(req.URL), nullableStr(req.Description),
		time.Now().Unix(),
	)
	if err != nil {
		return 0, fmt.Errorf("insert dict: %w", err)
	}
	return res.LastInsertId()
}

// NormalizedTerm matches the frontend's NormalizedTerm shape.
type NormalizedTerm struct {
	Term            string  `json:"term"`
	Reading         string  `json:"reading"`
	DefinitionsJSON string  `json:"definitions_json"`
	Popularity      float64 `json:"popularity"`
}

// ImportTermChunk bulk-inserts a chunk of term-bank entries inside one tx.

// maxImportChunk bounds one import_*_chunk call. The frontend already sends
// modest chunks; this is a backstop against a runaway client melting one
// transaction (the middleware's 64 MiB body cap bounds bytes, not rows).
const maxImportChunk = 50000

func (s *Store) ImportTermChunk(dictID int64, terms []NormalizedTerm) (int, error) {
	if len(terms) > maxImportChunk {
		return 0, clientErrf("chunk too large (%d rows; max %d); send smaller chunks", len(terms), maxImportChunk)
	}
	if len(terms) == 0 {
		return 0, nil
	}
	tx, err := s.q().Begin()
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.Prepare(`
		INSERT INTO dict_entries (dict_id, term, reading, definitions_json, popularity)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return 0, fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()
	n := 0
	for _, t := range terms {
		if t.Term == "" || t.DefinitionsJSON == "" {
			continue
		}
		if _, err := stmt.Exec(dictID, t.Term, nullableStr(t.Reading), t.DefinitionsJSON, t.Popularity); err != nil {
			return 0, fmt.Errorf("insert term: %w", err)
		}
		n++
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return n, nil
}

// NormalizedPitch matches the frontend's NormalizedPitch shape.
type NormalizedPitch struct {
	Expression string `json:"expression"`
	Reading    string `json:"reading"`
	Position   int    `json:"position"`
}

// ImportPitchChunk bulk-inserts pitch rows.
func (s *Store) ImportPitchChunk(dictID int64, pitches []NormalizedPitch) (int, error) {
	if len(pitches) > maxImportChunk {
		return 0, clientErrf("chunk too large (%d rows; max %d); send smaller chunks", len(pitches), maxImportChunk)
	}
	if len(pitches) == 0 {
		return 0, nil
	}
	tx, err := s.q().Begin()
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.Prepare(`INSERT INTO pitches (dict_id, expression, reading, position) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return 0, fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()
	n := 0
	for _, p := range pitches {
		if p.Expression == "" {
			continue
		}
		if _, err := stmt.Exec(dictID, p.Expression, nullableStr(p.Reading), p.Position); err != nil {
			return 0, fmt.Errorf("insert pitch: %w", err)
		}
		n++
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return n, nil
}

// NormalizedFreq matches the frontend's NormalizedFreq shape.
type NormalizedFreq struct {
	Expression string `json:"expression"`
	Reading    string `json:"reading"`
	Value      int    `json:"value"`
	Display    string `json:"display"`
}

// ImportFreqChunk bulk-inserts frequency rows.
func (s *Store) ImportFreqChunk(dictID int64, freqs []NormalizedFreq) (int, error) {
	if len(freqs) > maxImportChunk {
		return 0, clientErrf("chunk too large (%d rows; max %d); send smaller chunks", len(freqs), maxImportChunk)
	}
	if len(freqs) == 0 {
		return 0, nil
	}
	tx, err := s.q().Begin()
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.Prepare(`INSERT INTO frequencies (dict_id, expression, reading, value, display) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()
	n := 0
	for _, f := range freqs {
		if f.Expression == "" {
			continue
		}
		if _, err := stmt.Exec(dictID, f.Expression, nullableStr(f.Reading), f.Value, nullableStr(f.Display)); err != nil {
			return 0, fmt.Errorf("insert freq: %w", err)
		}
		n++
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return n, nil
}

// ListDicts returns all imported dicts with their entry counts.
func (s *Store) ListDicts() ([]ImportedDict, error) {
	rows, err := s.q().Query(`
		SELECT d.id, d.name, d.lang, d.gloss_lang, d.revision, d.author, d.url, d.description,
		       d.enabled, d.imported_at,
		       (SELECT COUNT(*) FROM dict_entries e WHERE e.dict_id = d.id) AS entry_count
		FROM dicts d
		WHERE d.lang = ?
		ORDER BY d.imported_at DESC
	`, s.lang)
	if err != nil {
		return nil, fmt.Errorf("list dicts: %w", err)
	}
	defer rows.Close()
	out := []ImportedDict{}
	for rows.Next() {
		var d ImportedDict
		var revision, author, url, description sql.NullString
		var enabled int
		var importedAt int64
		if err := rows.Scan(
			&d.ID, &d.Name, &d.Lang, &d.GlossLang, &revision, &author, &url, &description,
			&enabled, &importedAt, &d.EntryCount,
		); err != nil {
			return nil, fmt.Errorf("scan dict: %w", err)
		}
		d.Revision = revision.String
		d.Author = author.String
		d.URL = url.String
		d.Description = description.String
		d.Enabled = enabled != 0
		d.ImportedAt = time.Unix(importedAt, 0).UTC().Format(time.RFC3339)
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeleteDict removes a dict and all its entries/pitches/frequencies. The
// FK ON DELETE CASCADE handles the children once foreign_keys=ON (set in
// Open's PRAGMAs), but we delete explicitly too so it works regardless.
func (s *Store) DeleteDict(dictID int64) (int, error) {
	tx, err := s.q().Begin()
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, q := range []string{
		`DELETE FROM dict_entries WHERE dict_id = ?`,
		`DELETE FROM pitches      WHERE dict_id = ?`,
		`DELETE FROM frequencies  WHERE dict_id = ?`,
	} {
		if _, err := tx.Exec(q, dictID); err != nil {
			return 0, fmt.Errorf("delete children: %w", err)
		}
	}
	res, err := tx.Exec(`DELETE FROM dicts WHERE id = ?`, dictID)
	if err != nil {
		return 0, fmt.Errorf("delete dict: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// SetDictEnabled flips the enabled flag — disabled dicts are skipped in
// lookups but their data stays on disk.
func (s *Store) SetDictEnabled(dictID int64, enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	_, err := s.q().Exec(`UPDATE dicts SET enabled = ? WHERE id = ?`, v, dictID)
	return err
}

// --- Lookup integration ---------------------------------------------------

// LookupImported returns extra dict.Entry values from enabled imported dicts
// for the given queries. These are appended to the JMdict results in the
// /lookup handler. We synthesize negative IDs (-dict_entry_id) so they never
// collide with real JMdict sequence numbers and a client's dedupe-by-id
// keeps them distinct.
func (s *Store) LookupImported(queries []string) (map[string][]dict.Entry, error) {
	out := make(map[string][]dict.Entry, len(queries))
	if len(queries) == 0 {
		return out, nil
	}
	// Deduplicate queries to avoid redundant DB hits.
	uniq := make(map[string]bool, len(queries))
	for _, q := range queries {
		uniq[q] = true
	}
	for q := range uniq {
		rows, err := s.q().Query(`
			SELECT e.id, e.term, e.reading, e.definitions_json, d.name, d.gloss_lang
			FROM dict_entries e
			JOIN dicts d ON d.id = e.dict_id
			WHERE d.enabled = 1 AND d.lang = ? AND e.term = ?
			ORDER BY e.popularity DESC
		`, s.lang, q)
		if err != nil {
			return nil, fmt.Errorf("lookup imported: %w", err)
		}
		var entries []dict.Entry
		for rows.Next() {
			var id int64
			var term string
			var reading sql.NullString
			var defsJSON, dictName, glossLang string
			if err := rows.Scan(&id, &term, &reading, &defsJSON, &dictName, &glossLang); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan imported: %w", err)
			}
			e := dict.Entry{
				ID:     int(-id), // negative → never collides with JMdict
				Dict:   dictName,
				Lang:   glossLang,
				Kanji:  []string{term},
				Senses: []dict.Sense{{Glosses: flattenDefinitions(defsJSON)}},
			}
			if reading.Valid && reading.String != "" {
				e.Readings = []string{reading.String}
			}
			entries = append(entries, e)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		if len(entries) > 0 {
			out[q] = entries
		}
	}
	return out, nil
}

// AnnotatePitchFreq mutates the given entries in place, attaching pitch
// positions (parallel to each entry's Readings) and a frequency rank when an
// enabled pitch/freq dict has data for the entry's surface forms.
func (s *Store) AnnotatePitchFreq(entries []dict.Entry) error {
	for i := range entries {
		e := &entries[i]
		forms := append(append([]string{}, e.Kanji...), e.Readings...)
		if len(forms) == 0 {
			continue
		}

		// Pitch: for each reading, find a matching (expression in forms,
		// reading = thisReading) row from an enabled dict.
		if len(e.Readings) > 0 {
			pitches := make([]*int, len(e.Readings))
			any := false
			for ri, reading := range e.Readings {
				pos, ok, err := s.pitchFor(forms, reading)
				if err != nil {
					return err
				}
				if ok {
					p := pos
					pitches[ri] = &p
					any = true
				}
			}
			if any {
				e.Pitches = pitches
			}
		}

		// Frequency: min value across enabled freq dicts for any surface form.
		freq, ok, err := s.freqFor(forms)
		if err != nil {
			return err
		}
		if ok {
			f := freq
			e.Frequency = &f
		}
	}
	return nil
}

// pitchFor finds the pitch position for (expression IN forms, reading=reading)
// from an enabled dict. Returns the first match (Yomitan dicts agree on the
// common cases; clients only show one anyway).
func (s *Store) pitchFor(forms []string, reading string) (int, bool, error) {
	if len(forms) == 0 {
		return 0, false, nil
	}
	placeholders := strings.Repeat("?,", len(forms))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, 0, len(forms)+1)
	for _, f := range forms {
		args = append(args, f)
	}
	args = append(args, reading)
	args = append([]any{s.lang}, args...)
	var position int
	err := s.q().QueryRow(fmt.Sprintf(`
		SELECT p.position
		FROM pitches p
		JOIN dicts d ON d.id = p.dict_id
		WHERE d.enabled = 1 AND d.lang = ?
		  AND p.expression IN (%s)
		  AND (p.reading = ? OR p.reading IS NULL OR p.reading = '')
		LIMIT 1
	`, placeholders), args...).Scan(&position)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("pitch_for: %w", err)
	}
	return position, true, nil
}

// freqFor returns the lowest (most common) frequency value across enabled
// freq dicts for any of the surface forms.
func (s *Store) freqFor(forms []string) (int, bool, error) {
	if len(forms) == 0 {
		return 0, false, nil
	}
	placeholders := strings.Repeat("?,", len(forms))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(forms))
	for i, f := range forms {
		args[i] = f
	}
	var value sql.NullInt64
	err := s.q().QueryRow(fmt.Sprintf(`
		SELECT MIN(f.value)
		FROM frequencies f
		JOIN dicts d ON d.id = f.dict_id
		WHERE d.enabled = 1 AND d.lang = ? AND f.expression IN (%s)
	`, placeholders), append([]any{s.lang}, args...)...).Scan(&value)
	if err != nil {
		return 0, false, fmt.Errorf("freq_for: %w", err)
	}
	if !value.Valid {
		return 0, false, nil
	}
	return int(value.Int64), true, nil
}

// flattenDefinitions turns Yomitan's `definitions` JSON into a flat []string
// of human-readable glosses. Yomitan definitions can be:
//   - plain strings
//   - structured content objects (we extract their text recursively)
//
// We keep it pragmatic: strings pass through, objects get their string
// leaves concatenated.
func flattenDefinitions(defsJSON string) []string {
	var raw []any
	if err := jsonUnmarshal(defsJSON, &raw); err != nil {
		// If it doesn't parse as an array, return the raw text as a single
		// gloss so the user at least sees something.
		t := strings.TrimSpace(defsJSON)
		if t == "" {
			return nil
		}
		return []string{t}
	}
	var out []string
	for _, item := range raw {
		out = append(out, extractText(item)...)
	}
	return out
}

// extractText pulls human-readable strings out of a Yomitan definition
// element (string, or structured-content object/array).
func extractText(v any) []string {
	switch t := v.(type) {
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return nil
		}
		return []string{s}
	case []any:
		var out []string
		for _, e := range t {
			out = append(out, extractText(e)...)
		}
		return out
	case map[string]any:
		// Structured content. The text usually lives in "content" (string
		// or nested array) or a "text" field.
		var out []string
		if c, ok := t["content"]; ok {
			out = append(out, extractText(c)...)
		}
		if txt, ok := t["text"].(string); ok {
			if s := strings.TrimSpace(txt); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
