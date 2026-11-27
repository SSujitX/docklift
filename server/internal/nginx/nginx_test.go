package nginx

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"docklift/internal/certs"
	"docklift/internal/store"
)

func TestTemplatesRejectInjection(t *testing.T) {
	hostile := []string{
		"a.test;", "a.test\nserver_name evil.test", "a.test }", "a.test{", "a.test$x", `a.test"`, "a.test'",
		"a.test\r", "a..test", "", "localhost", "-a.test", "a.test\tb.test", "a.test  b.test", " a.test", "a.test ",
	}
	loc, err := BuildServiceProxyLocation(svcID, "web", 80)
	if err != nil {
		t.Fatal(err)
	}
	idx := newEnv(t).certs
	for _, h := range hostile {
		if _, err := BuildHTTPHTTPSServers(idx, ServerBlocks{ServerNames: h, PrimaryDomain: "a.test", ProxyLocation: loc}); err == nil {
			t.Errorf("server_name %q accepted", h)
		}
		if _, err := BuildHTTPHTTPSServers(idx, ServerBlocks{ServerNames: "a.test", PrimaryDomain: h, ProxyLocation: loc}); err == nil {
			t.Errorf("primary %q accepted", h)
		}
		if _, err := BuildPanelConf(idx, h, 8080, 8000, false); err == nil {
			t.Errorf("panel domain %q accepted", h)
		}
	}
	for _, bad := range []struct {
		id, container string
		port          int
	}{
		{"x;y", "web", 80}, {"../../etc", "web", 80}, {"", "web", 80}, {svcID, "web;", 80}, {svcID, "web\n", 80},
		{svcID, "$host", 80}, {svcID, "-web", 80}, {svcID, "web", 0}, {svcID, "web", 65536}, {svcID, "web", -1},
	} {
		if _, err := BuildServiceProxyLocation(bad.id, bad.container, bad.port); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
	for _, p := range [][2]int{{0, 8000}, {65536, 8000}, {8080, 0}} {
		if _, err := BuildPanelProxyLocation(p[0], p[1]); err == nil {
			t.Errorf("ports %v accepted", p)
		}
	}
	out, err := BuildHTTPHTTPSServers(idx, ServerBlocks{Comment: "Main server block for x\n}\nserver { listen 81;\r# ", ServerNames: "a.test", PrimaryDomain: "a.test", ProxyLocation: loc})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "\n# Main server block for x } server { listen 81; # \n# docklift-ssl:") {
		t.Fatalf("comment escaped its line:\n%s", out)
	}
	var inv *InvalidInputError
	_, err = BuildPanelProxyLocation(0, 1)
	if !errors.As(err, &inv) || inv.Field != "panel port" {
		t.Fatal(err)
	}
}

