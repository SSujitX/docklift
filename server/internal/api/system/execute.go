package system

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"time"

	"docklift/internal/execx"
)

// Limits of child_process.exec in POST /execute.
const (
	executeShell     = "/bin/bash"
	executeTimeout   = 30 * time.Second
	executeMaxBuffer = 1024 * 1024
)

// cappedBuffer is one output stream limited to executeMaxBuffer bytes; the
// first overflow cancels the command (like exec's maxBuffer).
type cappedBuffer struct {
	mu       sync.Mutex
	buf      bytes.Buffer
	name     string
	overflow *overflowState
}

type overflowState struct {
	once   sync.Once
	stream string
	cancel context.CancelFunc
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	room := executeMaxBuffer - b.buf.Len()
	if len(p) > room {
		if room > 0 {
			b.buf.Write(p[:room])
		}
		b.overflow.once.Do(func() {
			b.overflow.stream = b.name
			b.overflow.cancel()
		})
		return len(p), nil
	}
	return b.buf.Write(p)
}

func (b *cappedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// execResult is what exec's callback received: stdout and stderr, and on
// failure the error message exec built.
type execResult struct {
	Stdout string
	Stderr string
	// Failed is true when the command did not exit 0 (or never started).
	Failed bool
	// Message is error.message: "Command failed: <cmd>\n<stderr>" for a
	// non-zero exit or a timeout kill, "<stream> maxBuffer length exceeded",
	// or the spawn error.
	Message string
}

// runShell is exec(command, {shell, timeout: 30 s, maxBuffer: 1 MiB}): the
// command line is passed as one argument to `<shell> -c`, exactly as the
// route's purpose (an admin shell) requires; it is never spliced into
// another command.
func runShell(ctx context.Context, shell, command string) execResult {
	ctx, cancel := context.WithTimeout(ctx, executeTimeout)
	defer cancel()
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	ov := &overflowState{cancel: stop}
	stdout := &cappedBuffer{name: "stdout", overflow: ov}
	stderr := &cappedBuffer{name: "stderr", overflow: ov}

	// execx stops the whole process group on timeout or overflow, so
	// `sleep 600 &` or a pipeline started by the command cannot outlive the
	// 30-second limit (2.x only killed the shell).
	_, err := execx.Run(runCtx, execx.Cmd{Name: shell, Args: []string{"-c", command}, Stdout: stdout, Stderr: stderr})
	res := execResult{Stdout: stdout.String(), Stderr: stderr.String()}
	var startErr *execx.StartError
	switch {
	case ov.stream != "":
		res.Failed, res.Message = true, ov.stream+" maxBuffer length exceeded"
	case err == nil:
	case errors.As(err, &startErr):
		// The process never started (no such shell).
		res.Failed, res.Message = true, startErr.Err.Error()
	default:
		res.Failed, res.Message = true, "Command failed: "+command+"\n"+res.Stderr
	}
	return res
}
