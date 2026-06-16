package agents

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/opc/api/internal/auth"
	"github.com/opc/api/internal/models"
)

// TestProviderResolver_RealDB_PicksAnthropic is a one-off
// integration check: insert a credential via real GORM, call
// Resolver.Text, and assert the returned adapter's Name().
// Runs against an in-memory SQLite (":memory:") so it is
// hermetic and fast.
//
// Context: 2026-06-16 the api-smoke / verify-phase4 scripts
// stamped call_log.provider = "minimax" even though the
// user had an openai credential row. The fake-DB unit tests
// passed, so the bug was in the real-GORM lookup path. This
// test runs the real path and surfaces it.
func TestProviderResolver_RealDB_PicksAnthropic(t *testing.T) {
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	if err := gormDB.AutoMigrate(&models.Credential{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	const plaintextKey = "sk-fake-key-1234567890"
	// 32 bytes exactly. auth.Encrypt rejects < 32 with
	// "ENCRYPTION_KEY must decode to at least 32 bytes";
	// 30-byte constants like "test-encryption-key-32-bytes!!"
	// silently fail this test and waste 10 minutes of
	// debugging (as the auth_test.go recovery work caught).
	const testKey = "test-key-padded-out-to-32-bytes!"
	cipher, err := auth.Encrypt([]byte(plaintextKey), []byte(testKey))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	cred := models.Credential{
		UserID:       1,
		Provider:     "anthropic",
		Protocol:     "anthropic",
		BaseURL:      "https://example.invalid",
		ModelName:    "claude-test",
		Name:         "test-cred",
		EncryptedKey: cipher,
		Scope:        "all",
	}
	if err := gormDB.Create(&cred).Error; err != nil {
		t.Fatalf("insert cred: %v", err)
	}

	// Verify the row is actually readable with the exact
	// query the resolver makes. If this fails, the resolver
	// test is meaningless because we're testing a query
	// that returns nothing.
	var readBack models.Credential
	if err := gormDB.Where("user_id = ? AND scope = ?", uint(1), "all").
		Order("updated_at DESC").First(&readBack).Error; err != nil {
		t.Fatalf("readback: %v", err)
	}
	if readBack.Protocol != "anthropic" {
		t.Fatalf("readback protocol = %q, want anthropic", readBack.Protocol)
	}

	resolver := NewProviderResolver(gormDB, func(c []byte) ([]byte, error) {
		return auth.Decrypt(c, []byte(testKey))
	}, nil)

	p, err := resolver.Text(context.Background(), 1, "all")
	if err != nil {
		t.Fatalf("resolver.Text: %v", err)
	}
	if p == nil {
		t.Fatal("resolver returned nil provider")
	}
	if p.Name() != "anthropic" {
		t.Errorf("want adapter=anthropic, got %q", p.Name())
	}
}
