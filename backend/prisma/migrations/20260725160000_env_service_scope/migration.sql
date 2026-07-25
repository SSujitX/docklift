-- Per-service env: empty service_name = shared across all services in the project.
-- Recreate env_variables so SQLite can replace the unique index cleanly.

PRAGMA foreign_keys=OFF;

CREATE TABLE "new_env_variables" (
    "id" TEXT NOT NULL PRIMARY KEY,
    "project_id" TEXT NOT NULL,
    "service_name" TEXT NOT NULL DEFAULT '',
    "key" TEXT NOT NULL,
    "value" TEXT NOT NULL,
    "is_build_arg" BOOLEAN,
    "is_runtime" BOOLEAN,
    "is_secret" BOOLEAN DEFAULT false,
    "created_at" DATETIME DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT "env_variables_project_id_fkey" FOREIGN KEY ("project_id") REFERENCES "projects" ("id") ON DELETE CASCADE ON UPDATE NO ACTION
);

INSERT INTO "new_env_variables" ("id", "project_id", "service_name", "key", "value", "is_build_arg", "is_runtime", "is_secret", "created_at")
SELECT "id", "project_id", '', "key", "value", "is_build_arg", "is_runtime", "is_secret", "created_at"
FROM "env_variables";

DROP TABLE "env_variables";
ALTER TABLE "new_env_variables" RENAME TO "env_variables";

CREATE UNIQUE INDEX "env_variables_project_id_service_name_key_key" ON "env_variables"("project_id", "service_name", "key");

PRAGMA foreign_keys=ON;
