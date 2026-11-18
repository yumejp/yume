package srs

import (
	"testing"
	"time"
)

func intPtr(v int) *int { return &v }

// dayStart computes the daily boundary in the client's zone when an offset is
// supplied; nil/0 keep the server-local behavior, and absurd offsets clamp to
// ±16h instead of failing. `now` is constructed in UTC so the nil case is
// deterministic regardless of the machine running the tests.
func TestDayStart(t *testing.T) {
	// 20:00 UTC on July 2 — late enough that Japan (+9h) is already on July 3,
	// so a client offset flips which day the boundary lands on.
	now := time.Date(2026, 7, 2, 20, 0, 0, 0, time.UTC)
	utc := func(d, h int) int64 {
		return time.Date(2026, 7, d, h, 0, 0, 0, time.UTC).Unix()
	}
	cases := []struct {
		name string
		tz   *int
		want int64
	}{
		{"nil = server-local midnight", nil, utc(2, 0)},
		{"0 = server-local midnight (clients zero-omit)", intPtr(0), utc(2, 0)},
		// Japan is at 05:00 July 3; its midnight is 15:00 UTC July 2.
		{"+540 (Japan) flips the day", intPtr(540), utc(2, 15)},
		// UTC-7 is at 13:00 July 2; its midnight is 07:00 UTC.
		{"-420 (UTC-7)", intPtr(-420), utc(2, 7)},
		// Clamped to +16h → 12:00 July 3 there → midnight is 08:00 UTC July 2.
		{"huge positive clamps to +16h", intPtr(100000), utc(2, 8)},
		// Clamped to -16h → 04:00 July 2 there → midnight is 16:00 UTC July 2.
		{"huge negative clamps to -16h", intPtr(-100000), utc(2, 16)},
	}
	for _, tc := range cases {
		if got := dayStart(now, tc.tz); got != tc.want {
			t.Errorf("%s: dayStart = %s, want %s",
				tc.name, time.Unix(got, 0).UTC(), time.Unix(tc.want, 0).UTC())
		}
	}
}

// A first review at 14:00 UTC is "today" against the server-local (UTC)
// boundary but "yesterday" in Japan, whose day flipped at 15:00 UTC — so the
// client offset decides whether the new-card quota still counts it.
func TestNewCardsIntroducedTodayTZ(t *testing.T) {
	s := openTestStore(t)
	r, err := s.SaveCard(SaveCardReq{Lemma: "時差", Glosses: [][]string{{"time difference"}}})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := s.ReviewCard(r.ID, RatingGood); err != nil {
		t.Fatalf("review: %v", err)
	}
	// Pin the review to a known instant so the test doesn't depend on wall time.
	reviewedAt := time.Date(2026, 7, 2, 14, 0, 0, 0, time.UTC).Unix()
	if _, err := s.q().Exec(`UPDATE review_log SET reviewed_at = ?`, reviewedAt); err != nil {
		t.Fatalf("pin reviewed_at: %v", err)
	}
	now := time.Date(2026, 7, 2, 20, 0, 0, 0, time.UTC)

	n, err := s.newCardsIntroducedToday(now, nil)
	if err != nil {
		t.Fatalf("introduced (server-local): %v", err)
	}
	if n != 1 {
		t.Errorf("server-local boundary: introduced = %d, want 1", n)
	}
	n, err = s.newCardsIntroducedToday(now, intPtr(540))
	if err != nil {
		t.Fatalf("introduced (Japan): %v", err)
	}
	if n != 0 {
		t.Errorf("Japan boundary: introduced = %d, want 0 (review was yesterday there)", n)
	}
}

// The undo walk's "today" bound follows the client offset: a review still
// inside the server-local day but past midnight in the client's zone belongs
// to a finished sitting there and must not be undoable.
func TestUndoReviewTZBoundary(t *testing.T) {
	s := openTestStore(t)
	r, err := s.SaveCard(SaveCardReq{Lemma: "昨日", Glosses: [][]string{{"yesterday"}}})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := s.ReviewCard(r.ID, RatingGood); err != nil {
		t.Fatalf("review: %v", err)
	}
	reviewedAt := time.Date(2026, 7, 2, 14, 0, 0, 0, time.UTC).Unix()
	if _, err := s.q().Exec(`UPDATE review_log SET reviewed_at = ?`, reviewedAt); err != nil {
		t.Fatalf("pin reviewed_at: %v", err)
	}
	now := time.Date(2026, 7, 2, 20, 0, 0, 0, time.UTC)

	// In Japan the day rolled over at 15:00 UTC — nothing from "today" to undo.
	if _, _, err := s.undoReviewAt(now, intPtr(540)); err == nil {
		t.Fatal("undo crossed the client-zone day boundary")
	}
	// Server-local it is still July 2 — the same review undoes fine.
	cardID, _, err := s.undoReviewAt(now, nil)
	if err != nil {
		t.Fatalf("undo within server-local today: %v", err)
	}
	if cardID != r.ID {
		t.Errorf("undone card = %d, want %d", cardID, r.ID)
	}
}
