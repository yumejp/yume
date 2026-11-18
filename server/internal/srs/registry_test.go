package srs

import (
	"os"
	"path/filepath"
	"testing"
)

// TestForUserIsolation proves two users get independent decks: a write to one
// is invisible in the other, and their DB files are distinct.
func TestForUserIsolation(t *testing.T) {
	dir := t.TempDir()
	// Register after TempDir so it runs BEFORE TempDir's removal (LIFO):
	// open file handles must be closed before Windows can delete the files.
	t.Cleanup(CloseAll)
	t.Setenv("YUME_DB", filepath.Join(dir, "cards.db"))

	a, err := ForUser("alice")
	if err != nil {
		t.Fatalf("ForUser(alice): %v", err)
	}
	b, err := ForUser("bob")
	if err != nil {
		t.Fatalf("ForUser(bob): %v", err)
	}
	if a.path == b.path {
		t.Fatalf("expected distinct DB paths, both = %s", a.path)
	}

	if _, err := a.SaveCard(SaveCardReq{Lemma: "テスト"}); err != nil {
		t.Fatalf("SaveCard to alice: %v", err)
	}

	da, err := a.DeckStats()
	if err != nil {
		t.Fatalf("alice DeckStats: %v", err)
	}
	db, err := b.DeckStats()
	if err != nil {
		t.Fatalf("bob DeckStats: %v", err)
	}
	if da.Total != 1 {
		t.Errorf("alice total = %d, want 1", da.Total)
	}
	if db.Total != 0 {
		t.Errorf("bob total = %d, want 0 (write leaked across users)", db.Total)
	}
	if da.DBPath == db.DBPath {
		t.Errorf("DeckStats reported same db_path for both users: %s", da.DBPath)
	}
}

// TestBootstrapMovesLegacyDeckOnce proves the single-tenant deck is relocated
// under the owner exactly once, backed up first, and that a re-run is a no-op.
func TestBootstrapMovesLegacyDeckOnce(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(CloseAll) // see note in TestForUserIsolation
	legacy := filepath.Join(dir, "cards.db")
	t.Setenv("YUME_DB", legacy)

	// Seed a legacy single-tenant deck with one card.
	ls, err := Open(legacy)
	if err != nil {
		t.Fatalf("open legacy: %v", err)
	}
	if _, err := ls.SaveCard(SaveCardReq{Lemma: "古い"}); err != nil {
		t.Fatalf("seed legacy card: %v", err)
	}
	if err := ls.Close(); err != nil {
		t.Fatalf("close legacy: %v", err)
	}

	// Sibling artifacts that must travel with the deck.
	if err := os.MkdirAll(filepath.Join(dir, "media"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "media", "1-audio.mp3"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "userdict.txt"), []byte("エモい\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	const ownerID = "owner-1"
	moved, err := MigrateLegacyDeckTo(ownerID)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !moved {
		t.Fatal("expected moved=true on first migration")
	}
	if fileExists(legacy) {
		t.Error("legacy file should be gone after move")
	}
	if !fileExists(legacy + ".premultiuser.bak") {
		t.Error("expected .premultiuser.bak backup to exist")
	}

	// The owner's deck now holds the migrated card.
	owner, err := ForUser(ownerID)
	if err != nil {
		t.Fatalf("ForUser(owner): %v", err)
	}
	ds, err := owner.DeckStats()
	if err != nil {
		t.Fatalf("owner DeckStats: %v", err)
	}
	if ds.Total != 1 {
		t.Errorf("owner deck total = %d, want 1 (legacy card not migrated)", ds.Total)
	}

	// Sibling artifacts moved under the owner; gone from the data root.
	if !fileExists(filepath.Join(dir, "users", ownerID, "media", "1-audio.mp3")) {
		t.Error("media sidecar not relocated under owner")
	}
	if !fileExists(filepath.Join(dir, "users", ownerID, "userdict.txt")) {
		t.Error("userdict.txt not relocated under owner")
	}
	if dirExists(filepath.Join(dir, "media")) {
		t.Error("legacy media dir should be gone after move")
	}

	// Idempotent: a second run does nothing (legacy already gone).
	moved2, err := MigrateLegacyDeckTo(ownerID)
	if err != nil {
		t.Fatalf("migrate (2nd): %v", err)
	}
	if moved2 {
		t.Error("expected moved=false on second migration (not idempotent)")
	}
}
