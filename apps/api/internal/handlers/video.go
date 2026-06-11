package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/opc/api/internal/agents"
	"github.com/opc/api/internal/config"
)

// videoRequest is the JSON body for POST /api/video.
//
// prompt is the only required field. first_frame / last_frame
// unlock image-to-video (I2V) and start-end-frame (SEF) modes
// — when set, mmx uses the supplied frame as the opening
// still or the start/end bookends. subject_image is the
// subject-reference (S2V) image the model should anchor the
// subject's identity against. model selects the underlying
// video model; out_dir lets the operator persist the
// generated mp4 somewhere other than the default
// var/videos/ directory.
//
// Note: the URL field naming follows the task spec
// (first_frame, last_frame, subject_image, out_dir) so the
// frontend and the docs page stay in sync — internally the
// mmx CLI uses slightly different flag names and we map at
// the call site.
type videoRequest struct {
	Prompt     string `json:"prompt"`
	FirstFrame string `json:"first_frame,omitempty"`
	LastFrame  string `json:"last_frame,omitempty"`
	SubjectImg string `json:"subject_image,omitempty"`
	Model      string `json:"model,omitempty"`
	OutDir     string `json:"out_dir,omitempty"`
}

// videoResponse is the wire shape returned by /api/video.
// path is the on-disk path (relative to the API working
// directory) the saved mp4 lives at. provider is always
// "minimax" today — kept on the shape so the UI's
// provider-badge code does not have to branch on a missing
// key.
//
// We intentionally do not echo a duration / size on the
// response today — the mmx CLI does not surface them and
// we'd be hand-rolling ffprobe. The frontend can fetch the
// file metadata off the saved path once the static handler
// from #85 lands.
type videoResponse struct {
	Path     string `json:"path"`
	Provider string `json:"provider"`
}

// videoProviderName is the provider label the /api/video
// response always returns.
const videoProviderName = "minimax"

// videoOutDir is the on-disk directory MiniMax's mmx CLI
// writes the generated mp4 into. The frontend will be able
// to fetch the saved file via a static handler mounted in a
// follow-up PR (#85); for now the path is returned directly
// and the operator can scp it off the API box.
const videoOutDir = "var/videos"

// videoOutPrefix is the file-name prefix the video handler
// uses when asking the mmx CLI to save the result. Keeps the
// saved files grouped so an `ls` of var/videos/ tells the
// operator what each one is.
const videoOutPrefix = "video"

// videoTimeout caps how long a single /api/video call can
// wait on the underlying MiniMax video task. The mmx client
// already polls internally (5s cadence) up to its own
// MaxWait; the outer budget is defense-in-depth and
// prevents a stuck async task from holding a Gin handler
// for longer than the http.Server.WriteTimeout in main.go.
const videoTimeout = 5 * time.Minute

// VideoHandler exposes the video-generation endpoint. It is
// a thin wrapper around agents.MiniMax.Video; the handler
// owns input validation + response shape and leaves
// provider selection to the agent. The same MiniMax
// instance serves text/image/speech/video — Video is one
// method on the same client the AI/cover/speech handlers
// use.
type VideoHandler struct {
	text *agents.MiniMax
}

// NewVideoHandler wires a VideoHandler. The MiniMax client
// is required; a nil one is a programmer error caught at
// construction time (main.go always passes a real
// instance).
func NewVideoHandler(text *agents.MiniMax) *VideoHandler {
	return &VideoHandler{text: text}
}

// RegisterRoutes attaches the video endpoint to the router.
// Mounted at /api/video to match the rest of the protected
// /api/* surface; the call_log middleware (mounted in
// main.go) automatically picks up the row.
func (h *VideoHandler) RegisterRoutes(r gin.IRouter) {
	r.POST("/video", h.Render)
}

