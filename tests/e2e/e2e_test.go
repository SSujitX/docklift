package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestEndToEnd drives a real Docklift install through its HTTP API the way
// a user does: first admin, a GitHub project deployed with Docker, env vars,
// stop/restart, failing and cancelled builds, rollback, managed databases,
// volumes, a ZIP project, the file editor, logs/stats, the terminal,
// backup/restore, purge and project teardown. See README.md.
//
// Steps share state and run in order; a failed step stops the run.
func TestEndToEnd(t *testing.T) {
	cfg := loadConfig(t)
	e := &run{cfg: cfg, c: &client{base: cfg.baseURL, http: &http.Client{Timeout: 0}}}
	steps := []struct {
		name string
		f    func(t *testing.T)
	}{
		{"admin", e.admin},
		{"git project deploys", e.gitDeploy},
		{"app answers in its network and through the proxy", e.reachable},
		{"env vars reach the container exactly", e.envVars},
		{"stop and restart", e.stopRestart},
		{"zip upload project", e.zipProject},
		{"failing build releases the lock", e.failingBuild},
		{"second deploy is refused, cancel mid-build", e.cancelBuild},
		{"rollback to the previous image", e.rollback},
		{"managed databases", e.databases},
		{"storage volume persists across redeploy", e.storage},
		{"file editor", e.fileEditor},
		{"container logs and stats", e.logsStats},
		{"terminal", e.terminal},
		{"system endpoints", e.system},
		{"backup and restore", e.backupRestore},
		{"purge keeps in-use and rollback images", e.purge},
		{"project delete tears everything down", e.deleteProjects},
	}
	for _, s := range steps {
		if !t.Run(s.name, s.f) {
			t.Fatalf("step %q failed; stopping", s.name)
		}
	}
}

type run struct {
	cfg config
	c   *client

	app        map[string]any // git project row
	appID      string
	service    map[string]any // its single service row
	domain     string
	dbIDs      []string
	zipID      string
	firstImage string // image of the first successful deploy (rollback target)
	firstDepID string
}

func (e *run) needDocker(t *testing.T) {
	t.Helper()
	if !e.cfg.docker {
		t.Skip("host-side checks need the docker CLI (DOCKLIFT_E2E_DOCKER=1)")
	}
}

// ---------------------------------------------------------------- steps --

func (e *run) admin(t *testing.T) {
	e.c.get(t, "/api/health").want(t, 200)
	st := e.c.get(t, "/api/auth/status").want(t, 200).obj(t)
	if n, _ := st["userCount"].(float64); n == 0 {
		if e.cfg.bootstrap == "" {
			t.Fatal("server needs its first admin: set DOCKLIFT_E2E_BOOTSTRAP_SECRET")
		}
		r := e.c.do(t, "POST", "/api/auth/register", map[string]any{"email": e.cfg.email, "password": e.cfg.password, "name": "E2E Admin"},
			map[string]string{"x-bootstrap-secret": e.cfg.bootstrap}).want(t, 201).obj(t)
		e.c.token = str(r["token"])
	} else {
		r := e.c.post(t, "/api/auth/login", map[string]any{"email": e.cfg.email, "password": e.cfg.password}).want(t, 200).obj(t)
		e.c.token = str(r["token"])
	}
	if e.c.token == "" {
		t.Fatal("no session token")
	}
	me := e.c.get(t, "/api/auth/me").want(t, 200).obj(t)
	if u, _ := me["user"].(map[string]any); str(u["email"]) != e.cfg.email || u["role"] != "admin" {
		t.Fatalf("me = %v", me)
	}
}

func (e *run) deploy(t *testing.T, projectID string) (string, map[string]any) {
	t.Helper()
	s := e.c.open(t, "POST", "/api/deployments/"+projectID+"/deploy", nil)
	if s.Status != 200 {
		t.Fatalf("deploy: HTTP %d: %s", s.Status, s.all(10*time.Second))
	}
	log := s.all(30 * time.Minute)
	return log, e.lastDeployment(t, projectID)
}

