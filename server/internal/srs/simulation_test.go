package srs

import (
	"database/sql"
	"math"
	"math/rand"
	"testing"
	"time"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v3"
)

// FSRS scheduling simulations. Real-time testing of an SRS is impossible
// (intervals span months), but scheduling is a pure function of
// (state, rating, now) — reviewCardAt's injectable clock lets these tests
// compress a year of study into milliseconds and assert the properties a
// working SRS must have.

type cardState struct {
	due           int64
	stability     float64
	difficulty    float64
	scheduledDays float64
	reps, lapses  int
	state         int
}

func readCardState(t *testing.T, s *Store, id int64) cardState {
	t.Helper()
	var cs cardState
	err := s.q().QueryRow(`
		SELECT due, stability, difficulty, scheduled_days, reps, lapses, state
		FROM cards WHERE id = ?`, id).
		Scan(&cs.due, &cs.stability, &cs.difficulty, &cs.scheduledDays, &cs.reps, &cs.lapses, &cs.state)
	if err != nil {
		t.Fatalf("read card state: %v", err)
	}
	return cs
}

func newSimCard(t *testing.T, s *Store, lemma string) int64 {
	t.Helper()
	r, err := s.SaveCard(SaveCardReq{Lemma: lemma, Glosses: [][]string{{"gloss"}}})
	if err != nil {
		t.Fatalf("SaveCard: %v", err)
	}
	return r.ID
}

// Review the card with `rating` exactly when it comes due (plus an hour, like
// a human would), returning the new state and the review time used.
func reviewWhenDue(t *testing.T, s *Store, id int64, rating ReviewRating) (cardState, time.Time) {
	t.Helper()
	cs := readCardState(t, s, id)
	at := time.Unix(cs.due, 0).Add(time.Hour)
	if _, err := s.reviewCardAt(id, rating, at); err != nil {
		t.Fatalf("reviewCardAt: %v", err)
	}
	return readCardState(t, s, id), at
}

// A card answered Good every time must graduate to Review state and its
// intervals must grow — that's the entire point of spaced repetition. We
// also pin down rough magnitudes: double digits of days within ~8 reviews,
// and due always equals review-time + scheduled interval.
func TestIntervalsGrowUnderGoodRatings(t *testing.T) {
	s := openTestStore(t)
	id := newSimCard(t, s, "勉強")

	var prevInterval float64 = -1
	var intervals []float64
	for i := 0; i < 10; i++ {
		cs, at := reviewWhenDue(t, s, id, RatingGood)
		intervals = append(intervals, cs.scheduledDays)

		// due == review time + interval (FSRS contract through our storage).
		wantDue := at.Add(time.Duration(cs.scheduledDays) * 24 * time.Hour)
		if d := math.Abs(float64(cs.due - wantDue.Unix())); d > 90000 { // ≤ ~1 day slack (rounding)
			t.Fatalf("review %d: due drifted %fs from now+interval", i+1, d)
		}
		if cs.reps != i+1 {
			t.Fatalf("review %d: reps = %d", i+1, cs.reps)
		}
		// Monotone growth once in Review state (the learning steps may be
		// sub-day, so only enforce from the 2nd review on).
		if i >= 1 && cs.scheduledDays < prevInterval {
			t.Fatalf("interval shrank under Good: %v", intervals)
		}
		prevInterval = cs.scheduledDays
	}
	final := readCardState(t, s, id)
	if final.state != 2 {
		t.Errorf("after 10 Goods state = %d, want Review(2)", final.state)
	}
	if final.scheduledDays < 30 {
		t.Errorf("after 10 Goods interval = %.1f days, want ≥ 30; growth: %v",
			final.scheduledDays, intervals)
	}
	if final.lapses != 0 {
		t.Errorf("lapses = %d, want 0", final.lapses)
	}
	t.Logf("interval growth (days): %v", intervals)
}

