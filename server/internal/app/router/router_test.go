package router

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"docklift/internal/app/apptest"
	"docklift/internal/auth"
	"docklift/internal/config"
)

type response struct {
	code   int
	header http.Header
	body   string
}

func (r response) errorMsg() string {
	var e struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal([]byte(r.body), &e)
	return e.Error
}

func do(t *testing.T, h http.Handler, method, path string, headers map[string]string) response {
	t.Helper()
	return doFrom(t, h, "192.0.2.1:1234", method, path, headers)
}

// doFrom sends the request from the socket peer remote.
func doFrom(t *testing.T, h http.Handler, remote, method, path string, headers map[string]string) response {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = remote
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	body, _ := io.ReadAll(rec.Body)
	return response{rec.Code, rec.Header(), string(body)}
}

func TestMountTableAuthRules(t *testing.T) {
	d := apptest.NewDeps(t)
	h := New(d)
	_, session := apptest.CreateAdmin(t, d, "ada@example.com", "password123")
	u, _ := d.Store.GetUserByEmail(t.Context(), "ada@example.com")
	sse, _ := d.Auth.IssuePurpose(auth.TokenSubject{UserID: u.ID, PasswordVersion: u.PasswordVersion()}, auth.PurposeSSE)
	bearer := map[string]string{"Authorization": "Bearer " + session}

	cases := []struct {
		name    string
		method  string
		path    string
		headers map[string]string
		code    int
		err     string
	}{
		// Cases that pass a guard use sub-paths no group defines, so they hold
		// whatever the group's routes answer.
		{"unknown api path", "GET", "/api/nope", nil, 404, "Not found"},
		{"protected group", "GET", "/api/projects", nil, 401, "Authentication required"},
		{"protected unknown subpath still authenticates", "DELETE", "/api/deployments/x/y", nil, 401, "Authentication required"},
		{"authenticated reaches group", "GET", "/api/projects/x/y/z", bearer, 404, "Not found"},
		{"databases", "GET", "/api/databases", nil, 401, "Authentication required"},
		{"files", "GET", "/api/files", nil, 401, "Authentication required"},
		{"ports", "GET", "/api/ports", nil, 401, "Authentication required"},
		{"domains", "GET", "/api/domains", nil, 401, "Authentication required"},
		{"github webhook is public", "POST", "/api/github/webhook/x/y/z", nil, 404, "Not found"},
		{"github callback is public", "GET", "/api/github/callback/x/y/z?code=x", nil, 404, "Not found"},
		{"github manifest callback is public", "GET", "/api/github/manifest/callback/x/y/z", nil, 404, "Not found"},
		{"github setup prefix is public", "GET", "/api/github/setup-anything/x/y/z", nil, 404, "Not found"},
		{"github private", "GET", "/api/github/status", nil, 401, "Authentication required"},
		{"github encoded public path is not public", "POST", "/api/github/%77ebhook/x/y/z", nil, 401, "Authentication required"},
		{"system logs use sse token", "GET", "/api/system/logs/backend", nil, 401, "SSE token required"},
		{"system logs accept sse token", "GET", "/api/system/logs/backend/x/y/z?token=" + sse, nil, 404, "Not found"},
		{"system logs POST uses bearer", "POST", "/api/system/logs/backend?token=" + sse, nil, 401, "Authentication required"},
		{"system logs HEAD uses bearer", "HEAD", "/api/system/logs/backend?token=" + sse, nil, 401, ""},
		{"system other route bearer only", "GET", "/api/system/stats?token=" + sse, nil, 401, "Authentication required"},
		{"logs stream uses sse token", "GET", "/api/logs/c1/stream/x", nil, 401, "SSE token required"},
		{"logs list bearer only", "GET", "/api/logs/containers?token=" + sse, nil, 401, "Authentication required"},
		{"backup bearer", "GET", "/api/backup/list", nil, 401, "Authentication required"},
		{"backup restore-upload bad setup token", "POST", "/api/backup/restore-upload", map[string]string{"X-Setup-Token": "nope"}, 401, "Authentication required"},
		{"websocket path is never the SPA", "GET", "/ws/terminal", nil, 404, "Not found"},
		{"health", "GET", "/api/health", nil, 200, ""},
		{"health HEAD", "HEAD", "/api/health", nil, 200, ""},
		{"trailing slash routes like Express", "GET", "/api/health/", nil, 200, ""},
		{"trailing slash in a group", "GET", "/api/auth/me/", nil, 401, "Authentication required"},
	}
	for _, c := range cases {
		r := do(t, h, c.method, c.path, c.headers)
		if r.code != c.code || (c.err != "" && r.errorMsg() != c.err) {
			t.Errorf("%s: %s %s = %d %s", c.name, c.method, c.path, r.code, r.body)
		}
	}
}

