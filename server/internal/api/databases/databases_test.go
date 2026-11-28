package databases

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/moby/moby/client"

	"docklift/internal/app"
	"docklift/internal/app/apptest"
	"docklift/internal/deploy"
	"docklift/internal/dockerx"
	"docklift/internal/execx"
	"docklift/internal/httpx"
	"docklift/internal/store"
)

// fakeEngine answers Ping and the network calls of a link; every other
// Engine method is unused by this package (the embedded nil interface
// panics if one is reached).
type fakeEngine struct {
	dockerx.Engine
	mu         sync.Mutex
	connected  []string
	connectErr error
}

func (f *fakeEngine) Ping(context.Context, client.PingOptions) (client.PingResult, error) {
	return client.PingResult{}, nil
}

func (f *fakeEngine) NetworkInspect(context.Context, string, client.NetworkInspectOptions) (client.NetworkInspectResult, error) {
	return client.NetworkInspectResult{}, nil
}

func (f *fakeEngine) NetworkConnect(_ context.Context, network string, o client.NetworkConnectOptions) (client.NetworkConnectResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.connectErr != nil {
		return client.NetworkConnectResult{}, f.connectErr
	}
	f.connected = append(f.connected, network+" "+o.Container)
	return client.NetworkConnectResult{}, nil
}

func (f *fakeEngine) NetworkDisconnect(_ context.Context, network string, o client.NetworkDisconnectOptions) (client.NetworkDisconnectResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.connected = slices.DeleteFunc(f.connected, func(s string) bool { return s == network+" "+o.Container })
	return client.NetworkDisconnectResult{}, nil
}

// fakeCLI records docker CLI calls and answers with result/err.
type fakeCLI struct {
	mu     sync.Mutex
	calls  [][]string
	result execx.Result
	err    error
}

func (f *fakeCLI) run(_ context.Context, c execx.Cmd) (execx.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, append([]string{c.Name}, c.Args...))
	return f.result, f.err
}

// hubTags serves Docker Hub tag pages; nil tags answers 500 (Hub down).
func hubTags(t *testing.T, tags map[string][]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tags == nil {
			http.Error(w, "down", http.StatusInternalServerError)
			return
		}
		repo := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v2/repositories/library/"), "/tags")
		results := []map[string]string{}
		for _, n := range tags[repo] {
			results = append(results, map[string]string{"name": n})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results, "next": nil})
	}))
	t.Cleanup(srv.Close)
	return srv
}

type env struct {
	d   *app.Deps
	h   http.Handler
	cli *fakeCLI
}

func setup(t *testing.T, tags map[string][]string) *env {
	t.Helper()
	d := apptest.NewDeps(t)
	d.Deploy.Hub.BaseURL = hubTags(t, tags).URL
	cli := &fakeCLI{}
	d.DockerX.Run = cli.run
	r := chi.NewRouter()
	r.Use(httpx.ParseBody)
	Mount(r, d)
	return &env{d: d, h: r, cli: cli}
}

func (e *env) do(t *testing.T, method, path string, body any) (*httptest.ResponseRecorder, any) {
	t.Helper()
	var rd *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rd = bytes.NewReader(raw)
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	var out any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("%s %s: %d %q", method, path, rec.Code, rec.Body.String())
	}
	return rec, out
}

func obj(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("not an object: %#v", v)
	}
	return m
}

func expectError(t *testing.T, rec *httptest.ResponseRecorder, body any, status int, msg string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d (%v)", rec.Code, status, body)
	}
	got, _ := obj(t, body)["error"].(string)
	if msg != "" && got != msg {
		t.Fatalf("error = %q, want %q", got, msg)
	}
}

