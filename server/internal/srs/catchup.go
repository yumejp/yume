package srs

import (
	"fmt"
	"strconv"
	"time"
)

// Catch-up mode: when more reviews are due than the learner's daily limit,
// a session asks for the limit and serves the most at-risk cards first; the
// rest wait for later days.

const (
	reviewLimitKey     = "review_limit"
	defaultReviewLimit = 200
	maxReviewLimit     = 9999
)

// CatchUp is today's share of a review backlog.
type CatchUp struct {
	Backlog        int `json:"backlog"`
	TargetToday    int `json:"target_today"`
	DoneToday      int `json:"done_today"`
	RemainingToday int `json:"remaining_today"`
	// DaysLeft estimates the days to clear the backlog at the limit, counting
	// reviews that come due meanwhile; -1 when the limit can't keep up.
	DaysLeft int `json:"days_left"`
}

// ReviewLimit returns the daily review limit; 0 means no limit.
func (s *Store) ReviewLimit() (int, error) {
	v, _, err := s.GetPref(reviewLimitKey)
	if err != nil || v == "" {
		return defaultReviewLimit, err
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultReviewLimit, nil
	}
	return n, nil
}

// SetReviewLimit stores the daily review limit; 0 turns it off.
func (s *Store) SetReviewLimit(n int) error {
	if n < 0 || n > maxReviewLimit {
		return clientErrf("limit must be 0 to %d", maxReviewLimit)
	}
	_, err := s.SetPref(reviewLimitKey, strconv.Itoa(n))
	return err
}

// CatchUpPlan returns today's catch-up share, or nil when the reviews due
// today fit within the limit. It is derived from the deck each time.
func (s *Store) CatchUpPlan(tzOffsetMin *int) (*CatchUp, error) {
	limit, err := s.ReviewLimit()
	if err != nil || limit == 0 {
		return nil, err
	}
	cut := s.dueCutAt(clientLocation(tzOffsetMin))
	var backlog, upcoming int
	if err := s.q().QueryRow(`
		SELECT COALESCE(SUM(`+dueSQL+`), 0), COALESCE(SUM(NOT `+dueSQL+` AND due <= ?), 0)
		FROM cards WHERE lang = ? AND state != 0`,
		cut.dayEnd, cut.now, cut.dayEnd, cut.now, cut.at.Add(7*24*time.Hour).Unix(), s.lang,
	).Scan(&backlog, &upcoming); err != nil {
		return nil, fmt.Errorf("count backlog: %w", err)
	}
	var done int
	if err := s.q().QueryRow(`
		SELECT COUNT(DISTINCT rl.card_id) FROM review_log rl JOIN cards c ON c.id = rl.card_id
		WHERE c.lang = ? AND rl.prev_state != 0 AND rl.reviewed_at >= ?`,
		s.lang, studyDayEnd(time.Now(), clientLocation(tzOffsetMin))-24*3600,
	).Scan(&done); err != nil {
		return nil, fmt.Errorf("count reviewed today: %w", err)
	}
	return planCatchUp(backlog, done, upcoming, limit), nil
}

// planCatchUp: backlog is due reviews now, done is reviews finished today,
// upcoming is reviews coming due over the next seven days.
func planCatchUp(backlog, done, upcoming, limit int) *CatchUp {
	start := backlog + done
	if limit <= 0 || backlog == 0 || start <= limit {
		return nil
	}
	remaining := limit - done
	if remaining < 0 {
		remaining = 0
	}
	if remaining > backlog {
		remaining = backlog
	}
	daily := (upcoming + 6) / 7
	days := -1
	if limit > daily {
		days = 1 + (start-limit+limit-daily-1)/(limit-daily)
	}
	return &CatchUp{
		Backlog:        backlog,
		TargetToday:    limit,
		DoneToday:      done,
		RemainingToday: remaining,
		DaysLeft:       days,
	}
}

// GetCatchUpCards returns due reviews, lowest recall first. FSRS recall falls
// with elapsed time over stability, so ordering by that ratio orders by
// recall. New cards are left out: catching up is reviews only.
func (s *Store) GetCatchUpCards(limit int, tzOffsetMin *int) ([]DueCard, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > maxDueCards {
		limit = maxDueCards
	}
	cut := s.dueCutAt(clientLocation(tzOffsetMin))
	cards, err := s.queryDueCards(dueCardSelect+`
		WHERE lang = ? AND state != 0 AND `+dueSQL+`
		ORDER BY (? - COALESCE(last_review, due)) / MAX(stability, 0.01) DESC, due ASC
		LIMIT ?`, s.lang, cut.dayEnd, cut.now, cut.now, limit)
	if err != nil {
		return nil, err
	}
	if err := s.attachSystemTags(cards); err != nil {
		return nil, err
	}
	if err := s.attachLastReviewIDs(cards); err != nil {
		return nil, err
	}
	if err := s.attachSharedMnemonics(cards); err != nil {
		return nil, err
	}
	return cards, nil
}
