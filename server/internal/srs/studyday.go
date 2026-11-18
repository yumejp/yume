package srs

import "time"

// A study day runs from 4:00 to 4:00 in the learner's zone. A review card is
// due for the whole study day it falls in, so a day's reviews arrive together
// at 4:00 instead of trickling in at the hour of the last review. Learning
// steps (Again, first sightings) keep exact times.
const studyDayStartHour = 4

// studyDayEnd returns the next 4:00 in loc after now (unix seconds).
func studyDayEnd(now time.Time, loc *time.Location) int64 {
	t := now.In(loc).Add(-studyDayStartHour * time.Hour)
	y, m, d := t.Date()
	return time.Date(y, m, d+1, studyDayStartHour, 0, 0, 0, loc).Unix()
}

// dueSQL matches cards due at a dueCut; bind (cut.dayEnd, cut.now).
const dueSQL = `((state = 2 AND due < ?) OR (state != 2 AND due <= ?))`

// dueCut is the moment due-ness is judged at. Every count and fetch takes
// one from dueCutAt so they agree: a new rule (vacation, the study day)
// changes here, not in each query.
type dueCut struct {
	at     time.Time
	now    int64 // at, in unix seconds
	dayEnd int64 // the end of at's study day
}

// dueCutAt judges at the real time, or at the start of vacation while it is
// on: the clock stops, nothing new comes due, and "review anyway" works
// through only the pile left behind.
func (s *Store) dueCutAt(loc *time.Location) dueCut {
	at := time.Now()
	if since, err := s.VacationSince(); err == nil && since > 0 {
		at = time.Unix(since, 0)
	}
	return dueCut{at: at, now: at.Unix(), dayEnd: studyDayEnd(at, loc)}
}
