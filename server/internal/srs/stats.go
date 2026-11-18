package srs

import (
	"fmt"
	"sort"
	"time"
)

// Stats backs the progress dashboard, computed from word_status, cards and
// review_log. Days are bucketed in the client's zone when the request carries
// tz_offset_minutes, else server-local.

// Stats is the full dashboard payload.
type Stats struct {
	Totals       StatTotals        `json:"totals"`
	Streak       StatStreak        `json:"streak"`
	ThisWeek     StatThisWeek      `json:"this_week"`
	Heatmap      []DayCount        `json:"heatmap"`      // last ~120 local days with any activity
	MiningCurve  []CumulativePoint `json:"mining_curve"` // cumulative cards over last 90 days
	DueForecast  []DayCount        `json:"due_forecast"` // reviews over the next 14 days
	TotalReviews int               `json:"total_reviews"`
	// ReviewsToday counts since the learner's midnight; this_week.reviews is
	// rolling-7-day and the heatmap mixes in mining/status activity.
	ReviewsToday int           `json:"reviews_today"`
	Milestone    StatMilestone `json:"milestone"`
}

// StatTotals: DueNow and the forecast count reviews only. Never-seen cards
// are introduced on request, not scheduled, so they are NewWaiting instead;
// counting them as due made a fresh import read "2,001 due today".
type StatTotals struct {
	Known      int `json:"known"`
	Learning   int `json:"learning"`
	Ignored    int `json:"ignored"`
	Cards      int `json:"cards"`
	DueNow     int `json:"due_now"`
	NewWaiting int `json:"new_waiting"`
	// BySound: Learning words on furigana cards, known by sound only.
	BySound int `json:"by_sound"`
}

type StatStreak struct {
	Current int `json:"current"`
	Longest int `json:"longest"`
}

type StatThisWeek struct {
	KnownAdded int `json:"known_added"`
	Reviews    int `json:"reviews"`
	Mined      int `json:"mined"`
}

// DayCount is a (local date, count) pair. Date is "YYYY-MM-DD".
type DayCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// CumulativePoint is a running total at a given date.
type CumulativePoint struct {
	Date       string `json:"date"`
	Cumulative int    `json:"cumulative"`
}

// StatMilestone is the next "known words" goalpost.
type StatMilestone struct {
	Metric string `json:"metric"` // always "known" for now
	Target int    `json:"target"`
	Have   int    `json:"have"`
}

// clientLocation resolves a tz offset (minutes east of UTC, clamped like
// dayStart) to a fixed zone; nil keeps server-local. An explicit 0 is a real
// UTC zone, never "absent".
func clientLocation(tzOffsetMin *int) *time.Location {
	if tzOffsetMin == nil {
		return time.Local
	}
	off := *tzOffsetMin
	if off > maxTZOffsetMin {
		off = maxTZOffsetMin
	} else if off < -maxTZOffsetMin {
		off = -maxTZOffsetMin
	}
	return time.FixedZone("client", off*60)
}

// dateExpr returns a SQLite expression bucketing unix-seconds column col into
// a YYYY-MM-DD string in loc. Known limitation: the client's CURRENT offset
// applies to all timestamps, so DST transition days can mis-bucket an hour;
// fixing it needs an IANA zone name on the wire.
func dateExpr(col string, loc *time.Location) string {
	if loc == time.Local {
		return fmt.Sprintf("date(%s, 'unixepoch', 'localtime')", col)
	}
	_, offSec := time.Now().In(loc).Zone()
	return fmt.Sprintf("date(%s + %d, 'unixepoch')", col, offSec)
}

// ComputeStats is ComputeStatsTZ with server-local day buckets.
func (s *Store) ComputeStats() (Stats, error) {
	return s.ComputeStatsTZ(nil)
}

