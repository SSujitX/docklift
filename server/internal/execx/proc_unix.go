//go:build !windows

package execx

import (
	"os"
	"os/exec"
	"syscall"
)

// prepare starts the program in its own process group so a cancel reaches
// its children too (docker compose runs CLI plugins as children).
func prepare(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// terminateTree asks the whole process group to stop.
func terminateTree(p *os.Process) {
	if err := syscall.Kill(-p.Pid, syscall.SIGTERM); err != nil {
		_ = p.Signal(syscall.SIGTERM)
	}
}

// killTree force-kills the whole process group.
func killTree(p *os.Process) {
	if err := syscall.Kill(-p.Pid, syscall.SIGKILL); err != nil {
		_ = p.Kill()
	}
}
