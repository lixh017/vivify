package models

import "time"

// User represents a single person who can sign in to the OPC web UI. Phase 1
// was single-tenant (no users); Phase 2 introduces multi-tenancy, which
// requires per-user authentication. Passwords are stored as a bcrypt hash;
// the plaintext never reaches disk. The JSON contract deliberately omits the
// hash (`json:"-"`) so the field can never leak through a marshalled
// response by accident.
type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Email        string    `gorm:"uniqueIndex" json:"email"`
	PasswordHash string    `json:"-"` // never expose
	Name         string    `json:"name"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TableName pins the underlying SQLite table name so the model is
// unambiguous in logs, migrations, and the migrate tests' table list.
func (User) TableName() string { return "users" }
