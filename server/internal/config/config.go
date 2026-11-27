// Package config turns the process environment into a Config. It is the single
// source of truth for every environment variable the TypeScript backend read
// (backend/src/lib/config.ts plus the scattered process.env reads), with the
// same names and defaults.
package config

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"docklift/internal/js"
)

// DefaultDatabaseURL is used when DATABASE_URL is unset; it is the fallback
// backend/src/routes/backup.ts used and the value the Docker image sets.
const DefaultDatabaseURL = "file:/app/data/docklift.db"

// Config holds every setting read from the environment.
type Config struct {
	// Port is PORT (default 8000).
	Port int
	// DataPath is DATA_PATH resolved to an absolute path (default ./data).
	DataPath string
	// DeploymentsPath is DEPLOYMENTS_PATH resolved (default ./deployments).
	DeploymentsPath string
	// BackupPath is BACKUP_PATH resolved (default <DATA_PATH>/backups).
	BackupPath string
	// DockerNetwork is DOCKER_NETWORK (default docklift_network).
	DockerNetwork string

	// GitHubAppID is DOCKLIFT_GITHUB_APP_ID.
	GitHubAppID string
	// GitHubClientID is DOCKLIFT_GITHUB_CLIENT_ID.
	GitHubClientID string
	// GitHubClientSecret is DOCKLIFT_GITHUB_CLIENT_SECRET.
	GitHubClientSecret string
	// GitHubPrivateKeyPath is DOCKLIFT_GITHUB_PRIVATE_KEY_PATH, kept as given
	// (not resolved), default ./github-app.pem.
	GitHubPrivateKeyPath string
	// FrontendURL is DOCKLIFT_FRONTEND_URL (default http://localhost:3000).
	FrontendURL string
	// NginxConfPath is NGINX_CONF_PATH resolved (default ./nginx-proxy/conf.d).
	NginxConfPath string
	// PortRangeStart and PortRangeEnd are PORT_RANGE_START/PORT_RANGE_END
	// (defaults 5500 and 5600), the host port pool for deployed apps.
	PortRangeStart int
	PortRangeEnd   int

	// LetsEncryptPath is LETSENCRYPT_PATH resolved (default ./nginx-proxy/certbot/conf).
	LetsEncryptPath string
	// CertbotContainer is CERTBOT_CONTAINER (default docklift-certbot).
	CertbotContainer string
	// CertbotEmail is CERTBOT_EMAIL.
	CertbotEmail string
	// CertbotStaging is CERTBOT_STAGING == "true" or "1".
	CertbotStaging bool

	// CORSOrigins is CORS_ORIGIN split on commas (entries are not trimmed
	// here; the origin check trims them, as originCheck.ts does).
	CORSOrigins []string
	// NginxProxyContainer is NGINX_PROXY_CONTAINER (default docklift-nginx-proxy).
	NginxProxyContainer string
	// TerminalAllowNoOrigin is TERMINAL_ALLOW_NO_ORIGIN == "true".
	TerminalAllowNoOrigin bool
	// TrustedProxies is TRUSTED_PROXIES (default DefaultTrustedProxies): the
	// socket peers whose X-Forwarded-For/-Proto/-Host headers are believed.
	// New in the Go backend; the TypeScript one trusted every peer.
	TrustedProxies ProxyTrust

	// JWTSecret and InternalAPISecret are JWT_SECRET and INTERNAL_API_SECRET.
	// Empty means "use or generate DATA_PATH/.secrets".
	JWTSecret         string
	InternalAPISecret string

	// DatabaseURL is DATABASE_URL as given (default DefaultDatabaseURL).
	DatabaseURL string
	// DatabasePath is the SQLite file DatabaseURL points at.
	DatabasePath string
}

// Load reads the configuration from the process environment. Relative paths
// resolve against the current working directory, like path.resolve in Node.
func Load() (*Config, error) {
	return FromEnv(os.Getenv)
}

