package srs

import (
	"path/filepath"
	"testing"
)

func newVocabStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "cards.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// The count query and the page query must share the same card_per_lemma CTE,
// or a search filter referencing card content columns errors ("no such column:
// c.kanji"). Exercise every filter dimension against both queries.
func TestListVocabFilters(t *testing.T) {
	s := newVocabStore(t)

	// One card-backed lemma with full content...
	if _, err := s.SaveCard(SaveCardReq{
		Lemma:    "慣れる",
		Kanji:    []string{"慣れる"},
		Readings: []string{"なれる"},
		Glosses:  [][]string{{"to get used to"}},
		Sentence: "新しい生活に慣れてきた",
	}); err != nil {
		t.Fatal(err)
	}
	// ...one status-only lemma...
	if err := s.SetWordStatus("結局", 1); err != nil {
		t.Fatal(err)
	}
	// ...and a tag for the AND filter.
	if err := s.AddWordTag("慣れる", "n3"); err != nil {
		t.Fatal(err)
	}

	// No filter: both lemmas, correct total.
	all, err := s.ListVocab(VocabFilter{}, 50, 0)
	if err != nil {
		t.Fatalf("no filter: %v", err)
	}
	if all.Total != 2 || len(all.Rows) != 2 {
		t.Fatalf("no filter: total=%d rows=%d, want 2/2", all.Total, len(all.Rows))
	}

	// Search must not error, and must match across fields.
	for _, q := range []string{"慣れる", "なれる", "used to", "新しい生活"} {
		r, err := s.ListVocab(VocabFilter{Search: q}, 50, 0)
		if err != nil {
			t.Fatalf("search %q: %v", q, err)
		}
		if r.Total != 1 || len(r.Rows) != 1 || r.Rows[0].Lemma != "慣れる" {
			t.Fatalf("search %q: total=%d rows=%d, want the 慣れる row", q, r.Total, len(r.Rows))
		}
	}
	if r, err := s.ListVocab(VocabFilter{Search: "zzz-no-match"}, 50, 0); err != nil || r.Total != 0 {
		t.Fatalf("search miss: total=%d err=%v, want 0/nil", r.Total, err)
	}

	// Status / has_card / tags still work alongside.
	one := 1
	if r, err := s.ListVocab(VocabFilter{Status: &one}, 50, 0); err != nil || r.Total != 1 || r.Rows[0].Lemma != "結局" {
		t.Fatalf("status filter: %+v err=%v", r, err)
	}
	hasCard := true
	if r, err := s.ListVocab(VocabFilter{HasCard: &hasCard, Search: "慣"}, 50, 0); err != nil || r.Total != 1 {
		t.Fatalf("has_card + search: %+v err=%v", r, err)
	}
	if r, err := s.ListVocab(VocabFilter{Tags: []string{"n3"}, Search: "なれる"}, 50, 0); err != nil || r.Total != 1 {
		t.Fatalf("tags + search: %+v err=%v", r, err)
	}
}

func TestListVocabNewFilter(t *testing.T) {
	s := openTestStore(t)
	a, err := s.SaveCard(SaveCardReq{Lemma: "新しい", Glosses: [][]string{{"new"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveCard(SaveCardReq{Lemma: "古い", Glosses: [][]string{{"old"}}}); err != nil {
		t.Fatal(err)
	}
	yes := true
	got, err := s.ListVocab(VocabFilter{New: &yes}, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 2 {
		t.Fatalf("both unreviewed cards should be new, got %d", got.Total)
	}
	if _, err := s.ReviewCardGuarded(a.ID, 3, "t-1", ""); err != nil {
		t.Fatal(err)
	}
	got, _ = s.ListVocab(VocabFilter{New: &yes}, 50, 0)
	if got.Total != 1 || got.Rows[0].Lemma != "古い" {
		t.Errorf("after one review, new = %+v", got.Rows)
	}
}
