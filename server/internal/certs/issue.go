package certs

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"docklift/internal/dnscheck"
)

var (
	ansiEscape       = regexp.MustCompile("\x1b\\[[0-9;]*m")
	domainLine       = regexp.MustCompile(`(?i)^Domain:\s*(.+)$`)
	detailLine       = regexp.MustCompile(`(?i)^Detail:\s*(.+)$`)
	domainLinePrefix = regexp.MustCompile(`(?i)^Domain:`)
	boilerplate      = regexp.MustCompile(`(?i)^(saving debug log|ask for help|search for solutions|see the logfile|some challenges have failed|certbot failed to authenticate|the certificate authority reported|an unexpected error occurred|hint:)`)
	usefulLine       = regexp.MustCompile(`(?i)(error|failed|invalid|unauthorized|forbidden|nxdomain|timeout|timed out|connection|refused|rate limit|no such|not found|permission denied)`)
)

// maxSummary is the length cap of a summarised certbot error.
const maxSummary = 700

// truncate cuts s to n UTF-16 code units like String.prototype.slice(0, n),
// without splitting a character.
func truncate(s string, n int) string {
	units := 0
	for i, r := range s {
		w := 1
		if r > 0xffff {
			w = 2
		}
		if units+w > n {
			return s[:i]
		}
		units += w
	}
	return s
}

// SummarizeCertbotError turns certbot's verbose output into the first useful,
// user-facing cause. HTTP-01 failures come as Domain / Type / Detail blocks,
// and the CA's detail (NXDOMAIN, wrong IP, timeout, invalid response) is
// preferred over certbot's generic "Some challenges have failed" footer.
func SummarizeCertbotError(stdout, stderr string) string {
	var parts []string
	for _, p := range []string{stderr, stdout} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	output := strings.ReplaceAll(ansiEscape.ReplaceAllString(strings.Join(parts, "\n"), ""), "\r", "")
	raw := strings.Split(output, "\n")
	lines := make([]string, len(raw))
	for i, l := range raw {
		lines[i] = TrimJS(l)
	}

	for i := range lines {
		m := domainLine.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		domain := TrimJS(m[1])
		if domain == "" {
			continue
		}
		for j := i + 1; j < min(len(lines), i+8); j++ {
			if d := detailLine.FindStringSubmatch(lines[j]); d != nil {
				if detail := TrimJS(d[1]); detail != "" {
					return truncate(domain+": "+detail, maxSummary)
				}
			}
			if domainLinePrefix.MatchString(lines[j]) {
				break
			}
		}
	}

	for _, l := range lines {
		if l != "" && !boilerplate.MatchString(l) && usefulLine.MatchString(l) {
			return truncate(l, maxSummary)
		}
	}
	for _, l := range lines {
		if l != "" && !boilerplate.MatchString(l) {
			return truncate(l, maxSummary)
		}
	}
	return "Certificate issuance failed for an unknown reason"
}

// CertbotRequest is everything that shapes one certbot invocation.
type CertbotRequest struct {
	Container string
	Email     string
	CertName  string
	Staging   bool
	// Expand adds --expand (pure SAN superset of the existing lineage).
	Expand bool
	// ForceRenewal adds --force-renewal (forced retry, shrink or replace).
	ForceRenewal bool
	// Hosts become -d arguments, in order.
	Hosts []string
}

// Args returns the `docker` argument vector: exec <container> certbot
// certonly --webroot ... -d host....
func (r CertbotRequest) Args() []string {
	args := []string{
		"exec", r.Container, "certbot", "certonly", "--webroot", "-w", "/var/www/certbot",
		"--non-interactive", "--agree-tos", "--email", r.Email, "--cert-name", r.CertName,
	}
	if r.Staging {
		args = append(args, "--staging")
	}
	if r.Expand {
		args = append(args, "--expand")
	}
	if r.ForceRenewal {
		args = append(args, "--force-renewal")
	}
	for _, h := range r.Hosts {
		args = append(args, "-d", h)
	}
	return args
}

// planOrder decides the ACME order flags. existing are the names on the
// chosen lineage, order the final -d list. A pure --expand needs a strict
// superset; any other change to an existing lineage (shrink/replace) needs
// --force-renewal so certbot does not fork a "-0001" lineage, while a pure
// expand does not (saving Let's Encrypt quota). force always renews.
func planOrder(existing, order []string, force bool) (expand, forceRenewal bool) {
	expand = isStrictSuperset(order, existing)
	sameSet := len(existing) == len(order)
	if sameSet {
		for _, n := range existing {
			if !slices.Contains(order, n) {
				sameSet = false
				break
			}
		}
	}
	shrinkOrReplace := len(existing) > 0 && !sameSet && !expand
	return expand, force || shrinkOrReplace
}

// IssueOptions tunes IssueCertificate.
type IssueOptions struct {
	// Force re-issues even when a healthy covering certificate exists
	// (manual retry) and passes --force-renewal.
	Force bool
}

func dnsEventLevel(st dnscheck.Status) EventLevel {
	switch st {
	case dnscheck.StatusOK:
		return LevelInfo
	case dnscheck.StatusMissing:
		return LevelError
	}
	return LevelWarn
}

