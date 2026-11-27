package deployments

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"

	"docklift/internal/certs"
	"docklift/internal/dnscheck"
	"docklift/internal/httpx"
	"docklift/internal/js"
	"docklift/internal/nginx"
	"docklift/internal/store"
)

// domainFormatMsg is the 400 of an invalid domain list.
const domainFormatMsg = "Invalid domain format. Must be valid domain names (e.g., example.com, app.example.com)."

// maxDNSChecks bounds the hostnames of one DNS preflight.
const maxDNSChecks = 10

// sslMap is the certificate report of every hostname of a domain field, in
// field order (sslMapForDomainString).
func (s *Service) sslMap(ctx context.Context, domain *string) (jsObject[certs.Report], error) {
	var out jsObject[certs.Report]
	for _, d := range nginx.DomainList(domain) {
		rep, err := s.certs.GetCertificateReport(ctx, d)
		if err != nil {
			return out, err
		}
		out.set(d, rep)
	}
	return out, nil
}

type sslBody struct {
	Success *bool                  `json:"success,omitempty"`
	SSL     jsObject[certs.Report] `json:"ssl"`
	Events  []certs.Event          `json:"events"`
}

func yes() *bool { t := true; return &t }

func (h *handlers) serviceSSL(w http.ResponseWriter, r *http.Request) {
	fail := func(err error) {
		h.d.Log.Error("get SSL status", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "Failed to get SSL status")
	}
	svc, err := h.serviceOf(r.Context(), r)
	if err != nil {
		fail(err)
		return
	}
	if svc == nil {
		httpx.Error(w, http.StatusNotFound, "Service not found")
		return
	}
	ssl, err := h.svc.sslMap(r.Context(), svc.Domain)
	if err != nil {
		fail(err)
		return
	}
	httpx.JSON(w, http.StatusOK, sslBody{SSL: ssl, Events: h.svc.certs.GetSSLEvents(nginx.DomainList(svc.Domain))})
}

func (h *handlers) dnsCheck(w http.ResponseWriter, r *http.Request) {
	svc, err := h.serviceOf(r.Context(), r)
	if err != nil {
		h.d.Log.Error("DNS check", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "DNS check failed")
		return
	}
	if svc == nil {
		httpx.Error(w, http.StatusNotFound, "Service not found")
		return
	}
	var candidates []string
	if arr, ok := httpx.BodyFrom(r).Get("domains").Raw().([]any); ok {
		for _, v := range arr[:min(maxDNSChecks, len(arr))] {
			candidates = append(candidates, js.String(v))
		}
	} else {
		candidates = nginx.DomainList(svc.Domain)
	}
	hosts := []string{}
	for _, c := range candidates {
		d := certs.NormalizeHost(c)
		if d != "" && !strings.Contains(d, ",") && nginx.ValidDomainList(d) && len(hosts) < maxDNSChecks {
			hosts = append(hosts, d)
		}
	}
	var (
		checks   []dnscheck.Check
		serverIP *string
		wg       sync.WaitGroup
	)
	wg.Add(2)
	go func() { defer wg.Done(); checks = h.svc.dns.CheckDomains(r.Context(), hosts) }()
	go func() { defer wg.Done(); serverIP = h.svc.dns.GetServerPublicIP(r.Context()) }()
	wg.Wait()
	if checks == nil {
		checks = []dnscheck.Check{}
	}
	httpx.JSON(w, http.StatusOK, struct {
		Checks   []dnscheck.Check `json:"checks"`
		ServerIP *string          `json:"serverIp"`
	}{checks, serverIP})
}

func (h *handlers) sslRetry(w http.ResponseWriter, r *http.Request) {
	fail := func(err error) {
		h.d.Log.Error("SSL retry", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "SSL retry failed")
	}
	svc, err := h.serviceOf(r.Context(), r)
	if err != nil {
		fail(err)
		return
	}
	if svc == nil {
		httpx.Error(w, http.StatusNotFound, "Service not found")
		return
	}
	if strVal(svc.Domain) == "" || strVal(svc.ContainerName) == "" {
		httpx.Error(w, http.StatusBadRequest, "Service has no domain or is not deployed")
		return
	}
	// Certificate issuance may outlive the request; it must not be cut off
	// by a client that disconnects.
	ctx := context.WithoutCancel(r.Context())
	c := h.svc.certs
	primaries := nginx.DomainList(svc.Domain)
	for _, d := range primaries {
		if err := c.ClearSSLMeta(ctx, d); err != nil {
			fail(err)
			return
		}
	}
	c.ClearSSLEvents(primaries)
	c.AppendSSLEvent(primaries, certs.LevelInfo, "Manual SSL retry requested.")
	target := *svc
	if strVal(target.Status) == "" {
		target.Status = store.Ptr("running")
	}
	if err := h.svc.edge.UpdateServiceDomain(ctx, target, nginx.UpdateOptions{ForceSSL: true}); err != nil {
		fail(err)
		return
	}
	ssl, err := h.svc.sslMap(ctx, svc.Domain)
	if err != nil {
		fail(err)
		return
	}
	httpx.JSON(w, http.StatusOK, sslBody{Success: yes(), SSL: ssl, Events: c.GetSSLEvents(primaries)})
}