// ComputeStatsTZ assembles the dashboard with days bucketed in the client's
// zone (see dayStart).
func (s *Store) ComputeStatsTZ(tzOffsetMin *int) (Stats, error) {
	loc := clientLocation(tzOffsetMin)
	var out Stats

	// --- Totals -----------------------------------------------------------
	counts, err := s.WordStatusCounts()
	if err != nil {
		return out, err
	}
	out.Totals.Known = counts.Known
	out.Totals.Learning = counts.Learning
	out.Totals.Ignored = counts.Ignored

	if err := s.q().QueryRow(`SELECT COUNT(*) FROM cards WHERE lang = ?`, s.lang).Scan(&out.Totals.Cards); err != nil {
		return out, fmt.Errorf("count cards: %w", err)
	}
	if err := s.q().QueryRow(`SELECT COUNT(*) FROM word_tags WHERE lang = ? AND tag = ?`, s.lang, TagFurigana).Scan(&out.Totals.BySound); err != nil {
		return out, fmt.Errorf("count furigana: %w", err)
	}
	now := time.Now()
	cut := s.dueCutAt(loc)
	if err := s.q().QueryRow(`
		SELECT COALESCE(SUM(state != 0 AND `+dueSQL+`), 0), COALESCE(SUM(state = 0), 0)
		FROM cards WHERE lang = ?`, cut.dayEnd, cut.now, s.lang).
		Scan(&out.Totals.DueNow, &out.Totals.NewWaiting); err != nil {
		return out, fmt.Errorf("count due: %w", err)
	}

	// --- Total reviews ----------------------------------------------------
	if err := s.q().QueryRow(`SELECT COUNT(*) FROM review_log`).Scan(&out.TotalReviews); err != nil {
		return out, fmt.Errorf("count reviews: %w", err)
	}

	// --- This week (rolling 7 days) --------------------------------------
	weekAgo := now.AddDate(0, 0, -7).Unix()
	if err := s.q().QueryRow(
		`SELECT COUNT(*) FROM word_status WHERE lang = ? AND status = 2 AND updated_at >= ?`, s.lang, weekAgo,
	).Scan(&out.ThisWeek.KnownAdded); err != nil {
		return out, fmt.Errorf("week known: %w", err)
	}
	if err := s.q().QueryRow(
		`SELECT COUNT(*) FROM review_log WHERE reviewed_at >= ?`, weekAgo,
	).Scan(&out.ThisWeek.Reviews); err != nil {
		return out, fmt.Errorf("week reviews: %w", err)
	}
	if err := s.q().QueryRow(
		`SELECT COUNT(*) FROM cards WHERE created_at >= ?`, weekAgo,
	).Scan(&out.ThisWeek.Mined); err != nil {
		return out, fmt.Errorf("week mined: %w", err)
	}

	// --- Reviews today (learner's calendar day, not rolling) --------------
	if err := s.q().QueryRow(
		fmt.Sprintf(`SELECT COUNT(*) FROM review_log WHERE %s = ?`, dateExpr("reviewed_at", loc)),
		now.In(loc).Format("2006-01-02"),
	).Scan(&out.ReviewsToday); err != nil {
		return out, fmt.Errorf("reviews today: %w", err)
	}

	// --- Activity heatmap + streak ---------------------------------------
	// Activity = any review, card mined, or status change (not reviews-only).
	activity, err := s.activityByDay(loc)
	if err != nil {
		return out, err
	}
	out.Heatmap = lastNDays(activity, 120)
	out.Streak.Current, out.Streak.Longest = computeStreaks(activity, now, loc)

	// --- Mining curve (cumulative cards, last 90 days) -------------------
	out.MiningCurve, err = s.miningCurve(now, 90, loc)
	if err != nil {
		return out, err
	}

	// --- Due forecast (next 14 days) -------------------------------------
	out.DueForecast, err = s.dueForecast(cut, 14, loc)
	if err != nil {
		return out, err
	}

	// --- Next milestone --------------------------------------------------
	out.Milestone = StatMilestone{
		Metric: "known",
		Have:   out.Totals.Known,
		Target: nextMilestone(out.Totals.Known),
	}

	return out, nil
}

func (s *Store) activityByDay(loc *time.Location) (map[string]int, error) {
	rows, err := s.q().Query(`
		SELECT ` + dateExpr("ts", loc) + ` AS d, COUNT(*) AS c
		FROM (
			SELECT reviewed_at AS ts FROM review_log
			UNION ALL
			SELECT created_at  AS ts FROM cards
			UNION ALL
			SELECT updated_at  AS ts FROM word_status
		)
		GROUP BY d
	`)
	if err != nil {
		return nil, fmt.Errorf("activity by day: %w", err)
	}
	defer rows.Close()
	out := make(map[string]int)
	for rows.Next() {
		var d string
		var c int
		if err := rows.Scan(&d, &c); err != nil {
			return nil, err
		}
		out[d] = c
	}
	return out, rows.Err()
}

