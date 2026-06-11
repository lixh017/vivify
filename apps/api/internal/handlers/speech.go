package handlers

import (
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/opc/api/internal/agents"
	"github.com/opc/api/internal/config"
)

// speechRequest is the JSON body for POST /api/speech.
//
// text is the only required field. The remaining fields are
// optional tuning knobs that map 1:1 to the MiniMax TTS API:
// voice selects the speaker, speed/volume/pitch scale the
// corresponding acoustic dimensions, and language boosts
// pronunciation. out_dir lets the operator persist the
// generated mp3 somewhere other than the default var/speech/
// directory (e.g. a per-topic staging area).
type speechRequest struct {
	Text     string  `json:"text"`
	Voice    string  `json:"voice,omitempty"`
	Speed    float64 `json:"speed,omitempty"`
	Volume   float64 `json:"volume,omitempty"`
	Pitch    float64 `json:"pitch,omitempty"`
	Language string  `json:"language,omitempty"`
	OutDir   string  `json:"out_dir,omitempty"`
}

// speechResponse is the wire shape returned by /api/speech.
// path is the on-disk path (relative to the API working
// directory) the saved mp3 lives at. DurationMs / SizeBytes
// / SampleRate are echoed from the MiniMax provider so the
// frontend can build a waveform preview without re-fetching
// the file. provider is always "minimax" today — kept on the
// shape so the UI's provider-badge code does not have to
// branch on a missing key.
type speechResponse struct {
	Path       string `json:"path"`
	DurationMs int    `json:"duration_ms"`
	SizeBytes  int    `json:"size_bytes"`
	SampleRate int    `json:"sample_rate"`
	Provider   string `json:"provider"`
}

// speechProviderName is the provider label the /api/speech
// response always returns.
const speechProviderName = "minimax"

// speechOutDir is the on-disk directory MiniMax's mmx CLI
// writes the generated mp3 into. We serve the saved file
// back to the frontend via a static handler mounted in a
// follow-up PR (#85); for now the path is returned directly
// and the operator can curl it off the API box.
const speechOutDir = "var/speech"

// speechOutPrefix is the file-name prefix the speech handler
// uses when asking the mmx CLI to save the result. Keeps the
// saved files grouped so an `ls` of var/speech/ tells the
// operator what each one is.
const speechOutPrefix = "speech"

// SpeechHandler exposes the TTS (text-to-speech) endpoint.
// It is a thin wrapper around agents.MiniMax.Speech; the
// handler owns input validation + response shape and leaves
// provider selection to the agent. The same MiniMax instance
// serves text/image/speech/video — Speech is one method on
// the same client the AI/cover handlers use.
type SpeechHandler struct {
	text *agents.MiniMax
}

// NewSpeechHandler wires a SpeechHandler. The MiniMax
// client is required; a nil one is a programmer error caught
// at construction time (main.go always passes a real
// instance).
func NewSpeechHandler(text *agents.MiniMax) *SpeechHandler {
	return &SpeechHandler{text: text}
}

// RegisterRoutes attaches the speech endpoint to the router.
// Mounted at /api/speech to match the rest of the protected
// /api/* surface; the call_log middleware (mounted in
// main.go) automatically picks up the row.
func (h *SpeechHandler) RegisterRoutes(r gin.IRouter) {
	r.POST("/speech", h.Synthesize)
}

// Synthesize — POST /api/speech
//
// Body: {text, voice?, speed?, volume?, pitch?, language?, out_dir?}
// 200:  {path, duration_ms, size_bytes, sample_rate, provider}
// 400:  missing text
// 502:  TTS generation failed (MiniMax error or empty result)
// 503:  API key not configured (no demo path for TTS — speech
//
//	output is binary and cannot be canned)
//
// The response path is returned as a server-relative path
// the frontend will be able to play once the static handler
// from #85 lands. Until then, operators can fetch the file
// off the API box.
func (h *SpeechHandler) Synthesize(c *gin.Context) {
	var req speechRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Default().Warn("invalid speech body", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "text is required"})
		return
	}
	req.Voice = strings.TrimSpace(req.Voice)
	req.Language = strings.TrimSpace(req.Language)
	req.OutDir = strings.TrimSpace(req.OutDir)

	if !h.text.Available() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "TTS service unavailable: MINIMAX_API_KEY not configured",
		})
		return
	}

	outDir := req.OutDir
	if outDir == "" {
		outDir = speechOutDir
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		slog.Default().Error("speech: mkdir failed", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "speech storage unavailable"})
		return
	}

	res, err := h.text.Speech(c.Request.Context(), req.Text, agents.MiniMaxSpeechOptions{
		Voice:    req.Voice,
		Speed:    req.Speed,
		Volume:   req.Volume,
		Pitch:    req.Pitch,
		Language: req.Language,
		OutPath:  defaultSpeechOutPath(outDir),
	})
	if err != nil {
		slog.Default().Error(
			"speech synthesis failed",
			"err", err.Error(),
			"request_id", c.GetString("request_id"),
		)
		// Distinguish "API key not configured at the CLI layer"
		// (which surfaces as a non-zero exit / parse error here)
		// from "real provider failure" — both end up 502 today
		// because mmx does not give us a clean error code, but
		// the log line lets the operator triage.
		if isAPIKeyError(err) {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "TTS service unavailable: " + err.Error(),
			})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{
			"error": "speech synthesis failed; see server logs",
		})
		return
	}
	if res == nil || res.Path == "" {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": "speech synthesis returned no file path",
		})
		return
	}

	// Convert the on-disk path to a URL the frontend can hit.
	// res.Path is relative (e.g. "var/speech/speech.mp3");
	// strip the leading "var/" so the URL is the path under
	// the /static mount.
	urlPath := staticURLFor(res.Path)

	// Stamp cost: MiniMax TTS bills per 1k chars. CostForSkill
	// uses the configured SkillMiniMaxTTS28 row (1 cent / 1k
	// chars). We charge against the request text length rather
	// than the audio duration — the rate is set in characters,
	// not seconds, and the request body is the authoritative
	// unit count the provider bills.
	StampFlatCost(c, speechProviderName, config.SkillMiniMaxTTS28, 1)

	c.JSON(http.StatusOK, speechResponse{
		Path:       urlPath,
		DurationMs: res.DurationMs,
		SizeBytes:  res.SizeBytes,
		SampleRate: res.SampleRate,
		Provider:   speechProviderName,
	})
}

// defaultSpeechOutPath builds a deterministic absolute path
// the mmx CLI will write the mp3 to. We use a timestamped
// filename under outDir; the path is what the handler
// returns to the frontend (the future static handler will
// expose var/speech/<filename> over HTTP).
func defaultSpeechOutPath(outDir string) string {
	return outDir + "/" + speechOutPrefix + ".mp3"
}

// staticURLFor converts an on-disk path under the working
// directory's "var/" folder to the public /static/... URL.
// Returns the original string if it doesn't start with "var/"
// so a future test that injects an absolute path doesn't get
// silently rewritten.
func staticURLFor(diskPath string) string {
	const prefix = "var/"
	if !strings.HasPrefix(diskPath, prefix) {
		return diskPath
	}
	return "/static/" + diskPath[len(prefix):]
}

// isAPIKeyError returns true when the underlying mmx CLI
// failure looks like an auth / key problem. The CLI does not
// surface a structured error code, so we pattern-match on
// the stderr string. Used to promote a 502 into a 503 (the
// distinction matters for the operator dashboard — 503 means
// "not configured" and is the right code for a missing
// key).
func isAPIKeyError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{
		"api key",
		"unauthorized",
		"401",
		"403",
		"minimax_api_key",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}
