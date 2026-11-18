package srs

import (
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func schemaVersionOf(t *testing.T, s *Store) int {
	t.Helper()
	var v int
	if err := s.q().QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestMigrationsAreOrderedAndLatestIsIdempotent(t *testing.T) {
	if baselineVersion != 1 {
		t.Fatal("public baseline must start at 1")
	}
	s := openTestStore(t)
	if v := schemaVersionOf(t, s); v != SchemaVersion() {
		t.Fatalf("fresh store at %d, want %d", v, SchemaVersion())
	}
	if err := migrate(s.q(), s.path); err != nil {
		t.Fatalf("second migrate must be a no-op: %v", err)
	}
	for i := 1; i < len(migrations); i++ {
		if migrations[i].version != migrations[i-1].version+1 {
			t.Errorf("migration versions must be consecutive: %d then %d", migrations[i-1].version, migrations[i].version)
		}
	}
}

// rawDB opens a deck file without migrating it, for setting up old states.
func rawDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func pragmaInt(t *testing.T, db *sql.DB, name string) int {
	t.Helper()
	var v int
	if err := db.QueryRow(`PRAGMA ` + name).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// stamp sets user_version and application_id on a closed deck file.
func stamp(t *testing.T, path string, version, appID int) {
	t.Helper()
	db := rawDB(t, path)
	defer db.Close()
	for _, q := range []string{
		`PRAGMA user_version = ` + strconv.Itoa(version),
		`PRAGMA application_id = ` + strconv.Itoa(appID),
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
}

// v1Deck writes a public schema-1 deck holding one learning word, closed.
func v1Deck(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cards.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetWordStatus("結局", 1); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// withNextMigration appends a harmless schema step for the test's duration,
// standing in for the first post-baseline migration.
func withNextMigration(t *testing.T) int {
	t.Helper()
	saved := migrations
	next := SchemaVersion() + 1
	migrations = append(append([]migration(nil), saved...), migration{
		version: next,
		name:    "test: a column added after release",
		apply: func(tx *sql.Tx) error {
			_, err := tx.Exec(`ALTER TABLE cards ADD COLUMN test_after_release TEXT`)
			return err
		},
	})
	t.Cleanup(func() { migrations = saved })
	return next
}

func TestFreshDeckCarriesThePublicMarker(t *testing.T) {
	s := openTestStore(t)
	if got := pragmaInt(t, s.q(), "application_id"); got != applicationID {
		t.Fatalf("application_id = %#x, want %#x", got, applicationID)
	}
}

// A schema-1 deck written by a build from before the marker (the maintainer's own,
// every friend-beta deck) must keep opening, and gets marked.
func TestPublicV1DeckWithoutMarkerIsBackfilled(t *testing.T) {
	path := v1Deck(t)
	stamp(t, path, 1, 0)
	s, err := Open(path)
	if err != nil {
		t.Fatalf("an unmarked public schema-1 deck was refused: %v", err)
	}
	defer s.Close()
	if got := pragmaInt(t, s.q(), "application_id"); got != applicationID {
		t.Fatalf("marker not backfilled: %#x", got)
	}
	rows, err := s.GetWordStatuses([]string{"結局"})
	if err != nil || len(rows) != 1 || rows[0].Status != 1 {
		t.Fatalf("deck content after backfill: %+v %v", rows, err)
	}
	if list, _ := s.ListBackups(); len(list) != 0 {
		t.Fatalf("no upgrade happened, so no snapshot is due: %+v", list)
	}
}

func TestPreReleaseDatabaseIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T) string
	}{
		{"pre-release numbering above ours", func(t *testing.T) string {
			path := filepath.Join(t.TempDir(), "old.db")
			stamp(t, path, 13, 0)
			return path
		}},
		{"schema 1 without the baseline tables", func(t *testing.T) string {
			path := filepath.Join(t.TempDir(), "old.db")
			db := rawDB(t, path)
			if _, err := db.Exec(`CREATE TABLE cards (id INTEGER PRIMARY KEY, lemma TEXT NOT NULL)`); err != nil {
				t.Fatal(err)
			}
			db.Close()
			stamp(t, path, 1, 0)
			return path
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := tc.setup(t)
			s, err := Open(path)
			if err == nil {
				s.Close()
				t.Fatal("a pre-release database opened without error")
			}
			if SchemaErrKind(err) != ErrKindSchemaPreRelease || !strings.Contains(err.Error(), "pre-release Yume build") ||
				!strings.Contains(err.Error(), "Decks from pre-release builds") {
				t.Fatalf("want the pre-release refusal, got %q", err)
			}
		})
	}
}

// Once public migrations reach the numbers pre-release builds used, the
// marker is the only thing telling the two apart.
func TestPreReleaseDeckAtAPublicVersionIsRefused(t *testing.T) {
	path := v1Deck(t)
	next := withNextMigration(t)
	stamp(t, path, next, 0)
	s, err := Open(path)
	if err == nil {
		s.Close()
		t.Fatalf("an unmarked schema-%d deck opened", next)
	}
	if SchemaErrKind(err) != ErrKindSchemaPreRelease {
		t.Fatalf("want the pre-release refusal, got %q", err)
	}
}

func TestNewerAndForeignDatabasesAreRefused(t *testing.T) {
	path := v1Deck(t)
	stamp(t, path, SchemaVersion()+5, applicationID)
	if _, err := Open(path); SchemaErrKind(err) != ErrKindSchemaNewer || !strings.Contains(err.Error(), "is newer than supported schema") {
		t.Fatalf("newer deck: %v", err)
	}
	stamp(t, path, 1, 0x12345678)
	if _, err := Open(path); SchemaErrKind(err) != ErrKindNotYume {
		t.Fatalf("foreign database: %v", err)
	}
}

// Upgrading a released deck snapshots it first; the snapshot is the old
// schema, restorable like any other, and does not count against the
// learner's own backups.
func TestUpgradeFromV1SnapshotsFirst(t *testing.T) {
	path := v1Deck(t)
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxBackups; i++ {
		if _, _, err := s.CreateBackup("mine"); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()

	next := withNextMigration(t)
	s, err = Open(path)
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() }) // s is reopened below
	if v := schemaVersionOf(t, s); v != next {
		t.Fatalf("upgraded to %d, want %d", v, next)
	}
	list, err := s.ListBackups()
	if err != nil {
		t.Fatal(err)
	}
	var pre []BackupInfo
	manual := 0
	for _, b := range list {
		switch b.Kind {
		case BackupPreMigrate:
			pre = append(pre, b)
		case BackupManual:
			manual++
		}
	}
	if len(pre) != 1 || !strings.HasPrefix(pre[0].Name, "cards-premigrate-") || !strings.HasSuffix(pre[0].Name, "-v1.db") {
		t.Fatalf("want one cards-premigrate-*-v1.db, got %+v", list)
	}
	if manual != MaxBackups {
		t.Fatalf("the pre-migration snapshot pruned a manual backup: %+v", list)
	}
	snap := rawDB(t, filepath.Join(s.backupsDir(), pre[0].Name))
	defer snap.Close()
	if v := pragmaInt(t, snap, "user_version"); v != 1 {
		t.Fatalf("snapshot holds schema %d, want 1", v)
	}
	if id := pragmaInt(t, snap, "application_id"); id != applicationID {
		t.Fatalf("snapshot lost the marker: %#x", id)
	}
	var status int
	if err := snap.QueryRow(`SELECT status FROM word_status WHERE lemma = '結局'`).Scan(&status); err != nil || status != 1 {
		t.Fatalf("snapshot content: %d %v", status, err)
	}
	// Opening again is a no-op: no second snapshot.
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListBackups(); len(list) != len(pre)+manual {
		t.Fatalf("reopening took another snapshot: %+v", list)
	}
}

func TestUpgradeStopsWhenTheSnapshotFails(t *testing.T) {
	path := v1Deck(t)
	// A file where the backups folder belongs makes the snapshot fail.
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), "backups"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	withNextMigration(t)
	if s, err := Open(path); err == nil {
		s.Close()
		t.Fatal("the upgrade ran without its snapshot")
	} else if !strings.Contains(err.Error(), "nothing was changed") {
		t.Fatalf("unexpected error: %v", err)
	}
	db := rawDB(t, path)
	defer db.Close()
	if v := pragmaInt(t, db, "user_version"); v != 1 {
		t.Fatalf("deck moved to schema %d without a snapshot", v)
	}
}

// A snapshot a pre-release build wrote (older installs keep them in backups/)
// is refused before the swap, so the live deck keeps working.
func TestRestoreRefusesAPreReleaseSnapshot(t *testing.T) {
	s := openTestStore(t)
	if err := s.SetWordStatus("結局", 1); err != nil {
		t.Fatal(err)
	}
	info, _, err := s.CreateBackup("old-build")
	if err != nil {
		t.Fatal(err)
	}
	stamp(t, filepath.Join(s.backupsDir(), info.Name), 19, 0)

	_, _, err = s.RestoreBackup(info.Name)
	if err == nil || !IsClientErr(err) || !strings.Contains(err.Error(), "pre-release Yume build") {
		t.Fatalf("want a client error naming the pre-release build, got %v", err)
	}
	rows, err := s.GetWordStatuses([]string{"結局"})
	if err != nil || len(rows) != 1 || rows[0].Status != 1 {
		t.Fatalf("live deck after the refused restore: %+v %v", rows, err)
	}
	list, _ := s.ListBackups()
	for _, b := range list {
		if b.Kind == BackupPreRestore {
			t.Fatalf("a refused restore took a safety snapshot: %+v", list)
		}
	}
}
