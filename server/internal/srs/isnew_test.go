package srs

import "testing"

// GetDueCards must mark a never-reviewed card is_new (so the client teaches it
// on first exposure) and drop the flag once it's been reviewed.
func TestDueCardIsNewFlag(t *testing.T) {
	s := openTestStore(t)
	r, err := s.SaveCard(SaveCardReq{Lemma: "供給", Glosses: [][]string{{"supply"}}})
	if err != nil {
		t.Fatalf("SaveCard: %v", err)
	}
	// isNew reads the flag off the due list. A Good review pushes due into the
	// future (card leaves the queue), so before re-checking we force it due
	// again — we're testing the flag, not FSRS scheduling.
	isNew := func() bool {
		due, err := s.GetDueCards(50)
		if err != nil {
			t.Fatalf("GetDueCards: %v", err)
		}
		for _, c := range due {
			if c.ID == r.ID {
				return c.IsNew
			}
		}
		t.Fatalf("card %d not due (len=%d)", r.ID, len(due))
		return false
	}
	if !isNew() {
		t.Fatal("freshly saved card should be is_new")
	}
	if _, err := s.ReviewCard(r.ID, RatingGood); err != nil {
		t.Fatalf("ReviewCard: %v", err)
	}
	if _, err := s.q().Exec(`UPDATE cards SET due = 0 WHERE id = ?`, r.ID); err != nil {
		t.Fatalf("force due: %v", err)
	}
	if isNew() {
		t.Fatal("reviewed card should not be is_new")
	}
}

func TestDueCardCarriesMediaPaths(t *testing.T) {
	s := openTestStore(t)
	r, err := s.SaveCard(SaveCardReq{Lemma: "需要", Glosses: [][]string{{"demand"}}})
	if err != nil {
		t.Fatalf("SaveCard: %v", err)
	}
	if _, err := s.q().Exec(`UPDATE cards SET audio_file = 'a.webm', image_file = 'i.png' WHERE id = ?`, r.ID); err != nil {
		t.Fatalf("set media: %v", err)
	}
	due, err := s.GetDueCards(50)
	if err != nil {
		t.Fatalf("GetDueCards: %v", err)
	}
	if len(due) != 1 {
		t.Fatalf("due = %d, want 1", len(due))
	}
	if due[0].AudioPath == nil || *due[0].AudioPath != "a.webm" {
		t.Errorf("AudioPath = %v, want a.webm", due[0].AudioPath)
	}
	if due[0].ImagePath == nil || *due[0].ImagePath != "i.png" {
		t.Errorf("ImagePath = %v, want i.png", due[0].ImagePath)
	}
}
