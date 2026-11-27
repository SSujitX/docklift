package db

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// The database and its -wal/-shm files hold secrets: a new database is
// created 0600 (SQLite copies that mode to -wal/-shm) and an existing
// world-readable one is tightened on open.
func TestOpenMakesDatabasePrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file modes")
	}
	ctx := context.Background()
	for _, existing := range []bool{false, true} {
		dir := filepath.Join(t.TempDir(), "data")
		path := filepath.Join(dir, "docklift.db")
		if existing {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			for _, p := range []string{path, path + "-wal", path + "-shm"} {
				if err := os.WriteFile(p, nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
		d, err := Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := d.Writer().ExecContext(ctx, `CREATE TABLE t (x INTEGER); INSERT INTO t VALUES (1)`); err != nil {
			t.Fatal(err)
		}
		for _, p := range []string{path, path + "-wal", path + "-shm"} {
			fi, err := os.Stat(p)
			if err != nil {
				t.Fatal(err)
			}
			if fi.Mode().Perm() != 0o600 {
				t.Errorf("existing=%v: %s mode %v, want 0600", existing, filepath.Base(p), fi.Mode().Perm())
			}
		}
		if !existing {
			if fi, err := os.Stat(dir); err != nil || fi.Mode().Perm() != 0o700 {
				t.Errorf("new database directory mode %v (%v), want 0700", fi.Mode().Perm(), err)
			}
		}
		_ = d.Close()
	}
}
