package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"docklift/internal/db"
	"docklift/internal/httpx"
	"docklift/internal/store"
)

// API tokens are long-lived bearer credentials for scripts and AI agents
// (`Authorization: Bearer dl_...`). They are created by an admin from the
// dashboard or the CLI; only their sha256 hash is stored.
const (
	// APITokenPrefix starts every API token, so RequireSession can tell one
	// from a session JWT (which always starts with "eyJ").
	APITokenPrefix = "dl_"
	// apiTokenRandomBytes is the entropy of a token (256 bits).
	apiTokenRandomBytes = 32
	// apiTokenBodyLen is the base64url (unpadded) length of the random part.
	apiTokenBodyLen = 43
	// APITokenDisplayLen is how many leading characters are kept as the
	// token's display prefix ("dl_" plus 8).
	APITokenDisplayLen = 11

	// ScopeRead allows GET and HEAD requests only.
	ScopeRead = "read"
	// ScopeFull allows every method.
	ScopeFull = "full"

	// APITokenTouchInterval is how stale last_used_at must be before a
	// request writes it again (one write per token per minute at most).
	APITokenTouchInterval = time.Minute

	// APITokenFailureLimit invalid tokens per client address within
	// APITokenFailureWindow, then 429 until the window ends. A valid token
	// does not reset the count (one good token must not unlock guessing).
	APITokenFailureLimit  = 20
	APITokenFailureWindow = 15 * time.Minute
)

// Fixed messages of the token paths.
const (
	MsgInvalidAPIToken      = "Invalid API token"
	MsgAPITokenExpired      = "API token expired"
	MsgAPITokenRevoked      = "API token revoked: the account password changed after it was created"
	MsgAPITokenReadOnly     = "This API token is read-only (scope \"read\" allows GET requests only)"
	MsgTooManyAPITokenTries = "Too many invalid API tokens. Try again later."
	// MsgSessionOnly is the 403 of endpoints an API token can never reach.
	MsgSessionOnly = "This endpoint is not available with an API token. Sign in to the dashboard instead."
	// MsgFullScopeOnly is the 403 of GET endpoints a read token cannot use.
	MsgFullScopeOnly = "This endpoint needs an API token with scope \"full\""
	// MsgDangerousNotAllowed is the 403 of a step-up action called with a
	// token created without allow_dangerous.
	MsgDangerousNotAllowed = "This action asks for the account password in the dashboard. An API token can perform it only when it was created with \"allow dangerous actions\"."
)

// APITokenInfo describes the API token that authenticated a request.
type APITokenInfo struct {
	ID             string
	Name           string
	Prefix         string
	Scope          string
	AllowDangerous bool
}

// NewAPIToken returns a fresh token, its sha256 hash (hex) and its display
// prefix. The plain token is shown once and never stored.
func NewAPIToken() (plain, hash, prefix string, err error) {
	b := make([]byte, apiTokenRandomBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", "", fmt.Errorf("generate api token: %w", err)
	}
	plain = APITokenPrefix + base64.RawURLEncoding.EncodeToString(b)
	return plain, HashAPIToken(plain), plain[:APITokenDisplayLen], nil
}

// HashAPIToken is the stored form of a token: hex sha256 of its bytes.
func HashAPIToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// ValidAPITokenFormat reports whether s has the shape NewAPIToken produces.
func ValidAPITokenFormat(s string) bool {
	body, ok := strings.CutPrefix(s, APITokenPrefix)
	if !ok || len(body) != apiTokenBodyLen {
		return false
	}
	for i := 0; i < len(body); i++ {
		c := body[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

// IsAPITokenRequest reports whether r carries `Authorization: Bearer dl_...`.
func IsAPITokenRequest(r *http.Request) bool {
	token, ok := bearerToken(r)
	return ok && strings.HasPrefix(token, APITokenPrefix)
}

// bearerToken is the credential of `Authorization: Bearer <token>`, as
// RequireSession has always read it.
func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return "", false
	}
	token := strings.Split(header, " ")[1]
	return token, token != ""
}

// ValidScope reports whether scope is ScopeRead or ScopeFull.
func ValidScope(scope string) bool { return scope == ScopeRead || scope == ScopeFull }

// readOnlyMethod reports a method a read-scope token may use.
func readOnlyMethod(r *http.Request) bool {
	m := httpx.OriginalMethod(r)
	return m == http.MethodGet || m == http.MethodHead
}

// APITokenUsable reports why a stored token no longer authenticates ("" when
// it does): expired, or the owner's password changed after it was created.
func APITokenUsable(t *store.APIToken, owner *store.User, now time.Time) string {
	if t.ExpiresAt != nil && !now.Before(t.ExpiresAt.Time()) {
		return MsgAPITokenExpired
	}
	if owner == nil {
		return MsgInvalidAPIToken
	}
	if owner.PasswordChangedAt != nil && owner.PasswordChangedAt.Time().After(t.CreatedAt.Time()) {
		return MsgAPITokenRevoked
	}
	return ""
}

// serveAPIToken authenticates a `Bearer dl_...` request: failure throttle
// per client address, hash lookup, expiry and password-change revocation,
// then the scope. On success the request continues with a User carrying
// the token.
func (s *Service) serveAPIToken(w http.ResponseWriter, r *http.Request, token string, next http.Handler) {
	u, f := s.authenticateAPIToken(r, token, true)
	if f != nil {
		f.write(w)
		return
	}
	if !readOnlyMethod(r) {
		s.log.Info(fmt.Sprintf("[API-TOKEN] %s %s by token %q (%s) from IP: %s",
			httpx.OriginalMethod(r), r.URL.Path, u.Token.Name, u.Token.Prefix, httpx.ClientIP(r)))
	}
	next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), u)))
}

