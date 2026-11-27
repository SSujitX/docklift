//go:build linux

package system

import (
	"os/exec"
	"syscall"
)

// killProcessGroup makes the command lead its own process group and makes a
// timeout or cancellation SIGKILL the whole group, so `sleep 600 &` or a
// pipeline started by the command cannot outlive the 30-second limit
// (Node's exec only killed the shell).
func killProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
