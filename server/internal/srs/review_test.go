package srs

import (
	"testing"
)

func TestUndoReview(t *testing.T) {
	s := openTestStore(t)
	r, err := s.SaveCard(SaveCardReq{Lemma: "勉強", Glosses: [][]string{{"study"}}})
	if err != nil {
		t.Fatalf("SaveCard: %v", err)
	}

	readState := func() (due int64, reps, state int) {
		err := s.q().QueryRow(`SELECT due, reps, state FROM cards WHERE id = ?`, r.ID).
			Scan(&due, &reps, &state)
		if err != nil {
			t.Fatalf("read state: %v", err)
		}
		return
	}
	logCount := func() (n int) {
		if err := s.q().QueryRow(`SELECT COUNT(*) FROM review_log`).Scan(&n); err != nil {
			t.Fatalf("count log: %v", err)
		}
		return
	}

	dueBefore, repsBefore, stateBefore := readState()

	if _, err := s.ReviewCard(r.ID, 3); err != nil {
		t.Fatalf("ReviewCard: %v", err)
	}
	dueAfter, repsAfter, _ := readState()
	if dueAfter == dueBefore || repsAfter != repsBefore+1 || logCount() != 1 {
		t.Fatalf("review didn't take: due %d→%d reps %d→%d logs %d",
			dueBefore, dueAfter, repsBefore, repsAfter, logCount())
	}

	cardID, err := s.UndoReview()
	if err != nil {
		t.Fatalf("UndoReview: %v", err)
	}
	if cardID != r.ID {
		t.Errorf("undo card_id = %d, want %d", cardID, r.ID)
	}
	dueUndone, repsUndone, stateUndone := readState()
	if dueUndone != dueBefore || repsUndone != repsBefore || stateUndone != stateBefore {
		t.Errorf("state not restored: due %d (want %d) reps %d (want %d) state %d (want %d)",
			dueUndone, dueBefore, repsUndone, repsBefore, stateUndone, stateBefore)
	}
	if logCount() != 0 {
		t.Errorf("review_log rows = %d, want 0", logCount())
	}

	// Second undo must fail — single step only.
	if _, err := s.UndoReview(); err == nil {
		t.Error("second undo should error")
	}
}

func TestDeleteCardAndJunkLemmaGuard(t *testing.T) {
	s := openTestStore(t)
	r, err := s.SaveCard(SaveCardReq{Lemma: "勉強", Glosses: [][]string{{"study"}}})
	if err != nil {
		t.Fatalf("SaveCard: %v", err)
	}
	if _, _, err := s.AddCardNote(r.ID, "", "note", ""); err != nil {
		t.Fatalf("AddCardNote: %v", err)
	}

	deleted, err := s.DeleteCard(r.ID)
	if err != nil || !deleted {
		t.Fatalf("DeleteCard = %v, %v", deleted, err)
	}
	var n int
	if err := s.q().QueryRow(`SELECT COUNT(*) FROM card_notes WHERE card_id = ?`, r.ID).Scan(&n); err != nil || n != 0 {
		t.Errorf("notes not cascaded: n=%d err=%v", n, err)
	}
	if again, _ := s.DeleteCard(r.ID); again {
		t.Error("second delete should report not-found")
	}

	// Punctuation-only lemmas must never become cards.
	for _, junk := range []string{"？", "…", "！？", "。", "  "} {
		if _, err := s.SaveCard(SaveCardReq{Lemma: junk}); err == nil {
			t.Errorf("junk lemma %q was accepted", junk)
		}
	}
}
