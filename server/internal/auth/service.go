package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"docklift/internal/httpx"
	"docklift/internal/store"
)

// Password-version failures returned by CheckPasswordVersion.
const (
	msgUserNotFound    = "User not found"
	msgSessionExpired  = "Session expired. Please log in again."
	msgAuthUnavailable = "Authentication temporarily unavailable"
)

// Service issues and checks tokens and owns the setup secrets of one data
// directory.
type Service struct {
	store     *store.Store
	secrets   Secrets
	bootstrap *Bootstrap
	log       *slog.Logger
	now       func() time.Time
	// stepUpFailures counts wrong step-up passwords per user and client.
	stepUpFailures *httpx.FailureLimiter
	// sessions ends long-lived connections when their user's password changes.
	sessions sessionRegistry
	// usedTokens remembers consumed one-time purpose tokens until they expire.
	usedTokens usedTokens
}

// Step-up password guessing limit: StepUpFailureLimit wrong passwords per
// user and client address within StepUpFailureWindow, then 429 until the
// window (started by the first failure) ends. A correct password resets it.
const (
	StepUpFailureLimit  = 10
	StepUpFailureWindow = 15 * time.Minute
	// stepUpMaxKeys bounds the user/client pairs tracked at once.
	stepUpMaxKeys = 10_000
)

// MsgTooManyPasswordAttempts is the error of a throttled step-up check.
const MsgTooManyPasswordAttempts = "Too many failed password attempts. Try again later."

// New returns the auth service.
func New(st *store.Store, secrets Secrets, dataPath string, log *slog.Logger) *Service {
	s := &Service{
		store:     st,
		secrets:   secrets,
		bootstrap: NewBootstrap(dataPath, log),
		log:       log,
		now:       time.Now,
		sessions:  sessionRegistry{recheck: SessionRecheckInterval},
	}
	s.stepUpFailures = httpx.NewFailureLimiter(httpx.FailureLimitConfig{
		Window:  StepUpFailureWindow,
		Limit:   StepUpFailureLimit,
		MaxKeys: stepUpMaxKeys,
		Now:     func() time.Time { return s.now() },
	})
	return s
}

// Secrets returns the signing secret and the stored internal API secret
// (kept in .secrets for compatibility, no longer accepted as credentials).
func (s *Service) Secrets() Secrets { return s.secrets }

// Bootstrap returns the bootstrap-secret and setup-token files of this data directory.
func (s *Service) Bootstrap() *Bootstrap { return s.bootstrap }

// IssueSession signs a 7-day session token for u.
func (s *Service) IssueSession(u *store.User) (string, error) {
	return sign(s.secrets.JWTSecret, TokenSubject{UserID: u.ID, Email: u.Email, Role: u.Role, PasswordVersion: u.PasswordVersion()},
		"", SessionTTL, s.now())
}

// IssuePurpose signs a 5-minute token for one purpose (PurposeSSE or PurposeTerminal).
func (s *Service) IssuePurpose(sub TokenSubject, purpose string) (string, error) {
	return sign(s.secrets.JWTSecret, sub, purpose, PurposeTTL, s.now())
}

// Verify checks a token's signature and time claims.
func (s *Service) Verify(token string) (*Claims, error) {
	return verify(s.secrets.JWTSecret, token, s.now())
}

// CheckPasswordVersion is assertPasswordStillValid: it rejects tokens issued
// before the user's last password change. It returns "" when the token is
// still valid, else the 401 message. Database errors fail closed.
func (s *Service) CheckPasswordVersion(ctx context.Context, c *Claims) string {
	switch c.userIDState {
	case userIDFalsy:
		return ""
	case userIDInvalid:
		return msgAuthUnavailable
	}
	u, err := s.store.GetUser(ctx, c.UserID)
	if errors.Is(err, store.ErrNotFound) {
		return msgUserNotFound
	}
	if err != nil {
		s.log.Error("password version check failed", "error", err)
		return msgAuthUnavailable
	}
	if !c.HasPwdv || c.Pwdv != float64(u.PasswordVersion()) {
		return msgSessionExpired
	}
	return ""
}

// VerifyPurposeToken verifies and consumes a one-time purpose token (the
// terminal WebSocket upgrade): valid signature and expiry, matching purpose,
// current password version, and not used before. A token is accepted once;
// it is remembered until it expires. SSE tokens are not checked here: an
// EventSource reconnects with the same URL, so RequireSSE accepts them again.
func (s *Service) VerifyPurposeToken(ctx context.Context, token, purpose string) (*Claims, bool) {
	c, err := s.Verify(token)
	if err != nil || c.Purpose != purpose || s.CheckPasswordVersion(ctx, c) != "" {
		return nil, false
	}
	if !s.usedTokens.consume(token, c.exp, s.now()) {
		s.log.Warn("[SECURITY] Rejected reused or untrackable purpose token", "purpose", purpose, "userId", c.UserID)
		return nil, false
	}
	return c, true
}

// User is the authenticated caller attached to the request context.
type User struct {
	UserID  string
	Email   string
	Role    string
	Purpose string
}

type userKey struct{}

// WithUser attaches u to ctx.
func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, userKey{}, u)
}

// UserFrom returns the authenticated caller.
func UserFrom(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userKey{}).(User)
	return u, ok
}

// secureEqual compares two strings without leaking their content or length
// through timing.
func secureEqual(a, b string) bool {
	ha, hb := sha256.Sum256([]byte(a)), sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}

