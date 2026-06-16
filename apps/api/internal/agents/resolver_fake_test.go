package agents

import (
	"context"
	"fmt"

	"github.com/opc/api/internal/models"
)

// adapterForScope picks the right protocol adapter for (userID, scope)
// using the in-memory fakeCredentialDB. It mirrors the production
// ProviderResolver.Text adapter-selection logic but does NOT exercise
// the cache, decrypt, or DB code paths — those are covered by the
// production code itself when wired to a real *gorm.DB.
//
// Returns the provider, or (nil, error) for unsupported protocols.
func (f *fakeCredentialDB) adapterForScope(ctx context.Context, userID uint, scope string) (TextProvider, error) {
	row, ok, err := f.newestCredentialForScope(ctx, userID, scope)
	if err != nil {
		return nil, err
	}
	if !ok {
		return Echo, nil
	}
	switch models.Protocol(row.Protocol) {
	case models.ProtocolAnthropic:
		return NewAnthropicCompatProvider(AnthropicCompatConfig{
			APIKey:    string(row.APIKey),
			BaseURL:   row.BaseURL,
			ModelName: row.ModelName,
		}), nil
	case models.ProtocolOpenAI:
		return NewOpenAICompatProvider(OpenAICompatConfig{
			APIKey:    string(row.APIKey),
			BaseURL:   row.BaseURL,
			ModelName: row.ModelName,
		}), nil
	}
	return nil, fmt.Errorf("test resolver: unsupported protocol %q", row.Protocol)
}

// newResolverForTest wires a thin test helper around fakeCredentialDB.
// It is intentionally NOT a *ProviderResolver — it does not exercise
// the cache, decrypt, or DB code paths. The production code's
// *ProviderResolver is tested separately against a real *gorm.DB
// in the handlers package.
type testResolver struct{ db *fakeCredentialDB }

func (r *testResolver) text(ctx context.Context, userID uint, scope string) (TextProvider, error) {
	return r.db.adapterForScope(ctx, userID, scope)
}

func newResolverForTest(db *fakeCredentialDB) *testResolver {
	return &testResolver{db: db}
}
