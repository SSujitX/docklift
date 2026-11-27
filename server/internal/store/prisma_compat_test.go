package store

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"docklift/internal/db"
)

// prismaScript writes rows through the TypeScript backend's Prisma client and
// prints them, plus the include-shaped queries the routes send to clients, as
// JSON (exactly what res.json produced).
const prismaScript = `
const { createRequire } = require('module');
const req = createRequire(process.argv[2] + '/package.json');
const { PrismaClient } = req('@prisma/client');
const prisma = new PrismaClient();
(async () => {
  const user = await prisma.user.create({ data: { name: 'Ada', email: 'ada@example.com', password: '$2b$12$sZSa/UYWabCyqaBbObc2vu3Pgkb1JM6nA0pxemgBtoJaJK6ZfTfK.', role: 'admin', passwordChangedAt: new Date(1795000000123) } });
  const app = await prisma.project.create({ data: { name: 'app', source_type: 'github', github_url: 'https://github.com/a/b', project_type: 'app', status: 'running', auto_deploy: true, webhook_secret: 'whs', build_type: 'auto', base_directory: '.', internal_port: 3000 } });
  const dbp = await prisma.project.create({ data: { name: 'pg', source_type: 'managed', project_type: 'database', db_engine: 'postgres', status: 'pending', auto_deploy: false, build_type: 'dockerfile', base_directory: '.', dockerfile_path: null, internal_port: 5432, publish_host_port: false } });
  const project = await prisma.project.update({ where: { id: app.id }, data: { description: 'desc' } });
  const service = await prisma.service.create({ data: { project_id: app.id, name: 'web', dockerfile_path: 'Dockerfile', container_name: 'c1', internal_port: 3000, port: 5501, status: 'running', domain: 'a.example.com' } });
  const env = await prisma.envVariable.create({ data: { project_id: app.id, service_name: '', key: 'FOO', value: 'bar', is_build_arg: false, is_runtime: true, is_secret: false } });
  const envNulls = await prisma.envVariable.create({ data: { project_id: app.id, service_name: 'web', key: 'BAR', value: 'x' } });
  const deployment = await prisma.deployment.create({ data: { project_id: app.id, status: 'success', trigger: 'manual', commit_message: 'init', commit_sha: 'abc', image_tags: { web: 'docklift/app-web:1' }, logs: 'ok', finished_at: new Date(1795000005000) } });
  const deploymentNulls = await prisma.deployment.create({ data: { project_id: app.id, status: 'in_progress' } });
  const volume = await prisma.persistentVolume.create({ data: { project_id: app.id, service_name: 'web', name: 'data', display_name: 'Data', mount_path: '/data' } });
  await prisma.databaseLink.create({ data: { database_project_id: dbp.id, app_project_id: app.id, service_name: '', env_key: 'DATABASE_URL' } });
  await prisma.port.create({ data: { port: 5501, project_id: app.id, is_locked: true } });
  await prisma.port.create({ data: { port: 5502 } });
  await prisma.settings.upsert({ where: { key: 'k' }, update: { value: 'v' }, create: { key: 'k', value: 'v' } });
  const setting = await prisma.settings.findUnique({ where: { key: 'k' } });
  const databases = await prisma.project.findMany({ where: { project_type: 'database' }, orderBy: { created_at: 'desc' }, include: { services: true, databaseLinksAsDb: { include: { app_project: { select: { id: true, name: true, status: true } } } } } });
  const appLinks = await prisma.databaseLink.findMany({ where: { app_project_id: app.id }, orderBy: { created_at: 'desc' }, include: { database_project: { select: { id: true, name: true, status: true, db_engine: true } } } });
  const dbLinks = await prisma.databaseLink.findMany({ where: { database_project_id: dbp.id }, orderBy: { created_at: 'desc' }, include: { app_project: { select: { id: true, name: true, status: true, services: { select: { id: true, name: true, status: true } } } } } });
  const ports = await prisma.port.findMany({ include: { project: { select: { id: true, name: true, status: true } } } });
  const fullUser = { ...user };
  delete fullUser.password;
  console.log(JSON.stringify({ user: fullUser, project, dbProject: dbp, service, env, envNulls, deployment, deploymentNulls, volume, setting, databases, appLinks, dbLinks, ports }));
  await prisma.$disconnect();
})().catch((e) => { console.error(e); process.exit(1); });
`

// backendDir locates ../backend with an installed Prisma client, or skips.
func backendDir(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(file), "..", "..", "..", "backend")
	for _, need := range []string{"node_modules/@prisma/client", "node_modules/prisma/build/index.js", "prisma/schema.prisma"} {
		if _, err := os.Stat(filepath.Join(dir, need)); err != nil {
			t.Skipf("TypeScript backend with node_modules not available (%s missing)", need)
		}
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}
	return dir
}

