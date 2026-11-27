package github

import (
	"fmt"
	"html"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"docklift/internal/httpx"
)

// Express/WHATWG behaviour the GitHub routes depend on: req.query values,
// cookies, URL origins and serialisation, js.EncodeURIComponent and
// res.redirect.

// queryString is `typeof req.query[key] === 'string' ? req.query[key] : undefined`:
// a parameter given more than once is an array in Express, not a string.
func queryString(r *http.Request, key string) (string, bool) {
	vs := r.URL.Query()[key]
	if len(vs) != 1 {
		return "", false
	}
	return vs[0], true
}

// queryCast is `req.query[key] as string` used in a template or test: a
// repeated parameter becomes its values joined with commas (Array#toString).
func queryCast(r *http.Request, key string) string {
	return strings.Join(r.URL.Query()[key], ",")
}

// readCookie is the hand-rolled cookie reader of github.ts: the first
// `name=value` pair whose trimmed name matches, value decodeURIComponent-ed.
// A malformed percent-encoding is an error (decodeURIComponent throws).
func readCookie(r *http.Request, name string) (string, error) {
	raw := strings.Join(r.Header.Values("Cookie"), "; ")
	if raw == "" {
		return "", nil
	}
	for _, part := range strings.Split(raw, ";") {
		idx := strings.IndexByte(part, '=')
		if idx == -1 {
			continue
		}
		if strings.TrimSpace(part[:idx]) == name {
			return decodeURIComponent(strings.TrimSpace(part[idx+1:]))
		}
	}
	return "", nil
}

// decodeURIComponent decodes %XX sequences; it fails on a malformed escape or
// on bytes that are not valid UTF-8, like the JavaScript function.
func decodeURIComponent(s string) (string, error) {
	if !strings.Contains(s, "%") {
		return s, nil
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			b.WriteByte(s[i])
			continue
		}
		if i+2 >= len(s) || !isHex(s[i+1]) || !isHex(s[i+2]) {
			return "", fmt.Errorf("URI malformed")
		}
		n, _ := strconv.ParseUint(s[i+1:i+3], 16, 8)
		b.WriteByte(byte(n))
		i += 2
	}
	out := b.String()
	if !utf8.ValidString(out) {
		return "", fmt.Errorf("URI malformed")
	}
	return out, nil
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

const upperHex = "0123456789ABCDEF"

func isAlnum(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// formEncode is URLSearchParams serialisation of one value
// (application/x-www-form-urlencoded: space is '+', only *-._ and
// alphanumerics are kept).
func formEncode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case isAlnum(c) || strings.IndexByte("*-._", c) >= 0:
			b.WriteByte(c)
		case c == ' ':
			b.WriteByte('+')
		default:
			b.WriteByte('%')
			b.WriteByte(upperHex[c>>4])
			b.WriteByte(upperHex[c&15])
		}
	}
	return b.String()
}

// encodeURL is the encodeurl package Express applies to Location: characters
// outside the URL code points are percent-encoded, existing valid %XX escapes
// are kept and a stray '%' becomes %25. Invalid UTF-8 becomes U+FFFD first.
func encodeURL(s string) string {
	s = strings.ToValidUTF8(s, "�")
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '%' {
			if i+2 < len(s) && isHex(s[i+1]) && isHex(s[i+2]) {
				b.WriteByte(c)
			} else {
				b.WriteString("%25")
			}
			continue
		}
		if c == 0x21 || (c >= 0x23 && c <= 0x3B) || c == 0x3D || (c >= 0x3F && c <= 0x5F) || (c >= 0x61 && c <= 0x7A) || c == 0x7C || c == 0x7E {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(upperHex[c>>4])
		b.WriteByte(upperHex[c&15])
	}
	return b.String()
}

// redirect is Express res.redirect(url): 302 with an encoded Location and a
// short text or HTML body chosen from Accept (the address is HTML-escaped).
func redirect(w http.ResponseWriter, r *http.Request, location string) {
	address := encodeURL(location)
	h := w.Header()
	h.Set("Location", address)
	h.Add("Vary", "Accept")
	var body string
	switch negotiateRedirect(r.Header.Get("Accept")) {
	case "text":
		h.Set("Content-Type", "text/plain; charset=utf-8")
		body = "Found. Redirecting to " + address
	case "html":
		h.Set("Content-Type", "text/html; charset=utf-8")
		body = "<!DOCTYPE html><head><title>Found</title></head><body><p>Found. Redirecting to " + html.EscapeString(address) + "</p></body>"
	}
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusFound)
	if r.Method != http.MethodHead {
		_, _ = w.Write([]byte(body))
	}
}

