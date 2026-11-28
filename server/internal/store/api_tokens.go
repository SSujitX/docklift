package store

import (
	"context"

	"docklift/internal/db"
)

// APIToken is a row of "api_tokens": a bearer credential for scripts and AI
// agents. Only the sha256 of the token is stored (TokenHash, never
// serialised); Prefix is the first characters of the plain token, shown so
// the owner can tell tokens apart.
type APIToken struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	TokenHash      string   `json:"-"`
	Prefix         string   `json:"prefix"`
	UserID         string   `json:"user_id"`
	Scope          string   `json:"scope"`
	AllowDangerous bool     `json:"allow_dangerous"`
	CreatedAt      db.Time  `json:"created_at"`
	LastUsedAt     *db.Time `json:"last_used_at"`
	ExpiresAt      *db.Time `json:"expires_at"`
}

const apiTokenCols = `"id", "name", "token_hash", "prefix", "user_id", "scope", "allow_dangerous", "created_at", "last_used_at", "expires_at"`

func scanAPIToken(r rowScanner) (*APIToken, error) {
	var t APIToken
	if err := r.Scan(&t.ID, &t.Name, &t.TokenHash, &t.Prefix, &t.UserID, &t.Scope, &t.AllowDangerous, &t.CreatedAt, &t.LastUsedAt, &t.ExpiresAt); err != nil {
		return nil, err
	}
	return &t, nil
}

// APITokenCreate holds the fields of a new token.
type APITokenCreate struct {
	Name           string
	TokenHash      string
	Prefix         string
	UserID         string
	Scope          string
	AllowDangerous bool
	ExpiresAt      *db.Time
}

// CreateAPIToken inserts a token; a duplicate hash is ErrConflict.
func (s *Store) CreateAPIToken(ctx context.Context, in APITokenCreate) (*APIToken, error) {
	t := &APIToken{
		ID:             db.NewID(),
		Name:           in.Name,
		TokenHash:      in.TokenHash,
		Prefix:         in.Prefix,
		UserID:         in.UserID,
		Scope:          in.Scope,
		AllowDangerous: in.AllowDangerous,
		CreatedAt:      db.Now(),
		ExpiresAt:      in.ExpiresAt,
	}
	_, err := execAffected(ctx, s.writer(), "create api token",
		`INSERT INTO "api_tokens" (`+apiTokenCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Name, t.TokenHash, t.Prefix, t.UserID, t.Scope, t.AllowDangerous, t.CreatedAt, t.LastUsedAt, t.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// ListAPITokens returns every token, newest first.
func (s *Store) ListAPITokens(ctx context.Context) ([]*APIToken, error) {
	return queryList(ctx, s.reader(), "list api tokens",
		`SELECT `+apiTokenCols+` FROM "api_tokens" ORDER BY "created_at" DESC`, nil, scanAPIToken)
}

// GetAPIToken returns the token with this id.
func (s *Store) GetAPIToken(ctx context.Context, id string) (*APIToken, error) {
	return queryOne(ctx, s.reader(), "get api token",
		`SELECT `+apiTokenCols+` FROM "api_tokens" WHERE "id" = ?`, []any{id}, scanAPIToken)
}

// GetAPITokenByHash returns the token whose sha256 hash is hash (the lookup
// of an authenticating request: an indexed equality on the hash, so the
// plain token is never compared byte by byte).
func (s *Store) GetAPITokenByHash(ctx context.Context, hash string) (*APIToken, error) {
	return queryOne(ctx, s.reader(), "get api token by hash",
		`SELECT `+apiTokenCols+` FROM "api_tokens" WHERE "token_hash" = ?`, []any{hash}, scanAPIToken)
}

// FindAPITokensByPrefix returns the tokens whose display prefix starts with
// prefix (the CLI's `revoke <prefix>`).
func (s *Store) FindAPITokensByPrefix(ctx context.Context, prefix string) ([]*APIToken, error) {
	return queryList(ctx, s.reader(), "find api tokens by prefix",
		`SELECT `+apiTokenCols+` FROM "api_tokens" WHERE substr("prefix", 1, length(?)) = ? ORDER BY "created_at" DESC`,
		[]any{prefix, prefix}, scanAPIToken)
}

// TouchAPIToken sets last_used_at.
func (s *Store) TouchAPIToken(ctx context.Context, id string, at db.Time) error {
	_, err := execAffected(ctx, s.writer(), "touch api token", `UPDATE "api_tokens" SET "last_used_at" = ? WHERE "id" = ?`, at, id)
	return err
}

// DeleteAPIToken removes a token; a missing id is ErrNotFound.
func (s *Store) DeleteAPIToken(ctx context.Context, id string) error {
	return execOne(ctx, s.writer(), "delete api token", `DELETE FROM "api_tokens" WHERE "id" = ?`, id)
}
