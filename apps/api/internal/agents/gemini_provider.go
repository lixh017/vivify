package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// GeminiProvider implements Provider against Google's Generative
// Language API (v1beta), which is the public REST surface for
// Gemini models. We use the simple `:generateContent` shape rather
// than the streamed `:streamGenerateContent` because the OPC agents
// don't need partial-token streaming.
//
// Endpoint: https://generativelanguage.googleapis.com/v1beta/models/{model}:generateContent?key=API_KEY
// Models: gemini-2.5-pro (multimodal, video understanding),
//
//	gemini-2.5-flash (cheap/fast, text-only fallback)
//
// Auth: the public REST endpoint uses an `?key=` query-string
// parameter. We never log the URL — request logs strip the query
// string at the middleware layer.
const (
	GeminiDefaultModel    = "gemini-2.5-pro"
	GeminiDefaultEndpoint = "https://generativelanguage.googleapis.com/v1beta/models"
	GeminiDefaultMaxToks  = 4096
)

// GeminiProvider talks to the Google Generative Language API.
type GeminiProvider struct {
	apiKey       string
	endpointBase string
	defaultModel string
	httpClient   *http.Client
}

// NewGeminiProvider builds a provider against the public Gemini
// endpoint. Empty key → dev mode (Available() false, Complete
// returns ErrProviderUnavailable).
func NewGeminiProvider(apiKey string) *GeminiProvider {
	return &GeminiProvider{
		apiKey:       apiKey,
		endpointBase: GeminiDefaultEndpoint,
		defaultModel: GeminiDefaultModel,
		httpClient:   &http.Client{Timeout: 0},
	}
}

// NewGeminiProviderWithClient lets tests inject a fake transport.
// The endpointBase typically points at a httptest.Server URL that
// matches the `/{model}:generateContent` path shape.
func NewGeminiProviderWithClient(apiKey, endpointBase string, client *http.Client) *GeminiProvider {
	return &GeminiProvider{
		apiKey:       apiKey,
		endpointBase: endpointBase,
		defaultModel: GeminiDefaultModel,
		httpClient:   client,
	}
}

// Name implements Provider.
func (p *GeminiProvider) Name() string { return ProviderGemini }

// Available implements Provider.
func (p *GeminiProvider) Available() bool { return p.apiKey != "" }

// geminiRequest mirrors the subset of the v1beta generateContent
// body OPC actually uses (single user-turn text, no system
// instructions yet, no tools).
type geminiRequest struct {
	Contents         []geminiContent         `json:"contents"`
	GenerationConfig *geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiGenerationConfig struct {
	MaxOutputTokens int64 `json:"maxOutputTokens,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
		// FinishReason captured for future logging; not surfaced
		// in Phase 1.
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback *struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback,omitempty"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

// Complete implements Provider. Maps the prompt to a single
// user-content text part and returns the first candidate's first
// text block.
func (p *GeminiProvider) Complete(ctx context.Context, prompt string, opts CompleteOptions) (string, error) {
	if !p.Available() {
		return "", fmt.Errorf("gemini provider: %w", ErrProviderUnavailable)
	}
	model := p.defaultModel
	if opts.Model != "" {
		model = string(opts.Model)
	}
	maxToks := int64(GeminiDefaultMaxToks)
	if opts.MaxTokens > 0 {
		maxToks = opts.MaxTokens
	}
	timeout := DefaultProviderTimeout
	if opts.Timeout > 0 {
		timeout = opts.Timeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	endpoint := strings.TrimRight(p.endpointBase, "/") + "/" + url.PathEscape(model) + ":generateContent"
	endpoint += "?key=" + url.QueryEscape(p.apiKey)

	body, err := json.Marshal(geminiRequest{
		Contents: []geminiContent{
			{
				Role:  "user",
				Parts: []geminiPart{{Text: prompt}},
			},
		},
		GenerationConfig: &geminiGenerationConfig{
			MaxOutputTokens: maxToks,
		},
	})
	if err != nil {
		return "", fmt.Errorf("gemini provider: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(cctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("gemini provider: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini provider: http: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", fmt.Errorf("gemini provider: read body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("gemini provider: status %d: %s", resp.StatusCode, truncateForError(raw, 256))
	}

	var parsed geminiResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("gemini provider: decode response: %w", err)
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("gemini provider: api error %s: %s", parsed.Error.Status, parsed.Error.Message)
	}
	if parsed.PromptFeedback != nil && parsed.PromptFeedback.BlockReason != "" {
		return "", fmt.Errorf("gemini provider: prompt blocked: %s", parsed.PromptFeedback.BlockReason)
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return "", errors.New("gemini provider: no candidates returned")
	}
	return parsed.Candidates[0].Content.Parts[0].Text, nil
}

// Compile-time interface assertion.
var _ Provider = (*GeminiProvider)(nil)