// miningCurve returns cumulative cards per day over the last days days,
// emitting points only on days with mining (the frontend interpolates).
func (s *Store) miningCurve(now time.Time, days int, loc *time.Location) ([]CumulativePoint, error) {
	start := now.AddDate(0, 0, -days)
	var baseline int
	if err := s.q().QueryRow(
		`SELECT COUNT(*) FROM cards WHERE created_at < ?`, start.Unix(),
	).Scan(&baseline); err != nil {
		return nil, fmt.Errorf("mining baseline: %w", err)
	}
	rows, err := s.q().Query(`
		SELECT `+dateExpr("created_at", loc)+` AS d, COUNT(*) AS c
		FROM cards
		WHERE created_at >= ?
		GROUP BY d
		ORDER BY d
	`, start.Unix())
	if err != nil {
		return nil, fmt.Errorf("mining curve: %w", err)
	}
	defer rows.Close()
	var out []CumulativePoint
	running := baseline
	for rows.Next() {
		var d string
		var c int
		if err := rows.Scan(&d, &c); err != nil {
			return nil, err
		}
		running += c
		out = append(out, CumulativePoint{Date: d, Cumulative: running})
	}
	return out, rows.Err()
}

// dueForecast counts reviews due on each of the next days study days; reviews
// already due belong to Totals.DueNow and never-seen cards to Totals.NewWaiting.
func (s *Store) dueForecast(cut dueCut, days int, loc *time.Location) ([]DayCount, error) {
	end := cut.at.AddDate(0, 0, days)
	rows, err := s.q().Query(`
		SELECT `+dateExpr(fmt.Sprintf("(due - %d)", studyDayStartHour*3600), loc)+` AS d, COUNT(*) AS c
		FROM cards
		WHERE state != 0 AND NOT `+dueSQL+` AND due <= ?
		GROUP BY d
		ORDER BY d
	`, cut.dayEnd, cut.now, end.Unix())
	if err != nil {
		return nil, fmt.Errorf("due forecast: %w", err)
	}
	defer rows.Close()
	out := []DayCount{}
	for rows.Next() {
		var dc DayCount
		if err := rows.Scan(&dc.Date, &dc.Count); err != nil {
			return nil, err
		}
		out = append(out, dc)
	}
	return out, rows.Err()
}

// --- Pure helpers (no DB) -------------------------------------------------

// lastNDays returns the most recent n active days, sorted; zero days are
// not filled in (the frontend lays out the grid).
func lastNDays(activity map[string]int, n int) []DayCount {
	out := make([]DayCount, 0, len(activity))
	for d, c := range activity {
		out = append(out, DayCount{Date: d, Count: c})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	if len(out) > n {
		out = out[len(out)-n:]
	}
	return out
}

// computeStreaks returns (current, longest) runs of consecutive active days.
// A missing today still counts from yesterday. loc must match the zone the
// dates were bucketed in.
func computeStreaks(activity map[string]int, now time.Time, loc *time.Location) (current, longest int) {
	if len(activity) == 0 {
		return 0, 0
	}
	active := make(map[string]bool, len(activity))
	dates := make([]string, 0, len(activity))
	for d, c := range activity {
		if c > 0 {
			active[d] = true
			dates = append(dates, d)
		}
	}
	sort.Strings(dates)

	longest = 1
	run := 1
	for i := 1; i < len(dates); i++ {
		// AddDate, not a 24h duration: DST days are 23h/25h.
		prev, _ := time.ParseInLocation("2006-01-02", dates[i-1], loc)
		if prev.AddDate(0, 0, 1).Format("2006-01-02") == dates[i] {
			run++
		} else {
			run = 1
		}
		if run > longest {
			longest = run
		}
	}

	cursor := now.In(loc)
	if !active[cursor.Format("2006-01-02")] {
		cursor = cursor.AddDate(0, 0, -1)
	}
	for {
		d := cursor.Format("2006-01-02")
		if active[d] {
			current++
			cursor = cursor.AddDate(0, 0, -1)
		} else {
			break
		}
	}
	return current, longest
}

func nextMilestone(have int) int {
	ladder := []int{50, 100, 250, 500, 1000, 2000, 3000, 5000, 7500, 10000, 15000, 20000}
	for _, m := range ladder {
		if m > have {
			return m
		}
	}
	return ((have / 5000) + 1) * 5000
}
