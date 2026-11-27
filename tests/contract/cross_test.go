package contract

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func TestHealth(t *testing.T) {
	c := mainInst.Anon()
	first := c.Get(t, "/api/health").Expect(t, 200, Obj{"status": "ok", "timestamp": ISOms, "version": NonEmpty, "instanceId": UUID}).(map[string]any)
	second := c.Get(t, "/api/health").Expect(t, 200, Obj{"status": "ok", "timestamp": ISOms, "version": first["version"], "instanceId": first["instanceId"]})
	_ = second
	// Version reported by health and by /api/system/version is the package version.
	admin.Client().Get(t, "/api/system/version").Expect(t, 200, Obj{
		"current": first["version"], "latest": NonEmpty, "updateAvailable": Bool, "githubOk": Bool, "checkedAt": ISOms,
	})
	// Health is public and never rate limited.
	r := c.Get(t, "/api/health")
	if r.Header.Get("RateLimit-Limit") != "" {
		t.Fatalf("health must not carry rate-limit headers: %v", r.Header)
	}
}

var securityHeaders = map[string]string{
	"X-Content-Type-Options": "nosniff",
	"X-Frame-Options":        "SAMEORIGIN",
	"X-XSS-Protection":       "1; mode=block",
	"Referrer-Policy":        "strict-origin-when-cross-origin",
	"Permissions-Policy":     "camera=(), microphone=(), geolocation=()",
}

func assertSecurityHeaders(t *testing.T, r *Resp) {
	t.Helper()
	for k, want := range securityHeaders {
		vals := r.Header.Values(k)
		found := false
		for _, v := range vals {
			if v == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: header %s = %q, want %q", r.Path, k, vals, want)
		}
	}
}

func TestSecurityHeaders(t *testing.T) {
	c := mainInst.Anon()
	for _, r := range []*Resp{
		c.Get(t, "/api/health"),
		c.Get(t, "/api/projects"), // 401 from auth middleware
		c.Get(t, "/api/does-not-exist"),
		admin.Client().Get(t, "/api/projects"),
		c.Post(t, "/api/auth/login", map[string]any{}),
	} {
		assertSecurityHeaders(t, r)
		if v := r.Header.Get("Strict-Transport-Security"); v != "" {
			t.Errorf("%s over plain http must not send HSTS, got %q", r.Path, v)
		}
	}
	r := c.Get(t, "/api/health", "X-Forwarded-Proto", "https")
	assertSecurityHeaders(t, r)
	if got := r.Header.Get("Strict-Transport-Security"); got != "max-age=31536000; includeSubDomains" {
		t.Fatalf("HSTS behind https proxy = %q", got)
	}
	if got := c.Get(t, "/api/health", "X-Forwarded-Proto", "http").Header.Get("Strict-Transport-Security"); got != "" {
		t.Fatalf("HSTS with X-Forwarded-Proto: http = %q", got)
	}
}

