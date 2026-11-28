package router

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"docklift/internal/app"
	"docklift/internal/app/apptest"
	"docklift/internal/auth"
	"docklift/internal/db"
	"docklift/internal/store"
)

// doBody sends method path with a JSON body and headers from remote.
func doBody(t *testing.T, h http.Handler, remote, method, path, body string, headers map[string]string) response {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	req.RemoteAddr = remote
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	b, _ := io.ReadAll(rec.Body)
	return response{rec.Code, rec.Header(), string(b)}
}

func bearer(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

type tokenEnv struct {
	t       *testing.T
	d       *app.Deps
	h       http.Handler
	user    *store.User
	session string
}

func newTokenEnv(t *testing.T) *tokenEnv {
	d := apptest.NewDeps(t)
	u, session := apptest.CreateAdmin(t, d, "ada@example.com", "password123")
	return &tokenEnv{t: t, d: d, h: New(d), user: u, session: session}
}

// create makes a token through the API (session + step-up password).
func (e *tokenEnv) create(body string) (string, string) {
	e.t.Helper()
	r := doBody(e.t, e.h, "192.0.2.1:1", "POST", "/api/tokens", body, bearer(e.session))
	if r.code != 201 {
		e.t.Fatalf("create token: %d %s", r.code, r.body)
	}
	var out struct {
		Token    string `json:"token"`
		APIToken struct {
			ID     string `json:"id"`
			Prefix string `json:"prefix"`
			Scope  string `json:"scope"`
		} `json:"apiToken"`
	}
	if err := json.Unmarshal([]byte(r.body), &out); err != nil {
		e.t.Fatal(err)
	}
	if !auth.ValidAPITokenFormat(out.Token) || !strings.HasPrefix(out.Token, out.APIToken.Prefix) {
		e.t.Fatalf("bad token %q prefix %q", out.Token, out.APIToken.Prefix)
	}
	if strings.Contains(r.body, auth.HashAPIToken(out.Token)) {
		e.t.Fatal("response leaks the token hash")
	}
	return out.Token, out.APIToken.ID
}

func TestAPITokenCreateNeedsStepUp(t *testing.T) {
	e := newTokenEnv(t)
	if r := doBody(t, e.h, "192.0.2.1:1", "POST", "/api/tokens", `{"name":"ci"}`, bearer(e.session)); r.code != 403 || !strings.Contains(r.body, `"requirePassword":true`) {
		t.Fatalf("without password: %d %s", r.code, r.body)
	}
	if r := doBody(t, e.h, "192.0.2.1:1", "POST", "/api/tokens", `{"name":"ci","password":"wrong"}`, bearer(e.session)); r.code != 403 || r.errorMsg() != "Invalid password" {
		t.Fatalf("wrong password: %d %s", r.code, r.body)
	}
	for _, tc := range []struct{ body, err string }{
		{`{"password":"password123"}`, "name is required (max 100 characters)"},
		{`{"name":"x","scope":"admin","password":"password123"}`, `scope must be "read" or "full"`},
		{`{"name":"x","allowDangerous":true,"password":"password123"}`, `allowDangerous needs scope "full"`},
		{`{"name":"x","expiresInDays":0,"password":"password123"}`, "expiresInDays must be a whole number from 1 to 3650, or null for no expiry"},
		{`{"name":"x","expiresInDays":1.5,"password":"password123"}`, "expiresInDays must be a whole number from 1 to 3650, or null for no expiry"},
	} {
		if r := doBody(t, e.h, "192.0.2.1:1", "POST", "/api/tokens", tc.body, bearer(e.session)); r.code != 400 || r.errorMsg() != tc.err {
			t.Errorf("%s: %d %s", tc.body, r.code, r.body)
		}
	}
	tok, _ := e.create(`{"name":"ci","scope":"full","expiresInDays":30,"password":"password123"}`)
	row, err := e.d.Store.GetAPITokenByHash(t.Context(), auth.HashAPIToken(tok))
	if err != nil || row.ExpiresAt == nil || row.Scope != "full" || row.AllowDangerous {
		t.Fatalf("stored row %+v %v", row, err)
	}
	if d := time.Until(row.ExpiresAt.Time()); d < 29*24*time.Hour || d > 31*24*time.Hour {
		t.Fatalf("expiry in %v", d)
	}
	list := doBody(t, e.h, "192.0.2.1:1", "GET", "/api/tokens", "", bearer(e.session))
	if list.code != 200 || !strings.Contains(list.body, `"status":"active"`) || strings.Contains(list.body, "token_hash") || strings.Contains(list.body, tok) {
		t.Fatalf("list: %d %s", list.code, list.body)
	}
}

func TestAPITokenScopes(t *testing.T) {
	e := newTokenEnv(t)
	read, _ := e.create(`{"name":"reader","password":"password123"}`)
	full, _ := e.create(`{"name":"writer","scope":"full","password":"password123"}`)

	if r := doBody(t, e.h, "192.0.2.1:1", "GET", "/api/projects", "", bearer(read)); r.code != 200 {
		t.Fatalf("read GET: %d %s", r.code, r.body)
	}
	if r := doBody(t, e.h, "192.0.2.1:1", "HEAD", "/api/projects", "", bearer(read)); r.code != 200 {
		t.Fatalf("read HEAD: %d", r.code)
	}
	for _, m := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		if r := doBody(t, e.h, "192.0.2.1:1", m, "/api/projects/x", `{}`, bearer(read)); r.code != 403 || r.errorMsg() != auth.MsgAPITokenReadOnly {
			t.Errorf("read %s: %d %s", m, r.code, r.body)
		}
	}
	if r := doBody(t, e.h, "192.0.2.1:1", "GET", "/api/backup/download/x.zip", "", bearer(read)); r.code != 403 || r.errorMsg() != auth.MsgFullScopeOnly {
		t.Fatalf("read download: %d %s", r.code, r.body)
	}
	if r := doBody(t, e.h, "192.0.2.1:1", "GET", "/api/backup/download/x.zip", "", bearer(full)); r.code == 403 || r.code == 401 {
		t.Fatalf("full download refused: %d %s", r.code, r.body)
	}
	if r := doBody(t, e.h, "192.0.2.1:1", "POST", "/api/projects", `{"name":""}`, bearer(full)); r.code != 400 {
		t.Fatalf("full POST reaches the handler: %d %s", r.code, r.body)
	}
	// The identity is the token's owner.
	me := doBody(t, e.h, "192.0.2.1:1", "GET", "/api/auth/me", "", bearer(read))
	if me.code != 200 || !strings.Contains(me.body, e.user.ID) {
		t.Fatalf("me: %d %s", me.code, me.body)
	}
	// Log streams accept the Bearer API token instead of an SSE token.
	if r := doBody(t, e.h, "192.0.2.1:1", "GET", "/api/system/logs/nope", "", bearer(read)); r.code != 400 {
		t.Fatalf("sse path with token: %d %s", r.code, r.body)
	}
	// A session JWT on a stream path still needs the SSE token.
	if r := doBody(t, e.h, "192.0.2.1:1", "GET", "/api/system/logs/nope", "", bearer(e.session)); r.code != 401 {
		t.Fatalf("sse path with session: %d %s", r.code, r.body)
	}
}

