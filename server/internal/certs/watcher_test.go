package certs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// helperProcess is the child side of the ExecRunner tests.
func helperProcess() {
	switch os.Getenv("DOCKLIFT_CERTS_HELPER") {
	case "ok":
		fmt.Fprint(os.Stdout, "out:", os.Args[1:])
		fmt.Fprint(os.Stderr, "err")
	case "fail":
		fmt.Fprint(os.Stderr, "nginx: configuration file /etc/nginx/nginx.conf test failed")
		os.Exit(3)
	case "sleep":
		time.Sleep(10 * time.Second)
	}
}

func TestExecRunner(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	t.Setenv("DOCKLIFT_CERTS_HELPER", "ok")
	res, err := ExecRunner{}.Run(ctx, exe, "a b", "$(x);")
	if err != nil || res.ExitCode != 0 || res.Stdout != "out:[a b $(x);]" || res.Stderr != "err" {
		t.Fatalf("%+v %v", res, err)
	}
	t.Setenv("DOCKLIFT_CERTS_HELPER", "fail")
	res, err = ExecRunner{}.Run(ctx, exe)
	if err != nil || res.ExitCode != 3 || res.Stderr == "" {
		t.Fatalf("%+v %v", res, err)
	}
	t.Setenv("DOCKLIFT_CERTS_HELPER", "sleep")
	start := time.Now()
	res, err = ExecRunner{Timeout: 200 * time.Millisecond}.Run(ctx, exe)
	if !errors.Is(err, context.DeadlineExceeded) || res.ExitCode != -1 || time.Since(start) > 8*time.Second {
		t.Fatalf("%+v %v", res, err)
	}
	if _, err := (ExecRunner{}).Run(ctx, filepath.Join(t.TempDir(), "no-such-program")); err == nil {
		t.Fatal("missing program")
	}
}

func TestCappedBuffer(t *testing.T) {
	b := &cappedBuffer{limit: 4}
	n, _ := b.Write([]byte("abcdef"))
	_, _ = b.Write([]byte("gh"))
	if n != 6 || b.String() != "abcd" {
		t.Fatal(n, b.String())
	}
}

type reloadRecorder struct {
	calls atomic.Int32
	fail  atomic.Bool
	ch    chan struct{}
}

func newRecorder() *reloadRecorder { return &reloadRecorder{ch: make(chan struct{}, 64)} }

func (r *reloadRecorder) reload(context.Context) error {
	r.calls.Add(1)
	r.ch <- struct{}{}
	if r.fail.Load() {
		return errors.New("Nginx reload failed: boom")
	}
	return nil
}

func (r *reloadRecorder) wait(t *testing.T) {
	t.Helper()
	select {
	case <-r.ch:
	case <-time.After(5 * time.Second):
		t.Fatal("reload not called")
	}
}

func (r *reloadRecorder) none(t *testing.T, d time.Duration) {
	t.Helper()
	select {
	case <-r.ch:
		t.Fatal("unexpected reload")
	case <-time.After(d):
	}
}

