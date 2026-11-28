package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"docklift/internal/config"
	"docklift/internal/db"
	"docklift/internal/panelaccess"
	"docklift/internal/store"
)

// writeHTTPSPanelConf writes the conf of a dashboard panel domain with HTTPS.
func writeHTTPSPanelConf(t *testing.T, dir, domain string) {
	t.Helper()
	conf := "# docklift-panel-domain\n# docklift-panel-port: 8080\n" +
		"server {\n    listen 443 ssl;\n    server_name " + domain + ";\n" +
		"    location / {\n        set $dashboard docklift-backend.docklift_network;\n        proxy_pass http://$dashboard:8000;\n    }\n}\n"
	if err := os.MkdirAll(filepath.Join(dir, "nginx"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nginx", domain+".conf"), []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
}

// captureStdout runs fn and returns what it printed.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	out := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		out <- string(b)
	}()
	runErr := fn()
	os.Stdout = stdout
	w.Close()
	return <-out, runErr
}

func domainOnlyRow(t *testing.T, cfg *config.Config) string {
	t.Helper()
	d, err := db.Open(context.Background(), cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	v, _, err := store.New(d).GetSetting(context.Background(), panelaccess.SettingKey)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestPanelAccessCommand(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	setEnv(t, dir, freePort(t))
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	d, err := db.Open(ctx, cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	_ = d.Close()

	step := func(name string, args []string, wantExit int, wantRow string, wantOut ...string) {
		t.Helper()
		out, err := captureStdout(t, func() error { return runPanelAccess(ctx, cfg, args) })
		var exit exitCode
		switch {
		case wantExit == 0 && err != nil:
			t.Errorf("%s: %v\n%s", name, err, out)
		case wantExit != 0 && (!errors.As(err, &exit) || int(exit) != wantExit):
			t.Errorf("%s: err = %v, want exit %d\n%s", name, err, wantExit, out)
		}
		for _, s := range wantOut {
			if !strings.Contains(out, s) {
				t.Errorf("%s: output lacks %q:\n%s", name, s, out)
			}
		}
		if row := domainOnlyRow(t, cfg); row != wantRow {
			t.Errorf("%s: setting = %q, want %q", name, row, wantRow)
		}
	}

	step("status without domains", []string{"status"}, 0, "", "Dashboard access: open", "No panel domain with HTTPS is configured")
	step("close without domains", []string{"close"}, 1, "", "No panel domain with HTTPS is configured", "Nothing changed")
	step("open while open", []string{"open"}, 0, "", "already open")

	writeHTTPSPanelConf(t, dir, "a.example.com")
	step("status with a domain", []string{"status"}, 0, "", "Can be restricted to: a.example.com")
	step("close", []string{"close"}, 0, "a.example.com",
		"restricted to https://a.example.com", "within 2s", "also from this server", reopenCommand)
	step("status while closed", []string{"status"}, 0, "a.example.com", "only through https://a.example.com", reopenCommand)
	step("open", []string{"open"}, 0, "", "allowed again", "was restricted to https://a.example.com", "no restart needed")

	writeHTTPSPanelConf(t, dir, "b.example.com")
	step("close with several domains", []string{"close"}, 1, "", "Several panel domains", "a.example.com, b.example.com")
	step("close with a named domain", []string{"close", "B.example.com"}, 0, "b.example.com", "restricted to https://b.example.com")
	step("close with an unknown domain", []string{"close", "c.example.com"}, 1, "b.example.com", "c.example.com is not a panel domain")
	step("close with an invalid name", []string{"close", "../x"}, 1, "b.example.com", "is not a panel domain")
	step("open again", []string{"open"}, 0, "", "allowed again")

	t.Setenv("TRUSTED_PROXIES", "none")
	none, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg = none
	step("close without trusted proxies", []string{"close", "a.example.com"}, 1, "", "TRUSTED_PROXIES trusts no proxy")
}

func TestPanelAccessDispatch(t *testing.T) {
	stdout, stderr := os.Stdout, os.Stderr
	devnull, _ := os.Open(os.DevNull)
	os.Stdout, os.Stderr = devnull, devnull
	defer func() { os.Stdout, os.Stderr = stdout, stderr; devnull.Close() }()
	for _, args := range [][]string{{"panel-access"}, {"panel-access", "bogus"}, {"panel-access", "open", "x.example.com"}, {"panel-access", "status", "x"}, {"panel-access", "close", "a", "b"}} {
		if code := run(args); code != 2 {
			t.Errorf("%q = %d, want 2", args, code)
		}
	}
}

// TestPanelAccessReachesRunningServer: `panel-access open` and `close` run
// as another process against the same database apply to a running server
// within panelaccess.RefreshInterval, without a restart.
func TestPanelAccessReachesRunningServer(t *testing.T) {
	dir := t.TempDir()
	port := freePort(t)
	setEnv(t, dir, port)
	// The test client on loopback must count as direct: trust only an
	// address nothing here uses.
	t.Setenv("TRUSTED_PROXIES", "192.0.2.1")
	writeHTTPSPanelConf(t, dir, "panel.example.com")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if out, err := captureStdout(t, func() error {
		if err := migrate(context.Background(), quietLog()); err != nil {
			return err
		}
		return runPanelAccess(context.Background(), cfg, []string{"close"})
	}); err != nil {
		t.Fatalf("close before start: %v\n%s", err, out)
	}

	stdout := os.Stdout
	devnull, _ := os.Open(os.DevNull)
	os.Stdout = devnull
	defer func() { os.Stdout = stdout; devnull.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, quietLog()) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			t.Error("server did not shut down")
		}
	}()
	base := "http://127.0.0.1:" + strconv.Itoa(port)
	status := func(path string) int {
		resp, err := http.Get(base + path)
		if err != nil {
			return 0
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	deadline := time.Now().Add(20 * time.Second)
	for status("/api/health") != 200 {
		if time.Now().After(deadline) {
			t.Fatal("server did not start")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if code := status("/api/auth/status"); code != 403 {
		t.Fatalf("restricted at boot: /api/auth/status = %d, want 403", code)
	}

	// waitStatus measures how long the running server takes to follow the CLI.
	waitStatus := func(what string, want int) {
		t.Helper()
		start := time.Now()
		limit := panelaccess.RefreshInterval + time.Second
		for status("/api/auth/status") != want {
			if time.Since(start) > limit {
				t.Fatalf("%s: /api/auth/status not %d within %s", what, want, limit)
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Logf("%s applied after %s", what, time.Since(start).Round(10*time.Millisecond))
	}
	if out, err := captureStdout(t, func() error { return runPanelAccess(context.Background(), cfg, []string{"open"}) }); err != nil {
		t.Fatalf("open: %v\n%s", err, out)
	}
	waitStatus("panel-access open", 200)
	if out, err := captureStdout(t, func() error { return runPanelAccess(context.Background(), cfg, []string{"close"}) }); err != nil {
		t.Fatalf("close: %v\n%s", err, out)
	}
	waitStatus("panel-access close", 403)
	if code := status("/api/health"); code != 200 {
		t.Errorf("health while restricted = %d", code)
	}
}
