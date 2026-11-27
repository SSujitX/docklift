---
name: Database Management
description: Guide for managing the Docklift SQLite database - embedded SQL migrations, the Go store layer and schema changes.
---

# Database Management Guide

Docklift stores its state in one SQLite file, opened with the pure-Go `modernc.org/sqlite` driver
through `database/sql`. There is no ORM: the schema is plain SQL migrations and every query is
hand-written in `server/internal/store`.

## Where things live

| Path | Purpose |
|------|---------|
| `server/internal/db/db.go` | Opens the file: WAL, `synchronous=NORMAL`, `foreign_keys=1`, 10 s busy timeout. One single-connection **writer** pool (immediate transactions) and an 8-connection read-only **reader** pool. File and `-wal`/`-shm` are chmod 0600. `Reopen` swaps both pools after a restore replaces the file |
| `server/internal/db/migrate.go` | Embedded-migration runner (`//go:embed migrations/*/migration.sql`) |
| `server/internal/db/ensure.go` | Startup sequence: `Ensure` |
| `server/internal/db/migrations/<name>/migration.sql` | The migrations, applied in name order |
| `server/internal/db/types.go` | Column codecs (`db.Time`, `db.NewID()` UUID v4, `db.Now()`) |
| `server/internal/store/*.go` | One file per table: row struct, JSON tags, queries |

Database file: `data/docklift.db` on the host, `/app/data/docklift.db` in the backend container
(`DATABASE_URL=file:/app/data/docklift.db`). Backups use a `VACUUM INTO` snapshot (see
`system_administration`).

## Migration model

The binary applies migrations itself; there is no separate tool. `docklift serve` runs
`db.Ensure` before listening, and `docklift migrate` runs it and exits.

`Ensure` does, in order:

1. Dedupe `env_variables` rows (so the scoped unique index can apply). Non-fatal.
2. `Migrate`: apply every embedded migration not recorded as finished in the ledger.
3. If that fails, baseline a legacy `db push` install (tables present, empty ledger) by marking
   `20260725120000_init` applied, then migrate again.
4. `RepairLegacySchema`: idempotent repairs for old installs (missing `publish_host_port` and
   `db_engine` columns, `database_links`, the scoped env unique index).

### The `_prisma_migrations` ledger

2.x used Prisma, and existing databases carry its ledger table. The Go runner keeps writing the
same table with the same columns so a 2.x database upgrades in place and rolls back cleanly.
Rules it follows:

- Empty database: creates the ledger. Non-empty schema without a ledger: refuses (`P3005`) until baselined.
- A ledger row with `finished_at` NULL (a failed attempt) blocks all further migrations (`P3009`).
  The failure text is stored in `logs`.
- Each migration runs as one script on the writer connection: a row is inserted first, then
  `finished_at` and `applied_steps_count = 1` are set on success.
- `checksum` is the **sha256 of the exact bytes of `migration.sql`**.

Because of the checksums, **never edit a migration that has shipped**. Add a new one.
`server/.gitattributes` sets `internal/db/migrations/**/*.sql text eol=lf`; keep LF line endings
(a CRLF checkout changes the checksum). Never run `prisma db push`, `prisma migrate` or any Prisma tooling.

### Adding a migration

1. Create `server/internal/db/migrations/<YYYYMMDDHHMMSS>_<snake_case_name>/migration.sql`. The
   timestamp must sort after every existing folder (migrations run in name order). Write plain SQLite
   SQL, LF endings. For SQLite table rewrites follow the existing files (create `new_x`, copy,
   drop, rename, recreate indexes; see `20260725160000_env_service_scope`).
2. Update the store layer (`server/internal/store/<table>.go`) for the new columns or tables.
3. Update `lfChecksums` in `server/internal/db/migrate_test.go` with the sha256 of the new file
   (`sha256sum migration.sql`). `TestEmbeddedMigrations` fails if the count or any checksum differs.
4. If an old install could be missing the object even after migrating (a `db push` legacy install),
   consider `RepairLegacySchema` in `ensure.go`; usually a migration is enough.
5. Test on a **copy** of a real database: `docklift migrate` against it, or start the server.
   `tests/contract` also boots from `testdata/seed.db.gz`, a database written by the 2.1.6 release.
6. Update the contract/route docs if the API surface changed, and this skill's model table.

Migrations are forward-only. There are no down scripts.

## Store layer (`server/internal/store`)

