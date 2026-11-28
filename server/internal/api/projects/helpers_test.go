package projects

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/go-chi/chi/v5"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"

	"docklift/internal/app"
	"docklift/internal/app/apptest"
	"docklift/internal/certs"
	"docklift/internal/dockerx"
	"docklift/internal/execx"
	"docklift/internal/httpx"
	"docklift/internal/nginx"
	"docklift/internal/store"
)

// fakeEngine scripts the parts of the Engine API the routes reach; any
// other method panics through the nil embedded interface.
type fakeEngine struct {
	dockerx.Engine
	mu      sync.Mutex
	calls   []string
	pingErr error
	running map[string]bool
	// images are the local images ImageList returns.
	images []image.Summary
}

func (f *fakeEngine) record(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, s)
}

func (f *fakeEngine) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeEngine) Ping(context.Context, client.PingOptions) (client.PingResult, error) {
	return client.PingResult{}, f.pingErr
}

func (f *fakeEngine) ContainerInspect(_ context.Context, name string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	f.record("inspect " + name)
	running, ok := f.running[name]
	if !ok {
		return client.ContainerInspectResult{}, cerrdefs.ErrNotFound.WithMessage("No such container: " + name)
	}
	status := container.ContainerState("exited")
	if running {
		status = "running"
	}
	return client.ContainerInspectResult{Container: container.InspectResponse{State: &container.State{Status: status, Running: running}}}, nil
}

func (f *fakeEngine) ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error) {
	return client.ContainerListResult{}, nil
}

func (f *fakeEngine) NetworkInspect(_ context.Context, name string, _ client.NetworkInspectOptions) (client.NetworkInspectResult, error) {
	f.record("network inspect " + name)
	return client.NetworkInspectResult{}, nil
}

func (f *fakeEngine) NetworkConnect(_ context.Context, network string, o client.NetworkConnectOptions) (client.NetworkConnectResult, error) {
	f.record("network connect " + network + " " + o.Container)
	return client.NetworkConnectResult{}, nil
}

func (f *fakeEngine) NetworkDisconnect(_ context.Context, network string, o client.NetworkDisconnectOptions) (client.NetworkDisconnectResult, error) {
	f.record("network disconnect " + network + " " + o.Container)
	return client.NetworkDisconnectResult{}, nil
}

func (f *fakeEngine) ImageList(context.Context, client.ImageListOptions) (client.ImageListResult, error) {
	return client.ImageListResult{Items: f.images}, nil
}

func (f *fakeEngine) ImageRemove(_ context.Context, ref string, _ client.ImageRemoveOptions) (client.ImageRemoveResult, error) {
	f.record("image rm " + ref)
	return client.ImageRemoveResult{}, nil
}

func (f *fakeEngine) NetworkRemove(_ context.Context, name string, _ client.NetworkRemoveOptions) (client.NetworkRemoveResult, error) {
	f.record("network rm " + name)
	return client.NetworkRemoveResult{}, nil
}

// fakeRun is a scripted docker CLI; answer sees the joined arguments.
type fakeRun struct {
	mu     sync.Mutex
	calls  []execx.Cmd
	answer func(args string) (execx.Result, error)
}

func (f *fakeRun) Run(_ context.Context, c execx.Cmd) (execx.Result, error) {
	f.mu.Lock()
	f.calls = append(f.calls, c)
	f.mu.Unlock()
	if f.answer == nil {
		return execx.Result{}, nil
	}
	return f.answer(strings.Join(c.Args, " "))
}

func (f *fakeRun) Args() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	for i, c := range f.calls {
		out[i] = strings.Join(c.Args, " ")
	}
	return out
}

func (f *fakeRun) Cmds() []execx.Cmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]execx.Cmd(nil), f.calls...)
}

func exitErr(code int) error { return &execx.ExitError{Name: "docker", Code: code} }

// nginxRunner answers every `docker exec nginx ...` with success.
type nginxRunner struct{}

func (nginxRunner) Run(context.Context, string, ...string) (certs.ExecResult, error) {
	return certs.ExecResult{}, nil
}

type fixture struct {
	t      *testing.T
	d      *app.Deps
	h      http.Handler
	engine *fakeEngine
	run    *fakeRun
}

