// Package web serves the embedded dashboard build (frontend/dist copied into
// web/dist before building) with SPA fallback, replacing the docklift-frontend
// nginx container: hashed /assets/ files are immutable, index.html is never
// cached, and unknown client-side routes get index.html.
package web

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"docklift/internal/httpx"
)

//go:embed all:dist
var distFS embed.FS

// contentSecurityPolicy is the policy the dashboard gateway nginx added to
// every dashboard response.
const contentSecurityPolicy = "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; img-src 'self' data: https:; font-src 'self' data: https://fonts.gstatic.com; connect-src 'self' ws: wss: https://api.github.com; frame-ancestors 'self'; base-uri 'self'; form-action 'self' https://github.com;"

// Handler serves the dashboard. Paths under /api/ and /ws/ are never answered
// with the SPA: they get a JSON 404 so API clients never parse HTML.
func Handler() http.Handler {
	dist, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic("web: embedded dist directory missing: " + err.Error())
	}
	return &spa{files: dist, modTime: time.Now()}
}

type spa struct {
	files   fs.FS
	modTime time.Time
}

func (s *spa) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	// Express matched routes case-insensitively, so /API/... was an API path too.
	if lower := strings.ToLower(p); lower == "/api" || lower == "/ws" || strings.HasPrefix(lower, "/api/") || strings.HasPrefix(lower, "/ws/") {
		httpx.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		httpx.Error(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	w.Header().Set("Content-Security-Policy", contentSecurityPolicy)

	name := strings.TrimPrefix(path.Clean("/"+p), "/")
	if name != "" && name != "index.html" && s.isFile(name) {
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		s.serveFile(w, r, name)
		return
	}
	if strings.HasPrefix(name, "assets/") {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	s.serveFile(w, r, "index.html")
}

func (s *spa) isFile(name string) bool {
	st, err := fs.Stat(s.files, name)
	return err == nil && st.Mode().IsRegular()
}

func (s *spa) serveFile(w http.ResponseWriter, r *http.Request, name string) {
	data, err := fs.ReadFile(s.files, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, name, s.modTime, bytes.NewReader(data))
}
