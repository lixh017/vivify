package agents

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

// Cover providers we know how to call. The two real backends
// (Kling on 火山引擎, 即梦/Jimeng on 火山引擎) live behind
// separate flag values so the request body matches each
// provider's contract — the underlying transport is the same
// 火山引擎 OpenAPI endpoint but the payload schema differs.
const (
	CoverProviderKling  = "kling"
	CoverProviderJimeng = "jimeng" // 即梦
	CoverProviderAuto   = "auto"   // pick first configured, else mock
	CoverProviderMock   = "mock"   // never hits the network
)

// Default timeout for a single cover-image generation request.
// Generation is usually 5-15s; we leave a generous upper bound
// so a slow upstream does not surface as a transient 502 to the
// frontend.
const coverDefaultTimeout = 30 * time.Second

// maxCoverResponseBytes caps how many bytes we will buffer from
// the upstream provider response. 1 MiB is well above any sane
// 火山引擎 image-generation payload (a typical response is a few
// KB of JSON around the CDN URL) but protects the API server
// from OOMing if the upstream ever returns an unexpectedly
// large body (e.g. an HTML error page from a misconfigured
// proxy).
const maxCoverResponseBytes = 1 << 20

// Volcengine base URL for the image-generation endpoints. The
// path is appended per-provider by the request builder. We keep
// it as a var (not const) so tests can override it with a
// httptest server.
var volcengineBaseURL = "https://visual.volcengineapi.com"

// CoverRequest is the user-facing input to the cover-image
// pipeline. All fields except Title are optional; missing
// fields are filled in by sensible defaults (style, platform)
// or ignored (topic_id is only used as a cache key hint).
type CoverRequest struct {
	Title    string `json:"title"`
	Style    string `json:"style,omitempty"`
	Platform string `json:"platform,omitempty"`
	TopicID  uint   `json:"topic_id,omitempty"`
}

// CoverResult is the wire shape returned by Generate. image_url
// is whatever the provider returns (CDN URL for Kling/Jimeng,
// data: URL for the mock fallback). prompt_used is the actual
// prompt that was sent (or, for the mock, the prompt we would
// have sent) so the frontend can show "this is what the model
// saw" in a future UX iteration.
type CoverResult struct {
	ImageURL         string `json:"image_url"`
	PromptUsed       string `json:"prompt_used"`
	Provider         string `json:"provider"`
	GenerationTimeMs int64  `json:"generation_time_ms"`
}

// CoverGenerator is the small interface the cover handler
// depends on. It is satisfied by *CoverClient (production) and
// by fakes in tests; the handler does not care which
// underlying provider actually served the request.
type CoverGenerator interface {
	Generate(ctx context.Context, req CoverRequest) (CoverResult, error)
	Available() bool
}

