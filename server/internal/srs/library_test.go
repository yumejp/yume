package srs

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
)

func openTestLibrary(t *testing.T) *Library {
	t.Helper()
	l, err := OpenLibrary(filepath.Join(t.TempDir(), "content.db"))
	if err != nil {
		t.Fatalf("OpenLibrary: %v", err)
	}
	SetLibrary(l)
	t.Cleanup(func() { SetLibrary(nil); _ = l.Close() })
	return l
}

func TestLibraryMnemonicIsFallbackAndCardOverrides(t *testing.T) {
	lib := openTestLibrary(t)
	s := openTestStore(t)
	resp, err := s.SaveCard(SaveCardReq{Lemma: "把握", Readings: []string{"はあく"}, Glosses: [][]string{{"grasp"}}})
	if err != nil {
		t.Fatal(err)
	}
	shared := []MnemonicPart{{Kanji: "把", Meaning: "grip"}, {Kanji: "握", Meaning: "clench"}}
	if _, err := lib.SetMnemonic("ja", "把握", shared, "Grip and clench."); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.SetMnemonic("ja", "把握", []MnemonicPart{{Kanji: "食", Meaning: "eat"}}, "x"); !IsClientErr(err) {
		t.Errorf("library accepted a breakdown that misses the lemma: %v", err)
	}

	due, err := s.GetDueCards(10)
	if err != nil || len(due) != 1 {
		t.Fatalf("due = %+v, %v", due, err)
	}
	if due[0].Mnemonic == nil || !due[0].MnemonicShared || due[0].Mnemonic.Story != "Grip and clench." {
		t.Errorf("card without its own mnemonic should show the shared one: %+v shared=%v", due[0].Mnemonic, due[0].MnemonicShared)
	}
	audit, err := s.MnemonicAudit(false)
	if err != nil || len(audit) != 1 || audit[0].Kind != "ok" || !audit[0].Shared {
		t.Errorf("audit sees the shared mnemonic as ok: %+v %v", audit, err)
	}

	// The card's own mnemonic wins.
	if _, err := s.SetMnemonic(resp.ID, shared, "My own story."); err != nil {
		t.Fatal(err)
	}
	due, _ = s.GetDueCards(10)
	if due[0].MnemonicShared || due[0].Mnemonic.Story != "My own story." {
		t.Errorf("own mnemonic should override: %+v shared=%v", due[0].Mnemonic, due[0].MnemonicShared)
	}
	vocab, err := s.ListVocab(VocabFilter{}, 50, 0)
	if err != nil || len(vocab.Rows) != 1 || vocab.Rows[0].MnemonicShared || vocab.Rows[0].Mnemonic == nil {
		t.Errorf("vocab row = %+v, %v", vocab.Rows, err)
	}

	// Clearing the override falls back to the shared one.
	if _, err := s.SetMnemonic(resp.ID, nil, ""); err != nil {
		t.Fatal(err)
	}
	due, _ = s.GetDueCards(10)
	if !due[0].MnemonicShared || due[0].Mnemonic.Story != "Grip and clench." {
		t.Errorf("cleared override should show the shared one again: %+v", due[0].Mnemonic)
	}
	// Another language's library entry is never shown.
	if _, err := lib.SetMnemonic("zh", "把握", nil, "zh story"); err != nil {
		t.Fatal(err)
	}
	if m, _ := lib.Mnemonic("ja", "把握"); m.Story != "Grip and clench." {
		t.Errorf("ja entry changed: %+v", m)
	}
}

