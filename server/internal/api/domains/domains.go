// Package domains serves /api/domains: panel (dashboard) custom domains with
// their Let's Encrypt certificates, DNS checks, SSL status and activity,
// "Only allow access through the domain" (internal/panelaccess) and the ACME
// account email.
//
// Every hostname from the URL or the body is trimmed, lowercased and checked
// against the hostname grammar before it reaches the nginx and certs
// services, which validate again before touching a file or running a
// program. Provisioning (writing the conf, reloading nginx, certbot) runs on
// a context derived from the server's background context: like the 2.x
// handlers it keeps going when the client disconnects, and stops on
// shutdown.
package domains

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"docklift/internal/app"
	"docklift/internal/certs"
	"docklift/internal/dnscheck"
	"docklift/internal/httpx"
	"docklift/internal/js"
	"docklift/internal/nginx"
	"docklift/internal/panelaccess"
)

// maxDNSCheckHosts is how many hostnames one DNS check looks at.
const maxDNSCheckHosts = 10

// maxEmailLength is the longest address the ACME email setting accepts
// (RFC 5321 forward-path limit).
const maxEmailLength = 254

const (
	msgInvalidFormat = "Invalid domain format"
	msgNotFound      = "Domain configuration not found"
)

type handlers struct {
	d *app.Deps

	mu    sync.Mutex
	locks map[string]*domainLock
}

// domainLock is a reference-counted mutex so the map holds only domains with
// an operation in flight.
type domainLock struct {
	sync.Mutex
	refs int
}

// Mount registers the /api/domains routes on r, which is already scoped to
// /api/domains and wrapped in the group's authentication middleware.
func Mount(r chi.Router, d *app.Deps) {
	h := &handlers{d: d, locks: map[string]*domainLock{}}
	r.Get("/ssl/email", h.getEmail)
	r.Put("/ssl/email", h.putEmail)
	r.Get("/", h.list)
	r.Post("/", h.add)
	r.Post("/dns-check", h.dnsCheck)
	r.Get("/{domain}/ssl", h.sslStatus)
	r.Post("/{domain}/ssl/retry", h.sslRetry)
	r.Put("/{domain}/access", h.setAccess)
	r.Delete("/{domain}", h.remove)
}

// lock serialises the mutating operations on one domain (conf write, reload,
// certificate run) and returns the unlock function.
func (h *handlers) lock(domain string) func() {
	h.mu.Lock()
	m, ok := h.locks[domain]
	if !ok {
		m = &domainLock{}
		h.locks[domain] = m
	}
	m.refs++
	h.mu.Unlock()
	m.Lock()
	return func() {
		m.Unlock()
		h.mu.Lock()
		if m.refs--; m.refs == 0 {
			delete(h.locks, domain)
		}
		h.mu.Unlock()
	}
}

// opContext is the context of a provisioning step: cancelled on server
// shutdown, not when the client goes away.
func (h *handlers) opContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(h.d.Background)
}

// domainParam is the :domain path parameter, decoded once
// and normalised like the 2.x handlers.
func domainParam(r *http.Request) string {
	raw := chi.URLParam(r, "domain")
	if r.URL.RawPath != "" {
		if decoded, err := url.PathUnescape(raw); err == nil {
			raw = decoded
		}
	}
	return certs.NormalizeHost(raw)
}

// report is the SSL status with its health fields, as returned to the API.
func (h *handlers) report(ctx context.Context, domain string) (certs.Report, error) {
	return h.d.Edge.Certs.GetCertificateReport(ctx, domain)
}

func reportOf(st certs.CertificateStatus) certs.Report {
	return certs.Report{CertificateStatus: st, Health: certs.HealthOf(st, time.Now())}
}

func (h *handlers) fail(w http.ResponseWriter, msg string, err error) {
	h.d.Log.Error(msg, slog.Any("error", err))
	httpx.Error(w, http.StatusInternalServerError, msg)
}

// failProvision answers a failed provisioning step: nginx reload errors keep
// their message (the dashboard shows it), anything else is logged and
// answered generically so file system paths never leave the server.
func (h *handlers) failProvision(w http.ResponseWriter, msg string, err error) {
	var re *nginx.ReloadError
	if errors.As(err, &re) {
		h.d.Log.Error(msg, slog.Any("error", err))
		httpx.Error(w, http.StatusInternalServerError, re.Error())
		return
	}
	h.fail(w, msg, err)
}

