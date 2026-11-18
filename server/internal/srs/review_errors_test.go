package srs

import (
	"math"
	"strings"
	"testing"
	"time"
)

// The offline replayer permanently drops a rating on 4xx and retries on 5xx,
// so the client/internal split must hold: request-caused errors are
// IsClientErr, internal failures are not.
func TestReviewErrorClassification(t *testing.T) {
	s := openTestStore(t)
	id := newSimCard(t, s, "分類")

	if _, err := s.ReviewCard(id, ReviewRating(9)); !IsClientErr(err) {
		t.Errorf("invalid rating: IsClientErr = false, err = %v", err)
	}
	if _, err := s.ReviewCard(999999, RatingGood); !IsClientErr(err) {
		t.Errorf("unknown card: IsClientErr = false, err = %v", err)
	}
	if _, err := s.ReviewCardIdem(id, RatingGood, "bad token!"); !IsClientErr(err) {
		t.Errorf("invalid review_id: IsClientErr = false, err = %v", err)
	}
	if _, _, err := s.undoReviewAt(time.Now(), nil); !IsClientErr(err) {
		t.Errorf("nothing to undo: IsClientErr = false, err = %v", err)
	}
	if IsClientErr(nil) {
		t.Error("IsClientErr(nil) = true")
	}
}

// An untokened review must still leave a non-empty review_id at the chain
// tip — "" doubles as the concurrency guard's "no reviews yet" sentinel, so
// an untokened writer would otherwise let a stale offline rating replay on
// top of its review undetected.
func TestUntokenedReviewMintsToken(t *testing.T) {
	s := openTestStore(t)
	id := newSimCard(t, s, "刻印")
	if _, err := s.ReviewCard(id, RatingGood); err != nil {
		t.Fatalf("review: %v", err)
	}
	var tok string
	if err := s.q().QueryRow(
		`SELECT review_id FROM review_log WHERE card_id = ? ORDER BY id DESC LIMIT 1`, id,
	).Scan(&tok); err != nil {
		t.Fatalf("read token: %v", err)
	}
	if !strings.HasPrefix(tok, "srv-") || len(tok) != len("srv-")+32 {
		t.Errorf("minted token = %q, want srv-<32 hex>", tok)
	}
	if err := validateReviewID(tok); err != nil {
		t.Errorf("minted token fails its own validation: %v", err)
	}
}

// Undo reports the review_id of the row it reverted so a client can tell
// whether it undid its own rating or one another device committed since.
func TestUndoReturnsReviewID(t *testing.T) {
	s := openTestStore(t)
	id := newSimCard(t, s, "取消")
	if _, err := s.ReviewCardIdem(id, RatingGood, "tok-undo-1"); err != nil {
		t.Fatalf("review: %v", err)
	}
	cardID, reviewID, err := s.undoReviewAt(time.Now(), nil)
	if err != nil {
		t.Fatalf("undo: %v", err)
	}
	if cardID != id || reviewID != "tok-undo-1" {
		t.Errorf("undo = (card %d, %q), want (card %d, %q)", cardID, reviewID, id, "tok-undo-1")
	}
}

// NaN/Inf pass every ordered comparison, so they need explicit repair — one
// persisted NaN would otherwise cascade through go-fsrs and (stored as NULL
// by the sqlite driver) break every plain-float64 scan of the row.
func TestSanitizeFSRSNaN(t *testing.T) {
	nan, inf := math.NaN(), math.Inf(1)
	review := int(2) // fsrs.Review — non-New so repair applies

	if st, _ := sanitizeFSRS(review, nan, 5); st != 0.1 {
		t.Errorf("NaN stability -> %v, want 0.1", st)
	}
	if st, _ := sanitizeFSRS(review, inf, 5); st != 0.1 {
		t.Errorf("+Inf stability -> %v, want 0.1", st)
	}
	if _, d := sanitizeFSRS(review, 1, nan); d != 5 {
		t.Errorf("NaN difficulty -> %v, want 5", d)
	}
	if _, d := sanitizeFSRS(review, 1, inf); d != 10 {
		t.Errorf("+Inf difficulty -> %v, want 10", d)
	}
	if _, d := sanitizeFSRS(review, 1, -inf); d != 5 {
		t.Errorf("-Inf difficulty -> %v, want 5", d)
	}
	// In-domain values pass through untouched.
	if st, d := sanitizeFSRS(review, 3.5, 7); st != 3.5 || d != 7 {
		t.Errorf("valid values changed: (%v, %v)", st, d)
	}
}

// Non-Again ratings floor at one day: a card cleared in a sitting must not
// re-due minutes later (the intraday treadmill — "finish a batch, 50 more
// due"). Again stays short: its same-day return is the failed-card drill.
func TestNonAgainFloorsAtOneDay(t *testing.T) {
	s := openTestStore(t)
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)

	for _, tc := range []struct {
		rating ReviewRating
		name   string
	}{{RatingHard, "hard"}, {RatingGood, "good"}, {RatingEasy, "easy"}} {
		id := newSimCard(t, s, "床"+tc.name)
		res, err := s.reviewCardIdemAt(id, tc.rating, "", nil, now)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		due, err := time.Parse(time.RFC3339, res.NewDue)
		if err != nil {
			t.Fatalf("%s: parse due: %v", tc.name, err)
		}
		if due.Before(now.Add(24 * time.Hour)) {
			t.Errorf("%s on a new card due %v — want >= now+24h", tc.name, due.Sub(now))
		}
	}

	// Again keeps its short (sub-day) return.
	id := newSimCard(t, s, "床again")
	res, err := s.reviewCardIdemAt(id, RatingAgain, "", nil, now)
	if err != nil {
		t.Fatal(err)
	}
	due, _ := time.Parse(time.RFC3339, res.NewDue)
	if !due.Before(now.Add(24 * time.Hour)) {
		t.Errorf("again due %v out — want sub-day", due.Sub(now))
	}

	// The preview promises the same floor the persist path applies.
	if res.Preview == nil {
		t.Fatal("no preview")
	}
	for name, unix := range map[string]int64{
		"hard": res.Preview.Hard, "good": res.Preview.Good, "easy": res.Preview.Easy,
	} {
		if time.Unix(unix, 0).Before(now.Add(24 * time.Hour)) {
			t.Errorf("preview %s = %v — want >= now+24h", name, time.Unix(unix, 0).Sub(now))
		}
	}
	if !time.Unix(res.Preview.Again, 0).Before(now.Add(24 * time.Hour)) {
		t.Error("preview again should stay sub-day")
	}
}