func TestShareMnemonicsMovesDeckCopiesIntoLibrary(t *testing.T) {
	lib := openTestLibrary(t)
	s := openTestStore(t)
	var ids []int64
	for _, w := range []struct{ lemma, story string }{{"把握", "Grip and clench."}, {"方面", "The facing side."}, {"踊る", "Dance."}} {
		resp, err := s.SaveCard(SaveCardReq{Lemma: w.lemma, Glosses: [][]string{{"x"}}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.SetMnemonic(resp.ID, nil, w.story); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, resp.ID)
	}
	// The library already knows 方面 differently: the card keeps that as an override.
	if _, err := lib.SetMnemonic("ja", "方面", []MnemonicPart{{Kanji: "方", Meaning: "direction"}, {Kanji: "面", Meaning: "side"}}, "Library story."); err != nil {
		t.Fatal(err)
	}
	shared, cleared, err := s.ShareMnemonics(lib)
	if err != nil {
		t.Fatal(err)
	}
	if shared != 2 || cleared != 2 {
		t.Errorf("shared %d cleared %d, want 2 and 2", shared, cleared)
	}
	if m, _ := s.GetMnemonic(ids[0]); m != nil {
		t.Errorf("把握 should now be a library read, card still has %+v", m)
	}
	if m, _ := s.GetMnemonic(ids[1]); m == nil || m.Story != "The facing side." {
		t.Errorf("方面 should keep its override, got %+v", m)
	}
	entries, err := lib.Export("ja")
	if err != nil || len(entries) != 3 {
		t.Fatalf("export = %+v, %v", entries, err)
	}
	// Export → import round-trips into an empty library.
	other, err := OpenLibrary(filepath.Join(t.TempDir(), "content.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	added, replaced, skipped, err := other.Import(entries, false)
	if err != nil || added != 3 || replaced != 0 || skipped != 0 {
		t.Errorf("import = %d %d %d %v", added, replaced, skipped, err)
	}
	added, replaced, skipped, _ = other.Import(entries, false)
	if added != 0 || skipped != 3 {
		t.Errorf("second import = %d %d %d", added, replaced, skipped)
	}
	// Sharing again is a no-op.
	if shared, cleared, _ := s.ShareMnemonics(lib); shared != 0 || cleared != 0 {
		t.Errorf("re-share = %d %d", shared, cleared)
	}
}

func TestSharedKanjiNotesFallBackAndShare(t *testing.T) {
	lib := openTestLibrary(t)
	s := openTestStore(t)
	if err := lib.SetKanjiNote("ja", "案", "A tree under a roof: a plan."); err != nil {
		t.Fatal(err)
	}
	info, err := s.KanjiInfo("案")
	if err != nil || info.Note != "A tree under a roof: a plan." || !info.NoteShared {
		t.Errorf("shared story should show: %+v %v", info, err)
	}
	if err := s.SetKanjiNote("案", "My own story."); err != nil {
		t.Fatal(err)
	}
	if info, _ = s.KanjiInfo("案"); info.Note != "My own story." || info.NoteShared {
		t.Errorf("own story should override: %+v", info)
	}
	if err := s.SetKanjiNote("供", "Hands holding out."); err != nil {
		t.Fatal(err)
	}
	shared, cleared, err := s.ShareKanjiNotes(lib)
	if err != nil || shared != 1 || cleared != 1 {
		t.Errorf("share = %d %d %v, want 1 1", shared, cleared, err)
	}
	if info, _ = s.KanjiInfo("供"); !info.NoteShared {
		t.Errorf("供 should now be a library read: %+v", info)
	}
	if info, _ = s.KanjiInfo("案"); info.NoteShared {
		t.Errorf("案 keeps its override: %+v", info)
	}
	content, err := lib.ExportAll("")
	if err != nil || len(content.KanjiNotes) != 2 || len(content.Mnemonics) != 0 {
		t.Fatalf("export = %+v %v", content, err)
	}
	other, err := OpenLibrary(filepath.Join(t.TempDir(), "content.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if n, err := other.ImportAll(content, false); err != nil || n.Added != 2 {
		t.Errorf("import = %+v %v", n, err)
	}
}

// content.db is versioned like the decks: new and existing files end at the
// current version, an older versioned file is copied aside first, and a
// newer one is refused.
func TestLibrarySchemaVersioning(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "content.db")
	libVersion := func() int {
		t.Helper()
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		var v int
		if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	setVersion := func(v int) {
		t.Helper()
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		if _, err := db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, v)); err != nil {
			t.Fatal(err)
		}
	}
	latest := libraryMigrations[len(libraryMigrations)-1].version

	l, err := OpenLibrary(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.SetMnemonic("ja", "把握", []MnemonicPart{{Kanji: "把", Meaning: "grip"}, {Kanji: "握", Meaning: "clench"}}, "Grip and clench."); err != nil {
		t.Fatal(err)
	}
	l.Close()
	if v := libVersion(); v != latest {
		t.Fatalf("new content.db at %d, want %d", v, latest)
	}

	// An older versioned file: snapshot, then upgrade with rows intact.
	setVersion(1)
	if l, err = OpenLibrary(path); err != nil {
		t.Fatal(err)
	}
	m, err := l.Mnemonic("ja", "把握")
	l.Close()
	if err != nil || m == nil || m.Story != "Grip and clench." {
		t.Fatalf("library row after upgrade: %+v %v", m, err)
	}
	if v := libVersion(); v != latest {
		t.Fatalf("upgraded content.db at %d, want %d", v, latest)
	}
	if copies, _ := filepath.Glob(filepath.Join(dir, "backups", "content-premigrate-*-v1.db")); len(copies) != 1 {
		t.Fatalf("want one content-premigrate-*-v1.db, got %v", copies)
	}

	setVersion(latest + 1)
	if l, err := OpenLibrary(path); err == nil {
		l.Close()
		t.Fatal("a newer content.db opened")
	} else if SchemaErrKind(err) != ErrKindSchemaNewer {
		t.Fatalf("unexpected error: %v", err)
	}
}
