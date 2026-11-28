package contract

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var httpClient = &http.Client{
	Timeout: 3 * time.Minute,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
	Transport: &http.Transport{Proxy: nil, MaxIdleConnsPerHost: 16, DisableCompression: true},
}

// Every client gets its own X-Forwarded-For address. The server trusts one
// proxy hop (express "trust proxy" = 1), so this is the client IP the rate
// limiters key on; unique values keep tests from sharing limiter buckets.
var (
	ipCounter atomic.Uint32
	ipBase    = func() byte { b := make([]byte, 1); rand.Read(b); return b[0] }()
)

func nextIP() string {
	n := ipCounter.Add(1)
	return fmt.Sprintf("10.%d.%d.%d", ipBase, byte(n>>8), byte(n))
}

// Client sends requests to one instance as one (optionally authenticated) caller.
type Client struct {
	inst   *Instance
	token  string
	ip     string
	header http.Header
}

func (i *Instance) Anon() *Client { return &Client{inst: i, ip: nextIP(), header: http.Header{}} }

func (i *Instance) As(token string) *Client {
	c := i.Anon()
	c.token = token
	return c
}

func (s *Session) Client() *Client { return s.Inst.As(s.Token) }

// With returns a copy that also sends header k: v.
func (c *Client) With(k, v string) *Client {
	n := &Client{inst: c.inst, token: c.token, ip: c.ip, header: c.header.Clone()}
	n.header.Set(k, v)
	return n
}

// Resp is a fully read response.
type Resp struct {
	Status int
	Header http.Header
	Body   []byte
	Method string
	Path   string
}

func (r *Resp) String() string {
	return fmt.Sprintf("%s %s -> %d %s", r.Method, r.Path, r.Status, truncate(string(r.Body), 600))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("…(%d bytes)", len(s))
}

// Multipart is a multipart/form-data body.
type Multipart struct {
	Fields []([2]string)
	Files  []FilePart
}

type FilePart struct {
	Field, Filename, ContentType string
	Data                         []byte
}

// RawBody sends Data verbatim with the given content type ("" = no header).
type RawBody struct {
	ContentType string
	Data        []byte
}

func (c *Client) buildRequest(ctx context.Context, method, path string, body any, headers map[string]string) (*http.Request, error) {
	var rdr io.Reader
	contentType := ""
	switch b := body.(type) {
	case nil:
	case RawBody:
		rdr = bytes.NewReader(b.Data)
		contentType = b.ContentType
	case *Multipart:
		buf := &bytes.Buffer{}
		w := multipart.NewWriter(buf)
		for _, f := range b.Fields {
			if err := w.WriteField(f[0], f[1]); err != nil {
				return nil, err
			}
		}
		for _, f := range b.Files {
			h := textproto.MIMEHeader{}
			h.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, f.Field, f.Filename))
			ct := f.ContentType
			if ct == "" {
				ct = "application/octet-stream"
			}
			h.Set("Content-Type", ct)
			pw, err := w.CreatePart(h)
			if err != nil {
				return nil, err
			}
			pw.Write(f.Data)
		}
		w.Close()
		rdr = buf
		contentType = w.FormDataContentType()
	case url.Values:
		rdr = strings.NewReader(b.Encode())
		contentType = "application/x-www-form-urlencoded"
	default:
		j, err := json.Marshal(b)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(j)
		contentType = "application/json"
	}
	req, err := http.NewRequestWithContext(ctx, method, c.inst.BaseURL+path, rdr)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("X-Forwarded-For", c.ip)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	for k, vs := range c.header {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}
	for k, v := range headers {
		if strings.EqualFold(k, "Host") {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}
	return req, nil
}

func (c *Client) raw(method, path string, body any, headers map[string]string) (*Resp, error) {
	req, err := c.buildRequest(context.Background(), method, path, body, headers)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return &Resp{Status: resp.StatusCode, Header: resp.Header, Body: b, Method: method, Path: path}, nil
}

// Do sends a request and reads the whole response. body: nil, JSON-able value,
// *Multipart, url.Values or RawBody.
func (c *Client) Do(t *testing.T, method, path string, body any, headers ...string) *Resp {
	t.Helper()
	h := map[string]string{}
	for i := 0; i+1 < len(headers); i += 2 {
		h[headers[i]] = headers[i+1]
	}
	r, err := c.raw(method, path, body, h)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return r
}

func (c *Client) Get(t *testing.T, path string, headers ...string) *Resp {
	t.Helper()
	return c.Do(t, "GET", path, nil, headers...)
}

func (c *Client) Post(t *testing.T, path string, body any, headers ...string) *Resp {
	t.Helper()
	return c.Do(t, "POST", path, body, headers...)
}

func (c *Client) Put(t *testing.T, path string, body any, headers ...string) *Resp {
	t.Helper()
	return c.Do(t, "PUT", path, body, headers...)
}

func (c *Client) Patch(t *testing.T, path string, body any, headers ...string) *Resp {
	t.Helper()
	return c.Do(t, "PATCH", path, body, headers...)
}

func (c *Client) Delete(t *testing.T, path string, headers ...string) *Resp {
	t.Helper()
	return c.Do(t, "DELETE", path, nil, headers...)
}

// Stream is an open streaming response (text/plain deploy logs, SSE).
type Stream struct {
	Status int
	Header http.Header
	resp   *http.Response
	rd     *bufio.Reader
	cancel context.CancelFunc
	got    bytes.Buffer
}

// Open starts a request and returns once response headers arrive.
func (c *Client) Open(t *testing.T, method, path string, body any) *Stream {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	req, err := c.buildRequest(ctx, method, path, body, nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("%s %s: %v", method, path, err)
	}
	s := &Stream{Status: resp.StatusCode, Header: resp.Header, resp: resp, rd: bufio.NewReader(resp.Body), cancel: cancel}
	t.Cleanup(s.Close)
	return s
}

// ReadUntil reads until the accumulated body contains marker (or EOF/timeout).
func (s *Stream) ReadUntil(marker string, timeout time.Duration) (string, bool) {
	deadline := time.Now().Add(timeout)
	buf := make([]byte, 4096)
	for !strings.Contains(s.got.String(), marker) {
		if time.Now().After(deadline) {
			return s.got.String(), false
		}
		n, err := s.rd.Read(buf)
		s.got.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return s.got.String(), strings.Contains(s.got.String(), marker)
}

// ReadAll drains the stream.
func (s *Stream) ReadAll() string {
	b, _ := io.ReadAll(s.rd)
	s.got.Write(b)
	return s.got.String()
}

func (s *Stream) Close() {
	s.cancel()
	s.resp.Body.Close()
}

// ---------------------------------------------------------------------------
// Small helpers

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// uniq returns a per-test unique, slug-safe name.
func uniq(prefix string) string { return prefix + "-" + randHex(4) }

// jwtClaims decodes (without verifying) the payload of a JWT.
func jwtClaims(t *testing.T, token string) map[string]any {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("not a JWT: %q", token)
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("jwt payload: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("jwt payload json: %v", err)
	}
	return m
}

func jwtHeader(t *testing.T, token string) map[string]any {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(strings.Split(token, ".")[0])
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	json.Unmarshal(b, &m)
	return m
}
