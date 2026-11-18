package srs

import (
	"testing"
	"time"
)

func TestParkAndUnpark(t *testing.T) {
	s := openTestStore(t)
	var ids []int64
	for i, lemma := range []string{"供給", "鍵", "技術"} {
		id := newSimCard(t, s, lemma)
		if _, err := s.q().Exec(`UPDATE cards SET state = 2, due = 1000, lapses = ? WHERE id = ?`, 10-i, id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	fresh := newSimCard(t, s, "新語")

	n, err := s.ParkCards(append(ids, fresh))
	if err != nil || n != 3 {
		t.Fatalf("parked %d, %v; want 3 (new cards stay)", n, err)
	}
	if again, _ := s.ParkCards(ids); again != 0 {
		t.Errorf("re-parking parked %d", again)
	}

	n, err = s.UnparkCards(2, 1)
	if err != nil || n != 2 {
		t.Fatalf("unparked %d, %v; want 2", n, err)
	}
	dueOf := func(id int64) (d int64) {
		s.q().QueryRow(`SELECT due FROM cards WHERE id = ?`, id).Scan(&d)
		return d
	}
	now := time.Now().Unix()
	// Fewest lapses first (技術, then 鍵), one per day; 供給 stays parked.
	if d := dueOf(ids[2]); d < now-5 || d > now+5 {
		t.Errorf("技術 due %d, want now", d)
	}
	if d := dueOf(ids[1]); d < now+86400-5 || d > now+86400+5 {
		t.Errorf("鍵 due %d, want tomorrow", d)
	}
	if d := dueOf(ids[0]); d != parkedDue {
		t.Errorf("供給 due %d, want still parked", d)
	}
}
