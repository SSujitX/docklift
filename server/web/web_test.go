package web

import (
	"net/http"
	"net/http/httptest"
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