func TestAPITokenStepUpNeedsAllowDangerous(t *testing.T) {
	e := newTokenEnv(t)
	plain, _ := e.create(`{"name":"ci","scope":"full","password":"password123"}`)
	danger, _ := e.create(`{"name":"ops","scope":"full","allowDangerous":true,"password":"password123"}`)

	// Every step-up route refuses a token without allow_dangerous, even
	// with the right password in the body.
	stepUp := []struct{ method, path string }{
		{"POST", "/api/system/purge"}, {"POST", "/api/system/reboot"}, {"POST", "/api/system/reset"},
		{"POST", "/api/system/update-system"}, {"POST", "/api/system/upgrade"}, {"POST", "/api/system/execute"},
		{"POST", "/api/deployments/x/rollback"}, {"POST", "/api/backup/restore/x.zip"},
		{"POST", "/api/backup/restore-from-upload/x.zip"},
		// clear-critical-restore answers 400 before its step-up while no seal
		// exists; it calls RequireStepUpPassword like the others.
	}
	for _, s := range stepUp {
		r := doBody(t, e.h, "192.0.2.1:1", s.method, s.path, `{"password":"password123","command":"id"}`, bearer(plain))
		if r.code != 403 || r.errorMsg() != auth.MsgDangerousNotAllowed {
			t.Errorf("%s %s without allow_dangerous: %d %s", s.method, s.path, r.code, r.body)
		}
	}
	// With allow_dangerous the step-up passes without a password.
	if r := doBody(t, e.h, "192.0.2.1:1", "POST", "/api/deployments/x/rollback", `{}`, bearer(danger)); r.code != 400 || r.errorMsg() != "deploymentId is required" {
		t.Fatalf("rollback with allow_dangerous: %d %s", r.code, r.body)
	}
	if r := doBody(t, e.h, "192.0.2.1:1", "POST", "/api/backup/restore/..zip", `{}`, bearer(danger)); r.code == 403 {
		t.Fatalf("restore with allow_dangerous refused: %d %s", r.code, r.body)
	}
}