func TestEngines(t *testing.T) {
	t.Run("hub down uses the static catalog", func(t *testing.T) {
		e := setup(t, nil)
		rec, body := e.do(t, http.MethodGet, "/engines", nil)
		if rec.Code != 200 {
			t.Fatalf("status %d", rec.Code)
		}
		list := body.([]any)
		var ids []string
		for _, x := range list {
			ids = append(ids, obj(t, x)["id"].(string))
		}
		if strings.Join(ids, ",") != "postgres,mysql,mariadb,redis,mongodb" {
			t.Fatalf("ids = %v", ids)
		}
		pg := obj(t, list[0])
		if pg["image"] != "postgres:18-alpine" || pg["volumeMount"] != "/var/lib/postgresql/data" || pg["defaultEnvKey"] != "DATABASE_URL" {
			t.Fatalf("postgres = %v", pg)
		}
		if n := len(pg["versions"].([]any)); n != 8 {
			t.Fatalf("static versions = %d", n)
		}
	})
	t.Run("live tags", func(t *testing.T) {
		e := setup(t, map[string][]string{"redis": {"9-alpine", "8-alpine", "9.0-rc1", "latest"}})
		_, body := e.do(t, http.MethodGet, "/engines", nil)
		redis := obj(t, body.([]any)[3])
		var tags []string
		for _, v := range redis["versions"].([]any) {
			tags = append(tags, obj(t, v)["tag"].(string))
		}
		if strings.Join(tags, ",") != "9-alpine,8-alpine,latest" || redis["image"] != "redis:8-alpine" {
			t.Fatalf("redis = %v", redis)
		}
	})
}

func TestCreateValidation(t *testing.T) {
	e := setup(t, nil)
	invalidEngine := "Invalid engine. Use one of: postgres, mysql, mariadb, redis, mongodb"
	cases := []struct {
		name   string
		body   any
		status int
		msg    string
	}{
		{"no body", nil, 400, "Name is required (max 120 characters)"},
		{"empty", map[string]any{}, 400, "Name is required (max 120 characters)"},
		{"blank name", map[string]any{"name": "    ", "engine": "postgres"}, 400, "Name is required (max 120 characters)"},
		{"numeric name", map[string]any{"name": 12, "engine": "postgres"}, 400, "Name is required (max 120 characters)"},
		{"long name", map[string]any{"name": strings.Repeat("d", 121), "engine": "postgres"}, 400, "Name is required (max 120 characters)"},
		{"missing engine", map[string]any{"name": "db"}, 400, invalidEngine},
		{"unknown engine", map[string]any{"name": "db", "engine": "oracle"}, 400, invalidEngine},
		{"engine case", map[string]any{"name": "db", "engine": "Postgres"}, 400, invalidEngine},
		{"numeric engine", map[string]any{"name": "db", "engine": 1}, 400, invalidEngine},
		{"bad version", map[string]any{"name": "db", "engine": "postgres", "version": "not a tag!"}, 400, ""},
		{"other repo", map[string]any{"name": "db", "engine": "redis", "tag": "mysql:8"}, 400, ""},
		{"image injection", map[string]any{"name": "db", "engine": "postgres", "image": "evil.io/postgres:18"}, 400, ""},
		{"tag injection", map[string]any{"name": "db", "engine": "postgres", "version": "18 --privileged"}, 400, ""},
		{"digest", map[string]any{"name": "db", "engine": "postgres", "version": "18@sha256:abc"}, 400, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec, body := e.do(t, http.MethodPost, "/", c.body)
			expectError(t, rec, body, c.status, c.msg)
			if c.msg == "" && !strings.HasPrefix(obj(t, body)["error"].(string), "Invalid version for ") {
				t.Fatalf("error = %v", body)
			}
		})
	}
	if len(e.cli.calls) != 0 {
		t.Fatalf("validation failures must not run docker: %v", e.cli.calls)
	}
	ps, err := e.d.Store.ListProjects(context.Background(), store.ProjectFilter{}, store.OrderNone)
	if err != nil || len(ps) != 0 {
		t.Fatalf("validation failures must not create rows: %v %v", ps, err)
	}
}

