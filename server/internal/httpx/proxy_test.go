package httpx

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"docklift/internal/config"
)

func defaultTrust(t *testing.T) *TrustedProxies {
	t.Helper()
	pt, err := config.ParseTrustedProxies(config.DefaultTrustedProxies)
	if err != nil {
		t.Fatal(err)
	}
	return NewTrustedProxies(pt.Trusted, pt.Excluded)
}

func TestTrustedProxiesTrusts(t *testing.T) {
	trust := defaultTrust(t)
	cases := map[string]bool{
		"127.0.0.1":                      true,
		"127.10.20.30":                   true,
		"::1":                            true,
		"::ffff:127.0.0.1":               true,
		"172.28.0.2":                     false, // another container on docklift_network (a deployed app)
		"172.28.255.254":                 true,  // docklift-nginx-proxy's fixed address
		"172.28.0.1":                     false, // gateway: userland proxy / hairpin NAT for published ports
		"::ffff:172.28.0.1":              false,
		"fd12:3456:789a::5":              false,
		"fd12:3456:789a::ffff:fffe":      true,
		"fd12:3456:789a::1":              false, // IPv6 gateway
		"fd12:3456:789a:1::5":            false, // outside the /64
		"172.29.0.2":                     false,
		"10.0.0.1":                       false,
		"192.168.1.10":                   false,
		"203.0.113.7":                    false,
		"2001:db8::1":                    false,
		"fe80::1%eth0":                   false,
		"fd12:3456:789a::ffff:fffe%eth0": true, // zones are ignored
		"::ffff:203.0.113.7":             false,
		"0.0.0.0":                        false,
		"::":                             false,
		"255.255.255.255":                false,
		"fd12:3456:789a::ffff:ff":        false,
	}
	for s, want := range cases {
		if got := trust.Trusts(netip.MustParseAddr(s)); got != want {
			t.Errorf("Trusts(%s) = %v, want %v", s, got, want)
		}
	}
	if trust.Trusts(netip.Addr{}) {
		t.Error("the zero address must not be trusted")
	}
	var none *TrustedProxies
	if none.Trusts(netip.MustParseAddr("127.0.0.1")) || NewTrustedProxies(nil, nil).Trusts(netip.MustParseAddr("127.0.0.1")) {
		t.Error("an empty policy trusts nobody")
	}
}

// serveThrough runs h behind the trust middleware for a request from remote.
func serveThrough(trust *TrustedProxies, remote string, headers http.Header, h http.HandlerFunc) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remote
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	trust.Middleware(h).ServeHTTP(httptest.NewRecorder(), req)
}

