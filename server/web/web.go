// Package web serves the embedded dashboard build (frontend/dist copied into
// web/dist before building) with SPA fallback, replacing the docklift-frontend
// nginx container: hashed /assets/ files are immutable, index.html is never
// cached, and unknown client-side routes get index.html.
package web

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"docklift/internal/httpx"
)

//go:embed all:dist
var distFS embed.FS

// monacoCDNPattern matches the Monaco base URL @monaco-editor/loader bakes
// into the bundle (it loads the editor from jsDelivr unless configured
// otherwise).
var monacoCDNPattern = regexp.MustCompile(`https://cdn\.jsdelivr\.net/npm/monaco-editor@[0-9A-Za-z.+-]+/min/vs`)

// inlineScriptPattern matches <script> elements of index.html.
var inlineScriptPattern = regexp.MustCompile(`(?is)<script\b([^>]*)>(.*?)</script\s*>`)

// scriptSrcAttr matches a src attribute among a <script> element's attributes.
var scriptSrcAttr = regexp.MustCompile(`(?i)(?:^|\s)src\s*=`)

// policy is the Content-Security-Policy of the dashboard, derived once from
// the embedded build:
//
//   - scripts: same origin, the exact Monaco version path on jsDelivr found
//     in the bundle (nothing else on that CDN), and sha256 hashes of inline
//     scripts in index.html (none in the current build), never
//     'unsafe-inline';
//   - styles keep 'unsafe-inline': React, xterm and Monaco set style
//     attributes and inject <style> elements;
//   - connect: same origin plus the same host over ws:/wss: (added per
//     request, see header) and api.github.com (sidebar star count);
//   - workers: Monaco starts its language workers from blob: URLs that
//     importScripts the same pinned CDN path (allowed by script-src).
type policy struct {
	monaco       []string // "https://cdn.jsdelivr.net/npm/monaco-editor@x/"
	scriptHashes []string // "'sha256-...'"
}

// newPolicy scans the build for the Monaco CDN path and inline scripts.
func newPolicy(files fs.FS) policy {
	var p policy
	seen := map[string]bool{}
	assets, _ := fs.Glob(files, "assets/*.js")
	for _, name := range assets {
		data, err := fs.ReadFile(files, name)
		if err != nil {
			continue
		}
		for _, m := range monacoCDNPattern.FindAll(data, -1) {
			src := strings.TrimSuffix(string(m), "min/vs")
			if !seen[src] {
				seen[src] = true
				p.monaco = append(p.monaco, src)
			}
		}
	}
	slices.Sort(p.monaco)
	if index, err := fs.ReadFile(files, "index.html"); err == nil {
		for _, m := range inlineScriptPattern.FindAllSubmatch(index, -1) {
			if scriptSrcAttr.Match(m[1]) {
				continue
			}
			sum := sha256.Sum256(m[2])
			p.scriptHashes = append(p.scriptHashes, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
		}
	}
	return p
}

// hostPattern is a Host header value safe to place in the policy: a DNS name
// or IPv4 address, or a bracketed IPv6 address, with an optional port.
var hostPattern = regexp.MustCompile(`^(?:[A-Za-z0-9](?:[A-Za-z0-9.-]*[A-Za-z0-9])?|\[[0-9A-Fa-f:.]+\])(?::[0-9]{1,5})?$`)

// header returns the policy for a request to host. 'self' already covers
// same-host ws:/wss: in CSP Level 3 browsers; the explicit sources keep the
// terminal working in browsers that predate that rule. A host that is not a
// plain host[:port] adds nothing.
func (p policy) header(host string) string {
	script := append([]string{"'self'"}, p.monaco...)
	script = append(script, p.scriptHashes...)
	style := append([]string{"'self'", "'unsafe-inline'", "https://fonts.googleapis.com"}, p.monaco...)
	font := append([]string{"'self'", "data:", "https://fonts.gstatic.com"}, p.monaco...)
	connect := []string{"'self'"}
	if hostPattern.MatchString(host) {
		connect = append(connect, "ws://"+host, "wss://"+host)
	}
	connect = append(connect, "https://api.github.com")
	return "default-src 'self'; script-src " + strings.Join(script, " ") +
		"; style-src " + strings.Join(style, " ") +
		"; img-src 'self' data: https:; font-src " + strings.Join(font, " ") +
		"; connect-src " + strings.Join(connect, " ") +
		"; worker-src 'self' blob:; object-src 'none'; frame-ancestors 'self'; base-uri 'self'; form-action 'self' https://github.com;"
}

// Handler serves the dashboard. Paths under /api/ and /ws/ are never answered
// with the SPA: they get a JSON 404 so API clients never parse HTML.
func Handler() http.Handler {
	dist, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic("web: embedded dist directory missing: " + err.Error())
	}
	return &spa{files: dist, modTime: time.Now(), csp: newPolicy(dist)}
}

type spa struct {
	files   fs.FS
	modTime time.Time
	csp     policy
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
	w.Header().Set("Content-Security-Policy", s.csp.header(r.Host))

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
