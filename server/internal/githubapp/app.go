// Package githubapp is the GitHub App side of Docklift: App credentials kept
// in the settings table, the RS256 App JWT, cached installation access
// tokens, the GitHub REST client and the one-time setup states of the App
// manifest/installation flow. It is the port of the helpers exported by
// backend/src/routes/github.ts (getSetting, getPrivateKey, getAppId,
// createJwtToken, getInstallationToken, getInstallationIdForRepo) and of
// backend/src/lib/githubSetupState.ts.
package githubapp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"docklift/internal/js"
)

// Settings keys used by the GitHub integration.
const (
	KeyAppID          = "github_app_id"
	KeyAppSlug        = "github_app_slug"
	KeyAppName        = "github_app_name"
	KeyClientID       = "github_client_id"
	KeyClientSecret   = "github_client_secret"
	KeyPrivateKey     = "github_private_key"
	KeyWebhookSecret  = "github_webhook_secret"
	KeyUsername       = "github_username"
	KeyAvatarURL      = "github_avatar_url"
	KeyInstallationID = "github_installation_id"
	KeyPendingAppName = "github_pending_app_name"
)

// Error messages of createJwtToken.
var (
	ErrNoPrivateKey = errors.New("GitHub App private key not found. Please create a GitHub App first.")
	ErrNoAppID      = errors.New("GitHub App ID not found. Please create a GitHub App first.")
)

// tokenRefreshMargin is how long before expiry a cached installation token is
// replaced; defaultTokenLifetime applies when GitHub gives no expires_at.
const (
	tokenRefreshMargin   = 5 * time.Minute
	defaultTokenLifetime = time.Hour
)

// SettingsStore is the settings table (implemented by *store.Store).
type SettingsStore interface {
	GetSetting(ctx context.Context, key string) (string, bool, error)
	SetSetting(ctx context.Context, key, value string) error
	DeleteSetting(ctx context.Context, key string) error
}

// Config is what the App needs from the process configuration.
type Config struct {
	// AppID is DOCKLIFT_GITHUB_APP_ID (fallback when no App is stored).
	AppID string
	// PrivateKeyPath is DOCKLIFT_GITHUB_PRIVATE_KEY_PATH, resolved against the
	// working directory like path.resolve.
	PrivateKeyPath string
	// DataPath is DATA_PATH; setup states live in DATA_PATH/github-setup.
	DataPath string
}

type cachedToken struct {
	token     string
	expiresAt time.Time
}

// App holds the GitHub App credentials and the installation-token cache. It
// is safe for concurrent use; create one per process so the cache is shared.
type App struct {
	cfg      Config
	settings SettingsStore
	api      API
	log      *slog.Logger
	now      func() time.Time
	states   *SetupStates

	mu     sync.Mutex
	tokens map[string]cachedToken
}

// New returns an App using api for GitHub calls (NewClient("") in production).
func New(cfg Config, settings SettingsStore, api API, log *slog.Logger) *App {
	a := &App{cfg: cfg, settings: settings, api: api, log: log, now: time.Now, tokens: map[string]cachedToken{}}
	a.states = newSetupStates(filepath.Join(cfg.DataPath, "github-setup"), settings, func() time.Time { return a.now() })
	return a
}

// SetClock replaces the clock (tests).
func (a *App) SetClock(now func() time.Time) { a.now = now }

// API returns the GitHub client.
func (a *App) API() API { return a.api }

// States returns the setup-state registry.
func (a *App) States() *SetupStates { return a.states }

// Setting is getSetting: the stored value, with "" and NULL both reported as
// absent (setting?.value || null).
func (a *App) Setting(ctx context.Context, key string) (string, bool, error) {
	v, ok, err := a.settings.GetSetting(ctx, key)
	if err != nil {
		return "", false, err
	}
	return v, ok && v != "", nil
}

// SaveSetting is saveSetting (upsert).
func (a *App) SaveSetting(ctx context.Context, key, value string) error {
	return a.settings.SetSetting(ctx, key, value)
}

// DeleteSetting is deleteSetting.
func (a *App) DeleteSetting(ctx context.Context, key string) error {
	return a.settings.DeleteSetting(ctx, key)
}

