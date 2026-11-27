package nginx

import (
	"context"
	"log/slog"

	"docklift/internal/certs"
	"docklift/internal/config"
	"docklift/internal/dnscheck"
	"docklift/internal/store"
)

// Stack is the process-wide set of edge services. Create exactly one (the
// SSL activity log and the public-IP cache live in it) and share it between
// the domains and deployments route groups.
type Stack struct {
	DNS   *dnscheck.Checker
	Certs *certs.Service
	Nginx *Service
}

// NewStack wires the edge services from the process configuration.
func NewStack(cfg *config.Config, st *store.Store, log *slog.Logger) *Stack {
	dns := dnscheck.New()
	runner := certs.ExecRunner{}
	c := certs.New(certs.SettingsFrom(cfg), st, dns, log, certs.WithRunner(runner))
	n := New(Options{
		ConfDir:        cfg.NginxConfPath,
		ProxyContainer: cfg.NginxProxyContainer,
		BackendPort:    cfg.Port,
		Runner:         runner,
		Log:            log,
	}, c, st)
	return &Stack{DNS: dns, Certs: c, Nginx: n}
}

// Boot is the after-listen startup step: the panel upstream migration, then
// syncNginxConfigs. A migration failure is logged and does not stop the sync.
func (s *Stack) Boot(ctx context.Context) error {
	if _, err := s.Nginx.MigratePanelUpstreams(ctx); err != nil {
		s.Nginx.log.Error("Panel domain upstream migration failed", "error", err)
	}
	return s.Nginx.SyncConfigs(ctx)
}

// StartWatchers starts the certificate renew watcher (reloading nginx-proxy
// after renewals) until ctx is cancelled.
func (s *Stack) StartWatchers(ctx context.Context) <-chan struct{} {
	return s.Certs.StartCertRenewWatcher(ctx, s.Nginx.Reload)
}
