package contract

import (
	"encoding/json"
	"strings"
	"testing"
)

const (
	msgSessionOnly   = "This endpoint is not available with an API token. Sign in to the dashboard instead."
	msgReadOnly      = `This API token is read-only (scope "read" allows GET requests only)`
	msgDangerous     = `This action asks for the account password in the dashboard. An API token can perform it only when it was created with "allow dangerous actions".`
	msgInvalidToken  = "Invalid API token"
	reAPITokenString = `^dl_[A-Za-z0-9_-]{43}$`
)

func apiTokenShape(name, scope string, dangerous bool, expires any) Obj {
	return Obj{
		"id": UUID, "name": name, "prefix": Re(`^dl_[A-Za-z0-9_-]{8}$`), "user_id": UUID, "scope": scope,
		"allow_dangerous": dangerous, "created_at": ISOms, "last_used_at": nil, "expires_at": expires, "status": "active",
	}
}

// createToken makes a token through the API and returns it and its id.
func createToken(t *testing.T, c *Client, body map[string]any) (string, string) {
	t.Helper()
	body["password"] = admin.Password
	r := c.Post(t, "/api/tokens", body)
	o := r.Obj(t)
	if r.Status != 201 {
		t.Fatalf("create token: %s", r)
	}
	tok, _ := o["token"].(string)
	item, _ := o["apiToken"].(map[string]any)
	id, _ := item["id"].(string)
	return tok, id
}

