package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"

	"docklift/internal/app"
	"docklift/internal/app/apptest"
	"docklift/internal/app/router"
	authsvc "docklift/internal/auth"
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
	r := call(t, h, "POST", "/api/auth/register", `{"name":["x"],"email":"a@b.c","password":"password123"}`, map[string]string{"X-Bootstrap-Secret": secret})
	if r.code != 500 || r.body["error"] != "Registration failed" {
		t.Fatalf("non-string name: %d %v", r.code, r.body)
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
	internal := map[string]string{authsvc.InternalSecretHeader: "apptest-internal-secret"}
	r := call(t, h, "POST", "/api/auth/sse-token", "", internal)
	c, err := d.Auth.Verify(r.body["token"].(string))
	if r.code != 200 || err != nil || c.UserID != authsvc.InternalUserID || c.Pwdv != 0 {
		t.Errorf("internal sse token: %d %+v %v", r.code, c, err)
	}
	if r := call(t, h, "PATCH", "/api/auth/profile", `{"name":"I","email":"i@x"}`, internal); r.code != 500 || r.body["error"] != "Failed to update profile" {
		t.Errorf("profile for the internal caller (no user row): %d %v", r.code, r.body)
	}
	if r := call(t, h, "PATCH", "/api/auth/profile", `{"name":"Ada","email":5}`, bearer); r.code != 500 {
		t.Errorf("non-string email: %d", r.code)
	}
	if r := call(t, h, "POST", "/api/auth/change-password", `{"currentPassword":"password123","newPassword":["a","b","c","d","e","f","g","h"]}`, bearer); r.code != 500 || r.body["error"] != "Failed to change password" {
		t.Errorf("array newPassword passes the length check and then fails like bcrypt.hash: %d %v", r.code, r.body)
	}
}