func TestAPITokenNeverReachesSessionOnlyRoutes(t *testing.T) {
	e := newTokenEnv(t)
	tok, id := e.create(`{"name":"ops","scope":"full","allowDangerous":true,"password":"password123"}`)
	for _, s := range []struct{ method, path, body string }{
		{"GET", "/api/tokens", ""},
		{"POST", "/api/tokens", `{"name":"x","password":"password123"}`},
		{"DELETE", "/api/tokens/" + id, `{"password":"password123"}`},
		{"PATCH", "/api/auth/profile", `{"name":"x","email":"x@example.com"}`},
		{"POST", "/api/auth/change-password", `{"currentPassword":"password123","newPassword":"password456"}`},
		{"POST", "/api/auth/terminal-token", ""},
		{"POST", "/api/auth/sse-token", ""},
		{"PUT", "/api/domains/panel.example.com/access", `{"domainOnly":false}`},
	} {
		r := doBody(t, e.h, "192.0.2.1:1", s.method, s.path, s.body, bearer(tok))
		if r.code != 403 || r.errorMsg() != auth.MsgSessionOnly {
			t.Errorf("%s %s: %d %s", s.method, s.path, r.code, r.body)
		}
	}
	// The terminal WebSocket only takes a terminal purpose token, which a
	// token cannot obtain; a Bearer token on the upgrade is refused.
	r := doBody(t, e.h, "192.0.2.1:1", "GET", "/ws/terminal?token="+tok, "", map[string]string{
		"Connection": "Upgrade", "Upgrade": "websocket", "Origin": "http://example.com", "Host": "example.com",
		"Sec-WebSocket-Version": "13", "Sec-WebSocket-Key": "dGhlIHNhbXBsZSBub25jZQ==",
	})
	if r.code == http.StatusSwitchingProtocols {
		t.Fatalf("terminal upgrade with api token: %d", r.code)
	}
	// Nothing changed: the token still exists and the password is the same.
	if _, err := e.d.Store.GetAPIToken(t.Context(), id); err != nil {
		t.Fatal(err)
	}
}

func TestAPITokenExpiryRevokeAndPasswordChange(t *testing.T) {
	e := newTokenEnv(t)
	ctx := t.Context()
	plain, hash, prefix, err := auth.NewAPIToken()
	if err != nil {
		t.Fatal(err)
	}
	past := db.NewTime(time.Now().Add(-time.Minute))
	if _, err := e.d.Store.CreateAPIToken(ctx, store.APITokenCreate{Name: "old", TokenHash: hash, Prefix: prefix, UserID: e.user.ID, Scope: "full", ExpiresAt: &past}); err != nil {
		t.Fatal(err)
	}
	if r := doBody(t, e.h, "192.0.2.2:1", "GET", "/api/projects", "", bearer(plain)); r.code != 401 || r.errorMsg() != auth.MsgAPITokenExpired {
		t.Fatalf("expired: %d %s", r.code, r.body)
	}

	tok, id := e.create(`{"name":"ci","password":"password123"}`)
	if r := doBody(t, e.h, "192.0.2.2:1", "GET", "/api/projects", "", bearer(tok)); r.code != 200 {
		t.Fatalf("fresh token: %d", r.code)
	}
	row, _ := e.d.Store.GetAPIToken(ctx, id)
	if row.LastUsedAt == nil {
		t.Fatal("last_used_at not set")
	}
	// Revoking needs the password, then the token stops at once.
	if r := doBody(t, e.h, "192.0.2.2:1", "DELETE", "/api/tokens/"+id, "", bearer(e.session)); r.code != 403 {
		t.Fatalf("revoke without password: %d", r.code)
	}
	if r := doBody(t, e.h, "192.0.2.2:1", "DELETE", "/api/tokens/"+id, `{"password":"password123"}`, bearer(e.session)); r.code != 200 {
		t.Fatalf("revoke: %d %s", r.code, r.body)
	}
	if r := doBody(t, e.h, "192.0.2.2:1", "GET", "/api/projects", "", bearer(tok)); r.code != 401 || r.errorMsg() != auth.MsgInvalidAPIToken {
		t.Fatalf("revoked: %d %s", r.code, r.body)
	}
	if r := doBody(t, e.h, "192.0.2.2:1", "DELETE", "/api/tokens/"+id, `{"password":"password123"}`, bearer(e.session)); r.code != 404 {
		t.Fatalf("revoke twice: %d", r.code)
	}

	// A password change revokes every token created before it.
	tok2, _ := e.create(`{"name":"ci2","password":"password123"}`)
	later := db.NewTime(time.Now().Add(time.Second))
	if _, err := e.d.Store.UpdateUser(ctx, e.user.ID, store.UserUpdate{PasswordChangedAt: store.Some(&later)}); err != nil {
		t.Fatal(err)
	}
	if r := doBody(t, e.h, "192.0.2.2:1", "GET", "/api/projects", "", bearer(tok2)); r.code != 401 || r.errorMsg() != auth.MsgAPITokenRevoked {
		t.Fatalf("after password change: %d %s", r.code, r.body)
	}
}

