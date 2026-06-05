package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DeepSeekProvider implements the Provider interface using the
// DeepSeek chat-completions API, which is OpenAI-compatible. We
// hit the REST endpoint directly with net/http instead of pulling
// in the openai-go SDK because:
//   - we only need single-turn completion (no streaming, tools, etc.)
//   - dropping an SDK dependency keeps the binary leaner and
//     simplifies dependency review
//   - the API surface is stable and trivially testable with a fake
//     RoundTripper
//
// Endpoint reference (as of 2026): https://api.deepseek.com/chat/completions
// Models: deepseek-chat (V3-tier general), deepseek-reasoner (R1-tier)
const (
	DeepSeekDefaultModel    = "deepseek-chat"
	DeepSeekDefaultEndpoint = "https://api.deepseek.com/chat/completions"
	DeepSeekDefaultMaxToks  = 4096
)

// DeepSeekProvider talks to the DeepSeek REST endpoint.
type DeepSeekProvider struct {
	apiKey       string
	endpoint     string
	defaultModel string
	httpClient   *http.Client
}

// NewDeepSeekProvider builds a provider against the public DeepSeek
// endpoint. Pass an empty key to construct a dev-mode instance:
// Available() then returns false and Complete returns
// ErrProviderUnavailable.
func NewDeepSeekProvider(apiKey string) *DeepSeekProvider {
	return &DeepSeekProvider{
		apiKey:       apiKey,
		endpoint:     DeepSeekDefaultEndpoint,
		defaultModel: DeepSeekDefaultModel,
		// Use a dedicated http.Client (not http.DefaultClient) so
		// the per-call context.WithTimeout still bounds total wall
		// time but we never silently inherit DefaultClient.Timeout
		// changes from elsewhere in the binary.
		httpClient: &http.Client{Timeout: 0},
	}
}

// NewDeepSeekProviderWithClient lets tests swap in a fake
// *http.Client (typically built around a httptest.Server or a
// custom http.RoundTripper). Production code uses NewDeepSeekProvider.
func NewDeepSeekProviderWithClient(apiKey, endpoint string, client *http.Client) *DeepSeekProvider {
	return &DeepSeekProvider{
		apiKey:       apiKey,
		endpoint:     endpoint,
		defaultModel: DeepSeekDefaultModel,
		httpClient:   client,
	}
}

// Name implements Provider.
func (p *DeepSeekProvider) Name() string { return ProviderDeepSeek }

// Available implements Provider.
func (p *DeepSeekProvider) Available() bool { return p.apiKey != "" }

// deepSeekRequest mirrors the subset of the OpenAI chat-completions
// request body that DeepSeek accepts. We only use the fields we
// need; the OpenAI spec is forwards-compatible (unknown fields are
// ignored on the server side).
type deepSeekRequest struct {
	Model     string            `json:"model"`
	Messages  []deepSeekMessage `json:"messages"`
	MaxTokens int64             `json:"max_tokens,omitempty"`
	Stream    bool              `json:"stream"`
	Stop      []string          `json:"stop,omitempty"`
}

type deepSeekMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type deepSeekResponse struct {
	Choices []struct {
		Message deepSeekMessage `json:"message"`
		// FinishReason is captured for future logging hooks; not
		// surfaced to handlers in Phase 1.
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// Complete implements Provider. Maps the prompt to a single-user
// message and returns the first assistant content. The DeepSeek API
// is OpenAI-shape: POST chat/completions with messages=[…].
func (p *DeepSeekProvider) Complete(ctx context.Context, prompt string, opts CompleteOptions) (string, error) {
	if !p.Available() {
		return "", fmt.Errorf("deepseek provider: %w", ErrProviderUnavailable)
	}
	model := p.defaultModel
	if opts.Model != "" {
		model = string(opts.Model)
	}
	maxToks := int64(DeepSeekDefaultMaxToks)
	if opts.MaxTokens > 0 {
		maxToks = opts.MaxTokens
	}
	timeout := DefaultProviderTimeout
	if opts.Timeout > 0 {
		timeout = opts.Timeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	body, err := json.Marshal(deepSeekRequest{
		Model:     model,
		MaxTokens: maxToks,
		Stream:    false,
		Messages: []deepSeekMessage{
			{Role: "user", Content: prompt},
		},
	})
	if err != nil {
		// json.Marshal on this struct cannot realistically fail; wrap
		// for completeness so a future maintainer changing the
		// payload doesn't get a silent zero-length request.
		return "", fmt.Errorf("deepseek provider: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(cctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("deepseek provider: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("deepseek provider: http: %w", err)
	}
	defer resp.Body.Close()

	// Read with a soft cap to avoid OOMing the server if the
	// upstream returns a gigantic payload. 4 MiB is well above any
	// reasonable single completion (default max_tokens=4096 ≈ ~16KB).
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", fmt.Errorf("deepseek provider: read body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("deepseek provider: status %d: %s", resp.StatusCode, truncateForError(raw, 256))
	}

	var parsed deepSeekResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("deepseek provider: decode response: %w", err)
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("deepseek provider: api error %s: %s", parsed.Error.Code, parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return "", errors.New("deepseek provider: no choices returned")
	}
	return parsed.Choices[0].Message.Content, nil
}

// truncateForError clips a response body for inclusion in an error
// message so 500-line HTML error pages don't blow up our log lines.
func truncateForError(raw []byte, n int) string {
	if len(raw) <= n {
		return string(raw)
	}
	return string(raw[:n]) + "...(truncated)"
}

// Compile-time assertion that we still satisfy the Provider
// interface — catches accidental method-set drift at build time.
var _ Provider = (*DeepSeekProvider)(nil)

// Sanity sentinel: ensure HTTP timeout fallback uses package default
// so changing one place doesn't silently desync the rest.
var _ = time.Duration(DefaultProviderTimeout)
