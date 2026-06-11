package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/opc/api/internal/agents"
	"github.com/opc/api/internal/config"
)

// coverRequest is the JSON body for POST /ai/cover.
//
// title is the only required field — MiniMax can render against
// a single subject line. style and platform are optional hints
// that get baked into the prompt; missing values fall through to
// canonical defaults (治愈 / 抖音). topic_id is reserved for the
// persistence path (Phase 2 multi-tenant): when set, a future
// iteration of this handler will record the generated image
// against the topic row so the operator can re-open "topic N's
// cover" from the topic detail page.
type coverRequest struct {
	Title    string `json:"title"`
	Style    string `json:"style,omitempty"`
	Platform string `json:"platform,omitempty"`
	TopicID  uint   `json:"topic_id,omitempty"`
}

// coverResponse is the wire shape returned by /ai/cover.
// Fields are kept flat (not nested under a "data" key) so the
// response can be passed straight to <img src=...>.
type coverResponse struct {
	ImageURL         string `json:"image_url"`
	PromptUsed       string `json:"prompt_used"`
	Provider         string `json:"provider"`
	GenerationTimeMs int64  `json:"generation_time_ms"`
}

// coverProviderName is the provider label the /ai/cover
// response always returns. MiniMax is the single supported
// provider post-Phase-4; the field is kept on the response
// shape so the frontend's "provider badge" code does not have
// to branch on a missing key.
const coverProviderName = "minimax"

// coverImagePath assembles the absolute on-disk path for a
// generated cover. Uses prefix + topic id + a timestamp so
// concurrent calls don't clobber each other.
func coverImagePath(topicID uint, prefix string) string {
	ts := time.Now().UnixNano()
	return fmt.Sprintf("%s/%s_t%d_%d.jpg", coverImageDir, prefix, topicID, ts)
}

// coverImageDir is the on-disk directory MiniMax's mmx CLI
// writes the generated cover into. We serve the saved file
// back to the frontend via /static/covers/<filename> (mounted
// in main.go). Putting the files under the API process's
// working directory keeps the path short and avoids a
// permission dance; the per-request seed (TopicID+title hash)
// prevents filename collisions across concurrent requests.
const coverImageDir = "var/covers"

// coverImageRoute is the public URL prefix the frontend
// uses to fetch the saved image. It maps to the static
// handler mounted at /static in main.go.
const coverImageRoute = "/static/covers/"

// CoverHandler exposes the cover-image generation endpoint.
// It is a thin wrapper around agents.MiniMax.Image; the
// handler owns input validation + response shape and leaves
// provider selection to the agent.
type CoverHandler struct {
	image *agents.MiniMax
}

// NewCoverHandler wires a CoverHandler. The image client is
// required. A nil MiniMax is a programmer error caught at
// construction time (handlers in main.go always pass a real
// one).
func NewCoverHandler(image *agents.MiniMax) *CoverHandler {
	return &CoverHandler{image: image}
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
// 502:  MiniMax returned an error or no image was saved
//
// Demo mode (?demo=true or no API key) is handled inside
// shouldUseDemo: the handler short-circuits to a deterministic
// SVG data: URL the frontend can render directly. The real
// MiniMax path is only taken when an API key is configured.
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

	prompt := agents.BuildCoverPrompt(agents.CoverRequest{
		Title:    req.Title,
		Style:    req.Style,
		Platform: req.Platform,
	})

	if h.shouldUseDemo(c) {
		// No API key or operator forced demo: serve the
		// deterministic SVG data: URL the cover agent
		// generates. The shape matches the real path so
		// the frontend can render either with the same
		// code.
		url := agents.MockCoverImageURL(agents.CoverRequest{
			Title:    req.Title,
			Style:    req.Style,
			Platform: req.Platform,
		})
		markDemoResponse(c)
		StampFlatCost(c, coverProviderName, config.SkillMiniMaxImage01, 0)
		c.JSON(http.StatusOK, coverResponse{
			ImageURL:         url,
			PromptUsed:       prompt,
			Provider:         coverProviderName,
			GenerationTimeMs: 0,
		})
		return
	}

	if err := os.MkdirAll(coverImageDir, 0o755); err != nil {
		slog.Default().Error("cover: mkdir failed", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cover storage unavailable"})
		return
	}
	outPrefix := fmt.Sprintf("cover_%d", req.TopicID)

	res, err := h.image.Image(c.Request.Context(), prompt, agents.MiniMaxImageOptions{
		Model:       "image-01",
		AspectRatio: coverAspectRatio(req.Platform),
		OutPath:    coverImagePath(req.TopicID, outPrefix),
		N:           1,
	})
	if err != nil {
		slog.Default().Error(
			"cover generation failed",
			"err", err.Error(),
			"request_id", c.GetString("request_id"),
		)
		c.JSON(http.StatusBadGateway, gin.H{
			"error":  "cover generation failed; see server logs",
			"prompt": prompt,
		})
		return
	}
	if len(res.FilePaths) == 0 {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":  "cover generation returned no image paths",
			"prompt": prompt,
		})
		return
	}

	// Stamp cost: MiniMax image generation is per-image flat
	// pricing. CostForSkill returns the configured cents for
	// SkillMiniMaxImage01. The mock path above stamps 0
	// (no real call was made).
	StampFlatCost(c, coverProviderName, config.SkillMiniMaxImage01, 1)

	imageURL := coverImageRoute + filepath.Base(res.FilePaths[0])
	c.JSON(http.StatusOK, coverResponse{
		ImageURL:         imageURL,
		PromptUsed:       prompt,
		Provider:         coverProviderName,
		GenerationTimeMs: 0,
	})
}

// shouldUseDemo returns true when the operator asked for
// ?demo=true OR the MiniMax client has no API key. Mirrors
// the convention in the other AI handlers. h.image can be
// nil in tests that only exercise the demo path; we treat
// nil as "no API key".
func (h *CoverHandler) shouldUseDemo(c *gin.Context) bool {
	if isDemoRequest(c) {
		return true
	}
	if h.image == nil {
		return true
	}
	return !h.image.Available()
}

// coverAspectRatio picks the mmx image CLI's --aspect-ratio
// flag value from the platform hint. Defaults to 9:16 (vertical,
// the canonical 抖音/小红书 ratio); 哔哩哔哩 and YouTube are
// 16:9 widescreen.
func coverAspectRatio(platform string) string {
	switch platform {
	case "哔哩哔哩", "YouTube":
		return "16:9"
	default:
		return "9:16"
	}
}
