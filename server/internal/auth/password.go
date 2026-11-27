package auth

import (
	"bytes"
	"fmt"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

// BcryptCost is the cost Docklift 2.x hashed with.
const BcryptCost = 12

// bcryptMaxBytes is where bcrypt (and node's bcrypt) stops reading the password.
const bcryptMaxBytes = 72

// HashPassword returns a bcrypt hash in the `$2b$` form node's bcrypt writes.
// Passwords longer than 72 bytes are truncated exactly as bcrypt does.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword(truncate(password), BcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	// x/crypto writes the $2a$ identifier for the same algorithm; keep the
	// stored format identical to hashes created by Docklift 2.x.
	if bytes.HasPrefix(hash, []byte("$2a$")) {
		hash[2] = 'b'
	}
	return string(hash), nil
}

// ComparePassword reports whether password matches a bcrypt hash ($2a$, $2b$
// or $2y$). A malformed or empty hash never matches, like bcrypt.compare.
func ComparePassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), truncate(password)) == nil
}

// timingHash is a real cost-12 hash used to spend the same bcrypt time on
// unknown accounts as on known ones.
var timingHash = sync.OnceValue(func() string {
	hash, err := HashPassword("docklift-login-timing-equaliser")
	if err != nil {
		panic(fmt.Sprintf("bcrypt unavailable: %v", err))
	}
	return hash
})

// EqualizeLoginTiming runs one bcrypt comparison that always fails, so a
// login for an unknown email takes as long as a wrong password and does not
// reveal which emails exist.
func EqualizeLoginTiming(password string) {
	ComparePassword(timingHash(), password)
}

func truncate(password string) []byte {
	b := []byte(password)
	if len(b) > bcryptMaxBytes {
		b = b[:bcryptMaxBytes]
	}
	return b
}
