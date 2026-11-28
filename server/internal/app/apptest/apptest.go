// Package apptest builds real dependencies on a temporary data directory for
// route-group tests: a migrated SQLite database, the auth service and the
// maintenance state. There is no Engine client (Deps.Docker is nil); DockerX,
// Deploy, Edge and Git are real but talk to no daemon unless a test swaps
// their runners.
package apptest

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"docklift/internal/app"
	"docklift/internal/auth"
	"docklift/internal/config"
	"docklift/internal/db"
	"docklift/internal/deploy"
	"docklift/internal/dockerx"
	"docklift/internal/gitops"
	"docklift/internal/maintenance"
	"docklift/internal/nginx"
	"docklift/internal/panelaccess"
	"docklift/internal/store"
)

// NewDeps returns Deps rooted in t.TempDir(). Background is cancelled and the
// database closed when the test ends.
func NewDeps(t testing.TB) *app.Deps {
	t.Helper()
	dir := t.TempDir()
	cfg, err := config.FromEnv(func(key string) string {
		switch key {
		case "DATA_PATH":
			return filepath.Join(dir, "data")
		case "DEPLOYMENTS_PATH":
			return filepath.Join(dir, "deployments")
		case "NGINX_CONF_PATH":
			return filepath.Join(dir, "nginx")
		case "LETSENCRYPT_PATH":
			return filepath.Join(dir, "letsencrypt")
		case "DATABASE_URL":
			return "file:" + filepath.ToSlash(filepath.Join(dir, "data", "docklift.db"))
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithCancel(context.Background())
	database, err := db.Open(ctx, cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = database.Close()
	})
	if _, err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	st := store.New(database)
	secrets := auth.Secrets{JWTSecret: "apptest-jwt-secret", InternalAPISecret: "apptest-internal-secret"}
	dockerX := dockerx.New(nil, cfg.DockerNetwork, cfg.NginxProxyContainer, log)
	edge := nginx.NewStack(cfg, st, log)
	return &app.Deps{
		DockerX:     dockerX,
		Deploy:      deploy.NewRuntime(cfg, st, dockerX, log),
		Edge:        edge,
		PanelAccess: panelaccess.New(st, edge.Nginx, log),
		Git:         &gitops.Runner{},
		Config:      cfg,
		Log:         log,
		DB:          database,
		Store:       st,
		Auth:        auth.New(st, secrets, cfg.DataPath, log),
		Maintenance: maintenance.New(cfg.DataPath, log),
		InstanceID:  db.NewID(),
		Background:  ctx,
		Lifecycle:   app.NewLifecycle(),
	}
}

// CreateAdmin inserts an admin user with a bcrypt hash of password and
// returns it with a valid session token.
func CreateAdmin(t testing.TB, d *app.Deps, email, password string) (*store.User, string) {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	now := db.Now()
	u, err := d.Store.CreateUser(context.Background(), store.UserCreate{Name: "Admin", Email: email, Password: hash, PasswordChangedAt: &now})
	if err != nil {
		t.Fatal(err)
	}
	token, err := d.Auth.IssueSession(u)
	if err != nil {
		t.Fatal(err)
	}
	return u, token
}
