package httpx

import (
	"context"
	"strconv"
	"sync"
	"time"
)

// BodyBudget bounds how many request body bytes the parsers sharing it hold
// in memory at once. A parser reserves the body's declared size (its limit
// when the size is unknown or the body is compressed) before reading and
// returns it when the handler has finished, so a burst of large bodies
// cannot grow the process without bound: parsers that cannot reserve within
// a short wait answer 503 with Retry-After.
type BodyBudget struct {
	capacity int64

	mu      sync.Mutex
	used    int64
	release chan struct{} // closed (and replaced) whenever bytes are returned
}

// NewBodyBudget returns a budget of capacity bytes.
func NewBodyBudget(capacity int64) *BodyBudget {
	return &BodyBudget{capacity: capacity, release: make(chan struct{})}
}

// Budgets of the API. Unauthenticated parsing (login/register, the GitHub
// webhook) has its own budget so anonymous clients can never starve the
// dashboard's requests.
var (
	// SessionBodyBudget is shared by the parsers behind an auth guard: room
	// for three full MaxBodyBytes bodies at once.
	SessionBodyBudget = NewBodyBudget(3 * MaxBodyBytes)
	// PublicBodyBudget is shared by the parsers of unauthenticated routes.
	PublicBodyBudget = NewBodyBudget(2 * MaxBodyBytes)
)

// bodyBudgetWait is how long a parser waits for budget before answering 503.
var bodyBudgetWait = 2 * time.Second

// MsgServerBusy is the 503 error of a saturated body budget.
const MsgServerBusy = "Server is busy, please retry shortly"

// Acquire reserves n bytes (at most the capacity, so one body of any
// accepted size fits an idle budget), waiting up to wait or until ctx ends.
// It reports whether the bytes were reserved; the caller must Release them.
func (b *BodyBudget) Acquire(ctx context.Context, n int64, wait time.Duration) bool {
	n = b.clamp(n)
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		b.mu.Lock()
		if b.used+n <= b.capacity {
			b.used += n
			b.mu.Unlock()
			return true
		}
		ch := b.release
		b.mu.Unlock()
		select {
		case <-ch:
		case <-timer.C:
			return false
		case <-ctx.Done():
			return false
		}
	}
}

// Release returns n bytes reserved by Acquire.
func (b *BodyBudget) Release(n int64) {
	n = b.clamp(n)
	b.mu.Lock()
	b.used -= n
	close(b.release)
	b.release = make(chan struct{})
	b.mu.Unlock()
}

// InUse reports the bytes currently reserved.
func (b *BodyBudget) InUse() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.used
}

func (b *BodyBudget) clamp(n int64) int64 {
	if n > b.capacity {
		return b.capacity
	}
	if n < 0 {
		return 0
	}
	return n
}

// retryAfter is the Retry-After value (whole seconds, at least 1) of a
// saturated budget.
func retryAfter(wait time.Duration) string {
	secs := int(wait / time.Second)
	if secs < 1 {
		secs = 1
	}
	return strconv.Itoa(secs)
}
