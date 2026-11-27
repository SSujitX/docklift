package github

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"docklift/internal/githubapp"
	"docklift/internal/httpx"
)

// countingReader records how many body bytes the handler read.
type countingReader struct {
	r    *bytes.Reader
	read int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.read += n
	return n, err
}

// TestWebhookReadsBodyOnlyWhenSigned mounts the webhook without ParseBody,
// as the router does: the handler reads the raw body itself only once the
// request carries a signature, and decodes it only after the HMAC matched.
func TestWebhookReadsBodyOnlyWhenSigned(t *testing.T) {
	e := newEnv(t)
	r := chi.NewRouter()
	mount(r, e.hs)
	send := func(body []byte, headers ...string) (*httptest.ResponseRecorder, int) {
		t.Helper()
		cr := &countingReader{r: bytes.NewReader(body)}
		req := httptest.NewRequest("POST", "/webhook", cr)
		req.ContentLength = int64(len(body))
		req.Header.Set("Content-Type", "application/json")
		for i := 0; i+1 < len(headers); i += 2 {
			req.Header.Set(headers[i], headers[i+1])
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec, cr.read
	}
	body := push("refs/heads/main", "https://github.com/acme/none", nil)

	for _, tc := range []struct {
		name    string
		headers []string
		want    string
	}{
		{"other event", []string{"X-GitHub-Event", "ping"}, `{"message":"Ignoring ping event"}`},
		{"no secret configured", []string{"X-GitHub-Event", "push", "X-Hub-Signature-256", sign("x", body)}, `{"error":"Webhook secret not configured"}`},
	} {
		if rec, n := send(body, tc.headers...); rec.Body.String() != tc.want || n != 0 {
			t.Errorf("%s: %s, read %d body bytes, want none", tc.name, rec.Body, n)
		}
	}
	e.set(githubapp.KeyWebhookSecret, "s3cret")
	if rec, n := send(body, "X-GitHub-Event", "push"); rec.Code != 401 || n != 0 {
		t.Errorf("unsigned: %d, read %d body bytes, want none", rec.Code, n)
	}
	if rec, _ := send(body, "X-GitHub-Event", "push", "X-Hub-Signature-256", sign("s3cret", body)); rec.Code != 200 ||
		!strings.Contains(rec.Body.String(), "No matching projects") {
		t.Errorf("signed push: %d %s", rec.Code, rec.Body)
	}
	// A malformed body is decoded (and refused) only when its signature is valid.
	bad := []byte(`{"ref":`)
	if rec, _ := send(bad, "X-GitHub-Event", "push", "X-Hub-Signature-256", sign("nope", bad)); rec.Code != 401 {
		t.Errorf("malformed, bad signature: %d %s", rec.Code, rec.Body)
	}
	if rec, _ := send(bad, "X-GitHub-Event", "push", "X-Hub-Signature-256", sign("s3cret", bad)); rec.Code != 400 ||
		rec.Body.String() != `{"error":"Invalid JSON body"}` {
		t.Errorf("malformed, signed: %d %s", rec.Code, rec.Body)
	}
	big := bytes.Repeat([]byte(" "), httpx.MaxBodyBytes+1)
	// Refused from Content-Length; the rest is drained, never buffered.
	if rec, _ := send(big, "X-GitHub-Event", "push", "X-Hub-Signature-256", sign("s3cret", big)); rec.Code != 413 {
		t.Errorf("over the limit: %d %s", rec.Code, rec.Body)
	}
	if used := httpx.PublicBodyBudget.InUse(); used != 0 {
		t.Errorf("public body budget not returned: %d", used)
	}
}
