package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// MiniMax is the single-provider client for the OPC MCP server.
//
// The wire shapes (request bodies, response envelopes) mirror
// the public MiniMax REST API directly — see the official
// docs at https://platform.minimaxi.com/docs for the canonical
// reference. We do NOT shell out to the `mmx` CLI; that tool
// is useful for one-off operator experiments but adds a
// subprocess + 100ms+ of spawn overhead per call, which is
// undesirable on the request hot path.
//
// Endpoint matrix:
//
//	text    POST {BaseURL}/anthropic/v1/messages   (Anthropic-compatible)
//	image   POST {BaseURL}/v1/image_generation
//	speech  POST {BaseURL}/v1/t2a_v2
//	video   POST {BaseURL}/v1/video_generation
//	video   GET  {BaseURL}/v1/query/video_generation?task_id=...
//
// BaseURL defaults to https://api.minimaxi.com. The operator
// can override at construction time for staging mirrors.
//
// All four endpoints use the same Authorization scheme
// (Bearer <key>) — one operator token covers the whole
// pipeline. The key is read from MINIMAX_API_KEY or
// passed explicitly to NewMiniMax.
type MiniMax struct {
	APIKey  string
	BaseURL string
	Region  string // "cn" or "global"; currently advisory only
	HTTP    *http.Client

	// Test seams. When non-nil, the corresponding method
	// returns the canned result without making an HTTP call.
	// Production code never sets these.
	textOverride   func(ctx context.Context, prompt string, opts MiniMaxTextOptions) (*MiniMaxTextResult, error)
	imageOverride  func(ctx context.Context, prompt string, opts MiniMaxImageOptions) (*MiniMaxImageResult, error)
	speechOverride func(ctx context.Context, text string, opts MiniMaxSpeechOptions) (*MiniMaxSpeechResult, error)
	videoOverride  func(ctx context.Context, prompt string, opts MiniMaxVideoOptions) (*MiniMaxVideoResult, error)

	// demoOnly forces Available() to return false even when
	// the env has a real key. Set by NewMiniMaxForDemo.
	demoOnly bool
}

// DefaultBaseURL is the production MiniMax endpoint. Staging
// mirrors can swap at NewMiniMax time.
const DefaultBaseURL = "https://api.minimaxi.com"

// NewMiniMax constructs a MiniMax client. apiKey is the mmx
// token; pass "" to defer to env / config file.
func NewMiniMax(apiKey string) *MiniMax {
	return &MiniMax{
		APIKey:  apiKey,
		BaseURL: DefaultBaseURL,
		HTTP:    &http.Client{Timeout: 10 * time.Minute},
	}
}

// NewMiniMaxForDemo constructs a client that always reports
// Available()==false regardless of env, so handlers serve the
// canned demo path even on a machine with a real MINIMAX_API_KEY
// in its env. Used by tests that want to exercise the demo
// route; never wire this up in production.
func NewMiniMaxForDemo() *MiniMax {
	return &MiniMax{
		APIKey:  "",
		BaseURL: DefaultBaseURL,
		HTTP:    &http.Client{Timeout: 10 * time.Minute},
		// Mark as "demo-only" by setting a sentinel; the
		// Available() method checks this.
		demoOnly: true,
	}
}

// NewMiniMaxWithTextOverride returns a MiniMax whose Text()
// returns the supplied result without making a network call.
// Used by unit tests; production code should always use
// NewMiniMax.
func NewMiniMaxWithTextOverride(fn func(ctx context.Context, prompt string, opts MiniMaxTextOptions) (*MiniMaxTextResult, error)) *MiniMax {
	m := NewMiniMax("test-key-not-used")
	m.textOverride = fn
	return m
}

// NewMiniMaxWithImageOverride mirrors NewMiniMaxWithTextOverride
// for the Image path. The closure receives the prompt and
// options the handler passed and returns the canned result.
func NewMiniMaxWithImageOverride(fn func(ctx context.Context, prompt string, opts MiniMaxImageOptions) (*MiniMaxImageResult, error)) *MiniMax {
	m := NewMiniMax("test-key-not-used")
	m.imageOverride = fn
	return m
}

// NewMiniMaxWithSpeechOverride mirrors NewMiniMaxWithTextOverride
// for the Speech path.
func NewMiniMaxWithSpeechOverride(fn func(ctx context.Context, text string, opts MiniMaxSpeechOptions) (*MiniMaxSpeechResult, error)) *MiniMax {
	m := NewMiniMax("test-key-not-used")
	m.speechOverride = fn
	return m
}

