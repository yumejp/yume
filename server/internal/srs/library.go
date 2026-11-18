package srs

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/yumejp/yume/server/internal/dbsnap"
)

// The library is content every deck on this server shares, keyed by
// (lang, lemma) or (lang, kanji): mnemonics and kanji stories. It lives in
// <dataRoot>/content.db beside the per-user decks. A deck carries its own
// copy only as an override; what a learner sees is the deck's, else the
// library's.

// Library is the shared content database.
type Library struct {
	db *sql.DB
}

var (
	libraryMu sync.RWMutex
	library   *Library
)

// SetLibrary installs the process-wide library (nil: none; decks then show
// only their own mnemonics, which is how tests run).
func SetLibrary(l *Library) {
	libraryMu.Lock()
	library = l
	libraryMu.Unlock()
}

// SharedLibrary returns the installed library, or nil.
func SharedLibrary() *Library {
	libraryMu.RLock()
	defer libraryMu.RUnlock()
	return library
}

// content.db schema, versioned in PRAGMA user_version with the decks' rules
// (migrations.go): append-only, one transaction each. Builds before this list
// stamped version 2 on every open, so version 2 is the shape every existing
// file already has, and a file at 0 or 1 moves to 2 without changing.
var libraryMigrations = []struct {
	version int
	name    string
	stmts   []string
}{
	{2, "mnemonics + kanji_notes", []string{
		`CREATE TABLE IF NOT EXISTS mnemonics (
			lang       TEXT NOT NULL,
			lemma      TEXT NOT NULL,
			parts      TEXT,
			story      TEXT NOT NULL,
			created_at INTEGER NOT NULL CHECK (typeof(created_at) = 'integer'),
			updated_at INTEGER NOT NULL CHECK (typeof(updated_at) = 'integer'),
			PRIMARY KEY (lang, lemma)
		)`,
		`CREATE TABLE IF NOT EXISTS kanji_notes (
			lang       TEXT NOT NULL,
			kanji      TEXT NOT NULL,
			note       TEXT NOT NULL,
			created_at INTEGER NOT NULL CHECK (typeof(created_at) = 'integer'),
			updated_at INTEGER NOT NULL CHECK (typeof(updated_at) = 'integer'),
			PRIMARY KEY (lang, kanji)
		)`,
	}},
}

// OpenLibrary opens (creating if needed) the shared content database.
func OpenLibrary(path string) (*Library, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open library: %w", err)
	}
	registerPragmaHook()
	db.SetMaxIdleConns(2)
	db.SetConnMaxIdleTime(5 * time.Minute)
	if err := migrateLibrary(db, path); err != nil {
		db.Close()
		return nil, err
	}
	return &Library{db: db}, nil
}

// migrateLibrary brings content.db up to date, snapshotting a versioned file
// first (backups/content-premigrate-*) like the decks do.
func migrateLibrary(db *sql.DB, path string) error {
	var current int
	if err := db.QueryRow("PRAGMA user_version").Scan(&current); err != nil {
		return fmt.Errorf("content.db user_version: %w", err)
	}
	latest := libraryMigrations[len(libraryMigrations)-1].version
	if current > latest {
		return schemaErr{ErrKindSchemaNewer, fmt.Errorf(
			"content.db schema %d is newer than supported schema %d: a newer Yume build wrote it; run that build, or restore the snapshot taken before the upgrade (docs/backup-and-restore.md)",
			current, latest)}
	}
	if current > 0 && current < latest {
		if _, err := dbsnap.BeforeMigrate(db, path, current); err != nil {
			return fmt.Errorf("content.db snapshot before upgrading schema %d, so nothing was changed: %w", current, err)
		}
	}
	for _, m := range libraryMigrations {
		if m.version <= current {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("content.db begin migration %d: %w", m.version, err)
		}
		for _, q := range m.stmts {
			if _, err := tx.Exec(q); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("content.db migration %d (%s): %w", m.version, m.name, err)
			}
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", m.version)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("content.db set user_version %d: %w", m.version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("content.db commit migration %d: %w", m.version, err)
		}
	}
	return nil
}

// Close shuts the library down. Idempotent.
func (l *Library) Close() error {
	if l == nil || l.db == nil {
		return nil
	}
	return l.db.Close()
}

// SharedMnemonic is one library row; the export/import wire shape.
type SharedMnemonic struct {
	Lang  string         `json:"lang"`
	Lemma string         `json:"lemma"`
	Parts []MnemonicPart `json:"parts"`
	Story string         `json:"story"`
}

