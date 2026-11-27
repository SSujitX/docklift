package deploy

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"

	"docklift/internal/db"
	"docklift/internal/dockerx"
	"docklift/internal/store"
)

// fakeDocker implements DockerOps in memory.
type fakeDocker struct {
	mu         sync.Mutex
	calls      []string
	statuses   map[string]dockerx.ContainerStatus
	imageRefs  map[string]string
	images     []dockerx.Image
	removeErr  map[string]error
	connectErr map[string]error
	// networkErr fails a connect to that network (e.g. a not-found error).
	networkErr map[string]error
	prune      dockerx.PruneResult
}

func newFakeDocker() *fakeDocker {
	return &fakeDocker{
		statuses:   map[string]dockerx.ContainerStatus{},
		imageRefs:  map[string]string{},
		removeErr:  map[string]error{},
		connectErr: map[string]error{},
		networkErr: map[string]error{},
		prune:      dockerx.PruneResult{OK: true},
	}
}

func (f *fakeDocker) record(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, s)
}

func (f *fakeDocker) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeDocker) ContainerStatus(_ context.Context, name string) dockerx.ContainerStatus {
	f.record("status " + name)
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := f.statuses[name]; ok {
		return s
	}
	return dockerx.ContainerStatus{Status: dockerx.StatusNotFound}
}

func (f *fakeDocker) ContainerImageRef(_ context.Context, name string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.imageRefs[name]
}

func (f *fakeDocker) ConnectProxy(ctx context.Context, network string) error {
	return f.ConnectContainer(ctx, network, "proxy")
}

func (f *fakeDocker) DisconnectProxy(ctx context.Context, network string) {
	f.DisconnectContainer(ctx, network, "proxy")
}

func (f *fakeDocker) TeardownNetwork(_ context.Context, network string) {
	f.record("teardown " + network)
}

func (f *fakeDocker) ConnectContainer(_ context.Context, network, container string) error {
	f.record("connect " + network + " " + container)
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.networkErr[network]; err != nil {
		return err
	}
	return f.connectErr[container]
}

func (f *fakeDocker) DisconnectContainer(_ context.Context, network, container string) {
	f.record("disconnect " + network + " " + container)
}

func (f *fakeDocker) ListImages(context.Context) ([]dockerx.Image, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]dockerx.Image(nil), f.images...), nil
}

func (f *fakeDocker) RemoveImage(_ context.Context, ref string) (bool, error) {
	f.record("rmi " + ref)
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.removeErr[ref]; err != nil {
		return false, err
	}
	return true, nil
}

func (f *fakeDocker) BuilderPrune(_ context.Context, all bool) dockerx.PruneResult {
	if all {
		f.record("prune -af")
	} else {
		f.record("prune -f")
	}
	return f.prune
}

// newTestRuntime returns a Runtime over a migrated temporary database and a
// fake Docker.
func newTestRuntime(t *testing.T) (*Runtime, *fakeDocker) {
	t.Helper()
	dir := t.TempDir()
	ctx := context.Background()
	database, err := db.Open(ctx, filepath.Join(dir, "data", "docklift.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	fd := newFakeDocker()
	r := &Runtime{
		Store:           store.New(database),
		Docker:          fd,
		Tracker:         NewTracker(),
		Hub:             NewHubTags(nil, nil),
		DeploymentsPath: filepath.Join(dir, "deployments"),
		PortRangeStart:  5500,
		PortRangeEnd:    5503,
		Log:             slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	return r, fd
}

func mustProject(t *testing.T, r *Runtime, in store.ProjectCreate) *store.Project {
	t.Helper()
	p, err := r.Store.CreateProject(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func mustService(t *testing.T, r *Runtime, in store.ServiceCreate) *store.Service {
	t.Helper()
	s, err := r.Store.CreateService(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func mustDeployment(t *testing.T, r *Runtime, in store.DeploymentCreate) *store.Deployment {
	t.Helper()
	d, err := r.Store.CreateDeployment(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func strp(s string) *string { return &s }

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
