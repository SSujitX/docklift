// Package panelaccess is "Only allow access through the domain": while it is
// on, the backend serves only requests that came through docklift-nginx-proxy
// for a panel domain, and requests to the published dashboard port
// (http://SERVER_IP:8080) are redirected to https://<domain> or refused.
//
// Whether a request came through the proxy is decided from the TCP peer
// address alone (httpx.PeerTrusted: TRUSTED_PROXIES, by default loopback and
// the fixed addresses of docklift-nginx-proxy), never from a header. Clients
// of the published port reach the container from the Docker network gateway,
// which is never trusted, so this includes requests made on the host itself
// to 127.0.0.1:8080 or through an SSH tunnel; the escape hatch is the CLI
// (`docklift panel-access open`) inside the container.
//
// The state is the settings row SettingKey; its value is the panel domain
// access is restricted to, and no row means off. Guard keeps it in memory so
// the middleware never queries the database: writes through Guard update the
// cache at once, and Watch re-reads the row every RefreshInterval so a change
// made by another process (the CLI) reaches the running server within that
// interval.
package panelaccess

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"docklift/internal/certs"
	"docklift/internal/httpx"
	"docklift/internal/nginx"
)

// SettingKey is the settings row holding the domain access is restricted to.
const SettingKey = "panel_domain_only"

// RefreshInterval is how often a running server re-reads SettingKey, and so
// the longest a change made by `docklift panel-access` takes to apply.
const RefreshInterval = 2 * time.Second

// Settings is the settings table (implemented by *store.Store).
type Settings interface {
	GetSetting(ctx context.Context, key string) (string, bool, error)
	SetSetting(ctx context.Context, key, value string) error
	DeleteSetting(ctx context.Context, key string) error
}

// Panels reads panel-domain confs (implemented by *nginx.Service).
type Panels interface {
	PanelState(domain string) (nginx.PanelState, error)
}

// Guard owns the setting and its cached value. Safe for concurrent use.
type Guard struct {
	settings Settings
	panels   Panels
	log      *slog.Logger

	// mu serialises reading or writing the row together with updating
	// domain, so a refresh cannot overwrite a newer write with an older read.
	mu     sync.Mutex
	domain atomic.Pointer[string]
}

// New returns a Guard that is off until Load (or Boot) reads the setting.
func New(settings Settings, panels Panels, log *slog.Logger) *Guard {
	g := &Guard{settings: settings, panels: panels, log: log}
	g.domain.Store(new(string))
	return g
}

// Domain is the domain access is restricted to, "" when it is off. It reads
// only the cache.
func (g *Guard) Domain() string { return *g.domain.Load() }

func (g *Guard) set(domain string) { g.domain.Store(&domain) }

// Load reads the setting into the cache.
func (g *Guard) Load(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.loadLocked(ctx)
}

func (g *Guard) loadLocked(ctx context.Context) error {
	v, ok, err := g.settings.GetSetting(ctx, SettingKey)
	if err != nil {
		return err
	}
	if ok && !certs.IsValidHostname(v) {
		g.log.Warn("[panel-access] Ignoring an invalid domain in the settings — IP access is open again", "value", v)
		return g.disableLocked(ctx)
	}
	g.set(v)
	return nil
}

// Enable restricts access to domain. The caller has checked that domain is
// an HTTPS panel domain of the dashboard.
func (g *Guard) Enable(ctx context.Context, domain string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.settings.SetSetting(ctx, SettingKey, domain); err != nil {
		return err
	}
	g.set(domain)
	return nil
}

// Disable turns the restriction off and returns the domain it was
// restricted to ("" when it was already off).
func (g *Guard) Disable(ctx context.Context) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	was := g.Domain()
	return was, g.disableLocked(ctx)
}

func (g *Guard) disableLocked(ctx context.Context) error {
	if err := g.settings.DeleteSetting(ctx, SettingKey); err != nil {
		return err
	}
	g.set("")
	return nil
}

