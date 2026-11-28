package backup

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

// Modes of everything that holds the database, secrets or backups (which
// contain both): readable by the backend only. The nginx and letsencrypt
// areas keep their modes (nginx-proxy and certbot read them through their own
// mounts), except the letsencrypt private keys, which are owner-only.
const (
	privateFile fs.FileMode = 0o600
	privateDir  fs.FileMode = 0o700
)

// mkPrivateDir creates dir (0700) and tightens an existing one.
func mkPrivateDir(dir string) error {
	if err := os.MkdirAll(dir, privateDir); err != nil {
		return err
	}
	return os.Chmod(dir, privateDir)
}

// TightenModes makes the files of earlier versions private at boot: the data
// and backup directories become 0700 and every regular file directly inside
// them (database and its -wal/-shm/.pre-restore, .secrets, setup tokens,
// restore markers, backup and uploaded archives) and the GitHub App key
// become 0600. Links are never followed and failures are only logged.
func TightenModes(p Paths, log *slog.Logger) {
	dirs := []string{p.DataPath, filepath.Dir(p.DatabasePath), p.BackupDir, p.UploadsDir()}
	files := []string{p.GitHubKeyPath}
	for _, suffix := range []string{"", "-wal", "-shm", "-journal", ".pre-restore"} {
		files = append(files, p.DatabasePath+suffix)
	}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		fi, err := os.Lstat(dir)
		if err != nil || !fi.IsDir() {
			continue
		}
		if fi.Mode().Perm() != privateDir {
			if err := os.Chmod(dir, privateDir); err != nil {
				log.Warn("could not tighten directory mode", "path", dir, "error", err)
			}
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			log.Warn("could not list directory", "path", dir, "error", err)
			continue
		}
		for _, e := range entries {
			if e.Type().IsRegular() {
				files = append(files, filepath.Join(dir, e.Name()))
			}
		}
	}
	for _, f := range files {
		if f == "" {
			continue
		}
		fi, err := os.Lstat(f)
		if err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm() == privateFile {
			continue
		}
		if err := os.Chmod(f, privateFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
			log.Warn("could not tighten file mode", "path", f, "error", err)
		}
	}
	tightenCertKeys(p.LetsEncryptPath, log)
}

// tightenCertKeys removes group/other access from the certificate and ACME
// account private keys below the letsencrypt directory. Restores by 2.1.6
// (and by this version before restoreModes) wrote them 0644; certbot itself
// creates them owner-only, and nginx-proxy reads them as root. Only the key
// files are touched; links are not followed.
func tightenCertKeys(dir string, log *slog.Logger) {
	if dir == "" {
		return
	}
	for _, sub := range []string{"archive", "accounts", "keys"} {
		root := filepath.Join(dir, sub)
		if fi, err := os.Lstat(root); err != nil || !fi.IsDir() {
			continue
		}
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || !d.Type().IsRegular() {
				return nil
			}
			rel, err := filepath.Rel(dir, p)
			if err != nil || !isLetsEncryptKey(filepath.ToSlash(rel)) {
				return nil
			}
			fi, err := d.Info()
			if err != nil || fi.Mode().Perm()&0o077 == 0 {
				return nil
			}
			if err := os.Chmod(p, fi.Mode().Perm()&^0o077); err != nil {
				log.Warn("could not tighten certificate key mode", "path", p, "error", err)
			}
			return nil
		})
	}
}
