package models

import "time"

// Session represents a server-side login session. The opaque Token is the
// only value the client ever sees (and what we'll put in the auth cookie);
// the row itself is what makes the session revocable and gives us a single
// place to enforce an expiry. We do not store anything user-identifying
// beyond UserID; in Phase 2 the user-facing profile (name/email) is fetched
// separately via ValidateSession.
type Session struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index" json:"user_id"`
	Token     string    `gorm:"uniqueIndex" json:"token"`
	ExpiresAt time.Time `gorm:"index" json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName pins the underlying SQLite table name for consistency with the
// other models in this package.
func (Session) TableName() string { return "sessions" }
