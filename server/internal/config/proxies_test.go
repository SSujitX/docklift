package config

import (
	"net/netip"
	"reflect"
	"testing"
)

func prefixes(t *testing.T, ss ...string) []netip.Prefix {
	t.Helper()
	var out []netip.Prefix
	for _, s := range ss {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}

func TestParseTrustedProxies(t *testing.T) {
	cases := []struct {
		name     string
		spec     string
		trusted  []string
		excluded []string
	}{
		{"default", DefaultTrustedProxies,
			[]string{"127.0.0.0/8", "::1/128", "172.28.0.0/16", "fd12:3456:789a::/64"},
			[]string{"172.28.0.1/32", "fd12:3456:789a::1/128"}},
		{"none", "none", nil, nil},
		{"none is case-insensitive and trimmed", "  NONE ", nil, nil},
		{"single addresses become host prefixes", "10.0.0.5, 2001:db8::1", []string{"10.0.0.5/32", "2001:db8::1/128"}, nil},
		{"prefixes are masked", "10.1.2.3/8", []string{"10.0.0.0/8"}, nil},
		{"empty entries skipped", ",10.0.0.0/8,,", []string{"10.0.0.0/8"}, nil},
		{"exclusions", "10.0.0.0/8,!10.0.0.1, ! 10.9.0.0/16", []string{"10.0.0.0/8"}, []string{"10.0.0.1/32", "10.9.0.0/16"}},
		{"IPv4-mapped address unmapped", "::ffff:10.0.0.1", []string{"10.0.0.1/32"}, nil},
		{"IPv4-mapped prefix unmapped", "::ffff:10.0.0.0/104", []string{"10.0.0.0/8"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseTrustedProxies(c.spec)
			if err != nil {
				t.Fatal(err)
			}
			want := ProxyTrust{Trusted: prefixes(t, c.trusted...), Excluded: prefixes(t, c.excluded...)}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %+v, want %+v", got, want)
			}
		})
	}
}

func TestParseTrustedProxiesRejects(t *testing.T) {
	for _, spec := range []string{
		"proxy.local", "10.0.0.0/33", "10.0.0.256", "!", "none,10.0.0.1", "10.0.0.1,none",
		"fe80::1%eth0", "::ffff:0:0/80", "10.0.0.1:80", "*",
	} {
		if _, err := ParseTrustedProxies(spec); err == nil {
			t.Errorf("%q: want error", spec)
		}
	}
}

func TestTrustedProxiesFromEnv(t *testing.T) {
	c, err := FromEnv(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	def, _ := ParseTrustedProxies(DefaultTrustedProxies)
	if !reflect.DeepEqual(c.TrustedProxies, def) {
		t.Fatalf("default TrustedProxies = %+v", c.TrustedProxies)
	}
	c, err = FromEnv(env(map[string]string{"TRUSTED_PROXIES": "192.168.1.10"}))
	if err != nil {
		t.Fatal(err)
	}
	if want := prefixes(t, "192.168.1.10/32"); !reflect.DeepEqual(c.TrustedProxies.Trusted, want) || c.TrustedProxies.Excluded != nil {
		t.Fatalf("TrustedProxies = %+v", c.TrustedProxies)
	}
	if _, err := FromEnv(env(map[string]string{"TRUSTED_PROXIES": "not-an-ip"})); err == nil {
		t.Fatal("invalid TRUSTED_PROXIES must fail config loading")
	}
}
