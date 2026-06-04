package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/opc/api/internal/models"
)

// ErrSessionNotFound is returned by ValidateSession when the supplied
// token does not correspond to any session row. Exposed as a sentinel so
// handlers can map it to a 401.
var ErrSessionNotFound = errors.New("session not found")

// ErrSessionExpired is returned by ValidateSession when the row exists
// but ExpiresAt is in the past. We treat expiry as a separate error from
// "not found" so that an auth middleware can log them differently
// (expired = benign, not found = potentially tampered token).
var ErrSessionExpired = errors.New("session expired")

// sessionTTL is how long a freshly created session stays valid. The
// cookie the client receives is HttpOnly and this is the source of truth
// for expiry; there is no refresh-token dance in Phase 2.
const sessionTTL = 7 * 24 * time.Hour

// tokenBytes is the size of the random source we encode into a session
// token. 32 bytes = 256 bits, which is comfortably beyond brute force
// and renders to a 64-char hex string.
const tokenBytes = 32

// newSessionToken returns a fresh, hex-encoded session token. Exposed at
// package level (lowercase) for testability: tests can substitute a
// deterministic generator later if we ever need to. For now it's just
// crypto/rand.
func newSessionToken() (string, error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// CreateSession persists a fresh session for userID and returns the row.
// The returned Session.Token is what callers should hand to the client
// (typically by setting it in an HttpOnly cookie). The ExpiresAt field is
// authoritative — do not derive it from now() in middleware.
func CreateSession(db *gorm.DB, userID uint) (models.Session, error) {
	token, err := newSessionToken()
	if err != nil {
		return models.Session{}, fmt.Errorf("generate session token: %w", err)
	}
	now := time.Now().UTC()
	s := models.Session{
		UserID:    userID,
		Token:     token,
		ExpiresAt: now.Add(sessionTTL),
		CreatedAt: now,
	}
	if err := db.Create(&s).Error; err != nil {
		return models.Session{}, fmt.Errorf("create session row: %w", err)
	}
	return s, nil
}

// ValidateSession returns the User associated with token, or
// (zero, ErrSessionNotFound/ErrSessionExpired) if the token is unknown
// or stale. We delete the row in the expired case so a request with a
// known-bad token does not pollute the sessions table forever.
func ValidateSession(db *gorm.DB, token string) (models.User, error) {
	if token == "" {
		return models.User{}, ErrSessionNotFound
	}
	var s models.Session
	if err := db.Where("token = ?", token).First(&s).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.User{}, ErrSessionNotFound
		}
		return models.User{}, fmt.Errorf("lookup session: %w", err)
	}
	now := time.Now().UTC()
	if !s.ExpiresAt.IsZero() && !s.ExpiresAt.After(now) {
		// Best-effort cleanup; ignore the error because the auth decision
		// (reject) is already determined.
		_ = db.Delete(&s).Error
		return models.User{}, ErrSessionExpired
	}
	var u models.User
	if err := db.First(&u, s.UserID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// The session points at a user that no longer exists. Treat
			// the same as an unknown token rather than 500-ing.
			return models.User{}, ErrSessionNotFound
		}
		return models.User{}, fmt.Errorf("lookup user for session: %w", err)
	}
	return u, nil
}

// DeleteSession removes the session row matching token. A missing token
// is not an error — logout is idempotent on the server side, so a stale
// cookie that points at a deleted session should still produce a 204.
func DeleteSession(db *gorm.DB, token string) error {
	if token == "" {
		return nil
	}
	if err := db.Where("token = ?", token).Delete(&models.Session{}).Error; err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}
