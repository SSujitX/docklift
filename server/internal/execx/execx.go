// Package execx runs external programs without a shell: one context-aware
// runner with timeouts, environment control, stdin, captured or streamed
// output, exit codes and process-tree termination on cancel.
//
// It replaces the child_process spawn/spawnSync calls of the TypeScript
// backend. Arguments are passed to the program as-is (exec.Command semantics),
// never through `sh -c`, so user input cannot inject shell syntax.
//
// Errors never include the argument list, because arguments may carry
// secrets (build args, tokens); they name only the program.
package execx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// DefaultKillGrace is how long a cancelled process tree gets between the
// polite stop signal and the hard kill (the TS code waited 2-3 s).
const DefaultKillGrace = 3 * time.Second

// ErrTimeout is returned (wrapped) when Cmd.Timeout elapsed before the
// program exited.
var ErrTimeout = errors.New("execx: timed out")

// Cmd describes one program run.
type Cmd struct {
	// Name is the program (looked up in PATH when it has no separator).
	Name string
	// Args are the program arguments, without the program name.
	Args []string
	// Dir is the working directory ("" = current directory).
	Dir string
	// Env replaces the environment when non-nil. Nil inherits os.Environ().
	Env []string
	// ExtraEnv entries ("KEY=VALUE") are appended to the inherited or given
	// environment; a later entry wins over an earlier one with the same key.
	ExtraEnv []string
	// Stdin feeds the program's standard input (nil = empty input).
	Stdin io.Reader
	// Stdout and Stderr receive the output as it is produced. When nil the
	// stream is captured into Result. Passing the same comparable writer for
	// both yields one interleaved stream with serialized writes.
	Stdout io.Writer
	Stderr io.Writer
	// Timeout kills the process tree after this long (0 = no timeout).
	Timeout time.Duration
	// KillGrace is the delay between the stop signal and the hard kill of
	// the process tree on cancel or timeout (0 = DefaultKillGrace).
	KillGrace time.Duration
	// OnStart is called with the process id right after the program started.
	OnStart func(pid int)
}

// Result is the outcome of a run.
type Result struct {
	// Stdout and Stderr hold captured output (only for nil Cmd writers).
	Stdout []byte
	Stderr []byte
	// ExitCode is the exit status, or -1 when the program did not start or
	// was killed by a signal.
	ExitCode int
	// Duration is the wall time of the run.
	Duration time.Duration
}

// ExitError reports a program that ran but exited with a non-zero status.
type ExitError struct {
	Name string
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("%s exited with code %d", e.Name, e.Code)
}

// StartError reports a program that could not be started (not found, not
// executable, bad working directory).
type StartError struct {
	Name string
	Err  error
}

func (e *StartError) Error() string { return fmt.Sprintf("start %s: %v", e.Name, e.Err) }

// Unwrap returns the underlying error (e.g. exec.ErrNotFound).
func (e *StartError) Unwrap() error { return e.Err }

