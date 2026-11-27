package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"modernc.org/sqlite"

	"docklift/internal/db"
)

// restorer is the online-backup API of the modernc driver connection
// (sqlite3_backup_* with the live connection as destination).
type restorer interface {
	NewRestore(srcURI string) (*sqlite.Backup, error)
}

// stagedDB describes a database taken out of an archive and prepared for
// the swap.
type stagedDB struct {
	path string
	// migrations is how many migrations Ensure applied to upgrade it.
	migrations int
}

// openSingle opens one plain connection to a database file (no WAL pragma,
// so a rollback-journal file stays one).
func openSingle(path string) (*sql.DB, error) {
	if strings.ContainsRune(path, '?') {
		return nil, fmt.Errorf("database path %q must not contain '?'", path)
	}
	conn, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(10000)")
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	return conn, nil
}

// checkIntegrity runs PRAGMA integrity_check and fails unless it reports "ok".
func checkIntegrity(ctx context.Context, path string) error {
	conn, err := openSingle(path)
	if err != nil {
		return err
	}
	defer conn.Close()
	rows, err := conn.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	var problems []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return err
		}
		if line != "ok" {
			problems = append(problems, line)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(problems) > 0 {
		if len(problems) > 3 {
			problems = append(problems[:3], "...")
		}
		return fmt.Errorf("integrity check failed: %s", strings.Join(problems, "; "))
	}
	return nil
}

// appliedMigrations counts the finished rows of the Prisma ledger (0 when
// there is none).
func appliedMigrations(ctx context.Context, q db.Querier) int {
	ok, err := db.TableExists(ctx, q, "_prisma_migrations")
	if err != nil || !ok {
		return 0
	}
	var n int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM "_prisma_migrations" WHERE "finished_at" IS NOT NULL AND "rolled_back_at" IS NULL`).Scan(&n); err != nil {
		return 0
	}
	return n
}

// prepareStagedDB validates the database extracted from an archive and
// prepares it for the swap, touching nothing live:
//
//  1. PRAGMA integrity_check must pass (a truncated or foreign file fails here);
//  2. the ensureDb sequence (migrations, legacy baseline, repairs) runs on it,
//     so a backup from an older version is upgraded before it goes live and a
//     schema this binary cannot migrate is refused before anything changed;
//  3. it is rewritten (VACUUM) in rollback-journal mode with the live page
//     size, which the online-backup API needs to copy into a WAL database.
func prepareStagedDB(ctx context.Context, path string, livePageSize int, log *slog.Logger) (stagedDB, error) {
	if err := checkIntegrity(ctx, path); err != nil {
		return stagedDB{}, err
	}
	d, err := db.Open(ctx, path)
	if err != nil {
		return stagedDB{}, err
	}
	before := appliedMigrations(ctx, d.Writer())
	if err := d.Ensure(ctx, log); err != nil {
		return stagedDB{}, errors.Join(err, d.Close())
	}
	after := appliedMigrations(ctx, d.Writer())
	if err := d.Close(); err != nil {
		return stagedDB{}, err
	}
	conn, err := openSingle(path)
	if err != nil {
		return stagedDB{}, err
	}
	defer conn.Close()
	for _, stmt := range []string{
		"PRAGMA journal_mode=DELETE",
		fmt.Sprintf("PRAGMA page_size=%d", livePageSize),
		"VACUUM",
	} {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return stagedDB{}, fmt.Errorf("%s: %w", stmt, err)
		}
	}
	return stagedDB{path: path, migrations: max(0, after-before)}, nil
}

// pageSize is PRAGMA page_size of the live database.
func pageSize(ctx context.Context, d *db.DB) (int, error) {
	var n int
	err := d.Writer().QueryRowContext(ctx, "PRAGMA page_size").Scan(&n)
	return n, err
}

// createPreRestore writes the rollback copy of the live database:
// VACUUM INTO a temporary file (a consistent snapshot, WAL included), fsync,
// then rename over <db>.pre-restore. A crash at any point leaves either the
// previous .pre-restore or the complete new one, never a torn copy.
func createPreRestore(ctx context.Context, d *db.DB, paths Paths) error {
	tmp := paths.PreRestorePath() + preRestoreTempSuffix
	removeSnapshot(tmp)
	if err := vacuumInto(ctx, d, tmp); err != nil {
		removeSnapshot(tmp)
		return fmt.Errorf("copy current database: %w", err)
	}
	if err := syncFile(tmp); err != nil {
		removeSnapshot(tmp)
		return err
	}
	if err := os.Rename(tmp, paths.PreRestorePath()); err != nil {
		removeSnapshot(tmp)
		return err
	}
	syncDir(filepath.Dir(paths.PreRestorePath()))
	return nil
}

// restoreInto replaces the content of the live database with the database
// file at src through SQLite's online-backup API, on the single writer
// connection:
//
//   - holding the writer connection stops every other writer of this
//     process for the duration (they queue on the pool);
//   - the copy is one write transaction on the live database: in WAL mode it
//     commits with a single commit frame, so a crash or power loss before
//     that frame leaves the old database and after it the new one — never a
//     mix, and no -wal/-shm file has to be deleted by hand;
//   - the backup API bumps the schema cookie, so the open reader connections
//     re-read the schema on their next statement: no handle is closed and no
//     goroutine can observe a closed pool (DB.Reopen is not needed).
//
// A checkpoint afterwards folds the WAL into the main file.
func restoreInto(ctx context.Context, d *db.DB, src string) error {
	st, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() || st.Size() == 0 {
		return fmt.Errorf("%s is not a database file", src)
	}
	conn, err := d.Writer().Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Raw(func(dc any) error {
		r, ok := dc.(restorer)
		if !ok {
			return errors.New("sqlite driver does not support online restore")
		}
		b, err := r.NewRestore(src)
		if err != nil {
			return err
		}
		_, stepErr := b.Step(-1)
		return errors.Join(stepErr, b.Finish())
	})
	if err != nil {
		return err
	}
	_, _ = conn.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	return nil
}

// checkpoint folds the WAL into the main database file (best effort).
func checkpoint(ctx context.Context, d *db.DB) {
	_, _ = d.Writer().ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
}

// preRestoreExists reports whether a usable rollback copy is on disk.
func preRestoreExists(paths Paths) bool {
	st, err := os.Stat(paths.PreRestorePath())
	return err == nil && st.Mode().IsRegular() && st.Size() > 0
}

// fileSize is the size of p, 0 when it cannot be read.
func fileSize(p string) int64 {
	st, err := os.Stat(p)
	if err != nil {
		return 0
	}
	return st.Size()
}
