// Package backup creates and restores Docklift backup archives: the service
// half of backend/src/routes/backup.ts and the port of
// lib/restoreCommitPolicy.ts. The HTTP handlers live in internal/api/backup;
// the maintenance gate, restore lock and restore-critical seal live in
// internal/maintenance.
//
// Archive layout (identical to 2.1.6 in both directions):
//
//	database/docklift.db   consistent SQLite snapshot (VACUUM INTO)
//	deployments/...        DEPLOYMENTS_PATH, including .docklift/<id>/compose.yml
//	nginx-conf/...         NGINX_CONF_PATH
//	letsencrypt/...        LETSENCRYPT_PATH (live/ symlinks stored as symlink entries)
//	github-app.pem         DOCKLIFT_GITHUB_PRIVATE_KEY_PATH, when present
//
// Directory entries carry a trailing slash, names use forward slashes, files
// are deflated and directories and symlinks stored, with Unix modes in the
// external attributes — the shape archiver 8 (ZipArchive) produced.
package backup

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"docklift/internal/config"
)

// Paths are the directories and files a backup covers.
type Paths struct {
	// BackupDir is BACKUP_PATH (server-created backups; uploads below it).
	BackupDir string
	// DataPath is DATA_PATH (restore markers live there).
	DataPath string
	// DatabasePath is the live SQLite file.
	DatabasePath    string
	DeploymentsPath string
	NginxConfPath   string
	LetsEncryptPath string
	// GitHubKeyPath is DOCKLIFT_GITHUB_PRIVATE_KEY_PATH.
	GitHubKeyPath string
}

// PathsFrom reads the paths from the process configuration.
func PathsFrom(cfg *config.Config) Paths {
	return Paths{
		BackupDir:       cfg.BackupPath,
		DataPath:        cfg.DataPath,
		DatabasePath:    cfg.DatabasePath,
		DeploymentsPath: cfg.DeploymentsPath,
		NginxConfPath:   cfg.NginxConfPath,
		LetsEncryptPath: cfg.LetsEncryptPath,
		GitHubKeyPath:   cfg.GitHubPrivateKeyPath,
	}
}

// UploadsDir is BACKUP_PATH/uploads, where uploaded restore files are kept.
func (p Paths) UploadsDir() string { return filepath.Join(p.BackupDir, "uploads") }

// PreRestorePath is the copy of the live database taken before a restore
// replaces it (kept after the restore for manual rollback).
func (p Paths) PreRestorePath() string { return p.DatabasePath + ".pre-restore" }

// Archive entry names of the top-level areas.
const (
	entryDatabase    = "database/docklift.db"
	entryDeployments = "deployments"
	entryNginx       = "nginx-conf"
	entryLetsEncrypt = "letsencrypt"
	entryGitHubKey   = "github-app.pem"
)

// Work-file names. Everything with these shapes is disposable and removed by
// CleanupOnBoot after a crash.
const (
	// tempRestorePrefix + unix ms is the extraction directory in BACKUP_PATH.
	tempRestorePrefix = "temp-restore-"
	// snapshotPrefix + ms-pid.db is the VACUUM INTO snapshot next to the database.
	snapshotPrefix = ".docklift-backup-"
	// partialSuffix marks a backup or upload that is still being written.
	partialSuffix = ".partial"
	// preRestoreTempSuffix is the .pre-restore copy while it is written.
	preRestoreTempSuffix = ".tmp"
)

// exists is fs.existsSync: the path resolves (links followed).
func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// syncDir flushes a directory entry (rename/create/remove) to disk. Windows
// cannot open directories for syncing; NTFS journals metadata itself.
func syncDir(dir string) {
	f, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = f.Sync()
	_ = f.Close()
}

// syncFile flushes a file's content to disk.
func syncFile(p string) error {
	f, err := os.OpenFile(p, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}

// writeFileDurable writes data to path through a temporary file in the same
// directory, fsyncs it, renames it into place and fsyncs the directory, so
// after a power loss path holds either the old or the new content.
func writeFileDurable(path string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, perm); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	syncDir(dir)
	return nil
}

// copyFileDurable copies src over dst the same way (temporary file, fsync,
// rename), so dst is never left half written.
func copyFileDurable(src, dst string, perm fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	dir := filepath.Dir(dst)
	f, err := os.CreateTemp(dir, "."+filepath.Base(dst)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := io.Copy(f, in); err != nil {
		_ = f.Close()
		return fmt.Errorf("copy %s: %w", filepath.Base(src), err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, perm); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	ok = true
	syncDir(dir)
	return nil
}

// treeSize is the apparent size of everything below root (du -sb): files,
// directories and links by their lstat size. Unreadable parts count as 0.
func treeSize(root string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if fi, ierr := d.Info(); ierr == nil {
			total += fi.Size()
		}
		return nil
	})
	return total
}