func TestReadsPrismaCreatedDatabase(t *testing.T) {
	backend := backendDir(t)
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "prisma.db")
	envs := append(os.Environ(), "DATABASE_URL=file:"+filepath.ToSlash(dbPath))

	migrate := exec.Command("node", filepath.Join(backend, "node_modules", "prisma", "build", "index.js"),
		"migrate", "deploy", "--schema", filepath.Join(backend, "prisma", "schema.prisma"))
	migrate.Dir, migrate.Env = backend, envs
	if out, err := migrate.CombinedOutput(); err != nil {
		t.Fatalf("prisma migrate deploy: %v\n%s", err, out)
	}
	script := filepath.Join(tmp, "seed.cjs")
	if err := os.WriteFile(script, []byte(prismaScript), 0o600); err != nil {
		t.Fatal(err)
	}
	seed := exec.Command("node", script, backend)
	seed.Dir, seed.Env = backend, envs
	seed.Stderr = os.Stderr
	out, err := seed.Output()
	if err != nil {
		t.Fatalf("seed through Prisma: %v", err)
	}
	var want map[string]json.RawMessage
	if err := json.Unmarshal(out, &want); err != nil {
		t.Fatalf("seed output: %v\n%s", err, out)
	}

	ctx := context.Background()
	database, err := db.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	res, err := database.Migrate(ctx)
	if err != nil || len(res.Applied) != 0 {
		t.Fatalf("Go must see Prisma's ledger as complete: applied %v, %v", res.Applied, err)
	}
	s := New(database)

	idOf := func(key string) string {
		var row struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(want[key], &row); err != nil {
			t.Fatal(err)
		}
		return row.ID
	}
	userID, projectID, dbProjectID := idOf("user"), idOf("project"), idOf("dbProject")

	must := func(v any, err error) any {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	user := must(s.GetUser(ctx, userID)).(*User)
	if user.PasswordVersion() != 1795000000123 || user.Password[:4] != "$2b$" {
		t.Errorf("user pwdv=%d hash=%q", user.PasswordVersion(), user.Password)
	}
	setting, ok, err := s.GetSetting(ctx, "k")
	if err != nil || !ok || setting != "v" {
		t.Errorf("setting = %q %v %v", setting, ok, err)
	}
	envRows := must(s.ListEnvVariables(ctx, EnvFilter{ProjectID: &projectID}, OrderServiceKey)).([]*EnvVariable)
	deployments := must(s.ListDeployments(ctx, DeploymentFilter{ProjectID: &projectID}, OrderCreatedAsc, Page{})).([]*Deployment)
	volumes := must(s.ListVolumes(ctx, projectID, OrderCreatedAsc)).([]*PersistentVolume)
	services := must(s.ListServices(ctx, ServiceFilter{ProjectID: &projectID}, OrderNone)).([]*Service)
	if len(envRows) != 2 || len(deployments) != 2 || len(volumes) != 1 || len(services) != 1 {
		t.Fatalf("row counts env=%d deployments=%d volumes=%d services=%d", len(envRows), len(deployments), len(volumes), len(services))
	}
	envByKey := map[string]*EnvVariable{}
	for _, e := range envRows {
		envByKey[e.Key] = e
	}

	compare := func(key string, got any) {
		t.Helper()
		gotJSON, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		var g, w any
		_ = json.Unmarshal(gotJSON, &g)
		_ = json.Unmarshal(want[key], &w)
		if !reflect.DeepEqual(g, w) {
			t.Errorf("%s differs from Prisma's JSON:\n  go:     %s\n  prisma: %s", key, gotJSON, want[key])
		}
	}
	compare("user", user)
	compare("project", must(s.GetProject(ctx, projectID)))
	compare("dbProject", must(s.GetProject(ctx, dbProjectID)))
	compare("service", services[0])
	compare("env", envByKey["FOO"])
	compare("envNulls", envByKey["BAR"])
	compare("deployment", deployments[0])
	compare("deploymentNulls", deployments[1])
	compare("volume", volumes[0])
	compare("setting", Setting{Key: "k", Value: &setting})
	compare("databases", must(s.ListDatabaseProjects(ctx)))
	compare("appLinks", must(s.ListLinksForApp(ctx, projectID)))
	compare("dbLinks", must(s.ListLinksForDatabase(ctx, dbProjectID)))
	compare("ports", must(s.ListPortsWithProject(ctx)))

	// Go writes must read back through Prisma's encodings: integer milliseconds.
	if _, err := s.UpdateProject(ctx, projectID, ProjectUpdate{Status: Some(Ptr("stopped"))}); err != nil {
		t.Fatal(err)
	}
	var typ string
	if err := database.Reader().QueryRow(`SELECT typeof(updated_at) FROM projects WHERE id = ?`, projectID).Scan(&typ); err != nil || typ != "integer" {
		t.Errorf("updated_at stored as %s, %v", typ, err)
	}
}
