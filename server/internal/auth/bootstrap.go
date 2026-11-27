package auth

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// File names inside DATA_PATH.
const (
	BootstrapSecretFileName = ".bootstrap-secret"
	SetupTokenFileName      = ".setup-token"
	claimPrefix             = ".claimed-"
)

// SetupTokenHeader authorises a fresh-install restore upload.
const SetupTokenHeader = "X-Setup-Token"

// Bootstrap manages the fresh-install bootstrap secret (printed to the logs,
// never served by a public API) and the restore setup token — the ports of
// lib/bootstrap.ts and lib/setupRestoreAuth.ts.
type Bootstrap struct {
	dataPath string
	log      *slog.Logger
}

// NewBootstrap returns the bootstrap files of dataPath.
func NewBootstrap(dataPath string, log *slog.Logger) *Bootstrap {
	return &Bootstrap{dataPath: dataPath, log: log}
}

// SecretPath is DATA_PATH/.bootstrap-secret.
func (b *Bootstrap) SecretPath() string { return filepath.Join(b.dataPath, BootstrapSecretFileName) }

// SetupTokenPath is DATA_PATH/.setup-token.
func (b *Bootstrap) SetupTokenPath() string { return filepath.Join(b.dataPath, SetupTokenFileName) }

// EnsureSecret returns the bootstrap secret, creating it (32 random bytes,
// hex, mode 0600) when absent.
func (b *Bootstrap) EnsureSecret() (string, error) {
	return ensureTokenFile(b.dataPath, b.SecretPath())
}

// ReadSecret returns the trimmed secret, or "" when missing or empty.
func (b *Bootstrap) ReadSecret() string {
	return readTrimmed(b.SecretPath())
}

// VerifySecret compares provided with the bootstrap secret in constant time.
func (b *Bootstrap) VerifySecret(provided string) bool {
	return VerifySecretAt(b.SecretPath(), provided)
}

// ConsumeSecret deletes the bootstrap secret (closes the claim window).
func (b *Bootstrap) ConsumeSecret() {
	removeIfExists(b.SecretPath())
}

// TryClaim atomically renames the secret to a unique claim file so only one
// registration can proceed. It returns the claim path, or "" when lost.
func (b *Bootstrap) TryClaim() string {
	if _, err := os.Stat(b.SecretPath()); err != nil {
		return ""
	}
	suffix, err := randomHex(16)
	if err != nil {
		return ""
	}
	claim := filepath.Join(b.dataPath, claimPrefix+suffix)
	if err := os.Rename(b.SecretPath(), claim); err != nil {
		return ""
	}
	return claim
}

// VerifySecretAt compares provided with the trimmed content of path without
// leaking content or length through timing; empty values never match.
func VerifySecretAt(path, provided string) bool {
	expected := readTrimmed(path)
	if provided == "" || expected == "" {
		return false
	}
	return secureEqual(provided, expected)
}

// FinalizeClaim drops the claim file after a successful registration.
func (b *Bootstrap) FinalizeClaim(claim string) {
	removeIfExists(claim)
}

// AbortClaim puts the secret back when registration fails after the claim.
// The claimed secret is the one the operator holds, so it also replaces a
// secret that GET /status re-created while the claim was out (that one was
// never shown to anybody); otherwise a failed registration would lock the
// operator out until the next restart.
func (b *Bootstrap) AbortClaim(claim string) {
	if _, err := os.Stat(claim); err != nil {
		return
	}
	if err := os.Rename(claim, b.SecretPath()); err != nil {
		b.log.Warn("[bootstrap] Failed to restore claimed bootstrap secret", "error", err)
	}
}

// RecoverStaleClaims restores the newest claim file to .bootstrap-secret when
// a registration crashed between claim and completion, and removes the rest.
func (b *Bootstrap) RecoverStaleClaims() {
	if _, err := os.Stat(b.SecretPath()); err == nil {
		return
	}
	claims, err := b.claimFiles()
	if err != nil {
		b.log.Warn("[bootstrap] Failed to recover stale claims", "error", err)
		return
	}
	if len(claims) == 0 {
		return
	}
	if err := os.Rename(claims[0], b.SecretPath()); err != nil {
		b.log.Warn("[bootstrap] Failed to recover stale claims", "error", err)
		return
	}
	for _, orphan := range claims[1:] {
		removeIfExists(orphan)
	}
	b.log.Warn(fmt.Sprintf("[bootstrap] Restored stale claim %s → .bootstrap-secret", filepath.Base(claims[0])))
}

