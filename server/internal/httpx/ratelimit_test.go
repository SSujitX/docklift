package httpx

import (
	"fmt"
	"testing"
	"time"
)

func TestFailureLimiter(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	f := NewFailureLimiter(FailureLimitConfig{Window: 15 * time.Minute, Limit: 3, Now: func() time.Time { return now }})

	for i := 0; i < 3; i++ {
		if d := f.RetryAfter("u1"); d != 0 {
			t.Fatalf("attempt %d blocked early (%v)", i+1, d)
		}
		f.Fail("u1")
		now = now.Add(time.Minute)
	}
	if d := f.RetryAfter("u1"); d != 12*time.Minute {
		t.Fatalf("after the limit RetryAfter = %v, want the rest of the window", d)
	}
	if f.RetryAfter("u2") != 0 {
		t.Fatal("keys are independent")
	}

	// The window starts at the first failure and then forgets everything.
	now = now.Add(12 * time.Minute)
	if d := f.RetryAfter("u1"); d != 0 {
		t.Fatalf("window over, RetryAfter = %v", d)
	}

	// A success resets the count.
	f.Fail("u1")
	f.Fail("u1")
	f.Reset("u1")
	f.Fail("u1")
	f.Fail("u1")
	if f.RetryAfter("u1") != 0 {
		t.Fatal("Reset must forget earlier failures")
	}
	f.Reset("never-seen")
}

func TestFailureLimiterBoundedKeys(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	f := NewFailureLimiter(FailureLimitConfig{Window: time.Hour, Limit: 1, MaxKeys: 10, Now: func() time.Time { return now }})
	for i := 0; i < 100; i++ {
		f.Fail(fmt.Sprint("k", i))
		now = now.Add(time.Second)
	}
	if n := f.rl.size(); n != 10 {
		t.Fatalf("tracked %d keys, want the 10 newest", n)
	}
	if f.RetryAfter("k0") != 0 || f.RetryAfter("k99") == 0 {
		t.Fatal("the oldest key must be evicted first")
	}
}

func TestRetryAfterSeconds(t *testing.T) {
	for d, want := range map[time.Duration]string{0: "1", 500 * time.Millisecond: "1", time.Second: "1", 1500 * time.Millisecond: "2", 15 * time.Minute: "900"} {
		if got := RetryAfterSeconds(d); got != want {
			t.Errorf("RetryAfterSeconds(%v) = %s, want %s", d, got, want)
		}
	}
}
