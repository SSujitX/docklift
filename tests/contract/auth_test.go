package contract

import (
	"fmt"
	"strconv"
	"testing"
	"time"
)

const bootstrapRequiredMsg = "Bootstrap secret required. Copy it from the backend logs (or data/.bootstrap-secret on the host)."

func TestAuthStatusAfterSetup(t *testing.T) {
	r := mainInst.Anon().Get(t, "/api/auth/status")
	r.Expect(t, 200, Obj{"setupComplete": true, "userCount": 1, "bootstrapRequired": false})
	// /status is skipped by the auth limiter (no counting, no headers).
	if r.Header.Get("RateLimit-Limit") != "" {
		t.Fatalf("/api/auth/status must not carry rate-limit headers")
	}
}

func TestRegisterAfterSetup(t *testing.T) {
	c := mainInst.Anon()
	c.Post(t, "/api/auth/register", map[string]any{}).ExpectError(t, 400, "Name, email, and password are required")
	c.Post(t, "/api/auth/register", map[string]any{"name": "x", "email": "x@y.z"}).ExpectError(t, 400, "Name, email, and password are required")
	c.Post(t, "/api/auth/register", map[string]any{"name": "x", "email": "x@y.z", "password": "short"}).ExpectError(t, 400, "Password must be at least 8 characters")
	c.Post(t, "/api/auth/register", map[string]any{"name": "x", "email": "x@y.z", "password": "longenough"}).
		ExpectError(t, 403, "Setup already complete. Use login instead.")
	c.Post(t, "/api/auth/register", map[string]any{"name": "x", "email": "x@y.z", "password": "longenough"}, "X-Bootstrap-Secret", "anything").
		ExpectError(t, 403, "Setup already complete. Use login instead.")
	c.Get(t, "/api/auth/setup-token").ExpectError(t, 403, "Setup already complete. Setup tokens are only available before first user registration.")
	c.Get(t, "/api/auth/setup-token", "X-Bootstrap-Secret", "anything").ExpectError(t, 403, "Setup already complete. Setup tokens are only available before first user registration.")
}

func sessionClaims(t *testing.T, token string) map[string]any {
	t.Helper()
	claims := jwtClaims(t, token)
	match(t, "session JWT payload", Obj{"userId": UUID, "email": Str, "role": "admin", "pwdv": Int, "iat": Int, "exp": Int}, claims)
	if h := jwtHeader(t, token); h["alg"] != "HS256" || h["typ"] != "JWT" {
		t.Fatalf("JWT header = %v, want HS256/JWT", h)
	}
	if ttl := claims["exp"].(float64) - claims["iat"].(float64); ttl != 7*24*3600 {
		t.Fatalf("session token ttl = %vs, want 7d", ttl)
	}
	return claims
}

func TestLogin(t *testing.T) {
	c := mainInst.Anon()
	userObj := Obj{"id": admin.UserID, "name": admin.Name, "email": admin.Email, "role": "admin"}
	c.Post(t, "/api/auth/login", map[string]any{}).ExpectError(t, 400, "Email and password are required")
	c.Post(t, "/api/auth/login", map[string]any{"email": admin.Email}).ExpectError(t, 400, "Email and password are required")
	c.Post(t, "/api/auth/login", map[string]any{"password": "x"}).ExpectError(t, 400, "Email and password are required")
	c.Post(t, "/api/auth/login", map[string]any{"email": "", "password": "x"}).ExpectError(t, 400, "Email and password are required")
	c.Post(t, "/api/auth/login", map[string]any{"email": "nobody@contract.test", "password": "whatever1"}).ExpectError(t, 401, "Invalid email or password")
	c.Post(t, "/api/auth/login", map[string]any{"email": admin.Email, "password": "wrong-password"}).ExpectError(t, 401, "Invalid email or password")

	// Deliberate fix 3: wrong-typed fields are a 400, never a 500.
	c.Post(t, "/api/auth/login", map[string]any{"email": 42, "password": "x"}).ExpectError(t, 400, "Email and password must be strings")
	c.Post(t, "/api/auth/login", map[string]any{"email": admin.Email, "password": map[string]any{"$ne": ""}}).ExpectError(t, 400, "Email and password must be strings")
	c.Post(t, "/api/auth/login", map[string]any{"email": []string{admin.Email}, "password": admin.Password}).ExpectError(t, 400, "Email and password must be strings")

	r := c.Post(t, "/api/auth/login", map[string]any{"email": "CONTRACT-ADMIN@Contract.Test", "password": admin.Password})
	got := r.Expect(t, 200, Obj{"message": "Login successful", "token": JWT, "user": userObj}).(map[string]any)
	claims := sessionClaims(t, got["token"].(string))
	match(t, "claims", Obj{"userId": admin.UserID, "email": admin.Email, "role": "admin", "pwdv": Int, "iat": Int, "exp": Int}, claims)
	if claims["pwdv"].(float64) <= 0 {
		t.Fatalf("pwdv = %v, want passwordChangedAt in ms", claims["pwdv"])
	}
	// The new token works.
	mainInst.As(got["token"].(string)).Get(t, "/api/auth/me").ExpectStatus(t, 200)
}

