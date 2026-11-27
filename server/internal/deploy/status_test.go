package deploy

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"docklift/internal/dockerx"
	"docklift/internal/store"
)

func TestAggregateProjectStatus(t *testing.T) {
	// projectStatusSync.test.ts plus edges.
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"running", "running"}, "running"},
		{[]string{"running", "error"}, "error"},
		{[]string{"running", "stopped"}, "degraded"},
		{[]string{"stopped", "stopped"}, "stopped"},
		{nil, "stopped"},
		{[]string{"error", "stopped"}, "error"},
		{[]string{"running"}, "running"},
	}
	for _, c := range cases {
		if got := AggregateProjectStatus(c.in); got != c.want {
			t.Errorf("%v -> %s, want %s", c.in, got, c.want)
		}
	}
}

func TestDockerStatusToServiceStatus(t *testing.T) {
	for cs, want := range map[dockerx.ContainerStatus]string{
		{Status: "running", Running: true}: "running",
		{Status: "not_found"}:              "stopped",
		{Status: "dead"}:                   "error",
		{Status: "exited"}:                 "stopped",
		{Status: "restarting"}:             "stopped",
	} {
		if got := DockerStatusToServiceStatus(cs); got != want {
			t.Errorf("%+v -> %s", cs, got)
		}
	}
}

func TestSyncProjectStatus(t *testing.T) {
	r, fd := newTestRuntime(t)
	ctx := context.Background()
	p := mustProject(t, r, store.ProjectCreate{Name: "shop", Status: strp("building")})
	web := mustService(t, r, store.ServiceCreate{ProjectID: p.ID, Name: "web", ContainerName: strp("c_web"), Status: strp("building")})
	api := mustService(t, r, store.ServiceCreate{ProjectID: p.ID, Name: "api", ContainerName: strp("c_api"), Status: strp("stopped")})
	worker := mustService(t, r, store.ServiceCreate{ProjectID: p.ID, Name: "worker"})
	fd.statuses["c_web"] = dockerx.ContainerStatus{Status: "running", Running: true}
	fd.statuses["c_api"] = dockerx.ContainerStatus{Status: "exited"}

	got, err := r.SyncProjectStatus(ctx, p.ID)
	if err != nil || got != StatusDegraded {
		t.Fatalf("status %q err %v", got, err)
	}
	for id, want := range map[string]string{web.ID: "running", api.ID: "stopped", worker.ID: "stopped"} {
		s, _ := r.Store.GetService(ctx, id)
		if deref(s.Status) != want {
			t.Errorf("service %s status %s, want %s", s.Name, deref(s.Status), want)
		}
	}
	pr, _ := r.Store.GetProject(ctx, p.ID)
	if deref(pr.Status) != StatusDegraded {
		t.Fatalf("project %s", deref(pr.Status))
	}
	if _, err := r.SyncProjectStatus(ctx, "missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing project: %v", err)
	}
}

func TestRecoverOnBoot(t *testing.T) {
	r, fd := newTestRuntime(t)
	ctx := context.Background()
	running := mustProject(t, r, store.ProjectCreate{Name: "a", Status: strp("building")})
	mustService(t, r, store.ServiceCreate{ProjectID: running.ID, Name: "app", ContainerName: strp("c_a"), Status: strp("building")})
	dead := mustProject(t, r, store.ProjectCreate{Name: "b", Status: strp("building")})
	deadSvc := mustService(t, r, store.ServiceCreate{ProjectID: dead.ID, Name: "app", ContainerName: strp("c_b"), Status: strp("building")})
	idle := mustProject(t, r, store.ProjectCreate{Name: "c", Status: strp("running")})
	fd.statuses["c_a"] = dockerx.ContainerStatus{Status: "running", Running: true}

	inProg := mustDeployment(t, r, store.DeploymentCreate{ProjectID: running.ID, Status: strp(StatusInProgress), Logs: strp("🚀 Starting deployment...\n")})
	noLogs := mustDeployment(t, r, store.DeploymentCreate{ProjectID: dead.ID, Status: strp(StatusInProgress)})
	done := mustDeployment(t, r, store.DeploymentCreate{ProjectID: idle.ID, Status: strp(StatusSuccess), Logs: strp("ok")})

	if err := r.RecoverOnBoot(ctx); err != nil {
		t.Fatal(err)
	}
	d, _ := r.Store.GetDeployment(ctx, inProg.ID)
	if deref(d.Status) != StatusFailed || deref(d.Logs) != "🚀 Starting deployment...\n\n❌ interrupted by restart\n" || d.FinishedAt == nil {
		t.Fatalf("in-progress row %+v logs %q", d, deref(d.Logs))
	}
	d, _ = r.Store.GetDeployment(ctx, noLogs.ID)
	if deref(d.Logs) != "\n❌ interrupted by restart\n" {
		t.Fatalf("null logs %q", deref(d.Logs))
	}
	d, _ = r.Store.GetDeployment(ctx, done.ID)
	if deref(d.Status) != StatusSuccess || deref(d.Logs) != "ok" || d.FinishedAt != nil {
		t.Fatal("finished rows must be untouched")
	}
	for id, want := range map[string]string{running.ID: "running", dead.ID: "error", idle.ID: "running"} {
		p, _ := r.Store.GetProject(ctx, id)
		if deref(p.Status) != want {
			t.Errorf("project %s: %s, want %s", p.Name, deref(p.Status), want)
		}
	}
	s, _ := r.Store.GetService(ctx, deadSvc.ID)
	if deref(s.Status) != StatusError {
		t.Fatalf("dead service %s", deref(s.Status))
	}
	if slices.Contains(fd.Calls(), "status c_idle") {
		t.Fatal("only building projects are inspected")
	}
}

