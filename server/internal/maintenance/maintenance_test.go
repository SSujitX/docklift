package maintenance

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
)

func newState(t *testing.T) (*State, string) {
	t.Helper()
	dir := t.TempDir()
	return New(dir, slog.New(slog.NewTextHandler(io.Discard, nil))), dir
}

func TestMaintenanceMode(t *testing.T) {
	s, _ := newState(t)
	if s.Active() || s.Reason() != DefaultReason {
		t.Fatalf("initial state active=%v reason=%q", s.Active(), s.Reason())
	}
	s.Enter("Restoring backup")
	if !s.Active() || s.Reason() != "Restoring backup" {
		t.Errorf("after Enter active=%v reason=%q", s.Active(), s.Reason())
	}
	s.Exit()
	if s.Active() || s.Reason() != DefaultReason {
		t.Errorf("after Exit active=%v reason=%q", s.Active(), s.Reason())
	}
}

func TestRestoreLock(t *testing.T) {
	s, _ := newState(t)
	if !s.TryAcquireRestoreLock("restore-1") {
		t.Fatal("first acquire must succeed")
	}
	if s.TryAcquireRestoreLock("restore-2") || !s.RestoreLocked() || s.RestoreLockReason() != "restore-1" {
		t.Errorf("second acquire must fail; locked=%v reason=%q", s.RestoreLocked(), s.RestoreLockReason())
	}
	s.ReleaseRestoreLock()
	if s.RestoreLocked() || s.RestoreLockReason() != "A restore is already in progress" {
		t.Errorf("after release locked=%v reason=%q", s.RestoreLocked(), s.RestoreLockReason())
	}
}

// Ports of backend/src/lib/restoreCritical.test.ts.

func TestEnterRestoreCriticalPersistsMarkerAndBlocksNewLocks(t *testing.T) {
	s, dir := newState(t)
	live := filepath.Join(dir, "docklift.db")
	if err := s.EnterCritical("rollback failed", live); err != nil {
		t.Fatal(err)
	}
	if !s.Critical() {
		t.Fatal("seal not detected")
	}
	payload := s.ReadCritical()
	if payload == nil || payload.LiveDBPath != live || payload.PreRestorePath != live+".pre-restore" || payload.Detail != "rollback failed" {
		t.Fatalf("payload = %+v", payload)
	}
	if !regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{3}Z$`).MatchString(payload.At) {
		t.Errorf("at = %q, want ISO-8601", payload.At)
	}
	if s.TryAcquireRestoreLock("another-restore") || !s.RestoreLocked() {
		t.Error("critical seal must block new restore locks")
	}
	s.ReleaseRestoreLock()
	if !s.Critical() || s.TryAcquireRestoreLock("another-restore") {
		t.Error("release must not drop the lock while sealed")
	}
	if !s.Active() || s.RestoreLockReason() != "CRITICAL restore recovery required — clear .restore-critical after manual repair" {
		t.Errorf("maintenance=%v lockReason=%q", s.Active(), s.RestoreLockReason())
	}
	st, err := os.Stat(s.CriticalMarkerPath())
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
		t.Errorf("seal mode = %v, want 0600", st.Mode().Perm())
	}
}

func TestClearRestoreCriticalRemovesSealAndAllowsLocks(t *testing.T) {
	s, dir := newState(t)
	if err := s.EnterCritical("test", filepath.Join(dir, "docklift.db")); err != nil {
		t.Fatal(err)
	}
	prev, err := s.ClearCritical()
	if err != nil || prev == nil || prev.Detail != "test" {
		t.Fatalf("ClearCritical = %+v, %v", prev, err)
	}
	if s.Critical() || s.Active() {
		t.Errorf("critical=%v maintenance=%v after clear", s.Critical(), s.Active())
	}
	if !s.TryAcquireRestoreLock("ok") {
		t.Error("lock must be available after clear")
	}
	s.ReleaseRestoreLock()
}

func TestClearRestoreCriticalFailsClosedWhenMarkerCannotBeDeleted(t *testing.T) {
	s, dir := newState(t)
	live := filepath.Join(dir, "docklift.db")
	if err := s.EnterCritical("deletion-failure test", live); err != nil {
		t.Fatal(err)
	}
	// Replace the seal file with a directory so the unlink fails.
	if err := os.Remove(s.CriticalMarkerPath()); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(s.CriticalMarkerPath(), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := s.ClearCritical()
	if err == nil || !regexp.MustCompile(`(?i)Failed to delete critical seal|still present after delete`).MatchString(err.Error()) {
		t.Fatalf("ClearCritical err = %v", err)
	}
	if !s.Critical() || !s.Active() || s.TryAcquireRestoreLock("must-fail") {
		t.Errorf("must stay sealed: critical=%v maintenance=%v", s.Critical(), s.Active())
	}
	if p := s.ReadCritical(); p == nil || p.Detail != "invalid seal path (not a file)" {
		t.Errorf("directory seal payload = %+v", p)
	}

	if err := os.RemoveAll(s.CriticalMarkerPath()); err != nil {
		t.Fatal(err)
	}
	if err := s.EnterCritical("cleanup", live); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClearCritical(); err != nil {
		t.Fatal(err)
	}
}

func TestLoadCriticalOnBoot(t *testing.T) {
	s, dir := newState(t)
	s.LoadCriticalOnBoot()
	if s.Active() || s.RestoreLocked() {
		t.Fatal("no seal: boot must not enter maintenance")
	}
	if err := os.WriteFile(filepath.Join(dir, CriticalMarkerName), []byte(`{"at":"x","detail":"d","liveDbPath":"l","preRestorePath":"p"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	fresh := New(dir, slog.New(slog.NewTextHandler(io.Discard, nil)))
	fresh.LoadCriticalOnBoot()
	if !fresh.Active() || !fresh.RestoreLocked() || fresh.Reason() != "CRITICAL restore recovery required — do not retry restore. See "+fresh.CriticalMarkerPath() {
		t.Errorf("boot with seal: active=%v locked=%v reason=%q", fresh.Active(), fresh.RestoreLocked(), fresh.Reason())
	}
	if err := os.WriteFile(fresh.CriticalMarkerPath(), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if fresh.ReadCritical() != nil {
		t.Error("unparseable seal content reads as nil")
	}
}
