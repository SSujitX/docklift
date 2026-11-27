package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var ok = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { JSON(w, http.StatusOK, map[string]bool{"ok": true}) })

func TestSecurityHeaders(t *testing.T) {
	cases := []struct {
		proto string
		hsts  bool
	}{{"", false}, {"https", true}, {"http", false}, {"https, http", false}}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
		if c.proto != "" {
			req.Header.Set("X-Forwarded-Proto", c.proto)
		}
		rec := httptest.NewRecorder()
		SecurityHeaders(ok).ServeHTTP(rec, req)
		h := rec.Header()
		if h.Get("X-Content-Type-Options") != "nosniff" || h.Get("X-Frame-Options") != "SAMEORIGIN" ||
			h.Get("X-XSS-Protection") != "1; mode=block" || h.Get("Referrer-Policy") != "strict-origin-when-cross-origin" ||
			h.Get("Permissions-Policy") != "camera=(), microphone=(), geolocation=()" {
			t.Errorf("proto %q: missing security headers %v", c.proto, h)
		}
		if got := h.Get("Strict-Transport-Security") != ""; got != c.hsts {
			t.Errorf("proto %q: HSTS = %v, want %v", c.proto, got, c.hsts)
		}
	}
}

func TestIsTrustedOrigin(t *testing.T) {
	cases := []struct {
		name    string
		origin  string
		host    string
		headers map[string]string
		allow   []string
		want    bool
	}{
		{"same origin", "http://panel.example.com:8080", "panel.example.com:8080", nil, nil, true},
		{"case-insensitive host", "http://PANEL.example.com:8080", "panel.example.com:8080", nil, nil, true},
		{"explicit default port", "http://panel.example.com:80", "panel.example.com", nil, nil, true},
		{"deployed app port", "http://panel.example.com:5500", "panel.example.com:8080", nil, nil, false},
		{"other host", "http://evil.example", "panel.example.com", nil, nil, false},
		{"forwarded host and proto", "https://panel.example.com", "backend:8000", map[string]string{"X-Forwarded-Host": "panel.example.com", "X-Forwarded-Proto": "https"}, nil, true},
		{"first forwarded value", "https://a.example", "backend", map[string]string{"X-Forwarded-Host": "a.example, b.example", "X-Forwarded-Proto": "https, http"}, nil, true},
		{"tls stripped, both default ports", "https://panel.example.com", "panel.example.com", nil, nil, true},
		{"tls stripped, non-default port", "https://panel.example.com:8443", "panel.example.com:8443", nil, nil, false},
		{"allowlist exact", "http://localhost:3600", "localhost:8000", nil, []string{" http://localhost:3600 "}, true},
		{"allowlist scheme is deliberate", "https://localhost:3600", "localhost:8000", nil, []string{"http://localhost:3600"}, false},
		{"not http", "file:///etc/passwd", "x", nil, nil, false},
		{"null origin", "null", "x", nil, nil, false},
		{"garbage", "::::", "x", nil, nil, false},
		{"ipv6", "http://[::1]:8000", "[::1]:8000", nil, nil, true},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = c.host
		for k, v := range c.headers {
			req.Header.Set(k, v)
		}
		if got := IsTrustedOrigin(c.origin, req, RequestProtocol(req), c.allow); got != c.want {
			t.Errorf("%s: IsTrustedOrigin(%q) = %v", c.name, c.origin, got)
		}
	}
	if NormalizeOrigin("HTTPS://Example.com:0443/path") != "https://example.com:443" {
		t.Errorf("NormalizeOrigin = %q", NormalizeOrigin("HTTPS://Example.com:0443/path"))
	}
}

