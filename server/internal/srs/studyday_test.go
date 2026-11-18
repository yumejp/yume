package srs

import (
	"testing"
	"time"
)

func TestStudyDayEnd(t *testing.T) {
	jst := time.FixedZone("JST", 9*3600)
	cases := []struct{ now, want string }{
		{"2026-10-20 11:30", "2026-10-21 04:00"},
		{"2026-10-20 03:59", "2026-10-20 04:00"},
		{"2026-10-20 04:00", "2026-10-21 04:00"},
		{"2026-10-20 23:50", "2026-10-21 04:00"},
	}
	for _, c := range cases {
		now, _ := time.ParseInLocation("2006-01-02 15:04", c.now, jst)
		want, _ := time.ParseInLocation("2006-01-02 15:04", c.want, jst)
		if got := studyDayEnd(now, jst); got != want.Unix() {
			t.Errorf("%s: got %s, want %s", c.now, time.Unix(got, 0).In(jst).Format("2006-01-02 15:04"), c.want)
		}
	}
}

func TestReviewsDueForTheWholeStudyDay(t *testing.T) {
	s := openTestStore(t)
	now := time.Now()
	jst := 9 * 60
	end := studyDayEnd(now, clientLocation(&jst))
	set := func(lemma string, state int, due int64) {
		id := newSimCard(t, s, lemma)
		if _, err := s.q().Exec(`UPDATE cards SET state = ?, due = ? WHERE id = ?`, state, due, id); err != nil {
			t.Fatal(err)
		}
	}
	set("今日中", 2, end-60)         // review later today: due now
	set("明日", 2, end+60)          // review after the cutoff: not yet
	set("学習中", 1, now.Unix()+600) // learning step in 10 min: exact time

	ds, err := s.DeckStatsTZ(&jst)
	if err != nil {
		t.Fatal(err)
	}
	if ds.DueReviews != 1 {
		t.Errorf("due reviews = %d, want 1 (only the card due later today)", ds.DueReviews)
	}
	cards, err := s.GetDueCardsGatedTZ(20, 0, &jst)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 || cards[0].Lemma != "今日中" {
		t.Errorf("due cards = %v, want [今日中]", cards)
	}
}