func TestClientIP(t *testing.T) {
	trust := defaultTrust(t)
	cases := []struct {
		name   string
		remote string
		xff    []string
		want   string
	}{
		{"direct client, no header", "203.0.113.9:5555", nil, "203.0.113.9"},
		{"direct client spoofing", "203.0.113.9:5555", []string{"198.51.100.1"}, "203.0.113.9"},
		{"direct client spoofing loopback", "203.0.113.9:5555", []string{"127.0.0.1"}, "203.0.113.9"},
		{"direct IPv6 client spoofing", "[2001:db8::9]:443", []string{"198.51.100.1"}, "2001:db8::9"},
		{"gateway peer spoofing", "172.28.0.1:40000", []string{"198.51.100.1"}, "172.28.0.1"},
		{"IPv6 gateway peer spoofing", "[fd12:3456:789a::1]:40000", []string{"198.51.100.1"}, "fd12:3456:789a::1"},
		{"mapped gateway peer spoofing", "[::ffff:172.28.0.1]:40000", []string{"198.51.100.1"}, "::ffff:172.28.0.1"},
		{"proxy peer", "172.28.255.254:40000", []string{"203.0.113.7"}, "203.0.113.7"},
		{"deployed app on docklift_network spoofing", "172.28.0.3:40000", []string{"203.0.113.7"}, "172.28.0.3"},
		{"proxy peer, one hop only", "172.28.255.254:40000", []string{"198.51.100.1, 203.0.113.7"}, "203.0.113.7"},
		{"proxy peer, spaces", "172.28.255.254:40000", []string{"1.1.1.1,   203.0.113.7  "}, "203.0.113.7"},
		{"proxy peer, trailing empty entries", "172.28.255.254:40000", []string{"1.1.1.1, ,"}, "1.1.1.1"},
		{"proxy peer, repeated header", "172.28.255.254:40000", []string{"1.1.1.1", "2.2.2.2"}, "2.2.2.2"},
		{"IPv6 proxy peer", "[fd12:3456:789a::ffff:fffe]:40000", []string{"2001:db8::5"}, "2001:db8::5"},
		{"proxy peer, IPv6 client normalised", "172.28.255.254:40000", []string{"2001:DB8:0::5"}, "2001:db8::5"},
		{"proxy peer, entry with port", "172.28.255.254:40000", []string{"203.0.113.7:4711"}, "203.0.113.7"},
		{"proxy peer, bracketed IPv6 with port", "172.28.255.254:40000", []string{"[2001:db8::1]:80"}, "2001:db8::1"},
		{"proxy peer, malformed entry", "172.28.255.254:40000", []string{"not-an-ip"}, "172.28.255.254"},
		{"proxy peer, malformed last entry", "172.28.255.254:40000", []string{"203.0.113.7, 999.1.1.1"}, "172.28.255.254"},
		{"proxy peer, zone in entry", "172.28.255.254:40000", []string{"fe80::1%eth0"}, "172.28.255.254"},
		{"proxy peer, injected text", "172.28.255.254:40000", []string{"203.0.113.7 level=ERROR msg=pwned"}, "172.28.255.254"},
		{"proxy peer, empty header", "172.28.255.254:40000", []string{""}, "172.28.255.254"},
		{"loopback peer", "127.0.0.1:40000", []string{"203.0.113.7"}, "203.0.113.7"},
		{"IPv6 loopback peer", "[::1]:40000", []string{"203.0.113.7"}, "203.0.113.7"},
		{"unparseable RemoteAddr", "garbage", []string{"203.0.113.7"}, "garbage"},
	}
	for _, c := range cases {
		var got string
		serveThrough(trust, c.remote, http.Header{"X-Forwarded-For": c.xff}, func(_ http.ResponseWriter, r *http.Request) { got = ClientIP(r) })
		if got != c.want {
			t.Errorf("%s: ClientIP(remote=%s, xff=%q) = %q, want %q", c.name, c.remote, c.xff, got, c.want)
		}
	}

	// Without the middleware the peer is never trusted.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:1"
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	if got := ClientIP(req); got != "127.0.0.1" {
		t.Errorf("ClientIP without middleware = %q", got)
	}
	if got := ClientIP(req.WithContext(WithPeerTrusted(req.Context(), true))); got != "203.0.113.7" {
		t.Errorf("ClientIP with WithPeerTrusted = %q", got)
	}
	if got := PeerIP(req.WithContext(WithPeerTrusted(req.Context(), true))); got != "127.0.0.1" {
		t.Errorf("PeerIP = %q", got)
	}
}

func TestForwardedProtoAndHost(t *testing.T) {
	trust := defaultTrust(t)
	cases := []struct {
		name, remote string
		headers      http.Header
		proto        string
		origin       string
	}{
		{"proxy peer", "172.28.255.254:1", http.Header{"X-Forwarded-Proto": {"https"}, "X-Forwarded-Host": {"panel.example.com"}}, "https", "https://panel.example.com:443"},
		{"proxy peer, first values", "172.28.255.254:1", http.Header{"X-Forwarded-Proto": {"https, http"}, "X-Forwarded-Host": {"a.example, b.example"}}, "https", "https://a.example:443"},
		{"proxy peer, empty proto", "172.28.255.254:1", http.Header{"X-Forwarded-Proto": {""}}, "http", "http://docklift:8000"},
		{"direct client spoofing", "203.0.113.9:1", http.Header{"X-Forwarded-Proto": {"https"}, "X-Forwarded-Host": {"panel.example.com"}}, "http", "http://docklift:8000"},
		{"gateway spoofing", "172.28.0.1:1", http.Header{"X-Forwarded-Proto": {"https"}, "X-Forwarded-Host": {"evil.example"}}, "http", "http://docklift:8000"},
		{"IPv6 direct client spoofing", "[2001:db8::9]:1", http.Header{"X-Forwarded-Proto": {"https"}, "X-Forwarded-Host": {"evil.example"}}, "http", "http://docklift:8000"},
	}
	for _, c := range cases {
		serveThrough(trust, c.remote, c.headers, func(_ http.ResponseWriter, r *http.Request) {
			r.Host = "docklift:8000"
			if got := RequestProtocol(r); got != c.proto {
				t.Errorf("%s: RequestProtocol = %q, want %q", c.name, got, c.proto)
			}
			o := RequestOrigin(r, RequestProtocol(r))
			if got := o.Scheme + "//" + o.Hostname + ":" + o.Port; got != c.origin {
				t.Errorf("%s: RequestOrigin = %q, want %q", c.name, got, c.origin)
			}
		})
	}
}