func TestCORS(t *testing.T) {
	h := CORS([]string{"http://localhost:3600"})(ok)
	do := func(method, origin string, extra map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/auth/login", nil)
		req.Host = "panel.example.com"
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		for k, v := range extra {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	noOrigin := do(http.MethodGet, "", nil)
	if noOrigin.Header().Get("Access-Control-Allow-Origin") != "" || noOrigin.Header().Get("Vary") != "Origin" ||
		noOrigin.Header().Get("Access-Control-Allow-Credentials") != "true" || noOrigin.Code != 200 {
		t.Errorf("no Origin: %d %v", noOrigin.Code, noOrigin.Header())
	}
	trusted := do(http.MethodGet, "http://panel.example.com", nil)
	if trusted.Header().Get("Access-Control-Allow-Origin") != "http://panel.example.com" {
		t.Errorf("same-origin not reflected: %v", trusted.Header())
	}
	allowed := do(http.MethodGet, "http://localhost:3600", nil)
	if allowed.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3600" {
		t.Errorf("allowlisted origin not reflected: %v", allowed.Header())
	}
	evil := do(http.MethodGet, "http://evil.example", nil)
	if len(evil.Header().Values("Access-Control-Allow-Origin")) != 0 || evil.Header().Get("Vary") != "" || evil.Code != 200 {
		t.Errorf("untrusted origin must get no CORS headers but still be served: %d %v", evil.Code, evil.Header())
	}
	pre := do(http.MethodOptions, "http://panel.example.com", map[string]string{"Access-Control-Request-Headers": "content-type,authorization"})
	if pre.Code != 204 || pre.Header().Get("Access-Control-Allow-Methods") != "GET,HEAD,PUT,PATCH,POST,DELETE" ||
		pre.Header().Get("Access-Control-Allow-Headers") != "content-type,authorization" ||
		pre.Header().Get("Vary") != "Origin, Access-Control-Request-Headers" || pre.Header().Get("Content-Length") != "0" {
		t.Errorf("preflight: %d %v", pre.Code, pre.Header())
	}
	evilPre := do(http.MethodOptions, "http://evil.example", nil)
	if evilPre.Code != 200 {
		t.Errorf("untrusted preflight must fall through to routing, got %d", evilPre.Code)
	}
}

func TestClientIPTrustProxyOne(t *testing.T) {
	cases := []struct {
		remote, xff, want string
	}{
		{"10.0.0.2:5555", "", "10.0.0.2"},
		{"10.0.0.2:5555", "203.0.113.7", "203.0.113.7"},
		{"10.0.0.2:5555", "1.1.1.1, 203.0.113.7", "203.0.113.7"},
		{"10.0.0.2:5555", "1.1.1.1,   203.0.113.7  ", "203.0.113.7"},
		{"10.0.0.2:5555", "1.1.1.1, ,", "1.1.1.1"},
		{"[2001:db8::1]:443", "", "2001:db8::1"},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = c.remote
		if c.xff != "" {
			req.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := ClientIP(req); got != c.want {
			t.Errorf("ClientIP(remote=%s, xff=%q) = %q, want %q", c.remote, c.xff, got, c.want)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Add("X-Forwarded-For", "1.1.1.1")
	req.Header.Add("X-Forwarded-For", "2.2.2.2")
	if got := ClientIP(req); got != "2.2.2.2" {
		t.Errorf("repeated header = %q", got)
	}
}

func TestRateLimitKey(t *testing.T) {
	cases := map[string]string{
		"203.0.113.7":                 "203.0.113.7",
		"::ffff:203.0.113.7":          "203.0.113.7",
		"::203.0.113.7":               "203.0.113.7",
		"2001:db8:abcd:12ff:1::1":     "2001:db8:abcd:1200::/56",
		"2001:db8:abcd:1234:ffff::99": "2001:db8:abcd:1200::/56",
		"::1":                         "::/56",
		"not-an-ip":                   "not-an-ip",
	}
	for in, want := range cases {
		if got := RateLimitKey(in); got != want {
			t.Errorf("RateLimitKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRateLimiter(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	l := &RateLimiter{cfg: RateLimitConfig{Window: 15 * time.Minute, Limit: 2, Message: "Too many attempts, please try again later",
		Skip: func(r *http.Request) bool { return r.URL.Path == "/skip" }},
		now: func() time.Time { return now }, clients: map[string]*rateClient{}}
	h := l.Middleware(ok)
	hit := func(path, ip string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.RemoteAddr = ip + ":1234"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	first := hit("/login", "203.0.113.7")
	if first.Code != 200 || first.Header().Get("RateLimit-Policy") != "2;w=900" || first.Header().Get("RateLimit-Limit") != "2" ||
		first.Header().Get("RateLimit-Remaining") != "1" || first.Header().Get("RateLimit-Reset") != "900" || first.Header().Get("X-RateLimit-Limit") != "" {
		t.Errorf("first hit headers %v", first.Header())
	}
	now = now.Add(100*time.Second + 500*time.Millisecond)
	second := hit("/login", "203.0.113.7")
	if second.Header().Get("RateLimit-Remaining") != "0" || second.Header().Get("RateLimit-Reset") != "800" {
		t.Errorf("second hit headers %v", second.Header())
	}
	third := hit("/login", "203.0.113.7")
	var body ErrorBody
	_ = json.NewDecoder(third.Body).Decode(&body)
	if third.Code != 429 || body.Error != "Too many attempts, please try again later" || third.Header().Get("Retry-After") != "800" {
		t.Errorf("third hit: %d %q %v", third.Code, body.Error, third.Header())
	}
	if other := hit("/login", "198.51.100.1"); other.Code != 200 {
		t.Error("other clients have their own window")
	}
	if skipped := hit("/skip", "203.0.113.7"); skipped.Code != 200 || skipped.Header().Get("RateLimit-Limit") != "" {
		t.Errorf("skipped request must not be limited or counted: %d %v", skipped.Code, skipped.Header())
	}
	now = now.Add(15 * time.Minute)
	if again := hit("/login", "203.0.113.7"); again.Code != 200 || again.Header().Get("RateLimit-Remaining") != "1" {
		t.Errorf("window must reset: %d %v", again.Code, again.Header())
	}
}

type gateState struct {
	critical, active bool
}

func (g gateState) Critical() bool             { return g.critical }
func (g gateState) Active() bool               { return g.active }
func (g gateState) Reason() string             { return "Restoring" }
func (g gateState) CriticalMarkerPath() string { return "/data/.restore-critical" }

func TestMaintenanceGate(t *testing.T) {
	cases := []struct {
		state gateState
		path  string
		want  int
		body  string
	}{
		{gateState{}, "/api/projects", 200, ""},
		{gateState{active: true}, "/api/projects", 503, `{"error":"Restoring","maintenance":true}`},
		{gateState{active: true}, "/api/health", 200, ""},
		{gateState{active: true}, "/api/backup/restore-upload", 200, ""},
		{gateState{active: true}, "/api/backup/restore/stream", 200, ""},
		{gateState{active: true}, "/api/backup/clear-critical-restore", 503, ""},
		{gateState{critical: true}, "/api/health", 200, ""},
		{gateState{critical: true}, "/api/backup/critical-status", 200, ""},
		{gateState{critical: true}, "/api/backup/clear-critical-restore", 200, ""},
		{gateState{critical: true}, "/api/backup/restore-upload", 503, `{"error":"Restoring","maintenance":true,"critical":true,"sealPath":"/data/.restore-critical"}`},
		{gateState{critical: true}, "/api/auth/login", 503, ""},
		{gateState{active: true}, "/api/%68ealth", 503, ""},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, c.path, nil)
		rec := httptest.NewRecorder()
		MaintenanceGate(c.state)(ok).ServeHTTP(rec, req)
		if rec.Code != c.want || (c.body != "" && rec.Body.String() != c.body) {
			t.Errorf("%+v %s: %d %s", c.state, c.path, rec.Code, rec.Body.String())
		}
	}
}

func TestRecover(t *testing.T) {
	rec := httptest.NewRecorder()
	Recover(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != 500 || rec.Body.String() != `{"error":"Something went wrong!"}` {
		t.Errorf("panic response = %d %s", rec.Code, rec.Body.String())
	}
	defer func() {
		if p := recover(); p != http.ErrAbortHandler {
			t.Errorf("ErrAbortHandler must propagate, got %v", p)
		}
	}()
	Recover(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) })).
		ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

func TestHeadAsGet(t *testing.T) {
	var method, original string
	h := HeadAsGet(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		method, original = r.Method, OriginalMethod(r)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodHead, "/", nil))
	if method != http.MethodGet || original != http.MethodHead {
		t.Errorf("HEAD: method=%s original=%s", method, original)
	}
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil))
	if method != http.MethodPost || original != http.MethodPost {
		t.Errorf("POST: method=%s original=%s", method, original)
	}
	if !strings.HasPrefix(RoutePath(httptest.NewRequest(http.MethodGet, "/api/%68ealth", nil)), "/api/%68") {
		t.Error("RoutePath must keep the escaped form")
	}
}
