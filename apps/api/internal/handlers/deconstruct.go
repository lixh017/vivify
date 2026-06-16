package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/opc/api/internal/agents"
	"github.com/opc/api/internal/config"
)

// maxTranscriptBytes caps the size of the transcript a caller can
// post to /ai/deconstruct or /ai/viral-formula. Hard cap so a
// pasted novel cannot drive a MiniMax request that burns the entire
// monthly budget on a single call. ~256 KiB is enough for ~80k
// English words / ~50k Chinese chars — comfortably more than any
// single short-form video transcript.
const maxTranscriptBytes = 256 * 1024

// DeconstructHandler exposes the two OpusClip-style endpoints:
//   - POST /ai/deconstruct      — break a video transcript into hook /
//     structure / cta / emotional arc /
//     reusable patterns
//   - POST /ai/viral-formula    — extract a named, reusable formula
//
// Both follow the same demo/real split as the rest of the AI
// surface: an unconfigured client or ?demo=true short-circuits to
// canned data, otherwise the prompt is sent to the configured text
// provider and the response is parsed.
//
// Phase 4 wiring: the handler holds a textClientResolver
// (production) + a defaultText fallback (tests).
type DeconstructHandler struct {
	resolver    textClientResolver
	defaultText agents.TextProvider
	logger      *slog.Logger
}

// NewDeconstructHandler wires a DeconstructHandler that resolves the
// text provider per-request from the supplied resolver. No DB
// dependency — both endpoints are stateless (the transcript is
// posted in the request body, not stored).
func NewDeconstructHandler(resolver textClientResolver) *DeconstructHandler {
	return &DeconstructHandler{resolver: resolver, logger: slog.Default()}
}

// NewDeconstructHandlerWithDefault is a transitional constructor
// kept during the Phase 4 migration. It wires the handler with a
// fixed TextProvider and no resolver, used by tests that have not
// been migrated to a stub resolver yet. New callers should prefer
// NewDeconstructHandler(resolver).
func NewDeconstructHandlerWithDefault(text agents.TextProvider) *DeconstructHandler {
	return &DeconstructHandler{defaultText: text, logger: slog.Default()}
}

// shouldUseDemo mirrors AIHandler.shouldUseDemo. Duplicated rather
// than shared so handlers stay decoupled.
func (h *DeconstructHandler) shouldUseDemo(c *gin.Context) bool {
	if isDemoRequest(c) {
		return true
	}
	text := resolveTextForRequest(c, h.resolver, h.defaultText)
	return !text.Available()
}

// RegisterRoutes attaches the two endpoints under /ai.
func (h *DeconstructHandler) RegisterRoutes(r gin.IRouter) {
	r.POST("/ai/deconstruct", h.Deconstruct)
	r.POST("/ai/viral-formula", h.ViralFormula)
}

// ---------------------------------------------------------------------------
// /ai/deconstruct
// ---------------------------------------------------------------------------

// deconstructRequestMetadata mirrors the optional metadata block the
// caller can pass with the transcript. All fields are optional;
// missing fields are sent to the prompt as "(未提供)".
type deconstructRequestMetadata struct {
	Platform    string `json:"platform"`
	DurationSec int    `json:"duration_sec"`
	Views       int    `json:"views"`
	Likes       int    `json:"likes"`
}

// deconstructRequest is the JSON body for POST /ai/deconstruct.
type deconstructRequest struct {
	Transcript string                     `json:"transcript"`
	Metadata   deconstructRequestMetadata `json:"metadata"`
}

// hookBlock is one piece of the deconstruct response. The Type
// field is intentionally a free string (rather than a sealed enum)
// so a future MiniMax response with a new hook category does not
// break the parser; the frontend renders whatever comes back.
type hookBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Analysis string `json:"analysis"`
	Strength int    `json:"strength"`
}

// structureBeat is one beat in the video's structural arc.
type structureBeat struct {
	TimePct     int    `json:"time_pct"`
	Role        string `json:"role"`
	Description string `json:"description"`
}

// structureBlock describes the video's overall structure: which
// pattern it follows, the beat-by-beat arc, pacing, and information
// density.
type structureBlock struct {
	Pattern string          `json:"pattern"`
	Beats   []structureBeat `json:"beats"`
	Pacing  string          `json:"pacing"`
	Density int             `json:"density"`
}

// ctaBlock describes the call-to-action: whether present, what
// type, and where it lands in the timeline.
type ctaBlock struct {
	Present   bool   `json:"present"`
	Type      string `json:"type"`
	Placement string `json:"placement"`
}

// emotionalArcPoint is one sample on the emotional intensity curve.
type emotionalArcPoint struct {
	TimePct   int    `json:"time_pct"`
	Emotion   string `json:"emotion"`
	Intensity int    `json:"intensity"`
}

