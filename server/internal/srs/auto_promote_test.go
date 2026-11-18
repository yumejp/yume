package srs

import (
	"testing"
	"time"
)

// timeUnixHour is the review clock the sim tests use: due date + 1h, like a
// human reviewing the moment a card comes up.
func timeUnixHour(due int64) time.Time { return time.Unix(due, 0).Add(time.Hour) }

// wordStatus reads a lemma's word_status, returning -1 when no row exists
// (i.e. "unknown" — the review loop must never fabricate one).
func wordStatus(t *testing.T, s *Store, lemma string) int {
	t.Helper()
	var st int
	err := s.q().QueryRow(`SELECT status FROM word_status WHERE lemma = ?`, lemma).Scan(&st)
	if err != nil {
		return -1
	}
	return st
}

// A Learning card answered Good repeatedly must cross knownStabilityDays and
// auto-promote to Known — exactly once, reported via ReviewResult.Promoted,
// and idempotently thereafter (no longer status=1, so nothing to promote).
func TestAutoPromoteLearningToKnown(t *testing.T) {
	s := openTestStore(t)
	id := newSimCard(t, s, "習慣")
	if err := s.SetWordStatus("習慣", 1); err != nil { // Learning
		t.Fatalf("SetWordStatus: %v", err)
	}

	promotions := 0
	var crossed bool
	for i := 0; i < 15; i++ {
		cs := readCardState(t, s, id)
		at := timeUnixHour(cs.due)
		res, err := s.reviewCardAt(id, RatingGood, at)
		if err != nil {
			t.Fatalf("review %d: %v", i, err)
		}
		if res.Promoted {
			promotions++
			// Must coincide with stability actually crossing the threshold.
			ns := readCardState(t, s, id)
			if ns.stability < knownStabilityDays {
				t.Fatalf("promoted at stability %.2f < %.1f", ns.stability, knownStabilityDays)
			}
			crossed = true
		}
		if crossed && wordStatus(t, s, "習慣") != 2 {
			t.Fatalf("review %d: promoted but status = %d, want Known(2)", i, wordStatus(t, s, "習慣"))
		}
	}
	if promotions != 1 {
		t.Fatalf("Promoted fired %d times, want exactly 1", promotions)
	}
	if got := wordStatus(t, s, "習慣"); got != 2 {
		t.Fatalf("final status = %d, want Known(2)", got)
	}
}

// One lapse keeps Known: the card reschedules but the label survives a
// single miss. The second lapse from Review within 30 days demotes it.
func TestAutoDemoteKnownOnSecondLapse(t *testing.T) {
	s := openTestStore(t)
	id := newSimCard(t, s, "油断")
	for i := 0; i < 6; i++ {
		reviewWhenDue(t, s, id, RatingGood)
	}
	if st := readCardState(t, s, id).state; st != 2 {
		t.Fatalf("card state = %d, want Review(2) before lapse", st)
	}
	if err := s.SetWordStatus("油断", 2); err != nil {
		t.Fatalf("SetWordStatus: %v", err)
	}
	cs := readCardState(t, s, id)
	at := timeUnixHour(cs.due)
	res, err := s.reviewCardAt(id, RatingAgain, at)
	if err != nil {
		t.Fatalf("first lapse: %v", err)
	}
	if res.NewState != 3 || res.Demoted {
		t.Fatalf("first lapse: state=%d demoted=%v, want Relearning(3) and no demotion", res.NewState, res.Demoted)
	}
	// A same-session lap Again is not a second lapse.
	if res, err = s.reviewCardAt(id, RatingAgain, at.Add(5*time.Minute)); err != nil || res.Demoted {
		t.Fatalf("lap Again: err=%v demoted=%v", err, res.Demoted)
	}
	if got := wordStatus(t, s, "油断"); got != 2 {
		t.Fatalf("after one lapse status = %d, want Known(2)", got)
	}
	// Recover, then miss again within the window.
	reviewWhenDue(t, s, id, RatingGood)
	reviewWhenDue(t, s, id, RatingGood)
	if st := readCardState(t, s, id).state; st != 2 {
		t.Fatalf("card state = %d, want Review(2) before second lapse", st)
	}
	cs = readCardState(t, s, id)
	res, err = s.reviewCardAt(id, RatingAgain, timeUnixHour(cs.due))
	if err != nil {
		t.Fatalf("second lapse: %v", err)
	}
	if !res.Demoted {
		t.Fatal("second lapse within the window did not demote")
	}
	if got := wordStatus(t, s, "油断"); got != 1 {
		t.Fatalf("after second lapse status = %d, want Learning(1)", got)
	}
}