func TestCORS(t *testing.T) {
	c := mainInst.Anon()
	self := mainInst.BaseURL // http://127.0.0.1:<port>, same as the Host header

	allowed := func(t *testing.T, origin string, extra ...string) {
		t.Helper()
		r := c.Get(t, "/api/health", append([]string{"Origin", origin}, extra...)...)
		r.ExpectStatus(t, 200)
		if got := r.Header.Get("Access-Control-Allow-Origin"); got != origin {
			t.Fatalf("Origin %s: Access-Control-Allow-Origin = %q, want echo", origin, got)
		}
		if got := r.Header.Get("Access-Control-Allow-Credentials"); got != "true" {
			t.Fatalf("Origin %s: Access-Control-Allow-Credentials = %q", origin, got)
		}
		if !strings.Contains(r.Header.Get("Vary"), "Origin") {
			t.Fatalf("Origin %s: Vary = %q, want Origin", origin, r.Header.Get("Vary"))
		}
	}
	denied := func(t *testing.T, origin string, extra ...string) {
		t.Helper()
		r := c.Get(t, "/api/health", append([]string{"Origin", origin}, extra...)...)
		// Denied origins still get the response (CORS is enforced by the browser).
		r.ExpectStatus(t, 200)
		if got := r.Header.Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("Origin %s must be denied, got Access-Control-Allow-Origin %q", origin, got)
		}
		if got := r.Header.Get("Access-Control-Allow-Credentials"); got != "" {
			t.Fatalf("Origin %s must be denied, got Access-Control-Allow-Credentials %q", origin, got)
		}
	}

	t.Run("same origin as Host", func(t *testing.T) { allowed(t, self) })
	t.Run("different port on same host", func(t *testing.T) { denied(t, "http://127.0.0.1:5500") })
	t.Run("localhost alias of 127.0.0.1 is a different origin", func(t *testing.T) {
		u, _ := url.Parse(self)
		denied(t, "http://localhost:"+u.Port())
	})
	t.Run("foreign origin", func(t *testing.T) { denied(t, "http://evil.example") })
	t.Run("opaque null origin", func(t *testing.T) { denied(t, "null") })
	t.Run("non-http scheme", func(t *testing.T) { denied(t, "ftp://127.0.0.1") })
	t.Run("forwarded host and proto", func(t *testing.T) {
		allowed(t, "https://panel.contract.test", "X-Forwarded-Host", "panel.contract.test", "X-Forwarded-Proto", "https")
		// Default ports on both sides tolerate a scheme mismatch (TLS-terminating proxy without X-Forwarded-Proto)
		allowed(t, "https://panel.contract.test", "X-Forwarded-Host", "panel.contract.test")
		denied(t, "https://panel.contract.test:8443", "X-Forwarded-Host", "panel.contract.test")
		denied(t, "https://other.contract.test", "X-Forwarded-Host", "panel.contract.test", "X-Forwarded-Proto", "https")
	})
	t.Run("no Origin header", func(t *testing.T) {
		r := c.Get(t, "/api/health")
		if got := r.Header.Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("no Origin: Access-Control-Allow-Origin = %q", got)
		}
		if got := r.Header.Get("Access-Control-Allow-Credentials"); got != "true" {
			t.Fatalf("no Origin: Access-Control-Allow-Credentials = %q", got)
		}
	})
	t.Run("configured allowlist", func(t *testing.T) {
		requireBoot(t)
		allowed(t, corsAllowedOrigin)
		allowed(t, frontendURL)
		allowed(t, "HTTP://FRONTEND.TEST:3000") // normalized compare
		denied(t, "https://cors-allowed.test:4000")
	})
	t.Run("preflight allowed", func(t *testing.T) {
		r := c.Do(t, "OPTIONS", "/api/projects", nil, "Origin", self, "Access-Control-Request-Method", "DELETE",
			"Access-Control-Request-Headers", "authorization,content-type")
		r.ExpectStatus(t, 204)
		want := map[string]string{
			"Access-Control-Allow-Origin":      self,
			"Access-Control-Allow-Credentials": "true",
			"Access-Control-Allow-Methods":     "GET,HEAD,PUT,PATCH,POST,DELETE",
			"Access-Control-Allow-Headers":     "authorization,content-type",
		}
		for k, v := range want {
			if got := r.Header.Get(k); got != v {
				t.Errorf("preflight %s = %q, want %q", k, got, v)
			}
		}
		if len(r.Body) != 0 {
			t.Errorf("preflight body = %q", r.Body)
		}
	})
	t.Run("preflight denied falls through to auth", func(t *testing.T) {
		r := c.Do(t, "OPTIONS", "/api/projects", nil, "Origin", "http://evil.example", "Access-Control-Request-Method", "DELETE")
		r.ExpectError(t, 401, "Authentication required")
		if got := r.Header.Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("denied preflight got Access-Control-Allow-Origin %q", got)
		}
	})
}

