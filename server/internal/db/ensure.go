package db

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

// Ensure is the port of backend/src/scripts/ensureDb.ts, run before serving:
//
//  1. dedupe env_variables so the scoped unique index can apply (non-fatal),
//  2. apply migrations (`migrate deploy`),
//  3. on failure, baseline a legacy `db push` install (tables present, empty
//     history) by marking the init migration applied, then migrate again,
//  4. repair columns/indexes legacy installs may lack.
func (d *DB) Ensure(ctx context.Context, log *slog.Logger) error {
	if err := d.dedupeBeforeMigrate(ctx, log); err != nil {
		log.Warn("[ensureDb] Dedupe skipped", "error", err)
	}

	if err := d.migrateLogged(ctx, log); err != nil {
		log.Warn("[ensureDb] migrate deploy failed — attempting legacy baseline", "error", err)
		if err := d.baselineLegacyIfNeeded(ctx, log); err != nil {
			return fmt.Errorf("[ensureDb] legacy baseline failed: %w", err)
		}
		if err := d.migrateLogged(ctx, log); err != nil {
			return fmt.Errorf("[ensureDb] migrate deploy failed after baseline: %w", err)
		}
	}

	if err := d.RepairLegacySchema(ctx, log); err != nil {
		return fmt.Errorf("[ensureDb] schema repair failed: %w", err)
	}
	log.Info("[ensureDb] Database ready")
	return nil
}

func (d *DB) dedupeBeforeMigrate(ctx context.Context, log *slog.Logger) error {
	exists, err := TableExists(ctx, d.Writer(), "env_variables")
	if err != nil || !exists {
		return err
	}
	removed, err := DedupeEnvVariables(ctx, d.Writer())
	if err != nil {
		return err
	}
	if removed > 0 {
		log.Info(fmt.Sprintf("[ensureDb] Removed %d duplicate env_variables row(s)", removed))
	}
	return nil
}

func (d *DB) migrateLogged(ctx context.Context, log *slog.Logger) error {
	res, err := d.Migrate(ctx)
	for _, name := range res.Applied {
		log.Info("[ensureDb] Applied migration", "migration", name)
	}
	if err != nil {
		return err
	}
	if len(res.Applied) == 0 {
		log.Info("[ensureDb] No pending migrations to apply")
	}
	return nil
}

func (d *DB) baselineLegacyIfNeeded(ctx context.Context, log *slog.Logger) error {
	w := d.Writer()
	hasProjects, err := TableExists(ctx, w, "projects")
	if err != nil || !hasProjects {
		return err
	}
	hasLedger, err := TableExists(ctx, w, "_prisma_migrations")
	if err != nil {
		return err
	}
	if hasLedger {
		var n int
		if err := w.QueryRowContext(ctx, `SELECT COUNT(*) FROM "_prisma_migrations"`).Scan(&n); err != nil {
			return fmt.Errorf("count _prisma_migrations: %w", err)
		}
		if n > 0 {
			return nil
		}
	}
	log.Info("[ensureDb] Legacy database detected (no migration history) — baselining init")
	return d.MarkApplied(ctx, BaselineMigration)
}