func TestReload(t *testing.T) {
	ctx := context.Background()
	t.Run("ok", func(t *testing.T) {
		e := newEnv(t)
		if err := e.nginx.Reload(ctx); err != nil {
			t.Fatal(err)
		}
		if got := e.docker.Calls(); !slices.Equal(got, []string{"docker exec docklift-nginx-proxy nginx -s reload"}) {
			t.Fatal(got)
		}
	})
	t.Run("self-heal removes the named service conf then retries", func(t *testing.T) {
		e := newEnv(t)
		_ = os.MkdirAll(e.confDir, 0o755)
		bad := filepath.Join(e.confDir, "service-dead-1.conf")
		_ = os.WriteFile(bad, []byte("x"), 0o644)
		n := 0
		e.docker.reload = func() (certs.ExecResult, error) {
			n++
			if n == 1 {
				return certs.ExecResult{ExitCode: 1, Stderr: `nginx: [emerg] host not found in upstream "dead" in /etc/nginx/conf.d/service-dead-1.conf:5`}, nil
			}
			return certs.ExecResult{}, nil
		}
		if err := e.nginx.Reload(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(bad); !os.IsNotExist(err) {
			t.Fatal("bad conf kept")
		}
	})
	t.Run("failure details", func(t *testing.T) {
		for name, tc := range map[string]struct {
			fn   func() (certs.ExecResult, error)
			want string
		}{
			"stderr":    {func() (certs.ExecResult, error) { return certs.ExecResult{ExitCode: 1, Stderr: "  boom \n"}, nil }, "Nginx reload failed: boom"},
			"exit code": {func() (certs.ExecResult, error) { return certs.ExecResult{ExitCode: 2}, nil }, "Nginx reload failed: exit code 2"},
			"spawn": {func() (certs.ExecResult, error) {
				return certs.ExecResult{ExitCode: -1}, errors.New("spawn docker ENOENT")
			}, "Nginx reload failed: spawn docker ENOENT"},
		} {
			t.Run(name, func(t *testing.T) {
				e := newEnv(t)
				e.docker.reload = tc.fn
				err := e.nginx.Reload(ctx)
				if err == nil || err.Error() != tc.want || !IsReloadError(err) {
					t.Fatal(err)
				}
				if e.docker.count("reload") != 2 {
					t.Fatal("no retry")
				}
			})
		}
	})
}

func TestApplyConf(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	_ = os.MkdirAll(e.confDir, 0o755)
	p := filepath.Join(e.confDir, "x.example.com.conf")
	r := e.nginx.reloader

	if err := r.ApplyConf(ctx, p, "good"); err != nil {
		t.Fatal(err)
	}
	e.docker.test = invalidConf(`unknown directive "bogus"`)
	err := r.ApplyConf(ctx, p, "bad")
	var re *ReloadError
	if !errors.As(err, &re) || !re.RolledBack || !strings.HasPrefix(err.Error(), "Nginx reload failed: nginx: [emerg] unknown directive") {
		t.Fatal(err)
	}
	if readFile(t, p) != "good" {
		t.Fatal("previous conf not restored")
	}
	fresh := filepath.Join(e.confDir, "new.example.com.conf")
	if err := r.ApplyConf(ctx, fresh, "bad"); err == nil {
		t.Fatal("accepted")
	}
	if _, err := os.Stat(fresh); !os.IsNotExist(err) {
		t.Fatal("rejected new conf left behind")
	}
	entries, _ := os.ReadDir(e.confDir)
	if len(entries) != 1 {
		t.Fatalf("temp files left: %v", entries)
	}

	// nginx unreachable: keep the file (TS behaviour); the reload reports it.
	e.docker.test = noDocker
	if err := r.ApplyConf(ctx, p, "unverified"); err != nil || readFile(t, p) != "unverified" {
		t.Fatal(err)
	}
	e.docker.test = func() (certs.ExecResult, error) {
		return certs.ExecResult{ExitCode: 1, Stderr: "Error response from daemon: No such container: docklift-nginx-proxy"}, nil
	}
	if err := r.ApplyConf(ctx, p, "unverified2"); err != nil || readFile(t, p) != "unverified2" {
		t.Fatal(err)
	}

	// Another broken service conf is healed, then the test passes.
	other := filepath.Join(e.confDir, "service-gone.conf")
	_ = os.WriteFile(other, []byte("x"), 0o644)
	n := 0
	e.docker.test = func() (certs.ExecResult, error) {
		n++
		if n == 1 {
			return certs.ExecResult{ExitCode: 1, Stderr: "nginx: [emerg] host not found in upstream \"gone\" in /etc/nginx/conf.d/service-gone.conf:3\nnginx: configuration file /etc/nginx/nginx.conf test failed"}, nil
		}
		return certs.ExecResult{}, nil
	}
	if err := r.ApplyConf(ctx, p, "healed"); err != nil || readFile(t, p) != "healed" {
		t.Fatal(err)
	}
	if _, err := os.Stat(other); !os.IsNotExist(err) {
		t.Fatal("broken conf kept")
	}
}

func runningService(domain string) store.Service {
	return store.Service{ID: svcID, Name: "web", ContainerName: ptr("docklift-shop-web"), InternalPort: ptr(3000), Domain: ptr(domain), Status: ptr("running")}
}

// certbotWrites makes the fake certbot issue a lineage for the -d hosts.
func certbotWrites(t *testing.T, e *env) func(args []string) (certs.ExecResult, error) {
	return func(args []string) (certs.ExecResult, error) {
		var hosts []string
		name := ""
		for i := 0; i < len(args)-1; i++ {
			switch args[i] {
			case "-d":
				hosts = append(hosts, args[i+1])
			case "--cert-name":
				name = args[i+1]
			}
		}
		pemFor := map[string]string{"a.example.com": "covering-a-b.pem", "b.example.com": "orphan-b.pem"}
		if f, ok := pemFor[name]; ok && (len(hosts) <= 2) {
			writeLive(t, e.le, name, fixturePEM(t, f))
			return certs.ExecResult{}, nil
		}
		return certs.ExecResult{ExitCode: 1, Stderr: "Domain: " + hosts[0] + "\nDetail: no fixture"}, nil
	}
}

func TestUpdateServiceDomainIssuesAndEnablesHTTPS(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.docker.certbot = certbotWrites(t, e)
	svc := runningService(" A.example.com, b.example.com ")
	svc.InternalPort = ptr(8080)
	if err := e.nginx.UpdateServiceDomain(ctx, svc, UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	conf := readFile(t, filepath.Join(e.confDir, "service-"+svcID+".conf"))
	if conf != readFile(t, goldenPath("service-active")) {
		t.Fatalf("unexpected conf:\n%s", conf)
	}
	calls := e.docker.Calls()
	want := []string{
		"docker exec docklift-nginx-proxy nginx -t",
		"docker exec docklift-nginx-proxy nginx -s reload",
		"docker exec docklift-certbot certbot certonly --webroot -w /var/www/certbot --non-interactive --agree-tos --email admin@example.com --cert-name a.example.com -d a.example.com -d b.example.com",
		"docker exec docklift-nginx-proxy nginx -t",
		"docker exec docklift-nginx-proxy nginx -s reload",
	}
	if !slices.Equal(calls, want) {
		t.Fatalf("calls:\n%s", strings.Join(calls, "\n"))
	}
	var msgs []string
	for _, ev := range e.certs.GetSSLEvents([]string{"a.example.com"}) {
		msgs = append(msgs, string(ev.Level)+": "+ev.Message)
	}
	if msgs[0] != "info: HTTP vhost written and nginx reloaded — ACME challenge path is live." || msgs[len(msgs)-1] != "success: HTTPS enabled for a.example.com b.example.com." {
		t.Fatal(msgs)
	}
	// Re-running with the certificate in place reuses it and stays active.
	if err := e.nginx.UpdateServiceDomain(ctx, svc, UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if e.docker.count("certbot") != 1 {
		t.Fatal("covered hosts re-issued")
	}
}

func TestUpdateServiceDomainSoftFailsCertbot(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.docker.certbot = func([]string) (certs.ExecResult, error) {
		return certs.ExecResult{ExitCode: 1, Stderr: "Domain: app.example.com\nDetail: Timeout during connect"}, nil
	}
	if err := e.nginx.UpdateServiceDomain(ctx, runningService("app.example.com"), UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	conf := readFile(t, filepath.Join(e.confDir, "service-"+svcID+".conf"))
	if !strings.Contains(conf, "# docklift-ssl: pending|http-only primary=app.example.com") {
		t.Fatal(conf)
	}
	evs := e.certs.GetSSLEvents([]string{"app.example.com"})
	if last := evs[len(evs)-1]; last.Level != certs.LevelWarn || !strings.HasPrefix(last.Message, "Still serving plain HTTP") {
		t.Fatal(last)
	}
}

func TestUpdateServiceDomainPropagatesReloadFailure(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.docker.test = noDocker
	e.docker.reload = noDocker
	err := e.nginx.UpdateServiceDomain(ctx, runningService("app.example.com"), UpdateOptions{})
	if err == nil || !strings.HasPrefix(err.Error(), "Nginx reload failed: ") {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(e.confDir, "service-"+svcID+".conf")); err != nil {
		t.Fatal("conf should be written before the failing reload (TS behaviour)")
	}
	if e.docker.count("certbot") != 0 {
		t.Fatal("issued although the vhost is not live")
	}
	evs := e.certs.GetSSLEvents([]string{"app.example.com"})
	if evs[len(evs)-1].Message != "Failed to write the nginx vhost — check backend logs." {
		t.Fatal(evs)
	}

	// Reload failure after issuance propagates too.
	e2 := newEnv(t)
	e2.docker.certbot = certbotWrites(t, e2)
	n := 0
	e2.docker.reload = func() (certs.ExecResult, error) {
		n++
		if n > 1 {
			return certs.ExecResult{ExitCode: 1, Stderr: "boom"}, nil
		}
		return certs.ExecResult{}, nil
	}
	err = e2.nginx.UpdateServiceDomain(ctx, runningService("a.example.com,b.example.com"), UpdateOptions{})
	if !IsReloadError(err) {
		t.Fatal(err)
	}
	var msgs []string
	for _, ev := range e2.certs.GetSSLEvents([]string{"a.example.com"}) {
		msgs = append(msgs, ev.Message)
	}
	if !slices.Contains(msgs, "Certificate may exist but nginx reload failed — HTTPS vhost not live.") || msgs[len(msgs)-1] != "Nginx reload failed: boom" {
		t.Fatal(msgs)
	}
}

func TestUpdateServiceDomainRejectsHostileRows(t *testing.T) {
	ctx := context.Background()
	for name, svc := range map[string]store.Service{
		"domain injection": runningService("a.example.com;\n}\nserver { listen 80"),
		"domain spaces":    runningService("a.example.com b.example.com"),
		"container":        {ID: svcID, Name: "x", ContainerName: ptr("web;"), InternalPort: ptr(80), Domain: ptr("a.example.com"), Status: ptr("running")},
		"id traversal":     {ID: "../../x", Name: "x", ContainerName: ptr("web"), InternalPort: ptr(80), Domain: ptr("a.example.com"), Status: ptr("running")},
		"no port":          {ID: svcID, Name: "x", ContainerName: ptr("web"), Domain: ptr("a.example.com"), Status: ptr("running")},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			if err := e.nginx.UpdateServiceDomain(ctx, svc, UpdateOptions{}); err == nil {
				t.Fatal("accepted")
			}
			entries, _ := os.ReadDir(e.confDir)
			if len(entries) != 0 || len(e.docker.Calls()) != 0 {
				t.Fatalf("wrote %v / ran %v", entries, e.docker.Calls())
			}
		})
	}
}

func TestUpdateServiceDomainRemovesConf(t *testing.T) {
	ctx := context.Background()
	for name, mutate := range map[string]func(*store.Service){
		"stopped":      func(s *store.Service) { s.Status = ptr("stopped") },
		"no domain":    func(s *store.Service) { s.Domain = nil },
		"blank domain": func(s *store.Service) { s.Domain = ptr(" , ") },
		"no container": func(s *store.Service) { s.ContainerName = nil },
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			svc := runningService("app.example.com")
			if err := e.nginx.UpdateServiceDomain(ctx, svc, UpdateOptions{SkipIssue: true}); err != nil {
				t.Fatal(err)
			}
			mutate(&svc)
			before := e.docker.count("reload")
			if err := e.nginx.UpdateServiceDomain(ctx, svc, UpdateOptions{}); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(e.confDir, "service-"+svcID+".conf")); !os.IsNotExist(err) {
				t.Fatal("conf kept")
			}
			if e.docker.count("reload") != before+1 {
				t.Fatal("no reload after removal")
			}
			// Nothing to remove: no reload.
			if err := e.nginx.UpdateServiceDomain(ctx, svc, UpdateOptions{}); err != nil || e.docker.count("reload") != before+1 {
				t.Fatal(err)
			}
		})
	}
}

