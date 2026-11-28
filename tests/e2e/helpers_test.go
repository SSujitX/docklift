package e2e

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// config is read once from the environment (see README.md).
type config struct {
	baseURL    string
	bootstrap  string
	email      string
	password   string
	proxyURL   string
	installDir string
	repo       string
	repoBranch string
	repoDir    string
	repoPort   int
	domain     string
	engines    []string
	restore    bool
	docker     bool
}

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func loadConfig(t *testing.T) config {
	t.Helper()
	c := config{
		baseURL:    strings.TrimRight(env("DOCKLIFT_E2E_URL", ""), "/"),
		bootstrap:  env("DOCKLIFT_E2E_BOOTSTRAP_SECRET", ""),
		email:      env("DOCKLIFT_E2E_ADMIN_EMAIL", "e2e-admin@docklift.test"),
		password:   env("DOCKLIFT_E2E_ADMIN_PASSWORD", ""),
		proxyURL:   strings.TrimRight(env("DOCKLIFT_E2E_PROXY_URL", "http://127.0.0.1:80"), "/"),
		installDir: env("DOCKLIFT_E2E_INSTALL_DIR", "/opt/docklift"),
		repo:       env("DOCKLIFT_E2E_REPO", "https://github.com/SSujitX/docklift-test"),
		repoBranch: env("DOCKLIFT_E2E_REPO_BRANCH", "master"),
		repoDir:    env("DOCKLIFT_E2E_REPO_DIR", "server-test"),
		domain:     env("DOCKLIFT_E2E_DOMAIN", ""),
		restore:    env("DOCKLIFT_E2E_RESTORE", "1") == "1",
		docker:     env("DOCKLIFT_E2E_DOCKER", "1") == "1",
	}
	fmt.Sscan(env("DOCKLIFT_E2E_REPO_PORT", "4000"), &c.repoPort)
	for _, e := range strings.Split(env("DOCKLIFT_E2E_ENGINES", "postgres,redis"), ",") {
		if e = strings.TrimSpace(e); e != "" {
			c.engines = append(c.engines, e)
		}
	}
	if c.baseURL == "" {
		t.Skip("set DOCKLIFT_E2E_URL (and DOCKLIFT_E2E_BOOTSTRAP_SECRET or admin credentials) to run the end-to-end suite")
	}
	if c.password == "" {
		t.Fatal("DOCKLIFT_E2E_ADMIN_PASSWORD is required (the admin password to register or log in with)")
	}
	return c
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// client is a minimal JSON client for the Docklift API.
type client struct {
	base  string
	token string
	http  *http.Client
}

type response struct {
	Status int
	Header http.Header
	Body   []byte
}

func (r *response) obj(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(r.Body, &m); err != nil {
		t.Fatalf("response is not a JSON object (HTTP %d): %s", r.Status, truncate(string(r.Body), 400))
	}
	return m
}

func (r *response) arr(t *testing.T) []any {
	t.Helper()
	var a []any
	if err := json.Unmarshal(r.Body, &a); err != nil {
		t.Fatalf("response is not a JSON array (HTTP %d): %s", r.Status, truncate(string(r.Body), 400))
	}
	return a
}

func (r *response) want(t *testing.T, status int) *response {
	t.Helper()
	if r.Status != status {
		t.Fatalf("want HTTP %d, got %d: %s", status, r.Status, truncate(string(r.Body), 1500))
	}
	return r
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func (c *client) do(t *testing.T, method, path string, body any, hdr map[string]string) *response {
	t.Helper()
	req := c.request(t, context.Background(), method, path, body, hdr)
	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("%s %s: read body: %v", method, path, err)
	}
	return &response{Status: resp.StatusCode, Header: resp.Header, Body: b}
}

func (c *client) request(t *testing.T, ctx context.Context, method, path string, body any, hdr map[string]string) *http.Request {
	t.Helper()
	var rd io.Reader
	ct := ""
	switch b := body.(type) {
	case nil:
	case *multipartBody:
		rd, ct = bytes.NewReader(b.buf.Bytes()), b.contentType
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		rd, ct = bytes.NewReader(raw), "application/json"
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		t.Fatal(err)
	}
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	return req
}

func (c *client) get(t *testing.T, path string) *response {
	t.Helper()
	return c.do(t, "GET", path, nil, nil)
}
func (c *client) post(t *testing.T, path string, body any) *response {
	t.Helper()
	return c.do(t, "POST", path, body, nil)
}
func (c *client) put(t *testing.T, path string, body any) *response {
	t.Helper()
	return c.do(t, "PUT", path, body, nil)
}
func (c *client) del(t *testing.T, path string) *response {
	t.Helper()
	return c.do(t, "DELETE", path, nil, nil)
}

// stream is a streamed (chunked text) response read incrementally.
type stream struct {
	Status int
	resp   *http.Response
	cancel context.CancelFunc
	lines  chan string
	buf    strings.Builder
}

func (c *client) open(t *testing.T, method, path string, body any) *stream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	resp, err := c.http.Do(c.request(t, ctx, method, path, body, nil))
	if err != nil {
		cancel()
		t.Fatalf("%s %s: %v", method, path, err)
	}
	s := &stream{Status: resp.StatusCode, resp: resp, cancel: cancel, lines: make(chan string, 4096)}
	go func() {
		defer close(s.lines)
		r := bufio.NewReader(resp.Body)
		for {
			line, err := r.ReadString('\n')
			if line != "" {
				s.lines <- line
			}
			if err != nil {
				return
			}
		}
	}()
	return s
}

