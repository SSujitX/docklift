package auth

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"time"
)

// ErrSessionRevoked is the cancellation cause of a context bound with
// BindSession when the user's password changed (or the account was deleted)
// after the token was issued.
var ErrSessionRevoked = errors.New("session revoked")

// SessionRecheckInterval is how often a bound connection re-checks its
// token's password version against the database. RevokeUser ends bound
// connections at once; the recheck catches changes this process did not make
// (the reset-password command runs in a separate process).
const SessionRecheckInterval = 30 * time.Second

// sessionRegistry tracks the contexts of long-lived connections (SSE log
// streams, terminal sessions) per user.
type sessionRegistry struct {
	recheck time.Duration

	mu   sync.Mutex
	subs map[string]map[*boundSession]struct{}
}

type boundSession struct {
	cancel context.CancelCauseFunc
}

// BindSession returns a context derived from parent for a long-lived
// connection authenticated by c. The context is cancelled with cause
// ErrSessionRevoked when RevokeUser(c.UserID) runs or a periodic recheck
// (SessionRecheckInterval) finds that the token no longer matches the
// account's password version. A failed database lookup does not revoke (the
// connection was valid when it was opened; the next recheck decides). stop
// releases the binding and cancels the context; call it when the connection
// ends.
func (s *Service) BindSession(parent context.Context, c *Claims) (ctx context.Context, stop context.CancelFunc) {
	ctx, cancel := context.WithCancelCause(parent)
	b := &boundSession{cancel: cancel}
	reg := &s.sessions
	userID := c.UserID
	if userID != "" {
		reg.mu.Lock()
		if reg.subs == nil {
			reg.subs = map[string]map[*boundSession]struct{}{}
		}
		if reg.subs[userID] == nil {
			reg.subs[userID] = map[*boundSession]struct{}{}
		}
		reg.subs[userID][b] = struct{}{}
		reg.mu.Unlock()
	}
	done := make(chan struct{})
	var once sync.Once
	stop = func() {
		once.Do(func() {
			if userID != "" {
				reg.mu.Lock()
				delete(reg.subs[userID], b)
				if len(reg.subs[userID]) == 0 {
					delete(reg.subs, userID)
				}
				reg.mu.Unlock()
			}
			cancel(context.Canceled)
			close(done)
		})
	}
	if reg.recheck > 0 {
		go func() {
			t := time.NewTicker(reg.recheck)
			defer t.Stop()
			for {
				select {
				case <-done:
					return
				case <-ctx.Done():
					return
				case <-t.C:
					switch s.CheckPasswordVersion(ctx, c) {
					case msgSessionExpired, msgUserNotFound:
						s.log.Info("[SECURITY] Closing connection: password changed since its token was issued", "userId", userID)
						cancel(ErrSessionRevoked)
						return
					}
				}
			}
		}()
	}
	return ctx, stop
}

// RevokeUser ends every connection bound to userID with BindSession (open
// SSE log streams and terminal sessions). Call it after the user's password
// changed.
func (s *Service) RevokeUser(userID string) {
	reg := &s.sessions
	reg.mu.Lock()
	subs := reg.subs[userID]
	delete(reg.subs, userID)
	reg.mu.Unlock()
	if len(subs) > 0 {
		s.log.Info("[SECURITY] Closing open streams and terminal sessions after password change", "userId", userID, "connections", len(subs))
	}
	for b := range subs {
		b.cancel(ErrSessionRevoked)
	}
}

// maxUsedTokens bounds the one-time tokens remembered at once. Purpose
// tokens live five minutes and need an authenticated request each, so the
// bound is never reached in normal use; when it is, new tokens are refused
// (fail closed) until old ones expire.
const maxUsedTokens = 10_000

// usedTokens remembers consumed one-time tokens (by SHA-256) until expiry.
type usedTokens struct {
	mu   sync.Mutex
	seen map[[sha256.Size]byte]time.Time
}

// consume records token and reports whether it had not been used before. A
// token without an expiry is kept for PurposeTTL.
func (u *usedTokens) consume(token string, exp, now time.Time) bool {
	if exp.IsZero() {
		exp = now.Add(PurposeTTL)
	}
	key := sha256.Sum256([]byte(token))
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.seen == nil {
		u.seen = map[[sha256.Size]byte]time.Time{}
	}
	if until, ok := u.seen[key]; ok && now.Before(until) {
		return false
	}
	if len(u.seen) >= maxUsedTokens {
		for k, until := range u.seen {
			if !now.Before(until) {
				delete(u.seen, k)
			}
		}
		if len(u.seen) >= maxUsedTokens {
			return false
		}
	}
	u.seen[key] = exp
	return true
}
