package backup

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func mode(t *testing.T, p string) os.FileMode {
	t.Helper()
	fi, err := os.Lstat(p)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode().Perm()
}

// Backups and the restore staging directory are private; files of earlier
// versions are tightened at boot without touching the nginx/letsencrypt
// areas that nginx-proxy and certbot read.
func TestPrivateModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file modes")
	}
	e := newEnv(t)
	e.seed("src")
	archive := e.backup()
	if got := mode(t, archive); got != 0o600 {
		t.Errorf("backup archive mode %v, want 0600", got)
	}
	if got := mode(t, e.paths.BackupDir); got != 0o700 {
		t.Errorf("backup directory mode %v, want 0700", got)
	}

	// An install of an earlier version: everything world-readable.
	if err := os.MkdirAll(e.paths.UploadsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	upload := filepath.Join(e.paths.UploadsDir(), "restore-1-ab.zip")
	secrets := filepath.Join(e.paths.DataPath, ".secrets")
	conf := filepath.Join(e.paths.NginxConfPath, "service-src.conf")
	for _, f := range []string{upload, secrets, e.paths.DatabasePath + ".pre-restore"} {
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{archive, upload, secrets, e.paths.GitHubKeyPath, e.paths.DatabasePath, e.paths.DatabasePath + ".pre-restore"} {
		if err := os.Chmod(f, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range []string{e.paths.DataPath, e.paths.BackupDir, e.paths.UploadsDir()} {
		if err := os.Chmod(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	TightenModes(e.paths, quiet())
	for _, f := range []string{archive, upload, secrets, e.paths.GitHubKeyPath, e.paths.DatabasePath, e.paths.DatabasePath + ".pre-restore"} {
		if got := mode(t, f); got != 0o600 {
			t.Errorf("%s mode %v, want 0600", filepath.Base(f), got)
		}
	}
	for _, d := range []string{e.paths.DataPath, e.paths.BackupDir, e.paths.UploadsDir()} {
		if got := mode(t, d); got != 0o700 {
			t.Errorf("%s mode %v, want 0700", d, got)
		}
	}
	if got := mode(t, conf); got != 0o644 {
		t.Errorf("nginx conf must stay readable by nginx-proxy: %v", got)
	}
}

// The extraction directory of a restore is private while it exists.
func TestRestoreStagingIsPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file modes")
	}
	f := newFixture(t)
	var seen os.FileMode
	f.dst.r.hook = func(stage Stage) error {
		if stage != StageExtract {
			return nil
		}
		matches, _ := filepath.Glob(filepath.Join(f.dst.paths.BackupDir, tempRestorePrefix+"*"))
		if len(matches) == 1 {
			seen = mode(t, matches[0])
		}
		return nil
	}
	if outcome, log := f.dst.restore(f.archive, Request{Kind: KindBackup}); outcome != OutcomeCommitted {
		t.Fatalf("outcome %v:\n%s", outcome, log)
	}
	if seen != 0o700 {
		t.Fatalf("restore staging directory mode %v, want 0700", seen)
	}
}
