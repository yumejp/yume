package srs

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// KanjiWord is one deck word containing a kanji, with its word_status.
type KanjiWord struct {
	CardID   int64    `json:"card_id"`
	Lemma    string   `json:"lemma"`
	Readings []string `json:"readings,omitempty"`
	Status   int      `json:"status"`
}

// KanjiInfo is the /kanji_info response: the tutor's story for the kanji (if
// any) and every card in the deck whose lemma contains it.
type KanjiInfo struct {
	Kanji string `json:"kanji"`
	Note  string `json:"note,omitempty"`
	// NoteShared: the note is the server's shared one, not this deck's own.
	NoteShared bool        `json:"note_shared,omitempty"`
	Words      []KanjiWord `json:"words"`
	// Known: the kanji appears in a word this deck marks Known.
	Known bool `json:"known"`
	// Components: the parts the kanji is explained through (from the
	// mnemonic of its single-kanji word), each with its own story, so a
	// learner meeting an unfamiliar part reads it without leaving the sheet.
	Components []KanjiComponent `json:"components,omitempty"`
}

// KanjiComponent is one part a kanji is explained through.
type KanjiComponent struct {
	Kanji      string `json:"kanji"`
	Meaning    string `json:"meaning"`
	Note       string `json:"note,omitempty"`
	NoteShared bool   `json:"note_shared,omitempty"`
	Known      bool   `json:"known"`
}

func validKanji(k string) error {
	r, n := utf8.DecodeRuneInString(k)
	if n == 0 || n != len(k) || !unicode.Is(unicode.Han, r) {
		return clientErrf("kanji must be a single kanji character")
	}
	return nil
}

