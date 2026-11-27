package execx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The test binary doubles as the child program: with EXECX_HELPER set,
// TestMain runs a small behaviour instead of the tests.
func TestMain(m *testing.M) {
	if mode := os.Getenv("EXECX_HELPER"); mode != "" {
		os.Exit(helper(mode, os.Args[1:]))
	}
	os.Exit(m.Run())
}

func helper(mode string, args []string) int {
	switch mode {
	case "echo":
		fmt.Fprint(os.Stdout, strings.Join(args, " "))
		fmt.Fprint(os.Stderr, "err-out")
		return 0
	case "exit":
		n, _ := strconv.Atoi(args[0])
		return n
	case "env":
		fmt.Fprint(os.Stdout, os.Getenv(args[0]))
		return 0
	case "stdin":
		b, _ := io.ReadAll(os.Stdin)
		fmt.Fprint(os.Stdout, strings.ToUpper(string(b)))
		return 0
	case "pwd":
		wd, _ := os.Getwd()
		fmt.Fprint(os.Stdout, wd)
		return 0
	case "chunks":
		for i := range 3 {
			fmt.Fprintf(os.Stdout, "out%d\n", i)
			fmt.Fprintf(os.Stderr, "err%d\n", i)
		}
		return 0
	case "sleep":
		fmt.Fprintln(os.Stdout, "started")
		time.Sleep(time.Minute)
		return 0
	case "spawn":
		// Start a grandchild that sleeps, print its pid, then sleep too.
		child := exec.Command(os.Args[0])
		child.Env = append(os.Environ(), "EXECX_HELPER=sleep")
		if err := child.Start(); err != nil {
			return 3
		}
		fmt.Fprintf(os.Stdout, "%d\n", child.Process.Pid)
		time.Sleep(time.Minute)
		return 0
	}
	return 99
}

func self(t *testing.T, mode string, args ...string) Cmd {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return Cmd{Name: exe, Args: args, ExtraEnv: []string{"EXECX_HELPER=" + mode}}
}

func TestRunCapturesOutputAndExitCode(t *testing.T) {
	res, err := Run(context.Background(), self(t, "echo", "a b", "$HOME", "`x`;|&"))
	if err != nil {
		t.Fatal(err)
	}
	// Arguments reach the program verbatim: no shell expansion of any kind.
	if got := string(res.Stdout); got != "a b $HOME `x`;|&" {
		t.Fatalf("stdout = %q", got)
	}
	if string(res.Stderr) != "err-out" || res.ExitCode != 0 {
		t.Fatalf("stderr=%q code=%d", res.Stderr, res.ExitCode)
	}
}

func TestRunNonZeroExit(t *testing.T) {
	c := self(t, "exit", "7")
	c.Args = append(c.Args, "SECRET_TOKEN=hunter2")
	res, err := Run(context.Background(), c)
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != 7 || res.ExitCode != 7 {
		t.Fatalf("err=%v code=%d", err, res.ExitCode)
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("error leaks arguments: %v", err)
	}
}

func TestRunEnvironment(t *testing.T) {
	t.Setenv("EXECX_INHERITED", "parent")
	c := self(t, "env", "EXECX_INHERITED")
	out, _, _, err := Output(context.Background(), c)
	if err != nil || out != "parent" {
		t.Fatalf("inherited env: %q %v", out, err)
	}
	c.ExtraEnv = append(c.ExtraEnv, "EXECX_INHERITED=override")
	if out, _, _, err = Output(context.Background(), c); err != nil || out != "override" {
		t.Fatalf("ExtraEnv override: %q %v", out, err)
	}
	exe, _ := os.Executable()
	c = Cmd{Name: exe, Args: []string{"EXECX_INHERITED"}, Env: []string{"EXECX_HELPER=env", "SYSTEMROOT=" + os.Getenv("SYSTEMROOT")}}
	if out, _, _, err = Output(context.Background(), c); err != nil || out != "" {
		t.Fatalf("Env replaces environment: %q %v", out, err)
	}
}

