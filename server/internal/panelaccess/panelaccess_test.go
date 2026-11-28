package panelaccess_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"docklift/internal/app"
	"docklift/internal/app/apptest"
	"docklift/internal/config"
	"docklift/internal/httpx"
	"docklift/internal/panelaccess"
)

const domain = "panel.example.com"

// httpsDashboardConf is the relevant part of a panel conf with an active
// certificate (BuildPanelConf output for port 8080).
const httpsDashboardConf = `# docklift-panel-domain
# docklift-panel-port: 8080
server {
    listen 80;
    server_name panel.example.com;
}

server {
    listen 443 ssl;
    listen [::]:443 ssl;
    server_name panel.example.com;

    location / {
        set $dashboard docklift-backend.docklift_network;
        proxy_pass http://$dashboard:8000;
    }
}
`

// httpOnlyDashboardConf is the same domain without a certificate.
const httpOnlyDashboardConf = `# docklift-panel-domain
# docklift-panel-port: 8080
server {
    listen 80;
    server_name panel.example.com;

    location / {
        set $dashboard docklift-backend.docklift_network;
        proxy_pass http://$dashboard:8000;
    }
}
`

type env struct {
	d *app.Deps
	g *panelaccess.Guard
}

func newEnv(t *testing.T) *env {
	t.Helper()
	d := apptest.NewDeps(t)
	if err := os.MkdirAll(d.Config.NginxConfPath, 0o755); err != nil {
		t.Fatal(err)
	}
	return &env{d: d, g: d.PanelAccess}
}

