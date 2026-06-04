// Package auth implements password hashing and session lifecycle helpers
// for Phase 2's session-based authentication. It deliberately does NOT
// touch HTTP, cookies, or middleware — those are caller's concerns. The
// functions here are pure-ish: they take a *gorm.DB and primitives, and
// return either a value or a wrapped error. That keeps them trivially
// testable with an in-memory SQLite.
package auth

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// ErrPasswordTooLong is returned by HashPassword when the plaintext exceeds
// bcrypt's hard 72-byte limit. bcrypt itself returns an error in that case
// (bcrypt.ErrPasswordTooLong), but we surface a stable sentinel so callers
// can decide whether to truncate, reject the input, or re-prompt without
// importing golang.org/x/crypto just to match the constant.
var ErrPasswordTooLong = errors.New("password exceeds bcrypt 72-byte limit")

// HashPassword returns a bcrypt hash of plain at the package's default
// cost. We intentionally do not take a cost parameter: tuning cost
// belongs in a deploy-time config knob, not in a per-call argument.
func HashPassword(plain string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		// Map bcrypt's sentinel to ours so callers don't need to import
		// golang.org/x/crypto to detect the "too long" case.
		if errors.Is(err, bcrypt.ErrPasswordTooLong) {
			return "", ErrPasswordTooLong
		}
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

// VerifyPassword returns nil iff plain matches hash. Any error (mismatch,
// malformed hash, etc.) is collapsed into a single non-nil return so
// callers can't accidentally leak "the hash is malformed" vs "wrong
// password" via distinct error types — both should be treated as
// "authentication failed" at the boundary.
func VerifyPassword(hash, plain string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)); err != nil {
		return fmt.Errorf("verify password: %w", err)
	}
	return nil
}