func (s *Service) writeVerifyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidToken):
		httpx.Error(w, http.StatusUnauthorized, "Invalid token")
	case errors.Is(err, ErrTokenExpired):
		httpx.Error(w, http.StatusUnauthorized, "Token expired")
	default:
		httpx.Error(w, http.StatusInternalServerError, "Authentication failed")
	}
}

// RequireSession is authMiddleware: an `Authorization: Bearer <session JWT>`.
// Query tokens are never accepted and purpose tokens (sse/terminal) are
// refused. The X-Internal-Secret header of the TypeScript backend (a
// full-admin bypass used only by its webhook-to-deploy HTTP call, which the
// Go backend makes in process) is no longer accepted.
func (s *Service) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			httpx.Error(w, http.StatusUnauthorized, "Authentication required")
			return
		}
		token := strings.Split(header, " ")[1]
		if token == "" {
			httpx.Error(w, http.StatusUnauthorized, "Authentication required")
			return
		}
		c, err := s.Verify(token)
		if err != nil {
			s.writeVerifyError(w, err)
			return
		}
		if c.Purpose == PurposeSSE || c.Purpose == PurposeTerminal {
			httpx.Error(w, http.StatusUnauthorized, "Invalid token for this endpoint")
			return
		}
		if msg := s.CheckPasswordVersion(r.Context(), c); msg != "" {
			httpx.Error(w, http.StatusUnauthorized, msg)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), User{UserID: c.UserID, Email: c.Email, Role: c.Role})))
	})
}

// RequireSSE is sseAuthMiddleware: a `?token=` query JWT with purpose "sse"
// (EventSource cannot send headers). Mount it only on log stream routes. The
// request context is bound to the session (BindSession), so an open stream
// ends when the user's password changes.
func (s *Service) RequireSSE(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokens := r.URL.Query()["token"]
		if len(tokens) == 0 || (len(tokens) == 1 && tokens[0] == "") {
			httpx.Error(w, http.StatusUnauthorized, "SSE token required")
			return
		}
		if len(tokens) > 1 {
			// Express parses a repeated parameter as an array, which jsonwebtoken rejects.
			httpx.Error(w, http.StatusUnauthorized, "Invalid token")
			return
		}
		c, err := s.Verify(tokens[0])
		if err != nil {
			s.writeVerifyError(w, err)
			return
		}
		if c.Purpose != PurposeSSE {
			httpx.Error(w, http.StatusUnauthorized, "SSE token required")
			return
		}
		if msg := s.CheckPasswordVersion(r.Context(), c); msg != "" {
			httpx.Error(w, http.StatusUnauthorized, msg)
			return
		}
		ctx, stop := s.BindSession(r.Context(), c)
		defer stop()
		next.ServeHTTP(w, r.WithContext(WithUser(ctx, User{UserID: c.UserID, Email: c.Email, Role: c.Role, Purpose: PurposeSSE})))
	})
}

// RequireStepUpPassword is requireStepUpPassword: dangerous operations must
// re-send the account password in the JSON body. It writes the 401/403
// response itself and returns false when the caller must stop. A non-nil
// error means a database failure; nothing was written and the caller
// answers as its own error path did in TypeScript.
func (s *Service) RequireStepUpPassword(w http.ResponseWriter, r *http.Request) (bool, error) {
	password, ok := httpx.BodyFrom(r).Get("password").String()
	if !ok || password == "" {
		httpx.JSON(w, http.StatusForbidden, stepUpBody{Error: "Password confirmation required for this operation", RequirePassword: true})
		return false, nil
	}
	u, authed := UserFrom(r.Context())
	if !authed || u.UserID == "" {
		httpx.Error(w, http.StatusUnauthorized, "Unauthorized")
		return false, nil
	}
	dbUser, err := s.store.GetUser(r.Context(), u.UserID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return false, err
	}
	if dbUser == nil || dbUser.Password == "" {
		httpx.JSON(w, http.StatusForbidden, stepUpBody{Error: "Unable to verify password", RequirePassword: true})
		return false, nil
	}
	return s.VerifyStepUpPassword(w, r, u.UserID, dbUser.Password, password), nil
}

// VerifyStepUpPassword compares a re-entered account password with the
// user's bcrypt hash, limiting guesses (a stolen session token must not
// allow unlimited attempts at the password). It writes the refusal itself
// and returns false: 429 with Retry-After once the user has failed
// StepUpFailureLimit times from this client within StepUpFailureWindow
// (the hash is not even compared), else 403 "Invalid password" on a
// mismatch. A match resets the count. This throttle is not in the
// TypeScript backend (owner-approved security fix).
func (s *Service) VerifyStepUpPassword(w http.ResponseWriter, r *http.Request, userID, hash, password string) bool {
	key := userID + " " + httpx.RateLimitKey(httpx.ClientIP(r))
	if wait := s.stepUpFailures.RetryAfter(key); wait > 0 {
		s.log.Warn("[SECURITY] Step-up password attempts throttled", "userId", userID, "ip", httpx.ClientIP(r))
		w.Header().Set("Retry-After", httpx.RetryAfterSeconds(wait))
		httpx.JSON(w, http.StatusTooManyRequests, stepUpBody{Error: MsgTooManyPasswordAttempts, RequirePassword: true})
		return false
	}
	if !ComparePassword(hash, password) {
		s.stepUpFailures.Fail(key)
		httpx.JSON(w, http.StatusForbidden, stepUpBody{Error: "Invalid password", RequirePassword: true})
		return false
	}
	s.stepUpFailures.Reset(key)
	return true
}

type stepUpBody struct {
	Error           string `json:"error"`
	RequirePassword bool   `json:"requirePassword"`
}