func (e *run) mustDeploy(t *testing.T, projectID string) map[string]any {
	t.Helper()
	log, d := e.deploy(t, projectID)
	if !strings.Contains(log, "DEPLOY SUCCESSFUL") || d["status"] != "success" {
		t.Fatalf("deploy did not succeed (status %v):\n%s", d["status"], tail(log, 4000))
	}
	return d
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

func (e *run) lastDeployment(t *testing.T, projectID string) map[string]any {
	t.Helper()
	list := e.c.get(t, "/api/deployments/"+projectID).want(t, 200).arr(t)
	if len(list) == 0 {
		t.Fatal("no deployments")
	}
	return list[0].(map[string]any)
}

func (e *run) services(t *testing.T, projectID string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, s := range e.c.get(t, "/api/deployments/"+projectID+"/services").want(t, 200).arr(t) {
		out = append(out, s.(map[string]any))
	}
	return out
}

func (e *run) projectStatus(t *testing.T, id string) string {
	t.Helper()
	return str(e.c.get(t, "/api/projects/"+id).want(t, 200).obj(t)["status"])
}

func (e *run) gitDeploy(t *testing.T) {
	name := "e2e-git-" + randHex(3)
	e.app = e.c.post(t, "/api/projects", map[string]any{
		"name": name, "source_type": "github", "github_url": e.cfg.repo, "github_branch": e.cfg.repoBranch,
		"base_directory": e.cfg.repoDir, "internal_port": e.cfg.repoPort,
	}).want(t, 201).obj(t)
	e.appID = str(e.app["id"])
	d := e.mustDeploy(t, e.appID)
	e.firstDepID = str(d["id"])
	tags, _ := d["image_tags"].(map[string]any)
	e.firstImage = str(tags["app"])
	if !strings.HasPrefix(e.firstImage, "docklift-"+e.appID[:8]+"-app:") {
		t.Fatalf("image_tags = %v", d["image_tags"])
	}
	if st := e.projectStatus(t, e.appID); st != "running" {
		t.Fatalf("project status %q after deploy", st)
	}
	svcs := e.services(t, e.appID)
	if len(svcs) != 1 || svcs[0]["status"] != "running" {
		t.Fatalf("services = %v", svcs)
	}
	e.service = svcs[0]
}

func (e *run) container() string { return str(e.service["container_name"]) }

// inNetwork fetches a URL from inside the nginx proxy container, which is
// attached to every project network.
func (e *run) inNetwork(t *testing.T, rawURL string) (string, error) {
	t.Helper()
	return sh(t, "docker", "exec", "docklift-nginx-proxy", "wget", "-qO-", "-T", "10", rawURL)
}

func (e *run) reachable(t *testing.T) {
	e.needDocker(t)
	eventually(t, "app answers inside its network", 60*time.Second, func() error {
		out, err := e.inNetwork(t, fmt.Sprintf("http://%s:%d/", e.container(), e.cfg.repoPort))
		if err != nil || !strings.Contains(out, "Hello from Docklift") {
			return fmt.Errorf("%v: %s", err, out)
		}
		return nil
	})
	e.domain = e.cfg.domain
	if e.domain == "" {
		e.domain = "e2e-" + randHex(3) + ".docklift.test"
	}
	r := e.c.put(t, "/api/deployments/"+e.appID+"/services/"+str(e.service["id"]), map[string]any{"domain": e.domain}).want(t, 200).obj(t)
	if r["success"] != true {
		t.Fatalf("domain update: %v", r)
	}
	conf := filepath.Join(e.cfg.installDir, "nginx-proxy", "conf.d", "service-"+str(e.service["id"])+".conf")
	b, err := os.ReadFile(conf)
	if err != nil {
		t.Fatalf("nginx conf for the service: %v", err)
	}
	if !strings.Contains(string(b), "server_name "+e.domain) || !strings.Contains(string(b), e.container()) {
		t.Fatalf("nginx conf does not route %s to %s:\n%s", e.domain, e.container(), b)
	}
	mustSh(t, "docker", "exec", "docklift-nginx-proxy", "nginx", "-t")
	eventually(t, "app answers through the proxy", 60*time.Second, func() error {
		code, body, err := httpGet(e.cfg.proxyURL+"/", e.domain)
		if err != nil || code != 200 || !strings.Contains(body, "Hello from Docklift") {
			return fmt.Errorf("HTTP %d %v: %s", code, err, truncate(body, 300))
		}
		return nil
	})
}

// envCases are awkward values. want is what the container must see byte for
// byte: values are normalised when stored, as TS normalizeEnvValue did
// (outer whitespace trimmed, one pair of surrounding quotes removed).
var envCases = []struct{ key, value, want string }{
	{"E2E_SPACES", "  inner  spaces  kept  ", "inner  spaces  kept"},
	{"E2E_QUOTES", `it's "quoted" \back\slash`, `it's "quoted" \back\slash`},
	{"E2E_WRAPPED", `"wrapped in quotes"`, "wrapped in quotes"},
	{"E2E_HASH", "value # not a comment", "value # not a comment"},
	{"E2E_UNICODE", "héllo wörld — 日本語 ✅", "héllo wörld — 日本語 ✅"},
	{"E2E_MULTILINE", "line one\nline two\n\tindented line three", "line one\nline two\n\tindented line three"},
	{"E2E_EQUALS", "a=b=c;d&e|f<g>h", "a=b=c;d&e|f<g>h"},
	{"E2E_YAMLISH", "true", "true"},
	{"E2E_NUMBER", "007", "007"},
}

// envDollar documents compose interpolation of `$` (kept behaviour).
const envDollarKey, envDollarValue = "E2E_DOLLAR", "cost $5 and ${HOME} and $$"

func (e *run) containerEnv(t *testing.T, container string) map[string]string {
	t.Helper()
	out := mustSh(t, "docker", "exec", container, "cat", "/proc/1/environ")
	m := map[string]string{}
	for _, kv := range strings.Split(out, "\x00") {
		if k, v, ok := strings.Cut(kv, "="); ok {
			m[k] = v
		}
	}
	return m
}

func (e *run) envVars(t *testing.T) {
	for _, c := range envCases {
		e.c.post(t, "/api/projects/"+e.appID+"/env", map[string]any{"key": c.key, "value": c.value}).want(t, 201)
	}
	e.c.post(t, "/api/projects/"+e.appID+"/env", map[string]any{"key": envDollarKey, "value": envDollarValue}).want(t, 201)
	list := e.c.get(t, "/api/projects/"+e.appID+"/env").want(t, 200).arr(t)
	stored := map[string]string{}
	for _, v := range list {
		m := v.(map[string]any)
		stored[str(m["key"])] = str(m["value"])
	}
	for _, c := range envCases {
		if stored[c.key] != c.want {
			t.Errorf("stored %s = %q, want %q", c.key, stored[c.key], c.want)
		}
	}
	e.mustDeploy(t, e.appID)
	e.refreshService(t)
	if !e.cfg.docker {
		return
	}
	got := e.containerEnv(t, e.container())
	for _, c := range envCases {
		if v, ok := got[c.key]; !ok || v != c.want {
			t.Errorf("container %s = %q (present %v), want %q", c.key, v, ok, c.want)
		}
	}
	if got["PORT"] != fmt.Sprint(e.cfg.repoPort) {
		t.Errorf("container PORT = %q", got["PORT"])
	}
	t.Logf("compose interpolation (kept behaviour): %s stored %q, container sees %q", envDollarKey, envDollarValue, got[envDollarKey])
}

func (e *run) refreshService(t *testing.T) {
	t.Helper()
	svcs := e.services(t, e.appID)
	if len(svcs) != 1 {
		t.Fatalf("services = %v", svcs)
	}
	e.service = svcs[0]
}

func (e *run) action(t *testing.T, projectID, action string, body any) string {
	t.Helper()
	s := e.c.open(t, "POST", "/api/deployments/"+projectID+"/"+action, body)
	if s.Status != 200 {
		t.Fatalf("%s: HTTP %d: %s", action, s.Status, s.all(10*time.Second))
	}
	return s.all(10 * time.Minute)
}

func (e *run) containerRunning(t *testing.T, name string) bool {
	t.Helper()
	out, err := sh(t, "docker", "inspect", "-f", "{{.State.Running}}", name)
	return err == nil && strings.TrimSpace(out) == "true"
}

func (e *run) startedAt(t *testing.T) string {
	t.Helper()
	return strings.TrimSpace(mustSh(t, "docker", "inspect", "-f", "{{.State.StartedAt}}", e.container()))
}

// stopRestart follows the dashboard: Restart is offered while the project
// runs (compose restart), Stop removes the containers (compose down), and a
// stopped project is started again with Deploy. (As in TS, restart after
// stop finds no containers and still reports success.)
func (e *run) stopRestart(t *testing.T) {
	before := ""
	if e.cfg.docker {
		before = e.startedAt(t)
	}
	if log := e.action(t, e.appID, "restart", nil); !strings.Contains(log, "RESTART SUCCESSFUL") {
		t.Fatalf("restart log:\n%s", log)
	}
	eventually(t, "running after restart", 60*time.Second, func() error {
		if st := e.projectStatus(t, e.appID); st != "running" {
			return fmt.Errorf("status %q", st)
		}
		return nil
	})
	if e.cfg.docker {
		if !e.containerRunning(t, e.container()) {
			t.Fatal("container not running after restart")
		}
		if after := e.startedAt(t); after == before {
			t.Fatalf("container was not restarted (StartedAt %s)", after)
		}
	}
	if log := e.action(t, e.appID, "stop", nil); !strings.Contains(log, "STOP SUCCESSFUL") {
		t.Fatalf("stop log:\n%s", log)
	}
	if st := e.projectStatus(t, e.appID); st != "stopped" {
		t.Fatalf("status after stop = %q", st)
	}
	if e.cfg.docker && e.containerRunning(t, e.container()) {
		t.Fatal("container still running after stop")
	}
	e.mustDeploy(t, e.appID)
	if st := e.projectStatus(t, e.appID); st != "running" {
		t.Fatalf("status after deploying a stopped project = %q", st)
	}
	if e.cfg.docker {
		eventually(t, "app answers after stop + deploy", 60*time.Second, func() error {
			code, body, err := httpGet(e.cfg.proxyURL+"/", e.domain)
			if err != nil || code != 200 || !strings.Contains(body, "Hello from Docklift") {
				return fmt.Errorf("HTTP %d %v: %s", code, err, truncate(body, 300))
			}
			return nil
		})
	}
}

func (e *run) readFile(t *testing.T, projectID, path string) string {
	t.Helper()
	r := e.c.get(t, "/api/files/"+projectID+"/content?path="+urlQuery(path)).want(t, 200).obj(t)
	return str(r["content"])
}

func (e *run) writeFile(t *testing.T, projectID, path, content string) {
	t.Helper()
	e.c.put(t, "/api/files/"+projectID+"/content?path="+urlQuery(path), map[string]any{"content": content}).want(t, 200)
}

// The ZIP project serves page.txt with busybox httpd. Its files are edited
// through the file editor API; a GitHub project would not do, because every
// deploy resets its checkout to the remote branch (TS pullRepo).
const zipDockerfile = `FROM busybox:1.37
COPY page.txt /www/index.html
EXPOSE 8080
CMD ["httpd", "-f", "-v", "-p", "8080", "-h", "/www"]
`

func (e *run) zipContainer(t *testing.T) string {
	t.Helper()
	svcs := e.services(t, e.zipID)
	if len(svcs) != 1 {
		t.Fatalf("zip services = %v", svcs)
	}
	return str(svcs[0]["container_name"])
}

// zipServes waits until the ZIP app answers with want.
func (e *run) zipServes(t *testing.T, want string) {
	t.Helper()
	if !e.cfg.docker {
		return
	}
	name := e.zipContainer(t)
	eventually(t, "zip app serves "+want, 60*time.Second, func() error {
		out, err := e.inNetwork(t, "http://"+name+":8080/")
		if err != nil || strings.TrimSpace(out) != want {
			return fmt.Errorf("%v: %q", err, out)
		}
		return nil
	})
}

func (e *run) zipProject(t *testing.T) {
	z := buildZip(t, map[string]string{"Dockerfile": zipDockerfile, "page.txt": "zip-v1\n", "README.md": "e2e zip project\n"})
	p := e.c.post(t, "/api/projects", form(map[string]string{"name": "e2e-zip-" + randHex(3), "internal_port": "8080"}, "files", "app.zip", z)).want(t, 201).obj(t)
	e.zipID = str(p["id"])
	if p["source_type"] != "upload" {
		t.Fatalf("source_type = %v", p["source_type"])
	}
	d := e.mustDeploy(t, e.zipID)
	e.firstDepID = str(d["id"])
	tags, _ := d["image_tags"].(map[string]any)
	e.firstImage = str(tags["app"])
	e.zipServes(t, "zip-v1")
}

func (e *run) failingBuild(t *testing.T) {
	e.writeFile(t, e.zipID, "Dockerfile", "FROM busybox:1.37\nRUN echo e2e-broken-build && exit 3\n")
	log, d := e.deploy(t, e.zipID)
	// A build error ends the stream with "❌ Error: docker exited with code N"
	// (the TS catch path), not the DEPLOY FAILED banner of a failed compose up.
	if d["status"] != "failed" || !strings.Contains(log, "❌") || !strings.Contains(log, "e2e-broken-build") {
		t.Fatalf("broken build: status %v\n%s", d["status"], tail(log, 3000))
	}
	if st := e.projectStatus(t, e.zipID); st == "building" {
		t.Fatalf("project still building after a failed deploy")
	}
	// The previous container keeps serving.
	e.zipServes(t, "zip-v1")
	// The lock is released: the next deploy runs and succeeds.
	e.writeFile(t, e.zipID, "Dockerfile", zipDockerfile)
	d = e.mustDeploy(t, e.zipID)
	e.firstDepID = str(d["id"])
	tags, _ := d["image_tags"].(map[string]any)
	e.firstImage = str(tags["app"])
	e.zipServes(t, "zip-v1")
}

func (e *run) cancelBuild(t *testing.T) {
	// A slow step, cache-busted so it runs every time.
	slow := strings.Replace(zipDockerfile, "COPY page.txt", "RUN echo e2e-slow-step-"+randHex(4)+" && sleep 300\nCOPY page.txt", 1)
	e.writeFile(t, e.zipID, "Dockerfile", slow)
	s := e.c.open(t, "POST", "/api/deployments/"+e.zipID+"/deploy", nil)
	if s.Status != 200 {
		t.Fatalf("deploy: HTTP %d", s.Status)
	}
	defer s.close()
	if !s.until("e2e-slow-step", 10*time.Minute) {
		t.Fatalf("slow step never started:\n%s", tail(s.text(), 3000))
	}
	r := e.c.post(t, "/api/deployments/"+e.zipID+"/deploy", nil)
	if r.Status != 409 {
		t.Fatalf("second deploy while one runs: HTTP %d %s", r.Status, r.Body)
	}
	start := time.Now()
	clog := e.action(t, e.zipID, "cancel", nil)
	log := s.all(2 * time.Minute)
	if time.Since(start) > 90*time.Second {
		t.Errorf("cancel took %s", time.Since(start))
	}
	d := e.lastDeployment(t, e.zipID)
	if d["status"] != "cancelled" {
		t.Fatalf("status after cancel = %v\ncancel:\n%s\ndeploy:\n%s", d["status"], clog, tail(log, 2000))
	}
	if !strings.Contains(log, "❌ Error: Deployment cancelled") {
		t.Errorf("deploy stream does not report the cancel cleanly:\n%s", tail(log, 2000))
	}
	// Cancel tears the project down for a fresh start, as in TS.
	if !strings.Contains(clog, "Cancelled — containers down") {
		t.Fatalf("cancel log:\n%s", clog)
	}
	if st := e.projectStatus(t, e.zipID); st != "stopped" {
		t.Fatalf("status after cancel = %q", st)
	}
	if e.cfg.docker {
		if out, _ := sh(t, "docker", "ps", "--filter", "ancestor=busybox:1.37", "--format", "{{.Command}}"); strings.Contains(out, "sleep 300") {
			t.Errorf("build step still running after cancel: %s", out)
		}
	}
	// The lock is free: the next deploy works and serves the app again.
	e.writeFile(t, e.zipID, "Dockerfile", zipDockerfile)
	d = e.mustDeploy(t, e.zipID)
	e.firstDepID = str(d["id"])
	tags, _ := d["image_tags"].(map[string]any)
	e.firstImage = str(tags["app"])
	e.zipServes(t, "zip-v1")
}

func (e *run) runningImage(t *testing.T) string {
	t.Helper()
	return strings.TrimSpace(mustSh(t, "docker", "inspect", "-f", "{{.Config.Image}}", e.container()))
}

// rollback deploys new content (v2) and rolls back to the v1 deployment: the
// v1 image runs again and serves v1, without a rebuild.
func (e *run) rollback(t *testing.T) {
	e.writeFile(t, e.zipID, "page.txt", "zip-v2\n")
	e.mustDeploy(t, e.zipID)
	e.zipServes(t, "zip-v2")
	// Step-up: a wrong password is refused.
	e.c.post(t, "/api/deployments/"+e.zipID+"/rollback", map[string]any{"password": "wrong-" + randHex(2), "deploymentId": e.firstDepID}).want(t, 403)
	log := e.action(t, e.zipID, "rollback", map[string]any{"password": e.cfg.password, "deploymentId": e.firstDepID})
	if !strings.Contains(log, "ROLLBACK SUCCESSFUL") {
		t.Fatalf("rollback log:\n%s", tail(log, 3000))
	}
	if e.cfg.docker {
		img := strings.TrimSpace(mustSh(t, "docker", "inspect", "-f", "{{.Config.Image}}", e.zipContainer(t)))
		if img != e.firstImage {
			t.Fatalf("container runs %s after rollback, want %s", img, e.firstImage)
		}
	}
	e.zipServes(t, "zip-v1")
}

// dbProbe returns a command (run in a throwaway container of the database's
// own image on the app's network) that proves the URL authenticates.
func dbProbe(engine, connURL string) []string {
	switch engine {
	case "postgres":
		return []string{"psql", connURL, "-tAc", "select 41+1"}
	case "redis":
		return []string{"redis-cli", "-u", connURL, "ping"}
	case "mysql", "mariadb":
		// The mysql/mariadb clients take no URL: pass its parts.
		u, err := url.Parse(connURL)
		if err != nil {
			return nil
		}
		pw, _ := u.User.Password()
		port := u.Port()
		if port == "" {
			port = "3306"
		}
		return []string{engine, "--protocol=TCP", "-h", u.Hostname(), "-P", port, "-u", u.User.Username(), "-p" + pw,
			strings.TrimPrefix(u.Path, "/"), "-N", "-e", "select 41+1"}
	case "mongodb", "mongo":
		return []string{"mongosh", "--quiet", connURL, "--eval", "db.runCommand({ping:1}).ok"}
	}
	return nil
}

var dbProbeWant = map[string]string{"postgres": "42", "redis": "PONG", "mysql": "42", "mariadb": "42", "mongodb": "1", "mongo": "1"}

func (e *run) databases(t *testing.T) {
	for i, engine := range e.cfg.engines {
		t.Run(engine, func(t *testing.T) {
			db := e.c.post(t, "/api/databases", map[string]any{"name": "e2e-" + engine + "-" + randHex(2), "engine": engine}).want(t, 201).obj(t)
			id := str(db["id"])
			keep := i == 0 || engine == "postgres" || engine == "redis"
			if keep {
				e.dbIDs = append(e.dbIDs, id)
			} else {
				defer func() {
					e.c.del(t, "/api/projects/"+id).want(t, 200)
				}()
			}
			if !strings.Contains(str(db["connection_url"]), "://") {
				t.Fatalf("connection_url = %v", db["connection_url"])
			}
			e.mustDeploy(t, id)
			if st := e.projectStatus(t, id); st != "running" {
				t.Fatalf("database status %q", st)
			}
			// Engines share default keys (DATABASE_URL), so each link names its own.
			link := e.c.post(t, "/api/databases/"+id+"/links", map[string]any{"app_project_id": e.appID, "env_key": "E2E_" + strings.ToUpper(engine) + "_URL"}).want(t, 201).obj(t)
			envKey := str(link["env_key"])
			if envKey == "" {
				if l, ok := link["link"].(map[string]any); ok {
					envKey = str(l["env_key"])
				}
			}
			if envKey == "" {
				t.Fatalf("link response has no env_key: %v", link)
			}
			e.mustDeploy(t, e.appID)
			e.refreshService(t)
			if !e.cfg.docker {
				return
			}
			got := e.containerEnv(t, e.container())
			connURL := got[envKey]
			if connURL == "" {
				t.Fatalf("app container has no %s (link %v)", envKey, link)
			}
			image := strings.TrimSpace(mustSh(t, "docker", "inspect", "-f", "{{.Config.Image}}", e.dbContainer(t, id)))
			network := strings.TrimSpace(mustSh(t, "docker", "inspect", "-f", "{{range $k, $v := .NetworkSettings.Networks}}{{$k}} {{end}}", e.container()))
			probe := dbProbe(engine, connURL)
			if probe == nil {
				t.Skipf("no probe for engine %s", engine)
			}
			var out string
			eventually(t, engine+" accepts the injected URL from the app network", 90*time.Second, func() error {
				for _, n := range strings.Fields(network) {
					args := append([]string{"run", "--rm", "--network", n, "--entrypoint", probe[0], image}, probe[1:]...)
					o, err := sh(t, "docker", args...)
					out = o
					if err == nil && strings.Contains(o, dbProbeWant[engine]) {
						return nil
					}
				}
				return fmt.Errorf("probe output: %s", truncate(out, 500))
			})
			// The app container itself can open a TCP connection to the host in the URL.
			host := hostPort(connURL)
			if _, err := sh(t, "docker", "exec", e.container(), "sh", "-c", `nc -z -w 5 "${0%:*}" "${0##*:}"`, host); err != nil {
				t.Errorf("app container cannot reach %s: %v", host, err)
			}
			if !keep {
				// Unlink before the database goes away.
				e.c.del(t, "/api/databases/"+id+"/links/"+linkID(link)).want(t, 200)
			}
		})
	}
}

func linkID(link map[string]any) string {
	if id := str(link["id"]); id != "" {
		return id
	}
	if l, ok := link["link"].(map[string]any); ok {
		return str(l["id"])
	}
	return ""
}

// hostPort extracts host:port from a connection URL.
func hostPort(u string) string {
	rest := u[strings.Index(u, "://")+3:]
	if i := strings.LastIndex(rest, "@"); i >= 0 {
		rest = rest[i+1:]
	}
	if i := strings.IndexAny(rest, "/?"); i >= 0 {
		rest = rest[:i]
	}
	return rest
}

func (e *run) dbContainer(t *testing.T, id string) string {
	t.Helper()
	svcs := e.services(t, id)
	if len(svcs) != 1 {
		t.Fatalf("database services = %v", svcs)
	}
	return str(svcs[0]["container_name"])
}

func (e *run) storage(t *testing.T) {
	row := e.c.post(t, "/api/projects/"+e.appID+"/storage", map[string]any{"name": "e2e-data", "service_name": "app", "mount_path": "/data"}).want(t, 201).obj(t)
	e.mustDeploy(t, e.appID)
	e.refreshService(t)
	if !e.cfg.docker {
		return
	}
	marker := "persist-" + randHex(4)
	mustSh(t, "docker", "exec", e.container(), "sh", "-c", "echo "+marker+" > /data/marker")
	e.mustDeploy(t, e.appID)
	e.refreshService(t)
	out := mustSh(t, "docker", "exec", e.container(), "cat", "/data/marker")
	if strings.TrimSpace(out) != marker {
		t.Fatalf("volume content after redeploy = %q, want %q", out, marker)
	}
	vol := str(row["name"])
	if out := mustSh(t, "docker", "volume", "inspect", "-f", "{{.Name}}", vol); strings.TrimSpace(out) != vol {
		t.Fatalf("volume %s: %s", vol, out)
	}
}

func (e *run) fileEditor(t *testing.T) {
	tree := e.c.get(t, "/api/files/"+e.appID).want(t, 200)
	if strings.Contains(string(tree.Body), `".git"`) {
		t.Error("file tree lists .git")
	}
	// The editor edits existing files only (TS: 404 for a new path).
	e.c.put(t, "/api/files/"+e.appID+"/content?path="+urlQuery(e.cfg.repoDir+"/e2e-new.txt"), map[string]any{"content": "x"}).want(t, 404)
	path := "README.md"
	if r := e.c.get(t, "/api/files/"+e.appID+"/content?path="+urlQuery(path)); r.Status == 404 {
		path = e.cfg.repoDir + "/package.json"
	}
	orig := e.readFile(t, e.appID, path)
	content := orig + "\n<!-- edited by e2e ✅ -->\n"
	e.writeFile(t, e.appID, path, content)
	if got := e.readFile(t, e.appID, path); got != content {
		t.Fatalf("read back %q", got)
	}
	e.writeFile(t, e.appID, path, orig)
	for _, p := range []string{".git/config", ".git/HEAD", e.cfg.repoDir + "/../.git/config"} {
		r := e.c.get(t, "/api/files/"+e.appID+"/content?path="+urlQuery(p))
		if r.Status < 400 || r.Status >= 500 {
			t.Errorf("read %s: HTTP %d %s (want a 4xx refusal)", p, r.Status, truncate(string(r.Body), 200))
		}
		w := e.c.put(t, "/api/files/"+e.appID+"/content?path="+urlQuery(p), map[string]any{"content": "x"})
		if w.Status < 400 || w.Status >= 500 {
			t.Errorf("write %s: HTTP %d (want a 4xx refusal)", p, w.Status)
		}
	}
	r := e.c.get(t, "/api/files/"+e.appID+"/content?path="+urlQuery("../../etc/passwd"))
	if r.Status < 400 || r.Status >= 500 {
		t.Errorf("path traversal: HTTP %d", r.Status)
	}
}

func (e *run) logsStats(t *testing.T) {
	ctrs := e.c.get(t, "/api/logs/"+e.appID+"/containers").want(t, 200).arr(t)
	if len(ctrs) != 1 || ctrs[0].(map[string]any)["running"] != true {
		t.Fatalf("containers = %v", ctrs)
	}
	name := str(ctrs[0].(map[string]any)["container_name"])
	tok := str(e.c.post(t, "/api/auth/sse-token", nil).want(t, 200).obj(t)["token"])
	anon := &client{base: e.c.base, http: e.c.http}
	s := anon.open(t, "GET", "/api/logs/"+e.appID+"/stream/"+name+"?token="+urlQuery(tok)+"&tail=200", nil)
	if s.Status != 200 || !strings.HasPrefix(s.resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("log stream: HTTP %d %s", s.Status, s.resp.Header.Get("Content-Type"))
	}
	ok := s.until("Server running on port", 30*time.Second)
	s.close()
	if !strings.Contains(s.text(), `"type":"connected"`) || !ok || !strings.Contains(s.text(), "Server running on port") {
		t.Fatalf("log stream events:\n%s", truncate(s.text(), 2000))
	}
	stats := e.c.get(t, "/api/deployments/"+e.appID+"/stats").want(t, 200).obj(t)
	app, _ := stats["app"].(map[string]any)
	if app == nil || !strings.Contains(str(app["memory_usage"]), "MB") {
		t.Fatalf("stats = %v", stats)
	}
	logs := e.c.get(t, "/api/deployments/"+e.appID+"/logs").want(t, 200).obj(t)
	if l, _ := logs["logs"].(map[string]any); l == nil || !strings.Contains(str(l["app"]), "Server running") {
		t.Fatalf("container logs = %v", truncate(string(mustJSON(logs)), 500))
	}
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

func (e *run) terminal(t *testing.T) {
	tok := str(e.c.post(t, "/api/auth/terminal-token", nil).want(t, 200).obj(t)["token"])
	h := http.Header{}
	h.Set("Origin", e.cfg.baseURL)
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL(e.cfg.baseURL, "/ws/terminal", tok), h)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("dial: %v (HTTP %d)", err, status)
	}
	defer conn.Close()
	read := func() map[string]any {
		_ = conn.SetReadDeadline(time.Now().Add(20 * time.Second))
		var m map[string]any
		if err := conn.ReadJSON(&m); err != nil {
			t.Fatalf("ws read: %v", err)
		}
		return m
	}
	if m := read(); m["type"] != "auth_required" {
		t.Fatalf("first message %v", m)
	}
	_ = conn.WriteJSON(map[string]any{"type": "auth", "password": e.cfg.password, "cols": 100, "rows": 30})
	if m := read(); m["type"] != "auth_success" {
		t.Fatalf("auth: %v", m)
	}
	_ = conn.WriteJSON(map[string]any{"type": "input", "data": "echo e2e-$((40+2))-ok\n"})
	var out strings.Builder
	deadline := time.Now().Add(20 * time.Second)
	for !strings.Contains(out.String(), "e2e-42-ok") {
		if time.Now().After(deadline) {
			t.Fatalf("no command output; got %q", out.String())
		}
		m := read()
		if m["type"] == "output" {
			out.WriteString(str(m["data"]))
		}
	}
	_ = conn.WriteJSON(map[string]any{"type": "resize", "cols": 120, "rows": 40})
	_ = conn.WriteJSON(map[string]any{"type": "input", "data": "stty size\n"})
	out.Reset()
	deadline = time.Now().Add(20 * time.Second)
	for !strings.Contains(out.String(), "40 120") {
		if time.Now().After(deadline) {
			t.Fatalf("resize not applied; got %q", out.String())
		}
		m := read()
		if m["type"] == "output" {
			out.WriteString(str(m["data"]))
		}
	}
	_ = conn.WriteJSON(map[string]any{"type": "input", "data": "exit\n"})
	deadline = time.Now().Add(20 * time.Second)
	for {
		_ = conn.SetReadDeadline(deadline)
		var m map[string]any
		if err := conn.ReadJSON(&m); err != nil {
			break // closed by the server after the exit message
		}
		if m["type"] == "exit" {
			continue
		}
	}
}

