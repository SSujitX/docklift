package db

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
)

// lfChecksums are sha256 sums of the migration files as committed (LF), the
// values a Linux install recorded in _prisma_migrations.
var lfChecksums = map[string]string{
	"20260725120000_init":                     "6f1e67aa6163dee561130a96e214dc7f3c7313e26410f1423cd9a57783e494fb",
	"20260725160000_env_service_scope":        "1c89bb2b2cc819b4f8be72e584c3776b08ccfe6d82b24e094b01db0f49fef30d",
	"20260726040000_managed_databases":        "93189be5c9d32e278b30e652a019f7c97c5c706573481004dd2c48b8fc61e08b",
	"20260726043000_database_link_env_unique": "37516dea6fbdefbb81f85c2c894403a25e7f36ca096a309856b50974a440d2bf",
	"20260731120000_deployment_image_tags":    "5be5833db5fccb99c3a7754367ba6c39164e415c5bdeae45cecf2341dd110061",
	"20261128120000_api_tokens":               "bc80a9166722157df58ca2aae47fd98618b8780214a45cc8242bd5eee94910d4",
}

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func openTemp(t *testing.T) *DB {
	t.Helper()
	d, err := Open(context.Background(), filepath.Join(t.TempDir(), "nested", "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestEmbeddedMigrations(t *testing.T) {
	ms, err := Migrations()
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != len(lfChecksums) {
		t.Fatalf("got %d migrations, want %d", len(ms), len(lfChecksums))
	}
	for i, m := range ms {
		if want := lfChecksums[m.Name]; m.Checksum != want {
			t.Errorf("%s checksum %s, want %s (file must keep LF endings)", m.Name, m.Checksum, want)
		}
		if i > 0 && ms[i-1].Name >= m.Name {
			t.Errorf("migrations out of order: %s before %s", ms[i-1].Name, m.Name)
		}
	}
}

type ledgerEntry struct {
	name, checksum string
	finished       bool
	steps          int
	startedType    string
}

func ledger(t *testing.T, d *DB) []ledgerEntry {
	t.Helper()
	rows, err := d.Reader().Query(`SELECT migration_name, checksum, finished_at IS NOT NULL, applied_steps_count, typeof(started_at) FROM _prisma_migrations ORDER BY started_at, migration_name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []ledgerEntry
	for rows.Next() {
		var e ledgerEntry
		if err := rows.Scan(&e.name, &e.checksum, &e.finished, &e.steps, &e.startedType); err != nil {
			t.Fatal(err)
		}
		out = append(out, e)
	}
	return out
}

func TestMigrateFreshDatabase(t *testing.T) {
	ctx := context.Background()
	d := openTemp(t)
	res, err := d.Migrate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Applied) != len(lfChecksums) {
		t.Fatalf("applied %v", res.Applied)
	}
	entries := ledger(t, d)
	if len(entries) != len(lfChecksums) {
		t.Fatalf("ledger has %d rows", len(entries))
	}
	for _, e := range entries {
		if !e.finished || e.steps != 1 || e.checksum != lfChecksums[e.name] || e.startedType != "integer" {
			t.Errorf("ledger row %+v", e)
		}
	}
	for _, col := range [][2]string{{"projects", "db_engine"}, {"deployments", "image_tags"}, {"env_variables", "service_name"}} {
		if ok, err := ColumnExists(ctx, d.Writer(), col[0], col[1]); err != nil || !ok {
			t.Errorf("column %s.%s missing (%v)", col[0], col[1], err)
		}
	}
	var fk int
	if err := d.Writer().QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil || fk != 1 {
		t.Errorf("foreign_keys = %d, %v", fk, err)
	}
	var mode string
	if err := d.Writer().QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil || mode != "wal" {
		t.Errorf("journal_mode = %q, %v", mode, err)
	}

	again, err := d.Migrate(ctx)
	if err != nil || len(again.Applied) != 0 {
		t.Errorf("second migrate applied %v, %v", again.Applied, err)
	}
}

func execScript(t *testing.T, d *DB, sql string) {
	t.Helper()
	if _, err := d.Writer().Exec(sql); err != nil {
		t.Fatal(err)
	}
}

func initSQL(t *testing.T) string {
	t.Helper()
	ms, err := Migrations()
	if err != nil {
		t.Fatal(err)
	}
	return ms[0].SQL
}

func TestMigrateRefusesUnbaselinedSchema(t *testing.T) {
	d := openTemp(t)
	execScript(t, d, initSQL(t))
	if _, err := d.Migrate(context.Background()); !errors.Is(err, ErrSchemaNotEmpty) {
		t.Fatalf("err = %v, want P3005", err)
	}
	if ok, _ := TableExists(context.Background(), d.Writer(), "_prisma_migrations"); ok {
		t.Error("P3005 must not create the ledger")
	}
}

func TestMarkAppliedMatchesPrismaResolve(t *testing.T) {
	ctx := context.Background()
	d := openTemp(t)
	if err := d.MarkApplied(ctx, BaselineMigration); err != nil {
		t.Fatal(err)
	}
	var logs string
	var steps int
	var finishedEqualsStarted bool
	err := d.Reader().QueryRow(`SELECT logs, applied_steps_count, finished_at = started_at FROM _prisma_migrations`).Scan(&logs, &steps, &finishedEqualsStarted)
	if err != nil || logs != "" || steps != 0 || !finishedEqualsStarted {
		t.Errorf("resolve row logs=%q steps=%d same=%v err=%v", logs, steps, finishedEqualsStarted, err)
	}
	if err := d.MarkApplied(ctx, BaselineMigration); err == nil {
		t.Error("marking twice must fail")
	}
	if err := d.MarkApplied(ctx, "nope"); err == nil {
		t.Error("unknown migration must fail")
	}
}

func TestFailedMigrationBlocksLikeP3009(t *testing.T) {
	ctx := context.Background()
	d := openTemp(t)
	execScript(t, d, createLedgerSQL)
	execScript(t, d, `CREATE TABLE "projects" ("id" TEXT PRIMARY KEY)`)
	_, err := d.Migrate(ctx)
	if err == nil || errors.Is(err, ErrFailedMigration) {
		t.Fatalf("first run must fail applying init, got %v", err)
	}
	var logs string
	var finished bool
	if err := d.Reader().QueryRow(`SELECT logs, finished_at IS NOT NULL FROM _prisma_migrations`).Scan(&logs, &finished); err != nil {
		t.Fatal(err)
	}
	if finished || logs == "" {
		t.Errorf("failed row finished=%v logs=%q", finished, logs)
	}
	if _, err := d.Migrate(ctx); !errors.Is(err, ErrFailedMigration) {
		t.Errorf("second run err = %v, want P3009", err)
	}
	execScript(t, d, `UPDATE _prisma_migrations SET rolled_back_at = 1`)
	execScript(t, d, `DROP TABLE projects`)
	if res, err := d.Migrate(ctx); err != nil || len(res.Applied) != len(lfChecksums) {
		t.Errorf("after rollback resolve: %v %v", res.Applied, err)
	}
}

// A legacy install was created with `prisma db push` before migration history
// existed: the init tables, no ledger.
func TestEnsureBaselinesLegacyInstall(t *testing.T) {
	ctx := context.Background()
	d := openTemp(t)
	execScript(t, d, initSQL(t))
	execScript(t, d, `INSERT INTO projects (id, name) VALUES ('p1', 'legacy');
		INSERT INTO env_variables (id, project_id, key, value, created_at) VALUES ('e1', 'p1', 'K', 'old', 1000), ('e2', 'p1', 'K2', 'v', 1000);`)
	if err := d.Ensure(ctx, quietLog()); err != nil {
		t.Fatal(err)
	}
	entries := ledger(t, d)
	if len(entries) != len(lfChecksums) || entries[0].name != BaselineMigration || entries[0].steps != 0 {
		t.Fatalf("ledger after baseline: %+v", entries)
	}
	var service string
	if err := d.Reader().QueryRow(`SELECT service_name FROM env_variables WHERE id = 'e1'`).Scan(&service); err != nil || service != "" {
		t.Errorf("env row not carried into scoped table: %q %v", service, err)
	}
	if err := d.Ensure(ctx, quietLog()); err != nil {
		t.Errorf("Ensure must be idempotent: %v", err)
	}
}

func TestRepairLegacySchema(t *testing.T) {
	ctx := context.Background()
	d := openTemp(t)
	execScript(t, d, `
		CREATE TABLE "projects" ("id" TEXT NOT NULL PRIMARY KEY, "name" TEXT NOT NULL);
		CREATE TABLE "deployments" ("id" TEXT NOT NULL PRIMARY KEY, "project_id" TEXT NOT NULL);
		CREATE TABLE "env_variables" ("id" TEXT NOT NULL PRIMARY KEY, "project_id" TEXT NOT NULL, "key" TEXT NOT NULL, "value" TEXT NOT NULL);`)
	if err := d.RepairLegacySchema(ctx, quietLog()); err != nil {
		t.Fatal(err)
	}
	for _, col := range [][2]string{
		{"projects", "publish_host_port"}, {"projects", "db_engine"}, {"deployments", "commit_sha"},
		{"deployments", "image_tags"}, {"env_variables", "is_secret"}, {"database_links", "env_key"},
	} {
		if ok, err := ColumnExists(ctx, d.Writer(), col[0], col[1]); err != nil || !ok {
			t.Errorf("repair did not add %s.%s (%v)", col[0], col[1], err)
		}
	}
	var idx int
	if err := d.Reader().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='env_variables_project_id_key_key'`).Scan(&idx); err != nil || idx != 1 {
		t.Errorf("pre-scope env index = %d, %v", idx, err)
	}
	if err := d.RepairLegacySchema(ctx, quietLog()); err != nil {
		t.Errorf("repair must be idempotent: %v", err)
	}
}

func TestRepairDedupesDatabaseLinks(t *testing.T) {
	ctx := context.Background()
	d := openTemp(t)
	if _, err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	execScript(t, d, `DROP INDEX "database_links_app_project_id_service_name_env_key_key";
		INSERT INTO projects (id, name) VALUES ('db1', 'a'), ('db2', 'b'), ('app', 'c');
		INSERT INTO database_links (id, database_project_id, app_project_id, service_name, env_key, created_at) VALUES
		  ('old', 'db1', 'app', '', 'DATABASE_URL', 1000), ('new', 'db2', 'app', '', 'DATABASE_URL', 2000);`)
	if err := d.RepairLegacySchema(ctx, quietLog()); err != nil {
		t.Fatal(err)
	}
	var ids []string
	rows, _ := d.Reader().Query(`SELECT id FROM database_links`)
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) != 1 || ids[0] != "new" {
		t.Errorf("links after dedupe = %v, want only the newest", ids)
	}
}

func TestDedupeEnvVariables(t *testing.T) {
	ctx := context.Background()
	d := openTemp(t)
	if _, err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	execScript(t, d, `DROP INDEX "env_variables_project_id_service_name_key_key";
		INSERT INTO projects (id, name) VALUES ('p', 'p');
		INSERT INTO env_variables (id, project_id, service_name, key, value, created_at) VALUES
		  ('a1', 'p', '', 'K', 'oldest', 1000),
		  ('a2', 'p', '', 'K', 'newest', 3000),
		  ('a3', 'p', '', 'K', 'middle', 2000),
		  ('b1', 'p', 'api', 'K', 'scoped', 1000),
		  ('c1', 'p', '', 'OTHER', 'x', 1000);`)
	removed, err := DedupeEnvVariables(ctx, d.Writer())
	if err != nil || removed != 2 {
		t.Fatalf("removed %d, %v", removed, err)
	}
	var value string
	if err := d.Reader().QueryRow(`SELECT value FROM env_variables WHERE key = 'K' AND service_name = ''`).Scan(&value); err != nil || value != "newest" {
		t.Errorf("survivor = %q, %v", value, err)
	}
	if again, err := DedupeEnvVariables(ctx, d.Writer()); err != nil || again != 0 {
		t.Errorf("second dedupe removed %d, %v", again, err)
	}
}

func TestReopen(t *testing.T) {
	ctx := context.Background()
	d := openTemp(t)
	if _, err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := d.Reopen(ctx); err != nil {
		t.Fatal(err)
	}
	if ok, err := TableExists(ctx, d.Reader(), "users"); err != nil || !ok {
		t.Errorf("after reopen users table visible = %v, %v", ok, err)
	}
}
