// Package db opens the Docklift SQLite database, applies the embedded Prisma
// migrations (keeping Prisma's _prisma_migrations ledger) and provides the
// value codecs that read and write Prisma's on-disk encodings.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "modernc.org/sqlite" // registers the pure-Go "sqlite" driver
)

// busyTimeoutMS is how long a connection waits on a locked database before
// failing with SQLITE_BUSY.
const busyTimeoutMS = 10000

// DB holds the two connection pools used for one SQLite file: a single-
// connection writer (SQLite allows one writer; serialising in-process avoids
// SQLITE_BUSY between our own goroutines) and a reader pool for concurrent
// queries. Reopen swaps both, which the restore flow needs after replacing the
// database file (the port of reconnectPrisma in backend/src/lib/prisma.ts).
type DB struct {
	path string

	mu     sync.RWMutex
	writer *sql.DB
	reader *sql.DB
}

// Open opens (creating if needed) the SQLite database at path and verifies
// that it is reachable.
func Open(ctx context.Context, path string) (*DB, error) {
	if strings.ContainsRune(path, '?') {
		return nil, fmt.Errorf("database path %q must not contain '?'", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	d := &DB{path: path}
	if err := d.open(ctx); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *DB) open(ctx context.Context) error {
	common := fmt.Sprintf("_pragma=busy_timeout(%d)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)", busyTimeoutMS)
	writer, err := sql.Open("sqlite", d.path+"?"+common+"&_txlock=immediate")
	if err != nil {
		return fmt.Errorf("open sqlite writer: %w", err)
	}
	writer.SetMaxOpenConns(1)
	writer.SetMaxIdleConns(1)
	writer.SetConnMaxLifetime(0)
	writer.SetConnMaxIdleTime(0)
	if err := writer.PingContext(ctx); err != nil {
		_ = writer.Close()
		return fmt.Errorf("open sqlite database %s: %w", d.path, err)
	}

	reader, err := sql.Open("sqlite", d.path+"?"+common+"&_pragma=query_only(1)")
	if err != nil {
		_ = writer.Close()
		return fmt.Errorf("open sqlite reader: %w", err)
	}
	reader.SetMaxOpenConns(8)
	reader.SetMaxIdleConns(8)
	if err := reader.PingContext(ctx); err != nil {
		_ = writer.Close()
		_ = reader.Close()
		return fmt.Errorf("open sqlite reader %s: %w", d.path, err)
	}

	d.mu.Lock()
	d.writer, d.reader = writer, reader
	d.mu.Unlock()
	return nil
}

// Path is the SQLite file this handle points at.
func (d *DB) Path() string { return d.path }

// Writer returns the single-connection pool used for writes and transactions.
func (d *DB) Writer() *sql.DB {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.writer
}

// Reader returns the pool used for read-only queries.
func (d *DB) Reader() *sql.DB {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.reader
}

// Close closes both pools.
func (d *DB) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.closeLocked()
}

func (d *DB) closeLocked() error {
	var errs []error
	if d.writer != nil {
		errs = append(errs, d.writer.Close())
	}
	if d.reader != nil {
		errs = append(errs, d.reader.Close())
	}
	d.writer, d.reader = nil, nil
	return errors.Join(errs...)
}

// Reopen closes the current pools and opens fresh ones on the same path. Use it
// after the database file was replaced on disk (restore). Callers must keep
// API traffic out (maintenance mode) while it runs. An error closing the old
// pools is reported only together with a failure to open the new ones; once
// reopened, the old handles are gone either way.
func (d *DB) Reopen(ctx context.Context) error {
	d.mu.Lock()
	closeErr := d.closeLocked()
	d.mu.Unlock()
	if err := d.open(ctx); err != nil {
		return errors.Join(err, closeErr)
	}
	return nil
}
