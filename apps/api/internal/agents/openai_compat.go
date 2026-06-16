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

// OpenAICompatConfig is the user-tunable shape of an
// OpenAI-Chat-Completions-protocol endpoint. ModelName is
// required; BaseURL defaults to api.openai.com; APIKey is the
// user-supplied secret.
type OpenAICompatConfig struct {
	APIKey    string
	BaseURL   string
	ModelName string
}

// OpenAICompatProvider is the protocol adapter that hits any
// endpoint speaking the OpenAI Chat Completions API shape. It
// implements TextProvider. api.openai.com, DeepSeek, Azure
// OpenAI, and most Chinese open-source gateways are reachable
// from the same code — only the BaseURL differs.
type OpenAICompatProvider struct {
	cfg  OpenAICompatConfig
	http *http.Client
}

// NewOpenAICompatProvider constructs a provider. Missing
// APIKey is OK — Available() reports false and callers fall
// back to demo mode.
func NewOpenAICompatProvider(cfg OpenAICompatConfig) *OpenAICompatProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com"
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &OpenAICompatProvider{
		cfg:  cfg,
		http: &http.Client{Timeout: DefaultProviderTimeout},
	}
}

func (p *OpenAICompatProvider) Name() string    { return "openai" }
func (p *OpenAICompatProvider) Available() bool { return strings.TrimSpace(p.cfg.APIKey) != "" }

type openaiCompatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiCompatRequest struct {
	Model     string                `json:"model"`
	MaxTokens int                   `json:"max_tokens,omitempty"`
	Messages  []openaiCompatMessage `json:"messages"`
}

func (p *OpenAICompatProvider) CompleteWithUsage(ctx context.Context, prompt string, opts CompleteOptions) (string, Usage, error) {
	if !p.Available() {
		return "", Usage{}, fmt.Errorf("openai-compat: %w", ErrProviderUnavailable)
	}
	model := opts.Model
	if model == "" {
		model = p.cfg.ModelName
	}
	if model == "" {
		return "", Usage{}, fmt.Errorf("openai-compat: %w", ErrProviderUnavailable)
	}
	timeout := DefaultProviderTimeout
	if opts.Timeout > 0 {
		timeout = opts.Timeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req := openaiCompatRequest{
		Model:     model,
		MaxTokens: int(opts.MaxTokens),
		Messages:  []openaiCompatMessage{{Role: "user", Content: prompt}},
	}
	body, err := json.Marshal(req)
	if err != nil {
		return "", Usage{}, fmt.Errorf("openai-compat: marshal: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(cctx, http.MethodPost,
		p.cfg.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", Usage{}, fmt.Errorf("openai-compat: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

	resp, err := p.http.Do(httpReq)
	if err != nil {
		return "", Usage{}, fmt.Errorf("openai-compat: http: %w", err)
	}
	defer resp.Body.Close()
	// Soft cap at 4 MiB to match deepseek_provider.go:159 — well
	// above any reasonable single completion, but prevents OOM
	// if the upstream misbehaves.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", Usage{}, fmt.Errorf("openai-compat: read: %w", err)
	}
	if resp.StatusCode >= 400 {
		return "", Usage{}, fmt.Errorf("openai-compat: api %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	var env struct {
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", Usage{}, fmt.Errorf("openai-compat: parse: %w (raw=%s)", err, truncate(string(raw), 200))
	}
	if len(env.Choices) == 0 {
		return "", Usage{}, fmt.Errorf("openai-compat: empty choices in response")
	}
	return env.Choices[0].Message.Content,
		Usage{InputTokens: env.Usage.PromptTokens, OutputTokens: env.Usage.CompletionTokens},
		nil
}

// Compile-time check that OpenAICompatProvider satisfies TextProvider.
var _ TextProvider = (*OpenAICompatProvider)(nil)
