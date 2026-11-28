package main

import (
	"context"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"

	"docklift/internal/auth"
	"docklift/internal/config"
	"docklift/internal/db"
	"docklift/internal/store"
)

func TestAPITokenCommand(t *testing.T) {
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
	st := store.New(d)

	run := func(args ...string) (string, error) {
		t.Helper()
		return captureStdout(t, func() error { return runAPIToken(ctx, cfg, args) })
	}
	exitOf := func(err error) int {
		var e exitCode
		if errors.As(err, &e) {
			return int(e)
		}
		if err != nil {
			return -1
		}
		return 0
	}

	if out, err := run("create", "ci"); exitOf(err) != 1 || !strings.Contains(out, "No admin user found") {
		t.Fatalf("create without admin: %v %s", err, out)
	}
	if _, err := st.CreateUser(ctx, store.UserCreate{Name: "A", Email: "a@example.com", Password: "x"}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]string{{"create"}, {"create", "x", "--read", "--allow-dangerous"}, {"create", "x", "--expires", "0.5d"}, {"create", "x", "--bogus"}, {"revoke"}, {"list", "x"}} {
		if _, err := run(bad...); exitOf(err) != 2 {
			t.Errorf("%q: %v, want exit 2", bad, err)
		}
	}

	out, err := run("create", "deploy", "bot", "--allow-dangerous", "--expires", "90d")
	if err != nil {
		t.Fatalf("create: %v %s", err, out)
	}
	plain := regexp.MustCompile(`dl_[A-Za-z0-9_-]{43}`).FindString(out)
	if !auth.ValidAPITokenFormat(plain) || !strings.Contains(out, "only time the token is shown") {
		t.Fatalf("create output: %s", out)
	}
	row, err := st.GetAPITokenByHash(ctx, auth.HashAPIToken(plain))
	if err != nil || row.Name != "deploy bot" || row.Scope != "full" || !row.AllowDangerous || row.ExpiresAt == nil {
		t.Fatalf("row %+v %v", row, err)
	}
	if out, err := run("create", "reader", "--read"); err != nil || !strings.Contains(out, "Scope:   read") {
		t.Fatalf("create read: %v %s", err, out)
	}

	out, err = run("list")
	if err != nil || !strings.Contains(out, "deploy bot") || !strings.Contains(out, row.Prefix) || strings.Contains(out, plain) || !strings.Contains(out, "active") {
		t.Fatalf("list: %v %s", err, out)
	}

	if out, err := run("revoke", "dl_"); exitOf(err) != 1 {
		t.Fatalf("ambiguous/short prefix: %v %s", err, out)
	}
	if out, err := run("revoke", row.Prefix); err != nil || !strings.Contains(out, "revoked") {
		t.Fatalf("revoke by prefix: %v %s", err, out)
	}
	if _, err := st.GetAPIToken(ctx, row.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("token still there: %v", err)
	}
	if out, err := run("revoke", row.ID); exitOf(err) != 1 || !strings.Contains(out, "No API token") {
		t.Fatalf("revoke unknown: %v %s", err, out)
	}
	_ = d.Close()
}

func TestAPITokenDispatch(t *testing.T) {
	stdout, stderr := os.Stdout, os.Stderr
	devnull, _ := os.Open(os.DevNull)
	os.Stdout, os.Stderr = devnull, devnull
	defer func() { os.Stdout, os.Stderr = stdout, stderr; devnull.Close() }()
	for _, args := range [][]string{{"api-token"}, {"api-token", "bogus"}} {
		if code := run(args); code != 2 {
			t.Errorf("%q = %d, want 2", args, code)
		}
	}
}
