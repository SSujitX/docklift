//go:build !windows

package contract

import (
	"os/exec"
	"syscall"
	"time"
)

func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killTree signals the whole process group: SIGTERM first, SIGKILL after 5s.
func killTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	pgid := -cmd.Process.Pid
	_ = syscall.Kill(pgid, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			if syscall.Kill(pgid, 0) != nil {
				close(done)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		close(done)
	}()
	<-done
	_ = syscall.Kill(pgid, syscall.SIGKILL)
}
