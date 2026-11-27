// Package dnscheck is the port of backend/src/services/dnsCheck.ts: a DNS
// preflight for custom domains that answers "does this hostname point at this
// server?" before Let's Encrypt is asked for a certificate, so a missing record
// fails fast with an actionable message instead of burning an ACME failure.
package dnscheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"docklift/internal/js"
)

// Status classifies a hostname's DNS.
type Status string

const (
	// StatusOK means an A record points at this server's public IP.
	StatusOK Status = "ok"
	// StatusMismatch means the name resolves, but not to this server.
	StatusMismatch Status = "mismatch"
	// StatusMissing means the resolver answered that there is no A and no AAAA record.
	StatusMissing Status = "missing"
	// StatusUnknown means the lookup failed or this server's IP is unknown.
	StatusUnknown Status = "unknown"
)

// Check is one hostname's result; its JSON matches DomainDnsCheck in dnsCheck.ts.
type Check struct {
	Domain   string   `json:"domain"`
	Status   Status   `json:"status"`
	A        []string `json:"a"`
	AAAA     []string `json:"aaaa"`
	ServerIP *string  `json:"serverIp"`
	Message  string   `json:"message"`
}

const (
	// DefaultIPSourceURL is the public-IP discovery endpoint (HTTPS only).
	DefaultIPSourceURL = "https://api.ipify.org?format=json"
	// IPCacheTTL is how long a discovered (or failed) public IP is reused.
	IPCacheTTL = 5 * time.Minute
	// IPLookupTimeout bounds the public-IP request.
	IPLookupTimeout = 4 * time.Second
	// LookupTimeout bounds each A / AAAA query.
	LookupTimeout = 5 * time.Second
	// maxIPResponse caps the public-IP response body.
	maxIPResponse = 4 << 10
)

// Resolver looks up the addresses of a host for network "ip4" or "ip6".
// *net.Resolver satisfies it.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// Checker runs DNS preflights. The zero value is not usable; use New.
type Checker struct {
	resolver   Resolver
	httpClient *http.Client
	ipURL      string
	now        func() time.Time
	lookupTO   time.Duration
	ipTO       time.Duration

	mu        sync.Mutex
	cachedIP  string
	cachedAt  time.Time
	haveStamp bool
}

// Option configures a Checker.
type Option func(*Checker)

// WithResolver replaces the system resolver (tests).
func WithResolver(r Resolver) Option { return func(c *Checker) { c.resolver = r } }

// WithIPSource sets the public-IP endpoint and HTTP client (tests). The URL
// must use https://; any other scheme makes GetServerPublicIP report no IP.
func WithIPSource(url string, client *http.Client) Option {
	return func(c *Checker) {
		c.ipURL = url
		if client != nil {
			c.httpClient = client
		}
	}
}

// WithClock replaces time.Now (tests).
func WithClock(now func() time.Time) Option { return func(c *Checker) { c.now = now } }

// WithTimeouts overrides the per-lookup and public-IP timeouts (tests).
func WithTimeouts(lookup, ip time.Duration) Option {
	return func(c *Checker) {
		c.lookupTO = lookup
		c.ipTO = ip
	}
}

// New returns a Checker using the system resolver and DefaultIPSourceURL.
func New(opts ...Option) *Checker {
	c := &Checker{
		resolver: net.DefaultResolver,
		ipURL:    DefaultIPSourceURL,
		now:      time.Now,
		lookupTO: LookupTimeout,
		ipTO:     IPLookupTimeout,
	}
	for _, o := range opts {
		o(c)
	}
	if c.httpClient == nil {
		c.httpClient = &http.Client{}
	}
	// Never follow a redirect off HTTPS.
	base := *c.httpClient
	base.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return errors.New("dnscheck: refusing non-HTTPS redirect")
		}
		if len(via) >= 3 {
			return errors.New("dnscheck: too many redirects")
		}
		return nil
	}
	c.httpClient = &base
	return c
}

