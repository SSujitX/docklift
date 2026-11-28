package deployments

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"docklift/internal/deploy"
	"docklift/internal/dockerx"
	"docklift/internal/store"
)

func decode(t *testing.T, rec *httptest.ResponseRecorder) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("%d %q: %v", rec.Code, rec.Body.String(), err)
	}
	return v
}

func expectError(t *testing.T, rec *httptest.ResponseRecorder, status int, msg string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status %d, want %d: %s", rec.Code, status, rec.Body.String())
	}
	if got := decode(t, rec).(map[string]any)["error"]; got != msg {
		t.Fatalf("error = %v, want %q", got, msg)
	}
}

func TestDeployRoute(t *testing.T) {
	e := setup(t)
	p := e.project(t, "web", store.ProjectCreate{}, dockerfileApp)
	expectError(t, e.do(t, "POST", "/api/deployments/missing/deploy", ""), 404, "Project not found")
	expectError(t, e.do(t, "POST", "/api/deployments/"+p.ID+"/deploy", `{"trigger":5}`), 400, "trigger must be a string")

	rec := e.do(t, "POST", "/api/deployments/"+p.ID+"/deploy", `{"trigger":"webhook","commit_message":"msg"}`)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatalf("deploy: %d %v", rec.Code, rec.Header())
	}
	body := rec.Body.String()
	if !strings.HasPrefix(body, "🚀 Starting deployment...\n") {
		t.Fatalf("body = %q", body)
	}
	mustContain(t, "deploy body", body, "📍 Host ports disabled", "📊 Deployment complete! Status: SUCCESS ✅")
	last := e.rows(t, p.ID)[0]
	if strVal(last.Trigger) != "webhook" || strVal(last.CommitMessage) != "msg" {
		t.Fatalf("row = %s / %s", strVal(last.Trigger), strVal(last.CommitMessage))
	}
	// redeploy is the same pipeline; no body means trigger manual.
	rec = e.do(t, "POST", "/api/deployments/"+p.ID+"/redeploy", "")
	mustContain(t, "redeploy", rec.Body.String(), "Updating existing service: app\n")
	if strVal(e.rows(t, p.ID)[0].Trigger) != "manual" {
		t.Fatal("default trigger must be manual")
	}
}

func TestDeployContinuesAfterClientDisconnect(t *testing.T) {
	e := setup(t)
	e.docker.buildGate = newGate()
	p := e.project(t, "web", store.ProjectCreate{}, dockerfileApp)
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("POST", "/api/deployments/"+p.ID+"/deploy", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+e.token)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		e.h.ServeHTTP(rec, req)
		close(done)
	}()
	waitGate(t, e.docker.buildGate)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not return after the client left")
	}
	close(e.docker.buildGate.release)
	waitFor(t, "deploy to finish", func() bool {
		rows := e.rows(t, p.ID)
		return len(rows) == 1 && strVal(rows[0].Status) == deploy.StatusSuccess
	})
}

func TestStopRestartBeforeDeployAreJSON(t *testing.T) {
	e := setup(t)
	p := e.project(t, "web", store.ProjectCreate{}, dockerfileApp)
	for _, action := range []string{"stop", "restart"} {
		expectError(t, e.do(t, "POST", "/api/deployments/"+p.ID+"/"+action, ""), 400, deploy.ErrRuntimeMissing.Error())
		expectError(t, e.do(t, "POST", "/api/deployments/missing/"+action, ""), 404, "Project not found")
	}
	if len(e.rows(t, p.ID)) != 0 {
		t.Fatal("refused stop/restart must not create rows")
	}
	proj, _ := e.d.Store.GetProject(context.Background(), p.ID)
	if strVal(proj.Status) == deploy.StatusError {
		t.Fatal("refused restart must not touch the project status")
	}
}

func TestIdleCancelRoute(t *testing.T) {
	e := setup(t)
	p := e.project(t, "web", store.ProjectCreate{}, dockerfileApp)
	expectError(t, e.do(t, "POST", "/api/deployments/missing/cancel", ""), 404, "Project not found")
	e.docker.downOK = false
	rec := e.do(t, "POST", "/api/deployments/"+p.ID+"/cancel", "")
	body := rec.Body.String()
	if rec.Code != 200 || !strings.HasPrefix(body, "❌ Cancelling — tearing down so you can start fresh...\n") {
		t.Fatalf("cancel: %d %q", rec.Code, body)
	}
	mustContain(t, "cancel", body, "ℹ️ No tracked build process", "ℹ️ No active deploy row", "🛑 compose down -p dl-web-",
		"❌ Cancel teardown incomplete for \"dl-web-", "❌ Compose teardown failed; project status unchanged\n")
	if e.s.rt.Tracker.IsDeploying(p.ID) {
		t.Fatal("cancel lock must be released")
	}
	if len(e.rows(t, p.ID)) != 0 {
		t.Fatal("idle cancel must not add rows")
	}
}