func (e *run) system(t *testing.T) {
	st := e.c.get(t, "/api/system/stats").want(t, 200).obj(t)
	for _, k := range []string{"cpu", "memory", "disk"} {
		if _, ok := st[k]; !ok {
			t.Errorf("stats has no %q: %v", k, truncate(string(mustJSON(st)), 400))
		}
	}
	e.c.get(t, "/api/system/quick").want(t, 200).obj(t)
	v := e.c.get(t, "/api/system/version").want(t, 200).obj(t)
	if str(v["current"]) == "" && str(v["version"]) == "" {
		t.Errorf("version = %v", v)
	}
}

var backupFileRe = "  File: "

func (e *run) backupRestore(t *testing.T) {
	if !e.cfg.restore {
		t.Skip("DOCKLIFT_E2E_RESTORE=0")
	}
	r := e.c.post(t, "/api/backup/create", map[string]any{"name": "e2e"}).want(t, 200)
	log := string(r.Body)
	i := strings.Index(log, backupFileRe)
	if i < 0 || !strings.Contains(log, "BACKUP COMPLETE") {
		t.Fatalf("backup log:\n%s", log)
	}
	name := strings.TrimSpace(strings.SplitN(log[i+len(backupFileRe):], "\n", 2)[0])
	dl := e.c.get(t, "/api/backup/download/"+name).want(t, 200)
	if len(dl.Body) < 100 || string(dl.Body[:2]) != "PK" {
		t.Fatalf("download is not a zip (%d bytes)", len(dl.Body))
	}
	s := e.c.open(t, "POST", "/api/backup/restore/"+name, map[string]any{"password": e.cfg.password})
	if s.Status != 200 {
		t.Fatalf("restore: HTTP %d %s", s.Status, s.all(10*time.Second))
	}
	rlog := s.all(20 * time.Minute)
	if strings.Contains(rlog, "[ERROR]") || strings.Contains(rlog, "[CRITICAL]") || !strings.Contains(rlog, "RESTORE COMPLETE") {
		t.Fatalf("restore log:\n%s", tail(rlog, 4000))
	}
	t.Logf("restore log tail:\n%s", tail(rlog, 1200))
	eventually(t, "healthy after restore", 3*time.Minute, func() error {
		code, body, err := httpGet(e.cfg.baseURL+"/api/health", "")
		if err != nil || code != 200 {
			return fmt.Errorf("HTTP %d %v %s", code, err, body)
		}
		return nil
	})
	// Same admin, same projects, apps still running and answering.
	eventually(t, "login after restore", 2*time.Minute, func() error {
		r := e.c.do(t, "POST", "/api/auth/login", map[string]any{"email": e.cfg.email, "password": e.cfg.password}, nil)
		if r.Status != 200 {
			return fmt.Errorf("HTTP %d %s", r.Status, r.Body)
		}
		e.c.token = str(r.obj(t)["token"])
		return nil
	})
	for _, id := range append([]string{e.appID, e.zipID}, e.dbIDs...) {
		eventually(t, "project "+id+" running after restore", 5*time.Minute, func() error {
			if st := e.projectStatus(t, id); st != "running" {
				return fmt.Errorf("status %q", st)
			}
			return nil
		})
	}
	if e.cfg.docker {
		eventually(t, "app answers after restore", 2*time.Minute, func() error {
			code, body, err := httpGet(e.cfg.proxyURL+"/", e.domain)
			if err != nil || code != 200 || !strings.Contains(body, "Hello from Docklift") {
				return fmt.Errorf("HTTP %d %v: %s", code, err, truncate(body, 300))
			}
			return nil
		})
		mustSh(t, "docker", "exec", "docklift-nginx-proxy", "nginx", "-t")
	}
	e.c.del(t, "/api/backup/"+name).want(t, 200)
}