// RequireAPIToken accepts only `Authorization: Bearer dl_...` (the MCP
// endpoint). The token's scope is not checked against this request's
// method: the endpoint is a transport, and every operation it performs is
// a request of its own through the router, where the scope applies. A
// failure is 401 with `WWW-Authenticate: Bearer` (429 when throttled).
func (s *Service) RequireAPIToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok || !strings.HasPrefix(token, APITokenPrefix) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="docklift", error="invalid_token"`)
			httpx.Error(w, http.StatusUnauthorized, "API token required (Authorization: Bearer dl_...)")
			return
		}
		u, f := s.authenticateAPIToken(r, token, false)
		if f != nil {
			if f.status == http.StatusUnauthorized {
				w.Header().Set("WWW-Authenticate", `Bearer realm="docklift", error="invalid_token"`)
			}
			f.write(w)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), u)))
	})
}

// tokenFailure is a refused API token request.
type tokenFailure struct {
	status int
	msg    string
	wait   time.Duration
}

func (f *tokenFailure) write(w http.ResponseWriter) {
	if f.wait > 0 {
		w.Header().Set("Retry-After", httpx.RetryAfterSeconds(f.wait))
	}
	httpx.Error(w, f.status, f.msg)
}

// authenticateAPIToken checks token: failure throttle per client address,
// format, hash lookup, expiry and password-change revocation, then (with
// checkScope) the scope against r's method. It refreshes last_used_at at
// most once per APITokenTouchInterval.
func (s *Service) authenticateAPIToken(r *http.Request, token string, checkScope bool) (User, *tokenFailure) {
	ip := httpx.ClientIP(r)
	key := httpx.RateLimitKey(ip)
	if wait := s.tokenFailures.RetryAfter(key); wait > 0 {
		s.log.Warn("[SECURITY] API token attempts throttled", "ip", ip)
		return User{}, &tokenFailure{status: http.StatusTooManyRequests, msg: MsgTooManyAPITokenTries, wait: wait}
	}
	invalid := &tokenFailure{status: http.StatusUnauthorized, msg: MsgInvalidAPIToken}
	ctx := r.Context()
	if !ValidAPITokenFormat(token) {
		s.tokenFailures.Fail(key)
		return User{}, invalid
	}
	t, err := s.store.GetAPITokenByHash(ctx, HashAPIToken(token))
	if errors.Is(err, store.ErrNotFound) {
		s.tokenFailures.Fail(key)
		s.log.Warn("[SECURITY] Rejected unknown API token", "ip", ip)
		return User{}, invalid
	}
	if err != nil {
		s.log.Error("api token lookup failed", "error", err)
		return User{}, &tokenFailure{status: http.StatusUnauthorized, msg: msgAuthUnavailable}
	}
	owner, err := s.store.GetUser(ctx, t.UserID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.log.Error("api token owner lookup failed", "error", err)
		return User{}, &tokenFailure{status: http.StatusUnauthorized, msg: msgAuthUnavailable}
	}
	now := s.now()
	if msg := APITokenUsable(t, owner, now); msg != "" {
		return User{}, &tokenFailure{status: http.StatusUnauthorized, msg: msg}
	}
	if checkScope && t.Scope != ScopeFull && !readOnlyMethod(r) {
		return User{}, &tokenFailure{status: http.StatusForbidden, msg: MsgAPITokenReadOnly}
	}
	if t.LastUsedAt == nil || now.Sub(t.LastUsedAt.Time()) >= APITokenTouchInterval {
		if err := s.store.TouchAPIToken(context.WithoutCancel(ctx), t.ID, db.NewTime(now)); err != nil {
			s.log.Warn("api token last_used_at update failed", "error", err)
		}
	}
	info := &APITokenInfo{ID: t.ID, Name: t.Name, Prefix: t.Prefix, Scope: t.Scope, AllowDangerous: t.AllowDangerous}
	return User{UserID: owner.ID, Email: owner.Email, Role: owner.Role, Token: info}, nil
}

// SessionOnly refuses requests authenticated with an API token (403). Mount
// it, after the auth guard, on routes no token may reach whatever its flags:
// token management, password and profile changes, the terminal and SSE
// token endpoints and the domain-only access switch.
func SessionOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, _ := UserFrom(r.Context()); u.Token != nil {
			httpx.Error(w, http.StatusForbidden, MsgSessionOnly)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// FullScopeOnly refuses read-scope API tokens (403) on a GET route that a
// read token must not use (backup downloads).
func FullScopeOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, _ := UserFrom(r.Context()); u.Token != nil && u.Token.Scope != ScopeFull {
			httpx.Error(w, http.StatusForbidden, MsgFullScopeOnly)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// TokenStepUp decides a step-up check for a request authenticated with an
// API token, which has no password to re-enter. handled is false for
// session requests (the caller verifies the password as usual). For tokens
// handled is true and ok reports whether the action may proceed: only a
// token created with allow_dangerous may; otherwise the 403 is written.
func (s *Service) TokenStepUp(w http.ResponseWriter, r *http.Request) (handled, ok bool) {
	u, _ := UserFrom(r.Context())
	if u.Token == nil {
		return false, false
	}
	if !u.Token.AllowDangerous {
		httpx.JSON(w, http.StatusForbidden, stepUpBody{Error: MsgDangerousNotAllowed})
		return true, false
	}
	s.log.Info(fmt.Sprintf("[AUDIT] Dangerous action %s %s by API token %q (%s) from IP: %s",
		httpx.OriginalMethod(r), r.URL.Path, u.Token.Name, u.Token.Prefix, httpx.ClientIP(r)))
	return true, true
}
