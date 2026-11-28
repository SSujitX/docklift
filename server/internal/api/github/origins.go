package github

import (
	"net/http"
	"regexp"
	"strings"
)

var (
	trailingPathRe = regexp.MustCompile(`[/?#].*$`)
	dottedIPv4Re   = regexp.MustCompile(`^(\d{1,3}\.){3}\d{1,3}$`)
)

// isLoopbackHost reports localhost, 127.0.0.1 and ::1 (brackets ignored).
func isLoopbackHost(hostname string) bool {
	h := strings.TrimSuffix(strings.TrimPrefix(strings.ToLower(hostname), "["), "]")
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

// publicBaseURL is resolveGithubPublicBaseUrl: the origin GitHub should call
// back to. Hosts other than loopback and dotted IPv4 addresses are always
// https; the fallback is the configured frontend origin.
func (h *handlers) publicBaseURL(r *http.Request) string {
	fallback := "http://127.0.0.1:8080"
	if fe, ok := parseWebURL(h.d.Config.FrontendURL); ok {
		fallback = fe.Origin()
	}
	rawHost := trailingPathRe.ReplaceAllString(requestHost(r), "")
	if rawHost == "" {
		return fallback
	}
	parsed, ok := parseWebURL("http://" + rawHost)
	if !ok {
		return fallback
	}
	hostname := parsed.Hostname()
	isLocal := isLoopbackHost(hostname) || dottedIPv4Re.MatchString(hostname)
	protocol := "https"
	if isLocal && strings.ToLower(requestProto(r)) != "https" {
		protocol = "http"
	}
	u, ok := parseWebURL(protocol + "://" + rawHost)
	if !ok {
		return fallback
	}
	return u.Origin()
}

// allowedReturnOrigins is allowedGithubReturnOrigins: the configured
// frontend, this request's own origin and the CORS_ORIGIN entries, with
// loopback origins expanded to localhost/127.0.0.1/[::1] on the same port.
// Only http(s) origins are added (2.x also admitted the
// opaque "null" origin of other schemes).
func (h *handlers) allowedReturnOrigins(r *http.Request) map[string]bool {
	origins := map[string]bool{}
	add := func(value string) {
		if value == "" {
			return
		}
		if !strings.Contains(value, "://") {
			value = "http://" + value
		}
		u, ok := parseWebURL(value)
		if !ok {
			return
		}
		origins[u.Origin()] = true
		if isLoopbackHost(u.Hostname()) {
			port := ""
			if p := u.Port(); p != "" {
				port = ":" + p
			}
			for _, host := range []string{"localhost", "127.0.0.1", "[::1]"} {
				origins[u.u.Scheme+"://"+host+port] = true
			}
		}
	}
	add(h.d.Config.FrontendURL)
	if host := requestHost(r); host != "" {
		add(requestProto(r) + "://" + host)
	}
	for _, part := range h.d.Config.CORSOrigins {
		add(strings.TrimSpace(part))
	}
	return origins
}

// sanitizeReturnURL is sanitizeGithubReturnUrl: raw when it is a relative
// path or absolute http(s) URL on an allowed origin, else the frontend's
// settings page. isString is false when the value was not a string.
func (h *handlers) sanitizeReturnURL(raw string, isString bool, r *http.Request) string {
	fallback := h.d.Config.FrontendURL + "/settings"
	value := strings.TrimSpace(raw)
	if !isString || value == "" {
		return fallback
	}
	allowed := h.allowedReturnOrigins(r)
	if strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") {
		var bases []string
		if host := requestHost(r); host != "" {
			bases = append(bases, requestProto(r)+"://"+host)
		}
		bases = append(bases, h.d.Config.FrontendURL)
		for _, b := range bases {
			base, ok := parseWebURL(b)
			if !ok {
				return fallback
			}
			resolved, ok := base.resolve(value)
			if !ok {
				return fallback
			}
			if allowed[resolved.Origin()] {
				return resolved.String()
			}
		}
		return fallback
	}
	u, ok := parseWebURL(value)
	if ok && allowed[u.Origin()] {
		return u.String()
	}
	return fallback
}