func TestUpdateServiceDomainCancellation(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	allowed := 2
	opts := UpdateOptions{ShouldContinue: func(context.Context) bool { allowed--; return allowed >= 0 }}
	err := e.nginx.UpdateServiceDomain(ctx, runningService("app.example.com"), opts)
	if !errors.Is(err, ErrCancelled) || err.Error() != "Deployment cancelled" {
		t.Fatal(err)
	}
	if e.docker.count("reload") != 0 {
		t.Fatal("reloaded after cancel")
	}
	if len(e.certs.GetSSLEvents([]string{"app.example.com"})) != 0 {
		t.Fatal("cancel recorded as a failure")
	}
}

type inspector struct{ running bool }

func (i inspector) ContainerRunning(context.Context, string) (bool, error) { return i.running, nil }

func TestSyncConfigs(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.nginx.containers = inspector{running: false}
	e.docker.certbot = certbotWrites(t, e)
	coveringCerts(t, e.le)
	_ = os.MkdirAll(e.confDir, 0o755)
	keep := "11111111-2222-3333-4444-555555555555"
	_ = os.WriteFile(filepath.Join(e.confDir, "service-"+keep+".conf"), []byte("kept"), 0o644)
	_ = os.WriteFile(filepath.Join(e.confDir, "service-orphan.conf"), []byte("orphan"), 0o644)
	_ = os.WriteFile(filepath.Join(e.confDir, "panel.example.com.conf"), []byte("panel"), 0o644)
	e.services.rows = []*store.Service{
		{ID: keep, Name: "kept", Domain: ptr("k.example.com"), Status: ptr("stopped")},
		{ID: svcID, Name: "web", ContainerName: ptr("docklift-shop-web"), InternalPort: ptr(3000), Domain: ptr("a.example.com, b.example.com"), Status: ptr("running")},
	}
	if err := e.nginx.SyncConfigs(ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readFile(t, filepath.Join(e.confDir, "service-"+svcID+".conf")), "# docklift-ssl: active") {
		t.Fatal("missing conf not restored with the existing certificate")
	}
	if e.docker.count("certbot") != 0 {
		t.Fatal("issued although a certificate covers the hosts")
	}
	if readFile(t, filepath.Join(e.confDir, "service-"+keep+".conf")) != "kept" || readFile(t, filepath.Join(e.confDir, "panel.example.com.conf")) != "panel" {
		t.Fatal("known confs touched")
	}
	if _, err := os.Stat(filepath.Join(e.confDir, "service-orphan.conf")); !os.IsNotExist(err) {
		t.Fatal("orphan kept")
	}
	reloads := e.docker.count("reload")
	if err := e.nginx.SyncConfigs(ctx); err != nil || e.docker.count("reload") != reloads {
		t.Fatal("in-sync run reloaded", err)
	}
	if err := New(Options{ConfDir: filepath.Join(e.dir, "absent")}, e.certs, e.services).SyncConfigs(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestCleanupServiceDomain(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	_ = e.nginx.UpdateServiceDomain(ctx, runningService("app.example.com"), UpdateOptions{SkipIssue: true})
	e.docker.reload = noDocker
	e.nginx.CleanupServiceDomain(ctx, svcID) // reload failure only logged
	if _, err := os.Stat(filepath.Join(e.confDir, "service-"+svcID+".conf")); !os.IsNotExist(err) {
		t.Fatal("conf kept")
	}
	e.nginx.CleanupServiceDomain(ctx, "../../etc/passwd")
	e.nginx.CleanupServiceDomain(ctx, svcID)
}

func TestPanelDomainsWithoutDocker(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.docker.test = noDocker
	e.docker.reload = noDocker
	_, err := e.nginx.ProvisionPanelSSL(ctx, "panel.example.com", 8443, false)
	if err == nil || !strings.HasPrefix(err.Error(), "Nginx reload failed: ") {
		t.Fatal(err)
	}
	conf := readFile(t, filepath.Join(e.confDir, "panel.example.com.conf"))
	if !strings.HasPrefix(conf, "# docklift-panel-domain\n# docklift-panel-port: 8443\n") || !strings.Contains(conf, "server_name panel.example.com;") {
		t.Fatal(conf)
	}
	list, err := e.nginx.ListPanelDomains()
	if err != nil || !slices.Equal(list, []PanelDomain{{"panel.example.com", 8443}}) {
		t.Fatal(list, err)
	}
	if ok, _ := e.nginx.PanelDomainExists("panel.example.com"); !ok {
		t.Fatal("exists")
	}
	if ok, _ := e.nginx.PanelDomainExists("other.example.com"); ok {
		t.Fatal("exists")
	}
	if _, err := e.nginx.PanelDomainExists("../x"); err == nil {
		t.Fatal("traversal")
	}
	if _, err := e.nginx.RetryPanelSSL(ctx, "missing.example.com"); !errors.Is(err, ErrPanelDomainNotFound) {
		t.Fatal(err)
	}
	if _, err := e.nginx.RetryPanelSSL(ctx, "panel.example.com"); err == nil || !strings.HasPrefix(err.Error(), "Nginx reload failed: ") {
		t.Fatal(err)
	}
	if err := e.nginx.DeletePanelDomain(ctx, "panel.example.com"); !IsReloadError(err) {
		t.Fatal(err)
	}
	if err := e.nginx.DeletePanelDomain(ctx, "panel.example.com"); !errors.Is(err, ErrPanelDomainNotFound) || err.Error() != "Domain configuration not found" {
		t.Fatal(err)
	}
}

func TestProvisionPanelSSLEnablesHTTPS(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.docker.certbot = certbotWrites(t, e)
	st, err := e.nginx.ProvisionPanelSSL(ctx, "a.example.com", 8080, false)
	if err != nil || st.Status != certs.StatusActive {
		t.Fatal(st, err)
	}
	ts := readFile(t, goldenPath("panel-dashboard-active"))
	if got := readFile(t, filepath.Join(e.confDir, "a.example.com.conf")); got != rewritePanelUpstream(ts, 8000) {
		t.Fatalf("got:\n%s", got)
	}
	_ = e.settings.SetSetting(ctx, "ssl_meta_a.example.com", `{"status":"failed","error":"x","updatedAt":"x"}`)
	if _, err := e.nginx.RetryPanelSSL(ctx, "a.example.com"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.docker.Calls()[len(e.docker.Calls())-3], "--force-renewal") {
		t.Fatal("retry not forced")
	}
}

func TestParsePanelPort(t *testing.T) {
	for content, want := range map[string]int{
		"set $dashboard docklift-nginx;":                                8080,
		"# docklift-panel-port: 9000\nset $dashboard docklift-backend;": 9000,
		"proxy_pass http://$dashboard:8000;":                            8080,
		"proxy_pass http://host.docker.internal:3001;":                  3001,
		"# docklift-panel-port: 3001\nproxy_pass http://elsewhere:1;":   0,
	} {
		got, ok := ParsePanelPort(content)
		if got != want || ok != (want != 0) {
			t.Errorf("%q: %d %t", content, got, ok)
		}
	}
}

func TestAssertHostnamesAvailable(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.services.rows = []*store.Service{
		{ID: "s1", Domain: ptr("Shop.example.com, www.shop.example.com")},
		{ID: "s2", Domain: nil},
	}
	_ = os.MkdirAll(e.confDir, 0o755)
	_ = os.WriteFile(filepath.Join(e.confDir, "panel.example.com.conf"), []byte(PanelConfMarker+"\nserver_name panel.example.com alt.example.com;"), 0o644)
	_ = os.WriteFile(filepath.Join(e.confDir, "legacy.example.com.conf"), []byte("proxy_pass http://host.docker.internal:8080;"), 0o644)
	_ = os.WriteFile(filepath.Join(e.confDir, "service-x.conf"), []byte(PanelConfMarker+"\nserver_name svc.example.com;"), 0o644)
	_ = os.WriteFile(filepath.Join(e.confDir, "renamed.conf"), []byte("# Main server block for x\nservice-y docklift-nginx\nserver_name moved.example.com;"), 0o644)
	_ = os.WriteFile(filepath.Join(e.confDir, "default.conf"), []byte(PanelConfMarker+"\nserver_name _;"), 0o644)

	var ce *ConflictError
	err := e.nginx.AssertHostnamesAvailable(ctx, []string{"new.example.com", "shop.example.com"}, "")
	if !errors.As(err, &ce) || err.Error() != `Hostname "shop.example.com" is already assigned to another service` {
		t.Fatal(err)
	}
	if err := e.nginx.AssertHostnamesAvailable(ctx, []string{"shop.example.com"}, "s1"); err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"alt.example.com", "legacy.example.com"} {
		if err := e.nginx.AssertHostnamesAvailable(ctx, []string{h}, ""); err == nil || err.Error() != `Hostname "`+h+`" is already used by a panel domain` {
			t.Fatal(h, err)
		}
	}
	for _, h := range []string{"svc.example.com", "moved.example.com", "_", "free.example.com"} {
		if err := e.nginx.AssertHostnamesAvailable(ctx, []string{h}, ""); err != nil {
			t.Fatal(h, err)
		}
	}
	if err := e.nginx.AssertHostnamesAvailable(ctx, nil, ""); err != nil {
		t.Fatal(err)
	}
}

func TestDomainFieldHelpers(t *testing.T) {
	if got := NormalizeDomainString(" A.example.com ,b.example.com,, a.example.com"); !slices.Equal(got, []string{"a.example.com", "b.example.com"}) {
		t.Fatal(got)
	}
	if got := NormalizeDomainString("X.test,x.test, "); !slices.Equal(got, []string{"x.test"}) {
		t.Fatal(got)
	}
	if FormatDomainField(nil) != nil || *FormatDomainField([]string{"a.test", "b.test"}) != "a.test, b.test" {
		t.Fatal("format")
	}
	if got := DomainList(ptr("A.test, a.test,,b.test ")); !slices.Equal(got, []string{"a.test", "a.test", "b.test"}) {
		t.Fatal(got)
	}
	if DomainList(nil) != nil {
		t.Fatal("nil")
	}
	for s, want := range map[string]bool{
		"": true, "a.example.com": true, "a.example.com, B.example.com": true, " , a.test ,": true,
		"a.test;b": false, "a.test b.test": false, "a.test,localhost": false, "a.test,\nb.test": true, "a.test\nx": false,
	} {
		if ValidDomainList(s) != want {
			t.Errorf("%q: want %t", s, want)
		}
	}
}

func TestStackBootAndWatcher(t *testing.T) {
	e := newEnv(t)
	coveringCerts(t, e.le)
	s := &Stack{Certs: e.certs, Nginx: e.nginx}
	if err := s.Boot(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := s.StartWatchers(ctx)
	waitFor(t, func() bool { return e.docker.count("reload") >= 1 })
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("watcher did not stop")
	}
}
