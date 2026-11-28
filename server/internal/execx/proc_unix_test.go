//go:build !windows

package execx

import (
	"bufio"
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestCancelKillsProcessTree checks that cancelling a run also kills the
// grandchild the program started (process-group kill).
func TestCancelKillsProcessTree(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()
	c := self(t, "spawn")
	c.Stdout = pw
	c.KillGrace = 300 * time.Millisecond
	done := make(chan error, 1)
	go func() {
		_, err := Run(ctx, c)
		_ = pw.Close()
		done <- err
	}()
	line, err := bufio.NewReader(pr).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	go func() { _, _ = io.Copy(io.Discard, pr) }()
	grandchild, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		// The grandchild is not our child, so once killed it is reaped by
		// init and kill(pid, 0) reports ESRCH.
		if err := syscall.Kill(grandchild, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		if time.Now().After(deadline) {
			_ = syscall.Kill(grandchild, syscall.SIGKILL)
			t.Fatal("grandchild survived the cancel")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
