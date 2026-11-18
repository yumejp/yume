package srs

import (
	"fmt"
	"time"
)

// Parking takes cards out of review without losing their history: a parked
// card is due in 2100 and tagged "parked" so the learner can see which.
var parkedDue = time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC).Unix()

// ParkCards parks the given scheduled cards and returns how many it parked.
func (s *Store) ParkCards(ids []int64) (int, error) {
	n := 0
	for _, id := range ids {
		res, err := s.q().Exec(`UPDATE cards SET due = ? WHERE id = ? AND lang = ? AND state != 0 AND due < ?`, parkedDue, id, s.lang, parkedDue)
		if err != nil {
			return n, fmt.Errorf("park card %d: %w", id, err)
		}
		if k, _ := res.RowsAffected(); k == 0 {
			continue
		}
		if _, err := s.TagCard(id, TagParked, false); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// UnparkCards returns up to limit parked cards, fewest lapses first, perDay
// of them due each day from today, and returns how many it brought back.
func (s *Store) UnparkCards(limit, perDay int) (int, error) {
	if limit <= 0 || perDay <= 0 {
		return 0, clientErrf("limit and per_day must be positive")
	}
	rows, err := s.q().Query(`SELECT id FROM cards WHERE lang = ? AND due >= ? ORDER BY lapses, id LIMIT ?`, s.lang, parkedDue, limit)
	if err != nil {
		return 0, fmt.Errorf("list parked: %w", err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	now := time.Now().Unix()
	for i, id := range ids {
		if _, err := s.q().Exec(`UPDATE cards SET due = ? WHERE id = ?`, now+int64(i/perDay)*86400, id); err != nil {
			return i, fmt.Errorf("unpark card %d: %w", id, err)
		}
		if _, err := s.TagCard(id, TagParked, true); err != nil {
			return i, err
		}
	}
	return len(ids), nil
}