func TestFailDeploymentNeverOverwritesCancel(t *testing.T) {
	r, _ := newTestRuntime(t)
	ctx := context.Background()
	p := mustProject(t, r, store.ProjectCreate{Name: "a", Status: strp("building")})
	svc := mustService(t, r, store.ServiceCreate{ProjectID: p.ID, Name: "app", Status: strp("building")})
	cancelled := mustDeployment(t, r, store.DeploymentCreate{ProjectID: p.ID, Status: strp(StatusCancelled)})
	if r.FailDeployment(ctx, p.ID, cancelled.ID, strp("logs")) {
		t.Fatal("cancelled row was failed")
	}
	if pr, _ := r.Store.GetProject(ctx, p.ID); deref(pr.Status) != "building" {
		t.Fatal("project status changed for a cancelled deployment")
	}
	running := mustDeployment(t, r, store.DeploymentCreate{ProjectID: p.ID, Status: strp(StatusInProgress)})
	if !r.FailDeployment(ctx, p.ID, running.ID, strp("❌ Error: x\n")) {
		t.Fatal("fail")
	}
	d, _ := r.Store.GetDeployment(ctx, running.ID)
	pr, _ := r.Store.GetProject(ctx, p.ID)
	s, _ := r.Store.GetService(ctx, svc.ID)
	if deref(d.Status) != StatusFailed || deref(d.Logs) != "❌ Error: x\n" || d.FinishedAt == nil || deref(pr.Status) != "error" || deref(s.Status) != "error" {
		t.Fatalf("deployment %s project %s service %s", deref(d.Status), deref(pr.Status), deref(s.Status))
	}
	// nil logs keep the stored logs.
	keep := mustDeployment(t, r, store.DeploymentCreate{ProjectID: p.ID, Status: strp(StatusInProgress), Logs: strp("kept")})
	r.FailDeployment(ctx, p.ID, keep.ID, nil)
	if d, _ := r.Store.GetDeployment(ctx, keep.ID); deref(d.Logs) != "kept" {
		t.Fatal("logs overwritten")
	}
}

func TestIsDeploymentCancelled(t *testing.T) {
	r, _ := newTestRuntime(t)
	ctx := context.Background()
	p := mustProject(t, r, store.ProjectCreate{Name: "a"})
	d := mustDeployment(t, r, store.DeploymentCreate{ProjectID: p.ID, Status: strp(StatusInProgress)})
	if r.IsDeploymentCancelled(ctx, p.ID, d.ID) {
		t.Fatal("not cancelled")
	}
	if _, err := r.Store.UpdateDeployment(ctx, d.ID, store.DeploymentUpdate{Status: store.Some(strp(StatusCancelled))}); err != nil {
		t.Fatal(err)
	}
	if !r.IsDeploymentCancelled(ctx, p.ID, d.ID) {
		t.Fatal("row cancelled")
	}
	r.Tracker.MarkCancelled("other")
	if !r.IsDeploymentCancelled(ctx, p.ID, "other") {
		t.Fatal("marked cancelled")
	}
}