func TestUnknownAPIPath(t *testing.T) {
	c := mainInst.Anon()
	if targetIsGo() {
		// Deliberate fix 2: unknown /api paths get a JSON 404.
		for _, m := range []string{"GET", "POST", "DELETE"} {
			r := c.Do(t, m, "/api/no-such-thing", nil)
			r.ExpectError(t, 404, "Not found")
			if ct := r.Header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
				t.Fatalf("%s 404 Content-Type = %q", m, ct)
			}
		}
		c.Get(t, "/api/projects/x/y/z").ExpectError(t, 401, "Authentication required")
		admin.Client().Get(t, "/api/projects/x/y/z").ExpectError(t, 404, "Not found")
		admin.Client().Do(t, "PUT", "/api/auth/me", map[string]any{}).ExpectError(t, 404, "Not found")
		return
	}
	r := c.Get(t, "/api/no-such-thing")
	r.ExpectStatus(t, 404)
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("404 Content-Type = %q, want text/html (Express default)", ct)
	}
	if !strings.Contains(string(r.Body), "<pre>Cannot GET /api/no-such-thing</pre>") {
		t.Fatalf("404 body = %q", r.Body)
	}
	r = c.Post(t, "/api/no-such-thing", map[string]any{})
	r.ExpectStatus(t, 404)
	if !strings.Contains(string(r.Body), "<pre>Cannot POST /api/no-such-thing</pre>") {
		t.Fatalf("404 body = %q", r.Body)
	}
	// Under an authenticated mount the auth middleware answers first.
	c.Get(t, "/api/projects/x/y/z").ExpectError(t, 401, "Authentication required")
	r = admin.Client().Get(t, "/api/projects/x/y/z")
	r.ExpectStatus(t, 404)
	if !strings.Contains(string(r.Body), "<pre>Cannot GET /api/projects/x/y/z</pre>") {
		t.Fatalf("404 body = %q", r.Body)
	}
	r = admin.Client().Do(t, "PUT", "/api/auth/me", map[string]any{})
	r.ExpectStatus(t, 404)
}

// TestRequestBodies covers body parsing. The Go backend answers client
// mistakes with 4xx (deliberate fix 1); TS sent them to its 500 handler.
func TestRequestBodies(t *testing.T) {
	c := mainInst.Anon()
	t.Run("malformed JSON", func(t *testing.T) {
		for _, raw := range []string{`{"email":`, `{} trailing`, `"just a string"`, `null`} {
			r := c.Post(t, "/api/auth/login", RawBody{"application/json", []byte(raw)})
			if targetIsGo() {
				r.ExpectError(t, 400, "Invalid JSON body")
			} else {
				r.ExpectError(t, 500, "Something went wrong!")
			}
		}
	})
	t.Run("urlencoded body is parsed", func(t *testing.T) {
		c.Post(t, "/api/auth/login", url.Values{"email": {admin.Email}, "password": {"wrong-password"}}).
			ExpectError(t, 401, "Invalid email or password")
		c.Post(t, "/api/auth/login", url.Values{"email": {admin.Email}, "password": {admin.Password}}).
			Expect(t, 200, Obj{"message": "Login successful", "token": JWT, "user": Obj{"id": admin.UserID, "name": admin.Name, "email": admin.Email, "role": "admin"}})
	})
	t.Run("unparsed content type leaves body undefined", func(t *testing.T) {
		r := c.Post(t, "/api/auth/login", RawBody{"text/plain", []byte("email=a@b.c&password=x")})
		if targetIsGo() {
			// Deliberate fix 3: a missing body is a missing field, not a crash.
			r.ExpectError(t, 400, "Email and password are required")
		} else {
			r.ExpectError(t, 500, "Login failed")
		}
	})
	t.Run("JSON under the 10mb limit is accepted", func(t *testing.T) {
		big := strings.Repeat("a", 9*1024*1024)
		if !targetIsGo() {
			c.Post(t, "/api/auth/login", map[string]any{"email": big + "@x.test", "password": "x"}).ExpectError(t, 401, "Invalid email or password")
			return
		}
		// Go parses bodies only after the auth guard: the 10mb limit applies
		// to authenticated routes, unauthenticated ones (login, register,
		// GitHub callbacks) accept 64 KB.
		admin.Client().Patch(t, "/api/projects/"+missingProjectID, map[string]any{"name": big}).ExpectError(t, 404, "Project not found")
		c.Post(t, "/api/auth/login", map[string]any{"email": big + "@x.test", "password": "x"}).ExpectError(t, 413, "Request body too large")
		small := strings.Repeat("a", 60*1024)
		c.Post(t, "/api/auth/login", map[string]any{"email": small + "@x.test", "password": "x"}).ExpectError(t, 401, "Invalid email or password")
	})
	t.Run("unauthenticated bodies are not parsed", func(t *testing.T) {
		// TS parsed (and on errors 500ed) the body before the auth guard.
		r := c.Post(t, "/api/projects", RawBody{"application/json", []byte(`{"name":`)})
		if targetIsGo() {
			r.ExpectError(t, 401, "Authentication required")
		} else {
			r.ExpectError(t, 500, "Something went wrong!")
		}
	})
	tooLarge := func(t *testing.T, r *Resp) {
		t.Helper()
		if targetIsGo() {
			r.ExpectError(t, 413, "Request body too large")
		} else {
			r.ExpectError(t, 500, "Something went wrong!")
		}
	}
	t.Run("JSON over the 10mb limit", func(t *testing.T) {
		big := strings.Repeat("a", 11*1024*1024)
		tooLarge(t, c.Post(t, "/api/auth/login", RawBody{"application/json", []byte(fmt.Sprintf(`{"email":"%s"}`, big))}))
	})
	t.Run("urlencoded over the 10mb limit", func(t *testing.T) {
		big := strings.Repeat("a", 11*1024*1024)
		tooLarge(t, c.Post(t, "/api/auth/login", RawBody{"application/x-www-form-urlencoded", []byte("email=" + big)}))
	})
}