// Mnemonic returns the library's mnemonic for a lemma, or nil.
func (l *Library) Mnemonic(lang, lemma string) (*Mnemonic, error) {
	m, err := l.Mnemonics(lang, []string{lemma})
	if err != nil {
		return nil, err
	}
	return m[lemma], nil
}

// Mnemonics returns the library's mnemonics for the lemmas that have one.
func (l *Library) Mnemonics(lang string, lemmas []string) (map[string]*Mnemonic, error) {
	out := map[string]*Mnemonic{}
	if l == nil || len(lemmas) == 0 {
		return out, nil
	}
	// SQLite's default parameter cap is 999; chunk like /card_status does.
	const chunk = 400
	for start := 0; start < len(lemmas); start += chunk {
		end := start + chunk
		if end > len(lemmas) {
			end = len(lemmas)
		}
		args := make([]any, 0, end-start+1)
		args = append(args, lang)
		for _, lemma := range lemmas[start:end] {
			args = append(args, lemma)
		}
		q := `SELECT lemma, parts, story FROM mnemonics WHERE lang = ? AND lemma IN (?` +
			strings.Repeat(",?", end-start-1) + `)`
		rows, err := l.db.Query(q, args...)
		if err != nil {
			return nil, fmt.Errorf("library mnemonics: %w", err)
		}
		for rows.Next() {
			var lemma string
			var parts, story sql.NullString
			if err := rows.Scan(&lemma, &parts, &story); err != nil {
				rows.Close()
				return nil, err
			}
			if m := decodeMnemonic(parts, story); m != nil {
				out[lemma] = m
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// SetMnemonic validates and writes the library's mnemonic for a lemma,
// replacing any existing one. Empty parts and story delete it. Reports
// whether the lemma had one.
func (l *Library) SetMnemonic(lang, lemma string, parts []MnemonicPart, story string) (replaced bool, err error) {
	lemma = strings.TrimSpace(lemma)
	if lemma == "" {
		return false, clientErrf("lemma is required")
	}
	had, err := l.Mnemonic(lang, lemma)
	if err != nil {
		return false, err
	}
	replaced = had != nil
	story = strings.TrimSpace(story)
	if story == "" && len(parts) == 0 {
		_, err := l.db.Exec(`DELETE FROM mnemonics WHERE lang = ? AND lemma = ?`, lang, lemma)
		return replaced, err
	}
	parts, err = validateMnemonic(lemma, parts, story)
	if err != nil {
		return false, err
	}
	now := time.Now().Unix()
	_, err = l.db.Exec(`INSERT INTO mnemonics (lang, lemma, parts, story, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(lang, lemma) DO UPDATE SET parts = excluded.parts, story = excluded.story, updated_at = excluded.updated_at`,
		lang, lemma, encodeMnemonicParts(parts), story, now, now)
	return replaced, err
}

// SingleKanjiParts returns the parts of the library mnemonic for the
// shortest lemma whose only kanji is `kanji` (薄い for 薄), or nil.
func (l *Library) SingleKanjiParts(lang, kanji string) ([]MnemonicPart, error) {
	if l == nil {
		return nil, nil
	}
	rows, err := l.db.Query(`SELECT lemma, parts FROM mnemonics WHERE lang = ? AND instr(lemma, ?) > 0`, lang, kanji)
	if err != nil {
		return nil, fmt.Errorf("library kanji parts: %w", err)
	}
	defer rows.Close()
	var best string
	var bestParts sql.NullString
	for rows.Next() {
		var lemma string
		var parts sql.NullString
		if err := rows.Scan(&lemma, &parts); err != nil {
			return nil, err
		}
		if singleKanjiLemma(lemma, best) {
			best, bestParts = lemma, parts
		}
	}
	return decodeParts(bestParts), rows.Err()
}

// KanjiNote returns the library's story for a kanji, or "".
func (l *Library) KanjiNote(lang, kanji string) (string, error) {
	if l == nil {
		return "", nil
	}
	var note string
	err := l.db.QueryRow(`SELECT note FROM kanji_notes WHERE lang = ? AND kanji = ?`, lang, kanji).Scan(&note)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return note, err
}

// SetKanjiNote upserts the library's story for a kanji; empty deletes it.
func (l *Library) SetKanjiNote(lang, kanji, note string) error {
	if err := validKanji(kanji); err != nil {
		return err
	}
	note = strings.TrimSpace(note)
	if note == "" {
		_, err := l.db.Exec(`DELETE FROM kanji_notes WHERE lang = ? AND kanji = ?`, lang, kanji)
		return err
	}
	if err := checkText("note", note); err != nil {
		return err
	}
	now := time.Now().Unix()
	_, err := l.db.Exec(`INSERT INTO kanji_notes (lang, kanji, note, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(lang, kanji) DO UPDATE SET note = excluded.note, updated_at = excluded.updated_at`,
		lang, kanji, note, now, now)
	return err
}

// SharedKanjiNote is one kanji story row; the export/import wire shape.
type SharedKanjiNote struct {
	Lang  string `json:"lang"`
	Kanji string `json:"kanji"`
	Note  string `json:"note"`
}

// LibraryContent is the export file: everything the library holds.
type LibraryContent struct {
	Mnemonics  []SharedMnemonic  `json:"mnemonics"`
	KanjiNotes []SharedKanjiNote `json:"kanji_notes"`
}

// ExportAll writes the whole library for a language (every language when
// empty).
func (l *Library) ExportAll(lang string) (LibraryContent, error) {
	var out LibraryContent
	var err error
	if out.Mnemonics, err = l.Export(lang); err != nil {
		return out, err
	}
	rows, err := l.db.Query(`SELECT lang, kanji, note FROM kanji_notes WHERE ? = '' OR lang = ? ORDER BY lang, kanji`, lang, lang)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	out.KanjiNotes = []SharedKanjiNote{}
	for rows.Next() {
		var k SharedKanjiNote
		if err := rows.Scan(&k.Lang, &k.Kanji, &k.Note); err != nil {
			return out, err
		}
		out.KanjiNotes = append(out.KanjiNotes, k)
	}
	return out, rows.Err()
}

// ImportCounts reports what an import did.
type ImportCounts struct{ Added, Replaced, Skipped int }

// ImportAll loads an export file: entries the library lacks are added; with
// overwrite, existing ones are replaced too.
func (l *Library) ImportAll(c LibraryContent, overwrite bool) (ImportCounts, error) {
	added, replaced, skipped, err := l.Import(c.Mnemonics, overwrite)
	n := ImportCounts{added, replaced, skipped}
	if err != nil {
		return n, err
	}
	for _, k := range c.KanjiNotes {
		lang := k.Lang
		if lang == "" {
			lang = DefaultLang
		}
		had, err := l.KanjiNote(lang, k.Kanji)
		if err != nil {
			return n, err
		}
		if had != "" && !overwrite {
			n.Skipped++
			continue
		}
		if err := l.SetKanjiNote(lang, k.Kanji, k.Note); err != nil {
			if IsClientErr(err) {
				n.Skipped++
				continue
			}
			return n, err
		}
		if had != "" {
			n.Replaced++
		} else {
			n.Added++
		}
	}
	return n, nil
}

// ShareKanjiNotes copies this deck's kanji stories into the library where it
// has none and clears the copies that then match it.
func (s *Store) ShareKanjiNotes(l *Library) (shared, cleared int, err error) {
	rows, err := s.q().Query(`SELECT kanji, note FROM kanji_notes WHERE lang = ? ORDER BY kanji`, s.lang)
	if err != nil {
		return 0, 0, err
	}
	var own [][2]string
	for rows.Next() {
		var k, n string
		if err := rows.Scan(&k, &n); err != nil {
			rows.Close()
			return 0, 0, err
		}
		own = append(own, [2]string{k, strings.TrimSpace(n)})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}
	for _, o := range own {
		had, err := l.KanjiNote(s.lang, o[0])
		if err != nil {
			return shared, cleared, err
		}
		if had == "" {
			if err := l.SetKanjiNote(s.lang, o[0], o[1]); err != nil {
				if IsClientErr(err) {
					continue // the deck keeps a story the library's rules reject
				}
				return shared, cleared, err
			}
			shared++
			had = o[1]
		}
		if had == o[1] {
			if _, err := s.q().Exec(`DELETE FROM kanji_notes WHERE lang = ? AND kanji = ?`, s.lang, o[0]); err != nil {
				return shared, cleared, err
			}
			cleared++
		}
	}
	return shared, cleared, nil
}

// Export lists the library's mnemonics for a language (every language when
// empty), lemma order.
func (l *Library) Export(lang string) ([]SharedMnemonic, error) {
	rows, err := l.db.Query(`SELECT lang, lemma, parts, story FROM mnemonics WHERE ? = '' OR lang = ? ORDER BY lang, lemma`, lang, lang)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SharedMnemonic{}
	for rows.Next() {
		var e SharedMnemonic
		var parts, story sql.NullString
		if err := rows.Scan(&e.Lang, &e.Lemma, &parts, &story); err != nil {
			return nil, err
		}
		m := decodeMnemonic(parts, story)
		if m == nil {
			continue
		}
		e.Parts, e.Story = m.Parts, m.Story
		out = append(out, e)
	}
	return out, rows.Err()
}

// Import adds entries the library lacks; with overwrite, existing ones are
// replaced too. Invalid entries are skipped and counted.
func (l *Library) Import(entries []SharedMnemonic, overwrite bool) (added, replaced, skipped int, err error) {
	for _, e := range entries {
		lang := e.Lang
		if lang == "" {
			lang = DefaultLang
		}
		had, err := l.Mnemonic(lang, e.Lemma)
		if err != nil {
			return added, replaced, skipped, err
		}
		if had != nil && !overwrite {
			skipped++
			continue
		}
		if _, err := l.SetMnemonic(lang, e.Lemma, e.Parts, e.Story); err != nil {
			if IsClientErr(err) {
				skipped++
				continue
			}
			return added, replaced, skipped, err
		}
		if had != nil {
			replaced++
		} else {
			added++
		}
	}
	return added, replaced, skipped, nil
}

// ShareMnemonics copies this deck's card mnemonics into the library where it
// has none, then clears every card copy that now matches the library, so
// what remains on cards is only overrides. It reports how many entries the
// library gained and how many card copies became library reads.
func (s *Store) ShareMnemonics(l *Library) (shared, cleared int, err error) {
	rows, err := s.q().Query(`SELECT id, lemma, mnemonic_parts, mnemonic_story FROM cards
		WHERE lang = ? AND (mnemonic_parts IS NOT NULL OR mnemonic_story IS NOT NULL) ORDER BY id`, s.lang)
	if err != nil {
		return 0, 0, err
	}
	type own struct {
		id    int64
		lemma string
		m     *Mnemonic
	}
	var owns []own
	for rows.Next() {
		var o own
		var parts, story sql.NullString
		if err := rows.Scan(&o.id, &o.lemma, &parts, &story); err != nil {
			rows.Close()
			return 0, 0, err
		}
		if o.m = decodeMnemonic(parts, story); o.m != nil {
			owns = append(owns, o)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}
	for _, o := range owns {
		had, err := l.Mnemonic(s.lang, o.lemma)
		if err != nil {
			return shared, cleared, err
		}
		if had == nil {
			if _, err := l.SetMnemonic(s.lang, o.lemma, o.m.Parts, o.m.Story); err != nil {
				if IsClientErr(err) {
					continue // the card keeps a copy the library's rules reject
				}
				return shared, cleared, err
			}
			shared++
			had = o.m
		}
		if sameMnemonic(had, o.m) {
			if _, err := s.q().Exec(`UPDATE cards SET mnemonic_parts = NULL, mnemonic_story = NULL WHERE id = ?`, o.id); err != nil {
				return shared, cleared, err
			}
			cleared++
		}
	}
	return shared, cleared, nil
}

func sameMnemonic(a, b *Mnemonic) bool {
	if a == nil || b == nil || a.Story != b.Story || len(a.Parts) != len(b.Parts) {
		return false
	}
	for i := range a.Parts {
		if a.Parts[i] != b.Parts[i] {
			return false
		}
	}
	return true
}

// fillSharedMnemonics gives every card without a mnemonic of its own the
// library's, when there is one. get/set address the caller's rows.
func (s *Store) fillSharedMnemonics(n int, get func(i int) (lemma string, own *Mnemonic), set func(i int, m *Mnemonic)) error {
	l := SharedLibrary()
	if l == nil || n == 0 {
		return nil
	}
	want := map[string][]int{}
	var lemmas []string
	for i := 0; i < n; i++ {
		lemma, own := get(i)
		if own != nil {
			continue
		}
		if _, seen := want[lemma]; !seen {
			lemmas = append(lemmas, lemma)
		}
		want[lemma] = append(want[lemma], i)
	}
	shared, err := l.Mnemonics(s.lang, lemmas)
	if err != nil {
		return err
	}
	for lemma, idx := range want {
		m := shared[lemma]
		if m == nil {
			continue
		}
		for _, i := range idx {
			set(i, m)
		}
	}
	return nil
}
