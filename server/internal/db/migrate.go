package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
)

//go:embed migrations/*/migration.sql
var migrationFS embed.FS

// BaselineMigration is the migration ensureDb marks as applied on legacy
// installs created with `prisma db push`.
const BaselineMigration = "20260725120000_init"

// ErrSchemaNotEmpty is Prisma's P3005: the database has tables but no
// migration ledger, so it must be baselined first.
var ErrSchemaNotEmpty = errors.New("P3005: the database schema is not empty and has no migration history; baseline it first")

// ErrFailedMigration is Prisma's P3009: a previous migration attempt failed and
// must be resolved before new migrations are applied.
var ErrFailedMigration = errors.New("P3009: migrate found failed migrations in the target database, new migrations will not be applied")

// createLedgerSQL is the exact DDL Prisma uses for its ledger on SQLite.
const createLedgerSQL = `CREATE TABLE "_prisma_migrations" (
    "id"                    TEXT PRIMARY KEY NOT NULL,
    "checksum"              TEXT NOT NULL,
    "finished_at"           DATETIME,
    "migration_name"        TEXT NOT NULL,
    "logs"                  TEXT,
    "rolled_back_at"        DATETIME,
    "started_at"            DATETIME NOT NULL DEFAULT current_timestamp,
    "applied_steps_count"   INTEGER UNSIGNED NOT NULL DEFAULT 0
)`

// Migration is one embedded migrations/<name>/migration.sql file.
type Migration struct {
	Name     string
	SQL      string
	Checksum string
}

// Migrations returns the embedded migrations in application order.
func Migrations() ([]Migration, error) {
	dirs, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}
	out := make([]Migration, 0, len(dirs))
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		body, err := migrationFS.ReadFile(path.Join("migrations", d.Name(), "migration.sql"))
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", d.Name(), err)
		}
		sum := sha256.Sum256(body)
		out = append(out, Migration{Name: d.Name(), SQL: string(body), Checksum: hex.EncodeToString(sum[:])})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// TableExists reports whether a table with this name exists.
func TableExists(ctx context.Context, q Querier, name string) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("check table %s: %w", name, err)
	}
	return n > 0, nil
}

// ColumnExists reports whether table has a column with this name.
func ColumnExists(ctx context.Context, q Querier, table, column string) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("check column %s.%s: %w", table, column, err)
	}
	return n > 0, nil
}

// Querier is the subset of *sql.DB, *sql.Conn and *sql.Tx the helpers need.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type ledgerRow struct {
	name       string
	finished   bool
	rolledBack bool
}

func readLedger(ctx context.Context, q Querier) ([]ledgerRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT "migration_name", "finished_at" IS NOT NULL, "rolled_back_at" IS NOT NULL FROM "_prisma_migrations" ORDER BY "started_at" ASC`)
	if err != nil {
		return nil, fmt.Errorf("read _prisma_migrations: %w", err)
	}
	defer rows.Close()
	var out []ledgerRow
	for rows.Next() {
		var r ledgerRow
		if err := rows.Scan(&r.name, &r.finished, &r.rolledBack); err != nil {
			return nil, fmt.Errorf("scan _prisma_migrations: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read _prisma_migrations: %w", err)
	}
	return out, nil
}

// MigrateResult lists the migrations applied by a Migrate call.
type MigrateResult struct {
	Applied []string
}

// Migrate behaves like `prisma migrate deploy` (the ledger format is Prisma's): it creates the ledger on an
// empty database (refusing a non-empty schema without one, P3005), refuses to
// continue past a failed migration (P3009), and applies each embedded
// migration whose name is not recorded as finished. Each attempt is recorded
// like Prisma does: a row with started_at before the script runs, then
// finished_at and applied_steps_count = 1 on success, or the error text in
// logs (finished_at NULL) on failure.
func (d *DB) Migrate(ctx context.Context) (MigrateResult, error) {
	var res MigrateResult
	migrations, err := Migrations()
	if err != nil {
		return res, err
	}
	conn, err := d.Writer().Conn(ctx)
	if err != nil {
		return res, fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Close()

	hasLedger, err := TableExists(ctx, conn, "_prisma_migrations")
	if err != nil {
		return res, err
	}
	if !hasLedger {
		var tables int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`).Scan(&tables); err != nil {
			return res, fmt.Errorf("inspect schema: %w", err)
		}
		if tables > 0 {
			return res, ErrSchemaNotEmpty
		}
		if _, err := conn.ExecContext(ctx, createLedgerSQL); err != nil {
			return res, fmt.Errorf("create _prisma_migrations: %w", err)
		}
	}

	ledger, err := readLedger(ctx, conn)
	if err != nil {
		return res, err
	}
	applied := map[string]bool{}
	for _, r := range ledger {
		if r.rolledBack {
			continue
		}
		if !r.finished {
			return res, fmt.Errorf("%w: the `%s` migration failed", ErrFailedMigration, r.name)
		}
		applied[r.name] = true
	}

	for _, m := range migrations {
		if applied[m.Name] {
			continue
		}
		if err := applyMigration(ctx, conn, m); err != nil {
			return res, err
		}
		res.Applied = append(res.Applied, m.Name)
	}
	// A migration script may toggle foreign_keys; leave the connection enforcing them.
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		return res, fmt.Errorf("re-enable foreign keys: %w", err)
	}
	return res, nil
}

