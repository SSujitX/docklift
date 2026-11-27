package store

import (
	"context"
	"errors"

	"docklift/internal/db"
)

// Setting is a row of "settings" (key/value store for GitHub App and ACME
// configuration).
type Setting struct {
	Key       string   `json:"key"`
	Value     *string  `json:"value"`
	UpdatedAt *db.Time `json:"updated_at"`
}

// GetSetting returns a setting's value; a missing key or NULL value is ("", false).
func (s *Store) GetSetting(ctx context.Context, key string) (string, bool, error) {
	var v *string
	err := s.reader().QueryRowContext(ctx, `SELECT "value" FROM "settings" WHERE "key" = ?`, key).Scan(&v)
	if err := mapErr(err, "get setting"); err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	if v == nil {
		return "", false, nil
	}
	return *v, true, nil
}

// SetSetting creates or updates a setting (prisma.settings.upsert with
// update {value} / create {key, value}; updated_at is left untouched, as the
// TypeScript backend never set it).
func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := execAffected(ctx, s.writer(), "set setting",
		`INSERT INTO "settings" ("key", "value") VALUES (?, ?) ON CONFLICT ("key") DO UPDATE SET "value" = excluded."value"`,
		key, value)
	return err
}

// DeleteSetting removes a setting if present.
func (s *Store) DeleteSetting(ctx context.Context, key string) error {
	_, err := execAffected(ctx, s.writer(), "delete setting", `DELETE FROM "settings" WHERE "key" = ?`, key)
	return err
}
