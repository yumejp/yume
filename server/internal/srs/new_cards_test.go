package srs

import "testing"

func countNewCards(cards []DueCard) int {
	n := 0
	for _, c := range cards {
		if c.IsNew {
			n++
		}
	}
	return n
}

// The per-day new-card cap releases at most newLimit New cards across the day,
// counting cards already introduced (first-reviewed) today.
func TestNewCardGating(t *testing.T) {
	s := openTestStore(t)
	var first int64
	for i, w := range []string{"一", "二", "三", "四"} {
		r, err := s.SaveCard(SaveCardReq{Lemma: w, Glosses: [][]string{{"g"}}})
		if err != nil {
			t.Fatalf("save %s: %v", w, err)
		}
		if i == 0 {
			first = r.ID
		}
	}
	// Cap 2, nothing introduced yet → exactly 2 new cards released.
	due, err := s.GetDueCardsGated(50, 2)
	if err != nil {
		t.Fatalf("gated: %v", err)
	}
	if got := countNewCards(due); got != 2 {
		t.Fatalf("cap=2, 0 introduced: got %d new, want 2", got)
	}
	// Introduce one by reviewing it.
	if _, err := s.ReviewCard(first, RatingGood); err != nil {
		t.Fatalf("review: %v", err)
	}
	due, err = s.GetDueCardsGated(50, 2)
	if err != nil {
		t.Fatalf("gated2: %v", err)
	}
	if got := countNewCards(due); got != 1 {
		t.Fatalf("cap=2, 1 introduced: got %d new, want 1", got)
	}
	ds, err := s.DeckStats()
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if ds.NewIntroducedToday != 1 {
		t.Errorf("new_introduced_today = %d, want 1", ds.NewIntroducedToday)
	}
	if ds.NewTotal != 3 {
		t.Errorf("new_total = %d, want 3 (4 saved − 1 reviewed)", ds.NewTotal)
	}
	// The due split: the 3 un-introduced cards are due-new; the reviewed card
	// was rescheduled into the future so nothing is due as a review.
	if ds.DueNew != 3 || ds.DueReviews != 0 {
		t.Errorf("due split = %d reviews + %d new, want 0 + 3", ds.DueReviews, ds.DueNew)
	}
	if ds.Due != ds.DueReviews+ds.DueNew {
		t.Errorf("due = %d, want due_reviews+due_new = %d", ds.Due, ds.DueReviews+ds.DueNew)
	}
	// newLimit<0 disables the cap entirely.
	all, err := s.GetDueCardsGated(50, -1)
	if err != nil {
		t.Fatalf("ungated: %v", err)
	}
	if got := countNewCards(all); got != 3 {
		t.Fatalf("ungated: got %d new, want 3", got)
	}
}

// A card imported WITH review history (reps>0 but no review_log rows — the
// Anki-import shape) must not count as "introduced today" when it gets its
// first in-app review: it's a review, not a new word, and counting it eats
// the day's new-card allowance.
func TestIntroducedTodayIgnoresImportedReviewCards(t *testing.T) {
	s := openTestStore(t)
	r, err := s.SaveCard(SaveCardReq{Lemma: "輸入", Glosses: [][]string{{"g"}}})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	// Shape the card like an import: mature Review state, due now, empty log.
	if _, err := s.q().Exec(
		`UPDATE cards SET state = 2, reps = 6, stability = 100, difficulty = 5, due = 0 WHERE id = ?`,
		r.ID,
	); err != nil {
		t.Fatalf("shape import: %v", err)
	}
	if _, err := s.ReviewCard(r.ID, RatingGood); err != nil {
		t.Fatalf("review: %v", err)
	}
	ds, err := s.DeckStats()
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if ds.NewIntroducedToday != 0 {
		t.Errorf("new_introduced_today = %d, want 0 (imported review card is not an introduction)", ds.NewIntroducedToday)
	}
	// And the quota stays open: a genuinely new card is still released.
	if _, err := s.SaveCard(SaveCardReq{Lemma: "新語", Glosses: [][]string{{"g"}}}); err != nil {
		t.Fatalf("save new: %v", err)
	}
	due, err := s.GetDueCardsGated(50, 1)
	if err != nil {
		t.Fatalf("gated: %v", err)
	}
	if got := countNewCards(due); got != 1 {
		t.Errorf("cap=1 after imported-card review: got %d new, want 1", got)
	}
}

