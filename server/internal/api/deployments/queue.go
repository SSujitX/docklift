package deployments

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// queueRetryInterval is how often a queued deploy re-checks a busy project
// when no operation-end signal arrived (e.g. a stop or restart row).
const queueRetryInterval = 2 * time.Second

// startFunc starts one deploy (Service.StartDeploy).
type startFunc func(ctx context.Context, projectID string, opts DeployOptions) (*Run, error)

// Queue runs auto-deploys one at a time per project: at most one running
// and one pending, the latest request replacing an older pending one. A
// request for a busy project (a manual deploy, rollback, cancel, stop or
// restart in progress) waits until the project is free; it is never
// dropped while the server runs, unless the deploy is refused for good
// (project deleted, files missing).
type Queue struct {
	ctx     context.Context
	log     *slog.Logger
	start   startFunc
	changed func() <-chan struct{}
	retry   time.Duration

	mu       sync.Mutex
	projects map[string]*queueEntry
	workers  sync.WaitGroup
}

type queueEntry struct {
	pending *DeployOptions
}

// EnqueueResult says what Enqueue did with a request.
type EnqueueResult int

const (
	// Queued means a worker was started for the project.
	Queued EnqueueResult = iota
	// Pending means a deploy is already queued or running; the request is
	// the pending one now (replacing an older pending request).
	Pending
)

func newQueue(ctx context.Context, log *slog.Logger, start startFunc, changed func() <-chan struct{}) *Queue {
	return &Queue{ctx: ctx, log: log, start: start, changed: changed, retry: queueRetryInterval, projects: map[string]*queueEntry{}}
}

// Enqueue records a deploy request and returns at once.
func (q *Queue) Enqueue(projectID string, opts DeployOptions) EnqueueResult {
	q.mu.Lock()
	defer q.mu.Unlock()
	o := opts
	if e, ok := q.projects[projectID]; ok {
		e.pending = &o
		return Pending
	}
	q.projects[projectID] = &queueEntry{pending: &o}
	q.workers.Add(1)
	go q.work(projectID)
	return Queued
}

// take removes the pending request, or retires the worker when there is
// none (under the lock, so a concurrent Enqueue starts a new worker).
func (q *Queue) take(projectID string) (*DeployOptions, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	e := q.projects[projectID]
	if e.pending == nil {
		delete(q.projects, projectID)
		return nil, false
	}
	o := e.pending
	e.pending = nil
	return o, true
}

// putBack restores a request that could not start yet, unless a newer one
// arrived meanwhile (latest wins).
func (q *Queue) putBack(projectID string, o *DeployOptions) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if e := q.projects[projectID]; e.pending == nil {
		e.pending = o
	}
}

func (q *Queue) work(projectID string) {
	defer q.workers.Done()
	for {
		opts, ok := q.take(projectID)
		if !ok {
			return
		}
		if q.ctx.Err() != nil {
			q.drop(projectID)
			return
		}
		changed := q.changed()
		run, err := q.start(q.ctx, projectID, *opts)
		var re *RequestError
		switch {
		case err == nil:
			select {
			case <-run.Done():
				q.log.Info("[Auto-Deploy] finished", "project", projectID, "deployment", run.DeploymentID, "status", run.Status())
			case <-q.ctx.Done():
			}
		case errors.Is(err, ErrBusy) || errors.Is(err, ErrCancelling):
			q.putBack(projectID, opts)
			timer := time.NewTimer(q.retry)
			select {
			case <-changed:
			case <-timer.C:
			case <-q.ctx.Done():
			}
			timer.Stop()
		case errors.As(err, &re):
			q.log.Warn("[Auto-Deploy] deploy refused", "project", projectID, "error", re.Message)
		default:
			q.log.Error("[Auto-Deploy] failed to start deploy", "project", projectID, "error", err)
		}
	}
}

// drop forgets the project's requests (shutdown).
func (q *Queue) drop(projectID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.projects, projectID)
}

// wait blocks until every worker exited (tests; workers exit once idle or
// when the background context ends).
func (q *Queue) wait() { q.workers.Wait() }

// EnqueueDeploy queues an auto-deploy of the project (the github webhook's
// Deployer): it returns immediately; the deploy runs after any operation in
// progress, and of several requests that arrive meanwhile only the latest
// runs. ctx is not used beyond the call.
func (s *Service) EnqueueDeploy(_ context.Context, projectID, trigger, commitMessage string) error {
	s.queue.Enqueue(projectID, DeployOptions{Trigger: trigger, CommitMessage: commitMessage})
	return nil
}

// Queue returns the auto-deploy queue.
func (s *Service) Queue() *Queue { return s.queue }
