package nginx

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// legacyInstall writes confs exactly as the TypeScript backend left them.
func legacyInstall(t *testing.T, e *env) map[string]string {
	t.Helper()
	if err := os.MkdirAll(e.confDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"panel.example.com.conf":     readFile(t, goldenPath("panel-dashboard-http-only")),
		"a.example.com.conf":         readFile(t, goldenPath("panel-dashboard-active")),
		"x.example.com.conf":         readFile(t, goldenPath("panel-host-port-http-only")),
		"service-" + svcID + ".conf": readFile(t, goldenPath("service-active")),
		"default.conf":               "server { listen 80 default_server; }\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(e.confDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return files
}

func TestMigratePanelUpstreams(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	goldenCerts(t, e.le)
	old := legacyInstall(t, e)

	changed, err := e.nginx.MigratePanelUpstreams(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(changed, []string{"a.example.com.conf", "panel.example.com.conf"}) {
		t.Fatal(changed)
	}
	// The rewritten files equal what Go generates for the same mappings.
	for file, c := range map[string]goldenCase{"panel.example.com.conf": goldenCases[8], "a.example.com.conf": goldenCases[10]} {
		if got, want := readFile(t, filepath.Join(e.confDir, file)), renderGo(t, e, c); got != want {
			t.Fatalf("%s:\n%s\nwant\n%s", file, got, want)
		}
	}
	for _, untouched := range []string{"x.example.com.conf", "service-" + svcID + ".conf", "default.conf"} {
		if readFile(t, filepath.Join(e.confDir, untouched)) != old[untouched] {
			t.Fatalf("%s changed", untouched)
		}
	}
	if got := e.docker.Calls(); !slices.Equal(got, []string{"docker exec docklift-nginx-proxy nginx -t", "docker exec docklift-nginx-proxy nginx -s reload"}) {
		t.Fatal(got)
	}
	entries, _ := os.ReadDir(e.confDir)
	if len(entries) != len(old) {
		t.Fatal("temp files left behind")
	}
	// Listing still sees both panel domains with their ports.
	list, _ := e.nginx.ListPanelDomains()
	if !slices.Equal(list, []PanelDomain{{"a.example.com", 8080}, {"panel.example.com", 8080}, {"x.example.com", 3000}}) {
		t.Fatal(list)
	}

	// Idempotent: nothing to do, no nginx calls.
	changed, err = e.nginx.MigratePanelUpstreams(ctx)
	if err != nil || len(changed) != 0 || len(e.docker.Calls()) != 2 {
		t.Fatal(changed, err, e.docker.Calls())
	}

	// A changed PORT is followed.
	e.nginx.backendPort = 9000
	changed, _ = e.nginx.MigratePanelUpstreams(ctx)
	if len(changed) != 2 || !strings.Contains(readFile(t, filepath.Join(e.confDir, "a.example.com.conf")), "proxy_pass http://$dashboard:9000;") {
		t.Fatal(changed)
	}
}

func TestMigratePanelUpstreamsRollsBackRejectedConfig(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	goldenCerts(t, e.le)
	old := legacyInstall(t, e)
	e.docker.test = invalidConf(`host not found in upstream "docklift-backend"`)
	changed, err := e.nginx.MigratePanelUpstreams(ctx)
	if err == nil || !IsReloadError(err) || changed != nil {
		t.Fatal(changed, err)
	}
	for name, content := range old {
		if readFile(t, filepath.Join(e.confDir, name)) != content {
			t.Fatalf("%s not restored", name)
		}
	}
	if e.docker.count("reload") != 0 {
		t.Fatal("reloaded a rejected config")
	}
	// Next boot retries.
	e.docker.test = nil
	if changed, err := e.nginx.MigratePanelUpstreams(ctx); err != nil || len(changed) != 2 {
		t.Fatal(changed, err)
	}
}

func TestMigratePanelUpstreamsWithoutDocker(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	goldenCerts(t, e.le)
	legacyInstall(t, e)
	e.docker.test = noDocker
	e.docker.reload = noDocker
	changed, err := e.nginx.MigratePanelUpstreams(ctx)
	if len(changed) != 2 || !IsReloadError(err) {
		t.Fatal(changed, err)
	}
	// The new upstream is kept: it is the only correct one in the Go layout.
	if !strings.Contains(readFile(t, filepath.Join(e.confDir, "panel.example.com.conf")), "set $dashboard docklift-backend;") {
		t.Fatal("rewrite lost")
	}
}

func TestRewritePanelUpstream(t *testing.T) {
	for in, want := range map[string]string{
		"set $dashboard docklift-nginx;\nproxy_pass http://$dashboard:80;":        "set $dashboard docklift-backend;\nproxy_pass http://$dashboard:8000;",
		"set  $dashboard   docklift-nginx ;\nproxy_pass   http://$dashboard:80 ;": "set $dashboard docklift-backend;\nproxy_pass http://$dashboard:8000;",
		"set $dashboard docklift-backend;\nproxy_pass http://$dashboard:8000;":    "set $dashboard docklift-backend;\nproxy_pass http://$dashboard:8000;",
		"proxy_pass http://host.docker.internal:8080;":                            "proxy_pass http://host.docker.internal:8080;",
		"set $dashboard docklift-nginx-other;\nproxy_pass http://$dashboard:80;":  "set $dashboard docklift-nginx-other;\nproxy_pass http://$dashboard:80;",
	} {
		if got := rewritePanelUpstream(in, 8000); got != want {
			t.Errorf("%q -> %q", in, got)
		}
	}
}

func TestMigrateNothingToDo(t *testing.T) {
	e := newEnv(t)
	if changed, err := e.nginx.MigratePanelUpstreams(context.Background()); err != nil || changed != nil {
		t.Fatal(changed, err)
	}
	if len(e.docker.Calls()) != 0 {
		t.Fatal("nginx called with nothing to migrate")
	}
}