// PrivateKey is getPrivateKey: the PEM stored by the manifest flow, else the
// legacy key file at DOCKLIFT_GITHUB_PRIVATE_KEY_PATH.
func (a *App) PrivateKey(ctx context.Context) (string, bool, error) {
	if v, ok, err := a.Setting(ctx, KeyPrivateKey); err != nil || ok {
		return v, ok, err
	}
	path, err := filepath.Abs(a.cfg.PrivateKeyPath)
	if err != nil {
		return "", false, fmt.Errorf("resolve GitHub private key path: %w", err)
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read GitHub private key: %w", err)
	}
	return string(b), true, nil
}

// AppID is getAppId: the stored App ID, else DOCKLIFT_GITHUB_APP_ID.
func (a *App) AppID(ctx context.Context) (string, bool, error) {
	if v, ok, err := a.Setting(ctx, KeyAppID); err != nil || ok {
		return v, ok, err
	}
	return a.cfg.AppID, a.cfg.AppID != "", nil
}

// JWT is createJwtToken: an RS256 App JWT valid for ten minutes (issued a
// minute in the past to tolerate clock drift).
func (a *App) JWT(ctx context.Context) (string, error) {
	pem, ok, err := a.PrivateKey(ctx)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrNoPrivateKey
	}
	appID, ok, err := a.AppID(ctx)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrNoAppID
	}
	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(pem))
	if err != nil {
		return "", fmt.Errorf("parse GitHub App private key: %w", err)
	}
	now := a.now().Unix()
	// iss is parseInt(appId); NaN (and Infinity) serialize as null.
	claims := jwt.MapClaims{"iat": now - 60, "exp": now + 600, "iss": nil}
	if iss := js.ParseInt10(appID); !math.IsNaN(iss) && !math.IsInf(iss, 0) {
		claims["iss"] = iss
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
	if err != nil {
		return "", fmt.Errorf("sign GitHub App JWT: %w", err)
	}
	return signed, nil
}

// InstallationToken is getInstallationToken, with caching: a token is reused
// until five minutes before it expires. Tokens are cached per App ID and
// installation, so a newly created App never reuses an old App's tokens.
func (a *App) InstallationToken(ctx context.Context, installationID string) (string, error) {
	appID, _, err := a.AppID(ctx)
	if err != nil {
		return "", err
	}
	key := appID + "\x00" + installationID
	a.mu.Lock()
	cached, hit := a.tokens[key]
	a.mu.Unlock()
	if hit && a.now().Add(tokenRefreshMargin).Before(cached.expiresAt) {
		return cached.token, nil
	}
	appJWT, err := a.JWT(ctx)
	if err != nil {
		return "", err
	}
	t, err := a.api.CreateInstallationToken(ctx, appJWT, installationID)
	if err != nil {
		return "", err
	}
	exp := t.ExpiresAt
	if exp.IsZero() {
		exp = a.now().Add(defaultTokenLifetime)
	}
	a.mu.Lock()
	for k, v := range a.tokens {
		if !a.now().Before(v.expiresAt) {
			delete(a.tokens, k)
		}
	}
	a.tokens[key] = cachedToken{token: t.Token, expiresAt: exp}
	a.mu.Unlock()
	return t.Token, nil
}

// ForgetTokens empties the installation-token cache (disconnect, new App).
func (a *App) ForgetTokens() {
	a.mu.Lock()
	clear(a.tokens)
	a.mu.Unlock()
}

// InstallationIDForRepo is getInstallationIdForRepo: the installation that
// covers owner/repo, else the saved installation ID (Apps installed on "All
// repositories"), else the lookup error.
func (a *App) InstallationIDForRepo(ctx context.Context, owner, repo string) (string, error) {
	appJWT, err := a.JWT(ctx)
	if err != nil {
		return "", err
	}
	id, lookupErr := a.api.RepoInstallationID(ctx, appJWT, owner, repo)
	if lookupErr == nil {
		return id, nil
	}
	// Only a GitHub answer falls back; transport and decode errors propagate.
	if StatusOf(lookupErr) == 0 {
		return "", lookupErr
	}
	global, ok, err := a.Setting(ctx, KeyInstallationID)
	if err != nil {
		return "", err
	}
	if ok {
		return global, nil
	}
	return "", lookupErr
}
