package models_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/opc/api/internal/models"
)

func TestAgentGenerateKeyFormat(t *testing.T) {
	t.Parallel()

	key, prefix, err := models.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey error: %v", err)
	}
	if !strings.HasPrefix(key, "opc_agent_") {
		t.Errorf("key = %q, want opc_agent_ prefix", key)
	}
	if len(key) != len("opc_agent_")+32 {
		t.Errorf("key length = %d, want %d", len(key), len("opc_agent_")+32)
	}
	if prefix != key[:12] {
		t.Errorf("prefix = %q, want first 12 chars %q", prefix, key[:12])
	}
}

func TestAgentGenerateKeyUnique(t *testing.T) {
	t.Parallel()

	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		key, _, _ := models.GenerateKey()
		if seen[key] {
			t.Fatalf("duplicate key generated: %s", key)
		}
		seen[key] = true
	}
}

func TestAgentHashAndVerify(t *testing.T) {
	t.Parallel()

	key, _, _ := models.GenerateKey()
	hash, err := models.HashPassword(key)
	if err != nil {
		t.Fatalf("HashPassword error: %v", err)
	}
	if !models.VerifyPassword(hash, key) {
		t.Error("VerifyPassword(hash, original) returned false, want true")
	}
	if models.VerifyPassword(hash, "opc_agent_wrong_key_1234567890123456") {
		t.Error("VerifyPassword(hash, wrong) returned true, want false")
	}
}

func TestAgentKeyPrefix(t *testing.T) {
	t.Parallel()

	if got := models.KeyPrefix("opc_agent_abcdef123456"); got != "opc_agent_ab" {
		t.Errorf("prefix = %q, want opc_agent_ab", got)
	}
}

func TestAgentKeyPrefixShortKey(t *testing.T) {
	t.Parallel()

	if got := models.KeyPrefix("short"); got != "short" {
		t.Errorf("short key prefix = %q, want short (no truncation)", got)
	}
}

// TestAgentJSONShape pins the JSON serialization of models.Agent.
// The console /api-keys page reads a.id (lowercase) — without
// this contract a regression to embedding gorm.Model (which has
// no JSON tags) would silently break the UI even though every
// unit test on the handler passes. Caught by api-smoke.sh on
// 2026-06-15 (Task 5); kept here as a fast feedback loop.
func TestAgentJSONShape(t *testing.T) {
	t.Parallel()

	now := time.Now()
	a := models.Agent{
		ID:         42,
		CreatedAt:  now,
		UpdatedAt:  now,
		Name:       "claude-code-laptop-1",
		KeyPrefix:  "opc_agent_ab",
		HashedKey:  "$2a$12$shouldneverappear",
		CreatedBy:  7,
		Scope:      "all",
		Disabled:   false,
		LastUsedAt: nil,
	}
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, k := range []string{"id", "name", "key_prefix", "created_by", "scope", "disabled", "created_at", "updated_at"} {
		if _, ok := got[k]; !ok {
			t.Errorf("JSON missing key %q (full=%s)", k, string(b))
		}
	}
	if _, leaked := got["ID"]; leaked {
		t.Errorf("JSON leaked uppercase %q — Agent must use explicit json tags, not gorm.Model (full=%s)", "ID", string(b))
	}
	if _, leaked := got["hashed_key"]; leaked {
		t.Errorf("JSON leaked %q — bcrypt hash must stay private (full=%s)", "hashed_key", string(b))
	}
}
