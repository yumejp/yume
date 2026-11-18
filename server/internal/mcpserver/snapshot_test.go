package mcpserver

import (
	"path/filepath"
	"testing"

	"github.com/yumejp/yume/server/internal/srs"
)

// The tutor reads due_cards as a review backlog, so never-seen cards must be
// reported apart from it (new_waiting), not inside it.
func TestLearningSnapshotSeparatesNewCards(t *testing.T) {
	store, err := srs.Open(filepath.Join(t.TempDir(), "cards.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	var reviewed int64
	for i, w := range []string{"復習", "新一", "新二"} {
		r, err := store.SaveCard(srs.SaveCardReq{Lemma: w, Glosses: [][]string{{"g"}}})
		if err != nil {
			t.Fatalf("save %s: %v", w, err)
		}
		if i == 0 {
			reviewed = r.ID
		}
	}
	if _, err := store.ReviewCard(reviewed, srs.RatingGood); err != nil {
		t.Fatalf("review: %v", err)
	}

	out := learningSnapshot(store)
	if out.TotalCards != 3 || out.NewWaiting != 2 || out.NewIntroducedToday != 1 {
		t.Errorf("total/new_waiting/introduced = %d/%d/%d, want 3/2/1", out.TotalCards, out.NewWaiting, out.NewIntroducedToday)
	}
	// The reviewed card is scheduled ahead, so nothing is due, whatever the
	// two new cards' due dates say.
	if out.DueReviews != 0 || out.DueCards != 0 {
		t.Errorf("due_reviews/due_cards = %d/%d, want 0/0", out.DueReviews, out.DueCards)
	}
}
