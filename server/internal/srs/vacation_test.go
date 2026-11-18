package srs

import (
	"testing"
	"time"
)

func TestVacationShiftsDueByTimeAway(t *testing.T) {
	s := openTestStore(t)
	review := newSimCard(t, s, "復習")
	if _, err := s.q().Exec(`UPDATE cards SET state = 2, due = 5000 WHERE id = ?`, review); err != nil {
		t.Fatal(err)
	}
	reviewedAway := newSimCard(t, s, "休暇中")
	if _, err := s.q().Exec(`UPDATE cards SET state = 2, due = 9000, last_review = 4000 WHERE id = ?`, reviewedAway); err != nil {
		t.Fatal(err)
	}
	fresh := newSimCard(t, s, "新語")
	dueOf := func(id int64) (d int64) {
		s.q().QueryRow(`SELECT due FROM cards WHERE id = ?`, id).Scan(&d)
		return d
	}
	freshDue := dueOf(fresh)

	if since, _, err := s.setVacationAt(true, 10_000); err != nil || since != 10_000 {
		t.Fatalf("start = %d, %v", since, err)
	}
	if since, _, _ := s.setVacationAt(true, 20_000); since != 10_000 {
		t.Fatalf("second start moved the start to %d", since)
	}
	if got, _ := s.VacationSince(); got != 10_000 {
		t.Fatalf("VacationSince = %d", got)
	}

	// Reviewed during the break: its schedule is already current.
	if _, err := s.q().Exec(`UPDATE cards SET last_review = 96400, due = 442000 WHERE id = ?`, reviewedAway); err != nil {
		t.Fatal(err)
	}
	_, shifted, err := s.setVacationAt(false, 10_000+3*86400)
	if err != nil || shifted != 1 {
		t.Fatalf("end: shifted %d, %v; want 1", shifted, err)
	}
	if got := dueOf(review); got != 5000+3*86400 {
		t.Errorf("review due = %d, want %d", got, 5000+3*86400)
	}
	if got := dueOf(reviewedAway); got != 10_000+5*86400 {
		t.Errorf("card reviewed during vacation moved to %d", got)
	}
	if got := dueOf(fresh); got != freshDue {
		t.Errorf("new card moved to %d", got)
	}
	if got, _ := s.VacationSince(); got != 0 {
		t.Errorf("still on vacation since %d", got)
	}
	if _, shifted, _ := s.setVacationAt(false, 10_000+5*86400); shifted != 0 {
		t.Errorf("second end shifted %d cards again", shifted)
	}
}

func TestVacationStopsCardsComingDue(t *testing.T) {
	s := openTestStore(t)
	now := time.Now()
	setDue := func(lemma string, due time.Time) int64 {
		id := newSimCard(t, s, lemma)
		if _, err := s.q().Exec(`UPDATE cards SET state = 2, due = ?, last_review = ? WHERE id = ?`, due.Unix(), due.Add(-72*time.Hour).Unix(), id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	left := setDue("残り", now.Add(-96*time.Hour)) // due before the break: still waiting
	setDue("休暇中", now.Add(-24*time.Hour))        // came due during the break: frozen out
	if _, _, err := s.setVacationAt(true, now.Add(-72*time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}

	ds, err := s.DeckStatsTZ(nil)
	if err != nil {
		t.Fatal(err)
	}
	if ds.DueReviews != 1 {
		t.Errorf("due on vacation = %d, want 1 (only the card left behind)", ds.DueReviews)
	}
	cards, err := s.GetDueCardsGatedTZ(20, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 || cards[0].ID != left {
		t.Errorf("review anyway served %d cards, want just the one left behind", len(cards))
	}
}
