//go:build linux

package terminal

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// shellCommand and shellDir are what `script -q -c /bin/bash /dev/null` ran
// in the TypeScript backend.
const (
	shellCommand = "/bin/bash"
	shellDir     = "/root"
)

// shellEnv is the environment the TypeScript backend gave the shell, on top
// of the server's own environment (later entries win).
func shellEnv(cols, rows int) []string {
	return append(os.Environ(),
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"LANG=C.UTF-8",
		"COLUMNS="+strconv.Itoa(cols),
		"LINES="+strconv.Itoa(rows),
		`PS1=\[\e[1;32m\]\u@\h\[\e[0m\]:\[\e[1;34m\]\w\[\e[0m\]# `,
	)
}

// ptyShell is bash on a pseudo-terminal it leads (new session, controlling
// tty), so job control, resizing and Ctrl-C behave like a real terminal.
type ptyShell struct {
	cmd       *exec.Cmd
	tty       *os.File
	closeOnce sync.Once
	exited    atomic.Bool
}

// killGrace is how long a hung-up shell gets to exit before SIGKILL.
const killGrace = 2 * time.Second

// StartShell starts /bin/bash in /root on a new PTY of the given size.
func StartShell(cols, rows int) (Shell, error) {
	cmd := exec.Command(shellCommand)
	cmd.Dir = shellDir
	cmd.Env = shellEnv(cols, rows)
	tty, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return nil, err
	}
	return &ptyShell{cmd: cmd, tty: tty}, nil
}

func (s *ptyShell) Read(p []byte) (int, error)  { return s.tty.Read(p) }
func (s *ptyShell) Write(p []byte) (int, error) { return s.tty.Write(p) }

func (s *ptyShell) Resize(cols, rows int) error {
	return pty.Setsize(s.tty, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

func (s *ptyShell) Wait() int {
	err := s.cmd.Wait()
	s.exited.Store(true)
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if code := exitErr.ExitCode(); code > 0 {
			return code
		}
	}
	return 0
}

// Close hangs the terminal up: SIGHUP to the shell's process group (bash
// passes it on to its background jobs, as when `script` was killed and its
// PTY closed in TypeScript) and the PTY master is closed. A shell still
// alive after killGrace gets SIGKILL.
func (s *ptyShell) Close() {
	s.closeOnce.Do(func() {
		pgid := s.cmd.Process.Pid
		_ = syscall.Kill(-pgid, syscall.SIGHUP)
		_ = s.tty.Close()
		time.AfterFunc(killGrace, func() {
			if !s.exited.Load() {
				_ = syscall.Kill(-pgid, syscall.SIGKILL)
			}
		})
	})
}