- `store.New(db)` returns a `*Store`; `InTx(ctx, func(tx *Store) error)` runs a function in one
  writer transaction.
- Reads use `s.reader()`, writes `s.writer()`. Helpers: `queryList`, `queryOne`, `execAffected`,
  `execOne`, plus small `where` / `sets` builders.
- SQL is plain text with quoted identifiers and `?` placeholders. Never build SQL by
  concatenating user input.
- Errors map to `ErrNotFound` (update/delete of a missing key), `ErrConflict` (unique violation)
  and `ErrMissingTable` (table absent, e.g. a restored foreign file).
- Row structs carry JSON tags that match the API (snake_case columns, `passwordChangedAt` camelCase,
  `null` for NULL). Handlers return them directly.
- IDs are UUID v4 generated in Go (`db.NewID()`). `DateTime` columns are written as integer Unix
  milliseconds (the format 2.x wrote); reads also accept older TEXT values. Use `db.Now()`; the JSON
  form is ISO-8601.
- `foreign_keys` is on; child tables cascade on project delete.

## Core commands

```bash
cd server
go run ./cmd/docklift migrate                 # apply pending migrations and exit (uses DATABASE_URL / DATA_PATH)
go test ./internal/db/... ./internal/store/...
sqlite3 data/docklift.db ".tables"            # inspect (sqlite3 CLI, read-only habits; stop the server for writes)
```

In the container: `docker exec docklift-backend docklift migrate`. Password reset:
`docker exec -it docklift-backend docklift reset-password`.

## Tables

| Table | Purpose |
|-------|---------|
| `users` | Admin accounts. `passwordChangedAt` feeds the JWT `pwdv` claim |
| `projects` | An application or managed database: source, build settings, status, `project_type`, `db_engine` |
| `services` | One deployable unit within a project (Dockerfile, domain, ports, container name) |
| `deployments` | Build/deploy history, trigger, captured logs, `commit_sha`, `image_tags` |
| `env_variables` | Shared or per-service vars: `service_name` (`""` = all services), `is_build_arg` / `is_runtime` / `is_secret`; unique `(project_id, service_name, key)` |
| `persistent_volumes` | Configured named-volume mounts per service; unique `(project_id, name)` and `(project_id, service_name, mount_path)` |
| `ports` | Host port pool (`is_locked`); used only when a project has `publish_host_port` on |
| `settings` | Key/value system settings (GitHub App credentials, ACME email, panel domain) |
| `database_links` | Managed database -> app links: which env key is injected into which service; unique `(database_project_id, app_project_id, service_name, env_key)` and `(app_project_id, service_name, env_key)` |
| `_prisma_migrations` | Migration ledger (see above) |

### Build and storage fields on `projects`

| Field | Default | Meaning |
|-------|---------|---------|
| `build_type` | `"auto"` | `auto` / `dockerfile` / `railpack` |
| `base_directory` | `"."` | Subdirectory to build from (monorepos) |
| `dockerfile_path` | `NULL` | Explicit Dockerfile when not auto-detecting |
| `internal_port` | `3000` | Port the app listens on inside the container |
| `publish_host_port` | `false` | When true, publish host ports from the pool |

Env variable keys are validated by the API (invalid keys return 400, duplicates 409). The startup
dedupe lives in `internal/db` (`DedupeEnvVariables`); the API-side rules are in `internal/api/projects/env.go` and `internal/deploy/envvars.go`.

Child rows cascade on project delete: deleting a project removes its services, deployments, env
vars, volume records, links and frees its ports.

## Troubleshooting

- **Startup fails on migrate**: `docker logs docklift-backend` and look for `[ensureDb]`. A failed
  migration leaves a ledger row with `logs` filled in and `finished_at` NULL; restore the database
  from a backup, or fix the file by hand (set `rolled_back_at` on the failed ledger row) only with a
  copy in hand. Never edit a shipped migration to make it pass.
- **`P3005` schema not empty**: legacy `db push` database; `Ensure` baselines it automatically. If
  you see it from `docklift migrate` on a foreign file, the file is not a Docklift database.
- **`TestEmbeddedMigrations` fails**: wrong checksum (CRLF line endings, or an edited file) or a
  new migration missing from `lfChecksums`.
- **`database is locked`**: something else (a second process, a `sqlite3` shell holding a write
  lock) is using the file; the server waits 10 s before failing.
- **Restored backup looks stale**: restore also reconciles projects and reloads nginx; see
  `system_administration`.