func TestAPITokens(t *testing.T) {
	c := admin.Client()
	name := uniq("ci")

	t.Run("create validation and step-up", func(t *testing.T) {
		mainInst.Anon().Get(t, "/api/tokens").ExpectError(t, 401, "Authentication required")
		c.Post(t, "/api/tokens", map[string]any{"name": name}).Expect(t, 403, Obj{"error": "Password confirmation required for this operation", "requirePassword": true})
		c.Post(t, "/api/tokens", map[string]any{"password": admin.Password}).ExpectError(t, 400, "name is required (max 100 characters)")
		c.Post(t, "/api/tokens", map[string]any{"name": name, "scope": "root", "password": admin.Password}).ExpectError(t, 400, `scope must be "read" or "full"`)
		c.Post(t, "/api/tokens", map[string]any{"name": name, "allowDangerous": true, "password": admin.Password}).ExpectError(t, 400, `allowDangerous needs scope "full"`)
		c.Post(t, "/api/tokens", map[string]any{"name": name, "expiresInDays": 4000, "password": admin.Password}).
			ExpectError(t, 400, "expiresInDays must be a whole number from 1 to 3650, or null for no expiry")
		c.Post(t, "/api/tokens", map[string]any{"name": name, "scope": "full", "expiresInDays": 7, "password": admin.Password}).
			Expect(t, 201, Obj{"token": Re(reAPITokenString), "apiToken": apiTokenShape(name, "full", false, ISOms)})
	})

	read, _ := createToken(t, c, map[string]any{"name": uniq("reader")})
	full, _ := createToken(t, c, map[string]any{"name": uniq("writer"), "scope": "full"})
	danger, dangerID := createToken(t, c, map[string]any{"name": uniq("ops"), "scope": "full", "allowDangerous": true})
	rc, fc, dc := mainInst.As(read), mainInst.As(full), mainInst.As(danger)

	t.Run("list never shows the token", func(t *testing.T) {
		r := c.Get(t, "/api/tokens")
		r.Expect(t, 200, Obj{"tokens": ArrayOf(Obj{
			"id": UUID, "name": Str, "prefix": Str, "user_id": UUID, "scope": Either("read", "full"), "allow_dangerous": Bool,
			"created_at": ISOms, "last_used_at": OrNull(ISOms), "expires_at": OrNull(ISOms), "status": Either("active", "expired", "revoked"),
		}, 3)})
		for _, tok := range []string{read, full, danger} {
			if strings.Contains(string(r.Body), tok) {
				t.Fatal("token listed in clear")
			}
		}
	})

	t.Run("scopes", func(t *testing.T) {
		rc.Get(t, "/api/projects").ExpectStatus(t, 200)
		rc.Get(t, "/api/auth/me").Expect(t, 200, Obj{"user": Obj{"id": admin.UserID, "name": Str, "email": admin.Email, "role": "admin", "created_at": ISOms}})
		rc.Post(t, "/api/projects", map[string]any{"name": ""}).ExpectError(t, 403, msgReadOnly)
		rc.Delete(t, "/api/projects/x").ExpectError(t, 403, msgReadOnly)
		rc.Get(t, "/api/backup/download/x.zip").ExpectError(t, 403, `This endpoint needs an API token with scope "full"`)
		fc.Post(t, "/api/projects", map[string]any{"name": ""}).ExpectError(t, 400, "Project name is required (max 120 characters)")
		mainInst.As("dl_"+strings.Repeat("x", 43)).Get(t, "/api/projects").ExpectError(t, 401, msgInvalidToken)
	})

	t.Run("step-up needs allow_dangerous", func(t *testing.T) {
		for _, p := range []string{"/api/system/purge", "/api/system/reboot", "/api/system/reset", "/api/system/update-system",
			"/api/system/upgrade", "/api/system/execute", "/api/deployments/x/rollback", "/api/backup/restore/x.zip", "/api/backup/restore-from-upload/x.zip"} {
			fc.Post(t, p, map[string]any{"password": admin.Password, "command": "id"}).Expect(t, 403, Obj{"error": msgDangerous, "requirePassword": false})
		}
		dc.Post(t, "/api/deployments/"+randHex(4)+"/rollback", map[string]any{}).ExpectError(t, 400, "deploymentId is required")
	})

	t.Run("session-only routes", func(t *testing.T) {
		for _, s := range []struct {
			method, path string
			body         any
		}{
			{"GET", "/api/tokens", nil},
			{"POST", "/api/tokens", map[string]any{"name": "x", "password": admin.Password}},
			{"DELETE", "/api/tokens/" + dangerID, map[string]any{"password": admin.Password}},
			{"PATCH", "/api/auth/profile", map[string]any{"name": "x", "email": "x@example.com"}},
			{"POST", "/api/auth/change-password", map[string]any{"currentPassword": admin.Password, "newPassword": "something-else-1"}},
			{"POST", "/api/auth/sse-token", nil},
			{"POST", "/api/auth/terminal-token", nil},
			{"PUT", "/api/domains/panel.contract-test.dev/access", map[string]any{"domainOnly": false}},
		} {
			dc.Do(t, s.method, s.path, s.body).ExpectError(t, 403, msgSessionOnly)
		}
	})

	t.Run("openapi document", func(t *testing.T) {
		mainInst.Anon().Get(t, "/api/openapi.json").ExpectError(t, 401, "Authentication required")
		r := rc.Get(t, "/api/openapi.json")
		var doc struct {
			OpenAPI string                    `json:"openapi"`
			Paths   map[string]map[string]any `json:"paths"`
		}
		if r.Status != 200 || json.Unmarshal(r.Body, &doc) != nil || doc.OpenAPI != "3.1.0" {
			t.Fatalf("openapi: %s", r)
		}
		n := 0
		for _, ops := range doc.Paths {
			n += len(ops)
		}
		if n != 109 {
			t.Errorf("document lists %d operations, want 109 (ROUTES.md)", n)
		}
	})

	t.Run("revoke", func(t *testing.T) {
		tok, id := createToken(t, c, map[string]any{"name": uniq("short")})
		mainInst.As(tok).Get(t, "/api/projects").ExpectStatus(t, 200)
		c.Do(t, "DELETE", "/api/tokens/"+id, map[string]any{}).Expect(t, 403, Obj{"error": "Password confirmation required for this operation", "requirePassword": true})
		c.Do(t, "DELETE", "/api/tokens/"+id, map[string]any{"password": admin.Password}).Expect(t, 200, Obj{"success": true})
		mainInst.As(tok).Get(t, "/api/projects").ExpectError(t, 401, msgInvalidToken)
		c.Do(t, "DELETE", "/api/tokens/"+id, map[string]any{"password": admin.Password}).ExpectError(t, 404, "API token not found")
	})
}

