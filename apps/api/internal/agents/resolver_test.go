package agents

import (
	"context"
	"sync"
	"testing"
)

// fakeCredentialDB is a minimal in-memory stand-in for *gorm.DB.
// It only implements what the resolver needs: reading the
// "newest credential" for a (userID, scope) pair. A real test
// against *gorm.DB lives in handlers/credentials_test.go; this
// unit test pins the resolver's adapter-selection logic.
type fakeCredentialDB struct {
	mu   sync.Mutex
	rows map[uint]map[string]resolvedCredential // userID → scope → row
}

type resolvedCredential struct {
	Protocol  string
	BaseURL   string
	ModelName string
	APIKey    []byte // plaintext (resolver will not decrypt here — it's a unit test)
}

func (f *fakeCredentialDB) newestCredentialForScope(_ context.Context, userID uint, scope string) (*resolvedCredential, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	byScope, ok := f.rows[userID]
	if !ok {
		return nil, false, nil
	}
	// First check exact scope, then fall back to "all".
	if r, ok := byScope[scope]; ok {
		return &r, true, nil
	}
	if r, ok := byScope["all"]; ok {
		return &r, true, nil
	}
	return nil, false, nil
}

func TestProviderResolver_PicksAnthropicAdapter(t *testing.T) {
	db := &fakeCredentialDB{
		rows: map[uint]map[string]resolvedCredential{
			7: {"all": {Protocol: "anthropic", BaseURL: "https://example.com", ModelName: "claude-haiku-4-5", APIKey: []byte("sk")}},
		},
	}
	resolver := newResolverForTest(db)

	p, err := resolver.text(context.Background(), 7, "all")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if p.Name() != "anthropic" {
		t.Errorf("want adapter=anthropic, got %q", p.Name())
	}
	if !p.Available() {
		t.Error("want Available()=true")
	}
}

func TestProviderResolver_PicksOpenAIAdapter(t *testing.T) {
	db := &fakeCredentialDB{
		rows: map[uint]map[string]resolvedCredential{
			7: {"all": {Protocol: "openai", BaseURL: "https://api.deepseek.com", ModelName: "deepseek-chat", APIKey: []byte("sk")}},
		},
	}
	resolver := newResolverForTest(db)
	p, err := resolver.text(context.Background(), 7, "all")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if p.Name() != "openai" {
		t.Errorf("want adapter=openai, got %q", p.Name())
	}
}

func TestProviderResolver_NoCredential_ReturnsEcho(t *testing.T) {
	db := &fakeCredentialDB{rows: map[uint]map[string]resolvedCredential{}}
	resolver := newResolverForTest(db)
	p, err := resolver.text(context.Background(), 99, "all")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if p.Name() != "echo" {
		t.Errorf("want fallback=echo, got %q", p.Name())
	}
}