// claimFiles lists regular .claimed-* files, newest first.
func (b *Bootstrap) claimFiles() ([]string, error) {
	entries, err := os.ReadDir(b.dataPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read data directory: %w", err)
	}
	type claim struct {
		path  string
		mtime time.Time
	}
	var claims []claim
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), claimPrefix) {
			continue
		}
		p := filepath.Join(b.dataPath, e.Name())
		st, err := os.Stat(p)
		if err != nil || !st.Mode().IsRegular() {
			continue
		}
		claims = append(claims, claim{p, st.ModTime()})
	}
	sort.SliceStable(claims, func(i, j int) bool { return claims[i].mtime.After(claims[j].mtime) })
	out := make([]string, len(claims))
	for i, c := range claims {
		out[i] = c.path
	}
	return out, nil
}

// UserCounter counts users (implemented by *store.Store).
type UserCounter interface {
	CountUsers(ctx context.Context) (int, error)
}

// LogIfNeeded is logBootstrapIfNeeded: with an admin present it removes
// leftover claim files; on a fresh install (or an unreadable database) it
// recovers stale claims, ensures the secret and prints it to stdout.
func (b *Bootstrap) LogIfNeeded(ctx context.Context, users UserCounter) error {
	if n, err := users.CountUsers(ctx); err == nil && n > 0 {
		if entries, err := os.ReadDir(b.dataPath); err == nil {
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), claimPrefix) {
					removeIfExists(filepath.Join(b.dataPath, e.Name()))
				}
			}
		}
		return nil
	}
	b.RecoverStaleClaims()
	secret, err := b.EnsureSecret()
	if err != nil {
		return err
	}
	fmt.Printf(`
╔══════════════════════════════════════════════════════════════════╗
║  SECURITY: Fresh install — bootstrap secret required             ║
║                                                                  ║
║  Paste this into the Setup page (Register / Restore).            ║
║  It is NOT available via any public API.                         ║
║                                                                  ║
║  %s
║                                                                  ║
║  Also on host: data/.bootstrap-secret                            ║
╚══════════════════════════════════════════════════════════════════╝

`, secret)
	return nil
}

// EnsureSetupToken returns the setup token, creating it (32 random bytes,
// hex, mode 0600) when absent.
func (b *Bootstrap) EnsureSetupToken() (string, error) {
	return ensureTokenFile(b.dataPath, b.SetupTokenPath())
}

// ValidateSetupToken checks a setup token without consuming it.
func (b *Bootstrap) ValidateSetupToken(provided string) bool {
	return VerifySecretAt(b.SetupTokenPath(), provided)
}

// ConsumeSetupRestoreSecrets deletes the setup token and the bootstrap secret
// after a successful fresh-install restore.
func (b *Bootstrap) ConsumeSetupRestoreSecrets() {
	removeIfExists(b.SetupTokenPath())
	b.ConsumeSecret()
}

type setupTokenKey struct{}

// WithSetupTokenAuth marks a request as authorised by a valid setup token
// (fresh-install restore-upload; the token is consumed only on success).
func WithSetupTokenAuth(ctx context.Context) context.Context {
	return context.WithValue(ctx, setupTokenKey{}, true)
}

// SetupTokenAuthorized reports whether the request was authorised by a setup token.
func SetupTokenAuthorized(ctx context.Context) bool {
	ok, _ := ctx.Value(setupTokenKey{}).(bool)
	return ok
}

// tokenFileWait is how long ensureTokenFile waits for a token file another
// request is creating (it exists but is still empty) before replacing it.
const (
	tokenFileWait = 500 * time.Millisecond
	tokenFilePoll = 10 * time.Millisecond
)

// ensureTokenFile returns the token stored at path, creating it when absent.
// Creation is exclusive (O_EXCL), so concurrent callers (two GET /status or
// GET /setup-token on a fresh install) agree on one token instead of the
// last writer silently invalidating the value the first one returned. A file
// that stays empty (a crash between create and write) is replaced.
func ensureTokenFile(dir, path string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create data directory: %w", err)
	}
	name := filepath.Base(path)
	deadline := time.Now().Add(tokenFileWait)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		switch {
		case err == nil:
			if token := strings.TrimSpace(string(raw)); token != "" {
				return token, nil
			}
		case errors.Is(err, fs.ErrNotExist):
			token, created, err := createTokenFile(path)
			if err != nil {
				return "", fmt.Errorf("write %s: %w", name, err)
			}
			if created {
				return token, nil
			}
			continue
		default:
			return "", fmt.Errorf("read %s: %w", name, err)
		}
		time.Sleep(tokenFilePoll)
	}
	token, err := randomHex(32)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", name, err)
	}
	return token, nil
}

// createTokenFile writes a new random token to path unless the file already
// exists (created is then false).
func createTokenFile(path string) (token string, created bool, err error) {
	token, err = randomHex(32)
	if err != nil {
		return "", false, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	_, werr := f.WriteString(token)
	if err := errors.Join(werr, f.Close()); err != nil {
		_ = os.Remove(path)
		return "", false, err
	}
	return token, true, nil
}

func readTrimmed(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func removeIfExists(path string) {
	_ = os.Remove(path)
}
