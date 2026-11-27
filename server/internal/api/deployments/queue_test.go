package deployments

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"docklift/internal/store"
)

// fakeStarter records starts; each run stays open until the test finishes it.
type fakeStarter struct {
	mu      sync.Mutex
	started []DeployOptions
	runs    []*Run
	busyFor int
	refuse  error
	changes broadcast
	running int
	maxRun  int
}

func (f *fakeStarter) start(_ context.Context, _ string, o DeployOptions) (*Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.refuse != nil {
		return nil, f.refuse
	}
	if f.busyFor > 0 || f.running > 0 {
		f.busyFor--
		return nil, ErrBusy
	}
	f.running++
	f.maxRun = max(f.maxRun, f.running)
	run := newRun("d" + o.CommitMessage)
	f.started = append(f.started, o)
	f.runs = append(f.runs, run)
	return run, nil
}

func (f *fakeStarter) finishLast() {
	f.mu.Lock()
	run := f.runs[len(f.runs)-1]
	f.running--
	f.mu.Unlock()
	close(run.done)
	f.changes.signal()
}

func (f *fakeStarter) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.started)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func newTestQueue(t *testing.T, f *fakeStarter) (*Queue, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	q := newQueue(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), f.start, f.changes.wait)
	q.retry = 10 * time.Millisecond
	t.Cleanup(func() { cancel(); q.wait() })
	return q, cancel
}

func TestQueueOneRunningOnePendingLatestWins(t *testing.T) {
	f := &fakeStarter{}
	q, _ := newTestQueue(t, f)
	if got := q.Enqueue("p", DeployOptions{CommitMessage: "1"}); got != Queued {
		t.Fatalf("first enqueue = %v", got)
	}
	waitFor(t, "first start", func() bool { return f.count() == 1 })
	for _, m := range []string{"2", "3", "4"} {
		if got := q.Enqueue("p", DeployOptions{CommitMessage: m}); got != Pending {
			t.Fatalf("enqueue %s = %v", m, got)
		}
	}
	f.finishLast()
	waitFor(t, "second start", func() bool { return f.count() == 2 })
	f.finishLast()
	q.wait()
	if f.count() != 2 || f.started[1].CommitMessage != "4" {
		t.Fatalf("starts = %+v (want 1 then latest 4)", f.started)
	}
	if f.maxRun != 1 {
		t.Fatalf("ran %d at once", f.maxRun)
	}
}

func TestQueueWaitsForBusyProject(t *testing.T) {
	f := &fakeStarter{busyFor: 5}
	q, _ := newTestQueue(t, f)
	q.Enqueue("p", DeployOptions{CommitMessage: "a"})
	waitFor(t, "start after busy", func() bool { return f.count() == 1 })
	q.Enqueue("p", DeployOptions{CommitMessage: "b"})
	f.finishLast()
	waitFor(t, "pending start", func() bool { return f.count() == 2 })
	f.finishLast()
	q.wait()
	if f.started[0].CommitMessage != "a" || f.started[1].CommitMessage != "b" {
		t.Fatalf("starts = %+v", f.started)
	}
}

func TestQueueRefusedRequestRetiresWorker(t *testing.T) {
	f := &fakeStarter{refuse: ErrProjectNotFound}
	q, _ := newTestQueue(t, f)
	q.Enqueue("p", DeployOptions{})
	q.wait()
	f.mu.Lock()
	f.refuse = nil
	f.mu.Unlock()
	if got := q.Enqueue("p", DeployOptions{CommitMessage: "x"}); got != Queued {
		t.Fatalf("enqueue after refusal = %v", got)
	}
	waitFor(t, "start", func() bool { return f.count() == 1 })
	f.finishLast()
	q.wait()
}

func TestQueueStopsOnShutdown(t *testing.T) {
	f := &fakeStarter{busyFor: 1 << 30}
	q, cancel := newTestQueue(t, f)
	q.Enqueue("p", DeployOptions{})
	cancel()
	q.wait()
	if f.count() != 0 {
		t.Fatal("nothing must start after shutdown")
	}
}

func TestEnqueueDeployRunsRealPipeline(t *testing.T) {
	e := setup(t)
	e.docker.buildGate = newGate()
	p := e.project(t, "auto", store.ProjectCreate{}, dockerfileApp)
	if err := e.s.EnqueueDeploy(context.Background(), p.ID, "webhook", "first"); err != nil {
		t.Fatal(err)
	}
	waitGate(t, e.docker.buildGate)
	_ = e.s.EnqueueDeploy(context.Background(), p.ID, "webhook", "second")
	_ = e.s.EnqueueDeploy(context.Background(), p.ID, "webhook", "third")
	close(e.docker.buildGate.release)
	e.s.queue.wait()
	rows := e.rows(t, p.ID)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (one running + latest pending)", len(rows))
	}
	for _, r := range rows {
		if strVal(r.Status) != "success" || strVal(r.Trigger) != "webhook" {
			t.Fatalf("row %s/%s", strVal(r.Status), strVal(r.Trigger))
		}
	}
	if strVal(rows[0].CommitMessage) != "third" || strVal(rows[1].CommitMessage) != "first" {
		t.Fatalf("commit messages = %s, %s", strVal(rows[0].CommitMessage), strVal(rows[1].CommitMessage))
	}
}
