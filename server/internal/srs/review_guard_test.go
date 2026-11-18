package srs

import (
	"testing"
	"time"
)

// The offline-replay concurrency guard: a rating that carries a
// prev_review_id expectation only applies when the card's newest review still
// matches it — a rating queued offline on one device must not land on top of
// a review another device made in the meantime.
func TestReviewGuardConflict(t *testing.T) {
	s := openTestStore(t)
	id := newSimCard(t, s, "衝突")

	// Fresh card: expectation "" (no tokened reviews) matches → applies.
	res, err := s.ReviewCardGuarded(id, RatingGood, "mob-1", "")
	if err != nil {
		t.Fatalf("guarded review on fresh card: %v", err)
	}
	if res.Conflict {
		t.Fatal("fresh-card guard reported a conflict")
	}

	// Chain intact: expecting the review we just made → applies.
	res, err = s.ReviewCardGuarded(id, RatingGood, "mob-2", "mob-1")
	if err != nil {
		t.Fatalf("chained guarded review: %v", err)
	}
	if res.Conflict {
		t.Fatal("intact chain reported a conflict")
	}

	// Another device reviews the card (another client's live path).
	if _, err := s.ReviewCardIdem(id, RatingAgain, "desk-1"); err != nil {
		t.Fatalf("desktop review: %v", err)
	}
	before := readCardState(t, s, id)

	// The offline device replays a rating based on the pre-desktop chain tip
	// ("mob-2") — stale now, must be reported as a conflict and apply NOTHING.
	res, err = s.ReviewCardGuarded(id, RatingEasy, "mob-3", "mob-2")
	if err != nil {
		t.Fatalf("stale guarded review: %v", err)
	}
	if !res.Conflict {
		t.Fatal("stale expectation did not report a conflict")
	}
	if after := readCardState(t, s, id); after != before {
		t.Errorf("conflicted review changed card state: %+v -> %+v", before, after)
	}
	var n int
	if err := s.q().QueryRow(`SELECT COUNT(*) FROM review_log WHERE card_id = ?`, id).Scan(&n); err != nil {
		t.Fatalf("count log: %v", err)
	}
	if n != 3 {
		t.Errorf("review_log rows = %d, want 3 (conflict must not log)", n)
	}

	// Idempotent retry BEATS the guard: replaying an already-committed entry
	// (its POST timed out after committing) must return the stored result,
	// not a conflict — its own commit is what moved the chain tip.
	res, err = s.ReviewCardGuarded(id, RatingAgain, "desk-1", "mob-2")
	if err != nil {
		t.Fatalf("idempotent retry through guard: %v", err)
	}
	if res.Conflict {
		t.Error("idempotent retry reported a conflict instead of replaying")
	}

	// last_review_id rides the due-card wire for the next offline fetch.
	// (Force the card due — its relearning step is minutes out.)
	if _, err := s.q().Exec(`UPDATE cards SET due = strftime('%s','now') - 60 WHERE id = ?`, id); err != nil {
		t.Fatalf("force due: %v", err)
	}
	cards, err := s.GetDueCards(50)
	if err != nil {
		t.Fatalf("GetDueCards: %v", err)
	}
	for _, c := range cards {
		if c.ID == id {
			if c.LastReviewID != "desk-1" {
				t.Errorf("LastReviewID = %q, want desk-1", c.LastReviewID)
			}
			return
		}
	}
	t.Fatal("card not in due list (Again'd card should be due soon; widen limit?)")
}

func TestReviewGuardPreservesOfflineTimestamp(t *testing.T) {
	s := openTestStore(t)
	id := newSimCard(t, s, "時刻")
	firstAt := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	if _, err := s.reviewCardIdemAt(id, RatingGood, "base-1", nil, firstAt); err != nil {
		t.Fatalf("seed review: %v", err)
	}

	offlineAt := firstAt.Add(24 * time.Hour)
	res, err := s.ReviewCardGuardedAt(id, RatingGood, "mobile-1", "base-1", offlineAt)
	if err != nil {
		t.Fatalf("offline review: %v", err)
	}
	if res.Conflict {
		t.Fatal("intact offline chain reported a conflict")
	}

	var loggedAt, lastReview int64
	if err := s.q().QueryRow(
		`SELECT reviewed_at FROM review_log WHERE review_id = ?`, "mobile-1",
	).Scan(&loggedAt); err != nil {
		t.Fatalf("read review timestamp: %v", err)
	}
	if err := s.q().QueryRow(`SELECT last_review FROM cards WHERE id = ?`, id).Scan(&lastReview); err != nil {
		t.Fatalf("read card timestamp: %v", err)
	}
	if loggedAt != offlineAt.Unix() || lastReview != offlineAt.Unix() {
		t.Fatalf("timestamps = log %d card %d, want %d", loggedAt, lastReview, offlineAt.Unix())
	}
}

func TestReviewGuardRejectsTimestampBeforeLastReview(t *testing.T) {
	s := openTestStore(t)
	id := newSimCard(t, s, "逆順")
	lastAt := time.Now().Add(-time.Hour).Truncate(time.Second)
	if _, err := s.reviewCardIdemAt(id, RatingGood, "base-1", nil, lastAt); err != nil {
		t.Fatalf("seed review: %v", err)
	}
	if _, err := s.ReviewCardGuardedAt(
		id, RatingGood, "mobile-1", "base-1", lastAt.Add(-time.Minute),
	); !IsClientErr(err) {
		t.Fatalf("out-of-order timestamp error = %v, want client error", err)
	}
}

func TestTimestampedIdempotentRetryBeatsChronologyCheck(t *testing.T) {
	s := openTestStore(t)
	id := newSimCard(t, s, "再送")
	firstAt := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	if _, err := s.ReviewCardGuardedAt(id, RatingGood, "mobile-1", "", firstAt); err != nil {
		t.Fatalf("first review: %v", err)
	}
	secondAt := firstAt.Add(time.Hour)
	if _, err := s.ReviewCardGuardedAt(id, RatingGood, "mobile-2", "mobile-1", secondAt); err != nil {
		t.Fatalf("second review: %v", err)
	}

	// The older token is already committed. Its retry must return the stored
	// result even though its timestamp now precedes the card's chain tip.
	if _, err := s.ReviewCardGuardedAt(id, RatingGood, "mobile-1", "", firstAt); err != nil {
		t.Fatalf("idempotent retry: %v", err)
	}
}
