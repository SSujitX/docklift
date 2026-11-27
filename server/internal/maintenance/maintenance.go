// Package maintenance holds the process-wide maintenance gate, the global
// restore lock and the persistent restore-critical seal.
// The HTTP gate reads it; the backup/restore flow drives it.
package maintenance

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DefaultReason is reported while maintenance is on without a reason.
const DefaultReason = "System maintenance in progress"

// CriticalMarkerName is the seal file name inside DATA_PATH.
const CriticalMarkerName = ".restore-critical"

// State is the maintenance/restore state of one process.
type State struct {
	dataPath string
	log      *slog.Logger

	mu         sync.Mutex
	active     bool
	reason     string
	locked     bool
	lockReason string
}

// New returns the state for a data directory.
func New(dataPath string, log *slog.Logger) *State {
	return &State{dataPath: dataPath, log: log}
}

// Enter turns maintenance mode on.
func (s *State) Enter(why string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active, s.reason = true, why
}

// Exit turns maintenance mode off.
func (s *State) Exit() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active, s.reason = false, ""
}

// Active reports whether maintenance mode is on.
func (s *State) Active() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

// Reason is the maintenance reason, or DefaultReason.
func (s *State) Reason() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reason == "" {
		return DefaultReason
	}
	return s.reason
}

// CriticalMarkerPath is DATA_PATH/.restore-critical.
func (s *State) CriticalMarkerPath() string {
	return filepath.Join(s.dataPath, CriticalMarkerName)
}

// Critical reports whether the seal exists on disk (any file type counts: a
// leftover directory is still critical).
func (s *State) Critical() bool {
	_, err := os.Stat(s.CriticalMarkerPath())
	return err == nil
}

// TryAcquireRestoreLock takes the global restore lock. It fails while another
// restore holds it or while the critical seal exists.
func (s *State) TryAcquireRestoreLock(why string) bool {
	if s.Critical() {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locked {
		return false
	}
	s.locked, s.lockReason = true, why
	return true
}

// ForceRestoreLock holds the lock without checking prior state (critical seal).
func (s *State) ForceRestoreLock(why string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.locked, s.lockReason = true, why
}

// ReleaseRestoreLock drops the lock, except while the critical seal exists.
func (s *State) ReleaseRestoreLock() {
	critical := s.Critical()
	s.mu.Lock()
	defer s.mu.Unlock()
	if critical {
		s.locked = true
		if s.lockReason == "" {
			s.lockReason = "restore-critical"
		}
		return
	}
	s.locked, s.lockReason = false, ""
}

// RestoreLocked reports whether a restore holds the lock or the seal exists.
func (s *State) RestoreLocked() bool {
	if s.Critical() {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.locked
}

// RestoreLockReason explains why the restore lock is held.
func (s *State) RestoreLockReason() string {
	if s.Critical() {
		return "CRITICAL restore recovery required — clear .restore-critical after manual repair"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lockReason == "" {
		return "A restore is already in progress"
	}
	return s.lockReason
}

// CriticalPayload is the JSON content of the seal file.
type CriticalPayload struct {
	At             string `json:"at"`
	Detail         string `json:"detail"`
	LiveDBPath     string `json:"liveDbPath"`
	PreRestorePath string `json:"preRestorePath"`
}

// ReadCritical returns the seal content, a placeholder payload when the seal
// path is not a regular file, or nil when there is no readable seal.
func (s *State) ReadCritical() *CriticalPayload {
	p := s.CriticalMarkerPath()
	st, err := os.Stat(p)
	if err != nil {
		return nil
	}
	if !st.Mode().IsRegular() {
		return &CriticalPayload{Detail: "invalid seal path (not a file)"}
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var payload CriticalPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	return &payload
}

func (s *State) criticalReason() string {
	return "CRITICAL restore recovery required — do not retry restore. See " + s.CriticalMarkerPath()
}

// EnterCritical persists the seal (mode 0600), keeps maintenance on and holds
// the restore lock.
func (s *State) EnterCritical(detail, liveDBPath string) error {
	payload := CriticalPayload{
		At:             time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		Detail:         detail,
		LiveDBPath:     liveDBPath,
		PreRestorePath: liveDBPath + ".pre-restore",
	}
	if err := os.MkdirAll(s.dataPath, 0o755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("encode restore-critical seal: %w", err)
	}
	if err := os.WriteFile(s.CriticalMarkerPath(), raw, 0o600); err != nil {
		return fmt.Errorf("write restore-critical seal: %w", err)
	}
	s.Enter(s.criticalReason())
	s.ForceRestoreLock("restore-critical")
	s.log.Error("[CRITICAL] Restore sealed", "at", payload.At, "detail", payload.Detail,
		"liveDbPath", payload.LiveDBPath, "preRestorePath", payload.PreRestorePath)
	return nil
}

// LoadCriticalOnBoot re-applies maintenance and the lock when a previous
// process left the seal behind. It must run before serving traffic.
func (s *State) LoadCriticalOnBoot() {
	if !s.Critical() {
		return
	}
	payload := s.ReadCritical()
	s.Enter(s.criticalReason())
	s.ForceRestoreLock("restore-critical")
	s.log.Error("[CRITICAL] Loaded .restore-critical from disk", "payload", payload)
}

// ClearCritical is the explicit operator clear after manual repair. It fails
// closed: if the seal cannot be removed, maintenance and the lock stay on.
func (s *State) ClearCritical() (*CriticalPayload, error) {
	prev := s.ReadCritical()
	p := s.CriticalMarkerPath()
	if st, err := os.Lstat(p); err == nil {
		// unlink semantics (fs.unlinkSync): a directory at the seal path is not removed.
		rmErr := errIsDirectory
		if !st.IsDir() {
			rmErr = unwrapPathError(os.Remove(p))
		}
		if rmErr != nil {
			return nil, fmt.Errorf("Failed to delete critical seal at %s: %v. Maintenance remains active.", p, rmErr)
		}
	}
	if _, err := os.Stat(p); err == nil {
		return nil, fmt.Errorf("Critical seal still present after delete: %s. Maintenance remains active.", p)
	}
	s.Exit()
	s.ReleaseRestoreLock()
	s.log.Warn("[CRITICAL] Restore critical seal cleared by operator")
	return prev, nil
}

var errIsDirectory = errors.New("illegal operation on a directory")

func unwrapPathError(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}