func TestCreate(t *testing.T) {
	cases := []struct {
		engine, version, image, mount string
		env                           []string
	}{
		{"postgres", "", "postgres:18-alpine", "/var/lib/postgresql", []string{"POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB"}},
		{"postgres", "postgres:16-alpine", "postgres:16-alpine", "/var/lib/postgresql/data", []string{"POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB"}},
		{"mysql", "8.0", "mysql:8.0", "/var/lib/mysql", []string{"MYSQL_USER", "MYSQL_PASSWORD", "MYSQL_DATABASE", "MYSQL_ROOT_PASSWORD"}},
		{"mariadb", "", "mariadb:11", "/var/lib/mysql", []string{"MYSQL_USER", "MYSQL_PASSWORD", "MYSQL_DATABASE", "MYSQL_ROOT_PASSWORD"}},
		{"redis", "7-alpine", "redis:7-alpine", "/data", []string{"REDIS_PASSWORD"}},
		{"mongodb", "7", "mongo:7", "/data/db", []string{"MONGO_INITDB_ROOT_USERNAME", "MONGO_INITDB_ROOT_PASSWORD", "MONGO_INITDB_DATABASE"}},
	}
	for _, c := range cases {
		t.Run(c.engine+" "+c.version, func(t *testing.T) {
			e := setup(t, nil)
			ctx := context.Background()
			req := map[string]any{"name": " My App DB ", "engine": c.engine}
			if c.version != "" {
				req["version"] = c.version
			}
			rec, raw := e.do(t, http.MethodPost, "/", req)
			if rec.Code != 201 {
				t.Fatalf("status %d: %v", rec.Code, raw)
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("credentials response must not be cached")
			}
			body := obj(t, raw)
			id := body["id"].(string)
			if body["name"] != "My App DB" || body["source_type"] != "managed" || body["project_type"] != "database" ||
				body["db_engine"] != c.engine || body["status"] != "pending" || body["build_type"] != "dockerfile" ||
				body["publish_host_port"] != false || body["auto_deploy"] != false || body["dockerfile_path"] != nil ||
				body["image"] != c.image || body["version"] != strings.TrimPrefix(c.image, strings.Split(c.image, ":")[0]+":") {
				t.Fatalf("body = %v", body)
			}
			if !strings.HasSuffix(body["description"].(string), body["version"].(string)+" managed by DockLift") {
				t.Fatalf("description = %v", body["description"])
			}
			engine := obj(t, body["engine"])
			if engine["id"] != c.engine || engine["image"] != c.image {
				t.Fatalf("engine = %v", engine)
			}
			svc := obj(t, body["services"].([]any)[0])
			if svc["port"] != nil || svc["name"] != "db" || svc["dockerfile_path"] != "[managed:"+c.engine+"|"+c.image+"]" {
				t.Fatalf("service = %v", svc)
			}
			vol := obj(t, body["persistent_volumes"].([]any)[0])
			if vol["mount_path"] != c.mount || vol["display_name"] != "data" || !strings.HasPrefix(vol["name"].(string), "dl-"+id[:8]+"-data-") {
				t.Fatalf("volume = %v", vol)
			}

			creds := obj(t, body["credentials"])
			pw := creds["password"].(string)
			if b, err := base64.RawURLEncoding.DecodeString(pw); err != nil || len(b) != 24 {
				t.Fatalf("password %q is not 24 random bytes base64url", pw)
			}
			if creds["username"] != "docklift" || creds["database"] != "my_app_db" {
				t.Fatalf("credentials = %v", creds)
			}
			if !strings.Contains(body["connection_url"].(string), pw+"@"+svc["container_name"].(string)+":") {
				t.Fatalf("connection_url = %v", body["connection_url"])
			}

			rows, err := e.d.Store.ListEnvVariables(ctx, store.EnvFilter{ProjectID: &id}, store.OrderNone)
			if err != nil {
				t.Fatal(err)
			}
			var keys []string
			for _, r := range rows {
				keys = append(keys, r.Key)
				wantSecret := strings.Contains(strings.ToLower(r.Key), "password")
				if r.ServiceName != "" || !*r.IsRuntime || *r.IsBuildArg || *r.IsSecret != wantSecret {
					t.Fatalf("env %s = %+v", r.Key, r)
				}
			}
			slices.Sort(keys)
			wantKeys := slices.Sorted(slices.Values(c.env))
			if !slices.Equal(keys, wantKeys) {
				t.Fatalf("env keys = %v, want %v", keys, c.env)
			}

			if len(e.cli.calls) != 1 {
				t.Fatalf("docker calls = %v", e.cli.calls)
			}
			want := []string{"docker", "volume", "create",
				"--label", "com.docker.compose.project=" + deploy.ComposeProjectName("My App DB", id),
				"--label", "com.docklift.project=" + id,
				"--label", "com.docklift.role=managed-database", vol["name"].(string)}
			if !slices.Equal(e.cli.calls[0], want) {
				t.Fatalf("docker args = %q, want %q", e.cli.calls[0], want)
			}
			readme, err := os.ReadFile(filepath.Join(e.d.Config.DeploymentsPath, id, "README.docklift-db.md"))
			if err != nil || !strings.Contains(string(readme), "("+c.image+")") || strings.Contains(string(readme), pw) {
				t.Fatalf("README = %q %v", readme, err)
			}
		})
	}
}

