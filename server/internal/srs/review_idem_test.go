package srs

import (
	"strings"
	"testing"
	"time"
)

// A retry carrying the same review_id (timed-out-but-committed POST) must be
// answered from the stored state, not double-applied — a replayed Again would
// otherwise halve a mature card's rebuilt stability.
func TestReviewIdempotentReplay(t *testing.T) {
	s := openTestStore(t)
	id := newSimCard(t, s, "再試行")

	// Mature the card a bit so a double-applied Again would visibly differ.
	for i := 0; i < 5; i++ {
		cs := readCardState(t, s, id)
		if _, err := s.reviewCardAt(id, RatingGood, timeUnixHour(cs.due)); err != nil {
			t.Fatalf("warmup review %d: %v", i, err)
		}
	}

	cs := readCardState(t, s, id)
	at := timeUnixHour(cs.due)
	first, err := s.reviewCardIdemAt(id, RatingAgain, "tok-1", nil, at)
	if err != nil {
		t.Fatalf("first review: %v", err)
	}
	after := readCardState(t, s, id)

	logCount := func() (n int) {
		if err := s.q().QueryRow(`SELECT COUNT(*) FROM review_log WHERE card_id = ?`, id).Scan(&n); err != nil {
			t.Fatalf("count log: %v", err)
		}
		return
	}
	logsAfterFirst := logCount()

	// Replay: same token, a few seconds later (as a client retry would be).
	replay, err := s.reviewCardIdemAt(id, RatingAgain, "tok-1", nil, at.Add(15e9))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	replayed := readCardState(t, s, id)
	if replayed != after {
		t.Errorf("replay changed card state: %+v -> %+v", after, replayed)
	}
	if logCount() != logsAfterFirst {
		t.Errorf("replay added a review_log row: %d -> %d", logsAfterFirst, logCount())
	}
	if replay.CardID != first.CardID || replay.NewState != after.state {
		t.Errorf("replay result = %+v, want card %d state %d", replay, first.CardID, after.state)
	}
	if replay.Promoted || replay.Demoted {
		t.Errorf("replay must not re-report promotion/demotion: %+v", replay)
	}

	// A token can't be reused against a different card.
	other := newSimCard(t, s, "別件")
	if _, err := s.reviewCardIdemAt(other, RatingGood, "tok-1", nil, at.Add(30e9)); err == nil {
		t.Error("reusing a token on another card should error")
	}

	// Token validation: length and charset.
	if _, err := s.reviewCardIdemAt(id, RatingGood, strings.Repeat("x", 65), nil, at); err == nil {
		t.Error("65-char review_id should be rejected")
	}
	if _, err := s.reviewCardIdemAt(id, RatingGood, "bad token!", nil, at); err == nil {
		t.Error("review_id with space/! should be rejected")
	}
}

// Undo must revert the review's auto-promotion — a fat-fingered Easy that
// crossed the graduation bar must not leave the lemma Known after Z.
func TestUndoRestoresWordStatusAfterPromotion(t *testing.T) {
	s := openTestStore(t)
	id := newSimCard(t, s, "昇格戻し")
	if err := s.SetWordStatus("昇格戻し", 1); err != nil {
		t.Fatalf("SetWordStatus: %v", err)
	}

	for i := 0; i < 15; i++ {
		cs := readCardState(t, s, id)
		res, err := s.reviewCardAt(id, RatingGood, timeUnixHour(cs.due))
		if err != nil {
			t.Fatalf("review %d: %v", i, err)
		}
		if res.Promoted {
			if got := wordStatus(t, s, "昇格戻し"); got != 2 {
				t.Fatalf("after promotion status = %d, want Known(2)", got)
			}
			if _, err := s.UndoReview(); err != nil {
				t.Fatalf("UndoReview: %v", err)
			}
			if got := wordStatus(t, s, "昇格戻し"); got != 1 {
				t.Fatalf("after undo status = %d, want Learning(1)", got)
			}
			return
		}
	}
	t.Fatal("card never promoted in 15 Good reviews")
}

