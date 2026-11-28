package system

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/moby/moby/api/types/container"

	"docklift/internal/app/apptest"
	"docklift/internal/db"
	"docklift/internal/dockerx"
	"docklift/internal/httpx"
	"docklift/internal/store"
	"docklift/internal/sysinfo/sysinfotest"
)

// fakeOps is the Docker side of deploy.Runtime.
type fakeOps struct {
	mu      sync.Mutex
	images  []dockerx.Image
	refs    map[string]string
	removed []string
	pruned  []bool
}

func (f *fakeOps) ContainerStatus(context.Context, string) dockerx.ContainerStatus {
	return dockerx.ContainerStatus{Status: dockerx.StatusNotFound}
}
func (f *fakeOps) ContainerImageRef(_ context.Context, name string) string { return f.refs[name] }
func (f *fakeOps) ConnectProxy(context.Context, string) error              { return nil }
func (f *fakeOps) DisconnectProxy(context.Context, string)                 {}
func (f *fakeOps) TeardownNetwork(context.Context, string)                 {}
func (f *fakeOps) ConnectContainer(context.Context, string, string) error  { return nil }
func (f *fakeOps) DisconnectContainer(context.Context, string, string)     {}
func (f *fakeOps) ListImages(context.Context) ([]dockerx.Image, error)     { return f.images, nil }
func (f *fakeOps) RemoveImage(_ context.Context, ref string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, ref)
	return true, nil
}
func (f *fakeOps) BuilderPrune(_ context.Context, all bool) dockerx.PruneResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pruned = append(f.pruned, all)
	return dockerx.PruneResult{OK: true}
}

// TestPurgeWithDeployRuntime runs POST /purge through the default wiring
// (Deps.Deploy): only Docklift tags outside every project's keep-2 set and
// not used by any container are removed, then BuildKit is pruned with -af.
func TestPurgeWithDeployRuntime(t *testing.T) {
	d := apptest.NewDeps(t)
	_, token := apptest.CreateAdmin(t, d, "admin@example.com", password)
	ops := &fakeOps{refs: map[string]string{"svc-c": "docklift-aaaaaaaa-api:99999999"}}
	d.Deploy.Docker = ops

	p, err := d.Store.CreateProject(t.Context(), store.ProjectCreate{Name: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Store.CreateService(t.Context(), store.ServiceCreate{ProjectID: p.ID, Name: "api", DockerfilePath: "Dockerfile", ContainerName: store.Ptr("svc-c")}); err != nil {
		t.Fatal(err)
	}
	success := "success"
	for _, tag := range []string{"11111111", "22222222", "", "33333333"} {
		in := store.DeploymentCreate{ProjectID: p.ID, Status: &success}
		if tag != "" {
			j := db.JSON(`{"api":"docklift-aaaaaaaa-api:` + tag + `"}`)
			in.ImageTags = &j
		}
		if _, err := d.Store.CreateDeployment(t.Context(), in); err != nil {
			t.Fatal(err)
		}
		time.Sleep(3 * time.Millisecond) // distinct created_at
	}
	ops.images = []dockerx.Image{
		{ID: "sha256:1", Tags: []string{"docklift-aaaaaaaa-api:11111111"}},         // older than keep-2: removed
		{ID: "sha256:2", Tags: []string{"docklift-aaaaaaaa-api:22222222"}},         // previous: kept
		{ID: "sha256:3", Tags: []string{"docklift-aaaaaaaa-api:33333333"}},         // current: kept
		{ID: "sha256:9", Tags: []string{"docklift-aaaaaaaa-api:99999999"}},         // service container: in use
		{ID: "sha256:8", Tags: []string{"docklift-cccccccc-orphan:88888888"}},      // stopped container: in use
		{ID: "sha256:7", Tags: []string{"docklift-cccccccc-orphan:77777777"}},      // unused: removed
		{ID: "sha256:6", Tags: []string{"postgres:16", "docklift-backend:latest"}}, // foreign: never listed
	}
	docker := &fakeDocker{containers: []container.Summary{{Image: "docklift-cccccccc-orphan:88888888", ImageID: "sha256:8"}}}

	r := chi.NewRouter()
	r.Use(httpx.ParseBody, d.Auth.RequireSession)
	zero := time.Duration(0)
	MountWith(r, d, Options{Docker: docker, Source: &sysinfotest.Source{}, GOOS: "linux", SimulatedDelay: &zero})
	e := &env{d: d, h: r, token: token}
	res := e.do(t, "POST", "/purge", map[string]any{"password": password})
	if res.code != 200 {
		t.Fatalf("purge: %d %s", res.code, res.raw)
	}
	slices.Sort(ops.removed)
	if got := strings.Join(ops.removed, " "); got != "docklift-aaaaaaaa-api:11111111 docklift-cccccccc-orphan:77777777" {
		t.Fatalf("removed %q", got)
	}
	if !slices.Equal(ops.pruned, []bool{true}) {
		t.Fatalf("BuildKit prune calls %v, want one `builder prune -af`", ops.pruned)
	}
	if res.body["removedImages"] != 2.0 || res.body["keptImages"] != 2.0 {
		t.Fatalf("body %s", res.raw)
	}
}