func TestCreateVolumeFailureRollsBack(t *testing.T) {
	cases := []struct {
		name     string
		engineUp bool
		result   execx.Result
		err      error
		status   int
		msg      string
	}{
		{"docker down", false, execx.Result{Stderr: []byte("Cannot connect to the Docker daemon")}, &execx.ExitError{Name: "docker", Code: 1},
			503, "Docker is not available"},
		{"stderr", true, execx.Result{Stderr: []byte("  volume quota exceeded\n")}, &execx.ExitError{Name: "docker", Code: 1},
			500, "volume quota exceeded"},
		{"silent exit", true, execx.Result{}, &execx.ExitError{Name: "docker", Code: 1},
			500, "Failed to create Docker volume for database"},
		{"cli missing", true, execx.Result{ExitCode: -1}, &execx.StartError{Name: "docker", Err: errors.New(`exec: "docker": executable file not found`)},
			500, "Failed to create Docker volume for database"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := setup(t, nil)
			if c.engineUp {
				e.d.DockerX.Engine = &fakeEngine{}
			}
			e.cli.result, e.cli.err = c.result, c.err
			rec, body := e.do(t, http.MethodPost, "/", map[string]any{"name": "db", "engine": "postgres"})
			expectError(t, rec, body, c.status, c.msg)
			ctx := context.Background()
			ps, err := e.d.Store.ListProjects(ctx, store.ProjectFilter{}, store.OrderNone)
			if err != nil || len(ps) != 0 {
				t.Fatalf("project rows left: %v %v", ps, err)
			}
			for _, q := range []string{"services", "env_variables", "persistent_volumes"} {
				var n int
				if err := e.d.DB.Reader().QueryRowContext(ctx, `SELECT COUNT(*) FROM "`+q+`"`).Scan(&n); err != nil || n != 0 {
					t.Fatalf("%s rows left: %d %v", q, n, err)
				}
			}
			entries, _ := os.ReadDir(e.d.Config.DeploymentsPath)
			if len(entries) != 0 {
				t.Fatalf("project directory left: %v", entries)
			}
		})
	}
}

