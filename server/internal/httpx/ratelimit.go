package httpx

import (
	"context"
	"math"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ClientIP is Express req.ip with `trust proxy` = 1: the socket peer is
// trusted as one proxy hop, so the address is the last X-Forwarded-For entry
// when the header is present, otherwise the socket peer address.
func ClientIP(r *http.Request) string {
	if xff := forwardedFor(strings.Join(r.Header.Values("X-Forwarded-For"), ", ")); len(xff) > 0 {
		return xff[0]
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
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

// RateLimitKey is express-rate-limit's ipKeyGenerator with ipv6Subnet 56:
// IPv4 (and IPv4-mapped IPv6) addresses are keyed as-is, other IPv6
// addresses by their /56 network.
func RateLimitKey(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil || !addr.Is6() {
		return ip
	}
	if addr.Is4In6() {
		return addr.Unmap().String()
	}
	if strings.Contains(ip, ".") && isIPv4Compatible(addr) {
		b := addr.As16()
		return netip.AddrFrom4([4]byte(b[12:])).String()
	}
	prefix, err := addr.WithZone("").Prefix(56)
	if err != nil {
		return ip
	}
	return prefix.String()
}

// isIPv4Compatible reports the deprecated ::a.b.c.d form (inside ::/96).
func isIPv4Compatible(addr netip.Addr) bool {
	b := addr.As16()
	for _, x := range b[:12] {
		if x != 0 {
			return false
		}
	}
	return true
}

// RateLimitConfig configures a RateLimiter.
type RateLimitConfig struct {
	Window time.Duration
	Limit  int
	// Message is the error text of the 429 body `{"error": Message}`.
	Message string
	// Skip exempts requests from counting.
	Skip func(*http.Request) bool
}

type rateClient struct {
	hits    int
	resetAt time.Time
}

// RateLimiter is express-rate-limit with the default MemoryStore and
// standardHeaders: true (draft-6 RateLimit-* headers, no legacy headers):
// a fixed window per client key starting at its first hit; requests over the
// limit get 429 with Retry-After and the configured message.
type RateLimiter struct {
	cfg RateLimitConfig
	now func() time.Time

	mu      sync.Mutex
	clients map[string]*rateClient
}

// NewRateLimiter returns a limiter whose expired entries are swept every
// window until ctx is done.
func NewRateLimiter(ctx context.Context, cfg RateLimitConfig) *RateLimiter {
	l := &RateLimiter{cfg: cfg, now: time.Now, clients: map[string]*rateClient{}}
	go l.sweep(ctx)
	return l
}

func (l *RateLimiter) sweep(ctx context.Context) {
	t := time.NewTicker(l.cfg.Window)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			now := l.now()
			l.mu.Lock()
			for k, c := range l.clients {
				if !c.resetAt.After(now) {
					delete(l.clients, k)
				}
			}
			l.mu.Unlock()
		}
	}
}

// hit counts one request for key and returns the hit count and window reset.
func (l *RateLimiter) hit(key string) (int, time.Time) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	c, ok := l.clients[key]
	if !ok {
		c = &rateClient{}
		l.clients[key] = c
	}
	if !c.resetAt.After(now) {
		c.hits = 0
		c.resetAt = now.Add(l.cfg.Window)
	}
	c.hits++
	return c.hits, c.resetAt
}

// Middleware applies the limiter.
func (l *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if l.cfg.Skip != nil && l.cfg.Skip(r) {
			next.ServeHTTP(w, r)
			return
		}
		hits, resetAt := l.hit(RateLimitKey(ClientIP(r)))
		remaining := max(l.cfg.Limit-hits, 0)
		resetSeconds := max(int(math.Ceil(float64(resetAt.Sub(l.now()).Milliseconds())/1000)), 0)
		h := w.Header()
		h.Set("RateLimit-Policy", strconv.Itoa(l.cfg.Limit)+";w="+strconv.Itoa(int(math.Ceil(l.cfg.Window.Seconds()))))
		h.Set("RateLimit-Limit", strconv.Itoa(l.cfg.Limit))
		h.Set("RateLimit-Remaining", strconv.Itoa(remaining))
		h.Set("RateLimit-Reset", strconv.Itoa(resetSeconds))
		if hits > l.cfg.Limit {
			h.Set("Retry-After", strconv.Itoa(resetSeconds))
			Error(w, http.StatusTooManyRequests, l.cfg.Message)
			return
		}
		next.ServeHTTP(w, r)
	})
}