func applyMigration(ctx context.Context, conn *sql.Conn, m Migration) error {
	id := NewID()
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO "_prisma_migrations" ("id", "checksum", "migration_name", "started_at", "applied_steps_count") VALUES (?, ?, ?, ?, 0)`,
		id, m.Checksum, m.Name, Now()); err != nil {
		return fmt.Errorf("record migration %s start: %w", m.Name, err)
	}
	if _, execErr := conn.ExecContext(ctx, m.SQL); execErr != nil {
		logs := fmt.Sprintf("A migration failed to apply. New migrations cannot be applied before the error is recovered from.\n\nMigration name: %s\n\nDatabase error:\n%v", m.Name, execErr)
		if _, err := conn.ExecContext(ctx, `UPDATE "_prisma_migrations" SET "logs" = ? WHERE "id" = ?`, logs, id); err != nil {
			return errors.Join(fmt.Errorf("apply migration %s: %w", m.Name, execErr), fmt.Errorf("record migration failure: %w", err))
		}
		return fmt.Errorf("apply migration %s: %w", m.Name, execErr)
	}
	if _, err := conn.ExecContext(ctx,
		`UPDATE "_prisma_migrations" SET "finished_at" = ?, "applied_steps_count" = 1 WHERE "id" = ?`, Now(), id); err != nil {
		return fmt.Errorf("record migration %s finish: %w", m.Name, err)
	}
	return nil
}

// MarkApplied behaves like `prisma migrate resolve --applied <name>`: it
// creates the ledger if needed and records the migration as applied without
// running it (started_at = finished_at = now, empty logs, applied_steps_count 0).
func (d *DB) MarkApplied(ctx context.Context, name string) error {
	migrations, err := Migrations()
	if err != nil {
		return err
	}
	var m *Migration
	for i := range migrations {
		if migrations[i].Name == name {
			m = &migrations[i]
		}
	}
	if m == nil {
		return fmt.Errorf("migration %s is not embedded", name)
	}
	w := d.Writer()
	hasLedger, err := TableExists(ctx, w, "_prisma_migrations")
	if err != nil {
		return err
	}
	if !hasLedger {
		if _, err := w.ExecContext(ctx, createLedgerSQL); err != nil {
			return fmt.Errorf("create _prisma_migrations: %w", err)
		}
	}
	ledger, err := readLedger(ctx, w)
	if err != nil {
		return err
	}
	for _, r := range ledger {
		if r.name == name && r.finished && !r.rolledBack {
			return fmt.Errorf("migration %s is already recorded as applied", name)
		}
	}
	now := Now()
	if _, err := w.ExecContext(ctx,
		`INSERT INTO "_prisma_migrations" ("id", "checksum", "finished_at", "migration_name", "logs", "started_at", "applied_steps_count") VALUES (?, ?, ?, ?, '', ?, 0)`,
		NewID(), m.Checksum, now, name, now); err != nil {
		return fmt.Errorf("mark migration %s applied: %w", name, err)
	}
	return nil
}
