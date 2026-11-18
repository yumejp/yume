package dbsnap

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestBeforeMigrateKeepsTheNewestCopies(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "users.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE t (x); INSERT INTO t VALUES (1); PRAGMA user_version = 4`); err != nil {
		t.Fatal(err)
	}
	// Files that are not this database's pre-migration copies stay.
	backups := filepath.Join(dir, "backups")
	if err := os.MkdirAll(backups, 0o755); err != nil {
		t.Fatal(err)
	}
	others := []string{"cards-premigrate-20260101-000000.000-v1.db", "users-20260101-000000.000.db", "notes.txt"}
	for _, n := range others {
		if err := os.WriteFile(filepath.Join(backups, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var made []string
	for i := 0; i < Keep+2; i++ {
		out, err := BeforeMigrate(db, path, 4)
		if err != nil {
			t.Fatal(err)
		}
		made = append(made, filepath.Base(out))
	}
	for _, n := range made {
		if !strings.HasPrefix(n, "users-premigrate-") || !strings.Contains(n, "-v4") {
			t.Fatalf("copy name %q", n)
		}
	}
	left, _ := filepath.Glob(filepath.Join(backups, "users-premigrate-*"))
	if len(left) != Keep {
		t.Fatalf("kept %d copies, want %d: %v", len(left), Keep, left)
	}
	if _, err := os.Stat(filepath.Join(backups, made[len(made)-1])); err != nil {
		t.Fatalf("newest copy pruned: %v", err)
	}
	for _, n := range others {
		if _, err := os.Stat(filepath.Join(backups, n)); err != nil {
			t.Fatalf("%s removed: %v", n, err)
		}
	}

	copyDB, err := sql.Open("sqlite", filepath.Join(backups, made[len(made)-1]))
	if err != nil {
		t.Fatal(err)
	}
	defer copyDB.Close()
	var v, x int
	if err := copyDB.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil || v != 4 {
		t.Fatalf("copy user_version = %d (%v)", v, err)
	}
	if err := copyDB.QueryRow(`SELECT x FROM t`).Scan(&x); err != nil || x != 1 {
		t.Fatalf("copy content = %d (%v)", x, err)
	}
}
