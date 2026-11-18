package srs

import (
	"fmt"
	"strings"
	"time"
)

// Card notes are freeform study notes + AI practice corrections attached to a
// saved card. The Practice mode (AI-generated exercises) writes the user's
// mistakes here so they resurface at review time.

// CardNote mirrors the /list_card_notes row shape.
type CardNote struct {
	ID     int64  `json:"id"`
	CardID int64  `json:"card_id"`
	Kind   string `json:"kind"` // NoteKind*
	Note   string `json:"note"`
	// The example's translation (kind example only).
	Translation string `json:"translation,omitempty"`
	CreatedAt   string `json:"created_at"` // RFC3339
}

// Note kinds. A card's own sentence, translation and explanation are columns
// on cards; notes are what sits beside them.
const (
	NoteKindMnemonic = "mnemonic" // legacy kind; mnemonics use shared content or card overrides
	NoteKindTutor    = "tutor"    // a teaching note the learner sees once
	NoteKindExample  = "example"  // an extra example sentence; its translation is a field
	NoteKindTriage   = "triage"   // a tutor's "something looks wrong" marker
)

var noteKinds = map[string]bool{NoteKindMnemonic: true, NoteKindTutor: true, NoteKindExample: true, NoteKindTriage: true}

// Markers older clients and skills opened a note with. A note's kind is a
// field; the marker still decides the kind when none is sent
// and is dropped from the stored text either way.
var noteMarkers = map[string]string{NoteKindTutor: "🎓", NoteKindExample: "📝", NoteKindTriage: "⚠"}

// noteKindOf infers a kind for a note sent without one. "EN:" is refused:
// that content belongs on the card (UpdateCard translation/explanation).
func noteKindOf(note string) (string, error) {
	t := strings.TrimSpace(note)
	switch {
	case strings.HasPrefix(t, "EN:"):
		return "", clientErrf("a card's translation is a card field: send translation/explanation to /update_card")
	case IsMnemonicNote(t):
		return "", clientErrf("a mnemonic is a card field: send parts and story to /set_mnemonic")
	case strings.HasPrefix(t, noteMarkers[NoteKindExample]):
		return NoteKindExample, nil
	case strings.HasPrefix(t, noteMarkers[NoteKindTriage]):
		return NoteKindTriage, nil
	}
	return NoteKindTutor, nil
}

// normalizeNote is what the store keeps of a submitted note: the kind's
// legacy marker gone, and an example's translation moved off its second
// line into the field when the caller did not send one.
func normalizeNote(kind, note, translation string) (string, string) {
	text := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(note), noteMarkers[kind]))
	translation = strings.TrimSpace(translation)
	if kind == NoteKindExample && translation == "" {
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			text, translation = strings.TrimSpace(text[:i]), strings.TrimSpace(text[i+1:])
		}
	}
	if kind != NoteKindExample {
		translation = ""
	}
	return text, translation
}

// AddCardNote attaches a note to a card. Returns the new note id + timestamp
// (so the frontend can render it without a re-fetch).
func (s *Store) AddCardNote(cardID int64, kind, note, translation string) (int64, string, error) {
	if strings.TrimSpace(note) == "" {
		return 0, "", clientErrf("note text is required")
	}
	if kind == "" {
		k, err := noteKindOf(note)
		if err != nil {
			return 0, "", err
		}
		kind = k
	} else if kind == NoteKindMnemonic {
		return 0, "", clientErrf("a mnemonic is a card field: send parts and story to /set_mnemonic")
	} else if !noteKinds[kind] {
		return 0, "", clientErrf("kind must be one of tutor, example, triage")
	}
	note, translation = normalizeNote(kind, note, translation)
	if note == "" {
		return 0, "", clientErrf("note text is required")
	}
	if err := checkText("note", note); err != nil {
		return 0, "", err
	}
	now := time.Now()
	res, err := s.q().Exec(
		`INSERT INTO card_notes (card_id, kind, note, translation, created_at) VALUES (?, ?, ?, ?, ?)`,
		cardID, kind, note, translation, now.Unix(),
	)
	if err != nil {
		return 0, "", fmt.Errorf("insert note: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, "", err
	}
	return id, now.UTC().Format(time.RFC3339), nil
}

// ListCardNotes returns a card's notes, newest first.
func (s *Store) ListCardNotes(cardID int64) ([]CardNote, error) {
	rows, err := s.q().Query(
		`SELECT id, card_id, kind, note, translation, created_at FROM card_notes WHERE card_id = ? ORDER BY created_at DESC`,
		cardID,
	)
	if err != nil {
		return nil, fmt.Errorf("list notes: %w", err)
	}
	defer rows.Close()
	out := []CardNote{}
	for rows.Next() {
		var n CardNote
		var createdAt any
		if err := rows.Scan(&n.ID, &n.CardID, &n.Kind, &n.Note, &n.Translation, &createdAt); err != nil {
			return nil, fmt.Errorf("scan note: %w", err)
		}
		n.CreatedAt = noteTime(createdAt)
		out = append(out, n)
	}
	return out, rows.Err()
}

// noteTime renders epoch and supported textual timestamps as RFC3339.
func noteTime(v any) string {
	switch t := v.(type) {
	case int64:
		return time.Unix(t, 0).UTC().Format(time.RFC3339)
	case float64:
		return time.Unix(int64(t), 0).UTC().Format(time.RFC3339)
	case string:
		for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
			if ts, err := time.Parse(layout, t); err == nil {
				return ts.UTC().Format(time.RFC3339)
			}
		}
		return t
	case []byte:
		return noteTime(string(t))
	}
	return ""
}

// DeleteCardNote removes a single note by id.
func (s *Store) DeleteCardNote(noteID int64) (int, error) {
	res, err := s.q().Exec(`DELETE FROM card_notes WHERE id = ?`, noteID)
	if err != nil {
		return 0, fmt.Errorf("delete note: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