func TestBackupSetupTokenPath(t *testing.T) {
	d := apptest.NewDeps(t)
	h := New(d)
	token, err := d.Auth.Bootstrap().EnsureSetupToken()
	if err != nil {
		t.Fatal(err)
	}
	r := do(t, h, "POST", "/api/backup/restore-upload", map[string]string{"X-Setup-Token": token})
	// The route's own "no file" answer proves the guard let the request through.
	if r.code != 400 || r.errorMsg() != "No backup file uploaded" {
		t.Errorf("valid setup token must pass the guard to the backup routes: %d %s", r.code, r.body)
	}
	if _, err := os.Stat(d.Auth.Bootstrap().SetupTokenPath()); err != nil {
		t.Error("the guard must not consume the setup token")
	}
	if r := do(t, h, "GET", "/api/backup/restore-upload", map[string]string{"X-Setup-Token": token}); r.code != 401 {
		t.Errorf("only POST restore-upload accepts the setup token: %d", r.code)
	}
}

func TestSecurityHeadersAndSPA(t *testing.T) {
	d := apptest.NewDeps(t)
	h := New(d)
	for _, p := range []string{"/", "/projects/abc", "/index.html"} {
		r := do(t, h, "GET", p, nil)
		if r.code != 200 || !strings.Contains(r.body, "<html") || r.header.Get("Cache-Control") != "no-cache" ||
			r.header.Get("Content-Security-Policy") == "" || r.header.Get("X-Frame-Options") != "SAMEORIGIN" {
			t.Errorf("SPA %s: %d %v", p, r.code, r.header)
		}
	}
	if r := do(t, h, "GET", "/assets/missing.js", nil); r.code != 404 {
		t.Errorf("missing hashed asset must 404, got %d", r.code)
	}
	if r := do(t, h, "POST", "/", nil); r.code != 405 {
		t.Errorf("POST to the SPA = %d", r.code)
	}
	https := map[string]string{"X-Forwarded-Proto": "https"}
	api := doFrom(t, h, "172.28.0.3:40000", "GET", "/api/health", https)
	if api.header.Get("Strict-Transport-Security") == "" || api.header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("api security headers behind the proxy %v", api.header)
	}
	for _, remote := range []string{"192.0.2.1:1234", "172.28.0.1:40000", "[2001:db8::1]:443"} {
		if r := doFrom(t, h, remote, "GET", "/api/health", https); r.header.Get("Strict-Transport-Security") != "" {
			t.Errorf("X-Forwarded-Proto from untrusted peer %s must not enable HSTS", remote)
		}
	}
}

