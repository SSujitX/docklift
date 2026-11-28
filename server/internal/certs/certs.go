// Package certs manages Let's Encrypt
// certificates issued by the certbot sidecar (webroot HTTP-01), with the PEM
// files under LETSENCRYPT_PATH/live/<name>/ as the source of truth.
//
// It selects which certbot lineage serves a set of hostnames, builds the
// certbot command line, runs a DNS preflight, keeps an in-memory activity log
// per hostname for the UI, persists issuance status in the settings table
// (ssl_meta_<primary>) and watches the PEMs so nginx reloads after renewals.
package certs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"docklift/internal/config"
	"docklift/internal/dnscheck"
	"docklift/internal/js"
	"docklift/internal/store"
)

// SslStatus is the state of a hostname's certificate.
type SslStatus string

const (
	StatusMissing  SslStatus = "missing"
	StatusPending  SslStatus = "pending"
	StatusActive   SslStatus = "active"
	StatusExpiring SslStatus = "expiring"
	StatusExpired  SslStatus = "expired"
	StatusFailed   SslStatus = "failed"
)

// CertificateStatus is the SSL status the API returns for a hostname. Its JSON
// is exactly six keys, null for absent values.
type CertificateStatus struct {
	Status            SslStatus `json:"status"`
	Domain            string    `json:"domain"`
	ExpiresAt         *string   `json:"expiresAt"`
	Error             *string   `json:"error"`
	DiagnosticCommand *string   `json:"diagnosticCommand"`
	CertPath          *string   `json:"certPath"`
}

const (
	// ExpiringWindow is when status turns "expiring" (21 days, as in 2.x).
	ExpiringWindow = 21 * 24 * time.Hour
	// RenewalOverdueWindow: certbot renews 30 days before expiry, so a
	// certificate with less than 20 days left has failed to renew for at
	// least ten days and needs attention.
	RenewalOverdueWindow = 20 * 24 * time.Hour

	statusKeyPrefix = "ssl_meta_"
	acmeEmailKey    = "ssl_acme_email"
	// certbotTimeout bounds one certbot run (HTTP-01 for a few names takes
	// well under a minute; the cap only stops a wedged container exec).
	certbotTimeout = 10 * time.Minute
)

// ErrNoDomains is returned by IssueCertificate for an empty host list.
var ErrNoDomains = errors.New("No domains provided")

// ErrInvalidEmail is returned by SetAcmeEmail.
var ErrInvalidEmail = errors.New("Invalid email")

// ErrNoAcmeEmail is returned by GetAcmeEmail when nothing is configured.
var ErrNoAcmeEmail = errors.New("No ACME email configured. Set SSL email in Settings or CERTBOT_EMAIL.")

// Store is the subset of *store.Store this package uses.
type Store interface {
	GetSetting(ctx context.Context, key string) (string, bool, error)
	SetSetting(ctx context.Context, key, value string) error
	DeleteSetting(ctx context.Context, key string) error
	OldestUser(ctx context.Context) (*store.User, error)
}

// DNSChecker is the DNS preflight; *dnscheck.Checker satisfies it.
type DNSChecker interface {
	CheckDomain(ctx context.Context, hostname string) dnscheck.Check
}

// Settings are the certificate-related configuration values.
type Settings struct {
	// LetsEncryptPath is the certbot config root (holds live/<name>/).
	LetsEncryptPath string
	// DataPath holds the persisted renew-watcher state (ssl-cert-mtime).
	DataPath string
	// CertbotContainer is the container certbot runs in (docker exec).
	CertbotContainer string
	// CertbotEmail is the fallback ACME account email.
	CertbotEmail string
	// CertbotStaging selects the Let's Encrypt staging CA.
	CertbotStaging bool
}

// SettingsFrom extracts Settings from the process configuration.
func SettingsFrom(c *config.Config) Settings {
	return Settings{
		LetsEncryptPath:  c.LetsEncryptPath,
		DataPath:         c.DataPath,
		CertbotContainer: c.CertbotContainer,
		CertbotEmail:     c.CertbotEmail,
		CertbotStaging:   c.CertbotStaging,
	}
}

