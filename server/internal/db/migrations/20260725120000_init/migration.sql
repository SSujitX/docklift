-- CreateTable
CREATE TABLE "projects" (
    "id" TEXT NOT NULL PRIMARY KEY,
    "name" TEXT NOT NULL,
    "description" TEXT,
    "source_type" TEXT,
    "project_type" TEXT,
    "github_url" TEXT,
    "github_branch" TEXT,
    "domain" TEXT,
    "port" INTEGER,
    "status" TEXT,
    "container_name" TEXT,
    "auto_deploy" BOOLEAN DEFAULT false,
    "webhook_secret" TEXT,
    "build_type" TEXT NOT NULL DEFAULT 'auto',
    "base_directory" TEXT NOT NULL DEFAULT '.',
    "dockerfile_path" TEXT,
    "internal_port" INTEGER NOT NULL DEFAULT 3000,
    "publish_host_port" BOOLEAN NOT NULL DEFAULT false,
    "created_at" DATETIME DEFAULT CURRENT_TIMESTAMP,
    "updated_at" DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- CreateTable
CREATE TABLE "deployments" (
    "id" TEXT NOT NULL PRIMARY KEY,
    "project_id" TEXT NOT NULL,
    "status" TEXT,
    "trigger" TEXT DEFAULT 'manual',
    "commit_message" TEXT,
    "logs" TEXT,
    "created_at" DATETIME DEFAULT CURRENT_TIMESTAMP,
    "finished_at" DATETIME,
    CONSTRAINT "deployments_project_id_fkey" FOREIGN KEY ("project_id") REFERENCES "projects" ("id") ON DELETE CASCADE ON UPDATE NO ACTION
);

-- CreateTable
CREATE TABLE "services" (
    "id" TEXT NOT NULL PRIMARY KEY,
    "project_id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "dockerfile_path" TEXT NOT NULL,
    "container_name" TEXT,
    "port" INTEGER,
    "internal_port" INTEGER,
    "domain" TEXT,
    "status" TEXT,
    "created_at" DATETIME DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT "services_project_id_fkey" FOREIGN KEY ("project_id") REFERENCES "projects" ("id") ON DELETE CASCADE ON UPDATE NO ACTION
);

-- CreateTable
CREATE TABLE "persistent_volumes" (
    "id" TEXT NOT NULL PRIMARY KEY,
    "project_id" TEXT NOT NULL,
    "service_name" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "display_name" TEXT NOT NULL,
    "mount_path" TEXT NOT NULL,
    "created_at" DATETIME DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT "persistent_volumes_project_id_fkey" FOREIGN KEY ("project_id") REFERENCES "projects" ("id") ON DELETE CASCADE ON UPDATE NO ACTION
);

-- CreateTable
CREATE TABLE "env_variables" (
    "id" TEXT NOT NULL PRIMARY KEY,
    "project_id" TEXT NOT NULL,
    "key" TEXT NOT NULL,
    "value" TEXT NOT NULL,
    "is_build_arg" BOOLEAN,
    "is_runtime" BOOLEAN,
    "is_secret" BOOLEAN DEFAULT false,
    "created_at" DATETIME DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT "env_variables_project_id_fkey" FOREIGN KEY ("project_id") REFERENCES "projects" ("id") ON DELETE CASCADE ON UPDATE NO ACTION
);

-- CreateTable
CREATE TABLE "ports" (
    "port" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    "project_id" TEXT,
    "is_locked" BOOLEAN,
    CONSTRAINT "ports_project_id_fkey" FOREIGN KEY ("project_id") REFERENCES "projects" ("id") ON DELETE CASCADE ON UPDATE NO ACTION
);

-- CreateTable
CREATE TABLE "settings" (
    "key" TEXT NOT NULL PRIMARY KEY,
    "value" TEXT,
    "updated_at" DATETIME
);

-- CreateTable
CREATE TABLE "users" (
    "id" TEXT NOT NULL PRIMARY KEY,
    "name" TEXT NOT NULL,
    "email" TEXT NOT NULL,
    "password" TEXT NOT NULL,
    "role" TEXT NOT NULL DEFAULT 'admin',
    "passwordChangedAt" DATETIME,
    "created_at" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- CreateIndex
CREATE UNIQUE INDEX "persistent_volumes_project_id_name_key" ON "persistent_volumes"("project_id", "name");

-- CreateIndex
CREATE UNIQUE INDEX "persistent_volumes_project_id_service_name_mount_path_key" ON "persistent_volumes"("project_id", "service_name", "mount_path");

-- CreateIndex
CREATE UNIQUE INDEX "env_variables_project_id_key_key" ON "env_variables"("project_id", "key");

-- CreateIndex
CREATE UNIQUE INDEX "users_email_key" ON "users"("email");
