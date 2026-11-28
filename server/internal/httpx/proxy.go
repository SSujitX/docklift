package httpx

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// TrustedProxies decides whose forwarded headers are believed. 2.x ran
// with `trust proxy` = 1, which trusts X-Forwarded-* from every socket peer;
// the Go binary is reachable both through docklift-nginx-proxy and directly
// on the published dashboard port, so a direct client could forge its address
// (and dodge the login rate limits) or its scheme and host. X-Forwarded-For,
// X-Forwarded-Proto and X-Forwarded-Host are therefore honoured only when the
// socket peer is a trusted proxy.
type TrustedProxies struct {
	trusted  []netip.Prefix
	excluded []netip.Prefix
}

// NewTrustedProxies returns the policy trusting peers inside one of trusted
// and inside none of excluded (config.ProxyTrust). With no trusted prefixes
// no peer is trusted.
func NewTrustedProxies(trusted, excluded []netip.Prefix) *TrustedProxies {
	return &TrustedProxies{
		trusted:  append([]netip.Prefix(nil), trusted...),
		excluded: append([]netip.Prefix(nil), excluded...),
	}
}

// Trusts reports whether addr is a trusted proxy. IPv4-mapped IPv6 peers
// compare as IPv4 and zones are ignored; an invalid address is never trusted.
func (t *TrustedProxies) Trusts(addr netip.Addr) bool {
	if t == nil || !addr.IsValid() {
		return false
	}
	addr = addr.WithZone("").Unmap()
	for _, p := range t.excluded {
		if p.Contains(addr) {
			return false
		}
	}
	for _, p := range t.trusted {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

type peerTrustKey struct{}

// Middleware records on the request context whether the socket peer is a
// trusted proxy. Install it first, before anything that calls ClientIP,
// RequestProtocol, RequestOrigin, IsTrustedOrigin or SecurityHeaders.
func (t *TrustedProxies) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trusted := t.Trusts(peerAddr(r))
		next.ServeHTTP(w, r.WithContext(WithPeerTrusted(r.Context(), trusted)))
	})
}

// WithPeerTrusted marks the request's socket peer as a trusted proxy or not
// (TrustedProxies.Middleware; tests of handlers mounted without it).
func WithPeerTrusted(ctx context.Context, trusted bool) context.Context {
	return context.WithValue(ctx, peerTrustKey{}, trusted)
}

// PeerTrusted reports whether forwarded headers of r may be believed. It is
// false unless TrustedProxies.Middleware (or WithPeerTrusted) said otherwise,
// so a handler served without the middleware never trusts the headers.
func PeerTrusted(r *http.Request) bool {
	trusted, _ := r.Context().Value(peerTrustKey{}).(bool)
	return trusted
}

// peerAddr is the socket peer address (invalid when RemoteAddr is not ip:port
// or a bare IP).
func peerAddr(r *http.Request) netip.Addr {
	if ap, err := netip.ParseAddrPort(r.RemoteAddr); err == nil {
		return ap.Addr()
	}
	addr, _ := netip.ParseAddr(r.RemoteAddr)
	return addr
}

// PeerIP is the socket peer address without the port (the socket's
// remote address), regardless of forwarded headers.
func PeerIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ClientIP is the client address. From a trusted proxy it is
// the last X-Forwarded-For entry (one proxy hop: the address the proxy itself
// saw), normalised; when the peer is not trusted, or the header is absent or
// its last entry is not an IP address, it is the socket peer address.
func ClientIP(r *http.Request) string {
	if PeerTrusted(r) {
		if xff := forwardedFor(strings.Join(r.Header.Values("X-Forwarded-For"), ", ")); len(xff) > 0 {
			if addr, ok := parseForwardedAddr(xff[0]); ok {
				return addr.String()
			}
		}
	}
	return PeerIP(r)
}

// parseForwardedAddr accepts an X-Forwarded-For entry that is an IP address,
// optionally with a port ("1.2.3.4:5", "[::1]:5") as some proxies send.
func parseForwardedAddr(s string) (netip.Addr, bool) {
	if addr, err := netip.ParseAddr(s); err == nil && addr.Zone() == "" {
		return addr, true
	}
	if ap, err := netip.ParseAddrPort(s); err == nil && ap.Addr().Zone() == "" {
		return ap.Addr(), true
	}
	return netip.Addr{}, false
}

// forwardedFor parses X-Forwarded-For like the `forwarded` package: entries
// are returned from last to first, trimmed of spaces, empty ones skipped.
func forwardedFor(header string) []string {
	var list []string
	end, start := len(header), len(header)
	for i := len(header) - 1; i >= 0; i-- {
		switch header[i] {
		case ' ':
			if start == end {
				start, end = i, i
			}
		case ',':
			if start != end {
				list = append(list, header[start:end])
			}
			start, end = i, i
		default:
			start = i
		}
	}
	if start != end {
		list = append(list, header[start:end])
	}
	return list
}

// forwardedHeader is the first comma-separated value of a forwarded header,
// trimmed; "" when the peer is not trusted or the header is absent.
func forwardedHeader(r *http.Request, name string) string {
	if !PeerTrusted(r) {
		return ""
	}
	return firstHeader(r.Header, name)
}

func firstHeader(h http.Header, name string) string {
	values := h.Values(name)
	if len(values) == 0 {
		return ""
	}
	v, _, _ := strings.Cut(strings.Join(values, ", "), ",")
	return strings.TrimSpace(v)
}

// connProtocol is the scheme of the connection itself.
func connProtocol(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

// RequestProtocol is the request scheme: from a trusted proxy the first
// X-Forwarded-Proto value, otherwise (or when that is empty) "https" for TLS
// connections and "http" for plain ones.
func RequestProtocol(r *http.Request) string {
	if proto := forwardedHeader(r, "X-Forwarded-Proto"); proto != "" {
		return proto
	}
	return connProtocol(r)
}