func Test401Shapes(t *testing.T) {
	c := mainInst.Anon()
	protected := []string{"/api/projects", "/api/deployments/x", "/api/databases", "/api/files/x", "/api/ports",
		"/api/github/status", "/api/system/stats", "/api/domains", "/api/logs/x/containers", "/api/backup", "/api/auth/me"}
	for _, p := range protected {
		c.Get(t, p).ExpectError(t, 401, "Authentication required")
		c.Get(t, p, "Authorization", "Basic YTpi").ExpectError(t, 401, "Authentication required")
		c.Get(t, p, "Authorization", "Bearer").ExpectError(t, 401, "Authentication required")
		c.Get(t, p, "Authorization", "Bearer not.a.jwt").ExpectError(t, 401, "Invalid token")
		c.Get(t, p, "Authorization", "Bearer "+signJWT("wrong-secret", map[string]any{"userId": admin.UserID, "email": admin.Email, "role": "admin", "pwdv": 0})).
			ExpectError(t, 401, "Invalid token")
	}
	// Query tokens never authorize normal routes.
	sse := admin.Client().Post(t, "/api/auth/sse-token", nil).Obj(t)["token"].(string)
	c.Get(t, "/api/projects?token="+sse).ExpectError(t, 401, "Authentication required")
	c.Get(t, "/api/system/stats?token="+sse).ExpectError(t, 401, "Authentication required")
}

