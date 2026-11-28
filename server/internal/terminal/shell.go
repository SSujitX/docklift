package terminal

import (
	"errors"
	"io"
)

// Shell is an interactive shell attached to a terminal.
type Shell interface {
	// Read returns terminal output; it fails once the shell is gone.
	io.Reader
	// Write sends keyboard input.
	io.Writer
	// Resize sets the terminal size.
	Resize(cols, rows int) error
	// Wait blocks until the shell exits and returns its exit code (0 when
	// it was killed by a signal).
	Wait() int
	// Close kills the shell and everything it started, and releases the
	// terminal. It is safe to call more than once.
	Close()
}

// StartFunc starts a shell with an initial terminal size.
type StartFunc func(cols, rows int) (Shell, error)

// ErrUnsupported is returned by StartShell where there is no PTY support.
var ErrUnsupported = errors.New("interactive terminal is only supported on Linux")
