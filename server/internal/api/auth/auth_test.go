package auth_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"docklift/internal/app"
	"docklift/internal/app/apptest"
	"docklift/internal/app/router"
	authsvc "docklift/internal/auth"
	"docklift/internal/store"
)

type result struct {
	code int
	body map[string]any
}

func call(t *testing.T, h http.Handler, method, path, body string, headers map[string]string) result {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return result{rec.Code, out}
}

func setup(t *testing.T) (*app.Deps, http.Handler, string) {
	t.Helper()
	d := apptest.NewDeps(t)
	secret, err := d.Auth.Bootstrap().EnsureSecret()
	if err != nil {
		t.Fatal(err)
	}
	return d, router.New(d), secret
}

func TestRegisterFlow(t *testing.T) {
	d, h, secret := setup(t)
	r := call(t, h, "POST", "/api/auth/register", `{"name":"Ada","email":"Ada@Example.COM","password":"password123"}`,
		map[string]string{"X-Bootstrap-Secret": "  " + secret + "  "})
	if r.code != 201 || r.body["message"] != "Registration successful" {
		t.Fatalf("register: %d %v", r.code, r.body)
	}
	user := r.body["user"].(map[string]any)
	if user["email"] != "ada@example.com" || user["role"] != "admin" || len(user) != 4 {
		t.Errorf("user = %v", user)
	}
	if d.Auth.Bootstrap().ReadSecret() != "" {
		t.Error("bootstrap secret must be consumed")
	}
	entries, _ := os.ReadDir(d.Config.DataPath)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".claimed-") {
			t.Errorf("claim file left behind: %s", e.Name())
		}
	}
	claims, err := d.Auth.Verify(r.body["token"].(string))
	if err != nil || claims.UserID != user["id"] || !claims.HasPwdv || claims.Pwdv == 0 {
		t.Errorf("session claims = %+v, %v", claims, err)
	}
	again := call(t, h, "POST", "/api/auth/register", `{"name":"B","email":"b@example.com","password":"password123"}`, nil)
	if again.code != 403 || again.body["error"] != "Setup already complete. Use login instead." {
		t.Errorf("second register: %d %v", again.code, again.body)
	}
	status := call(t, h, "GET", "/api/auth/status", "", nil)
	if status.code != 200 || status.body["setupComplete"] != true || status.body["userCount"] != float64(1) || status.body["bootstrapRequired"] != false {
		t.Errorf("status = %v", status.body)
	}
	if d.Auth.Bootstrap().ReadSecret() != "" {
		t.Error("status must not recreate the secret once an admin exists")
	}
}

func TestConcurrentRegistrationOnlyOneWins(t *testing.T) {
	_, h, secret := setup(t)
	var wg sync.WaitGroup
	codes := make([]int, 4)
	for i := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body := `{"name":"U","email":"u` + string(rune('a'+i)) + `@example.com","password":"password123"}`
			codes[i] = call(t, h, "POST", "/api/auth/register", body, map[string]string{"X-Bootstrap-Secret": secret}).code
		}()
	}
	wg.Wait()
	created := 0
	for _, c := range codes {
		switch c {
		case 201:
			created++
		case 403:
		default:
			t.Errorf("unexpected status %d", c)
		}
	}
	// On Windows a rename can fail while another request is reading the
	// secret file, so every racer may lose there; never may two win.
	if created > 1 || (created == 0 && runtime.GOOS != "windows") {
		t.Errorf("%d registrations succeeded, want exactly 1 (codes %v)", created, codes)
	}
}

