//go:build !linux

package terminal

// StartShell reports that the interactive terminal needs Linux (a PTY and
// /bin/bash); the session then shows the error like a failed spawn did.
func StartShell(int, int) (Shell, error) { return nil, ErrUnsupported }
