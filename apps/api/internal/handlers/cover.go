package handlers

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/opc/api/internal/agents"
)

// coverRequest is the JSON body for POST /ai/cover.
//
// title is the only required field — the providers can render
// against a single subject line. style and platform are
// optional hints that get baked into the prompt; missing
// values fall through to the cover client's defaults (治愈
// / 抖音). topic_id is reserved for the persistence path
// (Phase 2 multi-tenant): when set, a future iteration of
// this handler will record the generated image against the
// topic row so the operator can re-open "topic N's cover"
// from the topic detail page.
type coverRequest struct {
	Title    string `json:"title"`
	Style    string `json:"style,omitempty"`
	Platform string `json:"platform,omitempty"`
	TopicID  uint   `json:"topic_id,omitempty"`
}

// coverResponse is the wire shape returned by /ai/cover.
// Fields are kept flat (not nested under a "data" key) so
// the response can be passed straight to <img src=...>.
type coverResponse struct {
	ImageURL         string `json:"image_url"`
	PromptUsed       string `json:"prompt_used"`
	Provider         string `json:"provider"`
	GenerationTimeMs int64  `json:"generation_time_ms"`
}

// CoverHandler exposes the cover-image generation endpoint.
// It is a thin wrapper around agents.CoverGenerator; the
// handler owns input validation + response shape and leaves
// provider selection / mock fallback to the agent.
type CoverHandler struct {
	gen agents.CoverGenerator
}

// NewCoverHandler wires a CoverHandler. The generator is
// required; callers who want a fully-disabled endpoint can
// pass a CoverClient constructed with the "mock" provider
// and an empty key. Logging uses slog.Default() (no logger
// injection point — the cover handler is small enough that
// the only thing the logger would do is forward to the
// default).
func NewCoverHandler(gen agents.CoverGenerator) *CoverHandler {
	return &CoverHandler{gen: gen}
}

// RegisterRoutes attaches the cover endpoint to the router.
func (h *CoverHandler) RegisterRoutes(r gin.IRouter) {
	r.POST("/ai/cover", h.GenerateCover)
}

// GenerateCover — POST /ai/cover
//
// Body: {title, style?, platform?, topic_id?}
// 200:  {image_url, prompt_used, provider, generation_time_ms}
// 400:  missing title
// 502:  provider returned a non-2xx (only when a real key is
//
//	configured; mock mode never returns 502)
//
// Demo mode (?demo=true or no API key) is handled inside
// the cover agent — the handler does not need to know
// whether the call hit a real provider or the mock. The
// response is identical in shape so the frontend renders
// both with the same code path.
func (h *CoverHandler) GenerateCover(c *gin.Context) {
	var req coverRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Default().Warn("invalid cover body", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}
	req.Style = strings.TrimSpace(req.Style)
	req.Platform = strings.TrimSpace(req.Platform)

	res, err := h.gen.Generate(c.Request.Context(), agents.CoverRequest{
		Title:    req.Title,
		Style:    req.Style,
		Platform: req.Platform,
		TopicID:  req.TopicID,
	})
	if err != nil {
		// Log the full upstream detail (which may include provider
		// URLs, key fragments, or stack-trace style messages) so
		// the operator can diagnose via logs, but do NOT echo it
		// to the public response body. The user-facing message
		// is a stable string that is safe to surface to a browser
		// or third-party consumer of the API.
		slog.Default().Error(
			"cover generation failed",
			"err", err.Error(),
			"provider", res.Provider,
			"request_id", c.GetString("request_id"),
		)
		c.JSON(http.StatusBadGateway, gin.H{
			"error":    "cover generation failed; see server logs",
			"prompt":   res.PromptUsed,
			"provider": res.Provider,
		})
		return
	}

	c.JSON(http.StatusOK, coverResponse{
		ImageURL:         res.ImageURL,
		PromptUsed:       res.PromptUsed,
		Provider:         res.Provider,
		GenerationTimeMs: res.GenerationTimeMs,
	})
}
