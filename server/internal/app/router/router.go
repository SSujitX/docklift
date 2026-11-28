// Package router assembles the HTTP handler: the middleware stack and mount
// table, the WebSocket terminal and the embedded dashboard.
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

// Rate-limit settings.
const (
	authWindow = 15 * time.Minute
	authLimit  = 100
	setupLimit = 10
	// sessionCheckLimit is the budget of sessionCheckPaths: they only answer
	// for an already valid session, so they cannot be used to guess a
	// password, and the dashboard calls them on every page load and stream.
	sessionCheckLimit = 1000
	// publicBodyLimit bounds the bodies of unauthenticated routes (login,
	// register, the GitHub callbacks); none needs more than a few hundred bytes.
	publicBodyLimit   = 64 << 10
	publicBodyTimeout = 30 * time.Second
)

// sessionCheckPaths are the cheap authenticated checks exempt from the
// shared auth limiter (a shared address, such as an office NAT, would lock
// every user out of the dashboard); they have their own generous limiter.
var sessionCheckPaths = []string{"/api/auth/me", "/api/auth/sse-token", "/api/auth/terminal-token"}

// publicBody parses the small bodies of unauthenticated routes, after their
// rate limiter, with the budget reserved for anonymous clients.
var publicBody = httpx.ParseBodyWith(httpx.BodyOptions{
	Limit: publicBodyLimit, Budget: httpx.PublicBodyBudget, ReadTimeout: publicBodyTimeout,
})

// New returns the root handler. Limiter sweepers stop when d.Background ends.
func New(d *app.Deps) http.Handler {
	root := chi.NewRouter()
	// Peer trust is decided first: everything below (terminal origin check,
	// HSTS, CORS, rate limits) reads forwarded headers only from trusted proxies.
	// The panel-access guard comes right after it, before the terminal
	// upgrade, every guard and body parser: while access is restricted to the
	// panel domain, requests that did not come through the proxy stop there.
	trust := httpx.NewTrustedProxies(d.Config.TrustedProxies.Trusted, d.Config.TrustedProxies.Excluded)
	root.Use(httpx.Recover, trust.Middleware, d.PanelAccess.Middleware, httpx.HeadAsGet)
	root.NotFound(httpx.NotFound)
	root.MethodNotAllowed(httpx.NotFound)

	// The upgrade listener handles /ws/terminal outside the HTTP routes.
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
	// StripSlashes changes only the routing path, so as in 2.x (strict
	// routing off) /api/health/ reaches /api/health while the gate, limiters
	// and guards still see the request path as sent. Bodies are parsed only
	// inside each group, after its rate limiter and auth guard, so an
	// unauthenticated client cannot make the server buffer or decode a body.
	r.Use(middleware.StripSlashes, httpx.MaintenanceGate(d.Maintenance))

	health.Mount(r, d)

	authLimiter := httpx.NewRateLimiter(d.Background, httpx.RateLimitConfig{
		Window:  authWindow,
		Limit:   authLimit,
		Message: "Too many attempts, please try again later",
		Skip: func(r *http.Request) bool {
			p := httpx.RoutePath(r)
			return p == "/api/auth/status" || under(p, sessionCheckPaths)
		},
	})
	sessionCheckLimiter := httpx.NewRateLimiter(d.Background, httpx.RateLimitConfig{
		Window:  authWindow,
		Limit:   sessionCheckLimit,
		Message: "Too many requests, please try again later",
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
	group("/auth", auth.Mount, onlyUnder(setupLimiter.Middleware, "/api/auth/register", "/api/auth/setup-token"), authLimiter.Middleware,
		onlyUnder(sessionCheckLimiter.Middleware, sessionCheckPaths...), publicBody)

	// One GitHub App (installation token cache) and one deploy service
	// (locks, auto-deploy queue, shutdown wait) serve every group.
	gh := github.NewApp(d)
	deployer := deployments.For(d)
	deployer.SetGitHubApp(gh)

	session, body := d.Auth.RequireSession, httpx.ParseBody
	group("/projects", func(r chi.Router, d *app.Deps) {
		projects.MountWith(r, d, projects.Options{GitHub: gh})
	}, session, body)
	group("/deployments", func(r chi.Router, d *app.Deps) {
		deployments.MountWith(r, d, deployer)
	}, session, body)
	group("/databases", databases.Mount, session, body)
	group("/files", files.Mount, session, body)
	group("/ports", ports.Mount, session, body)
	group("/github", func(r chi.Router, d *app.Deps) {
		github.MountWith(r, d, github.Options{App: gh, Deployer: deployer})
	}, githubGuard(session, body))
	group("/system", system.Mount, sseOr(d.Auth, "/api/system", func(rel string) bool { return strings.HasPrefix(rel, "/logs/") }), body)
	group("/domains", domains.Mount, session, body)
	group("/logs", logs.Mount, sseOr(d.Auth, "/api/logs", func(rel string) bool { return strings.Contains(rel, "/stream/") }), body)
	group("/backup", backup.Mount, backupGuard(d), body)
	return r
}

// relPath is the request path inside a mount: the route path below prefix.
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
			if under(httpx.RoutePath(r), paths) {
				wrapped.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// under reports whether p is at or below one of paths.
func under(p string, paths []string) bool {
	for _, prefix := range paths {
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			return true
		}
	}
	return false
}

// githubPublicPrefixes are reachable without auth (webhooks, OAuth/App callbacks).
var githubPublicPrefixes = []string{"/webhook", "/callback", "/manifest/callback", "/setup"}

// githubGuard authenticates everything but the public sub-paths and then
// parses the body with body. Public paths get only publicBody, except the
// webhook: it reads its raw body itself once the request carries a
// signature, and decodes it only after the signature is verified.
func githubGuard(session, body func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		protected, public := session(body(next)), publicBody(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rel := relPath(r, "/api/github")
			if strings.HasPrefix(rel, "/webhook") {
				next.ServeHTTP(w, r)
				return
			}
			for _, p := range githubPublicPrefixes {
				if strings.HasPrefix(rel, p) {
					public.ServeHTTP(w, r)
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
					// Decided before the restore lock and the upload: the
					// token is worthless once any user exists, and an
					// unreadable user count refuses it (fail closed).
					users, err := d.Store.CountUsers(r.Context())
					if err != nil {
						d.Log.Error("setup-token restore: count users", "error", err)
						w.Header().Set("Connection", "close")
						httpx.Error(w, http.StatusServiceUnavailable, "Could not verify setup state, please try again")
						return
					}
					if users > 0 {
						w.Header().Set("Connection", "close")
						httpx.Error(w, http.StatusForbidden, "Setup-token restore is only allowed before the first admin exists")
						return
					}
					d.Log.Info("[SECURITY] Restore-upload authorized via setup token (deferred consume)")
					next.ServeHTTP(w, r.WithContext(authsvc.WithSetupTokenAuth(r.Context())))
					return
				}
			}
			session.ServeHTTP(w, r)
		})
	}
}
