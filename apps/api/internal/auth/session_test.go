package auth

import (
	"errors"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/opc/api/internal/models"
)

// newTestDB returns a GORM connection with all the tables the auth
// package touches migrated in. We intentionally reuse the in-memory
// shared-cache pattern from the db package's own tests so concurrent
// test runs don't fight over a file lock.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	if err := gormDB.AutoMigrate(&models.User{}, &models.Session{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return gormDB
}

// seedUser inserts a fresh user with the given email and returns it.
// We don't go through HashPassword here because the session tests are
// about sessions, not passwords; the password_hash is just a
// placeholder string the rest of the suite doesn't care about.
func seedUser(t *testing.T, db *gorm.DB, email string) models.User {
	t.Helper()
	u := models.User{Email: email, PasswordHash: "stub", Name: email}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return u
}

// TestCreateSessionReturnsPersistedRow is the smoke test: create a
// session, read it back, and assert the row matches the returned value
// and the token is a 64-char hex string (32 random bytes encoded).
func TestCreateSessionReturnsPersistedRow(t *testing.T) {
	db := newTestDB(t)
	u := seedUser(t, db, "alice@example.com")

	s, err := CreateSession(db, u.ID)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if s.ID == 0 {
		t.Errorf("session ID is zero")
	}
	if s.UserID != u.ID {
		t.Errorf("session.UserID = %d, want %d", s.UserID, u.ID)
	}
	if len(s.Token) != 2*tokenBytes {
		t.Errorf("token length = %d, want %d (hex of %d bytes)", len(s.Token), 2*tokenBytes, tokenBytes)
	}
	for _, r := range s.Token {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			t.Errorf("token contains non-hex char %q in %q", r, s.Token)
			break
		}
	}

	// Round-trip: read it back.
	var got models.Session
	if err := db.First(&got, s.ID).Error; err != nil {
		t.Fatalf("read back session: %v", err)
	}
	if got.Token != s.Token {
		t.Errorf("persisted token %q != returned %q", got.Token, s.Token)
	}

	// Expiry is ~7 days out from creation. We allow a 1s skew to absorb
	// any time.Now() drift between CreateSession and the test's clock.
	want := s.CreatedAt.Add(sessionTTL)
	delta := got.ExpiresAt.Sub(want)
	if delta < -time.Second || delta > time.Second {
		t.Errorf("ExpiresAt drift = %v; want within 1s of %v", delta, want)
	}
}

// TestValidateSessionHappyPath ensures a freshly created session round
// trips back to the same user.
func TestValidateSessionHappyPath(t *testing.T) {
	db := newTestDB(t)
	u := seedUser(t, db, "bob@example.com")
	s, err := CreateSession(db, u.ID)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	got, err := ValidateSession(db, s.Token)
	if err != nil {
		t.Fatalf("ValidateSession: %v", err)
	}
	if got.ID != u.ID {
		t.Errorf("ValidateSession returned user ID %d, want %d", got.ID, u.ID)
	}
	if got.Email != u.Email {
		t.Errorf("ValidateSession returned email %q, want %q", got.Email, u.Email)
	}
}

// TestValidateSessionUnknownToken covers the not-found case. Empty
// tokens short-circuit to ErrSessionNotFound without hitting the DB;
// a random hex string is the "valid shape, no row" case.
func TestValidateSessionUnknownToken(t *testing.T) {
	db := newTestDB(t)

	t.Run("empty", func(t *testing.T) {
		_, err := ValidateSession(db, "")
		if !errors.Is(err, ErrSessionNotFound) {
			t.Errorf("empty token err = %v, want ErrSessionNotFound", err)
		}
	})
	t.Run("random", func(t *testing.T) {
		_, err := ValidateSession(db, "deadbeef")
		if !errors.Is(err, ErrSessionNotFound) {
			t.Errorf("random token err = %v, want ErrSessionNotFound", err)
		}
	})
}

// TestValidateSessionExpired is the negative case the spec calls out:
// backdate ExpiresAt, expect ErrSessionExpired, and confirm the row
// was deleted (so the next call also returns ErrSessionNotFound).
func TestValidateSessionExpired(t *testing.T) {
	db := newTestDB(t)
	u := seedUser(t, db, "carol@example.com")
	s, err := CreateSession(db, u.ID)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Backdate ExpiresAt to one minute ago. We update directly to avoid
	// exporting a "force expire" knob from the production API.
	if err := db.Model(&s).Update("expires_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatalf("backdate expires_at: %v", err)
	}

	_, err = ValidateSession(db, s.Token)
	if !errors.Is(err, ErrSessionExpired) {
		t.Errorf("ValidateSession err = %v, want ErrSessionExpired", err)
	}

	// Second call should now be "not found" because the expired row
	// was deleted as a side effect.
	if _, err := ValidateSession(db, s.Token); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("second ValidateSession err = %v, want ErrSessionNotFound", err)
	}

	// And the row really is gone from the table.
	var count int64
	if err := db.Model(&models.Session{}).Where("token = ?", s.Token).Count(&count).Error; err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if count != 0 {
		t.Errorf("expired session row still present (count=%d)", count)
	}
}

// TestDeleteSessionRemovesRow is the logout case. We delete, then
// re-validate, and expect ErrSessionNotFound.
func TestDeleteSessionRemovesRow(t *testing.T) {
	db := newTestDB(t)
	u := seedUser(t, db, "dave@example.com")
	s, err := CreateSession(db, u.ID)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if err := DeleteSession(db, s.Token); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := ValidateSession(db, s.Token); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("ValidateSession after delete err = %v, want ErrSessionNotFound", err)
	}
}

// TestDeleteSessionIdempotent: deleting a non-existent token must be
// a no-op, not an error. Empty token is also a no-op. Logout is
// supposed to succeed even when the cookie is already stale.
func TestDeleteSessionIdempotent(t *testing.T) {
	db := newTestDB(t)

	t.Run("missing", func(t *testing.T) {
		if err := DeleteSession(db, "no-such-token"); err != nil {
			t.Errorf("DeleteSession on missing token: %v", err)
		}
	})
	t.Run("empty", func(t *testing.T) {
		if err := DeleteSession(db, ""); err != nil {
			t.Errorf("DeleteSession on empty token: %v", err)
		}
	})
}

// TestSessionTokenUniquenessAcrossUsers makes sure the random token
// generator never collides within a realistic batch. 100 iterations is
// overkill for 256-bit tokens but keeps the test small while still
// failing loudly if a future regression makes the RNG deterministic.
func TestSessionTokenUniquenessAcrossUsers(t *testing.T) {
	db := newTestDB(t)
	u := seedUser(t, db, "eve@example.com")

	seen := make(map[string]struct{}, 100)
	for i := 0; i < 100; i++ {
		s, err := CreateSession(db, u.ID)
		if err != nil {
			t.Fatalf("CreateSession iter %d: %v", i, err)
		}
		if _, dup := seen[s.Token]; dup {
			t.Fatalf("token collision on iter %d: %s", i, s.Token)
		}
		seen[s.Token] = struct{}{}
	}
}