// FromEnv builds a Config from an environment lookup function.
func FromEnv(getenv func(string) string) (*Config, error) {
	or := func(key, def string) string {
		if v := getenv(key); v != "" {
			return v
		}
		return def
	}
	var err error
	c := &Config{
		DockerNetwork:         or("DOCKER_NETWORK", "docklift_network"),
		GitHubAppID:           getenv("DOCKLIFT_GITHUB_APP_ID"),
		GitHubClientID:        getenv("DOCKLIFT_GITHUB_CLIENT_ID"),
		GitHubClientSecret:    getenv("DOCKLIFT_GITHUB_CLIENT_SECRET"),
		GitHubPrivateKeyPath:  or("DOCKLIFT_GITHUB_PRIVATE_KEY_PATH", "./github-app.pem"),
		FrontendURL:           or("DOCKLIFT_FRONTEND_URL", "http://localhost:3000"),
		CertbotContainer:      or("CERTBOT_CONTAINER", "docklift-certbot"),
		CertbotEmail:          getenv("CERTBOT_EMAIL"),
		CertbotStaging:        getenv("CERTBOT_STAGING") == "true" || getenv("CERTBOT_STAGING") == "1",
		NginxProxyContainer:   or("NGINX_PROXY_CONTAINER", "docklift-nginx-proxy"),
		TerminalAllowNoOrigin: getenv("TERMINAL_ALLOW_NO_ORIGIN") == "true",
		JWTSecret:             getenv("JWT_SECRET"),
		InternalAPISecret:     getenv("INTERNAL_API_SECRET"),
		DatabaseURL:           or("DATABASE_URL", DefaultDatabaseURL),
	}
	if v := getenv("CORS_ORIGIN"); v != "" {
		c.CORSOrigins = strings.Split(v, ",")
	}

	if c.TrustedProxies, err = ParseTrustedProxies(or("TRUSTED_PROXIES", DefaultTrustedProxies)); err != nil {
		return nil, err
	}
	if c.Port, err = parseInt("PORT", or("PORT", "8000"), false); err != nil {
		return nil, err
	}
	if c.PortRangeStart, err = parseInt("PORT_RANGE_START", or("PORT_RANGE_START", "5500"), true); err != nil {
		return nil, err
	}
	if c.PortRangeEnd, err = parseInt("PORT_RANGE_END", or("PORT_RANGE_END", "5600"), true); err != nil {
		return nil, err
	}

	dataRaw := or("DATA_PATH", "./data")
	paths := []struct {
		dst *string
		raw string
	}{
		{&c.DataPath, dataRaw},
		{&c.DeploymentsPath, or("DEPLOYMENTS_PATH", "./deployments")},
		{&c.BackupPath, or("BACKUP_PATH", filepath.Join(dataRaw, "backups"))},
		{&c.NginxConfPath, or("NGINX_CONF_PATH", "./nginx-proxy/conf.d")},
		{&c.LetsEncryptPath, or("LETSENCRYPT_PATH", "./nginx-proxy/certbot/conf")},
	}
	for _, p := range paths {
		abs, err := filepath.Abs(p.raw)
		if err != nil {
			return nil, fmt.Errorf("resolve path %q: %w", p.raw, err)
		}
		*p.dst = abs
	}

	if c.DatabasePath, err = DatabasePathFromURL(c.DatabaseURL); err != nil {
		return nil, err
	}
	return c, nil
}

// DatabasePathFromURL converts a Prisma SQLite URL (`file:/abs/path.db` or
// `file:./relative.db`, optionally with `?params`) into a file path. Prisma
// resolves relative SQLite paths against the directory holding schema.prisma
// (`<backend root>/prisma`); the working directory plays the role of the
// backend root here, so `file:./data/docklift.db` maps to
// `<cwd>/prisma/data/docklift.db` exactly as it did for the TypeScript backend.
func DatabasePathFromURL(url string) (string, error) {
	rest, ok := strings.CutPrefix(url, "file:")
	if !ok {
		return "", fmt.Errorf("DATABASE_URL must use the file: scheme, got %q", url)
	}
	if i := strings.IndexByte(rest, '?'); i >= 0 {
		rest = rest[:i]
	}
	if rest == "" {
		return "", fmt.Errorf("DATABASE_URL %q has no file path", url)
	}
	if filepath.IsAbs(rest) || strings.HasPrefix(rest, "/") {
		return filepath.Clean(rest), nil
	}
	abs, err := filepath.Abs(filepath.Join("prisma", rest))
	if err != nil {
		return "", fmt.Errorf("resolve database path %q: %w", rest, err)
	}
	return abs, nil
}

// parseInt mimics JavaScript parseInt: leading whitespace and sign are
// accepted, parsing stops at the first invalid digit, and a value with no
// digits is rejected (NaN in JS, which made the Node server fail to listen).
// Without radix10 a 0x prefix selects hexadecimal, as parseInt(s) does.
func parseInt(name, s string, radix10 bool) (int, error) {
	n := js.ParseInt(s)
	if radix10 {
		n = js.ParseInt10(s)
	}
	switch {
	case math.IsNaN(n):
		return 0, fmt.Errorf("%s=%q is not a number", name, s)
	case math.Abs(n) > 1<<31:
		return 0, fmt.Errorf("%s=%q is out of range", name, s)
	}
	return int(n), nil
}