// Mirror case: undo must revert an auto-demotion (accidental Again on a
// Known word must not leave it demoted to Learning).
func TestUndoRestoresWordStatusAfterDemotion(t *testing.T) {
	s := openTestStore(t)
	id := newSimCard(t, s, "降格戻し")

	// Mature the card into Review state, then mark the lemma Known manually.
	for i := 0; i < 5; i++ {
		cs := readCardState(t, s, id)
		if _, err := s.reviewCardAt(id, RatingGood, timeUnixHour(cs.due)); err != nil {
			t.Fatalf("warmup review %d: %v", i, err)
		}
	}
	if err := s.SetWordStatus("降格戻し", 2); err != nil {
		t.Fatalf("SetWordStatus: %v", err)
	}

	// Demotion takes a second lapse from Review within the window.
	cs := readCardState(t, s, id)
	first := timeUnixHour(cs.due)
	if _, err := s.reviewCardAt(id, RatingAgain, first); err != nil {
		t.Fatalf("first lapse: %v", err)
	}
	if _, err := s.reviewCardAt(id, RatingGood, first.Add(5*time.Minute)); err != nil {
		t.Fatalf("recover: %v", err)
	}
	cs = readCardState(t, s, id)
	res, err := s.reviewCardAt(id, RatingAgain, timeUnixHour(cs.due))
	if err != nil {
		t.Fatalf("lapse review: %v", err)
	}
	if !res.Demoted {
		t.Fatalf("expected the second lapse to demote (state=%d)", res.NewState)
	}
	if got := wordStatus(t, s, "降格戻し"); got != 1 {
		t.Fatalf("after demotion status = %d, want Learning(1)", got)
	}
	if _, err := s.UndoReview(); err != nil {
		t.Fatalf("UndoReview: %v", err)
	}
	if got := wordStatus(t, s, "降格戻し"); got != 2 {
		t.Fatalf("after undo status = %d, want Known(2)", got)
	}
}

// Auto-promotion must retire an active watch word, same as a manual
// mark-Known through SetWordStatus.
func TestAutoPromoteRetiresWatchWord(t *testing.T) {
	s := openTestStore(t)
	id := newSimCard(t, s, "見張り")
	if err := s.SetWordStatus("見張り", 1); err != nil {
		t.Fatalf("SetWordStatus: %v", err)
	}
	if _, err := s.AddWatchWords([]WatchWord{{Lemma: "見張り"}}, "test"); err != nil {
		t.Fatalf("AddWatchWords: %v", err)
	}

	promoted := false
	for i := 0; i < 15 && !promoted; i++ {
		cs := readCardState(t, s, id)
		res, err := s.reviewCardAt(id, RatingGood, timeUnixHour(cs.due))
		if err != nil {
			t.Fatalf("review %d: %v", i, err)
		}
		promoted = res.Promoted
	}
	if !promoted {
		t.Fatal("card never promoted in 15 Good reviews")
	}
	words, err := s.ListWatchWords()
	if err != nil {
		t.Fatalf("ListWatchWords: %v", err)
	}
	for _, w := range words {
		if w.Lemma == "見張り" {
			t.Error("watch word survived auto-promotion; should be retired")
		}
	}
}

// The due-card preview must survive corrupt FSRS rows (stability=0 on a
// non-new card) — the shared sanitize keeps the button labels finite instead
// of feeding NaN epochs to the client.
func TestPreviewSanitizesCorruptRow(t *testing.T) {
	s := openTestStore(t)
	id := newSimCard(t, s, "破損")
	// Simulate what a bad import produced: a "Review" card with zero
	// stability and out-of-range difficulty, already due.
	if _, err := s.q().Exec(`
		UPDATE cards SET state = 2, stability = 0, difficulty = 0, reps = 3,
		                 due = strftime('%s','now') - 3600,
		                 last_review = strftime('%s','now') - 86400
		WHERE id = ?
	`, id); err != nil {
		t.Fatalf("corrupt row: %v", err)
	}

	cards, err := s.GetDueCards(10)
	if err != nil {
		t.Fatalf("GetDueCards: %v", err)
	}
	for _, c := range cards {
		if c.ID != id {
			continue
		}
		if c.Preview == nil {
			t.Fatal("no preview on corrupt card")
		}
		for name, v := range map[string]int64{
			"again": c.Preview.Again, "hard": c.Preview.Hard,
			"good": c.Preview.Good, "easy": c.Preview.Easy,
		} {
			// A NaN-poisoned time.Unix lands wildly out of range; a sane
			// preview is between now-ish and +100 years.
			if v < 1_000_000_000 || v > 10_000_000_000 {
				t.Errorf("preview %s = %d, not a sane epoch", name, v)
			}
		}
		return
	}
	t.Fatal("corrupt card not returned as due")
}
