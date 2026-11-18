package srs

import (
	"database/sql"
	"encoding/json"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// A mnemonic is structured data: parts (kanji + meaning, in reading order)
// and a story (docs/card-content.md § Mnemonic). The shared library holds
// one per lemma; cards.mnemonic_parts (JSON) and cards.mnemonic_story are a
// card's override. Clients render it and never parse text.

// MnemonicPart is one tile of the breakdown line: a kanji (or a radical, or
// the kana word itself for a kana-only lemma) and its meaning.
type MnemonicPart struct {
	Kanji   string `json:"kanji"`
	Meaning string `json:"meaning"`
	// Known: the part appears in a word this deck marks Known. Set when a
	// card is served, never stored; absent for kana parts.
	Known *bool `json:"known,omitempty"`
}

// Mnemonic is a card's memory aid: the breakdown and the story.
type Mnemonic struct {
	Parts []MnemonicPart `json:"parts"`
	Story string         `json:"story"`
}

var (
	mnemonicPrefix = regexp.MustCompile(`^(🎓\s*Mnemonic\b[\s:：—–-]*|💡\s*)`)
	cjkWord        = regexp.MustCompile(`^[\x{3400}-\x{9fff}\x{3040}-\x{30ff}\x{3005}]+$`)
)

// IsMnemonicNote reports whether note text is a mnemonic in a shape older
// tutors wrote; such notes are refused (the mnemonic is not a note).
func IsMnemonicNote(note string) bool {
	return mnemonicPrefix.MatchString(note)
}

func hanCount(s string) int {
	n := 0
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			n++
		}
	}
	return n
}

func hasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// encodeMnemonicParts is the JSON stored in cards.mnemonic_parts; empty
// parts store NULL.
func encodeMnemonicParts(parts []MnemonicPart) any {
	if len(parts) == 0 {
		return nil
	}
	b, _ := json.Marshal(parts)
	return string(b)
}

// decodeMnemonic rebuilds a card's Mnemonic from its two columns; nil when
// the card has none.
func decodeMnemonic(partsJSON, story sql.NullString) *Mnemonic {
	m := Mnemonic{Story: strings.TrimSpace(story.String)}
	if partsJSON.Valid && partsJSON.String != "" {
		_ = json.Unmarshal([]byte(partsJSON.String), &m.Parts)
	}
	if len(m.Parts) == 0 && m.Story == "" {
		return nil
	}
	if m.Parts == nil {
		m.Parts = []MnemonicPart{}
	}
	return &m
}

// GetMnemonic returns a card's mnemonic, or nil when it has none.
func (s *Store) GetMnemonic(cardID int64) (*Mnemonic, error) {
	var parts, story sql.NullString
	if err := s.q().QueryRow(`SELECT mnemonic_parts, mnemonic_story FROM cards WHERE id = ?`, cardID).Scan(&parts, &story); err != nil {
		if err == sql.ErrNoRows {
			return nil, clientErrf("card %d not found", cardID)
		}
		return nil, err
	}
	return decodeMnemonic(parts, story), nil
}

// SetMnemonic validates and writes a card's mnemonic, replacing any existing
// one. Empty parts and story clear it. It reports whether the card had one.
func (s *Store) SetMnemonic(cardID int64, parts []MnemonicPart, story string) (replaced bool, err error) {
	var lemma string
	var hadParts, hadStory sql.NullString
	if err := s.q().QueryRow(`SELECT lemma, mnemonic_parts, mnemonic_story FROM cards WHERE id = ?`, cardID).Scan(&lemma, &hadParts, &hadStory); err != nil {
		return false, clientErrf("card %d not found", cardID)
	}
	replaced = decodeMnemonic(hadParts, hadStory) != nil
	story = strings.TrimSpace(story)
	if story == "" && len(parts) == 0 {
		_, err := s.q().Exec(`UPDATE cards SET mnemonic_parts = NULL, mnemonic_story = NULL WHERE id = ?`, cardID)
		return replaced, err
	}
	parts, err = validateMnemonic(lemma, parts, story)
	if err != nil {
		return false, err
	}
	_, err = s.q().Exec(`UPDATE cards SET mnemonic_parts = ?, mnemonic_story = ? WHERE id = ?`, encodeMnemonicParts(parts), story, cardID)
	return replaced, err
}

