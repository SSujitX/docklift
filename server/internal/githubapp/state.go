package githubapp

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// StateTTL is how long a setup state stays valid (GITHUB_STATE_TTL_MS).
const StateTTL = time.Hour

// Legacy single-value state keys in settings, and the keys of the older
// pending-bypass flow that clearing a state also removes.
const (
	keyLegacyState      = "github_setup_state"
	keyLegacyStateAt    = "github_setup_state_at"
	keyInstallPending   = "github_install_pending"
	keyInstallPendingAt = "github_install_pending_at"
)

// stateToken is the only shape a state may have before it is used in a path:
// it arrives from a public query string or cookie.
var stateToken = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// IsSafeStateToken is isSafeSetupStateToken.
func IsSafeStateToken(state string) bool { return stateToken.MatchString(state) }

// StateFilePath is setupStateFilePath: the state file inside dir, or "" when
// state is not a plain token.
func StateFilePath(dir, state string) string {
	if !IsSafeStateToken(state) {
		return ""
	}
	return filepath.Join(dir, state+".json")
}

// stateFile is the JSON written to DATA_PATH/github-setup/<state>.json.
// CreatedAt is Unix milliseconds; a float so files with fractional or huge
// values written by hand still parse the way JSON.parse read them.
type stateFile struct {
	CreatedAt float64 `json:"createdAt,omitempty"`
	ReturnURL string  `json:"returnUrl,omitempty"`
}

// SetupStates is the registry of one-time CSRF nonces of the manifest and
// installation flow: random 48-hex-char tokens stored as files that expire
// after StateTTL and are deleted once a flow completes.
type SetupStates struct {
	dir      string
	settings SettingsStore
	now      func() time.Time
}

func newSetupStates(dir string, settings SettingsStore, now func() time.Time) *SetupStates {
	return &SetupStates{dir: dir, settings: settings, now: now}
}

// Dir is the directory holding the state files.
func (s *SetupStates) Dir() string { return s.dir }

func (s *SetupStates) ensureDir() error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create GitHub setup dir: %w", err)
	}
	return nil
}

func (s *SetupStates) expired(createdAt float64) bool {
	return createdAt == 0 || float64(s.now().UnixMilli())-createdAt > float64(StateTTL.Milliseconds())
}

func readStateFile(path string) (stateFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return stateFile{}, err
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		return stateFile{}, err
	}
	var f stateFile
	if n, ok := raw["createdAt"].(float64); ok && !math.IsNaN(n) {
		f.CreatedAt = n
	}
	if r, ok := raw["returnUrl"].(string); ok {
		f.ReturnURL = r
	}
	return f, nil
}

// Prune deletes expired and unreadable state files (pruneExpiredGithubSetupStates).
func (s *SetupStates) Prune() error {
	if err := s.ensureDir(); err != nil {
		return err
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return fmt.Errorf("list GitHub setup states: %w", err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") || e.IsDir() {
			continue
		}
		full := filepath.Join(s.dir, e.Name())
		f, err := readStateFile(full)
		if err != nil || s.expired(f.CreatedAt) {
			_ = os.Remove(full)
		}
	}
	return nil
}

// Create is createGithubSetupState: prune, then write a new state file
// (mode 0600) remembering returnURL when it is not empty.
func (s *SetupStates) Create(returnURL string) (string, error) {
	if err := s.Prune(); err != nil {
		return "", err
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate GitHub setup state: %w", err)
	}
	state := hex.EncodeToString(buf)
	body, err := json.Marshal(stateFile{CreatedAt: float64(s.now().UnixMilli()), ReturnURL: returnURL})
	if err != nil {
		return "", err
	}
	f, err := os.OpenFile(filepath.Join(s.dir, state+".json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("write GitHub setup state: %w", err)
	}
	_, werr := f.Write(body)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return "", fmt.Errorf("write GitHub setup state: %w", werr)
	}
	return state, nil
}

// ReturnURL is readGithubSetupStateReturnUrl: the return URL remembered by a
// state, "" when there is none or the state is not a plain token.
func (s *SetupStates) ReturnURL(state string) string {
	path := StateFilePath(s.dir, state)
	if path == "" {
		return ""
	}
	f, err := readStateFile(path)
	if err != nil {
		return ""
	}
	return f.ReturnURL
}

// Verify is verifyGithubSetupState: true when state names an unexpired state
// file, or matches the legacy single state stored in settings. Verification
// does not consume the state; Clear does once the flow completes.
func (s *SetupStates) Verify(ctx context.Context, state string) (bool, error) {
	if state == "" {
		return false, nil
	}
	if err := s.Prune(); err != nil {
		return false, err
	}
	path := StateFilePath(s.dir, state)
	if path == "" {
		return s.verifyLegacy(ctx, state)
	}
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return s.verifyLegacy(ctx, state)
	}
	f, err := readStateFile(path)
	if err != nil {
		return false, nil
	}
	if s.expired(f.CreatedAt) {
		_ = os.Remove(path)
		return false, nil
	}
	expected := strings.TrimSuffix(filepath.Base(path), ".json")
	return subtle.ConstantTimeCompare([]byte(state), []byte(expected)) == 1, nil
}

// verifyLegacy checks the single state older versions kept in settings.
func (s *SetupStates) verifyLegacy(ctx context.Context, state string) (bool, error) {
	expected, ok, err := s.setting(ctx, keyLegacyState)
	if err != nil || !ok {
		return false, err
	}
	at, ok, err := s.setting(ctx, keyLegacyStateAt)
	if err != nil || !ok {
		return false, err
	}
	// Number(at): a value that is not a number never counts as expired,
	// because NaN comparisons are false.
	if ms, perr := strconv.ParseFloat(strings.TrimSpace(at), 64); perr == nil && float64(s.now().UnixMilli())-ms > float64(StateTTL.Milliseconds()) {
		if err := s.settings.DeleteSetting(ctx, keyLegacyState); err != nil {
			return false, err
		}
		return false, s.settings.DeleteSetting(ctx, keyLegacyStateAt)
	}
	return subtle.ConstantTimeCompare([]byte(state), []byte(expected)) == 1, nil
}

func (s *SetupStates) setting(ctx context.Context, key string) (string, bool, error) {
	v, ok, err := s.settings.GetSetting(ctx, key)
	return v, ok && v != "", err
}

// Delete removes a state file (deleteGithubSetupStateFile); unsafe or
// unknown states are ignored.
func (s *SetupStates) Delete(state string) {
	if path := StateFilePath(s.dir, state); path != "" {
		_ = os.Remove(path)
	}
}

// Clear is clearGithubSetupState: delete the given state files plus the
// legacy state and pending-install settings.
func (s *SetupStates) Clear(ctx context.Context, states ...string) error {
	for _, st := range states {
		if st != "" {
			s.Delete(st)
		}
	}
	for _, k := range []string{keyLegacyState, keyLegacyStateAt, keyInstallPending, keyInstallPendingAt} {
		if err := s.settings.DeleteSetting(ctx, k); err != nil {
			return err
		}
	}
	return nil
}