func TestListRoute(t *testing.T) {
	e := setup(t)
	p := e.project(t, "web", store.ProjectCreate{}, dockerfileApp)
	ctx := context.Background()
	var ids []string
	for i, status := range []string{"failed", "success", "failed"} {
		d, err := e.d.Store.CreateDeployment(ctx, store.DeploymentCreate{ProjectID: p.ID, Status: store.Ptr(status)})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, d.ID)
		_ = i
		time.Sleep(3 * time.Millisecond)
	}
	list := func(q string) []any {
		rec := e.do(t, "GET", "/api/deployments/"+p.ID+q, "")
		if rec.Code != 200 {
			t.Fatalf("%s: %d", q, rec.Code)
		}
		return decode(t, rec).([]any)
	}
	if got := list(""); len(got) != 3 || got[0].(map[string]any)["id"] != ids[2] {
		t.Fatalf("default list = %v", got)
	}
	if got := list("?limit=abc&offset=-x"); len(got) != 3 {
		t.Fatalf("lenient limit = %d", len(got))
	}
	if got := list("?limit=1&offset=1"); len(got) != 1 || got[0].(map[string]any)["id"] != ids[1] {
		t.Fatalf("offset = %v", got)
	}
	if got := list("?limit=2px"); len(got) != 2 {
		t.Fatalf("parseInt limit = %d", len(got))
	}
	if got := list("?limit=-1"); len(got) != 1 || got[0].(map[string]any)["id"] != ids[0] {
		t.Fatalf("negative take = %v", got)
	}
	rec := e.do(t, "GET", "/api/deployments/"+p.ID+"?meta=1&status=%20failed%20&limit=1", "")
	m := decode(t, rec).(map[string]any)
	if m["total"] != float64(2) || len(m["items"].([]any)) != 1 {
		t.Fatalf("meta = %v", m)
	}
	rec = e.do(t, "GET", "/api/deployments/"+p.ID+"?offset=-1", "")
	expectError(t, rec, 500, "Failed to list deployments")
}

func TestLogsAndStatsRoutes(t *testing.T) {
	e := setup(t)
	expectEmpty := func(path, want string) {
		rec := e.do(t, "GET", path, "")
		if rec.Code != 200 || rec.Body.String() != want {
			t.Fatalf("%s = %d %s", path, rec.Code, rec.Body.String())
		}
	}
	expectEmpty("/api/deployments/missing/logs", `{"logs":{}}`)
	expectEmpty("/api/deployments/missing/stats", `{}`)

	p := e.project(t, "web", store.ProjectCreate{}, dockerfileApp)
	ctx := context.Background()
	for _, name := range []string{"zeta", "alpha"} {
		if _, err := e.d.Store.CreateService(ctx, store.ServiceCreate{ProjectID: p.ID, Name: name, ContainerName: store.Ptr("c_" + name)}); err != nil {
			t.Fatal(err)
		}
	}
	expectEmpty("/api/deployments/"+p.ID+"/logs", `{"logs":{"zeta":"logs of c_zeta","alpha":"logs of c_alpha"}}`)
	expectEmpty("/api/deployments/"+p.ID+"/stats", `{"zeta":null,"alpha":null}`)
	e.docker.pingErr = dockerx.ErrUnavailable
	expectError(t, e.do(t, "GET", "/api/deployments/"+p.ID+"/stats", ""), 503, "Docker is not available")
	expectError(t, e.do(t, "GET", "/api/deployments/"+p.ID+"/logs", ""), 503, "Docker is not available")
}

