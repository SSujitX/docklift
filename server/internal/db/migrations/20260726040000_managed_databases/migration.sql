-- AlterTable
ALTER TABLE "projects" ADD COLUMN "db_engine" TEXT;

-- CreateTable
CREATE TABLE "database_links" (
    "id" TEXT NOT NULL PRIMARY KEY,
    "database_project_id" TEXT NOT NULL,
    "app_project_id" TEXT NOT NULL,
    "service_name" TEXT NOT NULL DEFAULT '',
    "env_key" TEXT NOT NULL,
    "created_at" DATETIME DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT "database_links_database_project_id_fkey" FOREIGN KEY ("database_project_id") REFERENCES "projects" ("id") ON DELETE CASCADE ON UPDATE NO ACTION,
    CONSTRAINT "database_links_app_project_id_fkey" FOREIGN KEY ("app_project_id") REFERENCES "projects" ("id") ON DELETE CASCADE ON UPDATE NO ACTION
);

-- CreateIndex
CREATE UNIQUE INDEX "database_links_database_project_id_app_project_id_service_name_env_key_key" ON "database_links"("database_project_id", "app_project_id", "service_name", "env_key");
