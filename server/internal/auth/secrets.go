// Package auth ports the TypeScript authentication layer: persisted secrets,
// jsonwebtoken-compatible session and purpose tokens, bcrypt passwords, the
// Bearer / SSE-query-token / internal-secret middleware, password step-up,
// the fresh-install bootstrap secret and the restore setup token.
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// SecretsFileName is the secrets file inside DATA_PATH.
const SecretsFileName = ".secrets"

// Secrets are the process-wide signing and internal-call secrets.
type Secrets struct {
	JWTSecret         string `json:"jwtSecret"`
	InternalAPISecret string `json:"internalApiSecret"`
}

// LoadOrCreateSecrets resolves the secrets with the priority of
// authMiddleware.ts: (1) JWT_SECRET and INTERNAL_API_SECRET both set in the
// environment; (2) DATA_PATH/.secrets when it holds both values; (3) newly
// generated values (an env value still wins for its half), persisted to
// DATA_PATH/.secrets with mode 0600. A failed write only costs session
// persistence across restarts, as in TypeScript; a failure to create
// randomness or the data directory is returned.
func LoadOrCreateSecrets(dataPath, envJWT, envInternal string, log *slog.Logger) (Secrets, error) {
	if envJWT != "" && envInternal != "" {
		log.Info("🔐 Using secrets from environment variables")
		return Secrets{JWTSecret: envJWT, InternalAPISecret: envInternal}, nil
	}

	path := filepath.Join(dataPath, SecretsFileName)
	if raw, err := os.ReadFile(path); err == nil {
		var s Secrets
		if json.Unmarshal(raw, &s) == nil && s.JWTSecret != "" && s.InternalAPISecret != "" {
			log.Info("🔐 Loaded persisted secrets")
			return s, nil
		}
		log.Warn("⚠️  Failed to load secrets file, generating new ones")
	} else if !os.IsNotExist(err) {
		log.Warn("⚠️  Failed to load secrets file, generating new ones", "error", err)
	}

	s := Secrets{JWTSecret: envJWT, InternalAPISecret: envInternal}
	var err error
	if s.JWTSecret == "" {
		if s.JWTSecret, err = randomHex(64); err != nil {
			return Secrets{}, err
		}
	}
	if s.InternalAPISecret == "" {
		if s.InternalAPISecret, err = randomHex(64); err != nil {
			return Secrets{}, err
		}
	}
	if err := os.MkdirAll(dataPath, 0o755); err != nil {
		return Secrets{}, fmt.Errorf("create data directory: %w", err)
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return Secrets{}, fmt.Errorf("encode secrets: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		log.Warn("⚠️  Could not persist secrets (sessions will reset on restart)", "error", err)
		return s, nil
	}
	log.Info("🔐 Generated and saved new secrets")
	return s, nil
}

// randomHex returns n random bytes hex-encoded.
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random secret: %w", err)
	}
	return hex.EncodeToString(b), nil
}
