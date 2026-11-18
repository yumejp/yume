package srs

import (
	"path/filepath"
	"testing"
)

func newWatchStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "cards.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestWatchWordsLifecycle(t *testing.T) {
	s := newWatchStore(t)

	n, err := s.AddWatchWords([]WatchWord{
		{Lemma: "慣れる", Note: "you keep missing this in て-form"},
		{Lemma: "  "},           // blank → skipped
		{Lemma: "、"},            // junk lemma → skipped
		{Lemma: "結局", Note: ""}, // no note is fine
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("added %d, want 2", n)
	}

	// Upsert refreshes the note, doesn't duplicate.
	if _, err := s.AddWatchWords([]WatchWord{{Lemma: "慣れる", Note: "updated"}}, "test"); err != nil {
		t.Fatal(err)
	}
	words, err := s.ListWatchWords()
	if err != nil {
		t.Fatal(err)
	}
	if len(words) != 2 {
		t.Fatalf("listed %d, want 2", len(words))
	}
	var nareru *WatchWord
	for i := range words {
		if words[i].Lemma == "慣れる" {
			nareru = &words[i]
		}
	}
	if nareru == nil || nareru.Note != "updated" {
		t.Fatalf("upsert didn't refresh note: %+v", words)
	}
	if nareru.Status != 0 {
		t.Fatalf("expected implicit-unknown status, got %d", nareru.Status)
	}

	// Status joins through.
	if err := s.SetWordStatus("結局", 1); err != nil {
		t.Fatal(err)
	}
	words, _ = s.ListWatchWords()
	for _, w := range words {
		if w.Lemma == "結局" && w.Status != 1 {
			t.Fatalf("結局 status = %d, want 1", w.Status)
		}
	}

	// Marking Known retires the suggestion automatically.
	if err := s.SetWordStatus("慣れる", 2); err != nil {
		t.Fatal(err)
	}
	words, _ = s.ListWatchWords()
	if len(words) != 1 || words[0].Lemma != "結局" {
		t.Fatalf("Known should retire the watch word; got %+v", words)
	}

	// Manual remove + clear.
	ok, err := s.RemoveWatchWord("結局")
	if err != nil || !ok {
		t.Fatalf("remove: ok=%v err=%v", ok, err)
	}
	if n, _ := s.ClearWatchWords(); n != 0 {
		t.Fatalf("clear on empty = %d, want 0", n)
	}
}