// seedDB creates a managed database project with its service and env.
func seedDB(t *testing.T, d *app.Deps, name, engine, status string, envKV ...string) *store.Project {
	t.Helper()
	ctx := context.Background()
	p, err := d.Store.CreateProject(ctx, store.ProjectCreate{Name: name, ProjectType: store.Ptr("database"), DBEngine: &engine,
		Status: &status, SourceType: store.Ptr("managed")})
	if err != nil {
		t.Fatal(err)
	}
	cn := "dl_" + name + "_db"
	if _, err := d.Store.CreateService(ctx, store.ServiceCreate{ProjectID: p.ID, Name: "db", DockerfilePath: "[managed:" + engine + "]", ContainerName: &cn}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i+1 < len(envKV); i += 2 {
		if _, err := d.Store.CreateEnvVariable(ctx, store.EnvCreate{ProjectID: p.ID, Key: envKV[i], Value: envKV[i+1]}); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func seedApp(t *testing.T, d *app.Deps, name string, services ...string) *store.Project {
	t.Helper()
	ctx := context.Background()
	p, err := d.Store.CreateProject(ctx, store.ProjectCreate{Name: name, ProjectType: store.Ptr("app"), Status: store.Ptr("running")})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range services {
		if _, err := d.Store.CreateService(ctx, store.ServiceCreate{ProjectID: p.ID, Name: s, DockerfilePath: "Dockerfile"}); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

var pgEnv = []string{"POSTGRES_USER", "docklift", "POSTGRES_PASSWORD", "p@ss/w:rd", "POSTGRES_DB", "pg"}

func TestConnection(t *testing.T) {
	e := setup(t, nil)
	ctx := context.Background()
	pg := seedDB(t, e.d, "pg", "postgres", "running", pgEnv...)
	redis := seedDB(t, e.d, "rd", "redis", "running", "REDIS_PASSWORD", "rpw")
	noCreds := seedDB(t, e.d, "my", "mysql", "running", "MYSQL_USER", "u")
	appP := seedApp(t, e.d, "web", "app")

	rec, raw := e.do(t, http.MethodGet, "/"+pg.ID+"/connection", nil)
	body := obj(t, raw)
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status %d %v", rec.Code, raw)
	}
	want := map[string]any{"host": "dl_pg_db", "port": float64(5432), "username": "docklift", "database": "pg", "password": "p@ss/w:rd",
		"connection_url": "postgresql://docklift:p%40ss%2Fw%3Ard@dl_pg_db:5432/pg", "internal_only": true, "publish_host_port": false,
		"host_port": nil, "credentials_init_only": true,
		"note": "Reachable from linked apps on the app Docker network via the host above. Prefer linking over publishing host ports."}
	for k, v := range want {
		if body[k] != v {
			t.Fatalf("%s = %#v, want %#v", k, body[k], v)
		}
	}
	if obj(t, body["engine"])["image"] != "postgres:18-alpine" {
		t.Fatalf("engine = %v", body["engine"])
	}

	_, raw = e.do(t, http.MethodGet, "/"+redis.ID+"/connection", nil)
	if b := obj(t, raw); b["username"] != nil || b["database"] != "0" || b["connection_url"] != "redis://:rpw@dl_rd_db:6379" {
		t.Fatalf("redis = %v", b)
	}

	// A published host port is reported, with the warning note.
	if _, err := e.d.Store.UpdateProject(ctx, pg.ID, store.ProjectUpdate{PublishHostPort: store.Some(true)}); err != nil {
		t.Fatal(err)
	}
	_, raw = e.do(t, http.MethodGet, "/"+pg.ID+"/connection", nil)
	if b := obj(t, raw); b["internal_only"] != true || b["publish_host_port"] != true || b["host_port"] != nil {
		t.Fatalf("publish without port = %v", b)
	}
	if _, err := e.d.Store.UpdateServices(ctx, store.ServiceFilter{ProjectID: &pg.ID}, store.ServiceUpdate{Port: store.Some(store.Ptr(5600))}); err != nil {
		t.Fatal(err)
	}
	_, raw = e.do(t, http.MethodGet, "/"+pg.ID+"/connection", nil)
	if b := obj(t, raw); b["internal_only"] != false || b["host_port"] != float64(5600) || !strings.HasPrefix(b["note"].(string), "Host port 5600 is published") {
		t.Fatalf("published = %v", b)
	}

	for _, id := range []string{noCreds.ID, appP.ID, "missing"} {
		rec, raw := e.do(t, http.MethodGet, "/"+id+"/connection", nil)
		expectError(t, rec, raw, 404, "Managed database not found or credentials missing")
	}
}

func TestLinks(t *testing.T) {
	e := setup(t, nil)
	eng := &fakeEngine{}
	e.d.DockerX.Engine = eng
	ctx := context.Background()
	pg := seedDB(t, e.d, "pg", "postgres", "running", pgEnv...)
	other := seedDB(t, e.d, "pg2", "postgres", "degraded", "POSTGRES_USER", "u2", "POSTGRES_PASSWORD", "p2", "POSTGRES_DB", "d2")
	stopped := seedDB(t, e.d, "rd", "redis", "stopped", "REDIS_PASSWORD", "x")
	noCreds := seedDB(t, e.d, "my", "mysql", "running")
	webApp := seedApp(t, e.d, "web", "app", "worker")
	links := "/" + pg.ID + "/links"

	errs := []struct {
		name, path string
		body       any
		status     int
		msg        string
	}{
		{"no body", links, nil, 400, "app_project_id is required"},
		{"blank app", links, map[string]any{"app_project_id": "  "}, 400, "app_project_id is required"},
		{"numeric app", links, map[string]any{"app_project_id": 5}, 400, "app_project_id is required"},
		{"missing app", links, map[string]any{"app_project_id": "nope"}, 404, "App project not found"},
		{"database as app", links, map[string]any{"app_project_id": other.ID}, 404, "App project not found"},
		{"unknown service", links, map[string]any{"app_project_id": webApp.ID, "service_name": "api"}, 400, `Service "api" not found on that project`},
		{"bad key", links, map[string]any{"app_project_id": webApp.ID, "env_key": "bad-key"}, 400, "Invalid env_key"},
		{"digit key", links, map[string]any{"app_project_id": webApp.ID, "env_key": "1KEY"}, 400, "Invalid env_key"},
		{"long key", links, map[string]any{"app_project_id": webApp.ID, "env_key": strings.Repeat("K", 65)}, 400, "Invalid env_key"},
		{"missing db", "/nope/links", map[string]any{"app_project_id": webApp.ID}, 404, "Database not found"},
		{"app as db", "/" + webApp.ID + "/links", map[string]any{"app_project_id": webApp.ID}, 404, "Database not found"},
		{"no creds", "/" + noCreds.ID + "/links", map[string]any{"app_project_id": webApp.ID}, 400, "Database credentials missing — deploy the database first"},
		{"stopped", "/" + stopped.ID + "/links", map[string]any{"app_project_id": webApp.ID}, 409, "Database must be running before linking. Deploy it first."},
	}
	for _, c := range errs {
		t.Run(c.name, func(t *testing.T) {
			rec, body := e.do(t, http.MethodPost, c.path, c.body)
			expectError(t, rec, body, c.status, c.msg)
		})
	}
	if n, _ := e.d.Store.CountLinks(ctx, store.LinkFilter{}); n != 0 {
		t.Fatalf("failed links created %d rows", n)
	}

	// An existing manual env value is only replaced with overwrite=true.
	if _, err := e.d.Store.CreateEnvVariable(ctx, store.EnvCreate{ProjectID: webApp.ID, Key: "PG_URL", Value: "manual"}); err != nil {
		t.Fatal(err)
	}
	rec, body := e.do(t, http.MethodPost, links, map[string]any{"app_project_id": webApp.ID, "env_key": "PG_URL"})
	expectError(t, rec, body, 409, `"PG_URL" already exists on this project/service. Pass overwrite=true to replace it with the managed database URL.`)
	rec, body = e.do(t, http.MethodPost, links, map[string]any{"app_project_id": webApp.ID, "env_key": "PG_URL", "overwrite": "true"})
	expectError(t, rec, body, 409, "")
	rec, raw := e.do(t, http.MethodPost, links, map[string]any{"app_project_id": webApp.ID, "env_key": "PG_URL", "overwrite": true})
	if rec.Code != 201 {
		t.Fatalf("overwrite: %d %v", rec.Code, raw)
	}
	b := obj(t, raw)
	if b["env_key"] != "PG_URL" || b["service_name"] != nil || b["network_attached"] != true || b["network_error"] != nil ||
		b["note"] != "Connection URL injected as a runtime secret. Redeploy the app for containers to pick up the new env." {
		t.Fatalf("overwrite body = %v", b)
	}
	url := "postgresql://docklift:p%40ss%2Fw%3Ard@dl_pg_db:5432/pg"
	envRow, err := e.d.Store.FindEnvVariable(ctx, store.EnvFilter{ProjectID: &webApp.ID, ServiceName: store.Ptr(""), Key: store.Ptr("PG_URL")})
	if err != nil || envRow.Value != url || !*envRow.IsSecret || !*envRow.IsRuntime || *envRow.IsBuildArg {
		t.Fatalf("overwritten env = %+v %v", envRow, err)
	}
	if !slices.Equal(eng.connected, []string{deploy.ProjectNetworkName(webApp.ID) + " dl_pg_db"}) {
		t.Fatalf("network connects = %v", eng.connected)
	}

	// Service-scoped link with the engine default key; relinking reuses the row.
	_, raw = e.do(t, http.MethodPost, links, map[string]any{"app_project_id": " " + webApp.ID + " ", "service_name": " app "})
	first := obj(t, raw)
	if first["env_key"] != "DATABASE_URL" || first["service_name"] != "app" {
		t.Fatalf("scoped link = %v", first)
	}
	_, raw = e.do(t, http.MethodPost, links, map[string]any{"app_project_id": webApp.ID, "service_name": "app"})
	again := obj(t, raw)
	firstID := obj(t, first["link"])["id"]
	if obj(t, again["link"])["id"] != firstID {
		t.Fatal("relink must reuse the link row")
	}

	// Another database cannot take a key this one owns, even with overwrite.
	rec, body = e.do(t, http.MethodPost, "/"+other.ID+"/links", map[string]any{"app_project_id": webApp.ID, "service_name": "app", "overwrite": true})
	expectError(t, rec, body, 409, `"DATABASE_URL" on this project/service is already linked to database "pg". Unlink that first.`)

	// Network attach failures are reported, the link is still saved.
	e.d.DockerX.Engine = nil
	rec, raw = e.do(t, http.MethodPost, "/"+other.ID+"/links", map[string]any{"app_project_id": webApp.ID, "service_name": "worker"})
	b = obj(t, raw)
	if rec.Code != 201 || b["network_attached"] != false || b["network_error"] != "Docker is not available" ||
		b["note"] != "Connection URL saved. Network attach pending (Docker is not available). Redeploy the app (and ensure the database is running) to join networks." {
		t.Fatalf("detached link = %d %v", rec.Code, b)
	}
	otherLinkID := obj(t, b["link"])["id"].(string)
	e.d.DockerX.Engine = eng

	// Listings.
	_, raw = e.do(t, http.MethodGet, links, nil)
	listed := raw.([]any)
	if len(listed) != 2 {
		t.Fatalf("links = %v", listed)
	}
	appRef := obj(t, obj(t, listed[0])["app_project"])
	if appRef["id"] != webApp.ID || len(appRef["services"].([]any)) != 2 {
		t.Fatalf("app_project = %v", appRef)
	}
	_, raw = e.do(t, http.MethodGet, "/links/by-app/"+webApp.ID, nil)
	if byApp := raw.([]any); len(byApp) != 3 || obj(t, obj(t, byApp[0])["database_project"])["db_engine"] != "postgres" {
		t.Fatalf("by-app = %v", byApp)
	}
	_, raw = e.do(t, http.MethodGet, "/links/by-app/missing", nil)
	if len(raw.([]any)) != 0 {
		t.Fatalf("by-app missing = %v", raw)
	}
	_, raw = e.do(t, http.MethodGet, "/", nil)
	dbs := raw.([]any)
	if len(dbs) != 4 {
		t.Fatalf("list = %v", dbs)
	}
	seenPG := false
	for _, x := range dbs {
		m := obj(t, x)
		if m["id"] == pg.ID {
			seenPG = true
		}
		if m["id"] == pg.ID && len(m["databaseLinksAsDb"].([]any)) != 2 {
			t.Fatalf("pg links on listing = %v", m["databaseLinksAsDb"])
		}
	}
	if !seenPG {
		t.Fatal("pg missing from listing")
	}
	rec, body = e.do(t, http.MethodGet, "/"+webApp.ID+"/links", nil)
	expectError(t, rec, body, 404, "Database not found")

	// Unlink: wrong database is 404; the env goes with the last owner.
	rec, body = e.do(t, http.MethodDelete, "/"+other.ID+"/links/"+firstID.(string), nil)
	expectError(t, rec, body, 404, "Link not found")
	rec, raw = e.do(t, http.MethodDelete, links+"/"+firstID.(string), nil)
	if rec.Code != 200 || obj(t, raw)["status"] != "unlinked" {
		t.Fatalf("unlink = %d %v", rec.Code, raw)
	}
	rec, body = e.do(t, http.MethodDelete, links+"/"+firstID.(string), nil)
	expectError(t, rec, body, 404, "Link not found")
	if _, err := e.d.Store.FindEnvVariable(ctx, store.EnvFilter{ProjectID: &webApp.ID, ServiceName: store.Ptr("app"), Key: store.Ptr("DATABASE_URL")}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unlinked env must be removed: %v", err)
	}
	// pg still has the PG_URL link to the app: the network stays attached.
	if len(eng.connected) == 0 {
		t.Fatal("network detached while a link remains")
	}
	if rec, _ := e.do(t, http.MethodDelete, "/"+other.ID+"/links/"+otherLinkID, nil); rec.Code != 200 {
		t.Fatalf("unlink other = %d", rec.Code)
	}
}