// updateService changes a service's domains: validation, hostname
// ownership, the DB write, SSL activity, then the nginx vhost (with
// issuance) — rolling the DB back when nginx fails. A body without a
// `domain` field leaves the mapping unchanged; null or "" clears it
// (deliberate fix 5; TS cleared on a missing field).
func (h *handlers) updateService(w http.ResponseWriter, r *http.Request) {
	fail := func(err error) {
		h.d.Log.Error("update service", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "Failed to update service")
	}
	v := httpx.BodyFrom(r).Get("domain")
	domain, isStr := v.String()
	if v.Defined() && !v.IsNull() && !isStr {
		httpx.Error(w, http.StatusBadRequest, "Domain must be a comma-separated string")
		return
	}
	if domain != "" && !nginx.ValidDomainList(domain) {
		httpx.Error(w, http.StatusBadRequest, domainFormatMsg)
		return
	}
	ctx := context.WithoutCancel(r.Context())
	existing, err := h.serviceOf(ctx, r)
	if err != nil {
		fail(err)
		return
	}
	if existing == nil {
		httpx.Error(w, http.StatusNotFound, "Service not found or access denied")
		return
	}
	c := h.svc.certs
	if !v.Defined() {
		ssl, err := h.svc.sslMap(ctx, existing.Domain)
		if err != nil {
			fail(err)
			return
		}
		next := nginx.NormalizeDomainString(strVal(existing.Domain))
		httpx.JSON(w, http.StatusOK, sslBody{Success: yes(), SSL: ssl, Events: c.GetSSLEvents(next)})
		return
	}

	previous := nginx.DomainList(existing.Domain)
	next := nginx.NormalizeDomainString(domain)
	if err := h.svc.edge.AssertHostnamesAvailable(ctx, next, existing.ID); err != nil {
		var ce *nginx.ConflictError
		if errors.As(err, &ce) {
			httpx.Error(w, http.StatusConflict, ce.Message)
			return
		}
		fail(err)
		return
	}
	previousField := existing.Domain
	if _, err := h.d.Store.UpdateService(ctx, existing.ID, store.ServiceUpdate{Domain: store.Some(nginx.FormatDomainField(next))}); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "Service not found or access denied")
			return
		}
		fail(err)
		return
	}

	var dropped, added []string
	for _, d := range previous {
		if !slices.Contains(next, d) {
			dropped = append(dropped, d)
		}
	}
	for _, d := range next {
		if !slices.Contains(previous, d) {
			added = append(added, d)
		}
	}
	c.ClearSSLEvents(dropped)
	if len(added) > 0 {
		c.AppendSSLEvent(next, certs.LevelInfo, "Domain added: "+strings.Join(added, ", "))
	}

	svc, err := h.d.Store.GetService(ctx, existing.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		fail(err)
		return
	}
	switch {
	case svc != nil && strVal(svc.ContainerName) != "":
		if err := h.svc.edge.UpdateServiceDomain(ctx, *svc, nginx.UpdateOptions{}); err != nil {
			// Keep the DB aligned with live nginx when the write or reload failed.
			_, _ = h.d.Store.UpdateService(ctx, existing.ID, store.ServiceUpdate{Domain: store.Some(previousField)})
			fail(err)
			return
		}
	case len(next) > 0:
		c.AppendSSLEvent(next, certs.LevelWarn, "Service is not deployed yet — routing and HTTPS are set up on the next deploy.")
	}
	var current *string
	if svc != nil {
		current = svc.Domain
	}
	ssl, err := h.svc.sslMap(ctx, current)
	if err != nil {
		fail(err)
		return
	}
	httpx.JSON(w, http.StatusOK, sslBody{Success: yes(), SSL: ssl, Events: c.GetSSLEvents(next)})
}
