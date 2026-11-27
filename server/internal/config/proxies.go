package config

import (
	"fmt"
	"net/netip"
	"strings"
)

// DefaultTrustedProxies is the TRUSTED_PROXIES default, the value
// docker-compose.yml sets: loopback plus the fixed addresses of
// docklift-nginx-proxy on docklift_network (ipv4_address / ipv6_address in
// docker-compose.yml). Only the proxy is trusted, not the whole subnet: a
// deployed app's container on docklift_network must not be able to forge
// X-Forwarded-For (login rate limits) or X-Forwarded-Proto/-Host (origin
// checks). The network gateway, which is how clients of the published
// dashboard port appear, is never trusted.
const DefaultTrustedProxies = "127.0.0.0/8,::1/128,172.28.255.254,fd12:3456:789a::ffff:fffe"

// ProxyTrust is the parsed TRUSTED_PROXIES list: a socket peer is a trusted
// proxy when it is inside one of Trusted and inside none of Excluded.
type ProxyTrust struct {
	Trusted  []netip.Prefix
	Excluded []netip.Prefix
}

// ParseTrustedProxies parses a TRUSTED_PROXIES value: comma-separated IP
// addresses or CIDR prefixes (IPv4 or IPv6); an entry prefixed with "!"
// excludes that address or prefix again. The single entry "none" trusts no
// peer (forwarded headers are always ignored). Empty entries are skipped.
// IPv4-mapped IPv6 entries are stored as IPv4, matching how peers compare.
func ParseTrustedProxies(spec string) (ProxyTrust, error) {
	var pt ProxyTrust
	if strings.EqualFold(strings.TrimSpace(spec), "none") {
		return pt, nil
	}
	for _, raw := range strings.Split(spec, ",") {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		exclude := strings.HasPrefix(entry, "!")
		p, err := parseProxyEntry(strings.TrimSpace(strings.TrimPrefix(entry, "!")))
		if err != nil {
			return ProxyTrust{}, fmt.Errorf("TRUSTED_PROXIES entry %q: %w", entry, err)
		}
		if exclude {
			pt.Excluded = append(pt.Excluded, p)
		} else {
			pt.Trusted = append(pt.Trusted, p)
		}
	}
	return pt, nil
}

func parseProxyEntry(s string) (netip.Prefix, error) {
	if s == "" {
		return netip.Prefix{}, fmt.Errorf("empty address")
	}
	if strings.EqualFold(s, "none") {
		return netip.Prefix{}, fmt.Errorf(`"none" must be the only entry`)
	}
	if !strings.Contains(s, "/") {
		addr, err := netip.ParseAddr(s)
		if err != nil {
			return netip.Prefix{}, fmt.Errorf("not an IP address or CIDR prefix")
		}
		if addr.Zone() != "" {
			return netip.Prefix{}, fmt.Errorf("IPv6 zones are not supported")
		}
		addr = addr.Unmap()
		return netip.PrefixFrom(addr, addr.BitLen()), nil
	}
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("not an IP address or CIDR prefix")
	}
	if p.Addr().Is4In6() {
		if p.Bits() < 96 {
			return netip.Prefix{}, fmt.Errorf("IPv4-mapped prefix shorter than /96")
		}
		p = netip.PrefixFrom(p.Addr().Unmap(), p.Bits()-96)
	}
	return p.Masked(), nil
}
