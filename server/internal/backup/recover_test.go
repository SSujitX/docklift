package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"docklift/internal/maintenance"
)

func testPaths(t *testing.T) Paths {
	dir := t.TempDir()
	return Paths{
		BackupDir:    filepath.Join(dir, "backups"),
		DataPath:     filepath.Join(dir, "data"),
		DatabasePath: filepath.Join(dir, "data", "docklift.db"),
	}
}

func TestRecoverInterruptedSeals(t *testing.T) {
	p := testPaths(t)
	started := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := writeMarker(p, markerFor(p, "b.zip", string(StageDeployments), started)); err != nil {
		t.Fatal(err)
	}
	st := maintenance.New(p.DataPath, quiet())
	RecoverInterrupted(p, st, quiet())
	noFile(t, markerPath(p))
	seal := st.ReadCritical()
	if !st.Critical() || seal == nil || !st.Active() || st.TryAcquireRestoreLock("x") {
		t.Fatalf("not sealed: %+v", seal)
	}
	for _, part := range []string{"interrupted during deployments", "2026-01-02T03:04:05.000Z", "source b.zip", p.PreRestorePath()} {
		if !strings.Contains(seal.Detail, part) {
			t.Fatalf("seal detail %q lacks %q", seal.Detail, part)
		}
	}
	if seal.LiveDBPath != p.DatabasePath || seal.PreRestorePath != p.PreRestorePath() {
		t.Fatalf("seal paths %+v", seal)
	}
	// The boot after that: no marker, the seal alone keeps the instance sealed.
	again := maintenance.New(p.DataPath, quiet())
	again.LoadCriticalOnBoot()
	RecoverInterrupted(p, again, quiet())
	if !again.Critical() || again.ReadCritical().Detail != seal.Detail {
		t.Fatal("seal must persist unchanged")
	}
}

func TestRecoverInterruptedKeepsExistingSeal(t *testing.T) {
	p := testPaths(t)
	st := maintenance.New(p.DataPath, quiet())
	if err := st.EnterCritical("rollback failed", p.DatabasePath); err != nil {
		t.Fatal(err)
	}
	if err := writeMarker(p, markerFor(p, "b.zip", string(StageNginx), time.Now())); err != nil {
		t.Fatal(err)
	}
	boot := maintenance.New(p.DataPath, quiet())
	boot.LoadCriticalOnBoot()
	RecoverInterrupted(p, boot, quiet())
	noFile(t, markerPath(p))
	if got := boot.ReadCritical(); got == nil || got.Detail != "rollback failed" {
		t.Fatalf("the more specific seal must win: %+v", got)
	}
}

func TestRecoverInterruptedWithoutMarker(t *testing.T) {
	p := testPaths(t)
	st := maintenance.New(p.DataPath, quiet())
	RecoverInterrupted(p, st, quiet())
	if st.Critical() || st.Active() || st.RestoreLocked() {
		t.Fatal("a clean boot must not seal")
	}
}

func TestCleanupOnBoot(t *testing.T) {
	p := testPaths(t)
	mk := func(rel string, dir bool) string {
		full := filepath.Join(filepath.Dir(p.DataPath), filepath.FromSlash(rel))
		if dir {
			if err := os.MkdirAll(filepath.Join(full, "x"), 0o755); err != nil {
				t.Fatal(err)
			}
			return full
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		return full
	}
	gone := []string{
		mk("backups/temp-restore-123", true),
		mk("backups/.a.zip.0011.partial", false),
		mk("backups/uploads/restore-1-ab.zip.partial", false),
		mk("data/.docklift-backup-1-2.db", false),
		mk("data/.docklift-backup-1-2.db-wal", false),
		mk("data/docklift.db.pre-restore.tmp", false),
		mk("data/docklift.db.pre-restore.tmp-journal", false),
		mk("data/..restore-in-progress.tmp-123", false),
	}
	kept := []string{
		mk("backups/a.zip", false),
		mk("backups/uploads/restore-1-ab.zip", false),
		mk("backups/temp-restore-file", false),
		mk("data/docklift.db", false),
		mk("data/docklift.db.pre-restore", false),
		mk("data/.restore-critical", false),
	}
	CleanupOnBoot(p, quiet())
	for _, g := range gone {
		noFile(t, g)
	}
	for _, k := range kept {
		if _, err := os.Stat(k); err != nil {
			t.Fatalf("%s must be kept: %v", k, err)
		}
	}
}
