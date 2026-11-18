package srs

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackupRestoreLifecycle(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "cards.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// World A: one word.
	if err := s.SetWordStatus("結局", 1); err != nil {
		t.Fatal(err)
	}
	info, pruned, err := s.CreateBackup("world-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(pruned) != 0 || !strings.Contains(info.Name, "world-a") || info.SizeBytes == 0 {
		t.Fatalf("backup info: %+v pruned=%v", info, pruned)
	}

	// World B: mutate — add a word, learn the old one.
	if err := s.SetWordStatus("結局", 2); err != nil {
		t.Fatal(err)
	}
	if err := s.SetWordStatus("新規", 1); err != nil {
		t.Fatal(err)
	}

	// Restore world A.
	safety, pruned, err := s.RestoreBackup(info.Name)
	if err != nil {
		t.Fatal(err)
	}
	if safety.Kind != BackupPreRestore || !strings.HasPrefix(safety.Name, "cards-prerestore-") || len(pruned) != 0 {
		t.Fatalf("expected prerestore safety snapshot, got %+v pruned=%v", safety, pruned)
	}
	rows, err := s.GetWordStatuses([]string{"結局", "新規"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Lemma != "結局" || rows[0].Status != 1 {
		t.Fatalf("world A not restored: %+v", rows)
	}

	// The safety snapshot holds world B — restoring IT undoes the restore.
	if _, _, err := s.RestoreBackup(safety.Name); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.GetWordStatuses([]string{"結局", "新規"})
	if len(rows) != 2 {
		t.Fatalf("world B not restored via safety snapshot: %+v", rows)
	}

	// Store still fully usable after two swaps (write + nested-query read).
	if _, err := s.SaveCard(SaveCardReq{Lemma: "確認"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetDueCards(5); err != nil {
		t.Fatal(err)
	}
}

func TestBackupRotationCap(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "cards.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	names := make([]string, 0, MaxBackups+2)
	for i := 0; i < MaxBackups+2; i++ {
		info, _, err := s.CreateBackup("")
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, info.Name)
		time.Sleep(20 * time.Millisecond) // distinct mtimes not required; names dedupe
	}
	list, err := s.ListBackups()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != MaxBackups {
		t.Fatalf("kept %d backups, want %d", len(list), MaxBackups)
	}
	// Newest-first, and the survivors are the most recent creations.
	if list[0].Name != names[len(names)-1] {
		t.Fatalf("newest first: got %q want %q", list[0].Name, names[len(names)-1])
	}
	for _, kept := range list {
		if kept.Name == names[0] || kept.Name == names[1] {
			t.Fatalf("oldest snapshots should have been pruned, found %q", kept.Name)
		}
	}

	// Bogus restore names rejected.
	for _, bad := range []string{"", "../../etc/passwd", "nope.db", "cards-x.db"} {
		if _, _, err := s.RestoreBackup(bad); err == nil {
			t.Fatalf("restore %q should fail", bad)
		}
	}
}

// The audit's sequence: a tutor's safety backup, then imports and a restore.
// Automatic snapshots keep their own retention, so the learner's backup and
// the restore's safety snapshot survive any number of imports, and each
// import names the snapshot that undoes it.
func TestAutomaticSnapshotsNeverPruneManualOnes(t *testing.T) {
	s := openTestStore(t)
	mine, _, err := s.CreateBackup("probe-start")
	if err != nil {
		t.Fatal(err)
	}
	var imports []ImportCSVResp
	importOne := func(tag string) {
		t.Helper()
		resp, err := s.ImportCSV(ImportCSVReq{Rows: []ImportRow{{Lemma: tag}}, Tag: tag})
		if err != nil {
			t.Fatal(err)
		}
		if resp.Backup == "" || !strings.HasPrefix(resp.Backup, "cards-import-") || filepath.Base(resp.BackupPath) != resp.Backup {
			t.Fatalf("import must name its snapshot: %+v", resp)
		}
		imports = append(imports, resp)
	}
	importOne("known-list")
	safety, _, err := s.RestoreBackup(imports[0].Backup)
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"learning-list", "small-a", "small-b"} {
		importOne(tag)
	}

	list, err := s.ListBackups()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]string{}
	count := map[string]int{}
	for _, b := range list {
		byName[b.Name] = b.Kind
		count[b.Kind]++
	}
	if byName[mine.Name] != BackupManual || byName[safety.Name] != BackupPreRestore {
		t.Fatalf("manual or prerestore snapshot pruned by imports: %+v", list)
	}
	if count[BackupPreImport] != MaxBackups {
		t.Fatalf("kept %d pre-import snapshots, want %d: %+v", count[BackupPreImport], MaxBackups, list)
	}
	// The fourth import replaced the first import's snapshot, and says so.
	last := imports[len(imports)-1]
	if len(last.PrunedBackups) != 1 || last.PrunedBackups[0] != imports[0].Backup {
		t.Fatalf("pruned_backups = %v, want [%s]", last.PrunedBackups, imports[0].Backup)
	}
	if imports[1].PrunedBackups == nil || len(imports[1].PrunedBackups) != 0 {
		t.Fatalf("an import that pruned nothing reports []: %#v", imports[1].PrunedBackups)
	}

	// Restoring the newest import's snapshot undoes exactly that batch.
	if _, _, err := s.RestoreBackup(last.Backup); err != nil {
		t.Fatal(err)
	}
	rows, _ := s.GetWordStatuses([]string{"small-a", "small-b"})
	if len(rows) != 1 || rows[0].Lemma != "small-a" {
		t.Fatalf("after undoing the last batch: %+v", rows)
	}
}

// Snapshots written before kinds existed keep a sensible kind: the old
// "-prerestore" label stays prerestore, everything else counts as manual.
func TestLegacySnapshotNamesKeepTheirKind(t *testing.T) {
	s := openTestStore(t)
	info, _, err := s.CreateBackup("")
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(s.backupsDir(), info.Name)
	for _, name := range []string{
		"cards-20260901-120000.000-prerestore.db",
		"cards-20260901-120000.000-prerestore-2.db",
		"cards-20260901-120001.000-anki-2026-09.db",
		"cards-20260901-120002.000-before-prerestore.db",
	} {
		if err := copyFile(src, filepath.Join(s.backupsDir(), name)); err != nil {
			t.Fatal(err)
		}
	}
	want := map[string]string{
		"cards-20260901-120000.000-prerestore.db":        BackupPreRestore,
		"cards-20260901-120000.000-prerestore-2.db":      BackupPreRestore,
		"cards-20260901-120001.000-anki-2026-09.db":      BackupManual,
		"cards-20260901-120002.000-before-prerestore.db": BackupManual,
		info.Name: BackupManual,
	}
	list, err := s.ListBackups()
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range list {
		if want[b.Name] != b.Kind {
			t.Errorf("%s: kind %q, want %q", b.Name, b.Kind, want[b.Name])
		}
	}
	if len(list) != len(want) {
		t.Fatalf("listed %d, want %d: %+v", len(list), len(want), list)
	}
}
