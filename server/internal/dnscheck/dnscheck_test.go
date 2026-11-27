package dnscheck

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type answer struct {
	addrs []string
	err   error
	delay time.Duration
}

type fakeResolver struct {
	mu    sync.Mutex
	byKey map[string]answer
	calls []string
}

func (f *fakeResolver) LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error) {
	f.mu.Lock()
	f.calls = append(f.calls, network+" "+host)
	a, ok := f.byKey[network+" "+host]
	f.mu.Unlock()
	if !ok {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	if a.delay > 0 {
		select {
		case <-time.After(a.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if a.err != nil {
		return nil, a.err
	}
	out := make([]netip.Addr, 0, len(a.addrs))
	for _, s := range a.addrs {
		out = append(out, netip.MustParseAddr(s))
	}
	return out, nil
}

func notFound(host string) error {
	return &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

// ipServer serves the ipify JSON shape over TLS.
func ipServer(t *testing.T, body string, status int, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits != nil {
			hits.Add(1)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newChecker(t *testing.T, r Resolver, ip string) *Checker {
	t.Helper()
	if ip == "" {
		return New(WithResolver(r), WithIPSource("https://127.0.0.1:1/", nil), WithTimeouts(time.Second, 200*time.Millisecond))
	}
	srv := ipServer(t, `{"ip":"`+ip+`"}`, 200, nil)
	return New(WithResolver(r), WithIPSource(srv.URL, srv.Client()))
}

func TestCheckDomainClassification(t *testing.T) {
	const host = "app.example.com"
	servfail := &net.DNSError{Err: "server misbehaving", Name: host, IsTemporary: true}
	timeout := &net.DNSError{Err: "i/o timeout", Name: host, IsTimeout: true, IsNotFound: true}
	tests := []struct {
		name     string
		v4, v6   answer
		serverIP string
		status   Status
		message  string
		a, aaaa  []string
	}{
		{"ok", answer{addrs: []string{"203.0.113.7"}}, answer{err: notFound(host)}, "203.0.113.7", StatusOK,
			"app.example.com points at this server (203.0.113.7).", []string{"203.0.113.7"}, []string{}},
		{"ok among several", answer{addrs: []string{"198.51.100.1", "203.0.113.7"}}, answer{addrs: []string{"2001:db8::1"}}, "203.0.113.7", StatusOK,
			"app.example.com points at this server (203.0.113.7).", []string{"198.51.100.1", "203.0.113.7"}, []string{"2001:db8::1"}},
		{"mismatch", answer{addrs: []string{"198.51.100.1"}}, answer{addrs: []string{"2001:db8::1"}}, "203.0.113.7", StatusMismatch,
			"app.example.com resolves to 198.51.100.1, 2001:db8::1, not this server (203.0.113.7). If Cloudflare's proxy is enabled this is expected — otherwise fix the A record.",
			[]string{"198.51.100.1"}, []string{"2001:db8::1"}},
		{"aaaa only is a mismatch against an IPv4 server", answer{err: notFound(host)}, answer{addrs: []string{"2001:db8::7"}}, "203.0.113.7", StatusMismatch,
			"app.example.com resolves to 2001:db8::7, not this server (203.0.113.7). If Cloudflare's proxy is enabled this is expected — otherwise fix the A record.",
			[]string{}, []string{"2001:db8::7"}},
		{"missing", answer{err: notFound(host)}, answer{err: notFound(host)}, "203.0.113.7", StatusMissing,
			"No A or AAAA record for app.example.com. Add an A record pointing at 203.0.113.7, then retry.", []string{}, []string{}},
		{"missing without server ip", answer{err: notFound(host)}, answer{err: notFound(host)}, "", StatusMissing,
			"No A or AAAA record for app.example.com. Add an A record pointing at this server, then retry.", []string{}, []string{}},
		{"empty answer counts as no record", answer{addrs: []string{}}, answer{err: notFound(host)}, "203.0.113.7", StatusMissing,
			"No A or AAAA record for app.example.com. Add an A record pointing at 203.0.113.7, then retry.", []string{}, []string{}},
		{"servfail is unknown", answer{err: servfail}, answer{err: servfail}, "203.0.113.7", StatusUnknown,
			"Could not resolve app.example.com from this server (DNS lookup failed). Issuance will still be attempted.", []string{}, []string{}},
		{"timeout flagged not-found is still a failure", answer{err: timeout}, answer{err: notFound(host)}, "203.0.113.7", StatusUnknown,
			"Could not resolve app.example.com from this server (DNS lookup failed). Issuance will still be attempted.", []string{}, []string{}},
		{"generic error is unknown", answer{err: errors.New("boom")}, answer{err: notFound(host)}, "203.0.113.7", StatusUnknown,
			"Could not resolve app.example.com from this server (DNS lookup failed). Issuance will still be attempted.", []string{}, []string{}},
		{"resolves but no server ip", answer{addrs: []string{"198.51.100.1"}}, answer{err: servfail}, "", StatusUnknown,
			"app.example.com resolves to 198.51.100.1. This server's public IP could not be confirmed.", []string{"198.51.100.1"}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &fakeResolver{byKey: map[string]answer{"ip4 " + host: tt.v4, "ip6 " + host: tt.v6}}
			c := newChecker(t, r, tt.serverIP)
			got := c.CheckDomain(context.Background(), "  App.Example.COM \n")
			if got.Status != tt.status || got.Message != tt.message {
				t.Fatalf("got %q %q, want %q %q", got.Status, got.Message, tt.status, tt.message)
			}
			if got.Domain != host {
				t.Fatalf("domain %q", got.Domain)
			}
			if strings.Join(got.A, ",") != strings.Join(tt.a, ",") || strings.Join(got.AAAA, ",") != strings.Join(tt.aaaa, ",") {
				t.Fatalf("records a=%v aaaa=%v", got.A, got.AAAA)
			}
			if got.A == nil || got.AAAA == nil {
				t.Fatal("record lists must serialise as [] not null")
			}
			if (tt.serverIP == "") != (got.ServerIP == nil) || (got.ServerIP != nil && *got.ServerIP != tt.serverIP) {
				t.Fatalf("serverIp %v", got.ServerIP)
			}
		})
	}
}

func TestLookupTimeout(t *testing.T) {
	const host = "slow.example.com"
	r := &fakeResolver{byKey: map[string]answer{
		"ip4 " + host: {addrs: []string{"203.0.113.7"}, delay: time.Second},
		"ip6 " + host: {err: notFound(host)},
	}}
	c := New(WithResolver(r), WithIPSource("https://127.0.0.1:1/", nil), WithTimeouts(50*time.Millisecond, 50*time.Millisecond))
	start := time.Now()
	got := c.CheckDomain(context.Background(), host)
	if got.Status != StatusUnknown {
		t.Fatalf("status %q", got.Status)
	}
	if time.Since(start) > 800*time.Millisecond {
		t.Fatal("lookup timeout not applied")
	}
}

func TestPublicIPCacheAndHTTPSOnly(t *testing.T) {
	var hits atomic.Int32
	srv := ipServer(t, `{"ip":"203.0.113.9"}`, 200, &hits)
	now := time.Unix(1_700_000_000, 0)
	c := New(WithIPSource(srv.URL, srv.Client()), WithClock(func() time.Time { return now }))
	for range 3 {
		if ip := c.GetServerPublicIP(context.Background()); ip == nil || *ip != "203.0.113.9" {
			t.Fatalf("ip %v", ip)
		}
	}
	if hits.Load() != 1 {
		t.Fatalf("cached lookups hit the server %d times", hits.Load())
	}
	now = now.Add(IPCacheTTL)
	c.GetServerPublicIP(context.Background())
	if hits.Load() != 2 {
		t.Fatalf("expired cache not refreshed (%d hits)", hits.Load())
	}

	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ip":"203.0.113.9"}`))
	}))
	defer plain.Close()
	if ip := New(WithIPSource(plain.URL, nil)).GetServerPublicIP(context.Background()); ip != nil {
		t.Fatalf("plain HTTP source accepted: %v", *ip)
	}

	redirect := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL, http.StatusFound)
	}))
	defer redirect.Close()
	if ip := New(WithIPSource(redirect.URL, redirect.Client())).GetServerPublicIP(context.Background()); ip != nil {
		t.Fatalf("redirect to HTTP followed: %v", *ip)
	}
}

