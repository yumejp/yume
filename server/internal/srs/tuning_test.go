package srs

import (
	"os"
	"reflect"
	"path/filepath"
	"testing"
	"time"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v3"
)

func TestLoadTuningMissingFileDefaults(t *testing.T) {
	got, err := LoadTuning(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, DefaultTuning()) {
		t.Fatalf("missing file should yield defaults, got %+v", got)
	}
}

func TestLoadTuningOverridesAndValidates(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "srs-tuning.json")
	os.WriteFile(p, []byte(`{"request_retention":0.91,"min_non_again_interval_hours":12,"mature_lapse_floor":0.3,"mature_lapse_threshold_days":21}`), 0o644)
	got, err := LoadTuning(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.RequestRetention != 0.91 || got.MinNonAgainIntervalHours != 12 || got.MatureLapseFloor != 0.3 || got.MatureLapseThresholdDays != 21 {
		t.Fatalf("override not applied: %+v", got)
	}
	os.WriteFile(p, []byte(`{"request_retention":0.2}`), 0o644)
	if _, err := LoadTuning(p); err == nil {
		t.Fatal("out-of-domain retention must error")
	}
	os.WriteFile(p, []byte(`{"weights":[1,2,3]}`), 0o644)
	if _, err := LoadTuning(p); err == nil {
		t.Fatal("weights of the wrong length must error")
	}
	os.WriteFile(p, []byte(`{broken`), 0o644)
	if _, err := LoadTuning(p); err == nil {
		t.Fatal("malformed JSON must error")
	}
}

// The tuning file follows the data folder; the working directory's
// ./data copy that earlier builds read is used only when it is the one file.
func TestTuningPathInFollowsDataFolder(t *testing.T) {
	root := t.TempDir()
	cwd := t.TempDir()
	t.Chdir(cwd)
	want := filepath.Join(root, "srs-tuning.json")

	if got, legacy := TuningPathIn(root); got != want || legacy {
		t.Fatalf("no file anywhere: got %s legacy=%v, want %s", got, legacy, want)
	}
	os.MkdirAll("data", 0o755)
	os.WriteFile(filepath.Join("data", "srs-tuning.json"), []byte(`{}`), 0o644)
	if got, legacy := TuningPathIn(root); got != filepath.Join("data", "srs-tuning.json") || !legacy {
		t.Fatalf("only the old cwd file: got %s legacy=%v", got, legacy)
	}
	os.WriteFile(want, []byte(`{}`), 0o644)
	if got, legacy := TuningPathIn(root); got != want || legacy {
		t.Fatalf("data folder file must win: got %s legacy=%v", got, legacy)
	}
}

// A mature card's first lapse keeps floorFrac × its stability; a young card
// and non-Again ratings are untouched; the compounded second lapse decays.
func TestFloorMatureLapse(t *testing.T) {
	prev := fsrs.Card{State: fsrs.Review, ScheduledDays: 48, Stability: 50}
	crushed := fsrs.Card{State: fsrs.Relearning, Stability: 2}

	got := floorMatureLapse(crushed, prev, fsrs.Again, 0.22, 30)
	if want := 0.22 * 50; got.Stability != want {
		t.Fatalf("mature lapse floor: got %v want %v", got.Stability, want)
	}
	// Above-floor FSRS result is kept as-is.
	healthy := fsrs.Card{State: fsrs.Relearning, Stability: 20}
	if got := floorMatureLapse(healthy, prev, fsrs.Again, 0.22, 30); got.Stability != 20 {
		t.Fatalf("floor must not lower stability: %v", got.Stability)
	}
	// Young card: untouched.
	young := fsrs.Card{State: fsrs.Review, ScheduledDays: 10, Stability: 8}
	if got := floorMatureLapse(crushed, young, fsrs.Again, 0.22, 30); got.Stability != 2 {
		t.Fatalf("young card must not floor: %v", got.Stability)
	}
	// Non-Again: untouched.
	if got := floorMatureLapse(crushed, prev, fsrs.Good, 0.22, 30); got.Stability != 2 {
		t.Fatalf("non-Again must not floor: %v", got.Stability)
	}
	// Second consecutive lapse: prev now carries the floored (small)
	// stability and a short interval, so the floor no longer rescues it.
	relapsed := fsrs.Card{State: fsrs.Review, ScheduledDays: 11, Stability: 11}
	if got := floorMatureLapse(crushed, relapsed, fsrs.Again, 0.22, 30); got.Stability != 2 {
		t.Fatalf("second lapse below threshold must not floor: %v", got.Stability)
	}
}

// End-to-end through the Store: a lapse on a mature card must schedule its
// recovery Good at a real fraction of the old interval, not days.
func TestMatureLapseKeepsCredit(t *testing.T) {
	s := openTestStore(t)
	s.tune.MatureLapseFloor = 0.22 // off by default
	id := newSimCard(t, s, "熟語")
	// Grow the card mature with on-time Goods.
	var cs cardState
	for i := 0; i < 8; i++ {
		cs, _ = reviewWhenDue(t, s, id, RatingGood)
	}
	before := cs.scheduledDays
	if before < 30 {
		t.Skipf("card not mature after growth: %v days", before)
	}
	// Lapse it on time, then recover with Good a day later.
	_, lapsedAt := reviewWhenDue(t, s, id, RatingAgain)
	recoverAt := lapsedAt.Add(24 * time.Hour)
	if _, err := s.reviewCardAt(id, RatingGood, recoverAt); err != nil {
		t.Fatal(err)
	}
	after := readCardState(t, s, id)
	recovered := float64(after.due-recoverAt.Unix()) / 86400
	if min := 0.12 * before; recovered < min {
		t.Fatalf("recovery interval %.1fd keeps too little of %.0fd (floor should grant >= %.1fd)", recovered, before, min)
	}
}
