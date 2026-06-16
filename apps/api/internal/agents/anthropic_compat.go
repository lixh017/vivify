package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// AnthropicCompatConfig is the user-tunable shape of an
// Anthropic-protocol endpoint. ModelName is required; BaseURL
// defaults to api.anthropic.com; APIKey is the user-supplied
// secret.
type AnthropicCompatConfig struct {
	APIKey    string
	BaseURL   string
	ModelName string
}

// AnthropicCompatProvider is the protocol adapter that hits any
// endpoint speaking the Anthropic Messages API shape. It
// implements TextProvider. Both api.anthropic.com (real Claude)
// and MiniMax's /anthropic/v1/messages endpoint are reachable
// from the same code — only the BaseURL differs.
type AnthropicCompatProvider struct {
	cfg  AnthropicCompatConfig
	http *http.Client
}

// NewAnthropicCompatProvider constructs a provider. Missing
// APIKey is OK — Available() reports false and callers fall back
// to demo mode.
func NewAnthropicCompatProvider(cfg AnthropicCompatConfig) *AnthropicCompatProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.anthropic.com"
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &AnthropicCompatProvider{
		cfg:  cfg,
		http: &http.Client{Timeout: DefaultProviderTimeout},
	}
}

func (p *AnthropicCompatProvider) Name() string    { return "anthropic" }
func (p *AnthropicCompatProvider) Available() bool { return strings.TrimSpace(p.cfg.APIKey) != "" }

type anthropicCompatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicCompatRequest struct {
	Model     string                   `json:"model"`
	MaxTokens int                      `json:"max_tokens,omitempty"`
	System    string                   `json:"system,omitempty"`
	Messages  []anthropicCompatMessage `json:"messages"`
}

func (p *AnthropicCompatProvider) CompleteWithUsage(ctx context.Context, prompt string, opts CompleteOptions) (string, Usage, error) {
	if !p.Available() {
		return "", Usage{}, fmt.Errorf("anthropic-compat: %w", ErrProviderUnavailable)
	}
	model := opts.Model
	if model == "" {
		model = p.cfg.ModelName
	}
	if model == "" {
		return "", Usage{}, fmt.Errorf("anthropic-compat: %w", ErrProviderUnavailable)
	}
	timeout := DefaultProviderTimeout
	if opts.Timeout > 0 {
		timeout = opts.Timeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req := anthropicCompatRequest{
		Model:     model,
		MaxTokens: int(opts.MaxTokens),
		System:    "You are a helpful AI assistant.",
	}
	req.Messages = []anthropicCompatMessage{{Role: "user", Content: prompt}}
	body, err := json.Marshal(req)
	if err != nil {
		return "", Usage{}, fmt.Errorf("anthropic-compat: marshal: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(cctx, http.MethodPost,
		p.cfg.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", Usage{}, fmt.Errorf("anthropic-compat: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.cfg.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.http.Do(httpReq)
	if err != nil {
		return "", Usage{}, fmt.Errorf("anthropic-compat: http: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", Usage{}, fmt.Errorf("anthropic-compat: read: %w", err)
	}
	if resp.StatusCode >= 400 {
		return "", Usage{}, fmt.Errorf("anthropic-compat: api %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	result, err := parseAnthropicMessagesResponse(raw)
	if err != nil {
		return "", Usage{}, fmt.Errorf("anthropic-compat: %w", err)
	}
	return result.Text, Usage{InputTokens: result.InputTokens, OutputTokens: result.OutputTokens}, nil
}

// Compile-time check that AnthropicCompatProvider satisfies TextProvider.
var _ TextProvider = (*AnthropicCompatProvider)(nil)
