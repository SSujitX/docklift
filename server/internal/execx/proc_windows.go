//go:build windows

package execx

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

// prepare hides the console window of the child.
func prepare(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}

// terminateTree kills the program and its children with taskkill /T /F, as
// Docklift 2.x did. It runs while the process handle is still open, so the
// pid cannot have been reused.
func terminateTree(p *os.Process) {
	kill := exec.Command("taskkill", "/pid", strconv.Itoa(p.Pid), "/T", "/F")
	kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := kill.Run(); err != nil {
		_ = p.Kill()
	}
}

// killTree force-kills the program through its handle (never by pid, which
// Windows reuses quickly once the process has been waited for).
func killTree(p *os.Process) {
	_ = p.Kill()
}
