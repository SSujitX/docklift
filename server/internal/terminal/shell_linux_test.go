//go:build linux

package terminal

import (
	"bytes"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func requireRealShell(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath(shellCommand); err != nil {
		t.Skip("/bin/bash not installed")
	}
	if f, err := os.Open(shellDir); err != nil {
		t.Skipf("%s not accessible: %v", shellDir, err)
	} else {
		f.Close()
	}
}

// readUntil collects PTY output until it contains want.
func readUntil(t *testing.T, sh Shell, want string) string {
	t.Helper()
	var out bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for !strings.Contains(out.String(), want) {
			n, err := sh.Read(buf)
			out.Write(buf[:n])
			if err != nil {
				return
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		sh.Close()
		<-done
		t.Fatalf("no %q in %q", want, out.String())
	}
	return out.String()
}

func TestPTYShell(t *testing.T) {
	requireRealShell(t)
	sh, err := StartShell(100, 30)
	if err != nil {
		t.Fatal(err)
	}
	defer sh.Close()
	_, _ = sh.Write([]byte("echo TERM=$TERM; stty size\n"))
	readUntil(t, sh, "30 100")
	if err := sh.Resize(120, 40); err != nil {
		t.Fatal(err)
	}
	_, _ = sh.Write([]byte("stty size; exit 7\n"))
	readUntil(t, sh, "40 120")
	if code := sh.Wait(); code != 7 {
		t.Fatalf("exit code %d", code)
	}
}

func TestPTYShellCloseKillsProcessGroup(t *testing.T) {
	requireRealShell(t)
	sh, err := StartShell(80, 24)
	if err != nil {
		t.Fatal(err)
	}
	// The terminal echoes the command line, so match the expanded $! only.
	_, _ = sh.Write([]byte("sleep 300 & echo \"bg:$!:x\"\n"))
	var pid int
	for out := ""; pid == 0; {
		out = readUntil(t, sh, ":x")
		if m := regexp.MustCompile(`bg:(\d+):x`).FindStringSubmatch(out); m != nil {
			pid, _ = strconv.Atoi(m[1])
		}
	}
	codes := make(chan int, 1)
	go func() { codes <- sh.Wait() }()
	sh.Close()
	select {
	case code := <-codes:
		if code != 0 {
			t.Fatalf("killed shell must report 0, got %d", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("shell survived Close")
	}
	deadline := time.Now().Add(10 * time.Second)
	for alive(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("background job %d survived the hang-up", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// alive reports a process that exists and is not a zombie.
func alive(pid int) bool {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	_, after, _ := strings.Cut(string(b), ") ")
	return !strings.HasPrefix(after, "Z")
}
