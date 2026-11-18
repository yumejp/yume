package srs

import (
	"testing"
	"time"
)

// day returns the local-date string N days before the reference.
func day(ref time.Time, back int) string {
	return ref.AddDate(0, 0, -back).Format("2006-01-02")
}

func TestComputeStreaks(t *testing.T) {
	now := time.Date(2026, 5, 31, 10, 0, 0, 0, time.Local)

	cases := []struct {
		name        string
		activeBack  []int // days-ago that have activity
		wantCurrent int
		wantLongest int
	}{
		{
			name:        "empty",
			activeBack:  nil,
			wantCurrent: 0,
			wantLongest: 0,
		},
		{
			name:        "today only",
			activeBack:  []int{0},
			wantCurrent: 1,
			wantLongest: 1,
		},
		{
			name:        "three consecutive days ending today",
			activeBack:  []int{0, 1, 2},
			wantCurrent: 3,
			wantLongest: 3,
		},
		{
			name:        "streak alive from yesterday (nothing today yet)",
			activeBack:  []int{1, 2, 3},
			wantCurrent: 3, // counts back from yesterday
			wantLongest: 3,
		},
		{
			name:        "broken streak — gap two days ago",
			activeBack:  []int{0, 1, 3, 4, 5},
			wantCurrent: 2, // today + yesterday
			wantLongest: 3, // days 3,4,5
		},
		{
			name:        "stale — last activity 3 days ago",
			activeBack:  []int{3, 4},
			wantCurrent: 0, // neither today nor yesterday active
			wantLongest: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			activity := map[string]int{}
			for _, b := range tc.activeBack {
				activity[day(now, b)] = 1
			}
			cur, longest := computeStreaks(activity, now, time.Local)
			if cur != tc.wantCurrent {
				t.Errorf("current = %d, want %d", cur, tc.wantCurrent)
			}
			if longest != tc.wantLongest {
				t.Errorf("longest = %d, want %d", longest, tc.wantLongest)
			}
		})
	}
}

func TestComputeStreaksClientZone(t *testing.T) {
	// 17:00 UTC on July 9 is already July 10, 02:00 in Japan (+540). A review
	// bucketed on the learner's "today" must count toward the current streak
	// even though the server's day hasn't rolled over yet.
	jst := time.FixedZone("client", 540*60)
	now := time.Date(2026, 7, 9, 17, 0, 0, 0, time.UTC)
	activity := map[string]int{"2026-07-10": 1}
	cur, longest := computeStreaks(activity, now, jst)
	if cur != 1 || longest != 1 {
		t.Errorf("JST streak = (%d,%d), want (1,1)", cur, longest)
	}
}

func TestDateExprClientZone(t *testing.T) {
	jst := time.FixedZone("client", 540*60)
	if got, want := dateExpr("ts", jst), "date(ts + 32400, 'unixepoch')"; got != want {
		t.Errorf("dateExpr = %q, want %q", got, want)
	}
	if got, want := dateExpr("ts", time.Local), "date(ts, 'unixepoch', 'localtime')"; got != want {
		t.Errorf("dateExpr(local) = %q, want %q", got, want)
	}
}

// reviews_today must be bounded at the learner's midnight — a review that
// happened "yesterday" in the learner's zone must not count, and one from
// earlier the same learner-day must, regardless of the server's day.
func TestReviewsToday(t *testing.T) {
	s := openTestStore(t)
	r, err := s.SaveCard(SaveCardReq{Lemma: "操作", Glosses: [][]string{{"operation"}}})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	now := time.Now()
	// Two reviews today, one ~26h ago (yesterday whatever the zone).
	if _, err := s.reviewCardAt(r.ID, RatingGood, now.Add(-26*time.Hour)); err != nil {
		t.Fatalf("review -26h: %v", err)
	}
	if _, err := s.reviewCardAt(r.ID, RatingGood, now.Add(-2*time.Hour)); err != nil {
		t.Fatalf("review -2h: %v", err)
	}
	if _, err := s.reviewCardAt(r.ID, RatingGood, now); err != nil {
		t.Fatalf("review now: %v", err)
	}
	// Pass the machine's current offset explicitly and compute the expected
	// count in the SAME fixed zone the query buckets with — using raw local
	// Format here would disagree during the DST fall-back hour (the query
	// applies one fixed offset to all timestamps; local Format applies
	// per-timestamp DST rules).
	_, offSec := now.Zone()
	offMin := offSec / 60
	loc := time.FixedZone("client", offMin*60)
	today := now.In(loc).Format("2006-01-02")
	want := 1 // the `now` review is always today
	if now.Add(-2*time.Hour).In(loc).Format("2006-01-02") == today {
		want = 2
	}
	stats, err := s.ComputeStatsTZ(&offMin)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	// ComputeStatsTZ takes its own time.Now(); if midnight rolled between our
	// inserts and that read, day-bounded expectations are ambiguous — skip
	// (the check runs AFTER the call, so a same-day reading here proves the
	// internal Now was also same-day).
	if time.Now().In(loc).Format("2006-01-02") != today {
		t.Skip("crossed midnight mid-test — day-bounded assertion ambiguous")
	}
	if stats.ReviewsToday != want {
		t.Errorf("reviews_today = %d, want %d", stats.ReviewsToday, want)
	}
	if stats.TotalReviews != 3 {
		t.Errorf("total_reviews = %d, want 3", stats.TotalReviews)
	}
}