func (e *env) writeConf(t *testing.T, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(e.d.Config.NginxConfPath, domain+".conf"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (e *env) removeConf(t *testing.T) {
	t.Helper()
	if err := os.Remove(filepath.Join(e.d.Config.NginxConfPath, domain+".conf")); err != nil {
		t.Fatal(err)
	}
}

// row is the stored setting ("" when there is none).
func (e *env) row(t *testing.T) string {
	t.Helper()
	v, _, err := e.d.Store.GetSetting(context.Background(), panelaccess.SettingKey)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// handler is the root middleware order of the router: peer trust (the
// default TRUSTED_PROXIES), then the guard, then a stand-in for every route.
func (e *env) handler(t *testing.T) http.Handler {
	t.Helper()
	pt, err := config.ParseTrustedProxies(config.DefaultTrustedProxies)
	if err != nil {
		t.Fatal(err)
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	return httpx.NewTrustedProxies(pt.Trusted, pt.Excluded).Middleware(e.g.Middleware(next))
}

type request struct {
	name, method, path string
	headers            map[string]string
}

var (
	navigation = request{"html navigation", "GET", "/projects/x?tab=logs", map[string]string{"Accept": "text/html,application/xhtml+xml,*/*;q=0.8"}}
	headNav    = request{"html HEAD", "HEAD", "/", map[string]string{"Accept": "text/html"}}
	apiCall    = request{"api", "GET", "/api/projects", map[string]string{"Accept": "application/json"}}
	apiPost    = request{"api post", "POST", "/api/auth/login", map[string]string{"Content-Type": "application/json"}}
	websocket  = request{"terminal websocket", "GET", "/ws/terminal?token=x", map[string]string{
		"Upgrade": "websocket", "Connection": "Upgrade", "Sec-WebSocket-Version": "13", "Sec-WebSocket-Key": "dGhlIHNhbXBsZSBub25jZQ==", "Accept": "text/html"}}
	sse      = request{"sse", "GET", "/api/system/logs/backend?token=x", map[string]string{"Accept": "text/event-stream"}}
	asset    = request{"asset", "GET", "/assets/index.js", map[string]string{"Accept": "*/*"}}
	health   = request{"health", "GET", "/api/health", nil}
	healthHd = request{"health HEAD", "HEAD", "/api/health/", nil}
	healthPo = request{"health POST", "POST", "/api/health", nil}
	webhook  = request{"webhook", "POST", "/api/github/webhook", map[string]string{"X-GitHub-Event": "push"}}
	forged   = request{"forged forwarded headers", "GET", "/api/projects", map[string]string{
		"X-Forwarded-For": "172.28.255.254", "X-Forwarded-Proto": "https", "X-Forwarded-Host": domain, "X-Real-IP": "172.28.255.254"}}
	forgedNav = request{"forged headers, navigation", "GET", "/", map[string]string{
		"Accept": "text/html", "X-Forwarded-For": "127.0.0.1", "X-Forwarded-Proto": "https", "X-Forwarded-Host": domain}}
)

func serve(h http.Handler, peer string, rq request) *httptest.ResponseRecorder {
	r := httptest.NewRequest(rq.method, rq.path, nil)
	r.RemoteAddr = peer
	for k, v := range rq.headers {
		r.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

const refused = `{"error":"Dashboard access by IP is disabled. Use https://panel.example.com"}`

// TestMiddlewareDecisions is the decision table of the guard.
func TestMiddlewareDecisions(t *testing.T) {
	e := newEnv(t)
	h := e.handler(t)
	proxyPeers := []string{
		"172.28.255.254:41000",              // docklift-nginx-proxy, IPv4
		"[fd12:3456:789a::ffff:fffe]:41000", // docklift-nginx-proxy, IPv6
		"[::ffff:172.28.255.254]:41000",     // IPv4-mapped
		"127.0.0.1:41000",                   // in-container (Docker healthcheck, docker exec)
		"[::1]:41000",                       // in-container, IPv6
	}
	directPeers := []string{
		"172.28.0.1:41000",              // docklift_network gateway: clients of the published :8080
		"[fd12:3456:789a::1]:41000",     // gateway, IPv6
		"172.28.0.7:41000",              // an app container on docklift_network
		"10.20.0.3:41000",               // an app container on a project network
		"203.0.113.50:41000",            // a client without NAT (host networking setups)
		"[2001:db8::50]:41000",          // the same over IPv6
		"[fd12:3456:789a::ffff:fffd]:1", // the proxy's neighbour address
	}
	all := []request{navigation, headNav, apiCall, apiPost, websocket, sse, asset, health, healthHd, healthPo, webhook, forged, forgedNav}

	// Off: nothing is touched, whoever asks.
	for _, peer := range append(append([]string{}, proxyPeers...), directPeers...) {
		for _, rq := range all {
			if rec := serve(h, peer, rq); rec.Code != http.StatusTeapot {
				t.Errorf("off, %s from %s = %d, want the route", rq.name, peer, rec.Code)
			}
		}
	}

	e.writeConf(t, httpsDashboardConf)
	if err := e.g.Enable(context.Background(), domain); err != nil {
		t.Fatal(err)
	}
	for _, peer := range proxyPeers {
		for _, rq := range all {
			if rec := serve(h, peer, rq); rec.Code != http.StatusTeapot {
				t.Errorf("on, %s through the proxy %s = %d, want the route", rq.name, peer, rec.Code)
			}
		}
	}
	for _, peer := range directPeers {
		for _, rq := range []request{apiCall, apiPost, websocket, sse, asset, healthPo, forged} {
			rec := serve(h, peer, rq)
			if rec.Code != http.StatusForbidden || rec.Body.String() != refused || rec.Header().Get("Cache-Control") != "no-store" {
				t.Errorf("on, %s from %s = %d %q %v, want the fixed 403", rq.name, peer, rec.Code, rec.Body, rec.Header())
			}
		}
		for _, rq := range []request{navigation, headNav, forgedNav} {
			rec := serve(h, peer, rq)
			want := "https://" + domain + rq.path
			if rec.Code != http.StatusFound || rec.Header().Get("Location") != want || rec.Header().Get("Cache-Control") != "no-store" {
				t.Errorf("on, %s from %s = %d Location %q, want 302 %s", rq.name, peer, rec.Code, rec.Header().Get("Location"), want)
			}
		}
		for _, rq := range []request{health, healthHd} {
			rec := serve(h, peer, rq)
			if rec.Code != http.StatusOK || (rq.method == "GET" && rec.Body.String() != `{"status":"ok"}`) {
				t.Errorf("on, %s from %s = %d %q, want 200 status only", rq.name, peer, rec.Code, rec.Body)
			}
		}
		if rec := serve(h, peer, webhook); rec.Code != http.StatusTeapot {
			t.Errorf("on, webhook from %s = %d, want the route (HMAC-checked there)", peer, rec.Code)
		}
	}

	// Turning it off applies to the very next request.
	if _, err := e.g.Disable(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rec := serve(h, directPeers[0], apiCall); rec.Code != http.StatusTeapot {
		t.Errorf("after Disable = %d", rec.Code)
	}
}

// TestNoTrustedProxyRefusesEveryone documents why the CLI refuses to close
// with TRUSTED_PROXIES=none: no request can be told to come from the proxy.
func TestNoTrustedProxyRefusesEveryone(t *testing.T) {
	e := newEnv(t)
	e.writeConf(t, httpsDashboardConf)
	if err := e.g.Enable(context.Background(), domain); err != nil {
		t.Fatal(err)
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	h := httpx.NewTrustedProxies(nil, nil).Middleware(e.g.Middleware(next))
	if rec := serve(h, "172.28.255.254:1", apiCall); rec.Code != http.StatusForbidden {
		t.Errorf("untrusted proxy = %d", rec.Code)
	}
}

// TestCache: writes through the Guard update the cache at once, and the
// middleware reads only the cache (no database access per request).
func TestCache(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.writeConf(t, httpsDashboardConf)
	if err := e.g.Enable(ctx, domain); err != nil {
		t.Fatal(err)
	}
	if e.g.Domain() != domain || e.row(t) != domain {
		t.Fatalf("after Enable: cache %q row %q", e.g.Domain(), e.row(t))
	}
	// A row written behind the Guard's back is not seen until a refresh.
	if err := e.d.Store.DeleteSetting(ctx, panelaccess.SettingKey); err != nil {
		t.Fatal(err)
	}
	if rec := serve(e.handler(t), "172.28.0.1:1", apiCall); rec.Code != http.StatusForbidden {
		t.Errorf("middleware must use the cache: %d", rec.Code)
	}
	if err := e.g.Load(ctx); err != nil || e.g.Domain() != "" {
		t.Fatalf("Load = %v, cache %q", err, e.g.Domain())
	}
	if err := e.g.Enable(ctx, domain); err != nil {
		t.Fatal(err)
	}
	was, err := e.g.Disable(ctx)
	if err != nil || was != domain || e.g.Domain() != "" || e.row(t) != "" {
		t.Fatalf("Disable = %q %v, cache %q row %q", was, err, e.g.Domain(), e.row(t))
	}
	if was, err := e.g.Disable(ctx); err != nil || was != "" {
		t.Errorf("second Disable = %q %v", was, err)
	}
}

// TestBootTurnsOffWithoutDomain: a setting left on without a working panel
// domain is turned off before the server listens.
func TestBootTurnsOffWithoutDomain(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name, conf string
		keep       bool
	}{
		{"https panel domain", httpsDashboardConf, true},
		{"no conf", "", false},
		{"conf without certificate", httpOnlyDashboardConf, false},
		{"conf for another port", "# docklift-panel-domain\n# docklift-panel-port: 3000\nserver {\n    listen 443 ssl;\n    server_name panel.example.com;\n    location / { proxy_pass http://host.docker.internal:3000; }\n}\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			if tc.conf != "" {
				e.writeConf(t, tc.conf)
			}
			if err := e.d.Store.SetSetting(ctx, panelaccess.SettingKey, domain); err != nil {
				t.Fatal(err)
			}
			if err := e.g.Boot(ctx); err != nil {
				t.Fatal(err)
			}
			want := ""
			if tc.keep {
				want = domain
			}
			if e.g.Domain() != want || e.row(t) != want {
				t.Errorf("after Boot: cache %q row %q, want %q", e.g.Domain(), e.row(t), want)
			}
		})
	}
	t.Run("invalid stored domain", func(t *testing.T) {
		e := newEnv(t)
		if err := e.d.Store.SetSetting(ctx, panelaccess.SettingKey, "not a host/../x"); err != nil {
			t.Fatal(err)
		}
		if err := e.g.Boot(ctx); err != nil || e.g.Domain() != "" || e.row(t) != "" {
			t.Errorf("Boot = %v, cache %q row %q", err, e.g.Domain(), e.row(t))
		}
	})
}

// TestReconcileAfterDomainChanges: removing the conf or its certificate turns
// the restriction off in the same operation.
func TestReconcileAfterDomainChanges(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.writeConf(t, httpsDashboardConf)
	if err := e.g.Enable(ctx, domain); err != nil {
		t.Fatal(err)
	}
	if err := e.g.Reconcile(ctx); err != nil || e.g.Domain() != domain {
		t.Fatalf("Reconcile of a working domain = %v, cache %q", err, e.g.Domain())
	}
	e.writeConf(t, httpOnlyDashboardConf)
	if err := e.g.Reconcile(ctx); err != nil || e.g.Domain() != "" || e.row(t) != "" {
		t.Fatalf("certificate gone: %v, cache %q row %q", err, e.g.Domain(), e.row(t))
	}
	e.writeConf(t, httpsDashboardConf)
	if err := e.g.Enable(ctx, domain); err != nil {
		t.Fatal(err)
	}
	e.removeConf(t)
	if err := e.g.Reconcile(ctx); err != nil || e.g.Domain() != "" || e.row(t) != "" {
		t.Fatalf("conf gone: %v, cache %q row %q", err, e.g.Domain(), e.row(t))
	}
	if err := e.g.Reconcile(ctx); err != nil {
		t.Errorf("Reconcile while off = %v", err)
	}
}

// waitFor polls cond until it holds or d passes.
func waitFor(t *testing.T, d time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("%s: not within %s", what, d)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestWatch: changes made by another process (the CLI writes the row) reach
// the cache within the interval, and a conf that disappears turns it off.
func TestWatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e := newEnv(t)
	e.writeConf(t, httpsDashboardConf)
	const every = 20 * time.Millisecond
	go e.g.Watch(ctx, every)

	if err := e.d.Store.SetSetting(ctx, panelaccess.SettingKey, domain); err != nil {
		t.Fatal(err)
	}
	waitFor(t, time.Second, "external close", func() bool { return e.g.Domain() == domain })
	if err := e.d.Store.DeleteSetting(ctx, panelaccess.SettingKey); err != nil {
		t.Fatal(err)
	}
	waitFor(t, time.Second, "external open", func() bool { return e.g.Domain() == "" })

	// A certificate retry briefly rewrites the conf without :443: the
	// watcher leaves that to the retry's own reconcile.
	if err := e.g.Enable(ctx, domain); err != nil {
		t.Fatal(err)
	}
	e.writeConf(t, httpOnlyDashboardConf)
	time.Sleep(5 * every)
	if e.g.Domain() != domain {
		t.Fatal("watcher turned it off during a certificate retry")
	}
	e.removeConf(t)
	waitFor(t, time.Second, "conf removed", func() bool { return e.g.Domain() == "" && e.row(t) == "" })
}

// TestViaDomainAndStatus covers the request half of the enable guard.
func TestViaDomainAndStatus(t *testing.T) {
	e := newEnv(t)
	e.writeConf(t, httpsDashboardConf)
	st, err := e.d.Edge.Nginx.PanelState(domain)
	if err != nil || !panelaccess.Usable(st) {
		t.Fatalf("PanelState = %+v %v", st, err)
	}
	req := func(trusted bool, headers map[string]string) *http.Request {
		r := httptest.NewRequest("GET", "/api/domains", nil)
		r.Host = "203.0.113.9:8080"
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		return r.WithContext(httpx.WithPeerTrusted(r.Context(), trusted))
	}
	open := "Open the dashboard at https://" + domain + " first, then turn this on"
	for _, tc := range []struct {
		name    string
		trusted bool
		headers map[string]string
		reason  string
	}{
		{"through the proxy over https", true, map[string]string{"X-Forwarded-Proto": "https", "X-Forwarded-Host": domain}, ""},
		{"host header only", true, map[string]string{"X-Forwarded-Proto": "https", "Host": domain}, ""},
		{"direct with forged headers", false, map[string]string{"X-Forwarded-Proto": "https", "X-Forwarded-Host": domain}, open},
		{"through the proxy over http", true, map[string]string{"X-Forwarded-Proto": "http", "X-Forwarded-Host": domain}, open},
		{"through the proxy for another host", true, map[string]string{"X-Forwarded-Proto": "https", "X-Forwarded-Host": "other.example.com"}, open},
		{"by IP", true, nil, open},
	} {
		r := req(tc.trusted, tc.headers)
		if h, ok := tc.headers["Host"]; ok {
			r.Host = h
		}
		got := e.g.StatusOf(r, domain, st)
		want := panelaccess.Status{CanEnable: tc.reason == "", Reason: tc.reason}
		if got != want {
			t.Errorf("%s: %+v, want %+v", tc.name, got, want)
		}
	}
	if err := e.g.Enable(context.Background(), domain); err != nil {
		t.Fatal(err)
	}
	if got := e.g.StatusOf(req(false, nil), domain, st); !got.DomainOnly || got.CanEnable {
		t.Errorf("status while on = %+v", got)
	}
}
