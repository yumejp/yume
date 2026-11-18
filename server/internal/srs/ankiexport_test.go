package srs

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "cards.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// readApkg unzips an exported package into entry-name → bytes.
func readApkg(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip: %v", err)
	}
	out := map[string][]byte{}
	for _, f := range zr.File {
		r, err := f.Open()
		if err != nil {
			t.Fatalf("zip open %s: %v", f.Name, err)
		}
		b, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatalf("zip read %s: %v", f.Name, err)
		}
		out[f.Name] = b
	}
	return out
}

// openCollection writes the collection bytes to disk and opens them as SQLite.
func openCollection(t *testing.T, b []byte) *sql.DB {
	t.Helper()
	p := filepath.Join(t.TempDir(), "collection.anki2")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatalf("write collection: %v", err)
	}
	db, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatalf("open collection: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestExportApkgEmptyDeck(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.ExportApkg(""); err == nil {
		t.Fatal("expected error for empty deck")
	}
}

func TestExportApkg(t *testing.T) {
	s := openTestStore(t)

	// Card 1: full content + tag + audio sidecar, then reviewed twice so it
	// carries FSRS state and revlog history.
	r1, err := s.SaveCard(SaveCardReq{
		Lemma:    "今日",
		Kanji:    []string{"今日"},
		Readings: []string{"きょう"},
		Glosses:  [][]string{{"today"}, {"this day"}},
		Sentence: "今日はいい天気だ",
	})
	if err != nil {
		t.Fatalf("SaveCard 1: %v", err)
	}
	if err := s.AddWordTag("今日", "import:test"); err != nil {
		t.Fatalf("AddWordTag: %v", err)
	}
	audio := base64.StdEncoding.EncodeToString([]byte("fake-webm-bytes"))
	if err := s.AttachMedia(r1.ID, &MediaBlob{Mime: "audio/webm", Data: audio}, nil, "voicevox:test"); err != nil {
		t.Fatalf("AttachMedia: %v", err)
	}
	for _, rating := range []ReviewRating{3, 3} {
		if _, err := s.ReviewCard(r1.ID, rating); err != nil {
			t.Fatalf("ReviewCard: %v", err)
		}
	}

	// Card 2: never reviewed — must export as a new card.
	if _, err := s.SaveCard(SaveCardReq{
		Lemma:   "綺麗",
		Glosses: [][]string{{"pretty", "clean"}},
	}); err != nil {
		t.Fatalf("SaveCard 2: %v", err)
	}

	pkg, err := s.ExportApkg("My Yume Deck")
	if err != nil {
		t.Fatalf("ExportApkg: %v", err)
	}
	files := readApkg(t, pkg)

	// Media manifest maps the numbered zip entry back to the real filename.
	var manifest map[string]string
	if err := json.Unmarshal(files["media"], &manifest); err != nil {
		t.Fatalf("media manifest: %v", err)
	}
	if len(manifest) != 1 {
		t.Fatalf("manifest = %v, want exactly the audio file", manifest)
	}
	for n, name := range manifest {
		if !strings.HasSuffix(name, ".webm") {
			t.Errorf("media name = %q, want .webm", name)
		}
		if string(files[n]) != "fake-webm-bytes" {
			t.Errorf("media bytes for entry %s don't round-trip", n)
		}
	}

	db := openCollection(t, files["collection.anki2"])

	// col row: schema version + parseable JSON blobs naming our deck/model.
	var ver int
	var modelsJSON, decksJSON string
	if err := db.QueryRow(`SELECT ver, models, decks FROM col`).Scan(&ver, &modelsJSON, &decksJSON); err != nil {
		t.Fatalf("col row: %v", err)
	}
	if ver != 11 {
		t.Errorf("ver = %d, want 11", ver)
	}
	var models map[string]struct {
		Name string `json:"name"`
		Flds []struct {
			Name string `json:"name"`
		} `json:"flds"`
	}
	if err := json.Unmarshal([]byte(modelsJSON), &models); err != nil {
		t.Fatalf("models json: %v", err)
	}
	m, ok := models["1748000000001"]
	if !ok || m.Name != "Yume Vocab" {
		t.Fatalf("model missing or misnamed: %v", models)
	}
	if len(m.Flds) != len(ankiFields) {
		t.Errorf("model has %d fields, want %d", len(m.Flds), len(ankiFields))
	}
	if !strings.Contains(decksJSON, `"My Yume Deck"`) {
		t.Errorf("decks json missing deck name: %s", decksJSON)
	}

	// Notes: 2, with fields joined by \x1f, tags and [sound:] reference intact.
	rows, err := db.Query(`SELECT guid, flds, tags FROM notes ORDER BY id`)
	if err != nil {
		t.Fatalf("notes: %v", err)
	}
	defer rows.Close()
	type note struct{ guid, flds, tags string }
	var notes []note
	for rows.Next() {
		var n note
		if err := rows.Scan(&n.guid, &n.flds, &n.tags); err != nil {
			t.Fatalf("scan note: %v", err)
		}
		notes = append(notes, n)
	}
	if len(notes) != 2 {
		t.Fatalf("notes = %d, want 2", len(notes))
	}
	f1 := strings.Split(notes[0].flds, "\x1f")
	if len(f1) != len(ankiFields) {
		t.Fatalf("note 1 has %d fields, want %d", len(f1), len(ankiFields))
	}
	if f1[0] != "今日" || f1[1] != "きょう" || f1[2] != "today | this day" || f1[3] != "今日はいい天気だ" {
		t.Errorf("note 1 fields = %q", f1)
	}
	if !strings.HasPrefix(f1[5], "[sound:") {
		t.Errorf("audio field = %q, want [sound:...] reference", f1[5])
	}
	if notes[0].tags != " import:test " {
		t.Errorf("tags = %q, want %q", notes[0].tags, " import:test ")
	}
	if notes[0].guid == notes[1].guid {
		t.Error("note guids must be distinct")
	}

	// Cards: reviewed card is in the review queue with a real interval;
	// unreviewed card is new (queue 0) with factor 0.
	cardRows, err := db.Query(`SELECT type, queue, ivl, factor, reps FROM cards ORDER BY id`)
	if err != nil {
		t.Fatalf("cards: %v", err)
	}
	defer cardRows.Close()
	type ankiCard struct{ typ, queue, ivl, factor, reps int }
	var ankiCards []ankiCard
	for cardRows.Next() {
		var c ankiCard
		if err := cardRows.Scan(&c.typ, &c.queue, &c.ivl, &c.factor, &c.reps); err != nil {
			t.Fatalf("scan card: %v", err)
		}
		ankiCards = append(ankiCards, c)
	}
	if len(ankiCards) != 2 {
		t.Fatalf("cards = %d, want 2", len(ankiCards))
	}
	reviewed, fresh := ankiCards[0], ankiCards[1]
	if reviewed.queue != 2 || reviewed.typ != 2 || reviewed.ivl < 1 || reviewed.factor != 2500 || reviewed.reps != 2 {
		t.Errorf("reviewed card = %+v, want review queue with ivl>=1, factor 2500, reps 2", reviewed)
	}
	if fresh.queue != 0 || fresh.typ != 0 || fresh.factor != 0 {
		t.Errorf("new card = %+v, want new queue with factor 0", fresh)
	}

	// Revlog: both reviews present; first typed as learn (0), second review (1).
	var eases, types []int
	logRows, err := db.Query(`SELECT ease, type FROM revlog ORDER BY id`)
	if err != nil {
		t.Fatalf("revlog: %v", err)
	}
	defer logRows.Close()
	for logRows.Next() {
		var e, ty int
		if err := logRows.Scan(&e, &ty); err != nil {
			t.Fatalf("scan revlog: %v", err)
		}
		eases = append(eases, e)
		types = append(types, ty)
	}
	if len(eases) != 2 || eases[0] != 3 || eases[1] != 3 {
		t.Errorf("revlog eases = %v, want [3 3]", eases)
	}
	if len(types) != 2 || types[0] != 0 || types[1] != 1 {
		t.Errorf("revlog types = %v, want [0 1]", types)
	}
}

func TestAnkiTagString(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"a"}, " a "},
		{[]string{"a", "b"}, " a b "},
		{[]string{"has space"}, " has_space "},
		{[]string{"  ", ""}, ""},
	}
	for _, c := range cases {
		if got := ankiTagString(c.in); got != c.want {
			t.Errorf("ankiTagString(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