// A lapse more than 30 days after the previous one counts as a first lapse.
func TestAutoDemoteLapseWindowExpires(t *testing.T) {
	s := openTestStore(t)
	id := newSimCard(t, s, "窓")
	for i := 0; i < 6; i++ {
		reviewWhenDue(t, s, id, RatingGood)
	}
	if err := s.SetWordStatus("窓", 2); err != nil {
		t.Fatalf("SetWordStatus: %v", err)
	}
	cs := readCardState(t, s, id)
	first := timeUnixHour(cs.due)
	if _, err := s.reviewCardAt(id, RatingAgain, first); err != nil {
		t.Fatalf("first lapse: %v", err)
	}
	if _, err := s.reviewCardAt(id, RatingGood, first.Add(5*time.Minute)); err != nil {
		t.Fatalf("recover: %v", err)
	}
	window := time.Duration(s.tune.DemoteLapseWindowDays * float64(24*time.Hour))
	res, err := s.reviewCardAt(id, RatingAgain, first.Add(window+24*time.Hour))
	if err != nil {
		t.Fatalf("late lapse: %v", err)
	}
	if res.Demoted || wordStatus(t, s, "窓") != 2 {
		t.Fatalf("lapse outside the window demoted (demoted=%v)", res.Demoted)
	}
}

// demote_after_lapses = 1 restores per-miss demotion for deployments that
// want it.
func TestAutoDemoteEveryLapseWhenTuned(t *testing.T) {
	s := openTestStore(t)
	s.tune.DemoteAfterLapses = 1
	id := newSimCard(t, s, "即降格")
	for i := 0; i < 6; i++ {
		reviewWhenDue(t, s, id, RatingGood)
	}
	if err := s.SetWordStatus("即降格", 2); err != nil {
		t.Fatalf("SetWordStatus: %v", err)
	}
	cs := readCardState(t, s, id)
	res, err := s.reviewCardAt(id, RatingAgain, timeUnixHour(cs.due))
	if err != nil {
		t.Fatalf("lapse: %v", err)
	}
	if !res.Demoted || wordStatus(t, s, "即降格") != 1 {
		t.Fatalf("demote_after_lapses=1 did not demote on the first miss (demoted=%v)", res.Demoted)
	}
}

// Ignored (status=3) must be immune: a card the learner explicitly set aside
// stays Ignored no matter how well FSRS thinks it's remembered.
func TestAutoPromoteSkipsIgnored(t *testing.T) {
	s := openTestStore(t)
	id := newSimCard(t, s, "無視")
	if err := s.SetWordStatus("無視", 3); err != nil { // Ignored
		t.Fatalf("SetWordStatus: %v", err)
	}
	for i := 0; i < 15; i++ {
		cs := readCardState(t, s, id)
		res, err := s.reviewCardAt(id, RatingGood, timeUnixHour(cs.due))
		if err != nil {
			t.Fatalf("review %d: %v", i, err)
		}
		if res.Promoted {
			t.Fatalf("review %d: Ignored card reported Promoted", i)
		}
	}
	if got := wordStatus(t, s, "無視"); got != 3 {
		t.Fatalf("status = %d, want Ignored(3) untouched", got)
	}
}

// A card with no word_status row (unknown) must never have one invented by the
// review loop — promotion only ever flips an existing Learning row.
func TestAutoPromoteDoesNotCreateStatus(t *testing.T) {
	s := openTestStore(t)
	id := newSimCard(t, s, "未知")
	for i := 0; i < 15; i++ {
		cs := readCardState(t, s, id)
		res, err := s.reviewCardAt(id, RatingGood, timeUnixHour(cs.due))
		if err != nil {
			t.Fatalf("review %d: %v", i, err)
		}
		if res.Promoted {
			t.Fatalf("review %d: unknown card reported Promoted", i)
		}
	}
	if got := wordStatus(t, s, "未知"); got != -1 {
		t.Fatalf("status row was created (=%d); want none (-1)", got)
	}
}
