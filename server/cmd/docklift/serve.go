package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/moby/moby/client"

	"docklift/internal/app"
	"docklift/internal/app/router"
	"docklift/internal/auth"
	"docklift/internal/config"
	"docklift/internal/db"
	"docklift/internal/deploy"
	"docklift/internal/dockerx"
	"docklift/internal/gitops"
	"docklift/internal/maintenance"
	"docklift/internal/nginx"
	"docklift/internal/store"
)

// shutdownTimeout bounds how long in-flight requests may finish on shutdown.
const shutdownTimeout = 10 * time.Second

// loadConfig applies .env/.env.local from the working directory (the role the
// backend root had) and reads the environment.
func loadConfig(log *slog.Logger) (*config.Config, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("working directory: %w", err)
	}
	files, err := config.LoadDotenv(wd)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		log.Info("loaded environment file", "path", f)
	}
	return config.Load()
}

// openDatabase opens the database and runs the ensureDb sequence.
func openDatabase(ctx context.Context, cfg *config.Config, log *slog.Logger) (*db.DB, error) {
	database, err := db.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return nil, err
	}
	if err := database.Ensure(ctx, log); err != nil {
		return nil, errors.Join(err, database.Close())
	}
	return database, nil
}

func migrate(ctx context.Context, log *slog.Logger) error {
	cfg, err := loadConfig(log)
	if err != nil {
		return err
	}
	database, err := openDatabase(ctx, cfg, log)
	if err != nil {
		return err
	}
	return database.Close()
}

// serve runs the database migrations, secrets and directories, then the
// startup hooks in order, then graceful shutdown on SIGINT/SIGTERM.
func serve(ctx context.Context, log *slog.Logger) (err error) {
	cfg, err := loadConfig(log)
	if err != nil {
		return err
	}
	database, err := openDatabase(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, database.Close()) }()

	secrets, err := auth.LoadOrCreateSecrets(cfg.DataPath, cfg.JWTSecret, cfg.InternalAPISecret, log)
	if err != nil {
		return err
	}
	for _, dir := range []string{cfg.DataPath, cfg.DeploymentsPath, cfg.BackupPath} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}

	docker, err := client.New(client.FromEnv)
	if err != nil {
		return fmt.Errorf("configure docker client: %w", err)
	}
	defer func() { err = errors.Join(err, docker.Close()) }()

	background, cancelBackground := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelBackground()

	st := store.New(database)
	dockerX := dockerx.New(docker, cfg.DockerNetwork, cfg.NginxProxyContainer, log)
	d := &app.Deps{
		Config:      cfg,
		Log:         log,
		DB:          database,
		Store:       st,
		Auth:        auth.New(st, secrets, cfg.DataPath, log),
		Maintenance: maintenance.New(cfg.DataPath, log),
		Docker:      docker,
		DockerX:     dockerX,
		Deploy:      deploy.NewRuntime(cfg, st, dockerX, log),
		Edge:        nginx.NewStack(cfg, st, log),
		Git:         &gitops.Runner{},
		InstanceID:  db.NewID(),
		Background:  background,
		Lifecycle:   app.NewLifecycle(),
	}
	handler := newHandler(d)

	// Re-seal critical restore state from disk before accepting traffic.
	d.Maintenance.LoadCriticalOnBoot()

	// Docker is optional for the API itself (local smoke runs have none).
	if err := d.Lifecycle.Run(ctx, app.PhaseDockerNetwork); err != nil {
		log.Warn("⚠️  Docker unavailable — API starting without Docker", "error", err)
	}

	listener, err := net.Listen("tcp", ":"+strconv.Itoa(cfg.Port))
	if err != nil {
		return fmt.Errorf("listen on port %d: %w", cfg.Port, err)
	}
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: time.Minute,
		IdleTimeout:       2 * time.Minute,
		BaseContext:       func(net.Listener) context.Context { return background },
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(listener) }()
	log.Info("🚀 Docklift backend (Go) listening", "addr", listener.Addr().String(), "deployments", cfg.DeploymentsPath)

	go func() {
		if err := d.Lifecycle.Run(background, app.PhaseAfterListen); err != nil {
			log.Error("startup background task failed", "error", err)
		}
	}()

	// Fresh install: print the bootstrap secret to the logs (never via a public API).
	if err := d.Auth.Bootstrap().LogIfNeeded(ctx, st); err != nil {
		return shutdown(srv, d, cancelBackground, fmt.Errorf("bootstrap secret: %w", err))
	}
	if err := d.Lifecycle.Run(ctx, app.PhaseRecovery); err != nil {
		return shutdown(srv, d, cancelBackground, err)
	}
	if removed, err := st.DedupeEnvVariables(ctx); err != nil {
		log.Warn("[startup] Env dedupe skipped", "error", err)
	} else if removed > 0 {
		log.Info(fmt.Sprintf("[startup] Removed %d duplicate env_variables row(s)", removed))
	}
	if err := d.Lifecycle.Run(ctx, app.PhaseWatchers); err != nil {
		return shutdown(srv, d, cancelBackground, err)
	}

	select {
	case err := <-serveErr:
		return shutdown(srv, d, cancelBackground, fmt.Errorf("http server: %w", err))
	case <-ctx.Done():
		log.Info("🛑 Signal received — shutting down gracefully...")
		return shutdown(srv, d, cancelBackground, nil)
	}
}

// newHandler registers the service-layer startup hooks, then builds the
// router, whose route groups add their own startup and shutdown hooks.
func newHandler(d *app.Deps) http.Handler {
	registerStartupHooks(d)
	return router.New(d)
}

// registerStartupHooks wires the service-layer steps of the startup sequence:
// ensureNetwork, syncNginxConfigs, recoverDeploymentStateOnBoot and
// startCertRenewWatcher. Route groups add their own hooks from Mount.
func registerStartupHooks(d *app.Deps) {
	d.Lifecycle.On(app.PhaseDockerNetwork, "docker-network", d.DockerX.EnsureNetwork)
	// Before nginx-sync reloads the vhosts: a recreated proxy has lost its
	// project networks (not a 2.x step; see ReattachProxyOnBoot).
	d.Lifecycle.On(app.PhaseAfterListen, "proxy-reattach", d.Deploy.ReattachProxyOnBoot)
	d.Lifecycle.On(app.PhaseAfterListen, "nginx-sync", d.Edge.Boot)
	d.Lifecycle.On(app.PhaseRecovery, "deployment-recovery", d.Deploy.RecoverOnBoot)
	d.Lifecycle.On(app.PhaseWatchers, "cert-renew-watcher", func(context.Context) error {
		d.Edge.StartWatchers(d.Background)
		return nil
	})
}

// shutdown stops background work (which also ends streaming requests, whose
// contexts derive from it), drains the HTTP server, then runs the shutdown
// hooks. cause is returned joined with any shutdown error.
func shutdown(srv *http.Server, d *app.Deps, cancelBackground context.CancelFunc, cause error) error {
	cancelBackground()
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	err := srv.Shutdown(ctx)
	if errors.Is(err, context.DeadlineExceeded) {
		err = errors.Join(err, srv.Close())
	}
	d.Log.Info("   HTTP server closed")
	d.Lifecycle.Shutdown(ctx)
	d.Log.Info("   Shutdown hooks completed")
	if err != nil {
		return errors.Join(cause, fmt.Errorf("shutdown: %w", err))
	}
	return cause
}