func eventually(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestRenewWatcherReloadsOnChangeAndPersists(t *testing.T) {
	f := newFixture(t, WithWatcherIntervals(20*time.Millisecond, 20*time.Millisecond))
	writeLive(t, f.root, "a.example.com", fixturePEM(t, "orphan-b.pem"))
	pem := filepath.Join(f.root, "live", "a.example.com", "fullchain.pem")
	stamp := time.Date(2026, 11, 1, 10, 0, 0, 123456789, time.UTC)
	if err := os.Chtimes(pem, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(pem)
	want := mtimeMs(st.ModTime())

	ctx, cancel := context.WithCancel(context.Background())
	rec := newRecorder()
	done := f.svc.StartCertRenewWatcher(ctx, rec.reload)
	rec.wait(t) // startup: nothing persisted yet
	eventually(t, func() bool { return f.svc.readPersistedCertMtime() == want })
	raw, _ := os.ReadFile(filepath.Join(f.data, "ssl-cert-mtime"))
	if string(raw) != strconv.FormatFloat(want, 'f', -1, 64) {
		t.Fatalf("persisted %q", raw)
	}
	rec.none(t, 100*time.Millisecond)

	later := stamp.Add(time.Hour)
	if err := os.Chtimes(pem, later, later); err != nil {
		t.Fatal(err)
	}
	rec.wait(t)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("watcher did not stop on cancel")
	}
	n := rec.calls.Load()
	if err := os.Chtimes(pem, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	time.Sleep(80 * time.Millisecond)
	if rec.calls.Load() != n {
		t.Fatal("watcher ran after cancel")
	}

	// A new watcher with the persisted stamp does not reload at startup
	// unless the files changed while it was down.
	f.svc.writePersistedCertMtime(mtimeMs(stamp))
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	rec2 := newRecorder()
	f.svc.StartCertRenewWatcher(ctx2, rec2.reload)
	rec2.none(t, 100*time.Millisecond)
}

func TestRenewWatcherRetriesFailedReload(t *testing.T) {
	f := newFixture(t, WithWatcherIntervals(time.Hour, 20*time.Millisecond))
	writeLive(t, f.root, "a.example.com", fixturePEM(t, "orphan-b.pem"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rec := newRecorder()
	rec.fail.Store(true)
	f.svc.StartCertRenewWatcher(ctx, rec.reload)
	rec.wait(t)
	rec.wait(t) // retried without waiting for the hourly poll
	if f.svc.readPersistedCertMtime() != 0 {
		t.Fatal("stamp persisted after a failed reload")
	}
	rec.fail.Store(false)
	rec.wait(t)
	eventually(t, func() bool { return f.svc.readPersistedCertMtime() != 0 })
	rec.none(t, 100*time.Millisecond)
}

func TestRenewWatcherClearsAfterConsecutiveEmptyScans(t *testing.T) {
	f := newFixture(t, WithWatcherIntervals(20*time.Millisecond, 20*time.Millisecond))
	f.svc.writePersistedCertMtime(1234.5)
	w := &renewWatcher{s: f.svc, reload: func(context.Context) error { return nil }, lastStamp: f.svc.readPersistedCertMtime()}
	if w.lastStamp != 1234.5 {
		t.Fatal(w.lastStamp)
	}
	w.tick(context.Background(), "poll")
	if f.svc.readPersistedCertMtime() != 1234.5 {
		t.Fatal("cleared after one empty scan")
	}
	w.tick(context.Background(), "poll")
	if f.svc.readPersistedCertMtime() != 0 || w.lastStamp != 0 {
		t.Fatal("not cleared after two empty scans")
	}
	if _, err := os.Stat(filepath.Join(f.data, "ssl-cert-mtime")); !os.IsNotExist(err) {
		t.Fatal("state file kept")
	}
}

func TestPersistedCertMtimeParsing(t *testing.T) {
	f := newFixture(t)
	if err := os.MkdirAll(f.data, 0o755); err != nil {
		t.Fatal(err)
	}
	for raw, want := range map[string]float64{"1754851175123.4567\n": 1754851175123.4567, "": 0, "abc": 0, "-5": 0, "NaN": 0, "Infinity": 0} {
		if err := os.WriteFile(filepath.Join(f.data, "ssl-cert-mtime"), []byte(raw), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := f.svc.readPersistedCertMtime(); got != want {
			t.Errorf("%q: %v", raw, got)
		}
	}
	if got := mtimeMs(time.Unix(1754851175, 123456789)); got != 1754851175*1e3+123456789/1e6 {
		t.Fatal(got)
	}
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.conf")
	if err := WriteFileAtomic(p, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(p, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	entries, _ := os.ReadDir(dir)
	if string(raw) != "two" || len(entries) != 1 {
		t.Fatalf("%q %d", raw, len(entries))
	}
	if err := WriteFileAtomic(filepath.Join(dir, "missing", "x.conf"), []byte("x"), 0o644); err == nil {
		t.Fatal("missing dir")
	}
}