// An explicit zero offset is a REAL zone (UTC learner), not "unset" — it must
// bucket at UTC midnight, not the server's. nil stays server-local.
func TestExplicitZeroOffsetIsUTC(t *testing.T) {
	zero := 0
	if loc := clientLocation(&zero); loc == time.Local {
		t.Error("clientLocation(&0) returned time.Local — explicit UTC aliased to server zone")
	}
	now := time.Now()
	utcMidnight := func() int64 {
		y, m, d := now.UTC().Date()
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Unix()
	}()
	if got := dayStart(now, &zero); got != utcMidnight {
		t.Errorf("dayStart(&0) = %d, want UTC midnight %d", got, utcMidnight)
	}
}

func TestNextMilestone(t *testing.T) {
	cases := []struct{ have, want int }{
		{0, 50},
		{3, 50},
		{50, 100},
		{75, 100},
		{100, 250},
		{499, 500},
		{500, 1000},
		{9999, 10000},
		{10000, 15000},
		{23000, 25000}, // past the ladder → round up to next 5000
	}
	for _, c := range cases {
		if got := nextMilestone(c.have); got != c.want {
			t.Errorf("nextMilestone(%d) = %d, want %d", c.have, got, c.want)
		}
	}
}

func TestLastNDays(t *testing.T) {
	activity := map[string]int{
		"2026-05-01": 1,
		"2026-05-10": 2,
		"2026-05-20": 3,
		"2026-05-31": 4,
	}
	got := lastNDays(activity, 2)
	if len(got) != 2 {
		t.Fatalf("want 2 days, got %d", len(got))
	}
	// Must be the two most recent, sorted ascending.
	if got[0].Date != "2026-05-20" || got[1].Date != "2026-05-31" {
		t.Errorf("got %+v, want [05-20, 05-31]", got)
	}
}

// Never-seen cards are started on request, not scheduled: they must stay out
// of due_now and the forecast and be reported as new_waiting instead. An
// import used to show "All caught up" beside "2001 due today".
func TestDueCountsExcludeNewCards(t *testing.T) {
	s := openTestStore(t)
	ids := map[string]int64{}
	for _, w := range []string{"復習", "明日", "新一", "新二"} {
		r, err := s.SaveCard(SaveCardReq{Lemma: w, Glosses: [][]string{{"g"}}})
		if err != nil {
			t.Fatalf("save %s: %v", w, err)
		}
		ids[w] = r.ID
	}
	now := time.Now()
	for w, due := range map[string]int64{"復習": 0, "明日": now.Add(26 * time.Hour).Unix()} {
		if _, err := s.ReviewCard(ids[w], RatingGood); err != nil {
			t.Fatalf("review %s: %v", w, err)
		}
		if _, err := s.q().Exec(`UPDATE cards SET due = ? WHERE id = ?`, due, ids[w]); err != nil {
			t.Fatalf("set due %s: %v", w, err)
		}
	}
	// A never-seen card with a future due (as some imports write) must not
	// reach the forecast either.
	if _, err := s.q().Exec(`UPDATE cards SET due = ? WHERE id = ?`, now.Add(26*time.Hour).Unix(), ids["新二"]); err != nil {
		t.Fatalf("set new due: %v", err)
	}

	stats, err := s.ComputeStats()
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.Totals.DueNow != 1 || stats.Totals.NewWaiting != 2 {
		t.Errorf("due_now/new_waiting = %d/%d, want 1/2", stats.Totals.DueNow, stats.Totals.NewWaiting)
	}
	forecast := 0
	for _, d := range stats.DueForecast {
		forecast += d.Count
	}
	if forecast != 1 {
		t.Errorf("forecast total = %d, want 1 (the reviewed card due tomorrow): %+v", forecast, stats.DueForecast)
	}
}
