-- CreateIndex
CREATE UNIQUE INDEX "database_links_app_project_id_service_name_env_key_key" ON "database_links"("app_project_id", "service_name", "env_key");