// RepairLegacySchema is the idempotent repair pass of ensureDb.ts for installs
// that used `db push` before migration history existed.
func (d *DB) RepairLegacySchema(ctx context.Context, log *slog.Logger) error {
	w := d.Writer()
	has := func(table string) (bool, error) { return TableExists(ctx, w, table) }
	hasCol := func(table, col string) (bool, error) { return ColumnExists(ctx, w, table, col) }
	exec := func(stmt string) error {
		if _, err := w.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("%s: %w", firstLine(stmt), err)
		}
		return nil
	}
	addColumn := func(table, col, ddl, msg string) error {
		ok, err := hasCol(table, col)
		if err != nil || ok {
			return err
		}
		if err := exec(ddl); err != nil {
			return err
		}
		log.Info(msg)
		return nil
	}

	projects, err := has("projects")
	if err != nil || !projects {
		return err
	}
	if err := addColumn("projects", "publish_host_port",
		`ALTER TABLE "projects" ADD COLUMN "publish_host_port" BOOLEAN NOT NULL DEFAULT false`,
		"[ensureDb] Added projects.publish_host_port"); err != nil {
		return err
	}
	if err := addColumn("projects", "db_engine",
		`ALTER TABLE "projects" ADD COLUMN "db_engine" TEXT`,
		"[ensureDb] Added projects.db_engine"); err != nil {
		return err
	}

	links, err := has("database_links")
	if err != nil {
		return err
	}
	if !links {
		if err := exec(`CREATE TABLE "database_links" (
        "id" TEXT NOT NULL PRIMARY KEY,
        "database_project_id" TEXT NOT NULL,
        "app_project_id" TEXT NOT NULL,
        "service_name" TEXT NOT NULL DEFAULT '',
        "env_key" TEXT NOT NULL,
        "created_at" DATETIME DEFAULT CURRENT_TIMESTAMP,
        CONSTRAINT "database_links_database_project_id_fkey" FOREIGN KEY ("database_project_id") REFERENCES "projects" ("id") ON DELETE CASCADE ON UPDATE NO ACTION,
        CONSTRAINT "database_links_app_project_id_fkey" FOREIGN KEY ("app_project_id") REFERENCES "projects" ("id") ON DELETE CASCADE ON UPDATE NO ACTION
      )`); err != nil {
			return err
		}
		if err := exec(`CREATE UNIQUE INDEX IF NOT EXISTS "database_links_database_project_id_app_project_id_service_name_env_key_key" ON "database_links"("database_project_id", "app_project_id", "service_name", "env_key")`); err != nil {
			return err
		}
		log.Info("[ensureDb] Created database_links")
	}
	// Keep newest link per (app, service, env_key) before the unique index can apply.
	if err := exec(`DELETE FROM "database_links"
      WHERE "id" IN (
        SELECT a."id" FROM "database_links" a
        INNER JOIN "database_links" b
          ON a."app_project_id" = b."app_project_id"
         AND a."service_name" = b."service_name"
         AND a."env_key" = b."env_key"
         AND (
           COALESCE(a."created_at", '') < COALESCE(b."created_at", '')
           OR (COALESCE(a."created_at", '') = COALESCE(b."created_at", '') AND a."id" < b."id")
         )
      )`); err != nil {
		return err
	}
	if err := exec(`CREATE UNIQUE INDEX IF NOT EXISTS "database_links_app_project_id_service_name_env_key_key" ON "database_links"("app_project_id", "service_name", "env_key")`); err != nil {
		return err
	}

	deployments, err := has("deployments")
	if err != nil {
		return err
	}
	if deployments {
		if err := addColumn("deployments", "commit_sha",
			`ALTER TABLE "deployments" ADD COLUMN "commit_sha" TEXT`,
			"[ensureDb] Added deployments.commit_sha"); err != nil {
			return err
		}
		if err := addColumn("deployments", "image_tags",
			`ALTER TABLE "deployments" ADD COLUMN "image_tags" TEXT`,
			"[ensureDb] Added deployments.image_tags"); err != nil {
			return err
		}
	}

	envs, err := has("env_variables")
	if err != nil || !envs {
		return err
	}
	if err := addColumn("env_variables", "is_secret",
		`ALTER TABLE "env_variables" ADD COLUMN "is_secret" BOOLEAN DEFAULT false`,
		"[ensureDb] Added env_variables.is_secret"); err != nil {
		return err
	}
	scoped, err := hasCol("env_variables", "service_name")
	if err != nil {
		return err
	}
	if scoped {
		// Scoped env: unique is (project_id, service_name, key). Never recreate the
		// pre-scope (project_id, key) index — it blocks shared+service overrides.
		if err := exec(`DROP INDEX IF EXISTS "env_variables_project_id_key_key"`); err != nil {
			return err
		}
		return exec(`CREATE UNIQUE INDEX IF NOT EXISTS "env_variables_project_id_service_name_key_key" ON "env_variables"("project_id", "service_name", "key")`)
	}
	return exec(`CREATE UNIQUE INDEX IF NOT EXISTS "env_variables_project_id_key_key" ON "env_variables"("project_id", "key")`)
}

// DedupeEnvVariables keeps the newest row per (project_id, service_name, key)
// and deletes the rest, returning how many rows were removed. It is the port
// of dedupeEnvVariables in backend/src/lib/envVariables.ts (same ordering:
// project_id, service_name, key ascending, created_at descending; the first
// row of each group survives). It runs before migrations, so it tolerates a
// pre-scope table without service_name (treated as the shared scope "").
func DedupeEnvVariables(ctx context.Context, q Querier) (int, error) {
	scoped, err := ColumnExists(ctx, q, "env_variables", "service_name")
	if err != nil {
		return 0, err
	}
	scope := `''`
	if scoped {
		scope = `COALESCE("service_name", '')`
	}
	rows, err := q.QueryContext(ctx, `SELECT "id", "project_id", `+scope+`, "key" FROM "env_variables" ORDER BY "project_id" ASC, `+scope+` ASC, "key" ASC, "created_at" DESC`)
	if err != nil {
		return 0, fmt.Errorf("list env_variables: %w", err)
	}
	seen := map[string]bool{}
	var toDelete []any
	for rows.Next() {
		var id, project, service, key string
		if err := rows.Scan(&id, &project, &service, &key); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan env_variables: %w", err)
		}
		k := project + "\x00" + service + "\x00" + key
		if seen[k] {
			toDelete = append(toDelete, id)
			continue
		}
		seen[k] = true
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return 0, fmt.Errorf("list env_variables: %w", err)
	}
	if len(toDelete) == 0 {
		return 0, nil
	}
	const batch = 500
	for start := 0; start < len(toDelete); start += batch {
		chunk := toDelete[start:min(start+batch, len(toDelete))]
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",")
		if _, err := q.ExecContext(ctx, `DELETE FROM "env_variables" WHERE "id" IN (`+placeholders+`)`, chunk...); err != nil {
			return 0, fmt.Errorf("delete duplicate env_variables: %w", err)
		}
	}
	return len(toDelete), nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
