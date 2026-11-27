package certs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"
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

// ExecRunner runs programs with os/exec. Timeout (when > 0) bounds each run
// on top of the caller's context.
type ExecRunner struct {
	Timeout time.Duration
}

// Run implements Runner.
func (r ExecRunner) Run(ctx context.Context, name string, args ...string) (ExecResult, error) {
	if r.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.Timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	stdout := &cappedBuffer{limit: maxCapturedOutput}
	stderr := &cappedBuffer{limit: maxCapturedOutput}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = 5 * time.Second
	err := cmd.Run()
	res := ExecResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if err == nil {
		return res, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		res.ExitCode = -1
		return res, fmt.Errorf("%s: %w", name, ctxErr)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() >= 0 {
		res.ExitCode = exitErr.ExitCode()
		return res, nil
	}
	res.ExitCode = -1
	return res, err
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
