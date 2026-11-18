package srs

import (
	"path/filepath"
	"testing"
	"time"
)

// Multi-step: three reviews walk back to exactly each intermediate state,
// then the well runs dry.
func TestMultiStepUndo(t *testing.T) {
	s := openTestStore(t)
	id := newSimCard(t, s, "多段戻し")

	states := []cardState{readCardState(t, s, id)}
	at := time.Now()
	for i := 0; i < 3; i++ {
		cs := readCardState(t, s, id)
		at = timeUnixHour(cs.due)
		if _, err := s.reviewCardAt(id, RatingGood, at); err != nil {
			t.Fatalf("review %d: %v", i, err)
		}
		states = append(states, readCardState(t, s, id))
	}

	logCount := func() (n int) {
		if err := s.q().QueryRow(`SELECT COUNT(*) FROM review_log WHERE card_id = ?`, id).Scan(&n); err != nil {
			t.Fatalf("count log: %v", err)
		}
		return
	}
	if logCount() != 3 {
		t.Fatalf("log rows = %d, want 3", logCount())
	}

	for step := 3; step >= 1; step-- {
		cardID, err := s.UndoReview()
		if err != nil {
			t.Fatalf("undo to state %d: %v", step-1, err)
		}
		if cardID != id {
			t.Fatalf("undo returned card %d, want %d", cardID, id)
		}
		if got := readCardState(t, s, id); got != states[step-1] {
			t.Fatalf("after undo to state %d: %+v, want %+v", step-1, got, states[step-1])
		}
		if logCount() != step-1 {
			t.Fatalf("log rows = %d, want %d", logCount(), step-1)
		}
	}

	if _, err := s.UndoReview(); err == nil {
		t.Fatal("undo past the beginning should error")
	}
}

// Undo is DB-backed — it must survive closing and reopening the store
// (i.e. a server restart).
func TestUndoSurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cards.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	id := newSimCard(t, s, "再起動")
	before := readCardState(t, s, id)
	cs := readCardState(t, s, id)
	if _, err := s.reviewCardAt(id, RatingGood, timeUnixHour(cs.due)); err != nil {
		t.Fatalf("review: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = s2.Close() })
	if _, err := s2.UndoReview(); err != nil {
		t.Fatalf("undo after reopen: %v", err)
	}
	if got := readCardState(t, s2, id); got != before {
		t.Fatalf("state after restart-undo = %+v, want %+v", got, before)
	}
}

// A review of a since-deleted card can't be restored (no card row) and must
// not block undoing the review before it — the walk skips orphan rows.
func TestUndoSkipsDeletedCards(t *testing.T) {
	s := openTestStore(t)
	keep := newSimCard(t, s, "残る")
	gone := newSimCard(t, s, "消える")

	keepBefore := readCardState(t, s, keep)
	cs := readCardState(t, s, keep)
	if _, err := s.reviewCardAt(keep, RatingGood, timeUnixHour(cs.due)); err != nil {
		t.Fatalf("review keep: %v", err)
	}
	cs = readCardState(t, s, gone)
	if _, err := s.reviewCardAt(gone, RatingGood, timeUnixHour(cs.due)); err != nil {
		t.Fatalf("review gone: %v", err)
	}
	if _, err := s.DeleteCard(gone); err != nil {
		t.Fatalf("DeleteCard: %v", err)
	}

	cardID, err := s.UndoReview()
	if err != nil {
		t.Fatalf("undo: %v", err)
	}
	if cardID != keep {
		t.Fatalf("undo hit card %d, want it to skip the deleted card and hit %d", cardID, keep)
	}
	if got := readCardState(t, s, keep); got != keepBefore {
		t.Fatalf("keep card not restored: %+v, want %+v", got, keepBefore)
	}
}

// Undo is bounded to today's reviews — yesterday's schedule is settled and
// must not be silently unwindable.
func TestUndoBoundedToToday(t *testing.T) {
	s := openTestStore(t)
	id := newSimCard(t, s, "昨日分")
	cs := readCardState(t, s, id)
	if _, err := s.reviewCardAt(id, RatingGood, timeUnixHour(cs.due)); err != nil {
		t.Fatalf("review: %v", err)
	}
	// Age the review to the day before yesterday.
	if _, err := s.q().Exec(
		`UPDATE review_log SET reviewed_at = strftime('%s','now') - 2*86400 WHERE card_id = ?`, id,
	); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	if _, err := s.UndoReview(); err == nil {
		t.Fatal("undoing a review from a previous day should error")
	}

	// A fresh review today is still undoable over the old one.
	cs = readCardState(t, s, id)
	if _, err := s.reviewCardAt(id, RatingGood, time.Now()); err != nil {
		t.Fatalf("today review: %v", err)
	}
	cardID, err := s.UndoReview()
	if err != nil || cardID != id {
		t.Fatalf("today's review should undo: card=%d err=%v", cardID, err)
	}
	if got := readCardState(t, s, id); got != cs {
		t.Fatalf("state after undo = %+v, want %+v", got, cs)
	}
}

// Rows from before migration 10 carry no snapshot — the walk must stop
// rather than restore garbage.
func TestUndoStopsAtLegacyRows(t *testing.T) {
	s := openTestStore(t)
	id := newSimCard(t, s, "旧式")
	// Simulate a pre-migration review: a bare log row with NULL prev_*.
	if _, err := s.q().Exec(
		`INSERT INTO review_log (card_id, rating, reviewed_at) VALUES (?, 3, ?)`,
		id, time.Now().Unix(),
	); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	if _, err := s.UndoReview(); err == nil {
		t.Fatal("undo of a legacy (snapshot-less) row should error")
	}
}