// GetServerPublicIP returns this server's public IPv4/IPv6 address as reported
// by the IP source, or nil when it cannot be determined. Results (including
// failures) are cached for IPCacheTTL; the cache is stamped before the request
// so a server without internet access does not pay the timeout on every
// certificate attempt (concurrent callers during that request see the previous
// value, as in the TypeScript version).
func (c *Checker) GetServerPublicIP(ctx context.Context) *string {
	c.mu.Lock()
	if c.haveStamp && c.now().Sub(c.cachedAt) < IPCacheTTL {
		ip := c.cachedIP
		c.mu.Unlock()
		return strPtr(ip)
	}
	c.cachedAt = c.now()
	c.haveStamp = true
	c.mu.Unlock()

	if ip, ok := c.fetchPublicIP(ctx); ok {
		c.mu.Lock()
		c.cachedIP = ip
		c.mu.Unlock()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return strPtr(c.cachedIP)
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (c *Checker) fetchPublicIP(ctx context.Context) (string, bool) {
	if !strings.HasPrefix(strings.ToLower(c.ipURL), "https://") {
		return "", false
	}
	ctx, cancel := context.WithTimeout(ctx, c.ipTO)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.ipURL, nil)
	if err != nil {
		return "", false
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", false
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return "", false
	}
	var data struct {
		IP string `json:"ip"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, maxIPResponse)).Decode(&data); err != nil {
		return "", false
	}
	// Only accept a real address: the value is shown to users and compared
	// with A records.
	addr, err := netip.ParseAddr(data.IP)
	if err != nil {
		return "", false
	}
	return addr.Unmap().String(), true
}

type lookupResult struct {
	records      []string
	noRecord     bool
	lookupFailed bool
}

// lookup queries one address family. A "not found" answer (NXDOMAIN or no
// record of this type — ENOTFOUND/ENODATA in Node) means the resolver
// answered: there is no such record. Anything else (SERVFAIL, timeout, no
// resolver) is a lookup failure — our problem, not the user's.
func (c *Checker) lookup(ctx context.Context, host string, family int) lookupResult {
	ctx, cancel := context.WithTimeout(ctx, c.lookupTO)
	defer cancel()
	network := "ip4"
	if family == 6 {
		network = "ip6"
	}
	addrs, err := c.resolver.LookupNetIP(ctx, network, host)
	if err != nil {
		if isNotFound(err) {
			return lookupResult{records: []string{}, noRecord: true}
		}
		return lookupResult{records: []string{}, lookupFailed: true}
	}
	records := make([]string, 0, len(addrs))
	for _, a := range addrs {
		if family == 4 {
			a = a.Unmap()
			if !a.Is4() {
				continue
			}
		} else if a.Is4() {
			continue
		}
		records = append(records, a.String())
	}
	return lookupResult{records: records, noRecord: len(records) == 0}
}

// isNotFound reports a definitive negative answer. Timeouts and temporary
// failures are never "not found", even when the resolver flags them so.
func isNotFound(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return dnsErr.IsNotFound && !dnsErr.IsTimeout && !dnsErr.IsTemporary
	}
	return false
}

// CheckDomain classifies hostname's DNS against this server's public IP. The
// public-IP discovery and both lookups run concurrently.
func (c *Checker) CheckDomain(ctx context.Context, hostname string) Check {
	domain := strings.ToLower(js.Trim(hostname))
	var (
		wg       sync.WaitGroup
		serverIP *string
		v4, v6   lookupResult
	)
	wg.Add(3)
	go func() { defer wg.Done(); serverIP = c.GetServerPublicIP(ctx) }()
	go func() { defer wg.Done(); v4 = c.lookup(ctx, domain, 4) }()
	go func() { defer wg.Done(); v6 = c.lookup(ctx, domain, 6) }()
	wg.Wait()

	check := Check{Domain: domain, A: v4.records, AAAA: v6.records, ServerIP: serverIP}
	ipText := "this server"
	if serverIP != nil {
		ipText = *serverIP
	}

	switch {
	case v4.noRecord && v6.noRecord:
		check.Status = StatusMissing
		check.Message = fmt.Sprintf("No A or AAAA record for %s. Add an A record pointing at %s, then retry.", domain, ipText)
	case len(v4.records) == 0 && len(v6.records) == 0:
		check.Status = StatusUnknown
		check.Message = fmt.Sprintf("Could not resolve %s from this server (DNS lookup failed). Issuance will still be attempted.", domain)
	case serverIP == nil:
		check.Status = StatusUnknown
		check.Message = fmt.Sprintf("%s resolves to %s. This server's public IP could not be confirmed.", domain, joinAll(v4.records, v6.records))
	case contains(v4.records, *serverIP):
		check.Status = StatusOK
		check.Message = fmt.Sprintf("%s points at this server (%s).", domain, *serverIP)
	default:
		check.Status = StatusMismatch
		check.Message = fmt.Sprintf("%s resolves to %s, not this server (%s). If Cloudflare's proxy is enabled this is expected — otherwise fix the A record.",
			domain, joinAll(v4.records, v6.records), *serverIP)
	}
	return check
}

// CheckDomains checks each hostname in order, one at a time.
func (c *Checker) CheckDomains(ctx context.Context, hostnames []string) []Check {
	checks := make([]Check, 0, len(hostnames))
	for _, h := range hostnames {
		checks = append(checks, c.CheckDomain(ctx, h))
	}
	return checks
}

func joinAll(a, b []string) string {
	return strings.Join(append(append([]string{}, a...), b...), ", ")
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
