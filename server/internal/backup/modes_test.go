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

// A restore keeps the modes stored in the archive (2.1.6 and this port
// before restoreModes wrote every file 0644 and directory 0755, so certbot's
// private keys came back world-readable), never grants group/other write,
// and makes certificate private keys owner-only even when the archive
// stored them looser.
func TestRestoreKeepsArchiveModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file modes")
	}
	src := newEnv(t)
	src.seed("src")
	le := src.paths.LetsEncryptPath
	files := []struct {
		rel  string
		perm os.FileMode
	}{
		{"archive/src.test/privkey1.pem", 0o600},
		{"archive/src.test/privkey2.pem", 0o644}, // restored by 2.1.6: loose in the archive
		{"archive/src.test/fullchain1.pem", 0o644},
		{"accounts/acme/directory/abc/private_key.json", 0o400},
		{"accounts/acme/directory/abc/regr.json", 0o644},
	}
	for _, f := range files {
		p := filepath.Join(le, filepath.FromSlash(f.rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, f.perm); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range []string{"accounts", "archive"} {
		if err := os.Chmod(filepath.Join(le, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	script := filepath.Join(src.paths.DeploymentsPath, "run.sh")
	shared := filepath.Join(src.paths.DeploymentsPath, "shared.txt")
	for p, perm := range map[string]os.FileMode{script: 0o755, shared: 0o666} {
		if err := os.WriteFile(p, []byte("x"), perm); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, perm); err != nil {
			t.Fatal(err)
		}
	}
	archive := src.backup()

	dst := newEnv(t)
	dst.seed("dst")
	if outcome, log := dst.restore(archive, Request{Kind: KindBackup}); outcome != OutcomeCommitted {
		t.Fatalf("outcome %v:\n%s", outcome, log)
	}
	dle := dst.paths.LetsEncryptPath
	for rel, want := range map[string]os.FileMode{
		"archive/src.test/privkey1.pem":                0o600,
		"archive/src.test/privkey2.pem":                0o600,
		"archive/src.test/fullchain1.pem":              0o644,
		"accounts/acme/directory/abc/private_key.json": 0o400,
		"accounts/acme/directory/abc/regr.json":        0o644,
		"accounts":                                     0o700,
		"archive":                                      0o700,
		"archive/src.test":                             0o755,
	} {
		if got := mode(t, filepath.Join(dle, filepath.FromSlash(rel))); got != want {
			t.Errorf("letsencrypt/%s mode %v, want %v", rel, got, want)
		}
	}
	if got := mode(t, filepath.Join(dst.paths.DeploymentsPath, "run.sh")); got != 0o755 {
		t.Errorf("run.sh mode %v, want 0755", got)
	}
	if got := mode(t, filepath.Join(dst.paths.DeploymentsPath, "shared.txt")); got != 0o644 {
		t.Errorf("shared.txt mode %v, want 0644 (no group/other write)", got)
	}
}

// Boot tightens the letsencrypt private keys an earlier restore left
// world-readable, and nothing else in that area.
func TestTightenModesCertKeys(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file modes")
	}
	e := newEnv(t)
	le := e.paths.LetsEncryptPath
	want := map[string]os.FileMode{
		"archive/a.test/privkey1.pem":                  0o600,
		"archive/a.test/cert1.pem":                     0o644,
		"keys/0000_key-certbot.pem":                    0o600,
		"accounts/acme/directory/abc/private_key.json": 0o600,
		"accounts/acme/directory/abc/meta.json":        0o644,
		"renewal/a.test.conf":                          0o644,
	}
	for rel := range want {
		p := filepath.Join(le, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	TightenModes(e.paths, quiet())
	for rel, w := range want {
		if got := mode(t, filepath.Join(le, filepath.FromSlash(rel))); got != w {
			t.Errorf("letsencrypt/%s mode %v, want %v", rel, got, w)
		}
	}
}
