// Package app holds the dependencies shared by every route group (Deps) and
// the process lifecycle hooks that reproduce the startup order of main() in
// backend/src/index.ts. Router assembly lives in internal/app/router so that
// route packages can import this package without a cycle.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/moby/moby/client"

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

// Deps is everything a route group needs. Fields are set once at startup
// and are safe for concurrent use.
type Deps struct {
	Config      *config.Config
	Log         *slog.Logger
	DB          *db.DB
	Store       *store.Store
	Auth        *auth.Service
	Maintenance *maintenance.State
	// Docker is the Engine API client configured from DOCKER_HOST & co. It
	// does not connect until first used.
	Docker *client.Client
	// DockerX wraps the Engine client and the docker CLI (compose, buildx).
	DockerX *dockerx.Docker
	// Deploy owns deploy locks, runtime compose state, ports and image
	// retention. There is exactly one per process (one lock Tracker).
	Deploy *deploy.Runtime
	// Edge is the nginx/certificate/DNS stack. There is exactly one per
	// process so every route shares the same SSL event log.
	Edge *nginx.Stack
	// Git runs git with the hardened defaults of internal/gitops.
	Git *gitops.Runner
	// InstanceID is a random UUID generated once per process (/api/health).
	InstanceID string
	// Background is cancelled when the server shuts down; long-running work
	// started by handlers or hooks must derive its context from it.
	Background context.Context
	// Lifecycle collects startup and shutdown hooks.
	Lifecycle *Lifecycle
}

// Phase is a step of the startup sequence of index.ts main(), in order.
type Phase int

const (
	// PhaseDockerNetwork runs before the listener opens (ensureNetwork).
	// Failures are logged and startup continues without Docker.
	PhaseDockerNetwork Phase = iota
	// PhaseAfterListen runs in the background once the listener is open
	// (syncNginxConfigs). Failures are logged.
	PhaseAfterListen
	// PhaseRecovery runs after the bootstrap notice
	// (recoverDeploymentStateOnBoot). A failure aborts startup.
	PhaseRecovery
	// PhaseWatchers runs last (startCertRenewWatcher). A failure aborts startup.
	PhaseWatchers
)

func (p Phase) String() string {
	switch p {
	case PhaseDockerNetwork:
		return "docker-network"
	case PhaseAfterListen:
		return "after-listen"
	case PhaseRecovery:
		return "recovery"
	case PhaseWatchers:
		return "watchers"
	}
	return fmt.Sprintf("phase(%d)", int(p))
}

// Hook is a named startup step.
type Hook struct {
	Name string
	Run  func(ctx context.Context) error
}

// ShutdownHook is a named cleanup step run on SIGINT/SIGTERM before the
// database closes (cleanupAllSessions and friends).
type ShutdownHook struct {
	Name string
	Run  func(ctx context.Context)
}

// Lifecycle is the hook registry. Route groups register hooks from Mount.
type Lifecycle struct {
	mu       sync.Mutex
	hooks    map[Phase][]Hook
	shutdown []ShutdownHook
}

// NewLifecycle returns an empty registry.
func NewLifecycle() *Lifecycle {
	return &Lifecycle{hooks: map[Phase][]Hook{}}
}

// On registers a startup hook for a phase; hooks of one phase run in
// registration order.
func (l *Lifecycle) On(phase Phase, name string, run func(ctx context.Context) error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hooks[phase] = append(l.hooks[phase], Hook{Name: name, Run: run})
}

// OnShutdown registers a shutdown hook; they run in registration order.
func (l *Lifecycle) OnShutdown(name string, run func(ctx context.Context)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.shutdown = append(l.shutdown, ShutdownHook{Name: name, Run: run})
}

// Hooks returns the hooks of a phase.
func (l *Lifecycle) Hooks(phase Phase) []Hook {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]Hook(nil), l.hooks[phase]...)
}

// Run executes the hooks of a phase in order and returns their errors joined
// (every hook runs even when an earlier one fails).
func (l *Lifecycle) Run(ctx context.Context, phase Phase) error {
	var errs []error
	for _, h := range l.Hooks(phase) {
		if err := h.Run(ctx); err != nil {
			errs = append(errs, fmt.Errorf("%s hook %q: %w", phase, h.Name, err))
		}
	}
	return errors.Join(errs...)
}

// Shutdown runs the shutdown hooks in order.
func (l *Lifecycle) Shutdown(ctx context.Context) {
	l.mu.Lock()
	hooks := append([]ShutdownHook(nil), l.shutdown...)
	l.mu.Unlock()
	for _, h := range hooks {
		h.Run(ctx)
	}
}