// Run executes c and waits for it. It returns nil only for exit status 0.
// Otherwise the error is a *StartError, an *ExitError, ErrTimeout (wrapped),
// or the context's error when ctx was cancelled; Result is filled in every
// case. When ctx is cancelled or the timeout fires the whole process tree is
// stopped (process group on Unix, taskkill /T on Windows) and Run returns
// only after the program has exited, so no goroutine or child is left behind.
func Run(ctx context.Context, c Cmd) (Result, error) {
	start := time.Now()
	res := Result{ExitCode: -1}
	if c.Name == "" {
		return res, &StartError{Name: c.Name, Err: errors.New("empty program name")}
	}
	if err := ctx.Err(); err != nil {
		return res, err
	}
	runCtx := ctx
	var cancelTimeout context.CancelFunc = func() {}
	if c.Timeout > 0 {
		runCtx, cancelTimeout = context.WithTimeoutCause(ctx, c.Timeout, ErrTimeout)
	}
	defer cancelTimeout()

	grace := c.KillGrace
	if grace <= 0 {
		grace = DefaultKillGrace
	}
	cmd := exec.CommandContext(runCtx, c.Name, c.Args...)
	cmd.Dir = c.Dir
	if c.Env != nil || len(c.ExtraEnv) > 0 {
		env := c.Env
		if env == nil {
			env = os.Environ()
		}
		cmd.Env = append(append([]string(nil), env...), c.ExtraEnv...)
	}
	cmd.Stdin = c.Stdin
	var stdout, stderr bytes.Buffer
	cmd.Stdout = c.Stdout
	if cmd.Stdout == nil {
		cmd.Stdout = &stdout
	}
	cmd.Stderr = c.Stderr
	if cmd.Stderr == nil {
		cmd.Stderr = &stderr
	}
	tree := &processTree{grace: grace}
	prepare(cmd)
	cmd.Cancel = func() error { return tree.stop(cmd.Process) }
	// Bound the wait for output pipes held open by orphaned grandchildren.
	cmd.WaitDelay = grace + 2*time.Second

	if err := cmd.Start(); err != nil {
		res.Duration = time.Since(start)
		return res, &StartError{Name: c.Name, Err: err}
	}
	if c.OnStart != nil {
		c.OnStart(cmd.Process.Pid)
	}
	waitErr := cmd.Wait()
	tree.finish(cmd.Process)
	res.Duration = time.Since(start)
	res.Stdout, res.Stderr = stdout.Bytes(), stderr.Bytes()
	if cmd.ProcessState != nil {
		res.ExitCode = cmd.ProcessState.ExitCode()
	}

	if tree.stopped() {
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		if errors.Is(context.Cause(runCtx), ErrTimeout) {
			return res, fmt.Errorf("%s: %w after %s", c.Name, ErrTimeout, c.Timeout)
		}
	}
	if waitErr != nil {
		var ee *exec.ExitError
		if errors.As(waitErr, &ee) && res.ExitCode >= 0 {
			return res, &ExitError{Name: c.Name, Code: res.ExitCode}
		}
		if errors.Is(waitErr, exec.ErrWaitDelay) && res.ExitCode == 0 {
			return res, nil
		}
		return res, fmt.Errorf("wait %s: %w", c.Name, waitErr)
	}
	return res, nil
}

// processTree stops a started program together with its children.
type processTree struct {
	grace time.Duration

	mu        sync.Mutex
	cancelled bool
	hardKill  *time.Timer
}

// stop is exec.Cmd.Cancel: signal the tree and schedule the hard kill.
func (t *processTree) stop(p *os.Process) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cancelled || p == nil {
		return nil
	}
	t.cancelled = true
	terminateTree(p)
	t.hardKill = time.AfterFunc(t.grace, func() { killTree(p) })
	return nil
}

// finish runs after Wait: it cancels the pending hard-kill timer and, for a
// cancelled run, kills whatever is left of the tree right away, so nothing
// outlives Run.
func (t *processTree) finish(p *os.Process) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.hardKill != nil {
		t.hardKill.Stop()
		t.hardKill = nil
	}
	if t.cancelled && p != nil {
		killTree(p)
	}
}

func (t *processTree) stopped() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cancelled
}

// Output runs c with captured stdout/stderr and returns them as strings. It
// is a convenience for short commands (docker ps -q, volume rm ...).
func Output(ctx context.Context, c Cmd) (stdout, stderr string, exitCode int, err error) {
	c.Stdout, c.Stderr = nil, nil
	res, err := Run(ctx, c)
	return string(res.Stdout), string(res.Stderr), res.ExitCode, err
}

// TailBuffer is an io.Writer that keeps only the last Max bytes written. It
// is safe for concurrent use.
type TailBuffer struct {
	Max int

	mu  sync.Mutex
	buf []byte
}

// Write appends p, dropping the oldest bytes beyond Max.
func (b *TailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if b.Max > 0 && len(b.buf) > b.Max {
		b.buf = append(b.buf[:0], b.buf[len(b.buf)-b.Max:]...)
	}
	return len(p), nil
}

// String returns the retained bytes.
func (b *TailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

// WriterFunc adapts a function to io.Writer; it never fails.
type WriterFunc func(p []byte)

// Write calls f(p).
func (f WriterFunc) Write(p []byte) (int, error) {
	f(p)
	return len(p), nil
}