// At any single decision point, the four ratings must order the next due
// date: Again < Hard ≤ Good ≤ Easy, with Again collapsing hard relative to
// Easy. Checked via the same preview path get_due_cards serves.
func TestRatingOrdering(t *testing.T) {
	s := openTestStore(t)
	id := newSimCard(t, s, "順序")
	// Mature the card a bit first so the orderings aren't all sub-day steps.
	for i := 0; i < 5; i++ {
		reviewWhenDue(t, s, id, RatingGood)
	}
	cs := readCardState(t, s, id)
	at := time.Unix(cs.due, 0).Add(time.Hour)

	var dues [5]int64
	for r := 1; r <= 4; r++ {
		// Each rating must see the identical starting state: review, record
		// the resulting due, then roll back via the undo machinery (which
		// this also exercises under simulation).
		before := readCardState(t, s, id)
		res, err := s.reviewCardAt(id, ReviewRating(r), at)
		if err != nil {
			t.Fatalf("rating %d: %v", r, err)
		}
		parsed, _ := time.Parse(time.RFC3339, res.NewDue)
		dues[r] = parsed.Unix()
		if _, err := s.UndoReview(); err != nil {
			t.Fatalf("undo after rating %d: %v", r, err)
		}
		after := readCardState(t, s, id)
		if after != before {
			t.Fatalf("undo did not restore state for rating %d: %+v vs %+v", r, before, after)
		}
	}
	if !(dues[1] < dues[2] && dues[2] <= dues[3] && dues[3] <= dues[4]) {
		t.Errorf("rating ordering violated: Again=%d Hard=%d Good=%d Easy=%d",
			dues[1], dues[2], dues[3], dues[4])
	}
	if dues[4]-dues[1] < int64(24*time.Hour/time.Second) {
		t.Errorf("Easy and Again dues within a day of each other on a mature card")
	}
}

// Forgetting must hurt: Again on a mature card → Relearning state, a lapse,
// and a collapsed interval; subsequent Goods must rebuild the schedule.
func TestLapseCollapsesIntervalAndRecovers(t *testing.T) {
	s := openTestStore(t)
	id := newSimCard(t, s, "失敗")
	for i := 0; i < 8; i++ {
		reviewWhenDue(t, s, id, RatingGood)
	}
	mature := readCardState(t, s, id)

	lapsed, _ := reviewWhenDue(t, s, id, RatingAgain)
	if lapsed.lapses != 1 {
		t.Errorf("lapses = %d, want 1", lapsed.lapses)
	}
	if lapsed.state != 3 && lapsed.state != 1 { // Relearning (or Learning per param config)
		t.Errorf("state after Again = %d, want Relearning(3)", lapsed.state)
	}
	if lapsed.scheduledDays >= mature.scheduledDays {
		t.Errorf("interval did not collapse on lapse: %.1f → %.1f",
			mature.scheduledDays, lapsed.scheduledDays)
	}
	if lapsed.stability >= mature.stability {
		t.Errorf("stability did not drop on lapse: %.1f → %.1f",
			mature.stability, lapsed.stability)
	}

	// Recovery: a few Goods bring it back to Review with growing intervals.
	var rec cardState
	for i := 0; i < 4; i++ {
		rec, _ = reviewWhenDue(t, s, id, RatingGood)
	}
	if rec.state != 2 {
		t.Errorf("state after recovery = %d, want Review(2)", rec.state)
	}
	if rec.scheduledDays <= lapsed.scheduledDays {
		t.Errorf("no regrowth after lapse: %.1f → %.1f", lapsed.scheduledDays, rec.scheduledDays)
	}
}

// The interval preview shown on the rating buttons must match what actually
// happens when the user rates at that moment — an inconsistent preview
// erodes trust in the whole scheduler.
func TestPreviewMatchesActualSchedule(t *testing.T) {
	s := openTestStore(t)
	id := newSimCard(t, s, "予告")
	for i := 0; i < 3; i++ {
		reviewWhenDue(t, s, id, RatingGood)
	}
	cs := readCardState(t, s, id)
	at := time.Unix(cs.due, 0).Add(time.Hour)

	// Recompute the preview exactly as GetDueCards does, at the same clock.
	var stability, difficulty, elapsedDays, scheduledDays float64
	var reps, lapses, state int
	var lastReview sql.NullInt64
	var dueEpoch int64
	err := s.q().QueryRow(`SELECT due, stability, difficulty, elapsed_days, scheduled_days,
		reps, lapses, state, last_review FROM cards WHERE id = ?`, id).
		Scan(&dueEpoch, &stability, &difficulty, &elapsedDays, &scheduledDays, &reps, &lapses, &state, &lastReview)
	if err != nil {
		t.Fatal(err)
	}
	// Same reconstruction GetDueCards performs for its preview.
	card := fsrs.Card{
		Due:           time.Unix(dueEpoch, 0),
		Stability:     stability,
		Difficulty:    difficulty,
		ElapsedDays:   uint64(elapsedDays),
		ScheduledDays: uint64(scheduledDays),
		Reps:          uint64(reps),
		Lapses:        uint64(lapses),
		State:         fsrs.State(state),
	}
	if lastReview.Valid {
		card.LastReview = time.Unix(lastReview.Int64, 0)
	}
	previewGood := s.fsr.Repeat(card, at)[fsrs.Good].Card.Due.Unix()

	res, err := s.reviewCardAt(id, RatingGood, at)
	if err != nil {
		t.Fatal(err)
	}
	actual, _ := time.Parse(time.RFC3339, res.NewDue)
	if d := actual.Unix() - previewGood; d != 0 {
		t.Errorf("preview(Good)=%d actual=%d (Δ %ds)", previewGood, actual.Unix(), d)
	}
}

