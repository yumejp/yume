package srs

import (
	"slices"
	"testing"
	"time"
)

func TestFuriganaSwitchesOnAtEighthLapse(t *testing.T) {
	s := openTestStore(t)
	id := newSimCard(t, s, "供給")
	if err := s.SetWordStatus("供給", 2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.q().Exec(`UPDATE cards SET state = 2, stability = 5, reps = 20, lapses = 6, last_review = ?, due = ? WHERE id = ?`,
		time.Now().Add(-6*24*time.Hour).Unix(), time.Now().Add(-time.Hour).Unix(), id); err != nil {
		t.Fatal(err)
	}
	res, err := s.reviewCardAt(id, RatingAgain, time.Now())
	if err != nil || res.Furigana {
		t.Fatalf("7th lapse: furigana %v, %v; want off", res.Furigana, err)
	}
	if _, err := s.q().Exec(`UPDATE cards SET state = 2, due = ? WHERE id = ?`, time.Now().Unix(), id); err != nil {
		t.Fatal(err)
	}
	if err := s.SetWordStatus("供給", 2); err != nil {
		t.Fatal(err)
	}
	res, err = s.reviewCardAt(id, RatingAgain, time.Now().Add(time.Minute))
	if err != nil || !res.Furigana || !res.Demoted {
		t.Fatalf("8th lapse: %+v, %v; want switched and demoted", res, err)
	}
	if got := wordStatus(t, s, "供給"); got != 1 {
		t.Errorf("status %d, want Learning", got)
	}
	if _, err := s.q().Exec(`UPDATE cards SET due = ? WHERE id = ?`, time.Now().Unix()-60, id); err != nil {
		t.Fatal(err)
	}
	cards, err := s.GetDueCards(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 || !slices.Contains(cards[0].SystemTags, TagFurigana) || cards[0].TryKanji {
		t.Fatalf("due card %+v: want furigana tag, no try-kanji yet", cards)
	}
}

func TestFuriganaWordsStayLearning(t *testing.T) {
	s := openTestStore(t)
	id := newSimCard(t, s, "鍵")
	if err := s.SetWordStatus("鍵", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TagCard(id, TagFurigana, false); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 15; i++ {
		cs := readCardState(t, s, id)
		if res, err := s.reviewCardAt(id, RatingGood, timeUnixHour(cs.due)); err != nil || res.Promoted {
			t.Fatalf("review %d: promoted %v, %v", i, res.Promoted, err)
		}
	}
	if got := wordStatus(t, s, "鍵"); got != 1 {
		t.Fatalf("status %d, want Learning", got)
	}
	// Held for a month: offered back without furigana.
	if _, err := s.q().Exec(`UPDATE cards SET due = ? WHERE id = ?`, time.Now().Unix()-60, id); err != nil {
		t.Fatal(err)
	}
	cards, err := s.GetDueCards(10)
	if err != nil || len(cards) != 1 || !cards[0].TryKanji {
		t.Fatalf("due %+v, %v; want try_kanji", cards, err)
	}
	// Switching off lets it graduate again.
	if _, err := s.TagCard(id, TagFurigana, true); err != nil {
		t.Fatal(err)
	}
	cs := readCardState(t, s, id)
	if res, err := s.reviewCardAt(id, RatingGood, timeUnixHour(cs.due)); err != nil || !res.Promoted {
		t.Fatalf("after switching off: promoted %v, %v", res.Promoted, err)
	}
}

func TestFuriganaManualSwitchDemotesKnown(t *testing.T) {
	s := openTestStore(t)
	id := newSimCard(t, s, "技術")
	if err := s.SetWordStatus("技術", 2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TagCard(id, TagFurigana, false); err != nil {
		t.Fatal(err)
	}
	if got := wordStatus(t, s, "技術"); got != 1 {
		t.Fatalf("status %d, want Learning", got)
	}
	st, err := s.ComputeStats()
	if err != nil || st.Totals.BySound != 1 {
		t.Fatalf("by_sound %d, %v", st.Totals.BySound, err)
	}
}