// platformFitNotes is the per-platform recommendation block. The
// JSON keys are the Chinese platform names so the frontend can
// index directly without a mapping table.
type platformFitNotes struct {
	Douyin      string `json:"抖音"`
	Bilibili    string `json:"哔哩哔哩"`
	Xiaohongshu string `json:"小红书"`
}

// deconstructResponse is the full wire shape for POST /ai/deconstruct.
type deconstructResponse struct {
	Hook             hookBlock           `json:"hook"`
	Structure        structureBlock      `json:"structure"`
	CTA              ctaBlock            `json:"cta"`
	EmotionalArc     []emotionalArcPoint `json:"emotional_arc"`
	ReusablePatterns []string            `json:"reusable_patterns"`
	PlatformFitNotes platformFitNotes    `json:"platform_fit_notes"`
	OverallScore     int                 `json:"overall_score"`
}

// Deconstruct — POST /ai/deconstruct
//
// Body: {transcript, metadata?}
// 200:  {hook, structure, cta, emotional_arc, reusable_patterns,
//
//	platform_fit_notes, overall_score}
//
// 400:  invalid body / empty transcript / transcript too large
// 502:  Claude returned output that could not be parsed
// 503:  MiniMax is not configured (no API key) and no demo flag
func (h *DeconstructHandler) Deconstruct(c *gin.Context) {
	var req deconstructRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("invalid deconstruct body", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	req.Transcript = strings.TrimSpace(req.Transcript)
	if req.Transcript == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "transcript is required"})
		return
	}
	if len(req.Transcript) > maxTranscriptBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "transcript too large"})
		return
	}

	// Demo mode: short-circuit to the pre-canned panda-IP video
	// deconstruction. We seed with the transcript length so the
	// same input gets the same output during a demo, but the
	// operator can re-run with different transcripts to rotate
	// through the pool when we add more entries.
	if h.shouldUseDemo(c) {
		raw, _ := agents.DemoResponse(agents.DemoOpDeconstruct, len(req.Transcript))
		out, err := parseDeconstruct(raw)
		if err != nil {
			h.logger.Error("deconstruct demo parse failed", "err", err.Error(), "request_id", c.GetString("request_id"))
			c.JSON(http.StatusBadGateway, gin.H{"error": "AI demo response could not be parsed: " + err.Error()})
			return
		}
		markDemoResponse(c)
		c.JSON(http.StatusOK, out)
		return
	}

	meta := agents.DeconstructMetadata{
		Platform:    req.Metadata.Platform,
		DurationSec: req.Metadata.DurationSec,
		Views:       req.Metadata.Views,
		Likes:       req.Metadata.Likes,
	}
	prompt := agents.DeconstructPrompt(req.Transcript, meta)

	text := resolveTextForRequest(c, h.resolver, h.defaultText)
	ctx, cancel := context.WithTimeout(c.Request.Context(), aiTimeout)
	defer cancel()

	body, usage, err := text.CompleteWithUsage(ctx, prompt, agents.CompleteOptions{})
	if err != nil {
		h.logger.Error("deconstruct text failed", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AI service failed: " + err.Error()})
		return
	}
	StampClaudeCost(c, config.SkillMiniMaxM27, usage.InputTokens, usage.OutputTokens)

	out, err := parseDeconstruct(body)
	if err != nil {
		h.logger.Error("deconstruct parse failed", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusBadGateway, gin.H{"error": "AI returned output that could not be parsed: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

// parseDeconstruct extracts a deconstructResponse from a raw MiniMax
// response. Same lenient fence-stripping approach as the other
// parsers in the package: strip a single ```json ... ``` wrapper,
// then locate the first JSON object, then unmarshal.
func parseDeconstruct(raw string) (deconstructResponse, error) {
	if len(raw) > maxAIResultBytes {
		return deconstructResponse{}, errors.New("response too large")
	}
	cleaned := stripJSONFence(raw)
	if !strings.HasPrefix(cleaned, "{") {
		loc := jsonObjectRE.FindStringIndex(cleaned)
		if loc == nil {
			return deconstructResponse{}, errors.New("no JSON object found in response")
		}
		cleaned = cleaned[loc[0]:loc[1]]
	}
	var out deconstructResponse
	if err := json.Unmarshal([]byte(cleaned), &out); err != nil {
		return deconstructResponse{}, err
	}
	// Normalize nil slices to empty so the frontend never gets
	// `null` for an array — it can iterate without a guard.
	if out.Structure.Beats == nil {
		out.Structure.Beats = []structureBeat{}
	}
	if out.EmotionalArc == nil {
		out.EmotionalArc = []emotionalArcPoint{}
	}
	if out.ReusablePatterns == nil {
		out.ReusablePatterns = []string{}
	}
	// Clamp scores to [0,100] so a hallucinated 137 cannot leak to
	// the frontend.
	out.Hook.Strength = clamp(out.Hook.Strength, 0, 100)
	out.Structure.Density = clamp(out.Structure.Density, 0, 100)
	out.OverallScore = clamp(out.OverallScore, 0, 100)
	return out, nil
}

// ---------------------------------------------------------------------------
// /ai/viral-formula
// ---------------------------------------------------------------------------

// viralFormulaRequest is the JSON body for POST /ai/viral-formula.
type viralFormulaRequest struct {
	Transcript string `json:"transcript"`
}

// viralFormulaVariable is one slot in the extracted formula. weight
// is constrained to low/medium/high so the frontend can style
// consistently; the parser accepts any string and lets the
// frontend default if the value drifts.
type viralFormulaVariable struct {
	Name    string `json:"name"`
	Example string `json:"example"`
	Weight  string `json:"weight"`
}

// viralFormulaResponse is the wire shape for POST /ai/viral-formula.
type viralFormulaResponse struct {
	FormulaName        string                 `json:"formula_name"`
	Variables          []viralFormulaVariable `json:"variables"`
	Steps              []string               `json:"steps"`
	ExampleApplication string                 `json:"example_application"`
	Variations         []string               `json:"variations"`
}

// ViralFormula — POST /ai/viral-formula
//
// Body: {transcript}
// 200:  {formula_name, variables, steps, example_application, variations}
// 400:  invalid body / empty transcript / transcript too large
// 502:  Claude returned output that could not be parsed
// 503:  MiniMax is not configured (no API key) and no demo flag
func (h *DeconstructHandler) ViralFormula(c *gin.Context) {
	var req viralFormulaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("invalid viral-formula body", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	req.Transcript = strings.TrimSpace(req.Transcript)
	if req.Transcript == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "transcript is required"})
		return
	}
	if len(req.Transcript) > maxTranscriptBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "transcript too large"})
		return
	}

	if h.shouldUseDemo(c) {
		raw, _ := agents.DemoResponse(agents.DemoOpViralFormula, len(req.Transcript))
		out, err := parseViralFormula(raw)
		if err != nil {
			h.logger.Error("viral-formula demo parse failed", "err", err.Error(), "request_id", c.GetString("request_id"))
			c.JSON(http.StatusBadGateway, gin.H{"error": "AI demo response could not be parsed: " + err.Error()})
			return
		}
		markDemoResponse(c)
		c.JSON(http.StatusOK, out)
		return
	}

	prompt := agents.ViralFormulaPrompt(req.Transcript)

	text := resolveTextForRequest(c, h.resolver, h.defaultText)
	ctx, cancel := context.WithTimeout(c.Request.Context(), aiTimeout)
	defer cancel()

	body, usage, err := text.CompleteWithUsage(ctx, prompt, agents.CompleteOptions{})
	if err != nil {
		h.logger.Error("viral-formula text failed", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AI service failed: " + err.Error()})
		return
	}
	StampClaudeCost(c, config.SkillMiniMaxM27, usage.InputTokens, usage.OutputTokens)

	out, err := parseViralFormula(body)
	if err != nil {
		h.logger.Error("viral-formula parse failed", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusBadGateway, gin.H{"error": "AI returned output that could not be parsed: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

// parseViralFormula extracts a viralFormulaResponse from a raw
// Claude response.
func parseViralFormula(raw string) (viralFormulaResponse, error) {
	if len(raw) > maxAIResultBytes {
		return viralFormulaResponse{}, errors.New("response too large")
	}
	cleaned := stripJSONFence(raw)
	if !strings.HasPrefix(cleaned, "{") {
		loc := jsonObjectRE.FindStringIndex(cleaned)
		if loc == nil {
			return viralFormulaResponse{}, errors.New("no JSON object found in response")
		}
		cleaned = cleaned[loc[0]:loc[1]]
	}
	var out viralFormulaResponse
	if err := json.Unmarshal([]byte(cleaned), &out); err != nil {
		return viralFormulaResponse{}, err
	}
	if out.Variables == nil {
		out.Variables = []viralFormulaVariable{}
	}
	if out.Steps == nil {
		out.Steps = []string{}
	}
	if out.Variations == nil {
		out.Variations = []string{}
	}
	return out, nil
}

// stripJSONFence trims surrounding whitespace and a single
// ```json ... ``` wrapper from a raw Claude response. Used by both
// parsers to keep the fence-handling logic in one place — drift
// between parsers is a recurring bug source.
func stripJSONFence(raw string) string {
	cleaned := strings.TrimSpace(raw)
	if strings.HasPrefix(cleaned, "```") {
		if i := strings.Index(cleaned, "\n"); i >= 0 {
			cleaned = cleaned[i+1:]
		}
		if strings.HasSuffix(cleaned, "```") {
			cleaned = cleaned[:len(cleaned)-3]
		}
		cleaned = strings.TrimSpace(cleaned)
	}
	return cleaned
}
