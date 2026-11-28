package router

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"

	"docklift/internal/app/apptest"
)

// bigJSON is a body just under httpx.MaxBodyBytes.
var bigJSON = []byte(`{"name":"` + strings.Repeat("a", 10*1024*1024-64) + `"}`)

// allocatedDuring reports the bytes the process allocated while fn ran.
func allocatedDuring(fn func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	fn()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// sendParallel posts body n times at once and returns the status codes.
func sendParallel(t *testing.T, url string, n int, body []byte, chunked bool, headers map[string]string) []int {
	t.Helper()
	return sendParallelMethod(t, http.MethodPost, url, n, body, chunked, headers)
}

func sendParallelMethod(t *testing.T, method, url string, n int, body []byte, chunked bool, headers map[string]string) []int {
	t.Helper()
	codes := make([]int, n)
	// A fresh transport per batch: connections a previous batch left mid-upload
	// are never reused.
	tr := &http.Transport{DisableKeepAlives: true}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr}
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			var rd io.Reader = bytes.NewReader(body)
			if chunked {
				rd = io.MultiReader(rd) // hides the length: Transfer-Encoding: chunked
			}
			req, err := http.NewRequest(method, url, rd)
			if err != nil {
				t.Error(err)
				return
			}
			req.Header.Set("Content-Type", "application/json")
			for k, v := range headers {
				req.Header.Set(k, v)
			}
			resp, err := client.Do(req)
			if err != nil {
				// The server may close the connection after answering while
				// the client is still uploading; that is a refusal too.
				codes[i] = -1
				return
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			codes[i] = resp.StatusCode
		})
	}
	wg.Wait()
	return codes
}

// TestUnauthenticatedBodiesAreNotBuffered is the regression test of the
// unauthenticated memory exhaustion: bodies used to be read and decoded
// (up to 10 MB each, several times that once decoded) before any auth
// guard ran. Twenty parallel 10 MB bodies without credentials must now cost
// the server next to nothing, whichever group they target.
func TestUnauthenticatedBodiesAreNotBuffered(t *testing.T) {
	d := apptest.NewDeps(t)
	srv := httptest.NewServer(New(d))
	t.Cleanup(srv.Close)
	const n = 20
	// Before the fix every request below buffered at least its 10 MB body.
	const budget = n * 1024 * 1024

	cases := []struct {
		name    string
		path    string
		chunked bool
		headers map[string]string
		want    int
	}{
		{"protected group", "/api/projects", false, nil, 401},
		{"bad bearer token", "/api/system/execute", false, map[string]string{"Authorization": "Bearer not.a.jwt"}, 401},
		{"backup without setup token", "/api/backup/restore-upload", false, nil, 401},
		// The auth routes are public: their small limit refuses the body
		// from Content-Length, or after 64 KB when the length is hidden.
		{"login", "/api/auth/login", false, nil, 413},
		{"login, chunked", "/api/auth/login", true, nil, 413},
		{"github public callback", "/api/github/setup", false, nil, 413},
		// The webhook reads nothing without a configured secret.
		{"github webhook", "/api/github/webhook", false, map[string]string{"X-GitHub-Event": "push", "X-Hub-Signature-256": "sha256=00"}, 401},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var codes []int
			alloc := allocatedDuring(func() {
				codes = sendParallel(t, srv.URL+tc.path, n, bigJSON, tc.chunked, tc.headers)
			})
			for i, c := range codes {
				if c != tc.want && c != -1 {
					t.Fatalf("request %d: status %d, want %d", i, c, tc.want)
				}
			}
			if alloc > budget {
				t.Fatalf("%d parallel unauthenticated 10 MB bodies allocated %d MB, want under %d MB",
					n, alloc>>20, budget>>20)
			}
		})
	}
}

// TestAuthenticatedBodiesStillParsed checks the move of the parser behind
// the guards: an authenticated request still gets its full 10 MB body.
func TestAuthenticatedBodiesStillParsed(t *testing.T) {
	d := apptest.NewDeps(t)
	_, session := apptest.CreateAdmin(t, d, "ada@example.com", "password123")
	srv := httptest.NewServer(New(d))
	t.Cleanup(srv.Close)
	bearer := map[string]string{"Authorization": "Bearer " + session}

	// PATCH of a missing project answers 404 only after its handler read the
	// parsed body; a 413 or 400 here would mean the body was refused.
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/api/projects/missing", bytes.NewReader(bigJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearer["Authorization"])
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("authenticated 10 MB body: %d %s", resp.StatusCode, body)
	}
	over := append(append([]byte(`{"name":"`), bytes.Repeat([]byte("a"), 10*1024*1024)...), '"', '}')
	if codes := sendParallelMethod(t, http.MethodPatch, srv.URL+"/api/projects/missing", 1, over, false, bearer); codes[0] != 413 && codes[0] != -1 {
		t.Fatalf("authenticated body over 10 MB: %d", codes[0])
	}
	// Malformed JSON behind the guard is still the client's mistake.
	req, _ = http.NewRequest(http.MethodPatch, srv.URL+"/api/projects/missing", strings.NewReader(`{"name":`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearer["Authorization"])
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("authenticated malformed JSON: %d", resp.StatusCode)
	}
}

// TestSessionCheckLimiter covers the cheap authenticated checks: they do not
// spend the shared auth budget (so a shared address is not locked out of the
// dashboard) and have their own, larger one.
func TestSessionCheckLimiter(t *testing.T) {
	d := apptest.NewDeps(t)
	_, session := apptest.CreateAdmin(t, d, "ada@example.com", "password123")
	h := New(d)
	bearer := map[string]string{"Authorization": "Bearer " + session}
	for _, p := range sessionCheckPaths {
		method := "POST"
		if p == "/api/auth/me" {
			method = "GET"
		}
		r := do(t, h, method, p, bearer)
		if r.code != 200 {
			t.Fatalf("%s: %d %s", p, r.code, r.body)
		}
		if got := r.header.Get("RateLimit-Limit"); got != "1000" {
			t.Errorf("%s RateLimit-Limit = %q, want its own limiter (1000)", p, got)
		}
	}
	r := do(t, h, "POST", "/api/auth/login", nil)
	if got := r.header.Get("RateLimit-Remaining"); got != "99" {
		t.Errorf("session checks spent the auth budget: RateLimit-Remaining = %q", got)
	}
}
