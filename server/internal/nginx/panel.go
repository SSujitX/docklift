package nginx

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"docklift/internal/certs"
	"docklift/internal/store"
)

// ErrPanelDomainNotFound is returned for a panel domain without a conf.
var ErrPanelDomainNotFound = errors.New("Domain configuration not found")

// ConflictError reports a hostname that already belongs to another mapping
// (HTTP 409 in the routes).
type ConflictError struct{ Message string }

func (e *ConflictError) Error() string { return e.Message }

var (
	serverNameDirective = regexp.MustCompile(`server_name\s+([^;]+);`)
	panelPortLine       = regexp.MustCompile(`# docklift-panel-port:\s*(\d+)`)
	hostDockerPort      = regexp.MustCompile(`proxy_pass\s+http://host\.docker\.internal:(\d+);`)
)

// PanelConfPath is the conf of a panel domain (<NGINX_CONF_PATH>/<domain>.conf).
// The domain must be a valid hostname.
func (s *Service) PanelConfPath(domain string) (string, error) {
	if !certs.IsValidHostname(domain) {
		return "", &InvalidInputError{"domain", domain}
	}
	return filepath.Join(s.confDir, domain+".conf"), nil
}

// ParsePanelPort returns the panel port of a conf: the "# docklift-panel-port"
// line (default 8080) for dashboard confs, else the host.docker.internal port.
func ParsePanelPort(content string) (int, bool) {
	if strings.Contains(content, "docklift-nginx") || strings.Contains(content, "$dashboard") {
		if m := panelPortLine.FindStringSubmatch(content); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil {
				return n, true
			}
		}
		return DashboardPort, true
	}
	if m := hostDockerPort.FindStringSubmatch(content); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n, true
		}
	}
	return 0, false
}

// panelFile is a conf in the conf dir that looks like a panel domain.
type panelFile struct {
	name    string
	content string
}

// isPanelConf mirrors the filters of routes/domains.ts GET / and
// domainOwnership.ts panelHostnames.
func isPanelConf(name, content string) bool {
	if !strings.HasSuffix(name, ".conf") || name == "default.conf" || strings.HasPrefix(name, "service-") {
		return false
	}
	if !strings.Contains(content, PanelConfMarker) && !strings.Contains(content, "host.docker.internal") && !strings.Contains(content, "docklift-nginx") {
		return false
	}
	// A leftover service vhost under another name.
	return !(strings.Contains(content, "service-") && strings.Contains(content, "Main server block"))
}

func (s *Service) panelFiles() ([]panelFile, error) {
	if err := os.MkdirAll(s.confDir, 0o755); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.confDir)
	if err != nil {
		return nil, err
	}
	var out []panelFile
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".conf") || name == "default.conf" || strings.HasPrefix(name, "service-") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(s.confDir, name))
		if err != nil {
			return nil, err
		}
		if isPanelConf(name, string(raw)) {
			out = append(out, panelFile{name: name, content: string(raw)})
		}
	}
	return out, nil
}

// PanelDomain is one listed panel mapping.
type PanelDomain struct {
	Domain string
	Port   int
}

// ListPanelDomains returns the panel mappings (the first server_name of each
// panel conf with a known port), in file-name order. The route adds the SSL
// status of each domain.
func (s *Service) ListPanelDomains() ([]PanelDomain, error) {
	files, err := s.panelFiles()
	if err != nil {
		return nil, err
	}
	out := []PanelDomain{}
	for _, f := range files {
		m := serverNameDirective.FindStringSubmatch(f.content)
		port, ok := ParsePanelPort(f.content)
		if m == nil || !ok {
			continue
		}
		out = append(out, PanelDomain{Domain: strings.Fields(m[1])[0], Port: port})
	}
	return out, nil
}

// PanelDomainExists reports whether the domain has a panel conf.
func (s *Service) PanelDomainExists(domain string) (bool, error) {
	path, err := s.PanelConfPath(domain)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// WritePanelDomainConfig writes a panel conf (after `nginx -t`, see
// Reloader.ApplyConf). It does not reload.
func (s *Service) WritePanelDomainConfig(ctx context.Context, domain string, port int, enableHTTPS bool) error {
	path, err := s.PanelConfPath(domain)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.confDir, 0o755); err != nil {
		return err
	}
	content, err := BuildPanelConf(s.certs, domain, port, s.backendPort, enableHTTPS)
	if err != nil {
		return err
	}
	return s.reloader.ApplyConf(ctx, path, content)
}

