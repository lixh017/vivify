package agents

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/opc/api/internal/models"
	"gorm.io/gorm"
)

// ProviderResolver maps (user_id, scope) → a live TextProvider
// or MediaProvider by reading the credential table, decrypting
// the key, and wiring the right protocol adapter. The resolver
// is safe for concurrent use after construction; the underlying
// *gorm.DB is the source of truth and is read on every call so
// key rotations land on the next request without a restart.
type ProviderResolver struct {
	db           *gorm.DB
	decrypt      func([]byte) ([]byte, error)
	defaultMedia MediaProvider // MiniMax (built-in, env-driven)

	// clients memoizes adapters by (protocol, baseURL, credentialID, updatedAt).
	// The UpdatedAt suffix ensures a key rotation (which writes a new
	// UpdatedAt via Save) evicts the cache on the next call, so the new
	// API key is honored immediately without a process restart. Memory
	// grows by one entry per rotation; for Phase 4's small cardinality
	// (a handful of providers per user) this is acceptable.
	mu      sync.Mutex
	clients map[string]TextProvider
}

// NewProviderResolver wires a resolver. decrypt is the
// credentials column decryptor (auth.Decrypt with the master
// key); pass a stub in tests.
func NewProviderResolver(db *gorm.DB, decrypt func([]byte) ([]byte, error), defaultMedia MediaProvider) *ProviderResolver {
	return &ProviderResolver{
		db:           db,
		decrypt:      decrypt,
		defaultMedia: defaultMedia,
		clients:      make(map[string]TextProvider),
	}
}

// Text returns a TextProvider for the given (userID, scope).
// scope can be "all" or a specific skill name; the resolver
// prefers an exact match and falls back to "all".
func (r *ProviderResolver) Text(ctx context.Context, userID uint, scope string) (TextProvider, error) {
	if userID == 0 {
		return Echo, nil
	}
	cred, err := r.lookupCredential(ctx, userID, scope)
	if err != nil {
		return nil, err
	}
	if cred == nil {
		return Echo, nil
	}
	key, err := r.decrypt(cred.EncryptedKey)
	if err != nil {
		return nil, fmt.Errorf("resolver: decrypt: %w", err)
	}
	switch models.Protocol(cred.Protocol) {
	case models.ProtocolAnthropic:
		return r.cachedClient(r.cacheKey(cred, "anthropic"), func() TextProvider {
			return NewAnthropicCompatProvider(AnthropicCompatConfig{
				APIKey:    string(key),
				BaseURL:   cred.BaseURL,
				ModelName: cred.ModelName,
			})
		}), nil
	case models.ProtocolOpenAI:
		return r.cachedClient(r.cacheKey(cred, "openai"), func() TextProvider {
			return NewOpenAICompatProvider(OpenAICompatConfig{
				APIKey:    string(key),
				BaseURL:   cred.BaseURL,
				ModelName: cred.ModelName,
			})
		}), nil
	default:
		return nil, fmt.Errorf("resolver: unsupported protocol %q for credential %d", cred.Protocol, cred.ID)
	}
}

// Media returns the default MediaProvider. User-configurable
// media is out of scope for Phase 4 — MiniMax remains the
// built-in. The signature is here so handlers can adopt a
// uniform resolver surface.
func (r *ProviderResolver) Media(_ context.Context, _ uint, _ string) (MediaProvider, error) { //nolint:unused // Phase 4 placeholder; signature kept for uniform resolver surface
	if r.defaultMedia == nil {
		return nil, errors.New("resolver: no default media provider configured")
	}
	return r.defaultMedia, nil
}

func (r *ProviderResolver) lookupCredential(ctx context.Context, userID uint, scope string) (*models.Credential, error) {
	var cred models.Credential
	// Exact-scope first.
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND scope = ?", userID, scope).
		Order("updated_at DESC").
		First(&cred).Error
	if err == nil {
		return &cred, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("resolver: lookup: %w", err)
	}
	// Fall back to scope="all".
	err = r.db.WithContext(ctx).
		Where("user_id = ? AND scope = ?", userID, "all").
		Order("updated_at DESC").
		First(&cred).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("resolver: lookup all: %w", err)
	}
	return &cred, nil
}

func (r *ProviderResolver) cachedClient(key string, build func() TextProvider) TextProvider {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.clients[key]; ok {
		return c
	}
	c := build()
	r.clients[key] = c
	return c
}

// cacheKey derives a unique key per (protocol, baseURL, credentialID,
// updatedAt) so a key rotation (which writes a new UpdatedAt) evicts
// the cached provider on the next call. Including UpdatedAt rather
// than just ID is required because the rotate endpoint Save()s the
// row in place — the ID stays the same, but UpdatedAt advances.
func (r *ProviderResolver) cacheKey(cred *models.Credential, protocol string) string {
	return fmt.Sprintf("%s:%s:%d:%d", protocol, cred.BaseURL, cred.ID, cred.UpdatedAt.UnixNano())
}
