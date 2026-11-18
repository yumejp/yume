package srs

import (
	"testing"
	"time"
)

// Every screen that says how much is due must agree: the review count, the
// stats' due today, the cards served and catch-up's backlog.
func TestDueCountsAgree(t *testing.T) {
	s := openTestStore(t)
	now := time.Now()
	add := func(lemma string, state int, due time.Time) {
		id := newSimCard(t, s, lemma)
		if _, err := s.q().Exec(`UPDATE cards SET state = ?, due = ?, last_review = ? WHERE id = ?`,
			state, due.Unix(), due.Add(-48*time.Hour).Unix(), id); err != nil {
			t.Fatal(err)
		}
	}
	add("昨日", 2, now.Add(-30*time.Hour))
	add("先週", 2, now.Add(-7*24*time.Hour))
	add("学習中", 1, now.Add(-time.Minute))
	add("来週", 2, now.Add(7*24*time.Hour))
	newSimCard(t, s, "新語")                      // waits for "Learn new words"
	if err := s.SetReviewLimit(1); err != nil { // so catch-up reports its backlog
		t.Fatal(err)
	}

	check := func(when string, want int) {
		t.Helper()
		ds, err := s.DeckStatsTZ(nil)
		if err != nil {
			t.Fatal(err)
		}
		st, err := s.ComputeStats()
		if err != nil {
			t.Fatal(err)
		}
		cards, err := s.GetDueCardsGatedTZ(50, 0, nil)
		if err != nil {
			t.Fatal(err)
		}
		got := []int{ds.DueReviews, st.Totals.DueNow, len(cards)}
		plan, err := s.CatchUpPlan(nil)
		if err != nil {
			t.Fatal(err)
		}
		if plan != nil { // only when more is due than the limit of 1
			got = append(got, plan.Backlog)
		} else if want > 1 {
			t.Errorf("%s: no catch-up plan with %d due over a limit of 1", when, want)
		}
		for _, n := range got {
			if n != want {
				t.Errorf("%s: review count, stats, served, backlog = %v; want all %d", when, got, want)
				break
			}
		}
	}
	check("normal", 3)

	// Vacation started two days ago: 昨日 came due during it and waits.
	if _, _, err := s.setVacationAt(true, now.Add(-48*time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	check("on vacation", 1)
}