func TestRunStdinAndDir(t *testing.T) {
	c := self(t, "stdin")
	c.Stdin = strings.NewReader("hello")
	if out, _, _, err := Output(context.Background(), c); err != nil || out != "HELLO" {
		t.Fatalf("stdin: %q %v", out, err)
	}
	dir := t.TempDir()
	c = self(t, "pwd")
	c.Dir = dir
	out, _, _, err := Output(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	gotInfo, err1 := os.Stat(out)
	wantInfo, err2 := os.Stat(dir)
	if err1 != nil || err2 != nil || !os.SameFile(gotInfo, wantInfo) {
		t.Fatalf("dir = %q, want %q", out, dir)
	}
}

func TestRunStreamsToWriters(t *testing.T) {
	var combined bytes.Buffer
	w := &sharedWriter{w: &combined}
	c := self(t, "chunks")
	c.Stdout, c.Stderr = w, w
	res, err := Run(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Stdout) != 0 || len(res.Stderr) != 0 {
		t.Fatal("streamed output must not also be captured")
	}
	for _, want := range []string{"out0", "out2", "err0", "err2"} {
		if !strings.Contains(combined.String(), want) {
			t.Fatalf("combined output %q lacks %q", combined.String(), want)
		}
	}
}

type sharedWriter struct{ w io.Writer }

func (l *sharedWriter) Write(p []byte) (int, error) { return l.w.Write(p) }

func TestRunTimeoutKills(t *testing.T) {
	c := self(t, "sleep")
	c.Timeout = 300 * time.Millisecond
	c.KillGrace = 200 * time.Millisecond
	start := time.Now()
	res, err := Run(context.Background(), c)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
	if res.ExitCode == 0 {
		t.Fatalf("exit code = %d", res.ExitCode)
	}
	if time.Since(start) > 20*time.Second {
		t.Fatal("timeout did not stop the program promptly")
	}
}

func TestRunContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := self(t, "sleep")
	c.KillGrace = 200 * time.Millisecond
	started := make(chan int, 1)
	c.OnStart = func(pid int) { started <- pid }
	done := make(chan error, 1)
	go func() {
		_, err := Run(ctx, c)
		done <- err
	}()
	select {
	case pid := <-started:
		if pid <= 0 {
			t.Fatalf("pid = %d", pid)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("program never started")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestRunAlreadyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := false
	c := self(t, "echo")
	c.OnStart = func(int) { started = true }
	if _, err := Run(ctx, c); !errors.Is(err, context.Canceled) || started {
		t.Fatalf("err=%v started=%v", err, started)
	}
}

func TestRunStartError(t *testing.T) {
	res, err := Run(context.Background(), Cmd{Name: "docklift-no-such-program-xyz", Args: []string{"PASSWORD=x"}})
	var se *StartError
	if !errors.As(err, &se) || !errors.Is(err, exec.ErrNotFound) || res.ExitCode != -1 {
		t.Fatalf("err=%v code=%d", err, res.ExitCode)
	}
	if strings.Contains(err.Error(), "PASSWORD") {
		t.Fatalf("error leaks arguments: %v", err)
	}
	if _, err := Run(context.Background(), Cmd{}); err == nil {
		t.Fatal("empty name must fail")
	}
}

func TestTailBuffer(t *testing.T) {
	b := &TailBuffer{Max: 5}
	for _, s := range []string{"abc", "defg", "h"} {
		if _, err := b.Write([]byte(s)); err != nil {
			t.Fatal(err)
		}
	}
	if b.String() != "defgh" {
		t.Fatalf("tail = %q", b.String())
	}
	unlimited := &TailBuffer{}
	_, _ = unlimited.Write([]byte("0123456789"))
	if unlimited.String() != "0123456789" {
		t.Fatalf("unlimited = %q", unlimited.String())
	}
}

func TestWriterFunc(t *testing.T) {
	var got []string
	w := WriterFunc(func(p []byte) { got = append(got, string(p)) })
	n, err := w.Write([]byte("x"))
	if n != 1 || err != nil || len(got) != 1 {
		t.Fatalf("n=%d err=%v got=%v", n, err, got)
	}
}
