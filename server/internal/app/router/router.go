// Package router assembles the HTTP handler: the middleware stack and mount
// table of backend/src/index.ts, the WebSocket terminal and the embedded
// dashboard.
package router

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"docklift/internal/api/auth"
	"docklift/internal/api/backup"
	"docklift/internal/api/databases"
	"docklift/internal/api/deployments"
	"docklift/internal/api/domains"
	"docklift/internal/api/files"
	"docklift/internal/api/github"
	"docklift/internal/api/health"
	"docklift/internal/api/logs"
	"docklift/internal/api/ports"
	"docklift/internal/api/projects"
	"docklift/internal/api/system"
	"docklift/internal/api/terminal"
	"docklift/internal/app"
	authsvc "docklift/internal/auth"
	"docklift/internal/httpx"
	"docklift/web"
)

// Rate-limit settings of index.ts.
const (
	authWindow = 15 * time.Minute
	authLimit  = 100
	setupLimit = 10
)

// New returns the root handler. Limiter sweepers stop when d.Background ends.
func New(d *app.Deps) http.Handler {
	root := chi.NewRouter()
	// Peer trust is decided first: everything below (terminal origin check,
	// HSTS, CORS, rate limits) reads forwarded headers only from trusted proxies.
	trust := httpx.NewTrustedProxies(d.Config.TrustedProxies.Trusted, d.Config.TrustedProxies.Excluded)
	root.Use(httpx.Recover, trust.Middleware, httpx.HeadAsGet)
	root.NotFound(httpx.NotFound)
	root.MethodNotAllowed(httpx.NotFound)

	// The Node upgrade listener handled /ws/terminal outside Express.
	terminal.Attach(root, d)

	allow := append(append([]string(nil), d.Config.CORSOrigins...), d.Config.FrontendURL)
	root.Group(func(r chi.Router) {
		r.Use(httpx.SecurityHeaders, httpx.CORS(allow))
		r.Mount("/api", apiRouter(d))
		r.Handle("/*", web.Handler())
	})
	return root
}

func apiRouter(d *app.Deps) http.Handler {
	r := chi.NewRouter()
	r.NotFound(httpx.NotFound)
	r.MethodNotAllowed(httpx.NotFound)
	// StripSlashes changes only the routing path, so like Express (strict
	// routing off) /api/health/ reaches /api/health while the gate, limiters
	// and guards still see the request path as sent.
	r.Use(middleware.StripSlashes, httpx.ParseBody, httpx.MaintenanceGate(d.Maintenance))

	health.Mount(r, d)

	authLimiter := httpx.NewRateLimiter(d.Background, httpx.RateLimitConfig{
		Window:  authWindow,
		Limit:   authLimit,
		Message: "Too many attempts, please try again later",
		Skip:    func(r *http.Request) bool { return httpx.RoutePath(r) == "/api/auth/status" },
	})
	setupLimiter := httpx.NewRateLimiter(d.Background, httpx.RateLimitConfig{
		Window:  authWindow,
		Limit:   setupLimit,
		Message: "Too many setup attempts, please try again later",
	})
	// Like app.use(prefix, ...middleware, router): the middleware runs for every
	// request under prefix, whether or not a route of the group matches.
	group := func(prefix string, mount func(chi.Router, *app.Deps), middleware ...func(http.Handler) http.Handler) {
		sub := chi.NewRouter()
		sub.NotFound(httpx.NotFound)
		sub.MethodNotAllowed(httpx.NotFound)
		mount(sub, d)
		r.With(middleware...).Mount(prefix, sub)
	}
	group("/auth", auth.Mount, onlyUnder(setupLimiter.Middleware, "/api/auth/register", "/api/auth/setup-token"), authLimiter.Middleware)

	session := d.Auth.RequireSession
	group("/projects", projects.Mount, session)
	group("/deployments", deployments.Mount, session)
	group("/databases", databases.Mount, session)
	group("/files", files.Mount, session)
	group("/ports", ports.Mount, session)
	group("/github", github.Mount, githubGuard(session))
	group("/system", system.Mount, sseOr(d.Auth, "/api/system", func(rel string) bool { return strings.HasPrefix(rel, "/logs/") }))
	group("/domains", domains.Mount, session)
	group("/logs", logs.Mount, sseOr(d.Auth, "/api/logs", func(rel string) bool { return strings.Contains(rel, "/stream/") }))
	group("/backup", backup.Mount, backupGuard(d))
	return r
}

// relPath is Express req.path inside a mount: the route path below prefix.
func relPath(r *http.Request, prefix string) string {
	rel := strings.TrimPrefix(httpx.RoutePath(r), prefix)
	if rel == "" {
		return "/"
	}
	return rel
}

// onlyUnder applies mw to requests at or below one of the given paths, the
// way app.use(path, mw) mounts a middleware.
func onlyUnder(mw func(http.Handler) http.Handler, paths ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		wrapped := mw(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := httpx.RoutePath(r)
			for _, prefix := range paths {
				if p == prefix || strings.HasPrefix(p, prefix+"/") {
					wrapped.ServeHTTP(w, r)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// githubPublicPrefixes are reachable without auth (webhooks, OAuth/App callbacks).
var githubPublicPrefixes = []string{"/webhook", "/callback", "/manifest/callback", "/setup"}

func githubGuard(session func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		protected := session(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rel := relPath(r, "/api/github")
			for _, p := range githubPublicPrefixes {
				if strings.HasPrefix(rel, p) {
					next.ServeHTTP(w, r)
					return
				}
			}
			protected.ServeHTTP(w, r)
		})
	}
}

// sseOr authenticates GET requests whose mount-relative path satisfies isSSE
// with the SSE query token, and everything else with the session JWT.
func sseOr(a *authsvc.Service, prefix string, isSSE func(rel string) bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		sse, session := a.RequireSSE(next), a.RequireSession(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if httpx.OriginalMethod(r) == http.MethodGet && isSSE(relPath(r, prefix)) {
				sse.ServeHTTP(w, r)
				return
			}
			session.ServeHTTP(w, r)
		})
	}
}

// backupGuard lets a fresh-install POST /restore-upload through with a valid
// setup token (validated, not consumed: backup consumes it after a successful
// restore); everything else needs the session JWT.
func backupGuard(d *app.Deps) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		session := d.Auth.RequireSession(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if httpx.OriginalMethod(r) == http.MethodPost && relPath(r, "/api/backup") == "/restore-upload" {
				token := strings.Join(r.Header.Values(authsvc.SetupTokenHeader), ", ")
				if d.Auth.Bootstrap().ValidateSetupToken(token) {
					d.Log.Info("[SECURITY] Restore-upload authorized via setup token (deferred consume)")
					next.ServeHTTP(w, r.WithContext(authsvc.WithSetupTokenAuth(r.Context())))
					return
				}
			}
			session.ServeHTTP(w, r)
		})
	}
}
