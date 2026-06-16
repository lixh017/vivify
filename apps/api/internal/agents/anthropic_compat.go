package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	base := cfg.BaseURL
	if base == "" {
		base = "https://api.anthropic.com"
	}
	_ = base
	return &AnthropicCompatProvider{
		cfg:  cfg,
		http: &http.Client{Timeout: DefaultProviderTimeout},
	}
}

func (p *AnthropicCompatProvider) Name() string    { return "anthropic" }
func (p *AnthropicCompatProvider) Available() bool { return strings.TrimSpace(p.cfg.APIKey) != "" }

type anthropicCompatRequest struct {
	Model     string `json:"model"`
	MaxTokens int    `json:"max_tokens,omitempty"`
	System    string `json:"system,omitempty"`
	Messages  []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

func (p *AnthropicCompatProvider) CompleteWithUsage(ctx context.Context, prompt string, opts CompleteOptions) (string, Usage, error) {
	if !p.Available() {
		return "", Usage{}, errors.New("anthropic-compat: APIKey not configured")
	}
	model := opts.Model
	if model == "" {
		model = p.cfg.ModelName
	}
	if model == "" {
		return "", Usage{}, errors.New("anthropic-compat: model name not configured")
	}
	req := anthropicCompatRequest{
		Model:     model,
		MaxTokens: int(opts.MaxTokens),
		System:    "You are a helpful AI assistant.",
	}
	req.Messages = append(req.Messages, struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}{Role: "user", Content: prompt})
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(p.cfg.BaseURL, "/")+"/v1/messages", bytes.NewReader(body))
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
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", Usage{}, fmt.Errorf("anthropic-compat: read: %w", err)
	}
	if resp.StatusCode >= 400 {
		return "", Usage{}, fmt.Errorf("anthropic-compat: api %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	var env struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		BaseResp struct {
			StatusCode int    `json:"status_code"`
			StatusMsg  string `json:"status_msg"`
		} `json:"base_resp"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", Usage{}, fmt.Errorf("anthropic-compat: parse: %w (raw=%s)", err, truncate(string(raw), 200))
	}
	if env.BaseResp.StatusCode != 0 {
		return "", Usage{}, fmt.Errorf("anthropic-compat: api error %d: %s",
			env.BaseResp.StatusCode, env.BaseResp.StatusMsg)
	}
	var sb strings.Builder
	for _, b := range env.Content {
		if b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}
	return sb.String(), Usage{InputTokens: env.Usage.InputTokens, OutputTokens: env.Usage.OutputTokens}, nil
}

// Compile-time check that AnthropicCompatProvider satisfies TextProvider.
var _ TextProvider = (*AnthropicCompatProvider)(nil)