func TestFailedRegistrationRestoresClaim(t *testing.T) {
	d, h, secret := setup(t)
	// Make the insert after the claim fail.
	if _, err := d.DB.Writer().ExecContext(t.Context(), `CREATE TRIGGER fail_user_insert BEFORE INSERT ON users BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatal(err)
	}
	r := call(t, h, "POST", "/api/auth/register", `{"name":"x","email":"a@b.c","password":"password123"}`, map[string]string{"X-Bootstrap-Secret": secret})
	if r.code != 500 || r.body["error"] != "Registration failed" {
		t.Fatalf("failed insert: %d %v", r.code, r.body)
	}
	if d.Auth.Bootstrap().ReadSecret() != secret {
		t.Error("a failed registration must put the bootstrap secret back")
	}
}

func TestStatusOnUnmigratedDatabase(t *testing.T) {
	d, h, _ := setup(t)
	if _, err := d.DB.Writer().Exec(`DROP TABLE users`); err != nil {
		t.Fatal(err)
	}
	r := call(t, h, "GET", "/api/auth/status", "", nil)
	if r.code != 200 || r.body["needsRestore"] != true || r.body["setupComplete"] != false || r.body["bootstrapRequired"] != true || r.body["userCount"] != float64(0) {
		t.Errorf("missing users table: %d %v", r.code, r.body)
	}
}

func TestSetupTokenIsStable(t *testing.T) {
	d, h, secret := setup(t)
	first := call(t, h, "GET", "/api/auth/setup-token", "", map[string]string{"X-Bootstrap-Secret": secret})
	second := call(t, h, "GET", "/api/auth/setup-token", "", map[string]string{"X-Bootstrap-Secret": secret})
	tok, _ := first.body["setupToken"].(string)
	if first.code != 200 || len(tok) != 64 || second.body["setupToken"] != tok {
		t.Errorf("setup token: %v %v", first.body, second.body)
	}
	if !d.Auth.Bootstrap().ValidateSetupToken(tok) {
		t.Error("issued token must validate")
	}
}

func TestSessionRoutes(t *testing.T) {
	d, h, _ := setup(t)
	u, token := apptest.CreateAdmin(t, d, "ada@example.com", "password123")
	bearer := map[string]string{"Authorization": "Bearer " + token}

	me := call(t, h, "GET", "/api/auth/me", "", bearer)
	got := me.body["user"].(map[string]any)
	if me.code != 200 || got["id"] != u.ID || got["created_at"] == nil || len(got) != 5 {
		t.Errorf("me = %d %v", me.code, me.body)
	}
	for _, purpose := range []string{"sse", "terminal"} {
		r := call(t, h, "POST", "/api/auth/"+purpose+"-token", "", bearer)
		c, err := d.Auth.Verify(r.body["token"].(string))
		if r.code != 200 || err != nil || c.Purpose != purpose || c.Pwdv != float64(u.PasswordVersion()) {
			t.Errorf("%s token: %d %+v %v", purpose, r.code, c, err)
		}
	}
	// The 2.x X-Internal-Secret bypass is gone: the stored internal
	// secret is not a credential.
	internal := map[string]string{"X-Internal-Secret": "apptest-internal-secret"}
	if r := call(t, h, "POST", "/api/auth/sse-token", "", internal); r.code != 401 || r.body["error"] != "Authentication required" {
		t.Errorf("internal secret header: %d %v", r.code, r.body)
	}
}

// Changing the password ends the user's open SSE streams and terminal
// sessions at once (they were authenticated by tokens of the old password).
func TestChangePasswordRevokesOpenSessions(t *testing.T) {
	d, h, _ := setup(t)
	u, token := apptest.CreateAdmin(t, d, "ada@example.com", "password123")
	bearer := map[string]string{"Authorization": "Bearer " + token}
	sse := call(t, h, "POST", "/api/auth/sse-token", "", bearer).body["token"].(string)
	claims, err := d.Auth.Verify(sse)
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := d.Auth.BindSession(context.Background(), claims)
	defer stop()
	other, _ := apptest.CreateAdmin(t, d, "bob@example.com", "password123")
	otherClaims, _ := d.Auth.Verify(mustPurpose(t, d, other))
	otherCtx, otherStop := d.Auth.BindSession(context.Background(), otherClaims)
	defer otherStop()

	r := call(t, h, "POST", "/api/auth/change-password", `{"currentPassword":"password123","newPassword":"password456"}`, bearer)
	if r.code != 200 {
		t.Fatalf("change-password: %d %v", r.code, r.body)
	}
	select {
	case <-ctx.Done():
		if !errors.Is(context.Cause(ctx), authsvc.ErrSessionRevoked) {
			t.Errorf("cause = %v", context.Cause(ctx))
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("session of %s still open after its password changed", u.Email)
	}
	if otherCtx.Err() != nil {
		t.Error("another user's session must stay open")
	}
}

func mustPurpose(t *testing.T, d *app.Deps, u *store.User) string {
	t.Helper()
	tok, err := d.Auth.IssuePurpose(authsvc.TokenSubject{UserID: u.ID, Email: u.Email, Role: u.Role, PasswordVersion: u.PasswordVersion()}, authsvc.PurposeSSE)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// TestClientMistakesAre400 covers deliberate fix 3: wrong-typed or missing
// fields (and a missing body) are 400s with a clear message, never 500.
func TestClientMistakesAre400(t *testing.T) {
	d, h, secret := setup(t)
	_, token := apptest.CreateAdmin(t, d, "ada@example.com", "password123")
	bearer := map[string]string{"Authorization": "Bearer " + token}
	bootstrap := map[string]string{"X-Bootstrap-Secret": secret}
	cases := []struct {
		name, method, path, body string
		headers                  map[string]string
		code                     int
		err                      string
	}{
		{"login number email", "POST", "/api/auth/login", `{"email":42,"password":"x"}`, nil, 400, "Email and password must be strings"},
		{"login object password", "POST", "/api/auth/login", `{"email":"ada@example.com","password":{"$gt":""}}`, nil, 400, "Email and password must be strings"},
		{"login array email", "POST", "/api/auth/login", `{"email":["ada@example.com"],"password":"password123"}`, nil, 400, "Email and password must be strings"},
		{"login no body", "POST", "/api/auth/login", "", nil, 400, "Email and password are required"},
		{"login array body", "POST", "/api/auth/login", `[1]`, nil, 400, "Email and password are required"},
		{"register no body", "POST", "/api/auth/register", "", bootstrap, 400, "Name, email, and password are required"},
		{"register number name", "POST", "/api/auth/register", `{"name":1,"email":"a@b.c","password":"password123"}`, bootstrap, 400, "Name, email, and password must be strings"},
		{"register array password", "POST", "/api/auth/register", `{"name":"A","email":"a@b.c","password":["a","b","c","d","e","f","g","h"]}`, bootstrap, 400, "Name, email, and password must be strings"},
		{"register bool email", "POST", "/api/auth/register", `{"name":"A","email":true,"password":"password123"}`, bootstrap, 400, "Name, email, and password must be strings"},
		{"register short password counts UTF-16 units", "POST", "/api/auth/register", `{"name":"A","email":"a@b.c","password":"😀😀😀"}`, bootstrap, 400, "Password must be at least 8 characters"},
		{"profile number email", "PATCH", "/api/auth/profile", `{"name":"Ada","email":5}`, bearer, 400, "Name and email must be strings"},
		{"profile no body", "PATCH", "/api/auth/profile", "", bearer, 400, "Name and email are required"},
		{"change-password array", "POST", "/api/auth/change-password", `{"currentPassword":"password123","newPassword":["a","b","c","d","e","f","g","h"]}`, bearer, 400, "Current and new passwords must be strings"},
		{"change-password number current", "POST", "/api/auth/change-password", `{"currentPassword":12345678,"newPassword":"password456"}`, bearer, 400, "Current and new passwords must be strings"},
		{"change-password no body", "POST", "/api/auth/change-password", "", bearer, 400, "Current and new passwords are required"},
		{"malformed JSON", "POST", "/api/auth/login", `{"email":`, nil, 400, "Invalid JSON body"},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))
		if c.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		for k, v := range c.headers {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		if rec.Code != c.code || out["error"] != c.err {
			t.Errorf("%s: %d %v, want %d %q", c.name, rec.Code, out, c.code, c.err)
		}
	}
	// The bootstrap secret survives rejected registrations.
	if d.Auth.Bootstrap().ReadSecret() != secret {
		t.Error("a rejected registration must not consume the bootstrap secret")
	}
}

// Registering the first admin removes the fresh-install restore token (Docklift 2.x
// left it on disk, valid for the setup-token restore path).
func TestRegisterConsumesSetupToken(t *testing.T) {
	d, h, secret := setup(t)
	tok := call(t, h, "GET", "/api/auth/setup-token", "", map[string]string{"X-Bootstrap-Secret": secret})
	token, _ := tok.body["setupToken"].(string)
	if token == "" {
		t.Fatalf("setup token: %d %v", tok.code, tok.body)
	}
	r := call(t, h, "POST", "/api/auth/register", `{"name":"Ada","email":"ada@example.com","password":"password123"}`,
		map[string]string{"X-Bootstrap-Secret": secret})
	if r.code != 201 {
		t.Fatalf("register: %d %v", r.code, r.body)
	}
	if _, err := os.Stat(d.Auth.Bootstrap().SetupTokenPath()); !os.IsNotExist(err) {
		t.Fatalf(".setup-token must be removed after registration (err=%v)", err)
	}
	if d.Auth.Bootstrap().ValidateSetupToken(token) {
		t.Fatal("setup token must no longer validate")
	}
}

// A stolen session must not allow unlimited guesses at the current password.
func TestChangePasswordThrottlesGuesses(t *testing.T) {
	d, h, _ := setup(t)
	_, session := apptest.CreateAdmin(t, d, "guess@example.com", "password123")
	bearer := map[string]string{"Authorization": "Bearer " + session}
	wrong := `{"currentPassword":"not-the-password","newPassword":"password456"}`
	for i := 0; i < authsvc.StepUpFailureLimit; i++ {
		if r := call(t, h, "POST", "/api/auth/change-password", wrong, bearer); r.code != 400 || r.body["error"] != "Invalid current password" {
			t.Fatalf("attempt %d: %d %v", i, r.code, r.body)
		}
	}
	// Throttled now, even with the right password.
	r := call(t, h, "POST", "/api/auth/change-password", `{"currentPassword":"password123","newPassword":"password456"}`, bearer)
	if r.code != 429 || r.body["error"] != authsvc.MsgTooManyPasswordAttempts {
		t.Fatalf("throttled: %d %v", r.code, r.body)
	}
}