// NewMiniMaxWithVideoOverride mirrors NewMiniMaxWithTextOverride
// for the Video path.
func NewMiniMaxWithVideoOverride(fn func(ctx context.Context, prompt string, opts MiniMaxVideoOptions) (*MiniMaxVideoResult, error)) *MiniMax {
	m := NewMiniMax("test-key-not-used")
	m.videoOverride = fn
	return m
}

// ErrNoAPIKey is the sentinel returned when a method is
// called on a client that has no API key configured. Handlers
// can use errors.Is to branch on this in the same way they
// did with the legacy Claude shim's ErrNoAPIKey.
var ErrNoAPIKey = errors.New("minimax: MINIMAX_API_KEY not configured")

// Available reports whether the client is configured to make
// real API calls. The handler uses this to short-circuit into
// demo mode when no API key is set, so dev environments
// without a key still get a useful response (with an
// X-Demo-Mode header so the UI can badge it).
func (m *MiniMax) Available() bool {
	if m.demoOnly {
		return false
	}
	if m.APIKey != "" {
		return true
	}
	return os.Getenv("MINIMAX_API_KEY") != ""
}

// Name implements both Provider (legacy) and TextProvider
// (Phase 4). Returns the canonical short name for the
// registry + the X-Text-Provider header. The constructor
// doesn't take a name argument — the package currently has
// one MiniMax-shaped client — but a future AnthropicCompat
// wrapper pointed at api.anthropic.com would have its own
// "anthropic" name.
func (m *MiniMax) Name() string { return "minimax" }

// apiKey returns the resolved API key, taking the constructor
// value or the env var in that order.
func (m *MiniMax) apiKey() string {
	if m.APIKey != "" {
		return m.APIKey
	}
	return os.Getenv("MINIMAX_API_KEY")
}

// baseURL returns the resolved base URL, defaulting when the
// caller didn't override.
func (m *MiniMax) baseURL() string {
	if m.BaseURL != "" {
		return m.BaseURL
	}
	return DefaultBaseURL
}

// MiniMaxTextOptions tunes a single text generation call.
// Zero values fall through to MiniMax defaults.
type MiniMaxTextOptions struct {
	Model       string // e.g. "MiniMax-M2.7-highspeed"
	MaxTokens   int
	Temperature float64
}

// MiniMaxTextResult is the wire shape returned by Text.
type MiniMaxTextResult struct {
	Text         string
	InputTokens  int
	OutputTokens int
	Model        string
	StopReason   string
}

