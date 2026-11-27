package ports

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"docklift/internal/app"
	"docklift/internal/app/apptest"
	"docklift/internal/store"
)

type fakeChecker map[string]bool

func (f fakeChecker) Running(_ context.Context, name string) bool { return f[name] }

func setup(t *testing.T, running fakeChecker) (*app.Deps, http.Handler) {
	t.Helper()
	d := apptest.NewDeps(t)
	d.Config.PortRangeStart, d.Config.PortRangeEnd = 5500, 5504
	r := chi.NewRouter()
	MountWith(r, d, running)
	return d, r
}

func do(t *testing.T, h http.Handler, method, path string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("%s %s: %d %q", method, path, rec.Code, rec.Body.String())
	}
	return rec.Code, body
}

func project(t *testing.T, d *app.Deps, name, status, typ string, publish bool, port *int) *store.Project {
	t.Helper()
	p, err := d.Store.CreateProject(context.Background(), store.ProjectCreate{Name: name, Status: &status, ProjectType: &typ,
		PublishHostPort: &publish, Port: port})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestList(t *testing.T) {
	d, h := setup(t, nil)
	ctx := context.Background()
	held := project(t, d, "held", "running", "app", true, store.Ptr(5501))
	if _, err := d.Store.InsertLockedPort(ctx, 5501, held.ID); err != nil {
		t.Fatal(err)
	}
	// A row outside the pool still marks its project as allocated.
	outside := project(t, d, "outside", "running", "app", true, nil)
	if _, err := d.Store.InsertLockedPort(ctx, 9000, outside.ID); err != nil {
		t.Fatal(err)
	}
	private := project(t, d, "private", "pending", "app", false, nil)
	publish := project(t, d, "publish", "degraded", "app", true, nil)
	dbp := project(t, d, "db", "building", "database", false, nil)
	project(t, d, "stopped", "stopped", "app", false, nil)
	withSvc := project(t, d, "with-service-port", "running", "app", false, nil)
	if _, err := d.Store.CreateService(ctx, store.ServiceCreate{ProjectID: withSvc.ID, Name: "app", Port: store.Ptr(7000)}); err != nil {
		t.Fatal(err)
	}
	zeroSvc := project(t, d, "zero-port-service", "running", "app", false, nil)
	if _, err := d.Store.CreateService(ctx, store.ServiceCreate{ProjectID: zeroSvc.ID, Name: "app", Port: store.Ptr(0)}); err != nil {
		t.Fatal(err)
	}

	code, body := do(t, h, "GET", "/")
	if code != 200 {
		t.Fatalf("code %d", code)
	}
	ports := body["ports"].([]any)
	if len(ports) != 5 {
		t.Fatalf("ports = %v", ports)
	}
	free := ports[0].(map[string]any)
	if len(free) != 3 || free["port"] != float64(5500) || free["project_id"] != nil || free["is_locked"] != false {
		t.Errorf("free entry = %v", free)
	}
	alloc := ports[1].(map[string]any)
	proj, _ := alloc["project"].(map[string]any)
	if alloc["port"] != float64(5501) || alloc["project_id"] != held.ID || alloc["is_locked"] != true || proj["id"] != held.ID || proj["name"] != "held" || proj["status"] != "running" {
		t.Errorf("allocated entry = %v", alloc)
	}
	if pool := body["pool"].(map[string]any); pool["start"] != float64(5500) || pool["end"] != float64(5504) {
		t.Errorf("pool = %v", pool)
	}

	got := map[string]map[string]any{}
	for _, e := range body["private_running"].([]any) {
		m := e.(map[string]any)
		got[m["id"].(string)] = m
	}
	if len(got) != 4 {
		t.Errorf("private_running = %v", body["private_running"])
	}
	for id, reason := range map[string]string{private.ID: reasonPrivate, publish.ID: reasonPublish, dbp.ID: reasonDatabase, zeroSvc.ID: reasonPrivate} {
		if got[id]["reason"] != reason {
			t.Errorf("%s reason = %v", id, got[id]["reason"])
		}
	}
	if p := got[dbp.ID]; p["project_type"] != "database" || p["db_engine"] != nil || p["status"] != "building" || p["publish_host_port"] != false || len(p) != 7 {
		t.Errorf("db entry = %v", p)
	}
}

func TestRelease(t *testing.T) {
	d, h := setup(t, fakeChecker{"live-container": true})
	ctx := context.Background()
	for _, bad := range []string{"abc", "77abc", "-1", "+5", "1.5", "99999999999999999999999"} {
		if code, body := do(t, h, "DELETE", "/"+bad); code != 400 || body["error"] != "Invalid port" {
			t.Errorf("DELETE %s = %d %v", bad, code, body)
		}
	}
	if _, err := d.Store.InsertLockedPort(ctx, 5502, project(t, d, "gone", "stopped", "app", false, nil).ID); err != nil {
		t.Fatal(err)
	}
	if code, body := do(t, h, "DELETE", "/5502"); code != 200 || body["status"] != "deleted" {
		t.Errorf("delete = %d %v", code, body)
	}
	if row, err := d.Store.GetPort(ctx, 5502); err == nil {
		t.Errorf("row still present: %v", row)
	}
	if code, _ := do(t, h, "DELETE", "/99999"); code != 200 {
		t.Errorf("missing row = %d", code)
	}

	p := project(t, d, "svc", "running", "app", false, nil)
	if _, err := d.Store.CreateService(ctx, store.ServiceCreate{ProjectID: p.ID, Name: "a", Port: store.Ptr(5503), ContainerName: store.Ptr("live-container")}); err != nil {
		t.Fatal(err)
	}
	if code, body := do(t, h, "DELETE", "/5503"); code != 409 || body["error"] != "Port is in use by a running container" {
		t.Errorf("running = %d %v", code, body)
	}
	if _, err := d.Store.CreateService(ctx, store.ServiceCreate{ProjectID: p.ID, Name: "b", Port: store.Ptr(5504), ContainerName: store.Ptr("stopped-container")}); err != nil {
		t.Fatal(err)
	}
	if code, body := do(t, h, "DELETE", "/5504"); code != 409 || !strings.Contains(body["error"].(string), "still assigned") {
		t.Errorf("assigned service = %d %v", code, body)
	}
	project(t, d, "remembers", "stopped", "app", false, store.Ptr(5600))
	if code, body := do(t, h, "DELETE", "/5600"); code != 409 || body["error"] != "Port is still assigned to a service or project" {
		t.Errorf("project port = %d %v", code, body)
	}
}

func TestEngineCheckerWithoutDocker(t *testing.T) {
	if (engineChecker{}).Running(context.Background(), "x") {
		t.Error("nil client must report not running")
	}
}