func TestTrustedProxiesConfig(t *testing.T) {
	limited := func(t *testing.T, h http.Handler, remote string, rotate bool) string {
		t.Helper()
		var last response
		for i := range 3 {
			xff := "198.51.100.1"
			if rotate {
				xff = "198.51.100." + strconv.Itoa(i+1)
			}
			last = doFrom(t, h, remote, "POST", "/api/auth/login", map[string]string{"X-Forwarded-For": xff})
		}
		return last.header.Get("RateLimit-Remaining")
	}
	d := apptest.NewDeps(t)
	h := New(d)
	// Default policy: loopback is a trusted proxy, so rotated addresses get fresh buckets.
	if got := limited(t, h, "127.0.0.1:5000", true); got != "99" {
		t.Errorf("loopback proxy: RateLimit-Remaining = %s, want 99", got)
	}
	// A direct client cannot rotate its way out of its bucket.
	if got := limited(t, h, "192.0.2.50:5000", true); got != "97" {
		t.Errorf("direct client: RateLimit-Remaining = %s, want 97", got)
	}

	d = apptest.NewDeps(t)
	pt, err := config.ParseTrustedProxies("none")
	if err != nil {
		t.Fatal(err)
	}
	d.Config.TrustedProxies = pt
	h = New(d)
	if got := limited(t, h, "127.0.0.1:5000", true); got != "97" {
		t.Errorf("TRUSTED_PROXIES=none: RateLimit-Remaining = %s, want 97", got)
	}
}

func TestMaintenanceGateOnAPI(t *testing.T) {
	d := apptest.NewDeps(t)
	h := New(d)
	d.Maintenance.Enter("Restoring backup")
	if r := do(t, h, "GET", "/api/auth/status", nil); r.code != 503 || r.body != `{"error":"Restoring backup","maintenance":true}` {
		t.Errorf("gated: %d %s", r.code, r.body)
	}
	if r := do(t, h, "GET", "/api/health", nil); r.code != 200 {
		t.Errorf("health during maintenance: %d", r.code)
	}
	if r := do(t, h, "GET", "/", nil); r.code != 200 {
		t.Errorf("the dashboard itself stays reachable during maintenance: %d", r.code)
	}
	d.Maintenance.Exit()
	if err := d.Maintenance.EnterCritical("rollback failed", d.Config.DatabasePath); err != nil {
		t.Fatal(err)
	}
	r := do(t, h, "GET", "/api/projects", nil)
	if r.code != 503 || !strings.Contains(r.body, `"critical":true`) || !strings.Contains(r.body, `"sealPath"`) {
		t.Errorf("critical: %d %s", r.code, r.body)
	}
	if r := do(t, h, "GET", "/api/backup/critical-status", nil); r.code != 401 {
		t.Errorf("critical-status passes the gate (then needs auth): %d", r.code)
	}
}

func TestRateLimitScopes(t *testing.T) {
	d := apptest.NewDeps(t)
	h := New(d)
	login := do(t, h, "POST", "/api/auth/login", nil)
	if login.header.Get("RateLimit-Limit") != "100" || login.header.Get("RateLimit-Remaining") != "99" {
		t.Errorf("auth limiter headers %v", login.header)
	}
	if status := do(t, h, "GET", "/api/auth/status", nil); status.header.Get("RateLimit-Limit") != "" {
		t.Errorf("/status is exempt from the auth limiter: %v", status.header)
	}
	if status := do(t, h, "GET", "/api/auth/status/", nil); status.code != 200 || status.header.Get("RateLimit-Limit") != "100" {
		t.Errorf("Express skips only the exact /status path: %d %v", status.code, status.header)
	}
	for i := range 10 {
		r := do(t, h, "GET", "/api/auth/setup-token", nil)
		if r.code == 429 {
			t.Fatalf("setup limiter tripped early at %d", i)
		}
		if r.header.Get("RateLimit-Limit") != "100" {
			t.Errorf("auth limiter runs after the setup limiter and its headers win: %v", r.header)
		}
	}
	r := do(t, h, "POST", "/api/auth/register", nil)
	if r.code != 429 || r.errorMsg() != "Too many setup attempts, please try again later" || r.header.Get("RateLimit-Limit") != "10" || r.header.Get("Retry-After") == "" {
		t.Errorf("11th setup request: %d %s %v", r.code, r.body, r.header)
	}
	if r := do(t, h, "POST", "/api/auth/login", nil); r.code == 429 {
		t.Error("the setup limiter must not block other auth routes")
	}
	if r := do(t, h, "GET", "/api/projects", nil); r.header.Get("RateLimit-Limit") != "" {
		t.Error("only /api/auth is rate limited")
	}
}