func TestMe(t *testing.T) {
	admin.Client().Get(t, "/api/auth/me").Expect(t, 200, Obj{"user": Obj{
		"id": admin.UserID, "name": admin.Name, "email": admin.Email, "role": "admin", "created_at": ISOms,
	}})
	mainInst.Anon().Get(t, "/api/auth/me").ExpectError(t, 401, "Authentication required")
}

func TestPurposeTokens(t *testing.T) {
	c := admin.Client()
	session := sessionClaims(t, admin.Token)
	for _, tc := range []struct{ path, purpose string }{{"/api/auth/sse-token", "sse"}, {"/api/auth/terminal-token", "terminal"}} {
		t.Run(tc.purpose, func(t *testing.T) {
			tok := c.Post(t, tc.path, nil).Expect(t, 200, Obj{"token": JWT}).(map[string]any)["token"].(string)
			claims := jwtClaims(t, tok)
			want := Obj{
				"userId": admin.UserID, "email": admin.Email, "role": "admin", "purpose": tc.purpose,
				"pwdv": session["pwdv"], "iat": Int, "exp": Int,
			}
			if tc.purpose == "terminal" {
				// Terminal tokens are single-use; a random jti keeps two
				// issued in the same second apart.
				want["jti"] = NonEmpty
			}
			match(t, tc.purpose+" claims", want, claims)
			if ttl := claims["exp"].(float64) - claims["iat"].(float64); ttl != 300 {
				t.Fatalf("%s token ttl = %v, want 300s", tc.purpose, ttl)
			}
			// Purpose tokens never authenticate normal API routes.
			p := mainInst.As(tok)
			for _, path := range []string{"/api/projects", "/api/auth/me", "/api/system/stats", "/api/backup"} {
				p.Get(t, path).ExpectError(t, 401, "Invalid token for this endpoint")
			}
			p.Post(t, "/api/auth/sse-token", nil).ExpectError(t, 401, "Invalid token for this endpoint")
			mainInst.Anon().Post(t, tc.path, nil).ExpectError(t, 401, "Authentication required")
		})
	}
}

