package system

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/go-chi/chi/v5"
	"github.com/moby/moby/client"

	"docklift/internal/app"
	"docklift/internal/app/apptest"
	"docklift/internal/auth"
	"docklift/internal/httpx"
	"docklift/internal/store"
	"docklift/internal/sysinfo"
	"docklift/internal/sysinfo/sysinfotest"
	"docklift/internal/version"
)

const password = "correct-horse-battery"

type fakeHost struct {
	mu    sync.Mutex
	calls []string
	err   error
}

func (f *fakeHost) record(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, name)
	return f.err
}

func (f *fakeHost) Reboot() error       { return f.record("reboot") }
func (f *fakeHost) UpdateSystem() error { return f.record("update-system") }
func (f *fakeHost) Upgrade() error      { return f.record("upgrade") }

type fakeDocker struct {
	mu       sync.Mutex
	pingErr  error
	restarts []string
}

func (f *fakeDocker) ContainerInspect(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	return client.ContainerInspectResult{}, cerrdefs.ErrNotFound.WithMessage("No such container")
}
func (f *fakeDocker) ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error) {
	return client.ContainerListResult{}, nil
}
func (f *fakeDocker) ContainerLogs(context.Context, string, client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
	return nil, errors.New("unused")
}
func (f *fakeDocker) Ping(context.Context, client.PingOptions) (client.PingResult, error) {
	return client.PingResult{}, f.pingErr
}
func (f *fakeDocker) ContainerRestart(_ context.Context, name string, _ client.ContainerRestartOptions) (client.ContainerRestartResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.restarts = append(f.restarts, name)
	if name == "docklift-frontend" {
		return client.ContainerRestartResult{}, cerrdefs.ErrNotFound
	}
	return client.ContainerRestartResult{}, nil
}

func (f *fakeDocker) restarted() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.restarts...)
}

type fakeImages struct {
	keep      map[string][]string
	refs      map[string]string
	gotKeep   map[string]bool
	gotInUse  map[string]bool
	result    CleanupResult
	removeErr error
	onRemove  func()
	pruneOK   bool
	pruneOut  string
	pruned    bool
}

func (f *fakeImages) KeepTags(_ context.Context, projectID string) ([]string, error) {
	return f.keep[projectID], nil
}
func (f *fakeImages) ContainerImageRef(_ context.Context, name string) string { return f.refs[name] }
func (f *fakeImages) RemoveUnusedImages(_ context.Context, keep, inUse map[string]bool) (CleanupResult, error) {
	f.gotKeep, f.gotInUse = keep, inUse
	if f.onRemove != nil {
		f.onRemove()
	}
	return f.result, f.removeErr
}
func (f *fakeImages) PruneBuildKitAll(context.Context) (bool, string) {
	f.pruned = true
	return f.pruneOK, f.pruneOut
}

// connectionRefused is the error of the real client for a dead daemon.
var connectionRefused = sync.OnceValue(func() error {
	c, err := client.New(client.WithHost("tcp://127.0.0.1:1"))
	if err != nil {
		panic(err)
	}
	defer c.Close()
	_, err = c.Ping(context.Background(), client.PingOptions{})
	return err
})

type env struct {
	d       *app.Deps
	h       http.Handler
	token   string
	src     *sysinfotest.Source
	host    *fakeHost
	docker  *fakeDocker
	images  *fakeImages
	options Options
}

func newEnv(t *testing.T, mutate func(*Options)) *env {
	t.Helper()
	d := apptest.NewDeps(t)
	_, token := apptest.CreateAdmin(t, d, "admin@example.com", password)
	e := &env{d: d, token: token, host: &fakeHost{}, docker: &fakeDocker{}, images: &fakeImages{pruneOK: true},
		src: &sysinfotest.Source{CPUInfo: sysinfo.CPUInfo{Cores: 2, Speed: 3}, Load: 5, MemInfo: sysinfo.MemInfo{Total: 100, Used: 40, Free: 60}}}
	zero := time.Duration(0)
	e.options = Options{Images: e.images, Docker: e.docker, Host: e.host, Source: e.src, GOOS: "linux", SimulatedDelay: &zero,
		PublicIPURL: "http://127.0.0.1:1/", GitHubURL: "http://127.0.0.1:1/"}
	if mutate != nil {
		mutate(&e.options)
	}
	r := chi.NewRouter()
	r.Use(httpx.ParseBody, d.Auth.RequireSession)
	MountWith(r, d, e.options)
	e.h = r
	return e
}