func TestPublicIPRejectsBadResponses(t *testing.T) {
	for name, tc := range map[string]struct {
		body   string
		status int
	}{
		"server error": {`{"ip":"203.0.113.9"}`, 500},
		"not json":     {`<html>`, 200},
		"no ip":        {`{}`, 200},
		"not an ip":    {`{"ip":"evil; rm -rf"}`, 200},
	} {
		t.Run(name, func(t *testing.T) {
			srv := ipServer(t, tc.body, tc.status, nil)
			if ip := New(WithIPSource(srv.URL, srv.Client())).GetServerPublicIP(context.Background()); ip != nil {
				t.Fatalf("got %q", *ip)
			}
		})
	}
}

func TestFailedLookupIsCachedButKeepsLastGoodIP(t *testing.T) {
	var fail atomic.Bool
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			w.WriteHeader(503)
			return
		}
		_, _ = w.Write([]byte(`{"ip":"2001:db8::5"}`))
	}))
	defer srv.Close()
	now := time.Unix(1_700_000_000, 0)
	c := New(WithIPSource(srv.URL, srv.Client()), WithClock(func() time.Time { return now }))
	if ip := c.GetServerPublicIP(context.Background()); ip == nil || *ip != "2001:db8::5" {
		t.Fatalf("ip %v", ip)
	}
	fail.Store(true)
	now = now.Add(IPCacheTTL + time.Second)
	if ip := c.GetServerPublicIP(context.Background()); ip == nil || *ip != "2001:db8::5" {
		t.Fatalf("a failed refresh must keep the last known IP, got %v", ip)
	}
}

func TestCheckDomainsSequentialOrder(t *testing.T) {
	r := &fakeResolver{byKey: map[string]answer{
		"ip4 a.example.com": {addrs: []string{"203.0.113.7"}},
		"ip4 b.example.com": {addrs: []string{"198.51.100.2"}},
	}}
	c := newChecker(t, r, "203.0.113.7")
	got := c.CheckDomains(context.Background(), []string{"a.example.com", "b.example.com", "c.example.com"})
	want := []Status{StatusOK, StatusMismatch, StatusMissing}
	if len(got) != 3 {
		t.Fatalf("got %d checks", len(got))
	}
	for i, s := range want {
		if got[i].Status != s {
			t.Fatalf("check %d: %q want %q", i, got[i].Status, s)
		}
	}
	if len(c.CheckDomains(context.Background(), nil)) != 0 {
		t.Fatal("empty input")
	}
}