func TestRateLimitHeaders(t *testing.T) {
	c := mainInst.Anon()
	r := c.Post(t, "/api/auth/login", map[string]any{"email": "rl@contract.test", "password": "x"})
	r.ExpectStatus(t, 401)
	want := map[string]string{"RateLimit-Policy": "100;w=900", "RateLimit-Limit": "100", "RateLimit-Remaining": "99"}
	for k, v := range want {
		if got := r.Header.Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	reset, err := strconv.Atoi(r.Header.Get("RateLimit-Reset"))
	if err != nil || reset < 890 || reset > 900 {
		t.Errorf("RateLimit-Reset = %q", r.Header.Get("RateLimit-Reset"))
	}
	// Every /api/auth route (except /status) shares the 100/15min budget per
	// client IP. Go exempts the cheap session checks (/me, /sse-token,
	// /terminal-token): they get their own 1000/15min budget, so a shared
	// address is not locked out of the dashboard.
	r = c.Get(t, "/api/auth/me")
	spent := 2

	spent = 1
	if got := r.Header.Get("RateLimit-Limit"); got != "1000" {
		t.Errorf("/me RateLimit-Limit = %q, want its own limiter", got)
	}

	c.Get(t, "/api/auth/status")
	r = c.Post(t, "/api/auth/login", map[string]any{})
	if got, want := r.Header.Get("RateLimit-Remaining"), fmt.Sprint(100-spent-1); got != want {
		t.Errorf("/status must not count: RateLimit-Remaining = %q, want %q", got, want)
	}
	// A different client IP has its own bucket.
	if got := mainInst.Anon().Post(t, "/api/auth/login", map[string]any{}).Header.Get("RateLimit-Remaining"); got != "99" {
		t.Errorf("fresh IP RateLimit-Remaining = %q", got)
	}
	// Non-auth routes are not rate limited.
	if got := admin.Client().Get(t, "/api/projects").Header.Get("RateLimit-Limit"); got != "" {
		t.Errorf("/api/projects RateLimit-Limit = %q", got)
	}
}

func TestSetupRateLimit(t *testing.T) {
	c := mainInst.Anon()
	// register and setup-token share a 10/15min budget per IP (plus the 100 auth budget).
	for i := 1; i <= 10; i++ {
		path := "/api/auth/register"
		if i%2 == 0 {
			path = "/api/auth/setup-token"
		}
		var r *Resp
		if path == "/api/auth/register" {
			r = c.Post(t, path, map[string]any{})
			r.ExpectError(t, 400, "Name, email, and password are required")
		} else {
			r = c.Get(t, path)
			r.ExpectStatus(t, 403)
		}
		// The auth limiter runs after the setup limiter and overwrites its headers.
		if got, want := r.Header.Get("RateLimit-Remaining"), fmt.Sprint(100-i); got != want {
			t.Errorf("request %d: RateLimit-Remaining = %q, want %q", i, got, want)
		}
		if got := r.Header.Get("RateLimit-Limit"); got != "100" {
			t.Errorf("request %d: RateLimit-Limit = %q", i, got)
		}
	}
	r := c.Post(t, "/api/auth/register", map[string]any{})
	r.ExpectError(t, 429, "Too many setup attempts, please try again later")
	for k, v := range map[string]string{"RateLimit-Policy": "10;w=900", "RateLimit-Limit": "10", "RateLimit-Remaining": "0"} {
		if got := r.Header.Get(k); got != v {
			t.Errorf("429 %s = %q, want %q", k, got, v)
		}
	}
	if ra, err := strconv.Atoi(r.Header.Get("Retry-After")); err != nil || ra < 1 || ra > 900 {
		t.Errorf("429 Retry-After = %q", r.Header.Get("Retry-After"))
	}
	c.Get(t, "/api/auth/setup-token").ExpectError(t, 429, "Too many setup attempts, please try again later")
	// Login is still allowed for this IP (separate, larger budget).
	c.Post(t, "/api/auth/login", map[string]any{}).ExpectError(t, 400, "Email and password are required")
	// Other IPs are unaffected.
	mainInst.Anon().Post(t, "/api/auth/register", map[string]any{}).ExpectStatus(t, 400)
}

func TestAuthRateLimitExhaustion(t *testing.T) {
	c := mainInst.Anon()
	for i := 0; i < 100; i++ {
		c.Post(t, "/api/auth/login", map[string]any{}).ExpectStatus(t, 400)
	}
	r := c.Post(t, "/api/auth/login", map[string]any{"email": admin.Email, "password": admin.Password})
	r.ExpectError(t, 429, "Too many attempts, please try again later")
	if got := r.Header.Get("RateLimit-Remaining"); got != "0" {
		t.Errorf("RateLimit-Remaining = %q", got)
	}
	// Authenticated /api/auth routes from that IP are limited too; /status is
	// not, and on Go neither are the cheap session checks.
	me := c.With("Authorization", "Bearer "+admin.Token).Get(t, "/api/auth/me")

	me.ExpectStatus(t, 200)
	c.With("Authorization", "Bearer "+admin.Token).Post(t, "/api/auth/sse-token", nil).ExpectStatus(t, 200)
	c.With("Authorization", "Bearer "+admin.Token).Post(t, "/api/auth/change-password", map[string]any{}).
		ExpectError(t, 429, "Too many attempts, please try again later")

	c.Get(t, "/api/auth/status").ExpectStatus(t, 200)
}

func TestInternalSecretHeader(t *testing.T) {
	_, internal := readSecrets(t, mainInst)
	c := mainInst.Anon().With("X-Internal-Secret", internal)

	// Deliberate fix: the X-Internal-Secret full-admin bypass is removed
	// (the Go backend starts webhook deploys in process); the header is
	// ignored and the request needs a session like any other.
	for _, path := range []string{"/api/projects", "/api/ports", "/api/auth/me", "/api/backup/critical-status"} {
		c.Get(t, path).ExpectError(t, 401, "Authentication required")
	}
	c.Post(t, "/api/system/purge", map[string]any{"password": "anything"}).ExpectError(t, 401, "Authentication required")
	c.Get(t, "/api/system/logs/backend").ExpectError(t, 401, "SSE token required")
	return
}

func TestSelfSignedTokens(t *testing.T) {
	secret, _ := readSecrets(t, mainInst)
	pwdv := sessionClaims(t, admin.Token)["pwdv"]
	now := time.Now().Unix()
	base := func(over map[string]any) string {
		claims := map[string]any{"userId": admin.UserID, "email": admin.Email, "role": "admin", "pwdv": pwdv, "iat": now, "exp": now + 3600}
		for k, v := range over {
			if v == nil {
				delete(claims, k)
			} else {
				claims[k] = v
			}
		}
		return signJWT(secret, claims)
	}
	c := func(tok string) *Client { return mainInst.As(tok) }
	// A correctly signed HS256 token is accepted (tokens from either backend must interoperate).
	c(base(nil)).Get(t, "/api/auth/me").ExpectStatus(t, 200)
	c(base(map[string]any{"iat": nil, "exp": nil})).Get(t, "/api/projects").ExpectStatus(t, 200)
	c(base(map[string]any{"iat": now - 7200, "exp": now - 3600})).Get(t, "/api/projects").ExpectError(t, 401, "Token expired")
	c(base(map[string]any{"pwdv": 1})).Get(t, "/api/projects").ExpectError(t, 401, "Session expired. Please log in again.")
	c(base(map[string]any{"pwdv": nil})).Get(t, "/api/projects").ExpectError(t, 401, "Session expired. Please log in again.")
	c(base(map[string]any{"pwdv": fmt.Sprint(pwdv)})).Get(t, "/api/projects").ExpectError(t, 401, "Session expired. Please log in again.")
	c(base(map[string]any{"userId": "5eed0000-0000-4000-8000-00000000dead"})).Get(t, "/api/projects").ExpectError(t, 401, "User not found")
	c(base(map[string]any{"purpose": "sse"})).Get(t, "/api/projects").ExpectError(t, 401, "Invalid token for this endpoint")
	c(base(map[string]any{"purpose": "other"})).Get(t, "/api/projects").ExpectStatus(t, 200)
	// SSE routes accept only purpose=sse.
	c(base(nil)).Get(t, "/api/system/logs/nope?token="+base(map[string]any{"purpose": "sse"})).
		ExpectError(t, 400, invalidLogService())
	mainInst.Anon().Get(t, "/api/system/logs/nope?token="+base(map[string]any{"purpose": "sse", "pwdv": 2})).
		ExpectError(t, 401, "Session expired. Please log in again.")
	mainInst.Anon().Get(t, "/api/system/logs/nope?token="+base(map[string]any{"purpose": "sse", "iat": now - 7200, "exp": now - 3600})).
		ExpectError(t, 401, "Token expired")
	mainInst.Anon().Get(t, "/api/system/logs/nope?token="+base(map[string]any{"purpose": "terminal"})).
		ExpectError(t, 401, "SSE token required")

	// The internal identity went with the X-Internal-Secret bypass: a
	// token for it is checked like any other and has no user row.
	c(base(map[string]any{"userId": "internal", "pwdv": nil})).Get(t, "/api/projects").ExpectError(t, 401, "User not found")

}
