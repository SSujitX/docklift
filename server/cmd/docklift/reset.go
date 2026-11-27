package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"

	"docklift/internal/auth"
	"docklift/internal/config"
	"docklift/internal/db"
	"docklift/internal/store"
)

// resetPassword is the port of scripts/reset-password.ts: give the first
// admin a random 16-character password and print it. Unlike the script, it
// also bumps passwordChangedAt so sessions issued before the reset stop
// working (the reason an operator resets a password).
func resetPassword(ctx context.Context, log *slog.Logger) error {
	fmt.Print("\n🔐 Docklift Password Reset\n\n")
	cfg, err := loadConfig(log)
	if err != nil {
		return err
	}
	return resetAdminPassword(ctx, cfg)
}

func resetAdminPassword(ctx context.Context, cfg *config.Config) (err error) {
	database, err := db.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, database.Close()) }()
	st := store.New(database)

	user, err := st.FirstUserByRole(ctx, "admin")
	if errors.Is(err, store.ErrNotFound) {
		fmt.Print("❌ No admin user found. Please run setup first.\n\n")
		return exitCode(1)
	}
	if err != nil {
		fmt.Printf("❌ Error resetting password: %v\n", err)
		return exitCode(1)
	}

	password, err := randomPassword()
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	now := db.Now()
	if _, err := st.UpdateUser(ctx, user.ID, store.UserUpdate{Password: store.Some(hash), PasswordChangedAt: store.Some(&now)}); err != nil {
		fmt.Printf("❌ Error resetting password: %v\n", err)
		return exitCode(1)
	}

	fmt.Print("✅ Password reset successful!\n\n")
	fmt.Printf("   User:     %s\n", user.Email)
	fmt.Printf("   Password: %s\n\n", password)
	fmt.Print("⚠️  Please change this password after logging in.\n\n")
	return nil
}

// randomPassword is crypto.randomBytes(12).toString('base64').slice(0, 16).
func randomPassword() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate password: %w", err)
	}
	return base64.StdEncoding.EncodeToString(b)[:16], nil
}