func TestAPITokenGuessThrottle(t *testing.T) {
	e := newTokenEnv(t)
	good, _ := e.create(`{"name":"ci","password":"password123"}`)
	bogus, _, _, _ := auth.NewAPIToken()
	for i := 0; i < auth.APITokenFailureLimit; i++ {
		token := bogus
		if i%2 == 0 {
			token = "dl_short"
		}
		if r := doBody(t, e.h, "198.51.100.7:1", "GET", "/api/projects", "", bearer(token)); r.code != 401 {
			t.Fatalf("guess %d: %d %s", i, r.code, r.body)
		}
	}
	r := doBody(t, e.h, "198.51.100.7:1", "GET", "/api/projects", "", bearer(good))
	if r.code != 429 || r.header.Get("Retry-After") == "" || r.errorMsg() != auth.MsgTooManyAPITokenTries {
		t.Fatalf("after %d guesses: %d %s", auth.APITokenFailureLimit, r.code, r.body)
	}
	// Other addresses and session JWTs are not affected.
	if r := doBody(t, e.h, "198.51.100.8:1", "GET", "/api/projects", "", bearer(good)); r.code != 200 {
		t.Fatalf("other address: %d", r.code)
	}
	if r := doBody(t, e.h, "198.51.100.7:1", "GET", "/api/projects", "", bearer(e.session)); r.code != 200 {
		t.Fatalf("session from throttled address: %d", r.code)
	}
}

func TestAPITokenOwnerDeletedCascades(t *testing.T) {
	e := newTokenEnv(t)
	tok, id := e.create(`{"name":"ci","password":"password123"}`)
	if _, err := e.d.DB.Writer().ExecContext(context.Background(), `DELETE FROM "users" WHERE "id" = ?`, e.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.d.Store.GetAPIToken(t.Context(), id); err == nil {
		t.Fatal("token survived its owner")
	}
	if r := doBody(t, e.h, "192.0.2.3:1", "GET", "/api/projects", "", bearer(tok)); r.code != 401 {
		t.Fatalf("token of deleted owner: %d", r.code)
	}
}

func TestOpenAPIDocument(t *testing.T) {
	e := newTokenEnv(t)
	if r := doBody(t, e.h, "192.0.2.1:1", "GET", "/api/openapi.json", "", nil); r.code != 401 {
		t.Fatalf("anonymous: %d", r.code)
	}
	read, _ := e.create(`{"name":"reader","password":"password123"}`)
	r := doBody(t, e.h, "192.0.2.1:1", "GET", "/api/openapi.json", "", bearer(read))
	if r.code != 200 {
		t.Fatalf("openapi: %d %s", r.code, r.body)
	}
	var doc struct {
		OpenAPI string                               `json:"openapi"`
		Paths   map[string]map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal([]byte(r.body), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.OpenAPI != "3.1.0" || len(doc.Paths) < 80 {
		t.Fatalf("openapi %q with %d paths", doc.OpenAPI, len(doc.Paths))
	}
	op := doc.Paths["/api/projects/{id}/env"]["post"]
	body, _ := json.Marshal(op["requestBody"])
	if !strings.Contains(string(body), `"key"`) || !strings.Contains(string(body), `"required":["key"]`) {
		t.Fatalf("env create body: %s", body)
	}
	if doc.Paths["/api/system/reboot"]["post"]["x-docklift-step-up"] != true {
		t.Fatal("reboot not marked step-up")
	}
}