// until reads until the accumulated text contains marker (true) or the
// stream ends / times out (false).
func (s *stream) until(marker string, timeout time.Duration) bool {
	deadline := time.After(timeout)
	for !strings.Contains(s.buf.String(), marker) {
		select {
		case l, ok := <-s.lines:
			if !ok {
				return strings.Contains(s.buf.String(), marker)
			}
			s.buf.WriteString(l)
		case <-deadline:
			return false
		}
	}
	return true
}

// all reads to the end of the stream (bounded by timeout).
func (s *stream) all(timeout time.Duration) string {
	deadline := time.After(timeout)
	for {
		select {
		case l, ok := <-s.lines:
			if !ok {
				s.close()
				return s.buf.String()
			}
			s.buf.WriteString(l)
		case <-deadline:
			s.close()
			return s.buf.String() + "\n[e2e: stream read timed out]"
		}
	}
}

func (s *stream) text() string { return s.buf.String() }

func (s *stream) close() {
	s.cancel()
	_ = s.resp.Body.Close()
}

type multipartBody struct {
	buf         bytes.Buffer
	contentType string
}

// form builds a multipart body; files maps field name → (filename, content).
func form(fields map[string]string, fileField, fileName string, content []byte) *multipartBody {
	m := &multipartBody{}
	w := multipart.NewWriter(&m.buf)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	if fileField != "" {
		fw, _ := w.CreateFormFile(fileField, fileName)
		_, _ = fw.Write(content)
	}
	_ = w.Close()
	m.contentType = w.FormDataContentType()
	return m
}

func buildZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// sh runs a host command (docker CLI on the Docker host of the stack).
func sh(t *testing.T, name string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return string(out), err
}

func mustSh(t *testing.T, name string, args ...string) string {
	t.Helper()
	out, err := sh(t, name, args...)
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return out
}

// eventually retries f until it returns nil or the timeout passes.
func eventually(t *testing.T, what string, timeout time.Duration, f func() error) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var err error
	for {
		if err = f(); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: %v", what, err)
		}
		time.Sleep(2 * time.Second)
	}
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// httpGet fetches a URL with an optional Host header (proxy checks).
func httpGet(rawURL, host string) (int, string, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return 0, "", err
	}
	if host != "" {
		req.Host = host
	}
	c := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, string(b), nil
}

func urlQuery(s string) string { return url.QueryEscape(s) }

func wsURL(base, path, token string) string {
	u, _ := url.Parse(base + path)
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.RawQuery = "token=" + url.QueryEscape(token)
	return u.String()
}
