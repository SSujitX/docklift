package httpx

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Origin is a parsed http(s) origin with an explicit port.
type Origin struct {
	Scheme   string // "http:" or "https:"
	Hostname string // lowercase, IPv6 without brackets
	Port     string // always set; the scheme default when omitted
}

func defaultPort(scheme string) string {
	if scheme == "https:" {
		return "443"
	}
	return "80"
}

// ParseOrigin parses an http(s) URL into its origin; nil when the value is not
// an http(s) URL with a host.
func ParseOrigin(value string) *Origin {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil || u.Host == "" {
		return nil
	}
	scheme := strings.ToLower(u.Scheme) + ":"
	if scheme != "http:" && scheme != "https:" {
		return nil
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return nil
	}
	port := u.Port()
	if port == "" {
		port = defaultPort(scheme)
	} else {
		n, err := strconv.Atoi(port)
		if err != nil || n < 0 || n > 65535 {
			return nil
		}
		port = strconv.Itoa(n)
	}
	return &Origin{Scheme: scheme, Hostname: host, Port: port}
}

// NormalizeOrigin returns the canonical `scheme//host:port` form, or "" when
// the value is not an http(s) origin.
func NormalizeOrigin(value string) string {
	o := ParseOrigin(value)
	if o == nil {
		return ""
	}
	return o.Scheme + "//" + o.Hostname + ":" + o.Port
}

func firstHeader(h http.Header, name string) string {
	values := h.Values(name)
	if len(values) == 0 {
		return ""
	}
	v, _, _ := strings.Cut(strings.Join(values, ", "), ",")
	return strings.TrimSpace(v)
}

// RequestOrigin is the origin the browser addressed, reconstructed from
// X-Forwarded-Host/Host and X-Forwarded-Proto (first values).
func RequestOrigin(r *http.Request, fallbackProto string) *Origin {
	host := firstHeader(r.Header, "X-Forwarded-Host")
	if host == "" {
		host = strings.TrimSpace(strings.SplitN(r.Host, ",", 2)[0])
	}
	if host == "" {
		return nil
	}
	proto := firstHeader(r.Header, "X-Forwarded-Proto")
	if proto == "" {
		proto = fallbackProto
	}
	scheme := "http"
	if proto == "https" {
		scheme = "https"
	}
	return ParseOrigin(scheme + "://" + host)
}

func sameOrigin(a, b *Origin) bool {
	if a == nil || b == nil || a.Hostname != b.Hostname {
		return false
	}
	if a.Scheme == b.Scheme {
		return a.Port == b.Port
	}
	// A proxy that terminates TLS without a trustworthy X-Forwarded-Proto makes an
	// https panel look like http. Tolerate the scheme mismatch only when both sides
	// use their own default port, which no app in the host port pool ever does.
	return a.Port == defaultPort(a.Scheme) && b.Port == defaultPort(b.Scheme)
}

// IsTrustedOrigin is the port of originCheck.ts isTrustedOrigin: true when
// origin is the dashboard itself (same scheme, host and port as this request)
// or exactly matches an allowlisted origin (CORS_ORIGIN entries and allow).
func IsTrustedOrigin(origin string, r *http.Request, fallbackProto string, allow []string) bool {
	parsed := ParseOrigin(origin)
	if parsed == nil {
		return false
	}
	normalized := NormalizeOrigin(origin)
	for _, entry := range allow {
		if trimmed := strings.TrimSpace(entry); trimmed != "" && NormalizeOrigin(trimmed) == normalized {
			return true
		}
	}
	return sameOrigin(parsed, RequestOrigin(r, fallbackProto))
}

// RequestProtocol is Express req.protocol with trust proxy = 1: the first
// X-Forwarded-Proto value, else "https" for TLS connections, else "http".
func RequestProtocol(r *http.Request) string {
	if v := r.Header.Values("X-Forwarded-Proto"); len(v) > 0 {
		joined := strings.Join(v, ", ")
		if joined != "" {
			first, _, _ := strings.Cut(joined, ",")
			return strings.TrimSpace(first)
		}
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}
