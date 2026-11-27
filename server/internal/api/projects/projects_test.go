package projects

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"docklift/internal/store"
)

const missingID = "5eed0000-0000-4000-8000-00000000beef"

func TestListAndGetSyncStatus(t *testing.T) {
	f := newFixture(t, Options{})
	ctx := context.Background()
	older := f.project(store.ProjectCreate{Name: "older", Status: store.Ptr("pending")})
	f.service(older.ID, "app", store.Ptr("c-up"))
	time.Sleep(3 * time.Millisecond)
	newer := f.project(store.ProjectCreate{Name: "newer", Status: store.Ptr("building")})
	f.service(newer.ID, "app", store.Ptr("c-building"))
	time.Sleep(3 * time.Millisecond)
	bare := f.project(store.ProjectCreate{Name: "bare", Status: store.Ptr("pending")})
	f.engine.running["c-up"] = true
	f.engine.running["c-building"] = true

	list := f.do("GET", "/", nil)
	if list.code != 200 {
		t.Fatalf("status %d", list.code)
	}
	rows := list.arr(t)
	if len(rows) != 3 || rows[0].(map[string]any)["id"] != bare.ID || rows[2].(map[string]any)["id"] != older.ID {
		t.Fatalf("want newest first: %v", rows)
	}
	status := map[string]any{}
	for _, r := range rows {
		m := r.(map[string]any)
		status[m["name"].(string)] = m["status"]
	}
	// Synced from Docker; "building" is never overwritten; no container, no sync.
	if status["older"] != "running" || status["newer"] != "building" || status["bare"] != "pending" {
		t.Fatalf("statuses %v", status)
	}
	if svc, _ := f.d.Store.FindService(ctx, store.ServiceFilter{ProjectID: &older.ID}, store.OrderNone); *svc.Status != "running" {
		t.Fatalf("service status %v", *svc.Status)
	}

	f.engine.running["c-up"] = false
	got := f.do("GET", "/"+older.ID, nil).expect(t, 200)
	if got["status"] != "stopped" || got["id"] != older.ID {
		t.Fatalf("get %v", got)
	}
	f.do("GET", "/"+missingID, nil).expectError(t, 404, msgProjectNotFound)
	f.do("GET", "/not-a-uuid", nil).expectError(t, 404, msgProjectNotFound)
}

func TestGetFillsGithubBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	f := newFixture(t, Options{})
	p := f.project(store.ProjectCreate{SourceType: store.Ptr("github"), GithubURL: store.Ptr("https://github.com/a/b")})
	dir := f.d.Deploy.ProjectPath(p.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "feature-x"}, {"-c", "user.email=a@b.c", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "init"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("git %v: %v %s", args, err, out)
		}
	}
	if got := f.do("GET", "/"+p.ID, nil).expect(t, 200); got["github_branch"] != "feature-x" {
		t.Fatalf("branch %v", got["github_branch"])
	}
	stored, _ := f.d.Store.GetProject(context.Background(), p.ID)
	if stored.GithubBranch == nil || *stored.GithubBranch != "feature-x" {
		t.Fatal("branch must be stored")
	}
}

func TestUpdate(t *testing.T) {
	f := newFixture(t, Options{})
	p := f.createUpload("orig")
	id := p["id"].(string)
	got := f.do("PATCH", "/"+id, map[string]any{
		"name": "renamed", "description": "desc", "domain": "patched.example.dev", "build_type": "railpack",
		"base_directory": "  src  ", "dockerfile_path": "  docker/Dockerfile ", "internal_port": 99999, "publish_host_port": "yes",
		"project_type": "worker", "github_url": "not validated on patch",
		// Not editable: ignored.
		"status": "running", "webhook_secret": "x", "auto_deploy": true, "source_type": "github", "db_engine": "postgres",
	}).expect(t, 200)
	want := map[string]any{"name": "renamed", "description": "desc", "domain": "patched.example.dev", "build_type": "railpack",
		"base_directory": "src", "dockerfile_path": "docker/Dockerfile", "internal_port": float64(65535), "publish_host_port": true,
		"project_type": "worker", "github_url": "not validated on patch", "status": "pending", "webhook_secret": nil,
		"auto_deploy": false, "source_type": "upload", "db_engine": nil, "created_at": p["created_at"]}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %#v, want %#v", k, got[k], v)
		}
	}
	got = f.do("PATCH", "/"+id, map[string]any{
		"name": "", "project_type": "", "description": nil, "domain": nil, "dockerfile_path": "", "base_directory": "",
		"internal_port": "0", "publish_host_port": 0, "build_type": "weird",
	}).expect(t, 200)
	want = map[string]any{"name": "renamed", "project_type": "worker", "description": nil, "domain": nil, "dockerfile_path": nil,
		"base_directory": ".", "internal_port": float64(3000), "publish_host_port": false, "build_type": "auto"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %#v, want %#v", k, got[k], v)
		}
	}
	f.do("PATCH", "/"+id, map[string]any{"domain": "bad_domain"}).expectError(t, 400, msgDomainFormat)
	f.do("PATCH", "/"+id, map[string]any{"domain": 5}).expectError(t, 400, msgDomainFormat)
	f.do("PATCH", "/"+id, map[string]any{"domain": false}).expectError(t, 400, "domain must be a string")
	f.do("PATCH", "/"+id, map[string]any{"name": 5}).expectError(t, 400, "name must be a string")
	f.do("PATCH", "/"+id, map[string]any{"description": []any{}}).expectError(t, 400, "description must be a string")
	f.do("PATCH", "/"+id, map[string]any{"github_url": 1}).expectError(t, 400, "github_url must be a string")
	f.do("PATCH", "/"+missingID, map[string]any{"name": "x"}).expectError(t, 404, msgProjectNotFound)
	if got := f.do("PATCH", "/"+id, nil).expect(t, 200); got["name"] != "renamed" {
		t.Fatalf("empty patch %v", got)
	}
}