type resp struct {
	code int
	body map[string]any
	raw  string
}

func (e *env) do(t *testing.T, method, path string, body any, headers ...string) resp {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+e.token)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
		if headers[i] == "Authorization" && headers[i+1] == "" {
			req.Header.Del("Authorization")
		}
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	out := resp{code: rec.Code, raw: rec.Body.String()}
	_ = json.Unmarshal(rec.Body.Bytes(), &out.body)
	return out
}

func expect(t *testing.T, r resp, code int, want string) {
	t.Helper()
	if r.code != code || (want != "" && r.raw != want) {
		t.Fatalf("got %d %s\nwant %d %s", r.code, r.raw, code, want)
	}
}

func TestStepUpGuards(t *testing.T) {
	e := newEnv(t, nil)
	for _, path := range []string{"/purge", "/reboot", "/reset", "/update-system", "/upgrade"} {
		expect(t, e.do(t, "POST", path, nil), 403, `{"error":"Password confirmation required for this operation","requirePassword":true}`)
		expect(t, e.do(t, "POST", path, map[string]any{"password": 5}), 403, `{"error":"Password confirmation required for this operation","requirePassword":true}`)
		expect(t, e.do(t, "POST", path, map[string]any{"password": "nope"}), 403, `{"error":"Invalid password","requirePassword":true}`)
		expect(t, e.do(t, "POST", path, map[string]any{"password": password}, "Authorization", "", auth.InternalSecretHeader, e.d.Auth.Secrets().InternalAPISecret),
			401, `{"error":"Unauthorized"}`)
	}
	if len(e.host.calls) != 0 || len(e.docker.restarted()) != 0 || e.images.pruned {
		t.Fatal("no action may run without step-up")
	}
}

func TestHostActionsOnLinux(t *testing.T) {
	e := newEnv(t, nil)
	ok := map[string]string{
		"/reboot":        `{"message":"System is rebooting now..."}`,
		"/update-system": `{"message":"System update started on host. This may take several minutes."}`,
		"/upgrade":       `{"message":"Docklift upgrade started. The application will restart shortly."}`,
	}
	for path, want := range ok {
		expect(t, e.do(t, "POST", path, map[string]any{"password": password}), 200, want)
	}
	if len(e.host.calls) != 3 {
		t.Fatalf("calls %v", e.host.calls)
	}

	e.host.err = ErrHostActionsUnsupported
	fail := map[string]string{"/reboot": "Failed to initiate reboot", "/update-system": "Failed to start system update", "/upgrade": "Failed to start upgrade"}
	for path, msg := range fail {
		expect(t, e.do(t, "POST", path, map[string]any{"password": password}), 500,
			`{"error":"`+msg+`","details":"host actions (nsenter, reboot) are only supported on Linux"}`)
	}
}

func TestDevMachineSimulations(t *testing.T) {
	for _, goos := range []string{"windows", "darwin"} {
		e := newEnv(t, func(o *Options) { o.GOOS = goos })
		expect(t, e.do(t, "POST", "/reboot", map[string]any{"password": password}), 200, `{"message":"Dev Mode: Simulated Server Reboot successful."}`)
		expect(t, e.do(t, "POST", "/update-system", map[string]any{"password": password}), 200, `{"message":"Dev Mode: Simulated system update complete."}`)
		expect(t, e.do(t, "POST", "/upgrade", map[string]any{"password": password}), 200, `{"message":"Dev Mode: Simulated upgrade complete. No changes made to system."}`)
		if len(e.host.calls) != 0 {
			t.Fatalf("%s: simulations must not touch the host: %v", goos, e.host.calls)
		}
	}
	e := newEnv(t, func(o *Options) { o.GOOS = "windows" })
	expect(t, e.do(t, "POST", "/reset", map[string]any{"password": password}), 200, `{"message":"Dev Mode: Simulated Service Reset complete."}`)
}