func TestRateLimiterIgnoresSpoofedForwardedFor(t *testing.T) {
	trust := defaultTrust(t)
	now := time.Unix(1_800_000_000, 0)
	l := newRateLimiter(RateLimitConfig{Window: time.Minute, Limit: 3, Message: "slow down"}, func() time.Time { return now })
	h := trust.Middleware(l.Middleware(ok))
	hit := func(remote, xff string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
		req.RemoteAddr = remote
		req.Header.Set("X-Forwarded-For", xff)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	// A direct client rotating X-Forwarded-For still has one bucket.
	for i := 1; i <= 5; i++ {
		code := hit("203.0.113.9:1", fmt.Sprintf("198.51.100.%d", i))
		if want := map[bool]int{true: 200, false: 429}[i <= 3]; code != want {
			t.Fatalf("direct request %d: %d, want %d", i, code, want)
		}
	}
	// So does a client coming through the published port (gateway address).
	for i := 1; i <= 4; i++ {
		code := hit("172.28.0.1:1", fmt.Sprintf("198.51.100.%d", i))
		if want := map[bool]int{true: 200, false: 429}[i <= 3]; code != want {
			t.Fatalf("gateway request %d: %d, want %d", i, code, want)
		}
	}
	// Through the real proxy each forwarded client has its own bucket.
	for i := 1; i <= 5; i++ {
		if code := hit("172.28.255.254:1", fmt.Sprintf("198.51.100.%d", i)); code != 200 {
			t.Fatalf("proxied client %d: %d", i, code)
		}
	}
}

func TestRateLimiterMemoryIsBounded(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	l := newRateLimiter(RateLimitConfig{Window: time.Minute, Limit: 5, MaxKeys: 100}, func() time.Time { return now })

	// Expired buckets are evicted on the next hit.
	for i := range 50 {
		l.hit(fmt.Sprintf("10.0.0.%d", i))
	}
	if n := l.size(); n != 50 {
		t.Fatalf("size = %d, want 50", n)
	}
	now = now.Add(time.Minute)
	l.hit("10.1.0.1")
	if n := l.size(); n != 1 {
		t.Fatalf("after the window: size = %d, want 1", n)
	}

	// The periodic sweep drops expired buckets without any new hit.
	now = now.Add(time.Minute)
	l.mu.Lock()
	l.evictExpired(now)
	l.mu.Unlock()
	if n := l.size(); n != 0 {
		t.Fatalf("after sweep: size = %d, want 0", n)
	}

	// A flood of distinct keys inside one window never exceeds MaxKeys, and the
	// bucket whose window started first is the one forgotten.
	l.hit("victim")
	now = now.Add(time.Second)
	for i := range 99 {
		l.hit(fmt.Sprintf("flood-%d", i))
	}
	if hits, _ := l.hit("victim"); hits != 2 {
		t.Fatalf("victim bucket lost before the cap: hits = %d", hits)
	}
	for i := range 100_000 {
		l.hit(fmt.Sprintf("flood2-%d", i))
		if i%10_000 == 0 && l.size() > 100 {
			t.Fatalf("size %d exceeds MaxKeys", l.size())
		}
	}
	if n := l.size(); n != 100 {
		t.Fatalf("size = %d, want 100", n)
	}
	if hits, _ := l.hit("victim"); hits != 1 {
		t.Fatalf("victim bucket should have been evicted (hits = %d)", hits)
	}
	// Live buckets keep counting under the cap.
	for range 4 {
		l.hit("victim")
	}
	if hits, _ := l.hit("victim"); hits != 6 {
		t.Fatalf("victim hits = %d, want 6", hits)
	}
	if NewRateLimiter(t.Context(), RateLimitConfig{Window: time.Minute}).cfg.MaxKeys != DefaultRateLimitMaxKeys {
		t.Fatal("MaxKeys default not applied")
	}
}