// Text sends a single user-turn prompt to MiniMax via the
// Anthropic-compatible endpoint and returns the assistant's
// text + token usage. The wire shape is intentionally the
// Anthropic Messages API shape (system + messages + max_tokens
// + model) so we can swap provider by changing the base URL
// without touching the request body.
func (m *MiniMax) Text(ctx context.Context, prompt string, opts MiniMaxTextOptions) (*MiniMaxTextResult, error) {
	if m.textOverride != nil {
		return m.textOverride(ctx, prompt, opts)
	}
	if !m.Available() {
		return nil, fmt.Errorf("minimax: %w", ErrNoAPIKey)
	}
	if opts.Model == "" {
		opts.Model = "MiniMax-M2.7-highspeed"
	}
	req := anthropicMessagesRequest{
		Model:     opts.Model,
		MaxTokens: opts.MaxTokens,
		System:    "You are a helpful AI assistant.",
		Messages: []anthropicMessage{
			{Role: "user", Content: prompt},
		},
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("minimax text: marshal: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		m.baseURL()+"/anthropic/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("minimax text: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+m.apiKey())
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := m.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("minimax text: http: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxMiniMaxResponse))
	if err != nil {
		return nil, fmt.Errorf("minimax text: read: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("minimax text: api %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	return parseAnthropicMessagesResponse(raw)
}

// anthropicMessagesRequest is the request body for the
// Anthropic-compatible /v1/messages endpoint. The shape is
// standard — see the Anthropic API docs. We keep the struct
// tight (no extra metadata) because the upstream returns a
// strict subset and any extra fields we send are silently
// dropped.
type anthropicMessagesRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens,omitempty"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// parseAnthropicMessagesResponse unmarshals the Anthropic
// Messages API response envelope. MiniMax wraps the standard
// shape with a base_resp status check; non-zero status_code
// becomes an error.
func parseAnthropicMessagesResponse(raw []byte) (*MiniMaxTextResult, error) {
	var env struct {
		ID        string `json:"id"`
		Type      string `json:"type"`
		Role      string `json:"role"`
		Model     string `json:"model"`
		Content   []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		BaseResp struct {
			StatusCode int    `json:"status_code"`
			StatusMsg  string `json:"status_msg"`
		} `json:"base_resp"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("minimax text: parse: %w (raw=%s)", err, truncate(string(raw), 200))
	}
	if env.BaseResp.StatusCode != 0 {
		return nil, fmt.Errorf("minimax text: api error %d: %s", env.BaseResp.StatusCode, env.BaseResp.StatusMsg)
	}
	// Concatenate all text content blocks (the response may have
	// multiple if the model emits them in stages).
	var sb strings.Builder
	for _, b := range env.Content {
		if b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}
	return &MiniMaxTextResult{
		Text:        sb.String(),
		InputTokens: env.Usage.InputTokens,
		OutputTokens: env.Usage.OutputTokens,
		Model:       env.Model,
		StopReason:  env.StopReason,
	}, nil
}

// MiniMaxImageOptions tunes a single image generation call.
type MiniMaxImageOptions struct {
	Model        string // default: image-01
	AspectRatio  string // e.g. "9:16", "1:1"
	Width        int    // overrides aspect ratio when set
	Height       int    // overrides aspect ratio when set
	OutPath      string // exact file path; overrides OutDir
	N            int    // number of images; default 1
	Seed         int64
}

// MiniMaxImageResult is the wire shape returned by Image.
// FilePaths is the list of saved image files (the API
// returns base64 inline; we decode and write to OutPath or
// OutDir).
type MiniMaxImageResult struct {
	FilePaths []string
}

// Image generates image(s). Uses the synchronous
// /v1/image_generation endpoint and decodes the base64
// payloads to OutPath / OutDir.
func (m *MiniMax) Image(ctx context.Context, prompt string, opts MiniMaxImageOptions) (*MiniMaxImageResult, error) {
	if m.imageOverride != nil {
		return m.imageOverride(ctx, prompt, opts)
	}
	if !m.Available() {
		return nil, fmt.Errorf("minimax: %w", ErrNoAPIKey)
	}
	if opts.Model == "" {
		opts.Model = "image-01"
	}
	if opts.N == 0 {
		opts.N = 1
	}
	reqBody := map[string]any{
		"model":          opts.Model,
		"prompt":         prompt,
		"n":              opts.N,
		"response_format": "base64",
	}
	// Width/height win over aspect ratio when both set
	// (per MiniMax docs).
	if opts.Width > 0 && opts.Height > 0 {
		reqBody["width"] = opts.Width
		reqBody["height"] = opts.Height
	} else if opts.AspectRatio != "" {
		reqBody["aspect_ratio"] = opts.AspectRatio
	}
	if opts.Seed > 0 {
		reqBody["seed"] = opts.Seed
	}
	body, _ := json.Marshal(reqBody)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		m.baseURL()+"/v1/image_generation", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("minimax image: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+m.apiKey())

	resp, err := m.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("minimax image: http: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxMiniMaxResponse))
	if err != nil {
		return nil, fmt.Errorf("minimax image: read: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("minimax image: api %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	return parseImageResponse(raw, opts.OutPath)
}

func parseImageResponse(raw []byte, outPath string) (*MiniMaxImageResult, error) {
	var env struct {
		BaseResp struct {
			StatusCode int    `json:"status_code"`
			StatusMsg  string `json:"status_msg"`
		} `json:"base_resp"`
		Data struct {
			ImageBase64 []string `json:"image_base64"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("minimax image: parse: %w (raw=%s)", err, truncate(string(raw), 200))
	}
	if env.BaseResp.StatusCode != 0 {
		return nil, fmt.Errorf("minimax image: api error %d: %s", env.BaseResp.StatusCode, env.BaseResp.StatusMsg)
	}
	var paths []string
	for i, b64 := range env.Data.ImageBase64 {
		if b64 == "" {
			continue
		}
		path := outPath
		if path == "" {
			path = fmt.Sprintf("/tmp/image_%03d.jpg", i+1)
		}
		if err := writeBase64File(path, b64, 0o644); err != nil {
			return nil, fmt.Errorf("minimax image: write %s: %w", path, err)
		}
		paths = append(paths, path)
	}
	return &MiniMaxImageResult{FilePaths: paths}, nil
}

// MiniMaxSpeechOptions tunes a single TTS call.
type MiniMaxSpeechOptions struct {
	Model      string // default: speech-2.8-hd
	Voice      string // default: English_expressive_narrator
	Speed      float64
	Volume     float64
	Pitch      float64
	Format     string // mp3 / pcm / flac; default mp3
	SampleRate int    // 16000 / 24000 / 32000; default 32000
	Language   string
	OutPath    string // exact file path; default: /tmp/speech_<ts>.mp3
}

// MiniMaxSpeechResult is the wire shape returned by Speech.
type MiniMaxSpeechResult struct {
	Path       string
	DurationMs int
	SizeBytes  int
	SampleRate int
}

// Speech synthesizes text to audio via /v1/t2a_v2 and writes
// the bytes to OutPath. The endpoint returns hex-encoded
// audio by default; we request base64 to keep the wire
// consistent with the rest of the surface.
func (m *MiniMax) Speech(ctx context.Context, text string, opts MiniMaxSpeechOptions) (*MiniMaxSpeechResult, error) {
	if m.speechOverride != nil {
		return m.speechOverride(ctx, text, opts)
	}
	if !m.Available() {
		return nil, fmt.Errorf("minimax: %w", ErrNoAPIKey)
	}
	if opts.Model == "" {
		opts.Model = "speech-2.8-hd"
	}
	if opts.Voice == "" {
		opts.Voice = "English_expressive_narrator"
	}
	if opts.Format == "" {
		opts.Format = "mp3"
	}
	if opts.SampleRate == 0 {
		opts.SampleRate = 32000
	}
	if opts.OutPath == "" {
		opts.OutPath = fmt.Sprintf("/tmp/speech_%s.mp3", time.Now().Format("2006-01-02-15-04-05"))
	}
	reqBody := map[string]any{
		"model":  opts.Model,
		"text":   text,
		"voice_setting": map[string]any{
			"voice_id": opts.Voice,
		},
		"audio_setting": map[string]any{
			"format":      opts.Format,
			"sample_rate": opts.SampleRate,
		},
	}
	if opts.Speed > 0 {
		reqBody["voice_setting"].(map[string]any)["speed"] = opts.Speed
	}
	if opts.Volume > 0 {
		reqBody["voice_setting"].(map[string]any)["vol"] = opts.Volume
	}
	if opts.Pitch > 0 {
		reqBody["voice_setting"].(map[string]any)["pitch"] = opts.Pitch
	}
	if opts.Language != "" {
		reqBody["language_boost"] = opts.Language
	}
	body, _ := json.Marshal(reqBody)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		m.baseURL()+"/v1/t2a_v2", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("minimax speech: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+m.apiKey())

	resp, err := m.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("minimax speech: http: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxMiniMaxResponse))
	if err != nil {
		return nil, fmt.Errorf("minimax speech: read: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("minimax speech: api %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	return parseSpeechResponse(raw, opts.OutPath)
}

func parseSpeechResponse(raw []byte, outPath string) (*MiniMaxSpeechResult, error) {
	var env struct {
		BaseResp struct {
			StatusCode int    `json:"status_code"`
			StatusMsg  string `json:"status_msg"`
		} `json:"base_resp"`
		Data struct {
			Audio string `json:"audio"` // hex-encoded bytes
		} `json:"data"`
		ExtraInfo struct {
			AudioLength      int    `json:"audio_length"`        // ms
			AudioSampleRate  int    `json:"audio_sample_rate"`
			AudioSize        int    `json:"audio_size"`
			AudioFormat      string `json:"audio_format"`
		} `json:"extra_info"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("minimax speech: parse: %w (raw=%s)", err, truncate(string(raw), 200))
	}
	if env.BaseResp.StatusCode != 0 {
		return nil, fmt.Errorf("minimax speech: api error %d: %s", env.BaseResp.StatusCode, env.BaseResp.StatusMsg)
	}
	if env.Data.Audio == "" {
		return nil, errors.New("minimax speech: empty audio payload")
	}
	// The MiniMax speech endpoint returns the audio as a hex
	// string. The CLI decodes the hex to a file; we do the
	// same here so the on-disk file is a normal mp3 the
	// frontend can stream.
	audio, err := hexDecode(env.Data.Audio)
	if err != nil {
		return nil, fmt.Errorf("minimax speech: hex decode: %w", err)
	}
	if err := os.WriteFile(outPath, audio, 0o644); err != nil {
		return nil, fmt.Errorf("minimax speech: write %s: %w", outPath, err)
	}
	return &MiniMaxSpeechResult{
		Path:       outPath,
		DurationMs: env.ExtraInfo.AudioLength,
		SizeBytes:  env.ExtraInfo.AudioSize,
		SampleRate: env.ExtraInfo.AudioSampleRate,
	}, nil
}

// MiniMaxVideoOptions tunes a single video generation call.
type MiniMaxVideoOptions struct {
	Model      string // default: MiniMax-Hailuo-2.3
	FirstFrame string
	LastFrame  string
	SubjectImg string
	OutPath    string
	MaxWait    time.Duration
}

// MiniMaxVideoResult is the wire shape returned by Video.
type MiniMaxVideoResult struct {
	Path string
}

// Video generates a video synchronously. The flow is:
//   1. POST /v1/video_generation → task_id
//   2. Poll GET /v1/query/video_generation?task_id=... until
//      status is "success" / "failed" / "cancelled"
//   3. On success, download the file_url to OutPath
//
// MaxWait caps the total wait. Default 5 min (Hailuo clips
// take 1-2 min for a 5s clip; longer prompts need more).
func (m *MiniMax) Video(ctx context.Context, prompt string, opts MiniMaxVideoOptions) (*MiniMaxVideoResult, error) {
	if m.videoOverride != nil {
		return m.videoOverride(ctx, prompt, opts)
	}
	if !m.Available() {
		return nil, fmt.Errorf("minimax: %w", ErrNoAPIKey)
	}
	if opts.Model == "" {
		opts.Model = "MiniMax-Hailuo-2.3"
	}
	if opts.OutPath == "" {
		opts.OutPath = fmt.Sprintf("/tmp/video_%s.mp4", time.Now().Format("2006-01-02-15-04-05"))
	}
	if opts.MaxWait == 0 {
		opts.MaxWait = 5 * time.Minute
	}
	reqBody := map[string]any{
		"model":  opts.Model,
		"prompt": prompt,
	}
	if opts.FirstFrame != "" {
		reqBody["first_frame_image"] = opts.FirstFrame
	}
	if opts.LastFrame != "" {
		reqBody["last_frame_image"] = opts.LastFrame
	}
	if opts.SubjectImg != "" {
		reqBody["subject_reference"] = []string{opts.SubjectImg}
	}
	body, _ := json.Marshal(reqBody)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		m.baseURL()+"/v1/video_generation", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("minimax video submit: build: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+m.apiKey())
	resp, err := m.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("minimax video submit: http: %w", err)
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxMiniMaxResponse))
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("minimax video submit: api %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	var submitEnv struct {
		TaskID string `json:"task_id"`
		BaseResp struct {
			StatusCode int    `json:"status_code"`
			StatusMsg  string `json:"status_msg"`
		} `json:"base_resp"`
	}
	if err := json.Unmarshal(raw, &submitEnv); err != nil {
		return nil, fmt.Errorf("minimax video submit: parse: %w", err)
	}
	if submitEnv.BaseResp.StatusCode != 0 {
		return nil, fmt.Errorf("minimax video submit: api error %d: %s",
			submitEnv.BaseResp.StatusCode, submitEnv.BaseResp.StatusMsg)
	}
	if submitEnv.TaskID == "" {
		return nil, errors.New("minimax video submit: empty task_id")
	}

	// 2. Poll.
	deadline := time.Now().Add(opts.MaxWait)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("minimax video poll: %w", ctx.Err())
		case <-time.After(5 * time.Second):
		}
		polled, err := m.videoPoll(ctx, submitEnv.TaskID)
		if err != nil {
			// Network blip — keep polling until deadline.
			continue
		}
		switch polled.Status {
		case "Success", "success":
			return m.videoDownload(ctx, polled.FileURL, opts.OutPath)
		case "Fail", "fail", "Failed", "failed":
			return nil, fmt.Errorf("minimax video: task %s failed: %s",
				submitEnv.TaskID, polled.Reason)
		}
		// Still processing — loop.
	}
	return nil, fmt.Errorf("minimax video: task %s did not complete within %s",
		submitEnv.TaskID, opts.MaxWait)
}

type videoPollResult struct {
	Status  string // "Success" / "Fail" / "Processing" etc.
	FileURL string
	Reason  string
}

func (m *MiniMax) videoPoll(ctx context.Context, taskID string) (*videoPollResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		m.baseURL()+"/v1/query/video_generation?task_id="+taskID, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+m.apiKey())
	resp, err := m.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxMiniMaxResponse))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("poll %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	var env struct {
		Status   string `json:"status"`
		FileURL  string `json:"file_id"` // mmx polls return "file_id" key
		BaseResp struct {
			StatusCode int    `json:"status_code"`
			StatusMsg  string `json:"status_msg"`
		} `json:"base_resp"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	if env.BaseResp.StatusCode != 0 {
		return nil, fmt.Errorf("poll api error %d: %s",
			env.BaseResp.StatusCode, env.BaseResp.StatusMsg)
	}
	return &videoPollResult{
		Status:  env.Status,
		FileURL: env.FileURL,
	}, nil
}

func (m *MiniMax) videoDownload(ctx context.Context, fileID, outPath string) (*MiniMaxVideoResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		m.baseURL()+"/v1/files/retrieve?file_id="+fileID, nil)
	if err != nil {
		return nil, fmt.Errorf("minimax video download: build: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+m.apiKey())
	resp, err := m.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("minimax video download: http: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxMiniMaxResponse))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("download %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	// Response is JSON { file: { download_url: "https://..." } }.
	var env struct {
		File struct {
			DownloadURL string `json:"download_url"`
		} `json:"file"`
		BaseResp struct {
			StatusCode int    `json:"status_code"`
			StatusMsg  string `json:"status_msg"`
		} `json:"base_resp"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("minimax video download: parse: %w", err)
	}
	if env.BaseResp.StatusCode != 0 {
		return nil, fmt.Errorf("download api error %d: %s",
			env.BaseResp.StatusCode, env.BaseResp.StatusMsg)
	}
	if env.File.DownloadURL == "" {
		return nil, errors.New("minimax video download: empty download_url")
	}
	// Fetch the actual bytes from the signed URL.
	dlReq, err := http.NewRequestWithContext(ctx, http.MethodGet, env.File.DownloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("minimax video download: build dl: %w", err)
	}
	dlResp, err := m.HTTP.Do(dlReq)
	if err != nil {
		return nil, fmt.Errorf("minimax video download: dl http: %w", err)
	}
	defer dlResp.Body.Close()
	if dlResp.StatusCode >= 400 {
		return nil, fmt.Errorf("download url: %d", dlResp.StatusCode)
	}
	f, err := os.Create(outPath)
	if err != nil {
		return nil, fmt.Errorf("minimax video download: create %s: %w", outPath, err)
	}
	defer f.Close()
	if _, err := io.Copy(f, io.LimitReader(dlResp.Body, maxMiniMaxResponse*8)); err != nil {
		return nil, fmt.Errorf("minimax video download: write: %w", err)
	}
	return &MiniMaxVideoResult{Path: outPath}, nil
}

// CompleteWithUsage is the legacy Claude-shaped adapter around
// Text. Handlers that haven't migrated to the MiniMax native
// shape (string, agents.Usage, error) can drop in a MiniMax
// in place of the Claude shim and the same call site keeps
// working. New code should call Text directly.
func (m *MiniMax) CompleteWithUsage(ctx context.Context, prompt string, opts CompleteOptions) (string, Usage, error) {
	mmOpts := MiniMaxTextOptions{
		Model:     opts.Model,
		MaxTokens: int(opts.MaxTokens),
	}
	if mmOpts.Model == "" {
		mmOpts.Model = "MiniMax-M2.7-highspeed"
	}
	res, err := m.Text(ctx, prompt, mmOpts)
	if err != nil {
		return "", Usage{}, err
	}
	return res.Text, Usage{
		InputTokens:  res.InputTokens,
		OutputTokens: res.OutputTokens,
	}, nil
}

// Complete is the (string, error) shim. Same as
// CompleteWithUsage but discards usage; used by callers that
// don't need cost attribution yet.
func (m *MiniMax) Complete(ctx context.Context, prompt string) (string, error) {
	text, _, err := m.CompleteWithUsage(ctx, prompt, CompleteOptions{})
	return text, err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// maxMiniMaxResponse caps the bytes the wrapper will buffer
// from any MiniMax response. 8 MiB comfortably holds a 5s
// video metadata envelope + a generous audio blob; the
// actual video bytes are streamed via the separate download
// step and use a higher cap (8x).
const maxMiniMaxResponse = 8 << 20
