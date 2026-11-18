package auth

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// A users.db written before versioning (tables present, user_version 0)
// must come up at the latest version with its rows intact and stamped.
func TestMigrateUnversionedUsersDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range migrations[0].stmts {
		if _, err := raw.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := raw.Exec(`INSERT INTO users (id, display_name, role, created_at) VALUES ('u1', 'Owner', 'owner', 100)`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO sessions (token_hash, user_id, created_at, expires_at) VALUES ('h', 'u1', 100, 4102444800)`); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	var v int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&v); err != nil || v != SchemaVersion() {
		t.Fatalf("user_version = %d (%v), want %d", v, err, SchemaVersion())
	}
	var updated int64
	if err := s.db.QueryRow(`SELECT updated_at FROM users WHERE id = 'u1'`).Scan(&updated); err != nil || updated != 100 {
		t.Errorf("users.updated_at backfilled = %d (%v), want 100", updated, err)
	}
	if _, err := s.db.Exec(`UPDATE users SET display_name = 'Renamed' WHERE id = 'u1'`); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT updated_at FROM users WHERE id = 'u1'`).Scan(&updated); err != nil || updated < 1_700_000_000 {
		t.Errorf("users.updated_at after update = %d (%v), want now", updated, err)
	}
	if u, ok := s.GetUser("u1"); !ok || u.DisplayName != "Renamed" {
		t.Errorf("user after migration = %+v %v", u, ok)
	}
	// Opening again is a no-op.
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = s2.Close()
}

// A users.db at an older released schema is copied aside before it is
// upgraded; the copy keeps the old version and the accounts.
func TestUpgradeSnapshotsUsersDBFirst(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "users.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range append(append([]string(nil), migrations[0].stmts...),
		`INSERT INTO users (id, display_name, role, created_at) VALUES ('u1', 'Owner', 'owner', 100)`,
		`PRAGMA user_version = 1`) {
		if _, err := raw.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	_ = raw.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	copies, _ := filepath.Glob(filepath.Join(dir, "backups", "users-premigrate-*-v1.db"))
	if len(copies) != 1 {
		t.Fatalf("want one users-premigrate-*-v1.db, got %v", copies)
	}
	snap, err := sql.Open("sqlite", copies[0])
	if err != nil {
		t.Fatal(err)
	}
	defer snap.Close()
	var v, n int
	if err := snap.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil || v != 1 {
		t.Fatalf("snapshot schema = %d (%v), want 1", v, err)
	}
	if err := snap.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("snapshot users = %d (%v)", n, err)
	}
}

func TestNewerUsersDBIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, SchemaVersion()+1)); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()
	s, err := Open(path)
	if err == nil {
		s.Close()
		t.Fatal("a newer users.db opened")
	}
	if !strings.Contains(err.Error(), "is newer than supported schema") {
		t.Fatalf("unexpected error: %v", err)
	}
}