func (e *run) appImages(t *testing.T, projectID string) []string {
	t.Helper()
	out := mustSh(t, "docker", "images", "--format", "{{.Repository}}:{{.Tag}}")
	var imgs []string
	for _, l := range strings.Fields(out) {
		if strings.HasPrefix(l, "docklift-"+projectID[:8]+"-") {
			imgs = append(imgs, l)
		}
	}
	return imgs
}

func (e *run) purge(t *testing.T) {
	e.needDocker(t)
	inUse := e.runningImage(t)
	// Deploys already keep two tags; a stale third tag (as left by an older
	// release or an interrupted cleanup) is what purge must remove.
	stale := "docklift-" + e.appID[:8] + "-app:e2estale" + randHex(2)
	// (A tag sharing the running image ID counts as in use, as in TS, so the
	// stale tag points at another image.)
	mustSh(t, "docker", "pull", "-q", "busybox:1.37")
	mustSh(t, "docker", "tag", "busybox:1.37", stale)
	before := e.appImages(t, e.appID)
	r := e.c.post(t, "/api/system/purge", map[string]any{"password": e.cfg.password}).want(t, 200).obj(t)
	after := e.appImages(t, e.appID)
	if slices.Contains(after, stale) {
		t.Errorf("purge kept the stale tag %s", stale)
	}
	t.Logf("app images before purge %v, after %v; purge: %v", before, after, truncate(string(mustJSON(r)), 600))
	if !slices.Contains(after, inUse) {
		t.Fatalf("purge removed the in-use image %s (left %v)", inUse, after)
	}
	if len(before) >= 2 && len(after) < 2 {
		t.Fatalf("purge removed the rollback image (before %v, after %v)", before, after)
	}
	if len(after) > 2 {
		t.Errorf("more than two images kept after purge: %v", after)
	}
	if !e.containerRunning(t, e.container()) {
		t.Fatal("purge stopped the app")
	}
}