// ProvisionPanelSSL writes the HTTP panel vhost, reloads, issues a
// certificate for exactly this hostname (a www mapping is separate) and
// switches to HTTPS once one covers it. Reload failures are returned; the
// certificate outcome is in the returned status.
func (s *Service) ProvisionPanelSSL(ctx context.Context, domain string, port int, force bool) (certs.CertificateStatus, error) {
	if err := s.WritePanelDomainConfig(ctx, domain, port, false); err != nil {
		return certs.CertificateStatus{}, err
	}
	if err := s.Reload(ctx); err != nil {
		return certs.CertificateStatus{}, err
	}
	st, err := s.certs.IssueCertificate(ctx, []string{domain}, certs.IssueOptions{Force: force})
	if err != nil {
		return certs.CertificateStatus{}, err
	}
	if st.Status == certs.StatusActive || st.Status == certs.StatusExpiring || s.certs.FindCoveringCertName([]string{domain}) != "" {
		if err := s.WritePanelDomainConfig(ctx, domain, port, true); err != nil {
			return certs.CertificateStatus{}, err
		}
		if err := s.Reload(ctx); err != nil {
			return certs.CertificateStatus{}, err
		}
	}
	return st, nil
}

// RetryPanelSSL re-provisions an existing panel domain with a forced
// certificate request (port from its conf, default 8080), after clearing the
// persisted SSL status. ErrPanelDomainNotFound when it has no conf.
func (s *Service) RetryPanelSSL(ctx context.Context, domain string) (certs.CertificateStatus, error) {
	path, err := s.PanelConfPath(domain)
	if err != nil {
		return certs.CertificateStatus{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return certs.CertificateStatus{}, ErrPanelDomainNotFound
		}
		return certs.CertificateStatus{}, err
	}
	port, ok := ParsePanelPort(string(raw))
	if !ok {
		port = DashboardPort
	}
	if err := s.certs.ClearSSLMeta(ctx, domain); err != nil {
		return certs.CertificateStatus{}, err
	}
	return s.ProvisionPanelSSL(ctx, domain, port, true)
}

// DeletePanelDomain removes a panel conf, clears its SSL status and reloads
// nginx. ErrPanelDomainNotFound when it has no conf; the conf is already gone
// when the reload fails.
func (s *Service) DeletePanelDomain(ctx context.Context, domain string) error {
	path, err := s.PanelConfPath(domain)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return ErrPanelDomainNotFound
		}
		return err
	}
	if err := s.certs.ClearSSLMeta(ctx, domain); err != nil {
		return err
	}
	s.log.Info("Deleted Nginx config for " + domain)
	return s.Reload(ctx)
}

// panelHostnames collects the server names of every panel conf (the file
// name when a conf has no server_name). An unreadable directory yields none.
func (s *Service) panelHostnames() map[string]bool {
	hosts := map[string]bool{}
	files, err := s.panelFiles()
	if err != nil {
		return hosts
	}
	for _, f := range files {
		if m := serverNameDirective.FindStringSubmatch(f.content); m != nil {
			for _, n := range strings.Fields(m[1]) {
				hosts[strings.ToLower(n)] = true
			}
		} else {
			hosts[strings.ToLower(strings.TrimSuffix(f.name, ".conf"))] = true
		}
	}
	return hosts
}

// NormalizeDomainList trims and lowercases hostnames, dropping empties and
// duplicates (domainOwnership.ts normalizeDomainList for an array).
func NormalizeDomainList(parts []string) []string { return certs.CleanHosts(parts) }

// NormalizeDomainString is NormalizeDomainList over a comma-separated string.
func NormalizeDomainString(s string) []string { return certs.CleanHosts(strings.Split(s, ",")) }

// FormatDomainField joins hostnames for the services.domain column; nil when
// there are none.
func FormatDomainField(hosts []string) *string {
	if len(hosts) == 0 {
		return nil
	}
	v := strings.Join(hosts, ", ")
	return &v
}

// ValidDomainList is isValidDomainList from deployments.ts: empty is valid,
// otherwise every non-empty comma-separated entry must match DOMAIN_REGEX.
func ValidDomainList(s string) bool {
	for _, d := range strings.Split(s, ",") {
		if d = certs.TrimJS(d); d != "" && !certs.IsValidHostname(d) {
			return false
		}
	}
	return true
}

// AssertHostnamesAvailable returns a *ConflictError when a hostname is
// already assigned to another service (excludeServiceID is skipped) or used
// by a panel domain.
func (s *Service) AssertHostnamesAvailable(ctx context.Context, hosts []string, excludeServiceID string) error {
	if len(hosts) == 0 {
		return nil
	}
	requested := map[string]bool{}
	for _, h := range hosts {
		requested[h] = true
	}
	yes := true
	services, err := s.store.ListServices(ctx, store.ServiceFilter{HasDomain: &yes}, store.OrderNone)
	if err != nil {
		return err
	}
	for _, svc := range services {
		if excludeServiceID != "" && svc.ID == excludeServiceID {
			continue
		}
		for _, h := range NormalizeDomainString(strVal(svc.Domain)) {
			if requested[h] {
				return &ConflictError{`Hostname "` + h + `" is already assigned to another service`}
			}
		}
	}
	panel := s.panelHostnames()
	for _, h := range hosts {
		if panel[h] {
			return &ConflictError{`Hostname "` + h + `" is already used by a panel domain`}
		}
	}
	return nil
}