// IssueCertificate issues (or reuses) a certificate for domains. The first
// hostname is the UI/meta primary; the PEM directory follows the covering or
// expandable lineage. Certbot failures are not errors: they are recorded in
// the activity log and the ssl_meta setting and reflected in the returned
// status. The error is non-nil only for an empty or invalid host list or a
// database failure.
func (s *Service) IssueCertificate(ctx context.Context, domains []string, opts IssueOptions) (CertificateStatus, error) {
	cleaned := CleanHosts(domains)
	if len(cleaned) == 0 {
		return CertificateStatus{}, ErrNoDomains
	}
	for _, h := range cleaned {
		if !IsValidHostname(h) {
			return CertificateStatus{}, fmt.Errorf("Invalid domain: %q", h)
		}
	}
	primary := cleaned[0]

	lineage := s.SelectCertLineage(cleaned)
	if !opts.Force && lineage.Covering != "" {
		st := s.statusForLineage(lineage.Covering, primary)
		if st.Status == StatusActive || st.Status == StatusExpiring {
			msg := "Existing certificate already covers these hostnames — reusing it."
			if lineage.Covering != primary {
				msg = fmt.Sprintf("Existing certificate (%s) already covers these hostnames — reusing it.", lineage.Covering)
			}
			s.events.Append(cleaned, LevelInfo, msg)
			return st, nil
		}
	}

	if err := s.setMeta(ctx, primary, StatusPending, nil); err != nil {
		return CertificateStatus{}, err
	}
	staging := ""
	if s.cfg.CertbotStaging {
		staging = " (staging CA)"
	}
	s.events.Append(cleaned, LevelInfo, "Requesting certificate for "+strings.Join(cleaned, ", ")+staging)

	email, err := s.GetAcmeEmail(ctx)
	if err != nil {
		s.events.Append(cleaned, LevelError, err.Error())
		msg := err.Error()
		if err := s.setMeta(ctx, primary, StatusFailed, &msg); err != nil {
			return CertificateStatus{}, err
		}
		return s.statusForLineage(lineage.CertName, primary), nil
	}

	certName := lineage.CertName
	existing := s.ListCertDNSNames(certName)

	// ACME -d list: required hosts + extras still on the lineage. Extras are
	// dropped only when DNS is definitively missing; mismatch/unknown stay —
	// dropping them would make a non-superset --expand (fork -0001).
	order := append([]string{}, cleaned...)
	for _, extra := range existing {
		if slices.Contains(order, extra) {
			continue
		}
		if s.dns.CheckDomain(ctx, extra).Status == dnscheck.StatusMissing {
			s.events.Append(cleaned, LevelWarn, fmt.Sprintf("Leaving %s off this order — DNS record missing (certificate set will shrink).", extra))
		} else {
			order = append(order, extra)
		}
	}
	expand, forceRenewal := planOrder(existing, order, opts.Force)

	// Preflight DNS for every hostname on the ACME order.
	var unresolved []string
	for _, d := range order {
		check := s.dns.CheckDomain(ctx, d)
		s.events.Append(cleaned, dnsEventLevel(check.Status), check.Message)
		if check.Status == dnscheck.StatusMissing && slices.Contains(cleaned, d) {
			unresolved = append(unresolved, d)
		}
	}
	if len(unresolved) > 0 {
		msg := fmt.Sprintf("DNS record missing for %s — create an A record pointing at this server, then retry SSL.", strings.Join(unresolved, ", "))
		s.events.Append(cleaned, LevelError, "Skipped Let’s Encrypt: DNS is not ready yet.")
		if err := s.setMeta(ctx, primary, StatusFailed, &msg); err != nil {
			return CertificateStatus{}, err
		}
		return s.statusForLineage(certName, primary), nil
	}

	req := CertbotRequest{
		Container: s.cfg.CertbotContainer, Email: email, CertName: certName, Staging: s.cfg.CertbotStaging,
		Expand: expand, ForceRenewal: forceRenewal, Hosts: order,
	}
	s.log.Info(fmt.Sprintf("[SSL] Issuing certificate for %s cert-name=%s expand=%t force=%t (staging=%t)",
		strings.Join(order, ", "), certName, expand, forceRenewal, s.cfg.CertbotStaging))
	s.events.Append(cleaned, LevelInfo, fmt.Sprintf("Running certbot HTTP-01 challenge in %s…", s.cfg.CertbotContainer))

	runCtx, cancel := context.WithTimeout(ctx, certbotTimeout)
	res, runErr := s.runner.Run(runCtx, "docker", req.Args()...)
	cancel()
	if runErr != nil {
		// Could not start docker (or timed out): same as a spawn error in TS.
		res = ExecResult{ExitCode: 1, Stdout: res.Stdout, Stderr: runErr.Error()}
	}

	// Success = PEMs exist AND cover every requested hostname (not merely an
	// old folder being present).
	covers := s.CertificateFilesExist(certName) && s.CertCoversAllHosts(certName, cleaned)
	if res.ExitCode != 0 || !covers {
		var msg string
		if res.ExitCode != 0 {
			msg = SummarizeCertbotError(res.Stdout, res.Stderr)
		} else {
			msg = fmt.Sprintf("Certificate at live/%s does not cover %s", certName, strings.Join(cleaned, ", "))
		}
		s.log.Error("[SSL] Issue failed for "+primary, "error", msg)
		s.events.Append(cleaned, LevelError, msg)
		if err := s.setMeta(ctx, primary, StatusFailed, &msg); err != nil {
			return CertificateStatus{}, err
		}
		return s.statusForLineage(certName, primary), nil
	}

	if err := s.setMeta(ctx, primary, StatusActive, nil); err != nil {
		return CertificateStatus{}, err
	}
	s.log.Info(fmt.Sprintf("[SSL] Certificate active for %s (live/%s)", primary, certName))
	issued := s.statusForLineage(certName, primary)
	msg := "Certificate issued"
	if issued.ExpiresAt != nil {
		if t, err := time.Parse(time.RFC3339Nano, *issued.ExpiresAt); err == nil {
			msg = "Certificate issued — valid until " + t.UTC().Format(http.TimeFormat)
		}
	}
	s.events.Append(cleaned, LevelSuccess, msg)
	return issued, nil
}
