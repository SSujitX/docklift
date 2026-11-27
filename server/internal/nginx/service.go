package nginx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"docklift/internal/certs"
	"docklift/internal/store"
)

// ErrCancelled is returned when UpdateOptions.ShouldContinue reports false;
// the message matches the TypeScript "Deployment cancelled".
var ErrCancelled = errors.New("Deployment cancelled")

// ServiceStore is the subset of *store.Store this package uses.
type ServiceStore interface {
	ListServices(ctx context.Context, f store.ServiceFilter, order store.Order) ([]*store.Service, error)
}

// ContainerInspector reports whether a container is running. It is only used
// for a log line; nil skips the check.
type ContainerInspector interface {
	ContainerRunning(ctx context.Context, name string) (bool, error)
}

// Options configure a Service.
type Options struct {
	// ConfDir is NGINX_CONF_PATH (mounted as nginx-proxy's conf.d).
	ConfDir string
	// ProxyContainer is the nginx-proxy container (NGINX_PROXY_CONTAINER).
	ProxyContainer string
	// BackendPort is the Go backend's PORT, the panel upstream port.
	BackendPort int
	// Runner runs docker; nil uses certs.ExecRunner.
	Runner certs.Runner
	// Containers is optional (see ContainerInspector).
	Containers ContainerInspector
	Log        *slog.Logger
}

// Service manages nginx-proxy confs. Safe for concurrent use as far as the
// file system allows; callers serialise per service (deployment locks).
type Service struct {
	confDir     string
	backendPort int
	certs       *certs.Service
	store       ServiceStore
	containers  ContainerInspector
	reloader    *Reloader
	log         *slog.Logger
}

// New returns a Service.
func New(opts Options, c *certs.Service, st ServiceStore) *Service {
	log := opts.Log
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	runner := opts.Runner
	if runner == nil {
		runner = certs.ExecRunner{}
	}
	return &Service{
		confDir:     opts.ConfDir,
		backendPort: opts.BackendPort,
		certs:       c,
		store:       st,
		containers:  opts.Containers,
		reloader:    &Reloader{runner: runner, container: opts.ProxyContainer, confDir: opts.ConfDir, log: log},
		log:         log,
	}
}

// Reload reloads nginx-proxy (see Reloader.Reload). Errors are *ReloadError.
func (s *Service) Reload(ctx context.Context) error { return s.reloader.Reload(ctx) }

// ServiceConfPath is the conf file of a service.
func (s *Service) ServiceConfPath(serviceID string) (string, error) {
	if !serviceIDPattern.MatchString(serviceID) {
		return "", &InvalidInputError{"service id", serviceID}
	}
	return filepath.Join(s.confDir, "service-"+serviceID+".conf"), nil
}

// UpdateOptions tune UpdateServiceDomain.
type UpdateOptions struct {
	// SkipIssue writes the vhost with existing certificates only (no
	// certbot run). The TypeScript default issues.
	SkipIssue bool
	// ForceSSL forces a certificate re-issue (manual retry).
	ForceSSL bool
	// ShouldContinue is a cooperative cancel checked before every write,
	// reload and certificate step; false aborts with ErrCancelled.
	ShouldContinue func(ctx context.Context) bool
}

