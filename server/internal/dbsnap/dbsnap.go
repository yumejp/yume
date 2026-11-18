// Package dbsnap takes the safety copy a SQLite database gets before its
// schema is upgraded. Every versioned database (each deck's cards.db,
// users.db, content.db) calls BeforeMigrate when it is about to move from
// one released schema to a newer one, and refuses to migrate if the copy
// fails: a bad migration, or a later rollback to an older build, then has a
// file to return to.
package dbsnap

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Keep is how many pre-migration copies of one database are kept. They are
// pruned only by the next pre-migration copy of the same database, never by
// the deck's other snapshots.
const Keep = 3

// Kind is the filename component that marks a pre-migration copy. The deck's
// snapshot list (srs/backup.go) recognizes cards-premigrate-*.db, so a deck's
// copies can be listed and restored like any other snapshot.
const Kind = "premigrate"

// BeforeMigrate copies db, the open handle of the file at path, into
// <dir of path>/backups/<base>-premigrate-<stamp>-v<from>.db and prunes that
// database's older pre-migration copies beyond Keep. from is the schema the
// copy holds. VACUUM INTO writes a compact, self-contained file (no WAL
// sidecars) and keeps user_version and application_id. It returns the
// copy's path.
func BeforeMigrate(db *sql.DB, path string, from int) (string, error) {
	dir := filepath.Join(filepath.Dir(path), "backups")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("backups dir: %w", err)
	}
	prefix := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)) + "-" + Kind + "-"
	base := prefix + time.Now().Format("20060102-150405.000") + fmt.Sprintf("-v%d", from)
	name := base + ".db"
	for i := 2; ; i++ {
		if _, err := os.Stat(filepath.Join(dir, name)); errors.Is(err, os.ErrNotExist) {
			break
		}
		name = fmt.Sprintf("%s-%d.db", base, i)
	}
	out := filepath.Join(dir, name)
	if _, err := db.Exec(`VACUUM INTO ?`, out); err != nil {
		_ = os.Remove(out)
		return "", fmt.Errorf("vacuum into: %w", err)
	}
	// The copy is what the upgrade needs; a copy that could not be pruned
	// only costs disk space, so it never blocks the upgrade.
	_ = prune(dir, prefix)
	return out, nil
}

var stampRe = regexp.MustCompile(`^\d{8}-\d{6}\.\d{3}-v\d+(-\d+)?\.db$`)

// prune removes this database's pre-migration copies beyond Keep, oldest
// first by modification time; files it did not name are never touched.
func prune(dir, prefix string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	type snapshot struct {
		name string
		mod  time.Time
	}
	var copies []snapshot
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasPrefix(n, prefix) || !stampRe.MatchString(n[len(prefix):]) {
			continue
		}
		st, err := e.Info()
		if err != nil {
			continue
		}
		copies = append(copies, snapshot{n, st.ModTime()})
	}
	sort.Slice(copies, func(i, j int) bool {
		if !copies[i].mod.Equal(copies[j].mod) {
			return copies[i].mod.After(copies[j].mod)
		}
		return copies[i].name > copies[j].name
	})
	for i := Keep; i < len(copies); i++ {
		if err := os.Remove(filepath.Join(dir, copies[i].name)); err != nil {
			return err
		}
	}
	return nil
}