// A whole year with a 40-card deck and realistic rating noise. The invariants
// that must hold at every step: FSRS values stay in-domain (no NaN/Inf, no
// negative intervals, difficulty within [1,10]), the review log matches the
// review count, and a mature deck demands fewer reviews than a fresh one.
func TestYearLongDeckSimulation(t *testing.T) {
	s := openTestStore(t)
	rng := rand.New(rand.NewSource(7))

	ids := make([]int64, 40)
	for i := range ids {
		ids[i] = newSimCard(t, s, "語"+string(rune('a'+i%26))+string(rune('0'+i/26)))
	}

	start := time.Now()
	totalReviews := 0
	reviewsByMonth := map[int]int{}
	for day := 0; day < 365; day++ {
		dayClock := start.Add(time.Duration(day) * 24 * time.Hour).Add(20 * time.Hour)
		// Review everything due by tonight, like a nightly session.
		rows, err := s.q().Query(`SELECT id FROM cards WHERE due <= ?`, dayClock.Unix())
		if err != nil {
			t.Fatal(err)
		}
		var due []int64
		for rows.Next() {
			var id int64
			_ = rows.Scan(&id)
			due = append(due, id)
		}
		rows.Close()
		for _, id := range due {
			// 88% Good, 7% Hard, 3% Again, 2% Easy — plausible human noise.
			roll := rng.Float64()
			rating := RatingGood
			switch {
			case roll < 0.03:
				rating = RatingAgain
			case roll < 0.10:
				rating = RatingHard
			case roll > 0.98:
				rating = RatingEasy
			}
			if _, err := s.reviewCardAt(id, rating, dayClock); err != nil {
				t.Fatalf("day %d card %d: %v", day, id, err)
			}
			totalReviews++
			reviewsByMonth[day/30]++

			cs := readCardState(t, s, id)
			if math.IsNaN(cs.stability) || math.IsInf(cs.stability, 0) || cs.stability < 0 {
				t.Fatalf("day %d: stability degenerated to %v", day, cs.stability)
			}
			if cs.difficulty < 1 || cs.difficulty > 10 {
				t.Fatalf("day %d: difficulty out of domain: %v", day, cs.difficulty)
			}
			if cs.scheduledDays < 0 || cs.scheduledDays > 36500 {
				t.Fatalf("day %d: absurd interval %v days", day, cs.scheduledDays)
			}
			if cs.due <= dayClock.Unix()-60 {
				t.Fatalf("day %d: card rescheduled into the past", day)
			}
		}
	}

	var logged int
	if err := s.q().QueryRow(`SELECT COUNT(*) FROM review_log`).Scan(&logged); err != nil {
		t.Fatal(err)
	}
	if logged != totalReviews {
		t.Errorf("review_log rows = %d, reviews performed = %d", logged, totalReviews)
	}
	if reviewsByMonth[11] >= reviewsByMonth[0] {
		t.Errorf("workload did not decline as the deck matured: month0=%d month11=%d",
			reviewsByMonth[0], reviewsByMonth[11])
	}
	t.Logf("year simulation: %d total reviews; month 1 = %d, month 12 = %d",
		totalReviews, reviewsByMonth[0], reviewsByMonth[11])
}
