package deployments

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"docklift/internal/app"
	"docklift/internal/app/apptest"
	"docklift/internal/dnscheck"
	"docklift/internal/dockerx"
	"docklift/internal/execx"
	"docklift/internal/gitops"
	"docklift/internal/httpx"
	"docklift/internal/nginx"
	"docklift/internal/store"
)

// gate lets a test hold a fake command until it releases it (or the
// command's context is cancelled).
type gate struct {
	entered chan struct{}
	release chan struct{}
}

func newGate() *gate { return &gate{entered: make(chan struct{}, 8), release: make(chan struct{})} }

func (g *gate) hold(ctx context.Context) error {
	g.entered <- struct{}{}
	select {
	case <-g.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// fakeDocker implements Docker and deploy.DockerOps in memory.
type fakeDocker struct {
	mu        sync.Mutex
	calls     []string
	images    map[string]bool
	upCode    int
	upErr     error
	upGate    *gate
	buildGate *gate
	pullGate  *gate
	buildFail bool
	buildOut  string
	downOK    bool
	connErr   error
	pingErr   error
	built     []string
	panicOnUp bool
}

func newFakeDocker() *fakeDocker {
	return &fakeDocker{images: map[string]bool{}, downOK: true}
}

func (f *fakeDocker) record(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, s)
}

func (f *fakeDocker) called(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func (f *fakeDocker) Ping(context.Context) error { return f.pingErr }

func (f *fakeDocker) Compose(ctx context.Context, c dockerx.ComposeCmd) (execx.Result, error) {
	f.record("compose " + strings.Join(c.Args, " ") + " -p " + c.Project)
	if len(c.Args) > 0 && c.Args[0] == "up" {
		if f.panicOnUp {
			panic("boom")
		}
		if c.Output != nil {
			_, _ = io.WriteString(c.Output, "Container started\n")
		}
		if f.upGate != nil {
			if err := f.upGate.hold(ctx); err != nil {
				return execx.Result{ExitCode: -1}, err
			}
		}
		if f.upErr != nil {
			return execx.Result{ExitCode: -1}, f.upErr
		}
		return execx.Result{ExitCode: f.upCode}, nil
	}
	return execx.Result{}, nil
}

func (f *fakeDocker) ComposeDown(_ context.Context, c dockerx.ComposeCmd) dockerx.DownResult {
	f.record("down -p " + c.Project)
	return dockerx.DownResult{OK: f.downOK}
}

func (f *fakeDocker) PullImage(ctx context.Context, image string, o dockerx.PullOptions) error {
	f.record("pull " + image)
	if f.pullGate != nil {
		if err := f.pullGate.hold(ctx); err != nil {
			return errors.New("Pull cancelled: " + image)
		}
	}
	return nil
}

func (f *fakeDocker) RemoveContainer(_ context.Context, name string) { f.record("rm " + name) }

func (f *fakeDocker) ImageExists(_ context.Context, ref string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.images[ref]
}

func (f *fakeDocker) ContainerLogs(_ context.Context, name string, _ int) string {
	return "logs of " + name
}

func (f *fakeDocker) ContainerStats(context.Context, string) *dockerx.ContainerStats { return nil }

// deploy.DockerOps

func (f *fakeDocker) ContainerStatus(context.Context, string) dockerx.ContainerStatus {
	return dockerx.ContainerStatus{Status: "running", Running: true}
}
func (f *fakeDocker) ContainerImageRef(context.Context, string) string { return "" }
func (f *fakeDocker) ConnectProxy(_ context.Context, network string) error {
	f.record("connect " + network)
	return f.connErr
}
func (f *fakeDocker) DisconnectProxy(_ context.Context, network string) {
	f.record("disconnect " + network)
}
func (f *fakeDocker) TeardownNetwork(context.Context, string) {}
func (f *fakeDocker) ConnectContainer(context.Context, string, string) error {
	return nil
}
func (f *fakeDocker) DisconnectContainer(context.Context, string, string) {}
func (f *fakeDocker) ListImages(context.Context) ([]dockerx.Image, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []dockerx.Image
	for ref := range f.images {
		out = append(out, dockerx.Image{ID: "sha256:" + ref, Tags: []string{ref}})
	}
	return out, nil
}
func (f *fakeDocker) RemoveImage(_ context.Context, ref string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "rmi "+ref)
	delete(f.images, ref)
	return true, nil
}
func (f *fakeDocker) BuilderPrune(context.Context, bool) dockerx.PruneResult {
	return dockerx.PruneResult{OK: true}
}

// build is the fake docker build / railpack runner: it records the image
// tag and makes the image exist on success.
func (f *fakeDocker) build(ctx context.Context, c execx.Cmd) (execx.Result, error) {
	tag := ""
	for i, a := range c.Args {
		if a == "-t" && i+1 < len(c.Args) {
			tag = c.Args[i+1]
		}
	}
	f.record("build " + tag)
	if c.Stdout != nil && f.buildOut != "" {
		_, _ = io.WriteString(c.Stdout, f.buildOut)
	}
	if f.buildGate != nil {
		if err := f.buildGate.hold(ctx); err != nil {
			return execx.Result{ExitCode: -1}, err
		}
	}
	if f.buildFail {
		return execx.Result{ExitCode: 1}, &execx.ExitError{Name: c.Name, Code: 1}
	}
	f.mu.Lock()
	f.built = append(f.built, tag)
	if tag != "" {
		f.images[tag] = true
	}
	f.mu.Unlock()
	return execx.Result{}, nil
}

// fakeEdge records vhost updates.
type fakeEdge struct {
	mu      sync.Mutex
	updated []string
	err     error
}

func (e *fakeEdge) UpdateServiceDomain(ctx context.Context, svc store.Service, o nginx.UpdateOptions) error {
	if o.ShouldContinue != nil && !o.ShouldContinue(ctx) {
		return nginx.ErrCancelled
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.updated = append(e.updated, svc.Name+":"+strVal(svc.Status))
	return e.err
}
func (e *fakeEdge) CleanupServiceDomain(context.Context, string) {}
func (e *fakeEdge) AssertHostnamesAvailable(context.Context, []string, string) error {
	return nil
}

// fakeGit is a checkout whose origin may carry credentials.
type fakeGit struct {
	mu        sync.Mutex
	origin    string
	pullCreds *gitops.Credentials
	pullErr   error
	scrubErr  error
	resets    []string
	head      string
}

func (g *fakeGit) Pull(_ context.Context, _ string, out io.Writer, o gitops.PullOptions) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.pullCreds = o.Credentials
	if o.Credentials != nil {
		// A misbehaving remote echoing the token must not reach the log.
		_, _ = io.WriteString(out, "remote: hello "+o.Credentials.Password+"\n")
	}
	return g.pullErr
}
func (g *fakeGit) LastCommitMessage(context.Context, string) (string, bool) { return "head msg", true }
func (g *fakeGit) CommitSHA(context.Context, string) (string, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.head, g.head != ""
}
func (g *fakeGit) ResetToCommit(_ context.Context, _ string, sha string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.resets = append(g.resets, sha)
	g.head = sha
	return nil
}
func (g *fakeGit) ScrubOrigin(_ context.Context, _ string, clean string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.scrubErr != nil {
		return g.scrubErr
	}
	g.origin = clean
	return nil
}
func (g *fakeGit) VerifyNoCredentials(context.Context, string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if gitops.HasCredentials(g.origin) {
		return gitops.ErrCredentialsRemain
	}
	return nil
}
func (g *fakeGit) SetOriginURL(_ context.Context, _ string, u string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.origin = u
	return nil
}

type fakeGitHub struct{ token string }

func (f fakeGitHub) InstallationIDForRepo(context.Context, string, string) (string, error) {
	return "42", nil
}
func (f fakeGitHub) InstallationToken(context.Context, string) (string, error) { return f.token, nil }

type env struct {
	d      *app.Deps
	s      *Service
	docker *fakeDocker
	edge   *fakeEdge
	git    *fakeGit
	h      http.Handler
	token  string
}

func setup(t *testing.T) *env {
	t.Helper()
	d := apptest.NewDeps(t)
	fd := newFakeDocker()
	d.Deploy.Docker = fd
	s := NewService(d)
	fe, fg := &fakeEdge{}, &fakeGit{}
	s.docker, s.edge, s.git, s.buildRun, s.dns = fd, fe, fg, fd.build, fakeDNS{}
	s.queue.retry = 20 * time.Millisecond
	_, token := apptest.CreateAdmin(t, d, "admin@example.test", "correct-horse")
	r := chi.NewRouter()
	r.Use(httpx.ParseBody)
	r.Route("/api/deployments", func(sub chi.Router) {
		sub.Use(d.Auth.RequireSession)
		MountWith(sub, d, s)
	})
	t.Cleanup(func() { s.Wait(context.Background()) })
	return &env{d: d, s: s, docker: fd, edge: fe, git: fg, h: r, token: token}
}

// project creates an upload project with files in its checkout.
func (e *env) project(t *testing.T, name string, in store.ProjectCreate, files map[string]string) *store.Project {
	t.Helper()
	in.Name = name
	if in.SourceType == nil {
		in.SourceType = store.Ptr("upload")
	}
	p, err := e.d.Store.CreateProject(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	dir := e.s.rt.ProjectPath(p.ID)
	for rel, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if len(files) == 0 {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

var dockerfileApp = map[string]string{"Dockerfile": "FROM alpine:3.20\nEXPOSE 8080\nCMD [\"sleep\",\"1\"]\n"}

// do sends a request and returns the recorder (the body is read to the end).
func (e *env) do(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+e.token)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func (e *env) deployment(t *testing.T, id string) *store.Deployment {
	t.Helper()
	d, err := e.d.Store.GetDeployment(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func waitRun(t *testing.T, run *Run) {
	t.Helper()
	select {
	case <-run.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("operation did not finish")
	}
}

func waitGate(t *testing.T, g *gate) {
	t.Helper()
	select {
	case <-g.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("command never started")
	}
}

// rows lists the project's deployments, newest first.
func (e *env) rows(t *testing.T, pid string) []*store.Deployment {
	t.Helper()
	rows, err := e.d.Store.ListDeployments(context.Background(), store.DeploymentFilter{ProjectID: &pid}, store.OrderCreatedDesc, store.Page{})
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// fakeDNS answers without network access.
type fakeDNS struct{}

func (fakeDNS) CheckDomains(context.Context, []string) []dnscheck.Check { return nil }
func (fakeDNS) GetServerPublicIP(context.Context) *string               { return nil }
