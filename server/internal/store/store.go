// Package store is the typed query layer over the Docklift SQLite schema
// (backend/prisma/schema.prisma). Every table has a file with its row type,
// whose JSON tags reproduce the keys Prisma returned to API clients (snake_case
// columns, `passwordChangedAt` camelCase, null for NULL, ISO-8601 DateTimes),
// plus the queries the route handlers need.
//
// Writes follow Prisma's semantics: ids are UUID v4 generated here, DateTime
// defaults (`@default(now())`) and `@updatedAt` are set by the store in
// Prisma's integer-milliseconds encoding, and unique-constraint violations
// surface as ErrConflict (Prisma P2002). Updating or deleting a missing row by
// its key returns ErrNotFound (Prisma P2025).
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"docklift/internal/db"
)

// ErrNotFound is returned when a row addressed by its unique key does not exist.
var ErrNotFound = errors.New("store: record not found")

// ErrConflict is returned when a write violates a unique constraint.
var ErrConflict = errors.New("store: unique constraint violation")

// ErrMissingTable is returned when a queried table does not exist (Prisma
// P2021), e.g. on a database that was never migrated or restored.
var ErrMissingTable = errors.New("store: table does not exist")

// Store runs queries against the database, or against one transaction when
// obtained inside InTx.
type Store struct {
	db *db.DB
	tx *sql.Tx
}

// New returns a Store over d.
func New(d *db.DB) *Store { return &Store{db: d} }

// DB exposes the underlying handle (for Reopen after a restore, backups, and
// raw maintenance statements).
func (s *Store) DB() *db.DB { return s.db }

func (s *Store) reader() db.Querier {
	if s.tx != nil {
		return s.tx
	}
	return s.db.Reader()
}

func (s *Store) writer() db.Querier {
	if s.tx != nil {
		return s.tx
	}
	return s.db.Writer()
}

// InTx runs fn inside one write transaction (BEGIN IMMEDIATE) and commits when
// fn returns nil; any error or panic rolls back. The Store passed to fn must be
// used for every query of the transaction: the writer has a single connection,
// so writing through the outer Store from inside fn would wait forever. Calls
// nested inside an existing transaction join it.
func (s *Store) InTx(ctx context.Context, fn func(tx *Store) error) (err error) {
	if s.tx != nil {
		return fn(s)
	}
	tx, err := s.db.Writer().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
				err = errors.Join(err, fmt.Errorf("rollback: %w", rbErr))
			}
		}
	}()
	if err = fn(&Store{db: s.db, tx: tx}); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// mapErr turns driver errors into the store's sentinel errors.
func mapErr(err error, op string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	var se *sqlite.Error
	if errors.As(err, &se) {
		code := se.Code()
		if code == sqlite3.SQLITE_CONSTRAINT_UNIQUE || code == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY ||
			(code&0xff == sqlite3.SQLITE_CONSTRAINT && strings.Contains(se.Error(), "UNIQUE constraint failed")) {
			return fmt.Errorf("%s: %w: %s", op, ErrConflict, se.Error())
		}
		if strings.Contains(se.Error(), "no such table") {
			return fmt.Errorf("%s: %w: %s", op, ErrMissingTable, se.Error())
		}
	}
	return fmt.Errorf("%s: %w", op, err)
}

// Opt is an optional value in an update: only fields with Set=true are
// written. For nullable columns use Opt[*T]; a nil pointer writes NULL.
type Opt[T any] struct {
	Set   bool
	Value T
}

// Some returns an Opt that writes v.
func Some[T any](v T) Opt[T] { return Opt[T]{Set: true, Value: v} }

// Null returns an Opt that writes NULL to a nullable column.
func Null[T any]() Opt[*T] { return Opt[*T]{Set: true} }

// Ptr returns a pointer to v, for nullable fields.
func Ptr[T any](v T) *T { return &v }

// Order is a fixed ORDER BY clause; only the package-level values exist, so
// no caller-controlled text reaches SQL.
type Order struct{ sql string }