func TestAllocatePort(t *testing.T) {
	r, _ := newTestRuntime(t)
	ctx := context.Background()
	a := mustProject(t, r, store.ProjectCreate{Name: "a"})
	b := mustProject(t, r, store.ProjectCreate{Name: "b"})
	// 5500 locked by a; 5501 owned (unlocked) by a; 5502 free row; 5503 no row.
	if ok, err := r.Store.InsertLockedPort(ctx, 5500, a.ID); err != nil || !ok {
		t.Fatal(err)
	}
	if ok, err := r.Store.InsertLockedPort(ctx, 5501, a.ID); err != nil || !ok {
		t.Fatal(err)
	}
	if _, err := r.Store.DB().Writer().ExecContext(ctx, `UPDATE "ports" SET "is_locked" = 0 WHERE "port" = 5501`); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Store.DB().Writer().ExecContext(ctx, `INSERT INTO "ports" ("port", "project_id", "is_locked") VALUES (5502, NULL, 0)`); err != nil {
		t.Fatal(err)
	}
	port, err := r.AllocatePort(ctx, b.ID)
	if err != nil || port != 5502 {
		t.Fatalf("port %d err %v", port, err)
	}
	row, _ := r.Store.GetPort(ctx, 5502)
	if deref(row.ProjectID) != b.ID || row.IsLocked == nil || !*row.IsLocked {
		t.Fatalf("row %+v", row)
	}
	if pr, _ := r.Store.GetProject(ctx, b.ID); pr.Port == nil || *pr.Port != 5502 {
		t.Fatal("project port column not set")
	}
	if port, err = r.AllocatePort(ctx, b.ID); err != nil || port != 5503 {
		t.Fatalf("port %d err %v", port, err)
	}
	if _, err = r.AllocatePort(ctx, b.ID); err == nil || err.Error() != "No free ports in range 5500-5503" {
		t.Fatalf("exhausted: %v", err)
	}
	if err := r.ReleaseServicePort(ctx, b.ID, 5503); err != nil {
		t.Fatal(err)
	}
	if port, err = r.AllocatePort(ctx, a.ID); err != nil || port != 5503 {
		t.Fatalf("released port reused: %d %v", port, err)
	}
}

func TestAllocatePortConcurrentUnique(t *testing.T) {
	r, _ := newTestRuntime(t)
	r.PortRangeEnd = 5520
	ctx := context.Background()
	var ids []string
	for i := range 8 {
		ids = append(ids, mustProject(t, r, store.ProjectCreate{Name: fmt.Sprint("p", i)}).ID)
	}
	var mu sync.Mutex
	seen := map[int]bool{}
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Go(func() {
			port, err := r.AllocatePort(ctx, id)
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if seen[port] {
				t.Errorf("port %d allocated twice", port)
			}
			seen[port] = true
		})
	}
	wg.Wait()
}

func TestRuntimePathsAndNetworkHelpers(t *testing.T) {
	r, fd := newTestRuntime(t)
	ctx := context.Background()
	id := "abcdef12-3456-4789-8abc-def012345678"
	if r.ProjectPath(id) != filepath.Join(r.DeploymentsPath, id) || r.ComposePath(id) != filepath.Join(r.DeploymentsPath, ".docklift", id, "compose.yml") {
		t.Fatal("paths")
	}
	if _, err := r.ComposeFiles(id); !errors.Is(err, ErrRuntimeMissing) {
		t.Fatalf("err %v", err)
	}
	if err := WriteRuntimeCompose(r.ComposePath(id), nil, nil, RuntimeComposeOptions{ProjectID: id}); err != nil {
		t.Fatal(err)
	}
	if files, err := r.ComposeFiles(id); err != nil || len(files) != 1 || files[0] != r.ComposePath(id) {
		t.Fatalf("files %v err %v", files, err)
	}
	_ = r.ConnectProxyToProjectNetwork(ctx, id)
	r.DisconnectProxyFromProjectNetwork(ctx, id)
	r.TeardownProjectNetwork(ctx, id)
	_ = r.ConnectContainerToProjectNetwork(ctx, id, "dl_db")
	r.DisconnectContainerFromProjectNetwork(ctx, id, "dl_db")
	want := []string{"connect dl-net-abcdef12 proxy", "disconnect dl-net-abcdef12 proxy", "teardown dl-net-abcdef12", "connect dl-net-abcdef12 dl_db", "disconnect dl-net-abcdef12 dl_db"}
	if !slices.Equal(fd.Calls(), want) {
		t.Fatalf("calls %v", fd.Calls())
	}
}
