package certs

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"slices"
	"strings"
	"time"

	"docklift/internal/js"
)

// readLeaf parses the first certificate of live/<certName>/fullchain.pem (the
// leaf of a PEM bundle).
func (s *Service) readLeaf(certName string) (*x509.Certificate, bool) {
	if !IsValidCertName(certName) {
		return nil, false
	}
	raw, err := os.ReadFile(s.fullchainPath(certName))
	if err != nil {
		return nil, false
	}
	for {
		var block *pem.Block
		block, raw = pem.Decode(raw)
		if block == nil {
			return nil, false
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, false
		}
		return cert, true
	}
}

// ListCertDNSNames returns the DNS names on a lineage's leaf certificate:
// DNS SANs then the subject CN, lowercased and de-duplicated — never IP,
// email or URI SANs. A missing or unreadable lineage has no names.
func (s *Service) ListCertDNSNames(certName string) []string {
	if !s.CertificateFilesExist(certName) {
		return []string{}
	}
	cert, ok := s.readLeaf(certName)
	if !ok {
		return []string{}
	}
	names := make([]string, 0, len(cert.DNSNames)+1)
	add := func(n string) {
		n = strings.ToLower(js.Trim(n))
		if n != "" && !slices.Contains(names, n) {
			names = append(names, n)
		}
	}
	for _, n := range cert.DNSNames {
		add(n)
	}
	add(cert.Subject.CommonName)
	return names
}

// parseExpiry returns the leaf's notAfter.
func (s *Service) parseExpiry(certName string) (time.Time, bool) {
	cert, ok := s.readLeaf(certName)
	if !ok {
		return time.Time{}, false
	}
	return cert.NotAfter, true
}

// liveCertDirNames lists lineages under live/ that have both PEM files, in
// name order (README and unsafe names skipped).
func (s *Service) liveCertDirNames() []string {
	entries, err := os.ReadDir(s.liveRoot())
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		name := e.Name()
		if name == "README" || !s.CertificateFilesExist(name) {
			continue
		}
		names = append(names, name)
	}
	return names
}

// scoreCertLineage ranks a lineage for a host set; higher is better. When the
// overlap ties, covering the UI primary must beat a newer orphan (preferring
// the folder name used to lose to expiry and HSTS-brick the established host).
// The arithmetic is float64 in the same order as 2.x so ties and
// precision behave identically.
func (s *Service) scoreCertLineage(certName string, hosts []string) float64 {
	primary := ""
	if len(hosts) > 0 {
		primary = hosts[0]
	}
	sans := s.ListCertDNSNames(certName)
	var exp float64
	if t, ok := s.parseExpiry(certName); ok {
		exp = float64(t.UnixMilli())
	}
	valid := 0.0
	if exp > float64(s.now().UnixMilli()) {
		valid = 1
	}
	coversPrimary := 0.0
	if primary != "" && slices.Contains(sans, primary) {
		coversPrimary = 1
	}
	nameIsPrimary := 0.0
	if primary != "" && certName == primary {
		nameIsPrimary = 1
	}
	return coversPrimary*1e16 + valid*1e15 + float64(len(sans))*1e10 + nameIsPrimary*1e9 + exp
}

// CertCoversAllHosts reports whether lineage certName has every hostname.
func (s *Service) CertCoversAllHosts(certName string, hosts []string) bool {
	names := s.ListCertDNSNames(certName)
	if len(names) == 0 {
		return false
	}
	for _, h := range hosts {
		if !slices.Contains(names, h) {
			return false
		}
	}
	return true
}

// FindCoveringCertName returns the live/<name> lineage that covers every
// hostname, preferring healthy (non-expired) lineages over same-named orphans
// — the folder name alone never wins. "" when none covers them all.
func (s *Service) FindCoveringCertName(hosts []string) string {
	cleaned := CleanHosts(hosts)
	if len(cleaned) == 0 {
		return ""
	}
	best, bestScore := "", -1.0
	for _, name := range s.liveCertDirNames() {
		if !s.CertCoversAllHosts(name, cleaned) {
			continue
		}
		if score := s.scoreCertLineage(name, cleaned); score > bestScore {
			best, bestScore = name, score
		}
	}
	return best
}

// FindBestExpandableCertName returns the existing lineage to expand or replace
// for this host set. Only lineages named after one of the requested hosts are
// considered, so another service's leftover lineage is never mutated after a
// domain move. "" when none overlaps.
func (s *Service) FindBestExpandableCertName(hosts []string) string {
	cleaned := CleanHosts(hosts)
	if len(cleaned) == 0 {
		return ""
	}
	best, bestOverlap, bestScore := "", 0, -1.0
	for _, name := range s.liveCertDirNames() {
		if !slices.Contains(cleaned, name) {
			continue
		}
		sans := s.ListCertDNSNames(name)
		overlap := 0
		for _, h := range cleaned {
			if slices.Contains(sans, h) {
				overlap++
			}
		}
		if overlap == 0 {
			continue
		}
		score := s.scoreCertLineage(name, cleaned)
		if overlap > bestOverlap || (overlap == bestOverlap && score > bestScore) {
			best, bestOverlap, bestScore = name, overlap, score
		}
	}
	return best
}

// ResolveCertName returns the lineage serving one hostname (health-aware,
// not same-folder-first), or "".
func (s *Service) ResolveCertName(hostname string) string {
	host := NormalizeHost(hostname)
	if host == "" {
		return ""
	}
	return s.FindCoveringCertName([]string{host})
}

// Lineage is the certbot lineage chosen for a host set.
type Lineage struct {
	// CertName is the --cert-name: the covering lineage, else the best
	// expandable one, else the primary hostname.
	CertName string
	// Covering is the lineage that already covers every host, or "".
	Covering string
	// NeedsExpand is true for a pure SAN expansion: every existing name stays
	// and at least one new name is added.
	NeedsExpand bool
}

// SelectCertLineage chooses certbot's --cert-name for hosts.
func (s *Service) SelectCertLineage(hosts []string) Lineage {
	cleaned := CleanHosts(hosts)
	primary := "unknown"
	if len(cleaned) > 0 {
		primary = cleaned[0]
	}
	covering := s.FindCoveringCertName(cleaned)
	certName := covering
	if certName == "" {
		certName = s.FindBestExpandableCertName(cleaned)
	}
	if certName == "" {
		certName = primary
	}
	existing := s.ListCertDNSNames(certName)
	return Lineage{CertName: certName, Covering: covering, NeedsExpand: isStrictSuperset(cleaned, existing)}
}

// isStrictSuperset reports whether next keeps every existing name and adds
// at least one (existing must be non-empty).
func isStrictSuperset(next, existing []string) bool {
	if len(existing) == 0 {
		return false
	}
	for _, n := range existing {
		if !slices.Contains(next, n) {
			return false
		}
	}
	for _, n := range next {
		if !slices.Contains(existing, n) {
			return true
		}
	}
	return false
}
