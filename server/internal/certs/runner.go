package certs

import (
	"bytes"
	"context"
	"errors"
	"time"

	"docklift/internal/execx"
)

// ExecResult is the outcome of a program that ran to completion.
type ExecResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// Runner runs an external program with an argument vector — never through a
// shell. Run returns an error only when the program could not be started or
// was stopped by ctx (or the runner's own timeout); a non-zero exit status is
// reported in ExecResult.ExitCode with a nil error.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (ExecResult, error)
}

// maxCapturedOutput caps each captured stream so a chatty program cannot grow
// the heap without bound; the tail beyond the cap is dropped.
const maxCapturedOutput = 1 << 20

// ExecRunner runs programs with execx.Run (process-tree kill on cancel).
// Timeout (when > 0) bounds each run on top of the caller's context.
type ExecRunner struct {
	Timeout time.Duration
}

// Run implements Runner.
func (r ExecRunner) Run(ctx context.Context, name string, args ...string) (ExecResult, error) {
	if r.Timeout > 0 {
		// A context deadline rather than execx's Timeout, so a timed-out run
		// reports context.DeadlineExceeded.
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.Timeout)
		defer cancel()
	}
	stdout := &cappedBuffer{limit: maxCapturedOutput}
	stderr := &cappedBuffer{limit: maxCapturedOutput}
	res, err := execx.Run(ctx, execx.Cmd{Name: name, Args: args, Stdout: stdout, Stderr: stderr})
	out := ExecResult{ExitCode: res.ExitCode, Stdout: stdout.String(), Stderr: stderr.String()}
	var exitErr *execx.ExitError
	if err == nil || errors.As(err, &exitErr) {
		return out, nil
	}
	out.ExitCode = -1
	return out, err
}

type cappedBuffer struct {
	buf   bytes.Buffer
	limit int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if room := b.limit - b.buf.Len(); room > 0 {
		if len(p) > room {
			b.buf.Write(p[:room])
		} else {
			b.buf.Write(p)
		}
	}
	return len(p), nil
}

func (b *cappedBuffer) String() string { return b.buf.String() }