func newFixture(t *testing.T, o Options) *fixture {
	t.Helper()
	d := apptest.NewDeps(t)
	engine := &fakeEngine{running: map[string]bool{}}
	run := &fakeRun{}
	d.DockerX.Engine = engine
	d.DockerX.Run = run.Run
	d.Edge.Nginx = nginx.New(nginx.Options{ConfDir: d.Config.NginxConfPath, ProxyContainer: d.Config.NginxProxyContainer,
		BackendPort: d.Config.Port, Runner: nginxRunner{}, Log: d.Log}, d.Edge.Certs, d.Store)
	r := chi.NewRouter()
	MountWith(r, d, o)
	return &fixture{t: t, d: d, h: httpx.ParseBody(r), engine: engine, run: run}
}

type resp struct {
	code int
	raw  []byte
}

func (r resp) obj(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(r.raw, &m); err != nil {
		t.Fatalf("%d %s: %v", r.code, r.raw, err)
	}
	return m
}

func (r resp) arr(t *testing.T) []any {
	t.Helper()
	var a []any
	if err := json.Unmarshal(r.raw, &a); err != nil {
		t.Fatalf("%d %s: %v", r.code, r.raw, err)
	}
	return a
}

func (r resp) expectError(t *testing.T, code int, msg string) {
	t.Helper()
	if r.code != code {
		t.Fatalf("status %d, want %d: %s", r.code, code, r.raw)
	}
	if got := r.obj(t)["error"]; got != msg {
		t.Fatalf("error %q, want %q", got, msg)
	}
}

func (r resp) expect(t *testing.T, code int) map[string]any {
	t.Helper()
	if r.code != code {
		t.Fatalf("status %d, want %d: %s", r.code, code, r.raw)
	}
	return r.obj(t)
}

func (f *fixture) do(method, path string, body any) resp {
	f.t.Helper()
	var rd io.Reader
	ct := ""
	switch b := body.(type) {
	case nil:
	case *form:
		var buf bytes.Buffer
		ct = b.encode(f.t, &buf)
		rd = &buf
	case string:
		rd, ct = strings.NewReader(b), "application/json"
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			f.t.Fatal(err)
		}
		rd, ct = bytes.NewReader(raw), "application/json"
	}
	req := httptest.NewRequest(method, path, rd)
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	return resp{code: rec.Code, raw: rec.Body.Bytes()}
}

type filePart struct {
	field, name string
	data        []byte
}

type form struct {
	fields [][2]string
	files  []filePart
}

func (fm *form) encode(t *testing.T, buf *bytes.Buffer) string {
	t.Helper()
	mw := multipart.NewWriter(buf)
	for _, kv := range fm.fields {
		if err := mw.WriteField(kv[0], kv[1]); err != nil {
			t.Fatal(err)
		}
	}
	for _, fp := range fm.files {
		w, err := mw.CreateFormFile(fp.field, fp.name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(fp.data)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return mw.FormDataContentType()
}

func zipOf(t *testing.T, kv ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for i := 0; i+1 < len(kv); i += 2 {
		w, err := zw.Create(kv[i])
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(kv[i+1]))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func appZip(t *testing.T) []byte {
	return zipOf(t, "Dockerfile", "FROM alpine\n", "package.json", "{}", "src/app.js", "x")
}

func upload(name string, data []byte, kv ...string) *form {
	fm := &form{fields: [][2]string{{"name", name}}, files: []filePart{{"files", "app.zip", data}}}
	for i := 0; i+1 < len(kv); i += 2 {
		fm.fields = append(fm.fields, [2]string{kv[i], kv[i+1]})
	}
	return fm
}

// createUpload creates an upload project through the route.
func (f *fixture) createUpload(name string, kv ...string) map[string]any {
	f.t.Helper()
	return f.do("POST", "/", upload(name, appZip(f.t), kv...)).expect(f.t, 201)
}

func (f *fixture) project(over store.ProjectCreate) *store.Project {
	f.t.Helper()
	if over.Name == "" {
		over.Name = "p"
	}
	p, err := f.d.Store.CreateProject(context.Background(), over)
	if err != nil {
		f.t.Fatal(err)
	}
	return p
}

func (f *fixture) service(projectID, name string, container *string) *store.Service {
	f.t.Helper()
	s, err := f.d.Store.CreateService(context.Background(), store.ServiceCreate{ProjectID: projectID, Name: name, DockerfilePath: "Dockerfile", ContainerName: container})
	if err != nil {
		f.t.Fatal(err)
	}
	return s
}

// uploadsLeft lists files left in DATA_PATH/uploads.
func (f *fixture) uploadsLeft() []string {
	f.t.Helper()
	entries, err := os.ReadDir(filepath.Join(f.d.Config.DataPath, "uploads"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		f.t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func unavailable() error { return fmt.Errorf("dial: %w", dockerx.ErrUnavailable) }
