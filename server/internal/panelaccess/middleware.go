package panelaccess

import (
	"net/http"
	"strings"

	"docklift/internal/httpx"
)

// healthBody is what /api/health tells a direct client while access is
// restricted: enough for host-side health checks, no version or instance id.
type healthBody struct {
	Status string `json:"status"`
}

// Middleware enforces the restriction. Install it right after the
// trusted-proxy middleware, before every route (including the terminal
// upgrade), authentication and body parsing. While access is restricted to a
// domain, a request whose socket peer is not a trusted proxy gets:
//
//   - GET/HEAD /api/health: 200 {"status":"ok"} (Docker and install/upgrade
//     health checks; they run inside the container on loopback, which is
//     trusted, but host-side checks such as scripts/smoke.sh are direct);
//   - POST /api/github/webhook: served as usual (HMAC-signed by GitHub, and a
//     GitHub App created over http://SERVER_IP:8080 delivers there);
//   - a browser navigation (GET/HEAD accepting text/html, no upgrade): 302 to
//     https://<domain> with the same path and query;
//   - anything else (API, WebSocket upgrades, SSE, assets): 403 with a JSON
//     error naming the domain.
//
// The redirect is temporary and nothing is cacheable, so reopening access
// works at once in browsers that saw it.
func (g *Guard) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		domain := g.Domain()
		if domain == "" || httpx.PeerTrusted(r) {
			next.ServeHTTP(w, r)
			return
		}
		p := httpx.RoutePath(r)
		get := r.Method == http.MethodGet || r.Method == http.MethodHead
		switch {
		case get && (p == "/api/health" || p == "/api/health/"):
			w.Header().Set("Cache-Control", "no-store")
			httpx.JSON(w, http.StatusOK, healthBody{Status: "ok"})
		case r.Method == http.MethodPost && (p == "/api/github/webhook" || p == "/api/github/webhook/"):
			next.ServeHTTP(w, r)
		case get && isNavigation(r):
			w.Header().Set("Cache-Control", "no-store")
			http.Redirect(w, r, "https://"+domain+r.URL.RequestURI(), http.StatusFound)
		default:
			w.Header().Set("Cache-Control", "no-store")
			httpx.Error(w, http.StatusForbidden, "Dashboard access by IP is disabled. Use https://"+domain)
		}
	})
}

// isNavigation reports whether r looks like a browser loading a page: it
// accepts HTML and is not a WebSocket (or other) upgrade.
func isNavigation(r *http.Request) bool {
	if len(r.Header.Values("Upgrade")) > 0 {
		return false
	}
	for _, v := range r.Header.Values("Accept") {
		if strings.Contains(strings.ToLower(v), "text/html") {
			return true
		}
	}
	return false
}
