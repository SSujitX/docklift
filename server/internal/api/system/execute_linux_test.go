//go:build linux

package system

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// gone reports whether pid no longer runs (absent or a zombie).
func gone(pid int) bool {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return true
	}
	fields := strings.Fields(string(b))
	return len(fields) > 2 && fields[2] == "Z"
}

func TestRunShellKillsBackgroundChildren(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not installed")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	res := runShell(ctx, "bash", "sleep 60 & echo $!; wait")
	if !res.Failed || !strings.HasPrefix(res.Message, "Command failed: ") {
		t.Fatalf("a killed command must fail: %+v", res)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(res.Stdout))
	if err != nil {
		t.Fatalf("child pid %q: %v", res.Stdout, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !gone(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("background child %d survived the timeout", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
