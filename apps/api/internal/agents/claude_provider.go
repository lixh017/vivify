package agents

import (
	"context"
	"fmt"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// ClaudeProvider adapts the Anthropic SDK to the Provider interface.
// It is intentionally separate from the existing *Claude type so the
// migration to multi-model routing can land without rewriting every
// call site at once: the legacy *Claude continues to expose prompt
// builders + Complete; the new ClaudeProvider lives behind the
// Router for per-task model selection.
type ClaudeProvider struct {
	client       anthropic.Client
	apiKey       string
	defaultModel string
	override     CompleteFunc // test-only hook; nil in production
}

// NewClaudeProvider constructs a ClaudeProvider from an API key. An
// empty key is allowed (dev mode); Available() then returns false
// and Complete returns ErrProviderUnavailable so the router can fall
// back to demo data.
func NewClaudeProvider(apiKey string) *ClaudeProvider {
	p := &ClaudeProvider{
		apiKey:       apiKey,
		defaultModel: DefaultModel,
	}
	if apiKey != "" {
		p.client = anthropic.NewClient(option.WithAPIKey(apiKey))
	}
	return p
}

// NewClaudeProviderWithOverride is the test seam: it lets unit
// tests inject a fake completion function so they don't have to
// stand up a fake HTTP server for the Anthropic SDK. Production code
// must never set this. The override bypasses the API-key check so
// tests don't have to plumb a fake key through.
func NewClaudeProviderWithOverride(fn CompleteFunc) *ClaudeProvider {
	return &ClaudeProvider{
		apiKey:       "test-override",
		defaultModel: DefaultModel,
		override:     fn,
	}
}

// Name implements Provider.
func (p *ClaudeProvider) Name() string { return ProviderClaude }

// Available implements Provider.
func (p *ClaudeProvider) Available() bool { return p.apiKey != "" }

// Complete implements Provider. Honours opts.Model (if non-empty),
// opts.MaxTokens (if > 0), and opts.Timeout (if > 0); otherwise
// falls back to package defaults.
func (p *ClaudeProvider) Complete(ctx context.Context, prompt string, opts CompleteOptions) (string, error) {
	if p.override != nil {
		return p.override(ctx, prompt)
	}
	if !p.Available() {
		return "", fmt.Errorf("claude provider: %w", ErrProviderUnavailable)
	}
	model := p.defaultModel
	if opts.Model != "" {
		model = opts.Model
	}
	maxTokens := int64(DefaultMaxTokens)
	if opts.MaxTokens > 0 {
		maxTokens = opts.MaxTokens
	}
	timeout := DefaultProviderTimeout
	if opts.Timeout > 0 {
		timeout = opts.Timeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	resp, err := p.client.Messages.New(cctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: maxTokens,
		Messages: []anthropic.MessageParam{
			{
				Role: anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{
					anthropic.NewTextBlock(prompt),
				},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("claude provider complete: %w", err)
	}
	if len(resp.Content) == 0 {
		return "", fmt.Errorf("claude provider complete: %w", ErrNoContent)
	}
	return resp.Content[0].Text, nil
}
