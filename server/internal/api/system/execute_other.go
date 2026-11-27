//go:build !linux

package system

import "os/exec"

// killProcessGroup is a no-op outside Linux: the command itself is killed
// on timeout (POST /execute needs /bin/bash, which production Linux has).
func killProcessGroup(*exec.Cmd) {}