// Reconcile turns the restriction off when its domain is no longer an HTTPS
// panel domain of the dashboard (conf deleted, certificate gone), so the
// instance is never restricted to a domain that does not work. It runs after
// every panel-domain change and at boot.
func (g *Guard) Reconcile(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	domain := g.Domain()
	if domain == "" {
		return nil
	}
	st, err := g.panels.PanelState(domain)
	if err != nil {
		return err
	}
	if Usable(st) {
		return nil
	}
	g.log.Warn("[panel-access] " + domain + " is no longer an HTTPS panel domain — turned off \"Only allow access through the domain\"; the dashboard port is open again")
	return g.disableLocked(ctx)
}

// Boot loads the setting and reconciles it before the server listens.
func (g *Guard) Boot(ctx context.Context) error {
	if err := g.Load(ctx); err != nil {
		return err
	}
	if err := g.Reconcile(ctx); err != nil {
		return err
	}
	if domain := g.Domain(); domain != "" {
		g.log.Info("[panel-access] Dashboard access is restricted to https://" + domain)
	}
	return nil
}

// Watch re-reads the setting every interval until ctx ends, and turns it off
// when the conf of its domain disappears. Errors are logged and the cached
// value kept.
func (g *Guard) Watch(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := g.refresh(ctx); err != nil && ctx.Err() == nil {
				g.log.Warn("[panel-access] Could not re-read the setting", "error", err)
			}
		}
	}
}

// refresh is one Watch step. Only a missing conf turns the setting off here:
// a certificate retry rewrites the conf without :443 for a moment, and the
// retry reconciles itself once it is done.
func (g *Guard) refresh(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.loadLocked(ctx); err != nil {
		return err
	}
	domain := g.Domain()
	if domain == "" {
		return nil
	}
	st, err := g.panels.PanelState(domain)
	if err != nil {
		return err
	}
	if st.Exists {
		return nil
	}
	g.log.Warn("[panel-access] The conf of " + domain + " is gone — turned off \"Only allow access through the domain\"; the dashboard port is open again")
	return g.disableLocked(ctx)
}

// Usable reports whether access may be restricted to a panel domain in this
// state: its conf exists, proxies to the dashboard and serves HTTPS.
func Usable(st nginx.PanelState) bool { return st.Exists && st.Dashboard && st.HTTPS }

// ViaDomain reports whether r came through a trusted proxy over HTTPS for
// exactly domain (scheme and host as the proxy reports them).
func ViaDomain(r *http.Request, domain string) bool {
	if !httpx.PeerTrusted(r) {
		return false
	}
	o := httpx.RequestOrigin(r, httpx.RequestProtocol(r))
	return o != nil && o.Scheme == "https:" && o.Hostname == domain
}

// EnableBlocker is why r may not restrict access to domain, "" when it may.
func EnableBlocker(r *http.Request, domain string, st nginx.PanelState) string {
	switch {
	case !st.Exists:
		return "Domain configuration not found"
	case !st.Dashboard:
		return "This domain points to another port, not the Docklift dashboard"
	case !st.HTTPS:
		return "https://" + domain + " has no certificate yet. Wait until HTTPS is active"
	case !ViaDomain(r, domain):
		return "Open the dashboard at https://" + domain + " first, then turn this on"
	}
	return ""
}

// Status is the access state of one panel domain as the dashboard sees it.
type Status struct {
	// DomainOnly is true when access is restricted to this domain.
	DomainOnly bool `json:"domainOnly"`
	// CanEnable is true when this request may restrict access to it.
	CanEnable bool `json:"canEnable"`
	// Reason says why CanEnable is false.
	Reason string `json:"reason,omitempty"`
}

// StatusOf is the Status of domain (in state st) for the request r.
func (g *Guard) StatusOf(r *http.Request, domain string, st nginx.PanelState) Status {
	reason := EnableBlocker(r, domain, st)
	return Status{DomainOnly: g.Domain() == domain, CanEnable: reason == "", Reason: reason}
}
