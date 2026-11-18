package srs

import (
	"testing"
	"time"
)

func TestDueNowLeavesOutNewCards(t *testing.T) {
	s := openTestStore(t)
	review := newSimCard(t, s, "復習")
	if _, err := s.q().Exec(`UPDATE cards SET state = 2, due = ? WHERE id = ?`, time.Now().Add(-time.Hour).Unix(), review); err != nil {
		t.Fatal(err)
	}
	newSimCard(t, s, "新語") // never introduced: due since creation, but not a review

	st, err := s.ComputeStats()
	if err != nil {
		t.Fatal(err)
	}
	if st.Totals.DueNow != 1 {
		t.Errorf("due_now = %d, want 1 (the review only)", st.Totals.DueNow)
	}
}
