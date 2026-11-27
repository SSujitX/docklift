package httpx

import (
	"container/list"
	"context"
	"math"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"
)

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

// DefaultRateLimitMaxKeys bounds the clients one RateLimiter tracks.
const DefaultRateLimitMaxKeys = 50_000

// RateLimitConfig configures a RateLimiter.
type RateLimitConfig struct {
	Window time.Duration
	Limit  int
	// Message is the error text of the 429 body `{"error": Message}`.
	Message string
	// Skip exempts requests from counting.
	Skip func(*http.Request) bool
	// MaxKeys caps how many client keys are tracked at once (default
	// DefaultRateLimitMaxKeys). When a new key arrives at the cap, the key
	// whose window started first is forgotten.
	MaxKeys int
}

type rateClient struct {
	key     string
	hits    int
	resetAt time.Time
}

// RateLimiter is express-rate-limit with the default MemoryStore and
// standardHeaders: true (draft-6 RateLimit-* headers, no legacy headers):
// a fixed window per client key (RateLimitKey of ClientIP) starting at its
// first hit; requests over the limit get 429 with Retry-After and the
// configured message.
//
// Memory is bounded: clients are kept in window-start order (every window
// has the same length, so that is also expiry order), expired clients are
// dropped from the front on every hit and by a periodic sweep, and at most
// MaxKeys clients are tracked.
type RateLimiter struct {
	cfg RateLimitConfig
	now func() time.Time

	mu      sync.Mutex
	clients map[string]*list.Element // of *rateClient
	order   *list.List               // oldest window first
}

// NewRateLimiter returns a limiter whose expired entries are also swept
// every window until ctx is done.
func NewRateLimiter(ctx context.Context, cfg RateLimitConfig) *RateLimiter {
	l := newRateLimiter(cfg, time.Now)
	go l.sweep(ctx)
	return l
}

func newRateLimiter(cfg RateLimitConfig, now func() time.Time) *RateLimiter {
	if cfg.MaxKeys <= 0 {
		cfg.MaxKeys = DefaultRateLimitMaxKeys
	}
	return &RateLimiter{cfg: cfg, now: now, clients: map[string]*list.Element{}, order: list.New()}
}

func (l *RateLimiter) sweep(ctx context.Context) {
	t := time.NewTicker(l.cfg.Window)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			l.mu.Lock()
			l.evictExpired(l.now())
			l.mu.Unlock()
		}
	}
}

// evictExpired drops clients whose window has ended. l.mu must be held.
func (l *RateLimiter) evictExpired(now time.Time) {
	for e := l.order.Front(); e != nil; e = l.order.Front() {
		c := e.Value.(*rateClient)
		if c.resetAt.After(now) {
			return
		}
		l.remove(e)
	}
}

func (l *RateLimiter) remove(e *list.Element) {
	delete(l.clients, l.order.Remove(e).(*rateClient).key)
}

// size is the number of tracked clients.
func (l *RateLimiter) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.clients)
}

// hit counts one request for key and returns the hit count and window reset.
func (l *RateLimiter) hit(key string) (int, time.Time) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.evictExpired(now)
	e, ok := l.clients[key]
	if !ok {
		for len(l.clients) >= l.cfg.MaxKeys {
			l.remove(l.order.Front())
		}
		e = l.order.PushBack(&rateClient{key: key, resetAt: now.Add(l.cfg.Window)})
		l.clients[key] = e
	}
	c := e.Value.(*rateClient)
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

// FailureLimitConfig configures a FailureLimiter.
type FailureLimitConfig struct {
	// Window is how long failures are remembered, from the first failure.
	Window time.Duration
	// Limit is how many failures a key may make in one window; the next
	// attempt is refused until the window ends.
	Limit int
	// MaxKeys caps how many keys are tracked (default
	// DefaultRateLimitMaxKeys); at the cap the oldest window is forgotten.
	MaxKeys int
	// Now is the clock (default time.Now).
	Now func() time.Time
}

// FailureLimiter counts failed attempts (password guesses) per key in a
// fixed window that starts at the key's first failure. Unlike RateLimiter
// it counts only what the caller reports as a failure, and a success
// forgets the key. Expired keys are dropped on every call and at most
// MaxKeys are tracked, so memory stays bounded without a sweeper.
type FailureLimiter struct {
	limit int
	rl    *RateLimiter
}

// NewFailureLimiter returns an empty limiter.
func NewFailureLimiter(cfg FailureLimitConfig) *FailureLimiter {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &FailureLimiter{limit: cfg.Limit, rl: newRateLimiter(RateLimitConfig{Window: cfg.Window, Limit: cfg.Limit, MaxKeys: cfg.MaxKeys}, now)}
}

// RetryAfter reports how long key must wait before its next attempt: zero
// while it has failed fewer than Limit times in the current window.
func (f *FailureLimiter) RetryAfter(key string) time.Duration {
	l := f.rl
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.evictExpired(now)
	e, ok := l.clients[key]
	if !ok {
		return 0
	}
	c := e.Value.(*rateClient)
	if c.hits < f.limit {
		return 0
	}
	return c.resetAt.Sub(now)
}

// Fail records one failed attempt by key.
func (f *FailureLimiter) Fail(key string) { f.rl.hit(key) }

// Reset forgets key's failures (after a successful attempt).
func (f *FailureLimiter) Reset(key string) {
	l := f.rl
	l.mu.Lock()
	defer l.mu.Unlock()
	if e, ok := l.clients[key]; ok {
		l.remove(e)
	}
}

// RetryAfterSeconds is d as a Retry-After header value: whole seconds,
// rounded up, at least 1.
func RetryAfterSeconds(d time.Duration) string {
	return strconv.Itoa(max(int(math.Ceil(d.Seconds())), 1))
}
