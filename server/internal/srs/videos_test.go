package srs

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newVideoStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "cards.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func saveVideo(t *testing.T, s *Store, req VideoSnapshotReq) {
	t.Helper()
	if _, _, err := s.SaveVideoSnapshot(req); err != nil {
		t.Fatalf("SaveVideoSnapshot(%s): %v", req.VideoID, err)
	}
}

func listOne(t *testing.T, s *Store, lang string) VideoStats {
	t.Helper()
	videos, err := s.ListVideoSnapshots(lang)
	if err != nil {
		t.Fatal(err)
	}
	if len(videos) != 1 {
		t.Fatalf("listed %d videos, want 1", len(videos))
	}
	return videos[0]
}

func TestVideoSnapshotUpsert(t *testing.T) {
	s := newVideoStore(t)

	id, updated, err := s.SaveVideoSnapshot(VideoSnapshotReq{
		VideoID:   " site:abc123 ",
		URL:       "https://example.com/watch?v=abc123",
		Title:     "  日本語の動画  ",
		Platform:  "site",
		CueLemmas: [][]string{{"猫", "食べる"}, {"犬"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if id != "site:abc123" {
		t.Fatalf("video_id = %q, want trimmed", id)
	}
	if updated {
		t.Fatal("first insert reported updated=true")
	}

	v := listOne(t, s, "")
	if v.Title != "日本語の動画" || v.Lang != "ja" || v.Platform != "site" {
		t.Fatalf("unexpected row: %+v", v)
	}
	if v.CueCount != 2 {
		t.Fatalf("cue_count = %d, want 2", v.CueCount)
	}

	// Backdate created_at so preservation across the upsert is observable
	// (both saves land in the same second otherwise).
	past := time.Now().Add(-48 * time.Hour).Unix()
	if _, err := s.q().Exec(
		`UPDATE video_snapshots SET created_at = ?, updated_at = ? WHERE video_id = ?`,
		past, past, "site:abc123",
	); err != nil {
		t.Fatal(err)
	}

	// Re-save: refreshes url/title/cues/cue_count/updated_at, keeps created_at.
	_, updated, err = s.SaveVideoSnapshot(VideoSnapshotReq{
		VideoID:   "site:abc123",
		URL:       "https://example.com/watch?v=abc123&t=1",
		Title:     "新しいタイトル",
		Platform:  "site",
		CueLemmas: [][]string{{"猫"}, {"犬"}, {"鳥"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated {
		t.Fatal("upsert of existing video reported updated=false")
	}
	v = listOne(t, s, "ja")
	if v.Title != "新しいタイトル" || v.URL != "https://example.com/watch?v=abc123&t=1" || v.CueCount != 3 {
		t.Fatalf("update didn't refresh fields: %+v", v)
	}
	if v.CreatedAt != past {
		t.Fatalf("created_at = %d, want preserved %d", v.CreatedAt, past)
	}
	if v.UpdatedAt <= past {
		t.Fatalf("updated_at = %d, want refreshed past %d", v.UpdatedAt, past)
	}

	// Delete: true once, false on the second try.
	if ok, err := s.DeleteVideoSnapshot("site:abc123"); err != nil || !ok {
		t.Fatalf("delete: ok=%v err=%v", ok, err)
	}
	if ok, err := s.DeleteVideoSnapshot("site:abc123"); err != nil || ok {
		t.Fatalf("second delete: ok=%v err=%v, want false nil", ok, err)
	}
}

func TestVideoStatsMath(t *testing.T) {
	s := newVideoStore(t)

	// known1/known2 = Known, learn1 = Learning, ign1 = Ignored,
	// unk1/unk2 have no row = implicit unknown.
	for lemma, status := range map[string]int{
		"known1": 2, "known2": 2, "learn1": 1, "ign1": 3,
	} {
		if err := s.SetWordStatus(lemma, status); err != nil {
			t.Fatal(err)
		}
	}

	saveVideo(t, s, VideoSnapshotReq{
		VideoID:  "site:stats",
		URL:      "https://example.com/watch?v=stats",
		Title:    "stats",
		Platform: "site",
		CueLemmas: [][]string{
			{"known1", "unk1"},         // exactly one unknown → i+1
			{"known1", "unk1", "unk2"}, // two unknowns → NOT i+1
			{"known2", "ign1", "unk1"}, // ignored doesn't count → one unknown → i+1
			{"known1", "learn1"},       // zero unknowns → NOT i+1
			{"ign1"},                   // zero unknowns (ignored excluded) → NOT i+1
		},
	})

	v := listOne(t, s, "ja")
	// Unique non-ignored lemmas: known1 known2 learn1 unk1 unk2 = 5.
	if v.UniqueLemmas != 5 {
		t.Fatalf("unique_lemmas = %d, want 5 (ignored excluded)", v.UniqueLemmas)
	}
	if v.Known != 2 || v.Learning != 1 || v.Unknown != 2 {
		t.Fatalf("known/learning/unknown = %d/%d/%d, want 2/1/2", v.Known, v.Learning, v.Unknown)
	}
	if v.CoveragePct != 40.0 { // 2 / 5 * 100
		t.Fatalf("coverage_pct = %v, want 40.0", v.CoveragePct)
	}
	if v.I1Cues != 2 {
		t.Fatalf("i1_cues = %d, want 2", v.I1Cues)
	}
	if v.NewWords != 2 {
		t.Fatalf("new_words = %d, want 2 (= unknown)", v.NewWords)
	}
	if v.MaturingSoon != 0 {
		t.Fatalf("maturing_soon = %d, want 0 (no cards)", v.MaturingSoon)
	}

	// Marking a word Known moves the stats on the NEXT list — recomputed
	// fresh, never stored.
	if err := s.SetWordStatus("unk1", 2); err != nil {
		t.Fatal(err)
	}
	v = listOne(t, s, "ja")
	if v.Known != 3 || v.Unknown != 1 || v.CoveragePct != 60.0 {
		t.Fatalf("after learning unk1: known=%d unknown=%d coverage=%v, want 3/1/60.0", v.Known, v.Unknown, v.CoveragePct)
	}
	// Cue 2 (known1+unk1+unk2) now has exactly one unknown; cues 1 and 3
	// dropped to zero. i+1 count = 1.
	if v.I1Cues != 1 {
		t.Fatalf("i1_cues after status change = %d, want 1", v.I1Cues)
	}
}

func TestVideoCoverageRounding(t *testing.T) {
	s := newVideoStore(t)
	if err := s.SetWordStatus("known1", 2); err != nil {
		t.Fatal(err)
	}
	saveVideo(t, s, VideoSnapshotReq{
		VideoID:   "site:round",
		URL:       "https://example.com",
		Title:     "round",
		Platform:  "site",
		CueLemmas: [][]string{{"known1", "unkA", "unkB"}}, // 1/3 → 33.3
	})
	v := listOne(t, s, "")
	if v.CoveragePct != 33.3 {
		t.Fatalf("coverage_pct = %v, want 33.3 (one decimal)", v.CoveragePct)
	}
}

func TestVideoMaturingSoon(t *testing.T) {
	s := newVideoStore(t)
	now := time.Now().Unix()

	// Three Learning lemmas: one card due in 7 days (counts), one due in 30
	// days (doesn't), one with no card at all (doesn't).
	for _, lemma := range []string{"soon", "later", "cardless"} {
		if err := s.SetWordStatus(lemma, 1); err != nil {
			t.Fatal(err)
		}
	}
	for lemma, due := range map[string]int64{
		"soon":  now + 7*24*3600,
		"later": now + 30*24*3600,
	} {
		if _, err := s.SaveCard(SaveCardReq{Lemma: lemma}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.q().Exec(`UPDATE cards SET due = ? WHERE lemma = ?`, due, lemma); err != nil {
			t.Fatal(err)
		}
	}
	// A KNOWN lemma with a near due date must not count — MaturingSoon is a
	// Learning-only forecast.
	if err := s.SetWordStatus("alreadyknown", 2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveCard(SaveCardReq{Lemma: "alreadyknown"}); err != nil {
		t.Fatal(err)
	}

	saveVideo(t, s, VideoSnapshotReq{
		VideoID:   "site:maturing",
		URL:       "https://example.com",
		Title:     "maturing",
		Platform:  "site",
		CueLemmas: [][]string{{"soon", "later"}, {"cardless", "alreadyknown"}},
	})
	v := listOne(t, s, "ja")
	if v.Learning != 3 {
		t.Fatalf("learning = %d, want 3", v.Learning)
	}
	if v.MaturingSoon != 1 {
		t.Fatalf("maturing_soon = %d, want 1 (only the 7-day card)", v.MaturingSoon)
	}
}

func TestVideoSnapshotValidation(t *testing.T) {
	s := newVideoStore(t)
	base := VideoSnapshotReq{
		VideoID:   "site:v",
		URL:       "https://example.com",
		Title:     "ok",
		Platform:  "site",
		CueLemmas: [][]string{{"猫"}},
	}

	cases := []struct {
		name   string
		mutate func(*VideoSnapshotReq)
	}{
		{"empty video_id", func(r *VideoSnapshotReq) { r.VideoID = "  " }},
		{"empty url", func(r *VideoSnapshotReq) { r.URL = "" }},
		{"empty title", func(r *VideoSnapshotReq) { r.Title = " " }},
		{"title over 300 chars", func(r *VideoSnapshotReq) { r.Title = strings.Repeat("あ", 301) }},
		{"cue count over cap", func(r *VideoSnapshotReq) { r.CueLemmas = make([][]string, maxVideoCues+1) }},
		{"lemma over 100 chars", func(r *VideoSnapshotReq) {
			r.CueLemmas = [][]string{{strings.Repeat("あ", 101)}}
		}},
		{"total lemma count over cap", func(r *VideoSnapshotReq) {
			big := make([]string, 1000)
			for i := range big {
				big[i] = "x"
			}
			cues := make([][]string, maxVideoLemmas/1000+1)
			for i := range cues {
				cues[i] = big
			}
			r.CueLemmas = cues
		}},
	}
	for _, tc := range cases {
		req := base
		tc.mutate(&req)
		if _, _, err := s.SaveVideoSnapshot(req); err == nil {
			t.Errorf("%s: expected an error, got nil", tc.name)
		}
	}

	// A 300-rune title is exactly at the cap — accepted.
	req := base
	req.Title = strings.Repeat("あ", 300)
	if _, _, err := s.SaveVideoSnapshot(req); err != nil {
		t.Fatalf("300-char title should be accepted: %v", err)
	}
}

// vocabFixture builds the library used by the GetVideoVocab tests:
//   - statuses: learn1=Learning, known1=Known, ign1=Ignored;
//     unkA / alpha / zeta have no row = implicit unknown.
//   - ja videos: A (target), B, C — learn1 is in all three (reach 3),
//     unkA in A+B (reach 2), alpha/zeta only in A (reach 1).
//   - a zh video containing unkA, which must NOT count toward ja reach.
func vocabFixture(t *testing.T) *Store {
	t.Helper()
	s := newVideoStore(t)
	for lemma, status := range map[string]int{"learn1": 1, "known1": 2, "ign1": 3} {
		if err := s.SetWordStatus(lemma, status); err != nil {
			t.Fatal(err)
		}
	}
	saveVideo(t, s, VideoSnapshotReq{
		VideoID: "site:A", URL: "https://example.com/a", Title: "A", Platform: "site",
		CueLemmas: [][]string{
			{"unkA", "learn1", "known1"},
			{"unkA", "unkA", "ign1"}, // duplicated unkA — must count this cue once
			{"zeta"},
			{"alpha"},
		},
	})
	saveVideo(t, s, VideoSnapshotReq{
		VideoID: "site:B", URL: "https://example.com/b", Title: "B", Platform: "site",
		CueLemmas: [][]string{{"unkA", "learn1"}},
	})
	saveVideo(t, s, VideoSnapshotReq{
		VideoID: "site:C", URL: "https://example.com/c", Title: "C", Platform: "site",
		CueLemmas: [][]string{{"learn1"}},
	})
	saveVideo(t, s, VideoSnapshotReq{
		VideoID: "other:zh", URL: "https://example.com/zh", Title: "zh", Platform: "other",
		Lang: "zh", CueLemmas: [][]string{{"unkA", "learn1"}},
	})
	return s
}

func TestGetVideoVocabReachAndSort(t *testing.T) {
	s := vocabFixture(t)

	words, err := s.GetVideoVocab("site:A", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Default statuses {0,1}: known1 and ign1 are excluded. Sort is
	// LibraryVideos DESC → InVideoCues DESC → lemma ASC:
	//   learn1 (reach 3), unkA (reach 2), alpha (1,1), zeta (1,1).
	want := []VideoVocabWord{
		{Lemma: "learn1", Status: 1, InVideoCues: 1, LibraryVideos: 3},
		{Lemma: "unkA", Status: 0, InVideoCues: 2, LibraryVideos: 2},
		{Lemma: "alpha", Status: 0, InVideoCues: 1, LibraryVideos: 1},
		{Lemma: "zeta", Status: 0, InVideoCues: 1, LibraryVideos: 1},
	}
	if len(words) != len(want) {
		t.Fatalf("got %d words %+v, want %d", len(words), words, len(want))
	}
	for i, w := range want {
		if words[i] != w {
			t.Errorf("words[%d] = %+v, want %+v", i, words[i], w)
		}
	}
	// The zh snapshot contains unkA and learn1 but must not inflate ja reach —
	// pinned by the exact LibraryVideos values above (2 and 3, not 3 and 4).
}

func TestGetVideoVocabStatusFiltering(t *testing.T) {
	s := vocabFixture(t)

	// Explicit [2] → only the Known word.
	known, err := s.GetVideoVocab("site:A", []int{2})
	if err != nil {
		t.Fatal(err)
	}
	if len(known) != 1 || known[0].Lemma != "known1" || known[0].Status != 2 {
		t.Fatalf("statuses [2]: got %+v, want only known1", known)
	}

	// Ignored is excluded by default but reachable when explicitly requested.
	ign, err := s.GetVideoVocab("site:A", []int{3})
	if err != nil {
		t.Fatal(err)
	}
	if len(ign) != 1 || ign[0].Lemma != "ign1" || ign[0].Status != 3 {
		t.Fatalf("statuses [3]: got %+v, want only ign1", ign)
	}

	// Explicit [0] → only the absent-row (implicit unknown) lemmas.
	unk, err := s.GetVideoVocab("site:A", []int{0})
	if err != nil {
		t.Fatal(err)
	}
	if len(unk) != 3 {
		t.Fatalf("statuses [0]: got %+v, want unkA/alpha/zeta", unk)
	}
	for _, w := range unk {
		if w.Status != 0 {
			t.Errorf("statuses [0] returned %+v", w)
		}
	}
}

func TestGetVideoVocabMissingVideo(t *testing.T) {
	s := newVideoStore(t)
	if _, err := s.GetVideoVocab("site:nope", nil); err == nil || !strings.Contains(err.Error(), "video not found") {
		t.Fatalf("missing video: err = %v, want 'video not found'", err)
	}
	if _, err := s.VideoSnapshotLang("site:nope"); err == nil || !strings.Contains(err.Error(), "video not found") {
		t.Fatalf("missing video lang: err = %v, want 'video not found'", err)
	}
}

func TestGetVideoVocabCap(t *testing.T) {
	s := newVideoStore(t)
	cues := make([][]string, maxVideoVocabWords+50)
	for i := range cues {
		cues[i] = []string{fmt.Sprintf("w%04d", i)}
	}
	saveVideo(t, s, VideoSnapshotReq{
		VideoID: "site:big", URL: "https://example.com/big", Title: "big",
		Platform: "site", CueLemmas: cues,
	})
	words, err := s.GetVideoVocab("site:big", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(words) != maxVideoVocabWords {
		t.Fatalf("got %d words, want capped at %d", len(words), maxVideoVocabWords)
	}
}

func TestVideoSnapshotLang(t *testing.T) {
	s := vocabFixture(t)
	lang, err := s.VideoSnapshotLang("other:zh")
	if err != nil || lang != "zh" {
		t.Fatalf("lang = %q err = %v, want zh", lang, err)
	}
	lang, err = s.VideoSnapshotLang("site:A")
	if err != nil || lang != "ja" {
		t.Fatalf("lang = %q err = %v, want ja (default)", lang, err)
	}
}

func TestVideoLangFiltering(t *testing.T) {
	s := newVideoStore(t)
	saveVideo(t, s, VideoSnapshotReq{
		VideoID: "site:ja1", URL: "https://example.com/ja", Title: "ja video",
		Platform: "site", CueLemmas: [][]string{{"猫"}},
	})
	saveVideo(t, s, VideoSnapshotReq{
		VideoID: "other:zh1", URL: "https://example.com/zh", Title: "zh video",
		Platform: "other", Lang: "zh", CueLemmas: [][]string{{"貓"}},
	})

	ja, err := s.ListVideoSnapshots("") // "" defaults to ja
	if err != nil {
		t.Fatal(err)
	}
	if len(ja) != 1 || ja[0].VideoID != "site:ja1" {
		t.Fatalf("lang '' listed %+v, want only site:ja1", ja)
	}
	zh, err := s.ListVideoSnapshots("zh")
	if err != nil {
		t.Fatal(err)
	}
	if len(zh) != 1 || zh[0].VideoID != "other:zh1" || zh[0].Lang != "zh" {
		t.Fatalf("lang zh listed %+v, want only other:zh1", zh)
	}
}
