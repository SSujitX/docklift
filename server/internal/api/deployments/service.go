package deployments

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"

	"docklift/internal/app"
	"docklift/internal/certs"
	"docklift/internal/deploy"
	"docklift/internal/dnscheck"
	"docklift/internal/dockerx"
	"docklift/internal/execx"
	"docklift/internal/gitops"
	"docklift/internal/nginx"
	"docklift/internal/store"
)

// Docker is the part of *dockerx.Docker the deploy routes run directly;
// network attach, links and status sync go through deploy.Runtime.
type Docker interface {
	Ping(ctx context.Context) error
	Compose(ctx context.Context, c dockerx.ComposeCmd) (execx.Result, error)
	ComposeDown(ctx context.Context, c dockerx.ComposeCmd) dockerx.DownResult
	PullImage(ctx context.Context, image string, o dockerx.PullOptions) error
	RemoveContainer(ctx context.Context, name string)
	ImageExists(ctx context.Context, ref string) bool
	ContainerLogs(ctx context.Context, name string, tail int) string
	ContainerStats(ctx context.Context, name string) *dockerx.ContainerStats
}

var _ Docker = (*dockerx.Docker)(nil)

// Git is the part of *gitops.Runner the pipeline and rollback use.
type Git interface {
	Pull(ctx context.Context, dir string, out io.Writer, opts gitops.PullOptions) error
	LastCommitMessage(ctx context.Context, dir string) (string, bool)
	CommitSHA(ctx context.Context, dir string) (string, bool)
	ResetToCommit(ctx context.Context, dir, sha string) error
	ScrubOrigin(ctx context.Context, dir, cleanURL string) error
	VerifyNoCredentials(ctx context.Context, dir string) error
	SetOriginURL(ctx context.Context, dir, u string) error
}

var _ Git = (*gitops.Runner)(nil)

// Edge is the part of *nginx.Service the routes use.
type Edge interface {
	UpdateServiceDomain(ctx context.Context, svc store.Service, opts nginx.UpdateOptions) error
	CleanupServiceDomain(ctx context.Context, serviceID string)
	AssertHostnamesAvailable(ctx context.Context, hosts []string, excludeServiceID string) error
}

var _ Edge = (*nginx.Service)(nil)

// DNS is the part of *dnscheck.Checker the DNS preflight uses.
type DNS interface {
	CheckDomains(ctx context.Context, hostnames []string) []dnscheck.Check
	GetServerPublicIP(ctx context.Context) *string
}

var _ DNS = (*dnscheck.Checker)(nil)

// GitHubApp mints installation tokens for private repositories
// (getInstallationIdForRepo + getInstallationToken of routes/github.ts);
// *githubapp.App implements it.
type GitHubApp interface {
	InstallationIDForRepo(ctx context.Context, owner, repo string) (string, error)
	InstallationToken(ctx context.Context, installationID string) (string, error)
}

// Service owns "deploy a project": the deploy pipeline and the stop,
// restart, cancel and rollback operations, plus the per-project auto-deploy
// queue. There is one per process (For); every route group that deploys
// uses it. Operations run on the process background context, so a client
// that disconnects never aborts them.
type Service struct {
	store  *store.Store
	rt     *deploy.Runtime
	docker Docker
	git    Git
	edge   Edge
	certs  *certs.Service
	dns    DNS
	log    *slog.Logger
	// bg is cancelled on shutdown: it kills running commands.
	bg context.Context
	// buildRun runs docker build / railpack (nil = execx.Run).
	buildRun deploy.CommandRunner
	github   atomic.Pointer[githubHolder]

	// startMu serializes the admission checks of operations on one project
	// (lock + in_progress row checks, row creation, lock acquisition).
	startMu keyedMutex
	// running tracks pipelines so shutdown can wait for their final writes.
	running sync.WaitGroup
	changes broadcast
	queue   *Queue
}

type githubHolder struct{ app GitHubApp }

var (
	registryMu sync.Mutex
	registry   = map[*app.Deps]*Service{}
)

// For returns the process-wide Service of d, creating it on first use (and
// registering a shutdown hook that waits for running operations). Every
// route group must use this instance so that the auto-deploy queue is
// shared.
func For(d *app.Deps) *Service {
	registryMu.Lock()
	defer registryMu.Unlock()
	if s, ok := registry[d]; ok {
		return s
	}
	s := NewService(d)
	registry[d] = s
	d.Lifecycle.OnShutdown("deployments", s.Wait)
	return s
}

