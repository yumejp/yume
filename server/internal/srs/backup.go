package srs

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/yumejp/yume/server/internal/dbsnap"
)

// Managed vocab backups — snapshot/restore of the whole SQLite store
// (cards, word statuses, tags, notes, review history, watch words).
//
// Snapshots live in <dataDir>/backups/ and come in kinds: manual ones
// (backup_vocab, /create_backup) and the automatic ones taken before an
// import, a restore or a schema upgrade. Each kind keeps its own newest
// MaxBackups, pruned only when a snapshot of the same kind is created, so a
// run of imports never pushes out the learner's own backup or the snapshot a
// restore took. Restoring first takes a "prerestore" safety snapshot, so a
// bad restore is itself undoable. Media sidecar FILES are not part of the
// snapshot — they live next to the DB; consumers already tolerate stale media
// references. A restore moves back any file the restored cards point at from
// media/.trash (where delete_by_tag puts them), so restoring the snapshot an
// import took brings back the audio its undo removed.

// MaxBackups caps how many snapshots of each kind are kept. Newest wins;
// creating the N+1th of a kind prunes the oldest of that kind.
const MaxBackups = 3

// Snapshot kinds, the BackupInfo.Kind values.
const (
	BackupManual     = "manual"
	BackupPreImport  = "import"
	BackupPreRestore = "prerestore"
	BackupPreMigrate = dbsnap.Kind // taken by migrate, see dbsnap
)

// backupNameRe matches our own snapshot filenames — pruning only ever
// touches files we created, never anything else a user drops in the dir.
// Automatic kinds sit before the stamp (cards-import-<stamp>-<tag>.db), so a
// manual label can never be mistaken for one. Millisecond component prevents
// same-second name reuse (prune could free a name that the very next backup
// would then claim, confusing ordering).
var backupNameRe = regexp.MustCompile(`^cards-(?:(import|prerestore|premigrate)-)?\d{8}-\d{6}\.\d{3}(-[a-z0-9-]+)?(-\d+)?\.db$`)

// legacyPreRestoreRe: before kinds existed the pre-restore snapshot was a
// manual-style name labelled "prerestore"; it keeps its kind. Older pre-import
// snapshots were labelled with the batch tag and count as manual, and so
// does a manual label that only ends in "prerestore".
var legacyPreRestoreRe = regexp.MustCompile(`^cards-\d{8}-\d{6}\.\d{3}-prerestore(-\d+)?\.db$`)

// backupKind classifies one of our snapshot filenames.
func backupKind(name string) string {
	m := backupNameRe.FindStringSubmatch(name)
	switch {
	case m == nil:
		return ""
	case m[1] != "":
		return m[1]
	case legacyPreRestoreRe.MatchString(name):
		return BackupPreRestore
	}
	return BackupManual
}

var labelRe = regexp.MustCompile(`[^a-z0-9-]+`)

// BackupInfo describes one snapshot.
type BackupInfo struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"` // manual, import, prerestore or premigrate
	CreatedAt string `json:"created_at"`
	SizeBytes int64  `json:"size_bytes"`
}

func (s *Store) backupsDir() string {
	return filepath.Join(s.DataDir(), "backups")
}

// CreateBackup takes a manual snapshot of the live DB and prunes manual
// snapshots beyond MaxBackups; it returns the pruned names. The snapshot is
// produced with VACUUM INTO, so it's a compact, self-contained file with no
// WAL sidecars, taken safely while the DB is in use. `label` (optional) is
// sanitized into the filename.
func (s *Store) CreateBackup(label string) (BackupInfo, []string, error) {
	return s.createBackup(BackupManual, label)
}

func (s *Store) createBackup(kind, label string) (BackupInfo, []string, error) {
	s.backupMu.Lock()
	defer s.backupMu.Unlock()
	return s.createBackupLocked(kind, label)
}

func (s *Store) createBackupLocked(kind, label string) (BackupInfo, []string, error) {
	dir := s.backupsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return BackupInfo{}, nil, fmt.Errorf("backups dir: %w", err)
	}
	label = labelRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(label)), "-")
	if len(label) > 24 {
		label = label[:24]
	}
	base := "cards-"
	if kind != BackupManual {
		base += kind + "-"
	}
	base += time.Now().Format("20060102-150405.000")
	if label != "" {
		base += "-" + label
	}
	name := base + ".db"
	// Dedupe within the same second.
	for i := 2; ; i++ {
		if _, err := os.Stat(filepath.Join(dir, name)); errors.Is(err, os.ErrNotExist) {
			break
		}
		name = fmt.Sprintf("%s-%d.db", base, i)
	}
	path := filepath.Join(dir, name)
	if _, err := s.q().Exec(`VACUUM INTO ?`, path); err != nil {
		return BackupInfo{}, nil, fmt.Errorf("vacuum into: %w", err)
	}
	st, err := os.Stat(path)
	if err != nil {
		return BackupInfo{}, nil, err
	}
	pruned, err := s.pruneLocked(kind)
	if err != nil {
		return BackupInfo{}, nil, err
	}
	return BackupInfo{Name: name, Kind: kind, CreatedAt: st.ModTime().UTC().Format(time.RFC3339), SizeBytes: st.Size()}, pruned, nil
}

// pruneLocked deletes the oldest snapshots of one kind beyond MaxBackups
// (list is newest-first by mtime). Other kinds are never touched.
func (s *Store) pruneLocked(kind string) ([]string, error) {
	infos, err := s.listLocked()
	if err != nil {
		return nil, err
	}
	var pruned []string
	kept := 0
	for _, info := range infos { // infos is newest-first
		if info.Kind != kind {
			continue
		}
		if kept++; kept <= MaxBackups {
			continue
		}
		if err := os.Remove(filepath.Join(s.backupsDir(), info.Name)); err != nil {
			return pruned, err
		}
		pruned = append(pruned, info.Name)
	}
	return pruned, nil
}