// KanjiInfo lists the deck's words that use `kanji` (known first, then
// learning, then unknown) and the kanji's note.
func (s *Store) KanjiInfo(kanji string) (KanjiInfo, error) {
	if err := validKanji(kanji); err != nil {
		return KanjiInfo{}, err
	}
	out := KanjiInfo{Kanji: kanji, Words: []KanjiWord{}}
	var err error
	if out.Note, out.NoteShared, err = s.kanjiNote(kanji); err != nil {
		return KanjiInfo{}, err
	}
	known, err := s.knownKanji()
	if err != nil {
		return KanjiInfo{}, err
	}
	r, _ := utf8.DecodeRuneInString(kanji)
	out.Known = known[r]
	parts, err := s.kanjiComponents(kanji)
	if err != nil {
		return KanjiInfo{}, err
	}
	for _, p := range parts {
		c := KanjiComponent{Kanji: p.Kanji, Meaning: p.Meaning}
		if c.Note, c.NoteShared, err = s.kanjiNote(p.Kanji); err != nil {
			return KanjiInfo{}, err
		}
		cr, _ := utf8.DecodeRuneInString(p.Kanji)
		c.Known = known[cr]
		out.Components = append(out.Components, c)
	}
	rows, err := s.q().Query(`
		SELECT c.id, c.lemma, c.readings, COALESCE(w.status, 0)
		FROM cards c LEFT JOIN word_status w ON w.lang = c.lang AND w.lemma = c.lemma
		WHERE c.lang = ? AND instr(c.lemma, ?) > 0
		ORDER BY CASE COALESCE(w.status, 0) WHEN 2 THEN 0 WHEN 1 THEN 1 ELSE 2 END, c.lemma`, s.lang, kanji)
	if err != nil {
		return KanjiInfo{}, fmt.Errorf("kanji words: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var w KanjiWord
		var readings sql.NullString
		if err := rows.Scan(&w.CardID, &w.Lemma, &readings, &w.Status); err != nil {
			return KanjiInfo{}, fmt.Errorf("scan kanji word: %w", err)
		}
		if readings.Valid && readings.String != "" {
			_ = json.Unmarshal([]byte(readings.String), &w.Readings)
		}
		out.Words = append(out.Words, w)
	}
	return out, rows.Err()
}

// kanjiNote is the effective story for a kanji: the deck's own, else the
// library's (shared true).
func (s *Store) kanjiNote(kanji string) (note string, shared bool, err error) {
	var own sql.NullString
	if err := s.q().QueryRow(`SELECT note FROM kanji_notes WHERE lang = ? AND kanji = ?`, s.lang, kanji).Scan(&own); err != nil && err != sql.ErrNoRows {
		return "", false, fmt.Errorf("kanji note: %w", err)
	}
	if own.String != "" {
		return own.String, false, nil
	}
	note, err = SharedLibrary().KanjiNote(s.lang, kanji)
	return note, note != "", err
}

// kanjiComponents derives the parts a kanji is explained through from the
// mnemonic of its single-kanji word: the deck's own card first, else the
// library's entry.
func (s *Store) kanjiComponents(kanji string) ([]MnemonicPart, error) {
	rows, err := s.q().Query(`SELECT lemma, mnemonic_parts FROM cards WHERE lang = ? AND instr(lemma, ?) > 0 AND mnemonic_parts IS NOT NULL`, s.lang, kanji)
	if err != nil {
		return nil, fmt.Errorf("kanji parts: %w", err)
	}
	var best string
	var bestParts sql.NullString
	for rows.Next() {
		var lemma string
		var parts sql.NullString
		if err := rows.Scan(&lemma, &parts); err != nil {
			rows.Close()
			return nil, err
		}
		if singleKanjiLemma(lemma, best) {
			best, bestParts = lemma, parts
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	parts := decodeParts(bestParts)
	if parts == nil {
		if parts, err = SharedLibrary().SingleKanjiParts(s.lang, kanji); err != nil {
			return nil, err
		}
	}
	return componentsOf(kanji, parts), nil
}

// singleKanjiLemma reports whether lemma has exactly one kanji and is a
// better source than the current best (none, or a longer one).
func singleKanjiLemma(lemma, best string) bool {
	return hanCount(lemma) == 1 && (best == "" || len(lemma) < len(best))
}

func decodeParts(parts sql.NullString) []MnemonicPart {
	if !parts.Valid || parts.String == "" {
		return nil
	}
	var out []MnemonicPart
	if json.Unmarshal([]byte(parts.String), &out) != nil {
		return nil
	}
	return out
}

// componentsOf reads a single-kanji word's parts as components of the
// kanji: every single-kanji part other than the kanji itself, and, for a
// part that is the kanji opening into its first radical ("= 足 foot"), that
// radical.
func componentsOf(kanji string, parts []MnemonicPart) []MnemonicPart {
	var out []MnemonicPart
	for _, p := range parts {
		if p.Kanji == kanji {
			if !strings.HasPrefix(p.Meaning, "=") {
				continue
			}
			rest := strings.TrimSpace(p.Meaning[1:])
			if r, n := utf8.DecodeRuneInString(rest); n > 0 && unicode.Is(unicode.Han, r) {
				out = append(out, MnemonicPart{Kanji: string(r), Meaning: strings.TrimSpace(rest[n:])})
			}
			continue
		}
		if r, n := utf8.DecodeRuneInString(p.Kanji); n == len(p.Kanji) && unicode.Is(unicode.Han, r) {
			out = append(out, MnemonicPart{Kanji: p.Kanji, Meaning: p.Meaning})
		}
	}
	return out
}

// SetKanjiNote upserts the tutor's story for a kanji; an empty note deletes it.
func (s *Store) SetKanjiNote(kanji, note string) error {
	if err := validKanji(kanji); err != nil {
		return err
	}
	if note == "" {
		_, err := s.q().Exec(`DELETE FROM kanji_notes WHERE lang = ? AND kanji = ?`, s.lang, kanji)
		return err
	}
	if err := checkText("note", note); err != nil {
		return err
	}
	_, err := s.q().Exec(`INSERT INTO kanji_notes (lang, kanji, note, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(lang, kanji) DO UPDATE SET note = excluded.note, updated_at = excluded.updated_at`,
		s.lang, kanji, note, time.Now().Unix())
	return err
}