// GET /ssl/email — the ACME account email.
func (h *handlers) getEmail(w http.ResponseWriter, r *http.Request) {
	email, err := h.d.Edge.Certs.GetAcmeEmail(r.Context())
	if err != nil {
		email = ""
	}
	httpx.JSON(w, http.StatusOK, struct {
		Email string `json:"email"`
	}{email})
}

// PUT /ssl/email — save the ACME account email.
func (h *handlers) putEmail(w http.ResponseWriter, r *http.Request) {
	v := httpx.BodyFrom(r).Get("email")
	if v.Defined() && !v.IsNull() {
		if _, ok := v.String(); !ok {
			httpx.Error(w, http.StatusBadRequest, "Email must be a string")
			return
		}
	}
	email, _ := v.String()
	if len(email) > maxEmailLength || strings.ContainsAny(email, "\x00\r\n") {
		httpx.Error(w, http.StatusBadRequest, certs.ErrInvalidEmail.Error())
		return
	}
	if err := h.d.Edge.Certs.SetAcmeEmail(r.Context(), email); err != nil {
		if errors.Is(err, certs.ErrInvalidEmail) {
			httpx.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		h.fail(w, "Failed to save ACME email", err)
		return
	}
	httpx.JSON(w, http.StatusOK, struct {
		Success bool   `json:"success"`
		Email   string `json:"email"`
	}{true, strings.ToLower(js.Trim(email))})
}

type listedDomain struct {
	Domain string             `json:"domain"`
	Port   int                `json:"port"`
	SSL    certs.Report       `json:"ssl"`
	Access panelaccess.Status `json:"access"`
}

// access is the panel-access status of a listed domain for this request.
func (h *handlers) access(r *http.Request, domain string) (panelaccess.Status, error) {
	if !certs.IsValidHostname(domain) {
		return panelaccess.Status{Reason: msgInvalidFormat}, nil
	}
	st, err := h.d.Edge.Nginx.PanelState(domain)
	if err != nil {
		return panelaccess.Status{}, err
	}
	return h.d.PanelAccess.StatusOf(r, domain, st), nil
}

// GET / — panel domains with their SSL and access status.
func (h *handlers) list(w http.ResponseWriter, r *http.Request) {
	domains, err := h.d.Edge.Nginx.ListPanelDomains()
	if err != nil {
		h.fail(w, "Failed to list domains", err)
		return
	}
	out := make([]listedDomain, 0, len(domains))
	for _, p := range domains {
		ssl, err := h.report(r.Context(), p.Domain)
		if err != nil {
			h.fail(w, "Failed to list domains", err)
			return
		}
		access, err := h.access(r, p.Domain)
		if err != nil {
			h.fail(w, "Failed to list domains", err)
			return
		}
		out = append(out, listedDomain{Domain: p.Domain, Port: p.Port, SSL: ssl, Access: access})
	}
	httpx.JSON(w, http.StatusOK, out)
}

// parsePort is parsePanelPortInput: String(value ?? ”).trim() must be
// decimal digits within 1..65535. Booleans, arrays and objects never are.
func parsePort(v httpx.Value) (int, bool) {
	var digits string
	switch x := v.Raw().(type) {
	case string:
		digits = js.Trim(x)
	case json.Number:
		f, err := x.Float64()
		if err != nil || f != math.Trunc(f) || f < 1 || f > 65535 {
			return 0, false
		}
		return int(f), true
	default:
		return 0, false
	}
	for _, c := range digits {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(digits)
	if err != nil || n < 1 || n > 65535 {
		return 0, false
	}
	return n, true
}

type provisionBody struct {
	Success bool          `json:"success"`
	Domain  string        `json:"domain"`
	Port    *int          `json:"port,omitempty"`
	SSL     certs.Report  `json:"ssl"`
	Events  []certs.Event `json:"events"`
}

// events returns the activity of a hostname as a non-nil slice.
func (h *handlers) events(domain string) []certs.Event {
	ev := h.d.Edge.Certs.GetSSLEvents([]string{domain})
	if ev == nil {
		return []certs.Event{}
	}
	return ev
}

// POST / — add a panel domain and issue its certificate.
func (h *handlers) add(w http.ResponseWriter, r *http.Request) {
	body := httpx.BodyFrom(r)
	domainVal := body.Get("domain")
	domain, isString := domainVal.String()
	if domainVal.Defined() && !domainVal.IsNull() && !isString {
		domain = "?"
	}
	domain = certs.NormalizeHost(domain)
	port, portOK := parsePort(body.Get("port"))
	if domain == "" || !portOK {
		httpx.Error(w, http.StatusBadRequest, "Invalid domain or port")
		return
	}
	if !certs.IsValidHostname(domain) {
		httpx.Error(w, http.StatusBadRequest, msgInvalidFormat)
		return
	}

	ctx, cancel := h.opContext()
	defer cancel()
	unlock := h.lock(domain)
	defer unlock()

	if err := h.d.Edge.Nginx.AssertHostnamesAvailable(ctx, []string{domain}, ""); err != nil {
		var conflict *nginx.ConflictError
		if errors.As(err, &conflict) {
			httpx.Error(w, http.StatusConflict, conflict.Message)
			return
		}
		h.fail(w, "Failed to add domain", err)
		return
	}
	st, err := h.d.Edge.Nginx.ProvisionPanelSSL(ctx, domain, port, false)
	if err != nil {
		h.failProvision(w, "Failed to add domain", err)
		return
	}
	httpx.JSON(w, http.StatusOK, provisionBody{Success: true, Domain: domain, Port: &port, SSL: reportOf(st), Events: h.events(domain)})
}

// POST /dns-check — do panel hostnames resolve to this server?
func (h *handlers) dnsCheck(w http.ResponseWriter, r *http.Request) {
	hosts := []string{}
	if items, ok := httpx.BodyFrom(r).Get("domains").Raw().([]any); ok {
		for _, item := range items {
			s, ok := item.(string)
			if !ok {
				continue
			}
			if host := certs.NormalizeHost(s); certs.IsValidHostname(host) {
				hosts = append(hosts, host)
			}
		}
	}
	if len(hosts) == 0 {
		httpx.Error(w, http.StatusBadRequest, "Provide at least one domain")
		return
	}
	hosts = hosts[:min(len(hosts), maxDNSCheckHosts)]
	ctx := r.Context()
	// The public IP is resolved first: the per-host checks compare against the
	// cached value, which concurrent lookups would not see yet.
	serverIP := h.d.Edge.DNS.GetServerPublicIP(ctx)
	results := h.d.Edge.DNS.CheckDomains(ctx, hosts)
	httpx.JSON(w, http.StatusOK, struct {
		Checks   []dnscheck.Check `json:"checks"`
		ServerIP *string          `json:"serverIp"`
	}{results, serverIP})
}

// validDomainParam reads :domain and answers 400 for a malformed hostname.
func validDomainParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	domain := domainParam(r)
	if !certs.IsValidHostname(domain) {
		httpx.Error(w, http.StatusBadRequest, msgInvalidFormat)
		return "", false
	}
	return domain, true
}

// GET /:domain/ssl — status and Let's Encrypt activity of one hostname.
func (h *handlers) sslStatus(w http.ResponseWriter, r *http.Request) {
	domain, ok := validDomainParam(w, r)
	if !ok {
		return
	}
	exists, err := h.d.Edge.Nginx.PanelDomainExists(domain)
	if err != nil {
		h.fail(w, "Failed to read SSL status", err)
		return
	}
	if !exists {
		httpx.Error(w, http.StatusNotFound, msgNotFound)
		return
	}
	ssl, err := h.report(r.Context(), domain)
	if err != nil {
		h.fail(w, "Failed to read SSL status", err)
		return
	}
	httpx.JSON(w, http.StatusOK, struct {
		SSL    certs.Report  `json:"ssl"`
		Events []certs.Event `json:"events"`
	}{ssl, h.events(domain)})
}

// POST /:domain/ssl/retry — re-issue the certificate of a panel domain.
func (h *handlers) sslRetry(w http.ResponseWriter, r *http.Request) {
	domain, ok := validDomainParam(w, r)
	if !ok {
		return
	}
	ctx, cancel := h.opContext()
	defer cancel()
	unlock := h.lock(domain)
	defer unlock()

	st, err := h.d.Edge.Nginx.RetryPanelSSL(ctx, domain)
	// A retry that ends without HTTPS must not leave access restricted to it.
	if rerr := h.d.PanelAccess.Reconcile(ctx); rerr != nil {
		h.fail(w, "SSL retry failed", rerr)
		return
	}
	if err != nil {
		if errors.Is(err, nginx.ErrPanelDomainNotFound) {
			httpx.Error(w, http.StatusNotFound, msgNotFound)
			return
		}
		h.failProvision(w, "SSL retry failed", err)
		return
	}
	httpx.JSON(w, http.StatusOK, provisionBody{Success: true, Domain: domain, SSL: reportOf(st), Events: h.events(domain)})
}

// DELETE /:domain — remove a panel domain.
func (h *handlers) remove(w http.ResponseWriter, r *http.Request) {
	domain, ok := validDomainParam(w, r)
	if !ok {
		return
	}
	ctx, cancel := h.opContext()
	defer cancel()
	unlock := h.lock(domain)
	defer unlock()

	err := h.d.Edge.Nginx.DeletePanelDomain(ctx, domain)
	// The conf is gone even when the reload failed: never leave access
	// restricted to a domain that no longer exists.
	if rerr := h.d.PanelAccess.Reconcile(ctx); rerr != nil {
		h.fail(w, "Failed to delete domain", rerr)
		return
	}
	if err != nil {
		if errors.Is(err, nginx.ErrPanelDomainNotFound) {
			httpx.Error(w, http.StatusNotFound, msgNotFound)
			return
		}
		h.fail(w, "Failed to delete domain", err)
		return
	}
	httpx.JSON(w, http.StatusOK, struct {
		Success bool `json:"success"`
	}{true})
}

// PUT /:domain/access — turn "Only allow access through the domain" on
// ({"domainOnly": true, "password": ...}) or off ({"domainOnly": false}).
// Turning it on needs an HTTPS panel domain of the dashboard, a request that
// came through the proxy over HTTPS for exactly that domain (so the admin
// cannot lock themselves out of the page they are on) and the step-up
// password. Turning it off needs only the session.
func (h *handlers) setAccess(w http.ResponseWriter, r *http.Request) {
	const failMsg = "Failed to change dashboard access"
	domain, ok := validDomainParam(w, r)
	if !ok {
		return
	}
	on, isBool := httpx.BodyFrom(r).Get("domainOnly").Raw().(bool)
	if !isBool {
		httpx.Error(w, http.StatusBadRequest, "domainOnly must be true or false")
		return
	}
	ctx, cancel := h.opContext()
	defer cancel()
	unlock := h.lock(domain)
	defer unlock()

	st, err := h.d.Edge.Nginx.PanelState(domain)
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}
	if !st.Exists {
		httpx.Error(w, http.StatusNotFound, msgNotFound)
		return
	}
	switch {
	case on:
		if reason := panelaccess.EnableBlocker(r, domain, st); reason != "" {
			httpx.Error(w, http.StatusConflict, reason)
			return
		}
		ok, err := h.d.Auth.RequireStepUpPassword(w, r)
		if err != nil {
			h.fail(w, failMsg, err)
			return
		}
		if !ok {
			return
		}
		if err := h.d.PanelAccess.Enable(ctx, domain); err != nil {
			h.fail(w, failMsg, err)
			return
		}
		h.d.Log.Info("[AUDIT] Dashboard access restricted to https://" + domain + " from IP: " + httpx.ClientIP(r))
	case h.d.PanelAccess.Domain() == domain:
		if _, err := h.d.PanelAccess.Disable(ctx); err != nil {
			h.fail(w, failMsg, err)
			return
		}
		h.d.Log.Info("[AUDIT] Dashboard access by IP reopened from IP: " + httpx.ClientIP(r))
	}
	httpx.JSON(w, http.StatusOK, struct {
		Success bool               `json:"success"`
		Domain  string             `json:"domain"`
		Access  panelaccess.Status `json:"access"`
	}{true, domain, h.d.PanelAccess.StatusOf(r, domain, st)})
}
