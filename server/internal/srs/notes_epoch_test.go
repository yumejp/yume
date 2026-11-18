package srs

import "testing"

func TestListCardNotesToleratesTextTimestamps(t *testing.T) {
	s := openTestStore(t)
	resp, err := s.SaveCard(SaveCardReq{Lemma: "推移", Readings: []string{"すいい"}, Glosses: [][]string{{"transition"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.q().Exec(`INSERT INTO card_notes (card_id, kind, note, created_at) VALUES (?, 'mnemonic', ?, ?)`, resp.ID, "🎓 Mnemonic: x", "2026-08-31 09:01:54"); err == nil {
		t.Fatal("since migration 17 card_notes.created_at is CHECKed as integer; a text timestamp must be rejected")
	}
	if _, _, err := s.AddCardNote(resp.ID, "", "🎓 Compare 変化.", ""); err != nil {
		t.Fatal(err)
	}
	notes, err := s.ListCardNotes(resp.ID)
	if err != nil || len(notes) != 1 || notes[0].Kind != NoteKindTutor {
		t.Errorf("notes = %+v (%v)", notes, err)
	}
	// The tolerant reader still handles text the way older DBs stored it.
	if got := noteTime("2026-08-31 09:01:54"); got != "2026-08-31T09:01:54Z" {
		t.Errorf("noteTime(text) = %q", got)
	}
}
