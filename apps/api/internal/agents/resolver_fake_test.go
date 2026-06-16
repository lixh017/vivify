package agents

import (
	"context"

	"github.com/opc/api/internal/models"
)

type testResolver struct {
	db *fakeCredentialDB
}

func (r *testResolver) text(ctx context.Context, userID uint, scope string) (TextProvider, error) {
	row, ok, err := r.db.newestCredentialForScope(ctx, userID, scope)
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
	return Echo, nil
}

// newResolverForTest wires a resolver that reads from the
// supplied fakeCredentialDB (instead of *gorm.DB). Used only
// by resolver_test.go. The fake adapter is wired in test code.
func newResolverForTest(db *fakeCredentialDB) *testResolver {
	return &testResolver{db: db}
}
