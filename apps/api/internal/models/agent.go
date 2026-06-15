// Package models provides the GORM data models for the OPC
// platform. agent.go represents an API key holder (a non-human
// agent representing a 创作者 or OPC team).
package models

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Agent represents an API key holder. The full key is shown
// ONCE at creation; only the bcrypt hash is stored long-term.
type Agent struct {
	gorm.Model

	Name       string `gorm:"not null" json:"name"`
	KeyPrefix  string `gorm:"not null" json:"key_prefix"`
	HashedKey  string `gorm:"not null;uniqueIndex" json:"-"`
	CreatedBy  uint   `gorm:"not null" json:"created_by"`
	Scope      string `gorm:"default:all;not null" json:"scope"`
	Disabled   bool   `gorm:"default:false;not null" json:"disabled"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

// keyPrefixLen is the number of leading chars we keep as
// KeyPrefix for operator display. 12 chars of "opc_agent_XX"
// is enough to identify a key without leaking the secret.
const keyPrefixLen = 12

// GenerateKey returns a fresh "opc_agent_<random32>" key plus
// its 12-char prefix. Uses crypto/rand for entropy.
func GenerateKey() (key string, prefix string, err error) {
	bytes := make([]byte, 16) // 16 bytes = 32 hex chars
	if _, err := rand.Read(bytes); err != nil {
		return "", "", err
	}
	key = "opc_agent_" + hex.EncodeToString(bytes)
	return key, key[:keyPrefixLen], nil
}

// KeyPrefix returns the first 12 chars of a key. Used for
// operator display in console + MaybeAgentKey prefix lookup.
func KeyPrefix(key string) string {
	if len(key) < keyPrefixLen {
		return key
	}
	return key[:keyPrefixLen]
}

// HashPassword bcrypts a raw key for storage. Uses
// bcrypt.DefaultCost (12) to match User.PasswordHash cost.
func HashPassword(key string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(key), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifyPassword compares a raw key against the stored bcrypt hash.
// Returns true on match, false otherwise.
func VerifyPassword(hashedKey, rawKey string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hashedKey), []byte(rawKey)) == nil
}