func strVal(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// DomainList splits a comma-separated domain field into trimmed, lowercased,
// non-empty hostnames (no de-duplication; deployments.ts domainList).
func DomainList(domain *string) []string {
	var out []string
	for _, d := range strings.Split(strVal(domain), ",") {
		if h := certs.NormalizeHost(d); h != "" {
			out = append(out, h)
		}
	}
	return out
}

// removeConf deletes a conf and reloads when it exists.
func (s *Service) removeConf(ctx context.Context, path string, check func() error) error {
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	if err := check(); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	return s.Reload(ctx)
}

// UpdateServiceDomain writes (or removes) the vhost of a service. A service
// gets a conf only while it has a domain and a container and is running or
// starting. The HTTP vhost is written and nginx reloaded first, so the ACME
// challenge path is live (existing HTTPS — partial is fine — is kept so adding
// a SAN does not downgrade HSTS hosts); then, unless SkipIssue, a certificate
// is issued and the vhost rewritten with HTTPS. Write and reload failures are
// returned (callers must not report the domain as saved); certbot failures
// are soft (recorded in the SSL activity log and status) unless the HTTPS
// reload fails.
func (s *Service) UpdateServiceDomain(ctx context.Context, svc store.Service, opts UpdateOptions) error {
	check := func() error {
		if opts.ShouldContinue != nil && !opts.ShouldContinue(ctx) {
			return ErrCancelled
		}
		return nil
	}
	if err := os.MkdirAll(s.confDir, 0o755); err != nil {
		return err
	}
	confPath, err := s.ServiceConfPath(svc.ID)
	if err != nil {
		return err
	}

	status := strVal(svc.Status)
	container := strVal(svc.ContainerName)
	if strVal(svc.Domain) == "" || container == "" || (status != "running" && status != "starting") {
		return s.removeConf(ctx, confPath, check)
	}

	if s.containers != nil {
		running, err := s.containers.ContainerRunning(ctx, container)
		switch {
		case err != nil:
			s.log.Warn(fmt.Sprintf("Nginx config for %s: could not inspect container %s — config kept.", svc.Name, container))
		case !running && status != "starting":
			s.log.Warn(fmt.Sprintf("Nginx config for %s: container %s not running yet — config kept for when it starts.", svc.Name, container))
		}
	}
	if err := check(); err != nil {
		return err
	}

	domains := DomainList(svc.Domain)
	if len(domains) == 0 {
		return s.removeConf(ctx, confPath, check)
	}
	for _, d := range domains {
		if !certs.IsValidHostname(d) {
			return &InvalidInputError{"domain", d}
		}
	}
	if svc.InternalPort == nil {
		return &InvalidInputError{"internal port", "null"}
	}

	// Configure only hostnames the user entered (never an implicit www: a
	// missing www record would fail the whole certificate order).
	primary := domains[0]
	serverNames := strings.Join(domains, " ")
	loc, err := BuildServiceProxyLocation(svc.ID, container, *svc.InternalPort)
	if err != nil {
		return err
	}
	blocks := ServerBlocks{
		Comment:       "Main server block for " + svc.Name,
		ServerNames:   serverNames,
		PrimaryDomain: primary,
		ProxyLocation: loc,
		EnableHTTPS:   s.certs.FindCoveringCertName(domains) != "" || s.certs.FindBestExpandableCertName(domains) != "",
	}

	if err := s.writeAndReload(ctx, confPath, blocks, check); err != nil {
		if errors.Is(err, ErrCancelled) {
			return err
		}
		s.log.Error("Failed to write Nginx config", "error", err)
		s.certs.AppendSSLEvent(domains, certs.LevelError, "Failed to write the nginx vhost — check backend logs.")
		return err
	}
	s.certs.AppendSSLEvent(domains, certs.LevelInfo, "HTTP vhost written and nginx reloaded — ACME challenge path is live.")

	if opts.SkipIssue {
		s.log.Info(fmt.Sprintf("Updated Nginx config for %s (%s)", svc.Name, serverNames))
		return nil
	}
	err = s.issueAndEnableHTTPS(ctx, svc.Name, confPath, blocks, domains, opts.ForceSSL, check)
	if err == nil || errors.Is(err, ErrCancelled) {
		return err
	}
	s.log.Error("SSL issue failed for "+primary, "error", err)
	s.certs.AppendSSLEvent(domains, certs.LevelError, err.Error())
	// Propagate nginx reload failures; soft-fail pure issuance errors.
	if IsReloadError(err) {
		return err
	}
	return nil
}

// writeAndReload renders, applies and reloads one vhost.
func (s *Service) writeAndReload(ctx context.Context, path string, b ServerBlocks, check func() error) error {
	content, err := BuildHTTPHTTPSServers(s.certs, b)
	if err != nil {
		return err
	}
	if err := check(); err != nil {
		return err
	}
	if err := s.reloader.ApplyConf(ctx, path, content); err != nil {
		return err
	}
	if err := check(); err != nil {
		return err
	}
	return s.Reload(ctx)
}

func (s *Service) issueAndEnableHTTPS(ctx context.Context, name, path string, b ServerBlocks, domains []string, force bool, check func() error) error {
	if err := check(); err != nil {
		return err
	}
	st, err := s.certs.IssueCertificate(ctx, domains, certs.IssueOptions{Force: force})
	if err != nil {
		return err
	}
	if err := check(); err != nil {
		return err
	}
	// Rewrite with HTTPS when active (including certificates still named
	// under a previous primary).
	if st.Status != certs.StatusActive && st.Status != certs.StatusExpiring && s.certs.FindCoveringCertName(domains) == "" {
		msg := ""
		if st.Error != nil {
			msg = *st.Error
		}
		s.log.Warn(fmt.Sprintf("SSL not active for %s: %s %s", b.PrimaryDomain, st.Status, msg))
		s.certs.AppendSSLEvent(domains, certs.LevelWarn, "Still serving plain HTTP — HTTPS turns on automatically once a certificate exists.")
		return nil
	}
	b.EnableHTTPS = true
	if err := s.writeAndReload(ctx, path, b, check); err != nil {
		if IsReloadError(err) {
			s.certs.AppendSSLEvent(domains, certs.LevelError, "Certificate may exist but nginx reload failed — HTTPS vhost not live.")
		}
		return err
	}
	s.log.Info(fmt.Sprintf("Updated Nginx+SSL config for %s (%s) status=%s", name, b.ServerNames, st.Status))
	s.certs.AppendSSLEvent(domains, certs.LevelSuccess, "HTTPS enabled for "+b.ServerNames+".")
	return nil
}

// CleanupServiceDomain removes a service's conf and reloads nginx. Failures
// are logged, not returned (as in TS).
func (s *Service) CleanupServiceDomain(ctx context.Context, serviceID string) {
	path, err := s.ServiceConfPath(serviceID)
	if err != nil {
		s.log.Error("Failed to remove Nginx config for service "+serviceID, "error", err)
		return
	}
	if _, err := os.Stat(path); err != nil {
		return
	}
	if err := os.Remove(path); err != nil {
		s.log.Error("Failed to remove Nginx config for service "+serviceID, "error", err)
		return
	}
	s.log.Info("Removed Nginx config for service " + serviceID)
	if err := s.Reload(ctx); err != nil {
		s.log.Error("Failed to remove Nginx config for service "+serviceID, "error", err)
	}
}

var serviceConfName = regexp.MustCompile(`^service-(.+)\.conf$`)

// SyncConfigs restores missing confs of running services with domains (reusing
// existing certificates, issuing once only when none covers the hosts) and
// removes confs of services that no longer exist or have no domain, reloading
// nginx if anything was removed. It is the boot-time syncNginxConfigs.
func (s *Service) SyncConfigs(ctx context.Context) error {
	entries, err := os.ReadDir(s.confDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	fileIDs := map[string]bool{}
	var order []string
	for _, e := range entries {
		if m := serviceConfName.FindStringSubmatch(e.Name()); m != nil && !fileIDs[m[1]] {
			fileIDs[m[1]] = true
			order = append(order, m[1])
		}
	}

	yes := true
	known, err := s.store.ListServices(ctx, store.ServiceFilter{HasDomain: &yes}, store.OrderNone)
	if err != nil {
		return err
	}
	knownIDs := map[string]bool{}
	for _, k := range known {
		knownIDs[k.ID] = true
	}
	active, err := s.store.ListServices(ctx, store.ServiceFilter{HasDomain: &yes, HasContainerName: &yes, StatusIn: []string{"running", "starting"}}, store.OrderNone)
	if err != nil {
		return err
	}
	for _, svc := range active {
		if fileIDs[svc.ID] {
			continue
		}
		s.log.Info("Restoring missing Nginx config for " + svc.Name)
		// Avoid hammering Let's Encrypt on every restart: reuse certificates.
		if err := s.UpdateServiceDomain(ctx, *svc, UpdateOptions{SkipIssue: true}); err != nil {
			return err
		}
		if hosts := DomainList(svc.Domain); len(hosts) > 0 && s.certs.FindCoveringCertName(hosts) == "" {
			if err := s.UpdateServiceDomain(ctx, *svc, UpdateOptions{}); err != nil {
				return err
			}
		}
	}

	changed := false
	for _, id := range order {
		if knownIDs[id] {
			continue
		}
		file := "service-" + id + ".conf"
		s.log.Info("Removing invalid/orphaned Nginx config: " + file)
		if err := os.Remove(filepath.Join(s.confDir, file)); err != nil {
			return err
		}
		changed = true
	}
	if !changed {
		s.log.Info("Nginx configs are in sync.")
		return nil
	}
	return s.Reload(ctx)
}