func TestReset(t *testing.T) {
	e := newEnv(t, nil)
	expect(t, e.do(t, "POST", "/reset", map[string]any{"password": password}), 200, `{"message":"Services reset command sent successfully (Restarting in 1s)"}`)
	deadline := time.Now().Add(10 * time.Second)
	for len(e.docker.restarted()) < len(coreContainers) {
		if time.Now().After(deadline) {
			t.Fatalf("restarts %v", e.docker.restarted())
		}
		time.Sleep(20 * time.Millisecond)
	}
	got := strings.Join(e.docker.restarted(), " ")
	if got != "docklift-frontend docklift-nginx docklift-nginx-proxy docklift-certbot docklift-backend" {
		t.Fatalf("restart order %q: the backend's own container must go last", got)
	}

	e = newEnv(t, nil)
	e.docker.pingErr = connectionRefused()
	expect(t, e.do(t, "POST", "/reset", map[string]any{"password": password}), 503, `{"error":"Docker is not available"}`)
	e.docker.pingErr = errors.New("daemon error")
	expect(t, e.do(t, "POST", "/reset", map[string]any{"password": password}), 500, `{"error":"Failed to reset services","details":"daemon error"}`)
}

func TestResetWithoutDockerClient(t *testing.T) {
	d := apptest.NewDeps(t)
	_, token := apptest.CreateAdmin(t, d, "a@example.com", password)
	r := chi.NewRouter()
	r.Use(httpx.ParseBody, d.Auth.RequireSession)
	MountWith(r, d, Options{GOOS: "linux", Host: &fakeHost{}, Source: &sysinfotest.Source{}})
	req := httptest.NewRequest("POST", "/reset", strings.NewReader(`{"password":"`+password+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 503 {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
}

func seedProject(t *testing.T, d *app.Deps, container string) *store.Project {
	t.Helper()
	p, err := d.Store.CreateProject(t.Context(), store.ProjectCreate{Name: "p-" + container})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Store.CreateService(t.Context(), store.ServiceCreate{ProjectID: p.ID, Name: "app", DockerfilePath: "Dockerfile", ContainerName: store.Ptr(container)}); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPurge(t *testing.T) {
	e := newEnv(t, nil)
	p := seedProject(t, e.d, "c-1")
	e.images.keep = map[string][]string{p.ID: {"docklift-aaaaaaaa-app:11111111"}}
	e.images.refs = map[string]string{"c-1": "sha256:abc"}
	e.images.result = CleanupResult{Kept: []string{"docklift-aaaaaaaa-app:11111111"}, Removed: []string{"a", "b"}, SkippedInUse: []string{"c"},
		Errors: []string{"e1", "e2", "e3", "e4", "e5", "e6"}}
	r := e.do(t, "POST", "/purge", map[string]any{"password": password})
	expect(t, r, 200, "")
	want := `{"message":"DockLift-scoped cleanup completed","details":["✓ Removed 2 unused Docklift image tag(s); kept 1","○ Skipped 1 in-use tag(s)",` +
		`"! e1","! e2","! e3","! e4","! e5","✓ BuildKit cache cleared (builder prune -af)","○ Foreign images/containers/volumes were not touched",` +
		`"○ Host OS maintenance (swap, journal, apt, /tmp) is not available via the panel"],"removedImages":2,"keptImages":1,"memoryBefore":40,"memoryAfter":40,"memorySaved":"0%"}`
	if r.raw != want {
		t.Fatalf("body %s\nwant %s", r.raw, want)
	}
	if !e.images.gotKeep["docklift-aaaaaaaa-app:11111111"] || len(e.images.gotKeep) != 1 {
		t.Fatalf("keep set %v", e.images.gotKeep)
	}
	if !e.images.gotInUse["sha256:abc"] || !e.images.gotInUse["docklift-aaaaaaaa-app:11111111"] {
		t.Fatalf("in-use refs %v: keep set plus container images", e.images.gotInUse)
	}

	e.images.pruneOK, e.images.pruneOut = false, ""
	e.images.result = CleanupResult{}
	r = e.do(t, "POST", "/purge", map[string]any{"password": password})
	if !strings.Contains(r.raw, `"! BuildKit prune warning: failed"`) {
		t.Fatalf("prune failure: %s", r.raw)
	}

	// A deployment that starts during image cleanup keeps BuildKit intact.
	status := "in_progress"
	e.images.pruned = false
	e.images.onRemove = func() {
		if _, err := e.d.Store.CreateDeployment(t.Context(), store.DeploymentCreate{ProjectID: p.ID, Status: &status}); err != nil {
			t.Error(err)
		}
	}
	r = e.do(t, "POST", "/purge", map[string]any{"password": password})
	if !strings.Contains(r.raw, "○ Skipped BuildKit wipe — a deployment is in progress (images already cleaned)") || e.images.pruned {
		t.Fatalf("busy BuildKit: %s", r.raw)
	}
	e.images.onRemove = nil

	// Now the deployment is in progress from the start.
	dep, _ := e.d.Store.FindDeployment(t.Context(), store.DeploymentFilter{Status: &status}, store.OrderNone)
	expect(t, e.do(t, "POST", "/purge", map[string]any{"password": password}), 409,
		`{"error":"A deployment is in progress. Wait for it to finish before purging (avoids deleting in-flight images / BuildKit layers).","deploymentId":"`+dep.ID+`","projectId":"`+p.ID+`"}`)
}

func TestPurgeFailures(t *testing.T) {
	e := newEnv(t, nil)
	seedProject(t, e.d, "c-1")
	e.images.removeErr = connectionRefused()
	expect(t, e.do(t, "POST", "/purge", map[string]any{"password": password}), 503, `{"error":"Docker is not available"}`)
	e.images.removeErr = errors.New("weird")
	expect(t, e.do(t, "POST", "/purge", map[string]any{"password": password}), 500, `{"error":"Failed to complete purge operation"}`)

	unwired := newEnv(t, func(o *Options) { o.Images = nil })
	expect(t, unwired.do(t, "POST", "/purge", map[string]any{"password": password}), 500, `{"error":"Failed to complete purge operation"}`)
}

func TestExecuteValidation(t *testing.T) {
	e := newEnv(t, nil)
	expect(t, e.do(t, "POST", "/execute", map[string]any{}), 400, `{"error":"Command is required"}`)
	expect(t, e.do(t, "POST", "/execute", map[string]any{"command": 7, "password": password}), 400, `{"error":"Command is required"}`)
	expect(t, e.do(t, "POST", "/execute", map[string]any{"command": "id"}), 401, `{"error":"Password required for terminal access","requirePassword":true}`)
	expect(t, e.do(t, "POST", "/execute", map[string]any{"command": "id", "password": 1}), 401, `{"error":"Password required for terminal access","requirePassword":true}`)
	expect(t, e.do(t, "POST", "/execute", map[string]any{"command": "id", "password": "wrong"}), 403, `{"error":"Invalid password","requirePassword":true}`)
	expect(t, e.do(t, "POST", "/execute", map[string]any{"command": "id", "password": password}, "Authorization", "", auth.InternalSecretHeader, e.d.Auth.Secrets().InternalAPISecret),
		401, `{"error":"Authentication required"}`)

	missing := newEnv(t, func(o *Options) { o.Shell = "/definitely/not/a/shell" })
	r := missing.do(t, "POST", "/execute", map[string]any{"command": "echo hi", "password": password})
	if r.code != 200 || r.body["output"] != "" || r.body["error"] == "" || r.body["error"] == nil {
		t.Fatalf("missing shell: %d %s", r.code, r.raw)
	}
}

func TestExecuteRunsShell(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not installed")
	}
	e := newEnv(t, func(o *Options) { o.Shell = bash })
	run := func(cmd string) resp {
		return e.do(t, "POST", "/execute", map[string]any{"command": cmd, "password": password})
	}
	expect(t, run("echo contract-ok"), 200, `{"output":"contract-ok\n"}`)
	expect(t, run("echo oops >&2"), 200, `{"output":"oops\n","error":"oops\n"}`)
	expect(t, run("true"), 200, `{"output":"(no output)"}`)
	expect(t, run("echo out; echo err >&2"), 200, `{"output":"out\n"}`)
	expect(t, run("exit 3"), 200, `{"output":"","error":"Command failed: exit 3\n"}`)
	expect(t, run("echo partial; echo bad >&2; exit 1"), 200, `{"output":"partial\n","error":"bad\n"}`)
	r := run("head -c 2000000 /dev/zero | tr '\\0' a")
	if r.code != 200 || r.body["error"] != "stdout maxBuffer length exceeded" || len(r.body["output"].(string)) != executeMaxBuffer {
		t.Fatalf("max buffer: %d error=%v len=%d", r.code, r.body["error"], len(r.body["output"].(string)))
	}
}

func TestVersion(t *testing.T) {
	var mu sync.Mutex
	hits, status, tag := 0, 200, `"v99.1.0"`
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		hits++
		if r.Header.Get("User-Agent") != "Docklift" || r.Header.Get("Accept") != "application/vnd.github+json" || r.Header.Get("X-GitHub-Api-Version") != "2022-11-28" {
			t.Errorf("headers %v", r.Header)
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"tag_name":`+tag+`}`)
	}))
	defer gh.Close()
	e := newEnv(t, func(o *Options) { o.GitHubURL = gh.URL })
	r := e.do(t, "GET", "/version", nil)
	if r.code != 200 || r.body["current"] != version.Version || r.body["latest"] != "99.1.0" || r.body["updateAvailable"] != true || r.body["githubOk"] != true {
		t.Fatalf("version: %s", r.raw)
	}
	e.do(t, "GET", "/version?refresh=1", nil) // within the 30 s refresh cooldown
	mu.Lock()
	if hits != 1 {
		t.Fatalf("hits %d: the cache must hold", hits)
	}
	mu.Unlock()

	mu.Lock()
	status = 500
	mu.Unlock()
	failing := newEnv(t, func(o *Options) { o.GitHubURL = gh.URL })
	r = failing.do(t, "GET", "/version", nil)
	if r.body["githubOk"] != false || r.body["updateAvailable"] != false || r.body["latest"] != version.Version {
		t.Fatalf("github failure: %s", r.raw)
	}
	mu.Lock()
	status, tag = 200, `5`
	mu.Unlock()
	odd := newEnv(t, func(o *Options) { o.GitHubURL = gh.URL })
	if r = odd.do(t, "GET", "/version", nil); r.body["githubOk"] != false {
		t.Fatalf("non-string tag: %s", r.raw)
	}
}

func TestIsNewerVersion(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"2.1.7", "2.1.6", true}, {"2.1.6", "2.1.6", false}, {"2.2", "2.1.9", true}, {"2.1.5", "2.1.6", false},
		{"3.0.0-beta", "2.9.9", true}, {"x.y.z", "0.0.0", false}, {"2.1.6.9", "2.1.6", false}, {"10.0.0", "9.9.9", true},
	}
	for _, c := range cases {
		if got := IsNewerVersion(c.latest, c.current); got != c.want {
			t.Errorf("IsNewerVersion(%q, %q) = %v", c.latest, c.current, got)
		}
	}
}

func TestStatsRoutes(t *testing.T) {
	e := newEnv(t, nil)
	r := e.do(t, "GET", "/stats", nil)
	if r.code != 200 || r.body["cpu"].(map[string]any)["cores"] != 2.0 || r.body["server"].(map[string]any)["cpuCores"] != "2 Cores @ 3 GHz" {
		t.Fatalf("stats: %s", r.raw)
	}
	r = e.do(t, "GET", "/quick", nil)
	if r.code != 200 || r.body["cpu"] != 5.0 || r.body["memory"] != 40.0 {
		t.Fatalf("quick: %s", r.raw)
	}
	expect(t, e.do(t, "GET", "/ip", nil), 200, `{"ip":"N/A"}`)

	broken := newEnv(t, func(o *Options) { o.Source = &sysinfotest.Source{MemErr: errors.New("x")} })
	expect(t, broken.do(t, "GET", "/stats", nil), 500, `{"error":"Failed to fetch system statistics"}`)
	expect(t, broken.do(t, "GET", "/quick", nil), 500, `{"error":"Failed to fetch quick stats"}`)
}

func TestSystemLogStream(t *testing.T) {
	e := newEnv(t, nil)
	expect(t, e.do(t, "GET", "/logs/database", nil), 400, `{"error":"Invalid service name. Available: backend, frontend, proxy, nginx, certbot"}`)
	r := e.do(t, "GET", "/logs/proxy?tail=10", nil)
	if r.code != 200 || r.raw != `data: {"type":"error","message":"Container not found: No such container"}`+"\n\n" {
		t.Fatalf("stream: %d %q", r.code, r.raw)
	}
}
