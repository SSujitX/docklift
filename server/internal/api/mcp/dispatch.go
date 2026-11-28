package mcpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Output caps of one in-process request.
const (
	// MaxResponseBytes is the most of a JSON (or other non-stream) response
	// returned to the agent; longer bodies are cut at the end.
	MaxResponseBytes = 256 << 10
	// MaxStreamBytes is the most of a streamed log kept: its tail.
	MaxStreamBytes = 64 << 10
	// DefaultWait and MaxWait bound how long a streamed response (deploy
	// output, restore, backup) is followed before the tool returns.
	DefaultWait = 10 * time.Minute
	MaxWait     = 30 * time.Minute
	// requestTimeout bounds a non-streaming request.
	requestTimeout = 5 * time.Minute
)

// forwardedHeaders are copied from the MCP request to every in-process
// request, so the client address, scheme and host (and therefore rate
// limits, audit logs and trusted-proxy decisions) are the caller's.
var forwardedHeaders = []string{"Authorization", "X-Forwarded-For", "X-Forwarded-Proto", "X-Forwarded-Host", "User-Agent"}

// Request is one call through the router.
type Request struct {
	Method string
	// Path is the API path, starting with /api/.
	Path  string
	Query url.Values
	// Body is marshalled as JSON when non-nil.
	Body any
	// Wait bounds a streamed response (0: DefaultWait).
	Wait time.Duration
}

// Response is what the router answered.
type Response struct {
	Status      int
	ContentType string
	Body        []byte
	// Stream reports a text/plain or text/event-stream response.
	Stream bool
	// Truncated reports that output was dropped (the head of a stream, or
	// the end of a long JSON body).
	Truncated bool
	// TimedOut reports that a stream was still running when Wait ended.
	TimedOut bool
}

// JSON decodes the body into v.
func (r *Response) JSON(v any) error {
	d := json.NewDecoder(bytes.NewReader(r.Body))
	d.UseNumber()
	return d.Decode(v)
}

// ErrorMessage is the `error` of a JSON error body, or the raw text.
func (r *Response) ErrorMessage() string {
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(r.Body, &e) == nil && e.Error != "" {
		return e.Error
	}
	return strings.TrimSpace(string(r.Body))
}

// Dispatcher sends requests through the HTTP handler in process, with the
// credentials and client address of the MCP request.
type Dispatcher struct {
	// Handler is the root handler (the same middleware stack as over HTTP).
	Handler func() http.Handler
}

// Do runs req as outer's caller.
func (d *Dispatcher) Do(ctx context.Context, outer *http.Request, req Request) (*Response, error) {
	if !safeAPIPath(req.Path) {
		return nil, fmt.Errorf("path must be a clean path starting with /api/")
	}
	u := &url.URL{Path: req.Path}
	if len(req.Query) > 0 {
		u.RawQuery = req.Query.Encode()
	}
	var body io.Reader
	if req.Body != nil {
		raw, err := json.Marshal(req.Body)
		if err != nil {
			return nil, fmt.Errorf("encode body: %w", err)
		}
		body = bytes.NewReader(raw)
	}
	wait := req.Wait
	if wait <= 0 {
		wait = DefaultWait
	}
	wait = min(wait, MaxWait)
	// A fresh context: nothing from the MCP request's context (its
	// authenticated user, parsed body, peer trust) may leak into the inner
	// request, which the middleware stack authenticates again from scratch.
	// It still ends when the MCP request ends.
	inner, cancel := context.WithTimeout(context.Background(), max(wait, requestTimeout))
	defer cancel()
	stop := context.AfterFunc(ctx, cancel)
	defer stop()

	hr, err := http.NewRequestWithContext(inner, strings.ToUpper(req.Method), u.String(), body)
	if err != nil {
		return nil, err
	}
	hr.RemoteAddr = outer.RemoteAddr
	hr.Host = outer.Host
	hr.TLS = outer.TLS
	hr.Proto, hr.ProtoMajor, hr.ProtoMinor = outer.Proto, outer.ProtoMajor, outer.ProtoMinor
	for _, k := range forwardedHeaders {
		for _, v := range outer.Header.Values(k) {
			hr.Header.Add(k, v)
		}
	}
	hr.Header.Set("Accept", "application/json, text/plain, text/event-stream")
	if req.Body != nil {
		hr.Header.Set("Content-Type", "application/json")
	}

	rec := newCapture()
	streamTimer := time.AfterFunc(wait, func() {
		if rec.isStream() {
			rec.markTimedOut()
			cancel()
		}
	})
	defer streamTimer.Stop()
	d.Handler().ServeHTTP(rec, hr)
	return rec.response(), nil
}

// capture is an http.ResponseWriter that keeps the head of normal bodies
// and the tail of streams.
type capture struct {
	mu        sync.Mutex
	header    http.Header
	status    int
	stream    bool
	buf       []byte
	truncated bool
	timedOut  bool
}

func newCapture() *capture { return &capture{header: http.Header{}} }

func (c *capture) Header() http.Header { return c.header }

func (c *capture) WriteHeader(code int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.status != 0 {
		return
	}
	c.status = code
	ct := c.header.Get("Content-Type")
	c.stream = strings.HasPrefix(ct, "text/plain") || strings.HasPrefix(ct, "text/event-stream")
}

func (c *capture) Write(p []byte) (int, error) {
	c.mu.Lock()
	if c.status == 0 {
		c.mu.Unlock()
		c.WriteHeader(http.StatusOK)
		c.mu.Lock()
	}
	defer c.mu.Unlock()
	if c.stream {
		c.buf = append(c.buf, p...)
		if over := len(c.buf) - MaxStreamBytes; over > 0 {
			c.buf = append(c.buf[:0:0], c.buf[over:]...)
			c.truncated = true
		}
		return len(p), nil
	}
	room := MaxResponseBytes - len(c.buf)
	if room < len(p) {
		c.truncated = true
		if room > 0 {
			c.buf = append(c.buf, p[:room]...)
		}
		return len(p), nil
	}
	c.buf = append(c.buf, p...)
	return len(p), nil
}

// Flush lets streaming handlers flush (nothing to do in memory).
func (c *capture) Flush() {}

func (c *capture) isStream() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stream
}

func (c *capture) markTimedOut() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.timedOut = true
}

func (c *capture) response() *Response {
	c.mu.Lock()
	defer c.mu.Unlock()
	status := c.status
	if status == 0 {
		status = http.StatusOK
	}
	return &Response{
		Status: status, ContentType: c.header.Get("Content-Type"), Body: append([]byte(nil), c.buf...),
		Stream: c.stream, Truncated: c.truncated, TimedOut: c.timedOut,
	}
}
