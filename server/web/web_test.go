package web

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func testSPA() http.Handler {
	return &spa{modTime: time.Unix(1_800_000_000, 0), files: fstest.MapFS{
		"index.html":           {Data: []byte("<!doctype html><title>app</title>")},
		"favicon.svg":          {Data: []byte("<svg/>")},
		"assets/index-abc.js":  {Data: []byte("console.log(1)")},
		"assets/index-abc.css": {Data: []byte("body{}")},
	}}
}

func TestSPA(t *testing.T) {
	h := testSPA()
	cases := []struct {
		method, path string
		code         int
		cache        string
		contentType  string
		bodyHas      string
	}{
		{"GET", "/", 200, "no-cache", "text/html", "<title>app</title>"},
		{"GET", "/index.html", 200, "no-cache", "text/html", "<title>app</title>"},
		{"GET", "/projects/123/settings", 200, "no-cache", "text/html", "<title>app</title>"},
		{"GET", "/assets/index-abc.js", 200, "public, max-age=31536000, immutable", "javascript", "console.log"},
		{"GET", "/assets/index-abc.css", 200, "public, max-age=31536000, immutable", "text/css", "body{}"},
		{"GET", "/assets/gone-123.js", 404, "", "", ""},
		{"GET", "/favicon.svg", 200, "", "image/svg+xml", "<svg/>"},
		{"GET", "/../../etc/passwd", 200, "no-cache", "text/html", "<title>app</title>"},
		{"HEAD", "/", 200, "no-cache", "text/html", ""},
		{"GET", "/api/unknown", 404, "", "application/json", `"error":"Not found"`},
		{"GET", "/api", 404, "", "application/json", `"error":"Not found"`},
		{"GET", "/API/Health", 404, "", "application/json", `"error":"Not found"`},
		{"GET", "/ws/terminal", 404, "", "application/json", `"error":"Not found"`},
		{"POST", "/", 405, "", "application/json", "Method not allowed"},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(c.method, c.path, nil))
		if rec.Code != c.code || rec.Header().Get("Cache-Control") != c.cache ||
			!strings.Contains(rec.Header().Get("Content-Type"), c.contentType) || !strings.Contains(rec.Body.String(), c.bodyHas) {
			t.Errorf("%s %s: %d cache=%q type=%q body=%q", c.method, c.path, rec.Code,
				rec.Header().Get("Cache-Control"), rec.Header().Get("Content-Type"), rec.Body.String())
		}
	}
}

func TestEmbeddedDist(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "<html") || rec.Header().Get("Content-Security-Policy") == "" {
		t.Errorf("embedded index.html: %d %q", rec.Code, rec.Body.String())
	}
}

// directive returns the sources of one CSP directive.
func directive(policy, name string) []string {
	for _, d := range strings.Split(policy, ";") {
		f := strings.Fields(d)
		if len(f) > 0 && f[0] == name {
			return f[1:]
		}
	}
	return nil
}

func TestContentSecurityPolicy(t *testing.T) {
	inline := "window.__theme=1"
	files := fstest.MapFS{
		"index.html": {Data: []byte(`<!doctype html><script>` + inline + `</script>` +
			`<script type="module" crossorigin src="/assets/index-abc.js"></script>`)},
		"assets/index-abc.js": {Data: []byte(`a="https://cdn.jsdelivr.net/npm/monaco-editor@0.55.1/min/vs";b="https://cdn.jsdelivr.net/npm/other@1/x.js"`)},
	}
	sum := sha256.Sum256([]byte(inline))
	hash := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	monaco := "https://cdn.jsdelivr.net/npm/monaco-editor@0.55.1/"
	p := newPolicy(files)

	got := p.header("panel.example.com:8080")
	if s := directive(got, "script-src"); !slices.Equal(s, []string{"'self'", monaco, hash}) {
		t.Errorf("script-src %v", s)
	}
	if c := directive(got, "connect-src"); !slices.Equal(c, []string{"'self'", "ws://panel.example.com:8080", "wss://panel.example.com:8080", "https://api.github.com"}) {
		t.Errorf("connect-src %v", c)
	}
	if w := directive(got, "worker-src"); !slices.Equal(w, []string{"'self'", "blob:"}) {
		t.Errorf("worker-src %v", w)
	}
	for _, d := range []string{"style-src", "font-src"} {
		if !slices.Contains(directive(got, d), monaco) {
			t.Errorf("%s lacks the Monaco CDN path: %v", d, directive(got, d))
		}
	}
	if strings.Contains(got, "other@1") || strings.Contains(got, "https://cdn.jsdelivr.net ") {
		t.Errorf("only the Monaco path of the CDN may be allowed: %s", got)
	}

	// A Host header that is not host[:port] must not reach the policy.
	for _, host := range []string{"evil.com; script-src *", "a b", "", "x.com/path", "[::1]:8000"} {
		c := directive(p.header(host), "connect-src")
		want := []string{"'self'", "https://api.github.com"}
		if host == "[::1]:8000" {
			want = []string{"'self'", "ws://[::1]:8000", "wss://[::1]:8000", "https://api.github.com"}
		}
		if !slices.Equal(c, want) {
			t.Errorf("host %q: connect-src %v", host, c)
		}
	}
}

// The served policy never allows inline or arbitrary-origin scripts or
// sockets (2.1.x sent script-src 'unsafe-inline' and connect-src ws: wss:).
func TestServedPolicyIsStrict(t *testing.T) {
	for name, h := range map[string]http.Handler{"embedded": Handler(), "test": testSPA()} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		req.Host = "dash.example.com"
		h.ServeHTTP(rec, req)
		got := rec.Header().Get("Content-Security-Policy")
		script, connect := directive(got, "script-src"), directive(got, "connect-src")
		if len(script) == 0 || slices.Contains(script, "'unsafe-inline'") || slices.Contains(script, "'unsafe-eval'") {
			t.Errorf("%s script-src %v", name, script)
		}
		for _, src := range connect {
			if src == "ws:" || src == "wss:" || src == "https:" || src == "*" {
				t.Errorf("%s connect-src allows any origin: %v", name, connect)
			}
		}
		if !slices.Contains(connect, "wss://dash.example.com") {
			t.Errorf("%s connect-src lacks the same-host socket: %v", name, connect)
		}
	}
}