// Service manages certificates. It is safe for concurrent use; create one per
// process so the activity log is shared by every caller.
type Service struct {
	cfg    Settings
	store  Store
	dns    DNSChecker
	runner Runner
	log    *slog.Logger
	now    func() time.Time
	events *EventLog

	pollInterval time.Duration
	retryDelay   time.Duration
}

// Option configures a Service.
type Option func(*Service)

// WithRunner replaces the docker exec runner (tests).
func WithRunner(r Runner) Option { return func(s *Service) { s.runner = r } }

// WithClock replaces time.Now (tests).
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		s.now = now
		s.events.now = now
	}
}

// WithWatcherIntervals overrides the renew watcher's poll interval (5 min)
// and failed-reload retry delay (15 s) (tests).
func WithWatcherIntervals(poll, retry time.Duration) Option {
	return func(s *Service) {
		s.pollInterval = poll
		s.retryDelay = retry
	}
}

// New returns a Service. dns may be nil only in tests that never issue.
func New(cfg Settings, st Store, dns DNSChecker, log *slog.Logger, opts ...Option) *Service {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	s := &Service{
		cfg:          cfg,
		store:        st,
		dns:          dns,
		runner:       ExecRunner{},
		log:          log,
		now:          time.Now,
		events:       NewEventLog(),
		pollInterval: 5 * time.Minute,
		retryDelay:   15 * time.Second,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Settings returns the service configuration.
func (s *Service) Settings() Settings { return s.cfg }

// Events returns the shared SSL activity log.
func (s *Service) Events() *EventLog { return s.events }

// AppendSSLEvent records an activity line for every hostname given.
func (s *Service) AppendSSLEvent(hostnames []string, level EventLevel, message string) {
	s.events.Append(hostnames, level, message)
}

// GetSSLEvents returns merged, de-duplicated, oldest-first activity.
func (s *Service) GetSSLEvents(hostnames []string) []Event { return s.events.Get(hostnames) }

// ClearSSLEvents drops the activity of the given hostnames.
func (s *Service) ClearSSLEvents(hostnames []string) { s.events.Clear(hostnames) }

// DiagnosticCommand is the command users can run to read certbot's log.
func (s *Service) DiagnosticCommand() string {
	return "sudo docker exec " + s.cfg.CertbotContainer + " tail -n 100 /var/log/letsencrypt/letsencrypt.log"
}

// hostnamePattern is the domain-name regex:
// dot-separated labels of letters, digits and inner hyphens, at least two
// labels. It admits nothing nginx or a shell would treat specially.
var hostnamePattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)+$`)

// IsValidHostname reports whether s matches DOMAIN_REGEX. Every hostname that
// reaches an nginx conf or a certbot argument must pass it.
func IsValidHostname(s string) bool { return hostnamePattern.MatchString(s) }

// certNamePattern admits certbot lineage directory names (hostnames and
// suffixes like "-0001"); nothing that could escape live/ or break an nginx
// directive.
var certNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// IsValidCertName reports whether name is safe to use as a live/<name>
// directory and in an nginx ssl_certificate path.
func IsValidCertName(name string) bool {
	return len(name) <= 255 && certNamePattern.MatchString(name) && !strings.Contains(name, "..")
}

// NormalizeHost trims (as JavaScript's String.prototype.trim) and lowercases.
func NormalizeHost(s string) string { return strings.ToLower(js.Trim(s)) }

// CleanHosts normalises hostnames, dropping empties and duplicates while
// keeping first-seen order (the first is the UI/meta primary).
func CleanHosts(hosts []string) []string {
	seen := make(map[string]bool, len(hosts))
	out := make([]string, 0, len(hosts))
	for _, h := range hosts {
		h = NormalizeHost(h)
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		out = append(out, h)
	}
	return out
}

// isoMillis formats like Date.prototype.toISOString.
func isoMillis(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }

func strp(s string) *string { return &s }

// GetAcmeEmail returns the ACME account email: the ssl_acme_email setting,
// else CERTBOT_EMAIL, else the oldest user's email.
func (s *Service) GetAcmeEmail(ctx context.Context) (string, error) {
	v, ok, err := s.store.GetSetting(ctx, acmeEmailKey)
	if err != nil {
		return "", err
	}
	if ok && strings.Contains(v, "@") {
		return js.Trim(v), nil
	}
	if strings.Contains(s.cfg.CertbotEmail, "@") {
		return js.Trim(s.cfg.CertbotEmail), nil
	}
	u, err := s.store.OldestUser(ctx)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return "", err
	}
	if u != nil && u.Email != "" {
		return u.Email, nil
	}
	return "", ErrNoAcmeEmail
}

// SetAcmeEmail stores the ACME account email (trimmed, lowercased). It must
// contain "@", else ErrInvalidEmail.
func (s *Service) SetAcmeEmail(ctx context.Context, email string) error {
	if email == "" || !strings.Contains(email, "@") {
		return ErrInvalidEmail
	}
	return s.store.SetSetting(ctx, acmeEmailKey, strings.ToLower(js.Trim(email)))
}

type meta struct {
	Status    SslStatus `json:"status"`
	Error     *string   `json:"error"`
	UpdatedAt string    `json:"updatedAt"`
}

func (s *Service) setMeta(ctx context.Context, primary string, status SslStatus, errMsg *string) error {
	raw, err := json.Marshal(meta{Status: status, Error: errMsg, UpdatedAt: isoMillis(s.now())})
	if err != nil {
		return err
	}
	return s.store.SetSetting(ctx, statusKeyPrefix+primary, string(raw))
}

func (s *Service) getMeta(ctx context.Context, primary string) (*meta, error) {
	raw, ok, err := s.store.GetSetting(ctx, statusKeyPrefix+primary)
	if err != nil || !ok || raw == "" {
		return nil, err
	}
	var m meta
	if json.Unmarshal([]byte(raw), &m) != nil {
		return nil, nil
	}
	return &m, nil
}

// ClearSSLMeta deletes the persisted issuance status of a primary hostname.
func (s *Service) ClearSSLMeta(ctx context.Context, primary string) error {
	return s.store.DeleteSetting(ctx, statusKeyPrefix+primary)
}

// statusFromFiles classifies an existing lineage by expiry. ok is false when
// the lineage's files do not exist.
func (s *Service) statusFromFiles(certName, domain string, unreadableErr *string) (CertificateStatus, bool) {
	if !s.CertificateFilesExist(certName) {
		return CertificateStatus{}, false
	}
	path := s.fullchainPath(certName)
	expires, ok := s.parseExpiry(certName)
	if !ok {
		e := "Certificate file unreadable"
		if unreadableErr != nil && *unreadableErr != "" {
			e = *unreadableErr
		}
		return CertificateStatus{Status: StatusFailed, Domain: domain, Error: &e, CertPath: &path}, true
	}
	st := StatusActive
	left := expires.Sub(s.now())
	switch {
	case left <= 0:
		st = StatusExpired
	case left < ExpiringWindow:
		st = StatusExpiring
	}
	return CertificateStatus{Status: st, Domain: domain, ExpiresAt: strp(isoMillis(expires)), CertPath: &path}, true
}

// GetCertificateStatus reports the certificate serving hostname: the
// healthiest lineage covering it (else live/<hostname>), falling back to the
// persisted pending/failed issuance status.
func (s *Service) GetCertificateStatus(ctx context.Context, hostname string) (CertificateStatus, error) {
	domain := NormalizeHost(hostname)
	certName := s.ResolveCertName(domain)
	if certName == "" {
		certName = domain
	}
	m, err := s.getMeta(ctx, domain)
	if err != nil {
		return CertificateStatus{}, err
	}
	if m == nil {
		if m, err = s.getMeta(ctx, certName); err != nil {
			return CertificateStatus{}, err
		}
	}
	var metaErr *string
	if m != nil {
		metaErr = m.Error
	}
	if st, ok := s.statusFromFiles(certName, domain, metaErr); ok {
		return st, nil
	}
	if m != nil && m.Status == StatusPending {
		return CertificateStatus{Status: StatusPending, Domain: domain}, nil
	}
	if m != nil && m.Status == StatusFailed {
		return CertificateStatus{Status: StatusFailed, Domain: domain, Error: m.Error, DiagnosticCommand: strp(s.DiagnosticCommand())}, nil
	}
	return CertificateStatus{Status: StatusMissing, Domain: domain}, nil
}

// statusForLineage is the status of a specific live/ lineage labelled with the
// UI primary hostname.
func (s *Service) statusForLineage(certName, displayDomain string) CertificateStatus {
	domain := NormalizeHost(displayDomain)
	if st, ok := s.statusFromFiles(certName, domain, nil); ok {
		return st
	}
	return CertificateStatus{Status: StatusMissing, Domain: domain}
}

// Health is the expiry assessment added after a certificate expired unnoticed
// in production. It is additive: CertificateStatus keeps its six keys, and
// routes that want these fields return a Report (which flattens both into one
// JSON object).
type Health struct {
	// Expired is true when a certificate exists and its notAfter has passed.
	Expired bool `json:"expired"`
	// RenewalOverdue is true when a certificate exists and has less than
	// RenewalOverdueWindow (20 days) left, or has expired: certbot renews at
	// 30 days, so automatic renewal is not happening.
	RenewalOverdue bool `json:"renewalOverdue"`
	// DaysUntilExpiry is the whole number of days left (rounded down;
	// negative once expired), or null when there is no readable certificate.
	DaysUntilExpiry *int `json:"daysUntilExpiry"`
}

// Report is a CertificateStatus plus its Health, serialised as one object:
// the six status keys followed by expired, renewalOverdue, daysUntilExpiry.
type Report struct {
	CertificateStatus
	Health
}

// HealthOf assesses st at time now. Only statuses with an expiry date carry
// a verdict; missing/pending/failed report false/false/null.
func HealthOf(st CertificateStatus, now time.Time) Health {
	if st.ExpiresAt == nil {
		return Health{}
	}
	expires, err := time.Parse(time.RFC3339Nano, *st.ExpiresAt)
	if err != nil {
		return Health{}
	}
	left := expires.Sub(now)
	days := int(left / (24 * time.Hour))
	if left < 0 && left%(24*time.Hour) != 0 {
		days--
	}
	return Health{Expired: left <= 0, RenewalOverdue: left < RenewalOverdueWindow, DaysUntilExpiry: &days}
}

// GetCertificateReport is GetCertificateStatus plus Health.
func (s *Service) GetCertificateReport(ctx context.Context, hostname string) (Report, error) {
	st, err := s.GetCertificateStatus(ctx, hostname)
	if err != nil {
		return Report{}, err
	}
	return Report{CertificateStatus: st, Health: HealthOf(st, s.now())}, nil
}

// NginxCertPaths returns a lineage's PEM paths as seen inside the nginx-proxy
// and certbot containers.
func NginxCertPaths(certName string) (fullchain, privkey string) {
	return "/etc/letsencrypt/live/" + certName + "/fullchain.pem", "/etc/letsencrypt/live/" + certName + "/privkey.pem"
}

func (s *Service) liveRoot() string { return filepath.Join(s.cfg.LetsEncryptPath, "live") }

func (s *Service) fullchainPath(certName string) string {
	return filepath.Join(s.liveRoot(), certName, "fullchain.pem")
}

func (s *Service) privkeyPath(certName string) string {
	return filepath.Join(s.liveRoot(), certName, "privkey.pem")
}

// CertificateFilesExist reports whether live/<certName>/ holds both
// fullchain.pem and privkey.pem. Unsafe names never exist.
func (s *Service) CertificateFilesExist(certName string) bool {
	if !IsValidCertName(certName) {
		return false
	}
	if _, err := os.Stat(s.fullchainPath(certName)); err != nil {
		return false
	}
	_, err := os.Stat(s.privkeyPath(certName))
	return err == nil
}