// TestTrustedProxies covers deliberate fix 11 (Go only: Express ran with
// trust proxy = 1 and believed X-Forwarded-* from every peer). The instance
// trusts loopback except 127.0.0.1, which stands in for the Docker network
// gateway a client of the published dashboard port appears as: requests from
// 127.0.0.1 are "direct" and their forwarded headers must be ignored, while
// ::1 plays the trusted docklift-nginx-proxy.
func TestTrustedProxies(t *testing.T) {
	if !targetIsGo() {
		t.Skip("the TS backend trusts X-Forwarded-* from every peer (trust proxy = 1)")
	}
	inst := dedicatedInstance(t, BootOptions{Name: "proxytrust", ExtraEnv: map[string]string{
		"TRUSTED_PROXIES": "127.0.0.0/8,::1,!127.0.0.1",
	}})
	remaining := func(t *testing.T, c *Client) string {
		t.Helper()
		return c.Post(t, "/api/auth/login", map[string]any{}).Header.Get("RateLimit-Remaining")
	}

	t.Run("spoofed X-Forwarded-For shares the peer's bucket", func(t *testing.T) {
		// Every Anon client sends its own X-Forwarded-For; all of them are 127.0.0.1 here.
		for i, want := range []string{"99", "98", "97"} {
			if got := remaining(t, inst.Anon()); got != want {
				t.Fatalf("request %d with a fresh spoofed address: RateLimit-Remaining = %s, want %s", i+1, got, want)
			}
		}
		for _, xff := range []string{"::1", "127.0.0.2", "garbage", "198.51.100.7, 203.0.113.9", ""} {
			c := inst.Anon().With("X-Forwarded-For", xff)
			if got := remaining(t, c); got == "99" {
				t.Fatalf("X-Forwarded-For %q opened a fresh bucket", xff)
			}
		}
	})

	t.Run("spoofed X-Forwarded-Proto does not enable HSTS", func(t *testing.T) {
		r := inst.Anon().Get(t, "/api/health", "X-Forwarded-Proto", "https")
		r.ExpectStatus(t, 200)
		assertSecurityHeaders(t, r)
		if got := r.Header.Get("Strict-Transport-Security"); got != "" {
			t.Fatalf("HSTS from an untrusted peer = %q", got)
		}
	})

	t.Run("spoofed X-Forwarded-Host does not make an origin same-origin", func(t *testing.T) {
		c := inst.Anon()
		r := c.Get(t, "/api/health", "Origin", "https://evil.contract.test", "X-Forwarded-Host", "evil.contract.test", "X-Forwarded-Proto", "https")
		if got := r.Header.Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("forged forwarded host was trusted: Access-Control-Allow-Origin = %q", got)
		}
		// The real Host still counts.
		r = c.Get(t, "/api/health", "Origin", inst.BaseURL, "X-Forwarded-Host", "evil.contract.test")
		if got := r.Header.Get("Access-Control-Allow-Origin"); got != inst.BaseURL {
			t.Fatalf("same-origin request: Access-Control-Allow-Origin = %q", got)
		}
	})

	t.Run("a trusted proxy peer is believed (IPv6 loopback)", func(t *testing.T) {
		u, err := url.Parse(inst.BaseURL)
		if err != nil {
			t.Fatal(err)
		}
		v6 := &Instance{Name: inst.Name + "-v6", BaseURL: "http://[::1]:" + u.Port()}
		if _, err := v6.Anon().raw("GET", "/api/health", nil, nil); err != nil {
			t.Skipf("IPv6 loopback unavailable: %v", err)
		}
		for i := 0; i < 3; i++ {
			if got := remaining(t, v6.Anon()); got != "99" {
				t.Fatalf("proxied client %d: RateLimit-Remaining = %s, want its own bucket (99)", i+1, got)
			}
		}
		c := v6.Anon()
		remaining(t, c)
		if got := remaining(t, c); got != "98" {
			t.Fatalf("same proxied client twice: RateLimit-Remaining = %s, want 98", got)
		}
		r := v6.Anon().Get(t, "/api/health", "X-Forwarded-Proto", "https")
		if got := r.Header.Get("Strict-Transport-Security"); got != "max-age=31536000; includeSubDomains" {
			t.Fatalf("HSTS behind a trusted https proxy = %q", got)
		}
		r = v6.Anon().Get(t, "/api/health", "Origin", "https://panel.contract.test", "X-Forwarded-Host", "panel.contract.test", "X-Forwarded-Proto", "https")
		if got := r.Header.Get("Access-Control-Allow-Origin"); got != "https://panel.contract.test" {
			t.Fatalf("forwarded host from a trusted proxy: Access-Control-Allow-Origin = %q", got)
		}
		// A malformed forwarded address falls back to the proxy's own address.
		bad := v6.Anon().With("X-Forwarded-For", "not-an-ip")
		first, second := remaining(t, bad), remaining(t, v6.Anon().With("X-Forwarded-For", "also bad"))
		if first == "" || second == "" || first == second {
			t.Fatalf("malformed X-Forwarded-For must key on the proxy address: %s then %s", first, second)
		}
	})
}