// CoverClient is the production implementation. It wraps the
// 火山引擎 OpenAPI call and falls back to a deterministic mock
// when no API key is configured or when the caller passes
// ?provider=mock. The mock branch is exercised by the demo
// path so the operator can show the feature without burning
// real tokens.
type CoverClient struct {
	apiKey     string
	provider   string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewCoverClient constructs a CoverClient. apiKey is the
// 火山引擎 access key (VOLCENGINE_ACCESS_KEY). When apiKey is
// empty Available() returns false and Generate returns a mock
// image — the operator can still demo the feature end to end.
//
// provider selects which model family to call when a real
// generation is needed. Allowed values: "kling", "jimeng",
// "auto" (default: kling if configured else jimeng), "mock"
// (force mock even when a key is set). Unknown values are
// coerced to "auto" so a typo does not 400 the call, AND a
// warning is logged with the original value so the operator
// can spot COVER_PROVIDER=klign (typo) instead of silently
// getting a kling-or-mock fallback.
func NewCoverClient(apiKey, provider string) *CoverClient {
	original := provider
	p := strings.ToLower(strings.TrimSpace(provider))
	switch p {
	case "", CoverProviderAuto:
		p = CoverProviderAuto
	case CoverProviderKling, CoverProviderJimeng, CoverProviderMock:
		// keep
	default:
		// Unknown provider name — fall through to auto so the
		// call still has a chance to succeed. Log the original
		// (un-normalized) value so the operator can correlate
		// with COVER_PROVIDER in their .env / deployment config.
		slog.Default().Warn(
			"cover: unknown provider coerced to auto",
			"raw", original,
			"normalized", p,
		)
		p = CoverProviderAuto
	}
	return &CoverClient{
		apiKey:     apiKey,
		provider:   p,
		httpClient: &http.Client{Timeout: coverDefaultTimeout},
		logger:     slog.Default(),
	}
}

// Available reports whether the client has the credentials it
// needs to call a real provider. The cover handler uses this
// to pick the response provider label ("mock" vs "kling") and
// to decide whether to log a "falling back to mock" warning
// at request time.
func (c *CoverClient) Available() bool {
	if c.provider == CoverProviderMock {
		// The caller explicitly asked for mock. We treat that
		// as "available" so the handler does not warn; the
		// branch in Generate is explicit and obvious.
		return true
	}
	return c.apiKey != ""
}

// ProviderName returns the canonical provider identifier used
// in the response payload. Resolution order: explicit provider
// if it is "kling"/"jimeng"/"mock", else "kling" when the key
// is set, else "mock".
func (c *CoverClient) ProviderName() string {
	switch c.provider {
	case CoverProviderKling, CoverProviderJimeng, CoverProviderMock:
		return c.provider
	}
	if c.apiKey != "" {
		return CoverProviderKling
	}
	return CoverProviderMock
}

// Generate runs a cover-image generation. Behaviour:
//  1. If provider == "mock" OR apiKey is empty, return a
//     deterministic mock image (data: URL with a small SVG).
//  2. Otherwise build the prompt, call the 火山引擎 endpoint,
//     and return the CDN URL from the response.
//
// The function never returns an empty image_url: a transport
// failure is wrapped as an error and a mock is NOT returned in
// the failure path (a 503 is more honest than a silently-mocked
// image when the operator thinks they hit a real provider).
func (c *CoverClient) Generate(ctx context.Context, req CoverRequest) (CoverResult, error) {
	start := time.Now()
	prompt := buildCoverPrompt(req)

	if c.provider == CoverProviderMock || c.apiKey == "" {
		return CoverResult{
			ImageURL:         mockCoverImageURL(req),
			PromptUsed:       prompt,
			Provider:         CoverProviderMock,
			GenerationTimeMs: time.Since(start).Milliseconds(),
		}, nil
	}

	prov := c.ProviderName()
	body, err := c.buildProviderBody(prov, prompt)
	if err != nil {
		return CoverResult{}, fmt.Errorf("cover: build request: %w", err)
	}

	url := volcengineBaseURL + providerPath(prov)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return CoverResult{}, fmt.Errorf("cover: build http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return CoverResult{}, fmt.Errorf("cover: http call: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxCoverResponseBytes))
	if err != nil {
		return CoverResult{}, fmt.Errorf("cover: read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		// Include a short snippet of the body so an operator can
		// diagnose 401/403/429 without re-running with curl.
		snippet := string(raw)
		if len(snippet) > 200 {
			snippet = snippet[:200] + "..."
		}
		return CoverResult{}, fmt.Errorf("cover: provider %s returned %d: %s", prov, resp.StatusCode, snippet)
	}

	imageURL, err := parseProviderImageURL(prov, raw)
	if err != nil {
		return CoverResult{}, fmt.Errorf("cover: parse response: %w", err)
	}

	return CoverResult{
		ImageURL:         imageURL,
		PromptUsed:       prompt,
		Provider:         prov,
		GenerationTimeMs: time.Since(start).Milliseconds(),
	}, nil
}

// buildCoverPrompt turns a CoverRequest into a stable Chinese
// prompt the providers can render against. We do NOT echo the
// title verbatim — providers do a better job when the prompt
// carries the IP voice + style + format intent in addition to
// the subject. Missing style/platform fall back to canonical
// defaults so a half-filled request still produces a sensible
// image.
func buildCoverPrompt(req CoverRequest) string {
	style := strings.TrimSpace(req.Style)
	if style == "" {
		style = "治愈系"
	}
	platform := strings.TrimSpace(req.Platform)
	if platform == "" {
		platform = "抖音"
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "熊猫"
	}
	// Aspect ratio hint by platform. 抖音/小红书 are 9:16
	// vertical, 哔哩哔哩 is 16:9 widescreen. We embed the hint
	// in the prompt because the providers differ in how they
	// accept ratio params and a text hint works on both.
	ratioHint := "vertical 9:16"
	switch platform {
	case "哔哩哔哩", "YouTube":
		ratioHint = "widescreen 16:9"
	}
	return fmt.Sprintf(
		"%s 风格封面,主体:%s,平台:%s,画幅:%s,高质量,精细插画",
		style, title, platform, ratioHint,
	)
}

// providerPath returns the URL path for the given provider.
// Kling and 即梦 (Jimeng) are both served from the 火山引擎
// 视觉 endpoint, but the path is provider-specific. We
// centralize the mapping so a future provider can be added in
// one place.
func providerPath(prov string) string {
	switch prov {
	case CoverProviderJimeng:
		return "/api/v1/jimeng/image_generate"
	default:
		return "/api/v2/kling/image_generate"
	}
}

// buildProviderBody serializes the per-provider request body.
// The schemas below are the minimum subset the providers
// require; an empty extra map is fine because Go's encoding/json
// omits unset fields.
func (c *CoverClient) buildProviderBody(prov, prompt string) ([]byte, error) {
	switch prov {
	case CoverProviderJimeng:
		return json.Marshal(map[string]any{
			"prompt": prompt,
			"width":  720,
			"height": 1280,
		})
	default:
		return json.Marshal(map[string]any{
			"prompt":       prompt,
			"aspect_ratio": "9:16",
			"n":            1,
		})
	}
}

// parseProviderImageURL extracts the CDN image URL from the
// provider's response. Both backends return JSON; the field
// name differs (Kling nests it under data.image_urls[0], Jimeng
// returns data.image_url). Anything else is treated as a
// parse error.
func parseProviderImageURL(prov string, raw []byte) (string, error) {
	switch prov {
	case CoverProviderJimeng:
		var resp struct {
			Data struct {
				ImageURL string `json:"image_url"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return "", err
		}
		if resp.Data.ImageURL == "" {
			return "", errors.New("empty image_url in jimeng response")
		}
		return resp.Data.ImageURL, nil
	default:
		var resp struct {
			Data struct {
				ImageURLs []string `json:"image_urls"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return "", err
		}
		if len(resp.Data.ImageURLs) == 0 || resp.Data.ImageURLs[0] == "" {
			return "", errors.New("empty image_urls in kling response")
		}
		return resp.Data.ImageURLs[0], nil
	}
}

// mockCoverImageURL returns a deterministic data: URL that
// renders as a tiny SVG with the topic title burned in. The
// data: URL is preferred over an external placeholder because
// it requires no outbound network from the API server and the
// frontend can render it in <img> tags as-is.
//
// The URL embeds a hash of (title+style+platform) so two
// callers asking for the same input get a stable image — the
// demo flow caches nicely, and the frontend can show "we
// would have asked the model for ..." without the
// round-tripping the same SVG twice.
func mockCoverImageURL(req CoverRequest) string {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "未命名选题"
	}
	style := strings.TrimSpace(req.Style)
	if style == "" {
		style = "治愈系"
	}
	platform := strings.TrimSpace(req.Platform)
	if platform == "" {
		platform = "通用"
	}

	h := sha256.Sum256([]byte(title + "|" + style + "|" + platform))
	digest := hex.EncodeToString(h[:8])
	// Color is derived from the digest so the placeholder is
	// visually distinct per topic. oklch is not always supported
	// by every <img>-style renderer, so we stick to a #RRGGBB
	// pair derived from the hash bytes.
	r := h[0]
	g := h[1]
	b := h[2]
	bg := fmt.Sprintf("#%02x%02x%02x", r, g, b)

	svg := fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" width="720" height="1280" viewBox="0 0 720 1280">`+
			`<rect width="720" height="1280" fill="%s"/>`+
			`<text x="40" y="120" font-family="sans-serif" font-size="48" fill="#fff" font-weight="bold">%s · %s</text>`+
			`<text x="40" y="200" font-family="sans-serif" font-size="36" fill="#fff">%s</text>`+
			`<text x="40" y="1240" font-family="sans-serif" font-size="24" fill="#fff" opacity="0.7">mock:%s · set VOLCENGINE_ACCESS_KEY for real images</text>`+
			`</svg>`,
		bg, platform, style, escapeXML(title), digest,
	)
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
}

// escapeXML escapes the four characters that would otherwise
// break an SVG text element. title is user-supplied so we
// cannot trust it to be entity-safe.
func escapeXML(s string) string {
	r := strings.NewReplacer(
		`&`, "&amp;",
		`<`, "&lt;",
		`>`, "&gt;",
		`"`, "&quot;",
		`'`, "&apos;",
	)
	return r.Replace(s)
}

// EnvFirstCoverClient is a constructor used by main.go. It
// reads VOLCENGINE_ACCESS_KEY and COVER_PROVIDER from the
// environment with sensible defaults so a fresh deploy with
// an empty .env still works in mock mode.
func EnvFirstCoverClient() *CoverClient {
	return NewCoverClient(
		strings.TrimSpace(os.Getenv("VOLCENGINE_ACCESS_KEY")),
		strings.TrimSpace(os.Getenv("COVER_PROVIDER")),
	)
}
