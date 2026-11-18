package srs

import (
	"encoding/json"
	"fmt"
	"os"
)

// Tuning holds every scheduler knob layered on stock FSRS, loaded from
// srs-tuning.json ($YUME_SRS_TUNING). Missing file = defaults; malformed = error.
//
//   - weights: the 19 FSRS-5 parameters fitted to this deck's review log
//     (e.g. with fsrs-rs-python 0.2.x); empty = the library defaults, which
//     overestimated stability on the first deck (34% Again at a 93% target).
//   - request_retention: 0.90 is the FSRS default; 0.93 costs ~1.5× the
//     reviews on mature cards.
//   - min_non_again_interval_hours: non-Again ratings floor at one day, so
//     FSRS's sub-day learning steps never re-due a card within the session.
//   - demote_after_lapses / demote_lapse_window_days: a Known word drops to
//     Learning on its Nth lapse from Review within the window; 1 demotes on
//     every miss.
//   - mature_lapse_floor / mature_lapse_threshold_days: a lapse on a card
//     scheduled >= threshold keeps floor × its previous stability. Off by
//     default: floored recovery reviews failed ~half the time, matching
//     SuperMemo's finding that preserving post-lapse interval is harmful.
type Tuning struct {
	Weights                  []float64 `json:"weights,omitempty"`
	RequestRetention         float64   `json:"request_retention"`
	MinNonAgainIntervalHours float64   `json:"min_non_again_interval_hours"`
	DemoteAfterLapses        int       `json:"demote_after_lapses"`
	DemoteLapseWindowDays    float64   `json:"demote_lapse_window_days"`
	MatureLapseFloor         float64   `json:"mature_lapse_floor"`
	MatureLapseThresholdDays float64   `json:"mature_lapse_threshold_days"`
}

// DefaultTuning returns the deployment defaults.
func DefaultTuning() Tuning {
	return Tuning{
		RequestRetention:         0.90,
		MinNonAgainIntervalHours: 24,
		DemoteAfterLapses:        2,
		DemoteLapseWindowDays:    30,
		MatureLapseFloor:         0,
		MatureLapseThresholdDays: 30,
	}
}

// LoadTuning reads path; missing → defaults, malformed or out of domain → error.
func LoadTuning(path string) (Tuning, error) {
	t := DefaultTuning()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return t, nil
	}
	if err != nil {
		return t, fmt.Errorf("srs tuning: %w", err)
	}
	if err := json.Unmarshal(data, &t); err != nil {
		return t, fmt.Errorf("srs tuning %s: %w", path, err)
	}
	if n := len(t.Weights); n != 0 && n != 19 {
		return t, fmt.Errorf("srs tuning: weights has %d values, want 19", n)
	}
	if t.RequestRetention < 0.7 || t.RequestRetention > 0.99 {
		return t, fmt.Errorf("srs tuning: request_retention %v outside [0.7, 0.99]", t.RequestRetention)
	}
	if t.MinNonAgainIntervalHours < 0 || t.MinNonAgainIntervalHours > 72 {
		return t, fmt.Errorf("srs tuning: min_non_again_interval_hours %v outside [0, 72]", t.MinNonAgainIntervalHours)
	}
	if t.DemoteAfterLapses < 1 {
		return t, fmt.Errorf("srs tuning: demote_after_lapses %d must be >= 1", t.DemoteAfterLapses)
	}
	if t.DemoteLapseWindowDays < 1 {
		return t, fmt.Errorf("srs tuning: demote_lapse_window_days %v must be >= 1", t.DemoteLapseWindowDays)
	}
	if t.MatureLapseFloor < 0 || t.MatureLapseFloor > 0.9 {
		return t, fmt.Errorf("srs tuning: mature_lapse_floor %v outside [0, 0.9]", t.MatureLapseFloor)
	}
	if t.MatureLapseThresholdDays < 1 {
		return t, fmt.Errorf("srs tuning: mature_lapse_threshold_days %v must be >= 1", t.MatureLapseThresholdDays)
	}
	return t, nil
}
