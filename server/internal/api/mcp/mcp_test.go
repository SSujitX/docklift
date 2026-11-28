package mcpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"docklift/internal/api/tokens"
	"docklift/internal/app"
	"docklift/internal/app/apptest"
	"docklift/internal/app/router"
	"docklift/internal/auth"
	"docklift/internal/store"
)

type env struct {
	t   *testing.T
	d   *app.Deps
	srv *httptest.Server
	u   *store.User
}

func newEnv(t *testing.T) *env {
	t.Helper()
	d := apptest.NewDeps(t)
	u, _ := apptest.CreateAdmin(t, d, "ada@example.com", "password123")
	srv := httptest.NewServer(router.New(d))
	t.Cleanup(srv.Close)
	return &env{t: t, d: d, srv: srv, u: u}
}

func (e *env) token(scope string, dangerous bool) string {
	e.t.Helper()
	_, plain, err := tokens.Issue(context.Background(), e.d.Store, e.u.ID, tokens.CreateInput{Name: "mcp-" + scope, Scope: scope, AllowDangerous: dangerous}, time.Now())
	if err != nil {
		e.t.Fatal(err)
	}
	return plain
}

// rpc posts one JSON-RPC message and returns the status, headers and body.
func (e *env) rpc(token, protocol string, msg map[string]any, extra map[string]string) (int, http.Header, map[string]any) {
	e.t.Helper()
	raw, _ := json.Marshal(msg)
	req, _ := http.NewRequest("POST", e.srv.URL+"/mcp", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if protocol != "" {
		req.Header.Set("MCP-Protocol-Version", protocol)
	}
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(body, &out)
	if out == nil && len(body) > 0 {
		out = map[string]any{"raw": string(body)}
	}
	return resp.StatusCode, resp.Header, out
}

func TestMCPRequiresAPIToken(t *testing.T) {
	e := newEnv(t)
	init := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t", "version": "1"}}}
	for _, tok := range []string{"", "dl_" + strings.Repeat("A", 43)} {
		code, h, _ := e.rpc(tok, "", init, nil)
		if code != 401 || !strings.HasPrefix(h.Get("WWW-Authenticate"), "Bearer") {
			t.Fatalf("token %q: %d %q", tok, code, h.Get("WWW-Authenticate"))
		}
	}
	// A session JWT is not accepted on /mcp.
	session, _ := e.d.Auth.IssueSession(e.u)
	if code, h, _ := e.rpc(session, "", init, nil); code != 401 || h.Get("WWW-Authenticate") == "" {
		t.Fatalf("session jwt: %d", code)
	}
	// An untrusted browser Origin is refused before anything else.
	if code, _, _ := e.rpc(e.token("read", false), "", init, map[string]string{"Origin": "https://evil.example"}); code != 403 {
		t.Fatalf("foreign origin: %d", code)
	}
}

func TestMCPLegacyInitializeListAndCall(t *testing.T) {
	e := newEnv(t)
	tok := e.token("full", false)
	code, _, out := e.rpc(tok, "", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t", "version": "1"}}}, nil)
	if code != 200 {
		t.Fatalf("initialize: %d %v", code, out)
	}
	result, _ := out["result"].(map[string]any)
	info, _ := result["serverInfo"].(map[string]any)
	if info["name"] != "docklift" || result["protocolVersion"] != "2025-11-25" || result["instructions"] == nil {
		t.Fatalf("initialize result: %v", out)
	}

	code, _, out = e.rpc(tok, "2025-11-25", map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list"}, nil)
	if code != 200 {
		t.Fatalf("tools/list: %d %v", code, out)
	}
	list, _ := out["result"].(map[string]any)["tools"].([]any)
	names := map[string]bool{}
	for _, raw := range list {
		tool := raw.(map[string]any)
		names[tool["name"].(string)] = true
		if tool["description"] == "" || tool["inputSchema"] == nil {
			t.Errorf("tool %v lacks description or schema", tool["name"])
		}
	}
	for _, want := range []string{"docklift_list_projects", "docklift_deploy", "docklift_set_env", "docklift_api_request", "docklift_list_endpoints", "docklift_create_backup"} {
		if !names[want] {
			t.Errorf("tools/list lacks %s", want)
		}
	}

	code, _, out = e.rpc(tok, "2025-11-25", map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/call",
		"params": map[string]any{"name": "docklift_list_projects", "arguments": map[string]any{}}}, nil)
	text := toolText(t, code, out)
	if !strings.Contains(text, "GET /api/projects -> HTTP 200") || !strings.Contains(text, "[]") {
		t.Fatalf("list projects: %s", text)
	}
}

func toolText(t *testing.T, code int, out map[string]any) string {
	t.Helper()
	if code != 200 {
		t.Fatalf("tools/call: %d %v", code, out)
	}
	res, _ := out["result"].(map[string]any)
	content, _ := res["content"].([]any)
	var b strings.Builder
	for _, c := range content {
		b.WriteString(c.(map[string]any)["text"].(string))
	}
	if res["isError"] == true {
		return "ERROR: " + b.String()
	}
	return b.String()
}

// connect opens an SDK client session with the token (the client picks
// the newest protocol version it supports).
func connect(t *testing.T, e *env, token string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	hc := &http.Client{Transport: headerRT{token: token}}
	s, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{Endpoint: e.srv.URL + "/mcp", HTTPClient: hc}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

type headerRT struct{ token string }

func (h headerRT) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+h.token)
	return http.DefaultTransport.RoundTrip(r)
}

func callText(t *testing.T, s *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := s.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String(), res.IsError
}

func TestMCPToolsGoThroughTheRouter(t *testing.T) {
	e := newEnv(t)
	s := connect(t, e, e.token("full", false))

	tools, err := s.ListTools(t.Context(), nil)
	if err != nil || len(tools.Tools) < 20 {
		t.Fatalf("list tools: %v %d", err, len(tools.Tools))
	}
	if text, isErr := callText(t, s, "docklift_list_endpoints", map[string]any{"tag": "Projects"}); isErr || !strings.Contains(text, "POST /api/projects/{id}/env") {
		t.Fatalf("list endpoints: %s", text)
	}
	// Validation is the handler's.
	if text, isErr := callText(t, s, "docklift_api_request", map[string]any{"method": "POST", "path": "/api/projects", "body": map[string]any{"name": ""}}); !isErr || !strings.Contains(text, "HTTP 400") {
		t.Fatalf("validation: %v %s", isErr, text)
	}
	// Step-up routes need allow_dangerous.
	if text, isErr := callText(t, s, "docklift_api_request", map[string]any{"method": "POST", "path": "/api/system/execute", "body": map[string]any{"command": "id", "password": "password123"}}); !isErr || !strings.Contains(text, "HTTP 403") || !strings.Contains(text, "allow dangerous actions") {
		t.Fatalf("dangerous: %v %s", isErr, text)
	}
	// Session-only routes stay closed.
	if text, isErr := callText(t, s, "docklift_api_request", map[string]any{"method": "GET", "path": "/api/tokens"}); !isErr || !strings.Contains(text, auth.MsgSessionOnly) {
		t.Fatalf("tokens: %v %s", isErr, text)
	}
	// Only clean /api paths.
	for _, p := range []string{"/mcp", "/api/../mcp", "https://example.com/api/projects", "/api/projects%2Fx"} {
		if text, isErr := callText(t, s, "docklift_api_request", map[string]any{"method": "GET", "path": p}); !isErr {
			t.Errorf("path %s accepted: %s", p, text)
		}
	}
	// A missing project through a curated tool is the router's 404.
	if text, isErr := callText(t, s, "docklift_get_project", map[string]any{"projectId": "nope"}); !isErr || !strings.Contains(text, "HTTP 404") {
		t.Fatalf("get project: %s", text)
	}
}

func TestMCPReadTokenRefusedOnWriteTool(t *testing.T) {
	e := newEnv(t)
	s := connect(t, e, e.token("read", false))
	if text, isErr := callText(t, s, "docklift_list_projects", nil); isErr || !strings.Contains(text, "HTTP 200") {
		t.Fatalf("read tool: %s", text)
	}
	text, isErr := callText(t, s, "docklift_create_project", map[string]any{"name": "x", "github_url": "https://github.com/a/b"})
	if !isErr || !strings.Contains(text, "HTTP 403") || !strings.Contains(text, "This API token is read-only") {
		t.Fatalf("write tool with read token: %v %s", isErr, text)
	}
	if n, _ := e.d.Store.ListProjects(t.Context(), store.ProjectFilter{}, store.OrderNone); len(n) != 0 {
		t.Fatal("a project was created")
	}
}

func TestMCPDisallowedMethods(t *testing.T) {
	e := newEnv(t)
	tok := e.token("read", false)
	for _, m := range []string{"GET", "DELETE"} {
		req, _ := http.NewRequest(m, e.srv.URL+"/mcp", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Accept", "application/json, text/event-stream")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 405 {
			t.Errorf("%s /mcp: %d", m, resp.StatusCode)
		}
	}
}

// A modern (2026-07-28) client sends no initialize: per-request _meta and
// the mirrored Mcp-Method / Mcp-Name headers.
func TestMCPModernStatelessRequest(t *testing.T) {
	e := newEnv(t)
	tok := e.token("read", false)
	meta := map[string]any{
		"io.modelcontextprotocol/protocolVersion":    "2026-07-28",
		"io.modelcontextprotocol/clientInfo":         map[string]any{"name": "t", "version": "1"},
		"io.modelcontextprotocol/clientCapabilities": map[string]any{},
	}
	code, _, out := e.rpc(tok, "2026-07-28", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": "docklift_system_stats", "arguments": map[string]any{}, "_meta": meta}},
		map[string]string{"Mcp-Method": "tools/call", "Mcp-Name": "docklift_system_stats"})
	if text := toolText(t, code, out); !strings.Contains(text, "GET /api/system/stats -> HTTP") {
		t.Fatalf("modern call: %s", text)
	}
	// A header that disagrees with the body is refused (HeaderMismatch).
	code, _, out = e.rpc(tok, "2026-07-28", map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": map[string]any{"name": "docklift_system_stats", "arguments": map[string]any{}, "_meta": meta}},
		map[string]string{"Mcp-Method": "tools/call", "Mcp-Name": "docklift_list_projects"})
	if code != 400 {
		t.Fatalf("header mismatch: %d %v", code, out)
	}
}
