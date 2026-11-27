package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"docklift/internal/auth"
	"docklift/internal/config"
	"docklift/internal/db"
	"docklift/internal/store"
)

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// setEnv points every path setting at dir, like the smoke-test recipe.
func setEnv(t *testing.T, dir string, port int) {
	t.Helper()
	for k, v := range map[string]string{
		"PORT":                strconv.Itoa(port),
		"DATABASE_URL":        "file:" + filepath.ToSlash(filepath.Join(dir, "data", "docklift.db")),
		"DATA_PATH":           filepath.Join(dir, "data"),
		"DEPLOYMENTS_PATH":    filepath.Join(dir, "deployments"),
		"BACKUP_PATH":         filepath.Join(dir, "backups"),
		"NGINX_CONF_PATH":     filepath.Join(dir, "nginx"),
		"LETSENCRYPT_PATH":    filepath.Join(dir, "letsencrypt"),
		"DOCKER_HOST":         "tcp://127.0.0.1:1",
		"JWT_SECRET":          "",
		"INTERNAL_API_SECRET": "",
		"CORS_ORIGIN":         "",
	} {
		t.Setenv(k, v)
	}
}

type apiClient struct {
	t    *testing.T
	base string
}

func (c apiClient) do(method, path, body string, headers map[string]string) (int, http.Header, map[string]any) {
	c.t.Helper()
	req, err := http.NewRequest(method, c.base+path, strings.NewReader(body))
	if err != nil {
		c.t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, resp.Header, out
}

func TestServeEndToEnd(t *testing.T) {
	dir := t.TempDir()
	port := freePort(t)
	setEnv(t, dir, port)
	stdout := os.Stdout
	devnull, _ := os.Open(os.DevNull)
	os.Stdout = devnull
	defer func() { os.Stdout = stdout; devnull.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, quietLog()) }()
	c := apiClient{t: t, base: "http://127.0.0.1:" + strconv.Itoa(port)}

	deadline := time.Now().Add(20 * time.Second)
	for {
		resp, err := http.Get(c.base + "/api/health")
		if err == nil {
			resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server did not start: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	code, _, health := c.do("GET", "/api/health", "", nil)
	if code != 200 || health["status"] != "ok" || health["version"] == "" || health["instanceId"] == "" {
		t.Fatalf("health = %d %v", code, health)
	}
	for _, f := range []string{".secrets", ".bootstrap-secret", "docklift.db"} {
		if _, err := os.Stat(filepath.Join(dir, "data", f)); err != nil {
			t.Errorf("%s not created: %v", f, err)
		}
	}
	for _, d := range []string{"deployments", "backups"} {
		if st, err := os.Stat(filepath.Join(dir, d)); err != nil || !st.IsDir() {
			t.Errorf("%s directory not created", d)
		}
	}

	secret, _ := os.ReadFile(filepath.Join(dir, "data", ".bootstrap-secret"))
	code, hdr, reg := c.do("POST", "/api/auth/register", `{"name":"Ada","email":"ada@example.com","password":"password123"}`,
		map[string]string{"X-Bootstrap-Secret": strings.TrimSpace(string(secret))})
	if code != 201 || hdr.Get("RateLimit-Limit") != "100" || hdr.Get("RateLimit-Policy") != "100;w=900" {
		t.Fatalf("register = %d %v %v", code, reg, hdr)
	}

	code, hdr, login := c.do("POST", "/api/auth/login", `{"email":"ADA@example.com","password":"password123"}`, nil)
	if code != 200 || hdr.Get("RateLimit-Remaining") != "98" {
		t.Fatalf("login = %d %v remaining=%s", code, login, hdr.Get("RateLimit-Remaining"))
	}
	oldToken := login["token"].(string)

	code, _, me := c.do("GET", "/api/auth/me", "", map[string]string{"Authorization": "Bearer " + oldToken})
	if code != 200 || me["user"].(map[string]any)["email"] != "ada@example.com" {
		t.Fatalf("me = %d %v", code, me)
	}

	code, _, changed := c.do("POST", "/api/auth/change-password", `{"currentPassword":"password123","newPassword":"newpassword1"}`,
		map[string]string{"Authorization": "Bearer " + oldToken})
	if code != 200 || changed["message"] != "Password changed successfully" {
		t.Fatalf("change-password = %d %v", code, changed)
	}
	if code, _, body := c.do("GET", "/api/auth/me", "", map[string]string{"Authorization": "Bearer " + oldToken}); code != 401 || body["error"] != "Session expired. Please log in again." {
		t.Errorf("old token after password change = %d %v", code, body)
	}
	if code, _, _ := c.do("GET", "/api/auth/me", "", map[string]string{"Authorization": "Bearer " + changed["token"].(string)}); code != 200 {
		t.Errorf("new token = %d", code)
	}

	seal := filepath.Join(dir, "data", ".restore-critical")
	if err := os.WriteFile(seal, []byte(`{"at":"","detail":"test","liveDbPath":"","preRestorePath":""}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, body := c.do("GET", "/api/auth/status", "", nil); code != 503 || body["critical"] != true || body["maintenance"] != true {
		t.Errorf("critical gate = %d %v", code, body)
	}
	if code, _, _ := c.do("GET", "/api/health", "", nil); code != 200 {
		t.Errorf("health under critical seal = %d", code)
	}
	if err := os.Remove(seal); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := c.do("GET", "/api/auth/status", "", nil); code != 200 {
		t.Errorf("gate must lift once the seal is gone = %d", code)
	}
	if code, hdr, _ := c.do("GET", "/", "", nil); code != 200 || hdr.Get("Cache-Control") != "no-cache" {
		t.Errorf("dashboard = %d %v", code, hdr)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("graceful shutdown returned %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("server did not shut down")
	}
}

func TestMigrateCommand(t *testing.T) {
	dir := t.TempDir()
	setEnv(t, dir, freePort(t))
	if err := migrate(context.Background(), quietLog()); err != nil {
		t.Fatal(err)
	}
	d, err := db.Open(context.Background(), filepath.Join(dir, "data", "docklift.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if res, err := d.Migrate(context.Background()); err != nil || len(res.Applied) != 0 {
		t.Errorf("migrate left pending migrations: %v %v", res.Applied, err)
	}
}

func TestResetAdminPassword(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	setEnv(t, dir, freePort(t))
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = stdout }()

	d, err := db.Open(ctx, cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var exit exitCode
	if err := resetAdminPassword(ctx, cfg); !errors.As(err, &exit) || exit != 1 {
		t.Errorf("no admin: err = %v, want exit 1", err)
	}
	st := store.New(d)
	old := db.FromUnixMilli(1000)
	u, err := st.CreateUser(ctx, store.UserCreate{Name: "Ada", Email: "ada@example.com", Password: "x", PasswordChangedAt: &old})
	if err != nil {
		t.Fatal(err)
	}
	if err := resetAdminPassword(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	w.Close()
	out, _ := io.ReadAll(r)
	idx := strings.Index(string(out), "Password: ")
	if idx < 0 || !strings.Contains(string(out), "No admin user found") || !strings.Contains(string(out), "User:     ada@example.com") {
		t.Fatalf("output:\n%s", out)
	}
	password := strings.TrimSpace(strings.SplitN(string(out[idx+len("Password: "):]), "\n", 2)[0])
	if len(password) != 16 {
		t.Errorf("password %q must be 16 base64 characters", password)
	}
	after, err := st.GetUser(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !auth.ComparePassword(after.Password, password) || after.PasswordVersion() <= 1000 {
		t.Errorf("password not reset or sessions not revoked: pwdv=%d", after.PasswordVersion())
	}
	_ = d.Close()
}

func TestRunDispatch(t *testing.T) {
	stderr := os.Stderr
	devnull, _ := os.Open(os.DevNull)
	os.Stderr = devnull
	defer func() { os.Stderr = stderr; devnull.Close() }()
	if code := run([]string{"help"}); code != 0 {
		t.Errorf("help = %d", code)
	}
	if code := run([]string{"bogus"}); code != 2 {
		t.Errorf("unknown command = %d", code)
	}
	if code := run([]string{"serve", "extra"}); code != 2 {
		t.Errorf("extra argument = %d", code)
	}
}