// Render — POST /api/video
//
// Body: {prompt, first_frame?, last_frame?, subject_image?, model?, out_dir?}
// 200:  {path, provider}
// 400:  missing prompt
// 502:  video generation failed
// 503:  API key not configured
// 504:  upstream video task did not complete within the
//
//	allotted timeout
//
// Video generation is async inside the mmx CLI: the CLI
// submits the task, then polls. The handler also wraps the
// call in a context timeout so a stuck task surfaces as a
// 504 instead of holding the connection open.
func (h *VideoHandler) Render(c *gin.Context) {
	var req videoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Default().Warn("invalid video body", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Prompt == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "prompt is required"})
		return
	}
	req.FirstFrame = strings.TrimSpace(req.FirstFrame)
	req.LastFrame = strings.TrimSpace(req.LastFrame)
	req.SubjectImg = strings.TrimSpace(req.SubjectImg)
	req.Model = strings.TrimSpace(req.Model)
	req.OutDir = strings.TrimSpace(req.OutDir)

	if !h.text.Available() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "video service unavailable: MINIMAX_API_KEY not configured",
		})
		return
	}

	outDir := req.OutDir
	if outDir == "" {
		outDir = videoOutDir
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		slog.Default().Error("video: mkdir failed", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "video storage unavailable"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), videoTimeout)
	defer cancel()

	res, err := h.text.Video(ctx, req.Prompt, agents.MiniMaxVideoOptions{
		Model:      req.Model,
		FirstFrame: req.FirstFrame,
		LastFrame:  req.LastFrame,
		SubjectImg: req.SubjectImg,
		OutPath:    defaultVideoOutPath(outDir),
		MaxWait:    videoTimeout,
	})
	if err != nil {
		// Distinguish the three failure modes the spec
		// calls out:
		//   - context-deadline (504): the upstream never
		//     completed within the budget
		//   - auth-shaped (503): the mmx CLI refused on
		//     key/quota grounds
		//   - everything else (502): a generic provider
		//     failure
		if errors.Is(err, context.DeadlineExceeded) {
			slog.Default().Warn(
				"video generation timed out",
				"prompt_len", len(req.Prompt),
				"request_id", c.GetString("request_id"),
			)
			c.JSON(http.StatusGatewayTimeout, gin.H{
				"error": "video generation timed out; try a shorter prompt or try again",
			})
			return
		}
		if isAPIKeyError(err) {
			slog.Default().Error("video: api key error", "err", err.Error(), "request_id", c.GetString("request_id"))
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "video service unavailable: " + err.Error(),
			})
			return
		}
		slog.Default().Error(
			"video generation failed",
			"err", err.Error(),
			"request_id", c.GetString("request_id"),
		)
		// Surface the upstream message (e.g. "usage limit
		// exceeded") instead of a generic "see server logs" so
		// the operator knows whether to retry, wait for quota
		// reset, or call the provider. Truncate to keep the
		// response body small.
		msg := err.Error()
		if len(msg) > 400 {
			msg = msg[:400] + "..."
		}
		c.JSON(http.StatusBadGateway, gin.H{
			"error":  "video generation failed: " + msg,
			"detail": msg,
		})
		return
	}
	if res == nil || res.Path == "" {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": "video generation returned no file path",
		})
		return
	}

	// Stamp cost: MiniMax video bills flat per 5s clip. We
	// stamp 1 unit so CostForSkill returns the configured
	// flat rate (100 cents) regardless of the actual
	// duration — the rate is set in clips, not seconds,
	// and the request itself is the unit of billing.
	StampFlatCost(c, videoProviderName, config.SkillMiniMaxVideo, 1)

	c.JSON(http.StatusOK, videoResponse{
		Path:     res.Path,
		Provider: videoProviderName,
	})
}

// defaultVideoOutPath builds a deterministic absolute path
// the mmx CLI will write the mp4 to. We use a timestamped
// filename under outDir; the path is what the handler
// returns to the frontend (the future static handler will
// expose var/videos/<filename> over HTTP).
func defaultVideoOutPath(outDir string) string {
	return outDir + "/" + videoOutPrefix + ".mp4"
}
