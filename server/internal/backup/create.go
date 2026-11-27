package backup

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"docklift/internal/db"
)

// Creator writes backup archives into Paths.BackupDir.
type Creator struct {
	Paths Paths
	DB    *db.DB
	Log   *slog.Logger
	// Now is the clock (time.Now when nil).
	Now func() time.Time
}

func (c *Creator) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Create is POST /api/backup/create: it writes the progress log to out and
// returns the new backup's file name, or "" when the backup failed (the
// failure is in the log as "[ERROR] ..." — the response status stays 200,
// as in 2.1.6).
//
// The archive is written to a hidden ".<name>.<random>.partial" file, fsynced
// and renamed into place, so a crash or full disk never leaves a truncated
// .zip that the list, download or restore routes would offer.
func (c *Creator) Create(ctx context.Context, label string, out func(string)) string {
	name, err := c.create(ctx, label, out)
	if err != nil {
		out("\n[ERROR] Backup failed: " + err.Error() + "\n")
		c.Log.Error("Backup creation error", "error", err)
		return ""
	}
	return name
}

// LogFailure writes the failure line of a backup that could not start.
func LogFailure(out func(string), err error) {
	out("\n[ERROR] Backup failed: " + err.Error() + "\n")
}

func (c *Creator) create(ctx context.Context, label string, out func(string)) (_ string, err error) {
	if err := os.MkdirAll(c.Paths.BackupDir, 0o755); err != nil {
		return "", err
	}
	now := c.now()
	filename := BackupFilename(label, now)
	finalPath := filepath.Join(c.Paths.BackupDir, filename)

	out("\n" + rule + "\n")
	out("  CREATING BACKUP\n")
	out("  " + ISO(now) + "\n")
	out(rule + "\n\n")

	suffix := make([]byte, 4)
	_, _ = rand.Read(suffix)
	partial := filepath.Join(c.Paths.BackupDir, "."+filename+"."+hex.EncodeToString(suffix)+partialSuffix)
	f, err := os.OpenFile(partial, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	committed := false
	defer func() {
		if !committed {
			_ = f.Close()
			_ = os.Remove(partial)
		}
	}()
	buf := bufio.NewWriterSize(f, 1<<20)
	aw := newArchiveWriter(buf, out)

	// 1. Database: a consistent snapshot, never the live file (a raw copy
	// can mix pages of different transactions, and misses the WAL).
	out("[1/4] Adding database...\n")
	if !exists(c.Paths.DatabasePath) {
		out("\n[ERROR] Database not found at " + c.Paths.DatabasePath + " — refusing incomplete backup\n")
		return "", nil
	}
	snap, err := SnapshotDatabase(ctx, c.DB, c.Paths.DatabasePath)
	if err != nil {
		return "", err
	}
	defer removeSnapshot(snap)
	st, err := os.Stat(snap)
	if err != nil {
		return "", err
	}
	out("      + docklift.db (" + FormatBytes(st.Size()) + ") [consistent snapshot]\n")
	if err := aw.addFile(ctx, snap, entryDatabase); err != nil {
		return "", err
	}

	// 2-4. Directory trees.
	trees := []struct {
		header, root, prefix, added, missing string
	}{
		{"\n[2/4] Adding deployments...\n", c.Paths.DeploymentsPath, entryDeployments, "/deployments/", "      ! Deployments directory not found\n"},
		{"\n[3/5] Adding nginx configs...\n", c.Paths.NginxConfPath, entryNginx, "/nginx-conf/", "      - Nginx config directory not found (skipped)\n"},
		{"\n[4/5] Adding TLS certificates...\n", c.Paths.LetsEncryptPath, entryLetsEncrypt, "/letsencrypt/", "      - No Let's Encrypt data (skipped)\n"},
	}
	for _, t := range trees {
		out(t.header)
		if !exists(t.root) {
			out(t.missing)
			continue
		}
		out("      + " + t.added + " (" + FormatBytes(treeSize(t.root)) + ")\n")
		if err := aw.addDir(ctx, t.root, t.prefix); err != nil {
			return "", err
		}
	}

	// 5. GitHub App key.
	out("\n[5/5] Adding GitHub App key...\n")
	if exists(c.Paths.GitHubKeyPath) {
		if err := aw.addFile(ctx, c.Paths.GitHubKeyPath, entryGitHubKey); err != nil {
			return "", err
		}
		out("      + github-app.pem\n")
	} else {
		out("      - No GitHub App key configured (skipped)\n")
	}

	out("\nFinalizing backup...\n")
	if err := aw.Close(); err != nil {
		return "", err
	}
	if err := buf.Flush(); err != nil {
		return "", err
	}
	if err := f.Sync(); err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(partial, finalPath); err != nil {
		_ = os.Remove(partial)
		return "", err
	}
	committed = true
	syncDir(c.Paths.BackupDir)

	final, err := os.Stat(finalPath)
	if err != nil {
		return "", err
	}
	out("\n" + rule + "\n")
	out("  BACKUP COMPLETE\n")
	out(rule + "\n")
	out("  File: " + filename + "\n")
	out("  Size: " + FormatBytes(final.Size()) + "\n")
	out("  Location: " + c.Paths.BackupDir + "\n")
	return filename, nil
}

// SnapshotDatabase writes a consistent copy of the live database next to it
// with VACUUM INTO (safe under WAL and concurrent writers: it reads one
// snapshot of the database, WAL included) and returns its path. The caller
// removes it with removeSnapshot.
func SnapshotDatabase(ctx context.Context, d *db.DB, livePath string) (string, error) {
	p := filepath.Join(filepath.Dir(livePath),
		snapshotPrefix+strconv.FormatInt(time.Now().UnixMilli(), 10)+"-"+strconv.Itoa(os.Getpid())+".db")
	removeSnapshot(p)
	if err := vacuumInto(ctx, d, p); err != nil {
		removeSnapshot(p)
		return "", fmt.Errorf("snapshot database: %w", err)
	}
	return p, nil
}

// vacuumInto runs VACUUM INTO on the writer connection (the reader pool is
// query_only, which VACUUM INTO refuses).
func vacuumInto(ctx context.Context, d *db.DB, dest string) error {
	_, err := d.Writer().ExecContext(ctx, "VACUUM INTO ?", dest)
	return err
}

func removeSnapshot(p string) {
	for _, s := range []string{"", "-journal", "-wal", "-shm"} {
		_ = os.Remove(p + s)
	}
}