var (
	// OrderNone keeps SQLite's natural order, as Prisma does without orderBy.
	OrderNone = Order{}
	// OrderCreatedAsc orders by created_at ascending.
	OrderCreatedAsc = Order{`"created_at" ASC`}
	// OrderCreatedDesc orders by created_at descending.
	OrderCreatedDesc = Order{`"created_at" DESC`}
	// OrderUpdatedDesc orders by updated_at descending (projects).
	OrderUpdatedDesc = Order{`"updated_at" DESC`}
	// OrderServiceKey orders env variables by service_name then key.
	OrderServiceKey = Order{`"service_name" ASC, "key" ASC`}
)

func (o Order) clause() string {
	if o.sql == "" {
		return ""
	}
	return " ORDER BY " + o.sql
}

// where accumulates AND-ed conditions.
type where struct {
	parts []string
	args  []any
}

func (w *where) add(cond string, args ...any) {
	w.parts = append(w.parts, cond)
	w.args = append(w.args, args...)
}

func (w *where) eq(col string, v any) { w.add(col+" = ?", v) }

// ne is Prisma's `not: v`, which (like SQL) never matches NULL.
func (w *where) ne(col string, v any) { w.add(col+" <> ?", v) }

// in is Prisma's `in: vs`; an empty list matches nothing.
func (w *where) in(col string, vs []string) {
	if len(vs) == 0 {
		w.add("0")
		return
	}
	args := make([]any, len(vs))
	for i, v := range vs {
		args[i] = v
	}
	w.add(col+" IN ("+placeholders(len(vs))+")", args...)
}

// notIn is Prisma's `notIn: vs`; an empty list matches everything.
func (w *where) notIn(col string, vs []string) {
	if len(vs) == 0 {
		return
	}
	args := make([]any, len(vs))
	for i, v := range vs {
		args[i] = v
	}
	w.add(col+" NOT IN ("+placeholders(len(vs))+")", args...)
}

func (w *where) null(col string, isNull bool) {
	if isNull {
		w.add(col + " IS NULL")
	} else {
		w.add(col + " IS NOT NULL")
	}
}

func (w *where) clause() string {
	if len(w.parts) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(w.parts, " AND ")
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// sets accumulates the SET list of an UPDATE.
type sets struct {
	cols []string
	args []any
}

func (s *sets) add(col string, v any) {
	s.cols = append(s.cols, col+" = ?")
	s.args = append(s.args, v)
}

func setOpt[T any](s *sets, col string, o Opt[T]) {
	if o.Set {
		s.add(col, o.Value)
	}
}

func (s *sets) clause() string { return strings.Join(s.cols, ", ") }

type rowScanner interface {
	Scan(dest ...any) error
}

// queryList runs a query and scans every row with scan.
func queryList[T any](ctx context.Context, q db.Querier, op, query string, args []any, scan func(rowScanner) (T, error)) ([]T, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, mapErr(err, op)
	}
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, mapErr(err, op)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, mapErr(err, op)
	}
	return out, nil
}

// queryOne runs a query expected to return at most one row; no row is ErrNotFound.
func queryOne[T any](ctx context.Context, q db.Querier, op, query string, args []any, scan func(rowScanner) (T, error)) (T, error) {
	v, err := scan(q.QueryRowContext(ctx, query, args...))
	if err != nil {
		var zero T
		return zero, mapErr(err, op)
	}
	return v, nil
}

// execAffected runs a write and returns the affected row count.
func execAffected(ctx context.Context, q db.Querier, op, query string, args ...any) (int64, error) {
	res, err := q.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, mapErr(err, op)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, mapErr(err, op)
	}
	return n, nil
}

// execOne runs a write that must touch exactly one row (else ErrNotFound).
func execOne(ctx context.Context, q db.Querier, op, query string, args ...any) error {
	n, err := execAffected(ctx, q, op, query, args...)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%s: %w", op, ErrNotFound)
	}
	return nil
}

func count(ctx context.Context, q db.Querier, op, table string, w where) (int, error) {
	var n int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+w.clause(), w.args...).Scan(&n); err != nil {
		return 0, mapErr(err, op)
	}
	return n, nil
}

func errEmptyFilter(op string) error {
	return fmt.Errorf("%s: refusing to run without a filter", op)
}
