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

// RequestOrigin is the origin the browser addressed: the host is the first
// X-Forwarded-Host value from a trusted proxy, else Host; the scheme is the
// first X-Forwarded-Proto value from a trusted proxy, else fallbackProto.
func RequestOrigin(r *http.Request, fallbackProto string) *Origin {
	host := forwardedHeader(r, "X-Forwarded-Host")
	if host == "" {
		host = strings.TrimSpace(strings.SplitN(r.Host, ",", 2)[0])
	}
	if host == "" {
		return nil
	}
	proto := forwardedHeader(r, "X-Forwarded-Proto")
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

// IsTrustedOrigin reports true when
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