// validateMnemonic applies the rules of docs/card-content.md § Mnemonic to
// a non-empty mnemonic for lemma and returns the trimmed parts.
func validateMnemonic(lemma string, parts []MnemonicPart, story string) ([]MnemonicPart, error) {
	if story == "" {
		return nil, clientErrf("story is required")
	}
	if err := checkText("lemma", lemma); err != nil {
		return nil, err
	}
	if err := checkText("story", story); err != nil {
		return nil, err
	}
	if len(parts) > 6 {
		return nil, clientErrf("at most six parts")
	}
	covered := false
	for i, p := range parts {
		p.Kanji = strings.TrimSpace(p.Kanji)
		p.Meaning = strings.TrimSpace(p.Meaning)
		if p.Kanji == "" || p.Meaning == "" {
			return nil, clientErrf("part %d needs both kanji and meaning", i+1)
		}
		if !cjkWord.MatchString(p.Kanji) {
			return nil, clientErrf("part %d: %q is not kanji or kana", i+1, p.Kanji)
		}
		if strings.ContainsAny(p.Meaning, "\n") || strings.Contains(p.Meaning, " · ") {
			return nil, clientErrf("part %d: meaning must be one short phrase", i+1)
		}
		if strings.Contains(lemma, p.Kanji) {
			covered = true
		}
		p.Known = nil
		parts[i] = p
	}
	// A kana-only word may explain itself through its kanji origin (あらすじ:
	// 粗 rough · 筋 sinew) and a single-kanji word opens into radicals (踊:
	// 足 foot · 甬 passage), so coverage is demanded only of words with two
	// or more kanji.
	if len(parts) > 0 && !covered && hanCount(lemma) > 1 {
		return nil, clientErrf("no part matches the lemma %s", lemma)
	}
	return parts, nil
}

// MnemonicAuditRow is one card in the audit: its mnemonic state and content.
type MnemonicAuditRow struct {
	CardID int64          `json:"card_id"`
	Lemma  string         `json:"lemma"`
	Status int            `json:"status"`
	Kind   string         `json:"kind"` // ok | no_breakdown | missing
	Parts  []MnemonicPart `json:"parts,omitempty"`
	Story  string         `json:"story,omitempty"`
	// Shared: the mnemonic shown is the library's; the card has no override.
	Shared bool `json:"shared,omitempty"`
}

// MnemonicAudit classifies every card's effective mnemonic (its own, else
// the library's):
//
//	ok            parts and a story
//	no_breakdown  a story but no parts on a word with kanji; a tutor should add the breakdown
//	missing       no mnemonic (only reported for lemmas with kanji, or with kanaToo)
func (s *Store) MnemonicAudit(kanaToo bool) ([]MnemonicAuditRow, error) {
	type card struct {
		row    MnemonicAuditRow
		own    *Mnemonic
		shared *Mnemonic
	}
	var cards []card
	rows, err := s.q().Query(`
		SELECT c.id, c.lemma, COALESCE(w.status, 0), c.mnemonic_parts, c.mnemonic_story
		FROM cards c LEFT JOIN word_status w ON w.lang = c.lang AND w.lemma = c.lemma
		WHERE c.lang = ?
		ORDER BY c.id`, s.lang)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c card
		var parts, story sql.NullString
		if err := rows.Scan(&c.row.CardID, &c.row.Lemma, &c.row.Status, &parts, &story); err != nil {
			return nil, err
		}
		c.own = decodeMnemonic(parts, story)
		cards = append(cards, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	err = s.fillSharedMnemonics(len(cards),
		func(i int) (string, *Mnemonic) { return cards[i].row.Lemma, cards[i].own },
		func(i int, m *Mnemonic) { cards[i].shared = m })
	if err != nil {
		return nil, err
	}
	out := []MnemonicAuditRow{}
	for _, c := range cards {
		row, m := c.row, c.own
		if m == nil && c.shared != nil {
			m, row.Shared = c.shared, true
		}
		switch {
		case m == nil:
			if !hasHan(row.Lemma) && !kanaToo {
				continue
			}
			row.Kind = "missing"
		case len(m.Parts) == 0 && hasHan(row.Lemma):
			row.Kind = "no_breakdown"
			row.Story = m.Story
		default:
			row.Kind = "ok"
			row.Parts, row.Story = m.Parts, m.Story
		}
		out = append(out, row)
	}
	return out, nil
}

// knownKanji is every kanji in a word this deck marks Known.
func (s *Store) knownKanji() (map[rune]bool, error) {
	rows, err := s.q().Query(`SELECT lemma FROM word_status WHERE lang = ? AND status = 2`, s.lang)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	known := map[rune]bool{}
	for rows.Next() {
		var lemma string
		if err := rows.Scan(&lemma); err != nil {
			return nil, err
		}
		for _, r := range lemma {
			if unicode.Is(unicode.Han, r) {
				known[r] = true
			}
		}
	}
	return known, rows.Err()
}

// markKnown flags each single-kanji part of a served mnemonic as met in a
// Known word or not, so a client can point out the parts still new to the
// learner.
func markKnown(m *Mnemonic, known map[rune]bool) {
	if m == nil {
		return
	}
	for i, p := range m.Parts {
		if r, n := utf8.DecodeRuneInString(p.Kanji); n == len(p.Kanji) && unicode.Is(unicode.Han, r) {
			k := known[r]
			m.Parts[i].Known = &k
		}
	}
}