func (e *run) deleteProjects(t *testing.T) {
	ids := append([]string{e.appID, e.zipID}, e.dbIDs...)
	for _, id := range ids {
		if id == "" {
			continue
		}
		var svcIDs []string
		for _, s := range e.services(t, id) {
			svcIDs = append(svcIDs, str(s["id"]))
		}
		e.c.del(t, "/api/projects/"+id).want(t, 200)
		e.c.get(t, "/api/projects/"+id).want(t, 404)
		if !e.cfg.docker {
			continue
		}
		label := "label=com.docklift.project=" + id
		if out := mustSh(t, "docker", "ps", "-a", "-q", "--filter", label); strings.TrimSpace(out) != "" {
			t.Errorf("containers left for %s: %s", id, out)
		}
		if out := mustSh(t, "docker", "network", "ls", "-q", "--filter", label); strings.TrimSpace(out) != "" {
			t.Errorf("networks left for %s: %s", id, out)
		}
		if imgs := e.appImages(t, id); len(imgs) > 0 {
			t.Errorf("images left for %s: %v", id, imgs)
		}
		vols := mustSh(t, "docker", "volume", "ls", "-q", "--filter", "name=dl-"+id[:8]+"-")
		if strings.TrimSpace(vols) != "" {
			t.Errorf("volumes left for %s: %s", id, vols)
		}
		for _, sid := range svcIDs {
			if _, err := os.Stat(filepath.Join(e.cfg.installDir, "nginx-proxy", "conf.d", "service-"+sid+".conf")); err == nil {
				t.Errorf("nginx conf left for service %s", sid)
			}
		}
		if _, err := os.Stat(filepath.Join(e.cfg.installDir, "deployments", id)); err == nil {
			t.Errorf("project files left for %s", id)
		}
	}
	if e.cfg.docker {
		mustSh(t, "docker", "exec", "docklift-nginx-proxy", "nginx", "-t")
	}
}
