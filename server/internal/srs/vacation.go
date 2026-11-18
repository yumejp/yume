package srs

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// Vacation mode stops the review clock: ending it moves every scheduled card's
// due date forward by the time away, so a break adds nothing to the pile.
// Cards reviewed during the break already have a fresh schedule and stay put.
const vacationKey = "vacation_since"

// VacationSince returns when vacation mode started, or 0 when it is off.
func (s *Store) VacationSince() (int64, error) {
	v, _, err := s.GetPref(vacationKey)
	if err != nil || v == "" {
		return 0, err
	}
	since, _ := strconv.ParseInt(v, 10, 64)
	return since, nil
}

// SetVacation turns vacation mode on or off and returns the start time (0 when
// off) and how many cards were moved. Repeating either call changes nothing.
func (s *Store) SetVacation(on bool) (since, shifted int64, err error) {
	return s.setVacationAt(on, time.Now().Unix())
}

func (s *Store) setVacationAt(on bool, now int64) (since, shifted int64, err error) {
	tx, err := s.q().Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	var v string
	if err := tx.QueryRow(`SELECT value FROM user_prefs WHERE key = ?`, vacationKey).Scan(&v); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, 0, fmt.Errorf("read vacation: %w", err)
	}
	started, _ := strconv.ParseInt(v, 10, 64)
	switch {
	case on && started > 0:
		return started, 0, nil
	case on:
		_, err = tx.Exec(`INSERT INTO user_prefs (key, value, updated_at) VALUES (?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
			vacationKey, strconv.FormatInt(now, 10), now)
		since = now
	case started > 0:
		var res sql.Result
		if res, err = tx.Exec(`UPDATE cards SET due = due + ? WHERE lang = ? AND state != 0
			AND (last_review IS NULL OR last_review < ?)`, now-started, s.lang, started); err == nil {
			shifted, _ = res.RowsAffected()
			_, err = tx.Exec(`DELETE FROM user_prefs WHERE key = ?`, vacationKey)
		}
	}
	if err != nil {
		return 0, 0, fmt.Errorf("set vacation: %w", err)
	}
	return since, shifted, tx.Commit()
}
