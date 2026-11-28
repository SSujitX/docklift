package mcpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDispatchStreamTailAndWaitLimit(t *testing.T) {
	var gotAuth, gotIP, gotXFF string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotIP, gotXFF = r.Header.Get("Authorization"), r.RemoteAddr, r.Header.Get("X-Forwarded-For")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		line := strings.Repeat("x", 1023) + "\n"
		for i := 0; ; i++ {
			if _, err := w.Write([]byte(line)); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
				_, _ = w.Write([]byte("LAST LINE\n"))
				return
			case <-time.After(time.Millisecond):
			}
		}
	})
	d := &Dispatcher{Handler: func() http.Handler { return h }}
	outer := httptest.NewRequest("POST", "/mcp", nil)
	outer.RemoteAddr = "203.0.113.9:4242"
	outer.Header.Set("Authorization", "Bearer dl_x")
	outer.Header.Set("X-Forwarded-For", "198.51.100.1")
	outer.Header.Set("Cookie", "secret=1")
	start := time.Now()
	resp, err := d.Do(t.Context(), outer, Request{Method: "POST", Path: "/api/deployments/p/deploy", Wait: 300 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("wait limit ignored")
	}
	if !resp.Stream || !resp.TimedOut || !resp.Truncated || len(resp.Body) > MaxStreamBytes || !strings.HasSuffix(string(resp.Body), "LAST LINE\n") {
		t.Fatalf("stream=%v timedOut=%v truncated=%v len=%d", resp.Stream, resp.TimedOut, resp.Truncated, len(resp.Body))
	}
	if gotAuth != "Bearer dl_x" || gotIP != "203.0.113.9:4242" || gotXFF != "198.51.100.1" {
		t.Fatalf("forwarded: %q %q %q", gotAuth, gotIP, gotXFF)
	}
	res := render(Request{Method: "POST", Path: "/api/x"}, resp)
	text := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "Stopped following the stream") || !strings.Contains(text, "only the last part is shown") {
		t.Fatalf("render notes: %.300s", text)
	}
}

func TestDispatchJSONHeadCap(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" {
			t.Error("cookie forwarded")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`"` + strings.Repeat("a", MaxResponseBytes+10) + `"`))
	})
	d := &Dispatcher{Handler: func() http.Handler { return h }}
	outer := httptest.NewRequest("POST", "/mcp", nil)
	outer.Header.Set("Cookie", "secret=1")
	resp, err := d.Do(t.Context(), outer, Request{Method: "GET", Path: "/api/projects"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Stream || !resp.Truncated || len(resp.Body) != MaxResponseBytes || resp.Body[0] != '"' {
		t.Fatalf("json cap: stream=%v truncated=%v len=%d", resp.Stream, resp.Truncated, len(resp.Body))
	}
	for _, bad := range []string{"/mcp", "/api/../x", "/api//x", "/api/x?y"} {
		if _, err := d.Do(t.Context(), outer, Request{Method: "GET", Path: bad}); err == nil {
			t.Errorf("path %q accepted", bad)
		}
	}
}

func TestSSEText(t *testing.T) {
	raw := "data: {\"type\":\"connected\",\"container\":\"web\"}\n\ndata: {\"type\":\"log\",\"message\":\"hello\\n\"}\n\ndata: {\"type\":\"end\",\"message\":\"Log stream ended\"}\n\n"
	if got := sseText([]byte(raw)); got != "[connected to web]\nhello\n[end] Log stream ended\n" {
		t.Fatalf("sseText: %q", got)
	}
}