// ListBackups returns the snapshots, newest first.
func (s *Store) ListBackups() ([]BackupInfo, error) {
	s.backupMu.Lock()
	defer s.backupMu.Unlock()
	return s.listLocked()
}

func (s *Store) listLocked() ([]BackupInfo, error) {
	entries, err := os.ReadDir(s.backupsDir())
	if errors.Is(err, os.ErrNotExist) {
		return []BackupInfo{}, nil
	}
	if err != nil {
		return nil, err
	}
	type withTime struct {
		info BackupInfo
		mod  time.Time
	}
	var rows []withTime
	for _, e := range entries {
		if e.IsDir() || !backupNameRe.MatchString(e.Name()) {
			continue
		}
		st, err := e.Info()
		if err != nil {
			continue
		}
		rows = append(rows, withTime{
			info: BackupInfo{
				Name:      e.Name(),
				Kind:      backupKind(e.Name()),
				CreatedAt: st.ModTime().UTC().Format(time.RFC3339),
				SizeBytes: st.Size(),
			},
			mod: st.ModTime(),
		})
	}
	// Newest first by mtime (ns precision), name as tie-break — filenames
	// alone missort same-second "-2" dedupe suffixes.
	sort.Slice(rows, func(i, j int) bool {
		if !rows[i].mod.Equal(rows[j].mod) {
			return rows[i].mod.After(rows[j].mod)
		}
		return rows[i].info.Name > rows[j].info.Name
	})
	out := make([]BackupInfo, len(rows))
	for i, r := range rows {
		out[i] = r.info
	}
	return out, nil
}

// RestoreBackup replaces the live store with the named snapshot. A
// "prerestore" safety snapshot of the CURRENT state is taken first (kept
// with the other prerestore snapshots), so the restore itself is undoable.
// Returns the safety snapshot's info and the prerestore snapshots it pruned.
//
// In-flight requests racing the swap may see transient errors — acceptable
// for a single-user local server; callers (MCP tool description) say so.
func (s *Store) RestoreBackup(name string) (BackupInfo, []string, error) {
	s.backupMu.Lock()
	defer s.backupMu.Unlock()

	if name == "" || filepath.Base(name) != name || !backupNameRe.MatchString(name) {
		return BackupInfo{}, nil, clientErrf("invalid backup name %q", name)
	}
	src := filepath.Join(s.backupsDir(), name)
	if _, err := os.Stat(src); err != nil {
		return BackupInfo{}, nil, clientErrf("backup %q not found", name)
	}

	// Copy the source aside first — the safety backup below prunes the
	// oldest prerestore snapshot, which could be the very file being restored.
	tmp, err := copyToTemp(src)
	if err != nil {
		return BackupInfo{}, nil, err
	}
	defer os.Remove(tmp)

	// A snapshot this build would refuse (one a pre-release or newer build
	// wrote) must not replace a working deck: the swap could not reopen it.
	if err := vetSnapshot(tmp); err != nil {
		if SchemaErrKind(err) != "" {
			return BackupInfo{}, nil, clientErrf("backup %q cannot be restored by this server: %v", name, err)
		}
		return BackupInfo{}, nil, fmt.Errorf("check backup %q: %w", name, err)
	}

	safety, pruned, err := s.createBackupLocked(BackupPreRestore, "")
	if err != nil {
		return BackupInfo{}, nil, fmt.Errorf("pre-restore safety backup: %w", err)
	}

	if err := s.swapDBLocked(tmp); err != nil {
		return safety, pruned, err
	}
	// The database is already restored, so a trash problem must not report
	// the restore as failed: a retry would take another snapshot and rotate
	// out older ones, perhaps the one the learner wanted.
	if err := s.untrashReferencedMedia(); err != nil {
		log.Printf("restore %s: bring back trashed media: %v", name, err)
	}
	// Review undo state is included in the restored database.
	return safety, pruned, nil
}

// swapDBLocked closes the live handle, replaces the DB file with srcPath,
// and reopens it. Schema migrations run during reopen. On failure, it attempts
// to reopen the original database.
func (s *Store) swapDBLocked(srcPath string) error {
	if err := s.q().Close(); err != nil {
		return fmt.Errorf("close live db: %w", err)
	}
	// Drop WAL/SHM sidecars associated with the replaced database.
	_ = os.Remove(s.path + "-wal")
	_ = os.Remove(s.path + "-shm")

	stage := s.path + ".restore-tmp"
	if err := copyFile(srcPath, stage); err != nil {
		s.reopenLocked() // best effort recovery
		return err
	}
	if err := os.Rename(stage, s.path); err != nil {
		_ = os.Remove(stage)
		s.reopenLocked()
		return err
	}
	if err := s.reopenLocked(); err != nil {
		return fmt.Errorf("reopen after restore: %w", err)
	}
	return nil
}

func (s *Store) reopenLocked() error {
	db, err := openDB(s.path)
	if err != nil {
		return err
	}
	s.setDB(db)
	return nil
}

// vetSnapshot runs the open-time schema checks on a snapshot copy without
// migrating it.
func vetSnapshot(path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()
	_, _, err = checkSchema(db)
	return err
}

func copyToTemp(src string) (string, error) {
	f, err := os.CreateTemp("", "yume-restore-*.db")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	f.Close()
	if err := copyFile(src, tmp); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return tmp, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
