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
	"os"
	"strconv"
	"sync/atomic"

	"golang.org/x/crypto/bcrypt"
)

// ErrPasswordTooLong is returned by HashPassword when the plaintext exceeds
// bcrypt's hard 72-byte limit. bcrypt itself returns an error in that case
// (bcrypt.ErrPasswordTooLong), but we surface a stable sentinel so callers
// can decide whether to truncate, reject the input, or re-prompt without
// importing golang.org/x/crypto just to match the constant.
var ErrPasswordTooLong = errors.New("password exceeds bcrypt 72-byte limit")

// BcryptCostEnv is the env var that overrides the bcrypt cost. Operators
// can raise it on faster hardware without rebuilding the binary; lowering
// below MinCost (4) is rejected and we fall back to DefaultCost (10). We
// clamp the upper bound at bcrypt.MaxCost (31) so a typo cannot wedge
// the process behind an hour-long hash.
const BcryptCostEnv = "OPC_BCRYPT_COST"

// bcryptCost is the resolved per-process cost. We cache it in an
// atomic.Int32 so HashPassword does not parse the env var on every call,
// and tests (or future config-reload paths) can call SetBcryptCost to
// adjust it deterministically.
var bcryptCost atomic.Int32

// resolveBcryptCost returns the configured cost the first time it is
// called and caches the result. The resolution rule is:
//
//   - read OPC_BCRYPT_COST
//   - if unset or not a base-10 integer: bcrypt.DefaultCost (10)
//   - if < bcrypt.MinCost (4): bcrypt.DefaultCost (10) — values below
//     MinCost are not safe and we treat them as misconfiguration
//   - if > bcrypt.MaxCost (31): bcrypt.MaxCost (31)
//   - otherwise the integer itself
//
// We treat misconfiguration as "fall back to default" rather than crash
// because password hashing is on the request hot path; making the API
// refuse to start because of a typo in an env var is worse than logging
// nothing and using the safe default. Operators who care about the
// exact value can verify it by calling CurrentBcryptCost at startup.
func resolveBcryptCost() int {
	raw := os.Getenv(BcryptCostEnv)
	if raw == "" {
		return bcrypt.DefaultCost
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return bcrypt.DefaultCost
	}
	if n < bcrypt.MinCost {
		return bcrypt.DefaultCost
	}
	if n > bcrypt.MaxCost {
		return bcrypt.MaxCost
	}
	return n
}

// CurrentBcryptCost returns the cost HashPassword will use for the next
// invocation. Tests use it to assert the env-var override path. The
// value is resolved lazily on first call and then cached.
func CurrentBcryptCost() int {
	if c := bcryptCost.Load(); c != 0 {
		return int(c)
	}
	c := resolveBcryptCost()
	bcryptCost.Store(int32(c))
	return c
}

// SetBcryptCost overrides the per-process cost programmatically. Tests
// use this to exercise the cost path without spinning up a new process;
// production code should prefer OPC_BCRYPT_COST. Out-of-range values
// are clamped using the same rules as resolveBcryptCost.
func SetBcryptCost(cost int) {
	if cost < bcrypt.MinCost {
		cost = bcrypt.DefaultCost
	}
	if cost > bcrypt.MaxCost {
		cost = bcrypt.MaxCost
	}
	bcryptCost.Store(int32(cost))
}

// HashPassword returns a bcrypt hash of plain at the per-process cost
// resolved from OPC_BCRYPT_COST (defaulting to bcrypt.DefaultCost, 10).
// The cost is read once and cached in an atomic so the hot path is just
// an atomic load. See CurrentBcryptCost / SetBcryptCost for the override
// surface.
func HashPassword(plain string) (string, error) {
	cost := CurrentBcryptCost()
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), cost)
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