func TestRollbackRouteValidation(t *testing.T) {
	e := setup(t)
	r := "/api/deployments/missing/rollback"
	rec := e.do(t, "POST", r, "")
	if rec.Code != 403 || !strings.Contains(rec.Body.String(), `"requirePassword":true`) {
		t.Fatalf("no password: %d %s", rec.Code, rec.Body.String())
	}
	expectError(t, e.do(t, "POST", r, `{"password":"correct-horse"}`), 400, "deploymentId is required")
	expectError(t, e.do(t, "POST", r, `{"password":"correct-horse","deploymentId":"  "}`), 400, "deploymentId is required")
	expectError(t, e.do(t, "POST", r, `{"password":"correct-horse","deploymentId":"abc"}`), 404, "Project not found")
	p := e.project(t, "web", store.ProjectCreate{}, dockerfileApp)
	expectError(t, e.do(t, "POST", "/api/deployments/"+p.ID+"/rollback", `{"password":"correct-horse","deploymentId":"abc"}`),
		404, "Target successful deployment not found")
}

func TestServiceDomainRoutes(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	p := e.project(t, "web", store.ProjectCreate{}, dockerfileApp)
	other := e.project(t, "other", store.ProjectCreate{}, dockerfileApp)
	svc, err := e.d.Store.CreateService(ctx, store.ServiceCreate{ProjectID: p.ID, Name: "app", ContainerName: store.Ptr("c_app"), InternalPort: store.Ptr(8080)})
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/deployments/" + p.ID + "/services/" + svc.ID
	expectError(t, e.do(t, "GET", "/api/deployments/"+other.ID+"/services/"+svc.ID+"/ssl", ""), 404, "Service not found")
	expectError(t, e.do(t, "PUT", base, `{"domain":5}`), 400, "Domain must be a comma-separated string")
	expectError(t, e.do(t, "PUT", base, `{"domain":"bad_domain.test"}`), 400, domainFormatMsg)
	expectError(t, e.do(t, "POST", base+"/ssl/retry", ""), 400, "Service has no domain or is not deployed")

	rec := e.do(t, "PUT", base, `{"domain":"App.example.test, app.example.test,www.example.test"}`)
	if rec.Code != 200 {
		t.Fatalf("put: %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.HasPrefix(body, `{"success":true,"ssl":{"app.example.test":{"status":"missing","domain":"app.example.test","expiresAt":null,"error":null,"diagnosticCommand":null,"certPath":null,"expired":false,"renewalOverdue":false,"daysUntilExpiry":null},"www.example.test":`) {
		t.Fatalf("put body = %s", body)
	}
	mustContain(t, "events", body, `"message":"Domain added: app.example.test, www.example.test"`)
	got, _ := e.d.Store.GetService(ctx, svc.ID)
	if strVal(got.Domain) != "app.example.test, www.example.test" {
		t.Fatalf("domain = %q", strVal(got.Domain))
	}

	// No domain field: unchanged (deliberate fix 5).
	rec = e.do(t, "PUT", base, `{}`)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"app.example.test"`) {
		t.Fatalf("put {}: %d %s", rec.Code, rec.Body.String())
	}
	if got, _ := e.d.Store.GetService(ctx, svc.ID); strVal(got.Domain) != "app.example.test, www.example.test" {
		t.Fatalf("domain changed by an empty body: %q", strVal(got.Domain))
	}

	rec = e.do(t, "POST", base+"/ssl/retry", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Manual SSL retry requested.") {
		t.Fatalf("retry: %d %s", rec.Code, rec.Body.String())
	}

	// nginx failure rolls the domain back.
	e.edge.err = errTest
	expectError(t, e.do(t, "PUT", base, `{"domain":"new.example.test"}`), 500, "Failed to update service")
	if got, _ := e.d.Store.GetService(ctx, svc.ID); strVal(got.Domain) != "app.example.test, www.example.test" {
		t.Fatalf("domain not rolled back: %q", strVal(got.Domain))
	}
	e.edge.err = nil

	rec = e.do(t, "PUT", base, `{"domain":null}`)
	if rec.Code != 200 || rec.Body.String() != `{"success":true,"ssl":{},"events":[]}` {
		t.Fatalf("clear: %d %s", rec.Code, rec.Body.String())
	}

	rec = e.do(t, "POST", base+"/dns-check", `{"domains":["not a domain","a,b.test",7,""]}`)
	if rec.Code != 200 || !bytes.HasPrefix(rec.Body.Bytes(), []byte(`{"checks":[],"serverIp":`)) {
		t.Fatalf("dns-check: %d %s", rec.Code, rec.Body.String())
	}
}

var errTest = &RequestError{Status: http.StatusTeapot, Message: "nginx reload failed"}