// NewService builds a Service on d's shared dependencies. Use For in
// production code; NewService is for tests that need an isolated instance.
func NewService(d *app.Deps) *Service {
	s := &Service{
		store:  d.Store,
		rt:     d.Deploy,
		docker: d.DockerX,
		git:    d.Git,
		edge:   d.Edge.Nginx,
		certs:  d.Edge.Certs,
		dns:    d.Edge.DNS,
		log:    d.Log,
		bg:     d.Background,
	}
	s.queue = newQueue(s.bg, s.log, s.StartDeploy, s.changes.wait)
	return s
}

// SetGitHubApp wires the GitHub App used to refresh installation tokens
// before pulling GitHub projects. Without one, the pull of a GitHub project
// logs a token warning and runs unauthenticated, as the TS code did when the
// App was not configured.
func (s *Service) SetGitHubApp(a GitHubApp) {
	s.github.Store(&githubHolder{app: a})
}

func (s *Service) githubApp() GitHubApp {
	if h := s.github.Load(); h != nil {
		return h.app
	}
	return nil
}

// Wait blocks until every running operation finished or ctx is done (the
// shutdown hook; Background is cancelled first, which kills their commands).
func (s *Service) Wait(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		s.running.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// dbCtx is the context of database writes: not cancelled on shutdown, so a
// deployment row always reaches its terminal status.
func (s *Service) dbCtx() context.Context { return context.WithoutCancel(s.bg) }

// RequestError is an operation refused before it started: the HTTP status
// and the `{"error": Message}` body the route answers.
type RequestError struct {
	Status  int
	Message string
}

func (e *RequestError) Error() string { return e.Message }

func reqErr(status int, msg string) *RequestError { return &RequestError{Status: status, Message: msg} }

// Errors of StartDeploy that callers may test for.
var (
	ErrProjectNotFound = reqErr(http.StatusNotFound, "Project not found")
	ErrFilesNotFound   = reqErr(http.StatusBadRequest, "Project files not found")
	ErrCancelling      = reqErr(http.StatusConflict, "A cancel is in progress for this project")
	ErrBusy            = reqErr(http.StatusConflict, "A deployment is already running for this project")
)

// Run is an operation running in the background.
type Run struct {
	// DeploymentID is the deployment row of the operation ("" for cancel).
	DeploymentID string
	done         chan struct{}
	status       atomic.Value
}

func newRun(id string) *Run { return &Run{DeploymentID: id, done: make(chan struct{})} }

// Done is closed when the operation finished.
func (r *Run) Done() <-chan struct{} { return r.done }

// Status is the final deployment status ("success", "failed",
// "cancelled"); "" until Done, and for operations without a row.
func (r *Run) Status() string {
	v, _ := r.status.Load().(string)
	return v
}

// finish records the final status of the row and closes Done.
func (s *Service) finish(run *Run) {
	if run.DeploymentID != "" {
		if d, err := s.store.GetDeployment(s.dbCtx(), run.DeploymentID); err == nil && d.Status != nil {
			run.status.Store(*d.Status)
		}
	}
	close(run.done)
	s.changes.signal()
}

// broadcast wakes every waiter when something changed (an operation ended).
type broadcast struct {
	mu sync.Mutex
	ch chan struct{}
}

// wait returns a channel closed at the next signal.
func (b *broadcast) wait() <-chan struct{} {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.ch == nil {
		b.ch = make(chan struct{})
	}
	return b.ch
}

func (b *broadcast) signal() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.ch != nil {
		close(b.ch)
		b.ch = nil
	}
}

// keyedMutex is a mutex per key.
type keyedMutex struct {
	mu    sync.Mutex
	locks map[string]*keyedEntry
}

type keyedEntry struct {
	mu   sync.Mutex
	refs int
}

func (k *keyedMutex) lock(key string) func() {
	k.mu.Lock()
	if k.locks == nil {
		k.locks = map[string]*keyedEntry{}
	}
	e := k.locks[key]
	if e == nil {
		e = &keyedEntry{}
		k.locks[key] = e
	}
	e.refs++
	k.mu.Unlock()
	e.mu.Lock()
	return func() {
		e.mu.Unlock()
		k.mu.Lock()
		e.refs--
		if e.refs == 0 {
			delete(k.locks, key)
		}
		k.mu.Unlock()
	}
}