func TestAutoDeploy(t *testing.T) {
	f := newFixture(t, Options{})
	up := f.createUpload("upload")
	upID := up["id"].(string)
	f.do("GET", "/"+upID+"/auto-deploy", nil).expect(t, 200)
	f.do("PATCH", "/"+upID+"/auto-deploy", map[string]any{"enabled": true}).expectError(t, 400, "Auto-deploy is only available for GitHub projects")
	f.do("GET", "/"+missingID+"/auto-deploy", nil).expectError(t, 404, msgProjectNotFound)
	f.do("PATCH", "/"+missingID+"/auto-deploy", map[string]any{"enabled": "yes"}).expectError(t, 404, msgProjectNotFound)

	gh := f.project(store.ProjectCreate{SourceType: store.Ptr("github")})
	got := f.do("GET", "/"+gh.ID+"/auto-deploy", nil).expect(t, 200)
	if got["auto_deploy"] != false || got["webhook_secret"] != nil || got["webhook_url"] != nil || got["available"] != true {
		t.Fatalf("status %v", got)
	}
	f.do("PATCH", "/"+gh.ID+"/auto-deploy", map[string]any{"enabled": "yes"}).expectError(t, 400, "enabled must be a boolean")
	on := f.do("PATCH", "/"+gh.ID+"/auto-deploy", map[string]any{"enabled": true}).expect(t, 200)
	secret, _ := on["webhook_secret"].(string)
	if on["auto_deploy"] != true || len(secret) != 64 || on["webhook_url"] != webhookURL {
		t.Fatalf("enable %v", on)
	}
	if got := f.do("GET", "/"+gh.ID+"/auto-deploy", nil).expect(t, 200); got["webhook_secret"] != secret || got["webhook_url"] != webhookURL {
		t.Fatalf("status %v", got)
	}
	off := f.do("PATCH", "/"+gh.ID+"/auto-deploy", map[string]any{"enabled": false}).expect(t, 200)
	if off["auto_deploy"] != false || off["webhook_secret"] != secret || off["webhook_url"] != nil {
		t.Fatalf("disable keeps the secret: %v", off)
	}
	// The secret is hidden while auto-deploy is off and reused on re-enable.
	if got := f.do("GET", "/"+gh.ID+"/auto-deploy", nil).expect(t, 200); got["webhook_secret"] != nil {
		t.Fatalf("secret revealed: %v", got)
	}
	if again := f.do("PATCH", "/"+gh.ID+"/auto-deploy", map[string]any{"enabled": true}).expect(t, 200); again["webhook_secret"] != secret {
		t.Fatal("secret must be reused")
	}
}

func TestBuildDetect(t *testing.T) {
	f := newFixture(t, Options{})
	p := f.createUpload("detect")
	got := f.do("GET", "/"+p["id"].(string)+"/build/detect", nil).expect(t, 200)
	if got["resolvedType"] != "dockerfile" || got["dockerfilePath"] != "Dockerfile" || got["requestedType"] != "auto" {
		t.Fatalf("detect %v", got)
	}
	if _, ok := got["services"]; ok {
		t.Fatal("services must not be returned")
	}
	esc := f.createUpload("escape", "base_directory", "../outside")
	f.do("GET", "/"+esc["id"].(string)+"/build/detect", nil).expectError(t, 400, "Build paths must stay inside the project directory")
	f.do("GET", "/"+missingID+"/build/detect", nil).expectError(t, 404, msgProjectNotFound)
	nofiles := f.project(store.ProjectCreate{})
	f.do("GET", "/"+nofiles.ID+"/build/detect", nil).expectError(t, 400, "Project files not found")
}