// Same-day relearning churn must not swallow the new-card quota: cards
// re-due from TODAY'S own reviews (Again'd minutes ago) are reported as
// due_intraday so clients can keep a reviews-before-new gate open, and the
// due page RESERVES room for the remaining allowance so a page-filling pile
// of re-due reviews can't return a batch with zero new words in it.
func TestIntradayReviewsDontStarveNewCards(t *testing.T) {
	s := openTestStore(t)
	// Three cards reviewed today and forced re-due (the Again'd-cards shape),
	// plus one genuinely new card.
	for _, w := range []string{"再一", "再二", "再三"} {
		r, err := s.SaveCard(SaveCardReq{Lemma: w, Glosses: [][]string{{"g"}}})
		if err != nil {
			t.Fatalf("save %s: %v", w, err)
		}
		if _, err := s.ReviewCard(r.ID, RatingAgain); err != nil {
			t.Fatalf("review %s: %v", w, err)
		}
		if _, err := s.q().Exec(`UPDATE cards SET due = 0 WHERE id = ?`, r.ID); err != nil {
			t.Fatalf("force due: %v", err)
		}
	}
	if _, err := s.SaveCard(SaveCardReq{Lemma: "新顔", Glosses: [][]string{{"g"}}}); err != nil {
		t.Fatalf("save new: %v", err)
	}
	ds, err := s.DeckStats()
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if ds.DueReviews != 3 || ds.DueIntraday != 3 {
		t.Errorf("due_reviews/due_intraday = %d/%d, want 3/3 (all re-due from today)", ds.DueReviews, ds.DueIntraday)
	}
	// Page of 3 with 1 allowance: the new card must claim its reserved slot
	// even though the 3 re-due reviews alone would fill the page.
	due, err := s.GetDueCardsGated(3, 4) // introduced today: the 3 Again'd cards
	if err != nil {
		t.Fatalf("gated: %v", err)
	}
	if got := countNewCards(due); got != 1 {
		t.Errorf("full page of reviews: got %d new, want 1 reserved", got)
	}
	if len(due) != 3 {
		t.Errorf("page size = %d, want 3", len(due))
	}
	if len(due) == 3 && !due[2].IsNew {
		t.Error("new card should close the page, after the reviews")
	}
}

// The cap must never gate review cards — a reviewed/demoted card (reps>0) is
// always due when due, regardless of the new-card limit.
func TestNewCardGatingIgnoresReviews(t *testing.T) {
	s := openTestStore(t)
	r, err := s.SaveCard(SaveCardReq{Lemma: "復習", Glosses: [][]string{{"g"}}})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := s.ReviewCard(r.ID, RatingGood); err != nil {
		t.Fatalf("review: %v", err)
	}
	if _, err := s.q().Exec(`UPDATE cards SET due = 0 WHERE id = ?`, r.ID); err != nil {
		t.Fatalf("force due: %v", err)
	}
	if _, err := s.SaveCard(SaveCardReq{Lemma: "新規", Glosses: [][]string{{"g"}}}); err != nil {
		t.Fatalf("save new: %v", err)
	}
	// Cap 0 new words — the reviewed card must still surface, no new ones do.
	due, err := s.GetDueCardsGated(50, 0)
	if err != nil {
		t.Fatalf("gated: %v", err)
	}
	sawReview, sawNew := false, false
	for _, c := range due {
		if c.ID == r.ID {
			sawReview = true
		}
		if c.IsNew {
			sawNew = true
		}
	}
	if !sawReview {
		t.Error("reviewed card should appear even with new cap 0")
	}
	if sawNew {
		t.Error("no new cards should appear with cap 0")
	}
}
