package srs

import (
	"testing"
	"time"
)

func TestPlanCatchUp(t *testing.T) {
	cases := []struct {
		name                           string
		backlog, done, upcoming, limit int
		wantNil                        bool
		remaining, days                int
	}{
		{name: "within the limit", backlog: 140, upcoming: 200, limit: 150, wantNil: true},
		{name: "limit off", backlog: 900, upcoming: 200, limit: 0, wantNil: true},
		{name: "nothing due", backlog: 0, done: 400, upcoming: 200, limit: 150, wantNil: true},
		{name: "over the limit", backlog: 402, upcoming: 266, limit: 150, remaining: 150, days: 4},
		{name: "part done today", backlog: 352, done: 50, upcoming: 266, limit: 150, remaining: 100, days: 4},
		{name: "today's share done", backlog: 252, done: 150, upcoming: 266, limit: 150, remaining: 0, days: 4},
		{name: "limit can't keep up", backlog: 500, upcoming: 7 * 160, limit: 150, remaining: 150, days: -1},
	}
	for _, c := range cases {
		got := planCatchUp(c.backlog, c.done, c.upcoming, c.limit)
		if c.wantNil {
			if got != nil {
				t.Errorf("%s: got %+v, want no plan", c.name, got)
			}
			continue
		}
		if got == nil || got.TargetToday != c.limit || got.RemainingToday != c.remaining || got.DaysLeft != c.days {
			t.Errorf("%s: got %+v, want remaining %d days %d", c.name, got, c.remaining, c.days)
		}
	}
}

func TestReviewLimit(t *testing.T) {
	s := openTestStore(t)
	if n, err := s.ReviewLimit(); err != nil || n != defaultReviewLimit {
		t.Fatalf("default = %d, %v", n, err)
	}
	if err := s.SetReviewLimit(0); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.ReviewLimit(); n != 0 {
		t.Fatalf("after off = %d", n)
	}
	if err := s.SetReviewLimit(-1); err == nil {
		t.Error("negative limit accepted")
	}
}

func TestCatchUpCardsLowestRecallFirst(t *testing.T) {
	s := openTestStore(t)
	now := time.Now()
	day := int64(24 * 60 * 60)
	set := func(lemma string, stability float64, lastReviewDaysAgo int64) {
		id := newSimCard(t, s, lemma)
		if _, err := s.q().Exec(`UPDATE cards SET state = 2, stability = ?, last_review = ?, due = ? WHERE id = ?`,
			stability, now.Unix()-lastReviewDaysAgo*day, now.Unix()-day, id); err != nil {
			t.Fatal(err)
		}
	}
	set("安定", 60, 10)      // ratio 0.17: still well remembered
	set("危険", 2, 8)        // ratio 4: most at risk
	set("普通", 5, 7)        // ratio 1.4
	newSimCard(t, s, "新語") // never reviewed: not part of catch-up

	cards, err := s.GetCatchUpCards(10, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range cards {
		got = append(got, c.Lemma)
	}
	want := []string{"危険", "普通", "安定"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
