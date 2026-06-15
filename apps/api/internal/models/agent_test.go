package models_test

import (
	"strings"
	"testing"

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