// negotiateRedirect is res.format({text, html, default}): "text" or "html" by
// Accept quality (text wins ties and an absent header), "" when neither is
// acceptable.
func negotiateRedirect(accept string) string {
	if strings.TrimSpace(accept) == "" {
		return "text"
	}
	quality := func(typ, sub string) float64 {
		best, bestSpec := 0.0, -1
		for _, part := range strings.Split(accept, ",") {
			mt, params, err := mime.ParseMediaType(strings.TrimSpace(part))
			if err != nil {
				continue
			}
			t, s, _ := strings.Cut(mt, "/")
			spec := -1
			switch {
			case t == typ && s == sub:
				spec = 2
			case t == typ && s == "*":
				spec = 1
			case t == "*" && s == "*":
				spec = 0
			}
			if spec < 0 || spec < bestSpec {
				continue
			}
			q := 1.0
			if v, ok := params["q"]; ok {
				if f, err := strconv.ParseFloat(v, 64); err == nil {
					q = f
				}
			}
			if spec > bestSpec {
				best, bestSpec = q, spec
			}
		}
		return best
	}
	text, htmlQ := quality("text", "plain"), quality("text", "html")
	switch {
	case text <= 0 && htmlQ <= 0:
		return ""
	case htmlQ > text:
		return "html"
	}
	return "text"
}

// forbiddenHost matches characters a WHATWG URL parser refuses in a host.
var forbiddenHost = regexp.MustCompile(`[\x00-\x20"#%/<>?@\[\\\]^|\x7f]`)

// webURL is a parsed absolute http(s) URL with the WHATWG normalisations the
// TypeScript code relied on (lowercase scheme and host, default port dropped,
// "/" for an empty path).
type webURL struct {
	u *url.URL
}

// parseWebURL parses an absolute http(s) URL; ok is false where `new URL`
// would throw or the scheme is not http(s).
func parseWebURL(raw string) (webURL, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" {
		return webURL{}, false
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" {
		return webURL{}, false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || forbiddenHost.MatchString(host) {
		return webURL{}, false
	}
	port := u.Port()
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n > 65535 {
			return webURL{}, false
		}
		port = strconv.Itoa(n)
		if (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443") {
			port = ""
		}
	}
	hostPort := host
	if strings.Contains(host, ":") {
		hostPort = "[" + host + "]"
	}
	if port != "" {
		hostPort += ":" + port
	}
	u.Host = hostPort
	if u.Path == "" && u.RawPath == "" {
		u.Path = "/"
	}
	return webURL{u: u}, true
}

// Origin is URL#origin.
func (w webURL) Origin() string { return w.u.Scheme + "://" + w.u.Host }

// Hostname is URL#hostname without IPv6 brackets.
func (w webURL) Hostname() string { return w.u.Hostname() }

// Port is URL#port ("" for the scheme default).
func (w webURL) Port() string { return w.u.Port() }

// String is URL#toString.
func (w webURL) String() string { return w.u.String() }

// resolve is `new URL(ref, w)`. Backslashes before the query are slashes,
// as for every special scheme in the WHATWG parser.
func (w webURL) resolve(ref string) (webURL, bool) {
	end := strings.IndexAny(ref, "?#")
	if end == -1 {
		end = len(ref)
	}
	ref = strings.ReplaceAll(ref[:end], `\`, "/") + ref[end:]
	rel, err := url.Parse(ref)
	if err != nil {
		return webURL{}, false
	}
	return parseWebURL(w.u.ResolveReference(rel).String())
}

// requestHost is `(req.headers['x-forwarded-host'] || req.headers.host || ”)`
// split on commas, first part trimmed. X-Forwarded-Host counts only from a
// trusted proxy (deliberate fix 11).
func requestHost(r *http.Request) string {
	if httpx.PeerTrusted(r) {
		if v := strings.Join(r.Header.Values("X-Forwarded-Host"), ", "); v != "" {
			first, _, _ := strings.Cut(v, ",")
			return strings.TrimSpace(first)
		}
	}
	first, _, _ := strings.Cut(r.Host, ",")
	return strings.TrimSpace(first)
}

// requestProto is `(req.headers['x-forwarded-proto'] || req.protocol)` first
// value, trimmed (forwarded only from a trusted proxy).
func requestProto(r *http.Request) string {
	return httpx.RequestProtocol(r)
}