func TestMCPEndpoint(t *testing.T) {
	c := admin.Client()
	read, _ := createToken(t, c, map[string]any{"name": uniq("mcp-read")})
	full, _ := createToken(t, c, map[string]any{"name": uniq("mcp-full"), "scope": "full"})
	accept := []string{"Accept", "application/json, text/event-stream"}
	initialize := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "contract", "version": "1"}}}

	t.Run("auth", func(t *testing.T) {
		r := mainInst.Anon().Post(t, "/mcp", initialize, accept...)
		if r.Status != 401 || !strings.HasPrefix(r.Header.Get("WWW-Authenticate"), "Bearer") {
			t.Fatalf("anonymous: %s %v", r, r.Header)
		}
		r = c.Post(t, "/mcp", initialize, accept...)
		if r.Status != 401 {
			t.Fatalf("session jwt accepted: %s", r)
		}
		if r := mainInst.As(read).Post(t, "/mcp", initialize, append(accept, "Origin", "https://evil.example")...); r.Status != 403 {
			t.Fatalf("foreign origin: %s", r)
		}
		for _, m := range []string{"GET", "DELETE"} {
			if r := mainInst.As(read).Do(t, m, "/mcp", nil, accept...); r.Status != 405 {
				t.Errorf("%s /mcp: %s", m, r)
			}
		}
	})

	t.Run("initialize, list, call", func(t *testing.T) {
		rpc := func(tok string, msg map[string]any) map[string]any {
			t.Helper()
			r := mainInst.As(tok).Post(t, "/mcp", msg, append(accept, "MCP-Protocol-Version", "2025-11-25")...)
			if r.Status != 200 {
				t.Fatalf("%v: %s", msg["method"], r)
			}
			return r.Obj(t)
		}
		res := rpc(read, initialize)["result"].(map[string]any)
		if res["serverInfo"].(map[string]any)["name"] != "docklift" {
			t.Fatalf("initialize: %v", res)
		}
		tools := rpc(read, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list"})["result"].(map[string]any)["tools"].([]any)
		if len(tools) < 20 {
			t.Fatalf("%d tools", len(tools))
		}
		call := func(tok, name string, args map[string]any) (string, bool) {
			t.Helper()
			out := rpc(tok, map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
			result := out["result"].(map[string]any)
			text := ""
			for _, c := range result["content"].([]any) {
				text += c.(map[string]any)["text"].(string)
			}
			return text, result["isError"] == true
		}
		if text, isErr := call(read, "docklift_list_projects", map[string]any{}); isErr || !strings.Contains(text, "GET /api/projects -> HTTP 200") {
			t.Fatalf("list projects: %s", text)
		}
		if text, isErr := call(read, "docklift_create_project", map[string]any{"name": "x", "github_url": "https://github.com/a/b"}); !isErr || !strings.Contains(text, "HTTP 403") {
			t.Fatalf("read token on write tool: %s", text)
		}
		if text, isErr := call(full, "docklift_api_request", map[string]any{"method": "POST", "path": "/api/projects", "body": map[string]any{"name": ""}}); !isErr || !strings.Contains(text, "HTTP 400") {
			t.Fatalf("validation through the router: %s", text)
		}
		if text, isErr := call(full, "docklift_api_request", map[string]any{"method": "POST", "path": "/api/system/reboot", "body": map[string]any{"password": admin.Password}}); !isErr || !strings.Contains(text, "HTTP 403") {
			t.Fatalf("step-up without allow_dangerous: %s", text)
		}
	})
}
