package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/opc/api/internal/agents"
	"github.com/opc/api/internal/models"
)

// QualityClient extends AIClient with the two extra prompt methods
// the quality handler needs (score + platform-adapt). The existing
// AIHandler already has IsDemoMode / DemoResponse / Complete, so
// the only NEW methods are the two prompt builders and the two
// corresponding demo ops.
type QualityClient interface {
	AIClient
	ScoreContentPrompt(title, script, platform string) string
	PlatformAdaptPrompt(title, angle, sourcePlatform string) string
}

// QualityHandler exposes content-quality AI endpoints:
//   - POST /ai/score              — rule-based or AI 0-100 scoring
//   - POST /ai/platform-adapt     — adapt one idea to 抖音/哔哩哔哩/小红书
//   - POST /ai/publish-checklist  — pre-publish rule-based checks
//
// The first two follow the same demo/Claude split as the rest of
// the AI surface. The third is rule-based and always available.
type QualityHandler struct {
	claude QualityClient
	db     *gorm.DB
	logger *slog.Logger
}

// NewQualityHandler wires a QualityHandler. db is optional — the
// /ai/publish-checklist endpoint needs it (to look up Script +
// ContentItem); the other two endpoints are DB-free. Tests that
// only need score/adapt can pass nil.
func NewQualityHandler(claude QualityClient, db ...*gorm.DB) *QualityHandler {
	l := slog.Default()
	var d *gorm.DB
	if len(db) > 0 {
		d = db[0]
	}
	return &QualityHandler{claude: claude, db: d, logger: l}
}

// RegisterRoutes attaches the three quality endpoints.
func (h *QualityHandler) RegisterRoutes(r gin.IRouter) {
	r.POST("/ai/score", h.ScoreContent)
	r.POST("/ai/platform-adapt", h.PlatformAdapt)
	r.POST("/ai/publish-checklist", h.PublishChecklist)
}

// scoreFallbackHeader is the response header the /ai/score
// handler sets when the rule-based fallback path itself fails (so
// the frontend gets a clamped zero response with empty
// suggestions, but at least knows the score is unreliable).
// The X- prefix keeps it out of CORS preflight by default; if a
// future deployment crosses origins and needs to read it, add
// `Access-Control-Expose-Headers: X-Score-Fallback` to the CORS
// middleware.
//
// The possible values today are:
//   - "failed" — qualityResponseFromMap returned an error; the
//     response body is a zero-value qualityScoreResponse and the
//     frontend should show a degraded banner instead of a 0/0/0/0.
const scoreFallbackHeader = "X-Score-Fallback"

const (
	scoreFallbackFailed = "failed"
)

// ---------------------------------------------------------------------------
// /ai/score
// ---------------------------------------------------------------------------

// scoreRequest is the JSON body for POST /ai/score. All three fields
// are required; we trim whitespace before validating so a stray
// space doesn't 400 a real script.
type scoreRequest struct {
	Title    string `json:"title"`
	Script   string `json:"script"`
	Platform string `json:"platform"`
}

// qualitySuggestion matches the {category, problem, rewrite,
// severity} shape the upgraded ScoreContentPrompt and the demo
// pool (see DemoQualityScores in demo_data.go) produce. Each
// suggestion is a "before/after" pair: problem is what the
// heuristic flagged, rewrite is the concrete fix the user can paste
// back into their script. The legacy {category, message, severity}
// contract from the pre-Phase-1 wire shape is gone — both the
// rule-based fallback (buildSuggestions) and the live Claude path
// emit the same fields, so the frontend does not need to branch
// on which mode produced the response.
type qualitySuggestion struct {
	Category string `json:"category"`
	Problem  string `json:"problem"`
	Rewrite  string `json:"rewrite"`
	Severity string `json:"severity"`
}

// qualityScoreResponse is the wire shape returned by /ai/score.
type qualityScoreResponse struct {
	OverallScore  int                 `json:"overall_score"`
	HookStrength  int                 `json:"hook_strength"`
	Structure     int                 `json:"structure"`
	PlatformFit   int                 `json:"platform_fit"`
	Suggestions   []qualitySuggestion `json:"suggestions"`
	RewrittenHook string              `json:"rewritten_hook"`
}

// ScoreContent — POST /ai/score
//
// Body: {title, script, platform}
// 200:  {overall_score, hook_strength, structure, platform_fit, suggestions, rewritten_hook}
// 400:  missing title or script
// 503:  Claude is not configured (no API key) and no demo flag
//
// Behaviour:
//   - ?demo=true (or no API key): serve canned response
//   - else: ask Claude, parse JSON, fall back to rule-based scoring
//     on parse failure so the frontend never gets a 502 for a
//     slightly-off-shape response
func (h *QualityHandler) ScoreContent(c *gin.Context) {
	var req scoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("invalid score body", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	req.Script = strings.TrimSpace(req.Script)
	req.Platform = strings.TrimSpace(req.Platform)
	if req.Title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}
	if req.Script == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "script is required"})
		return
	}
	if req.Platform == "" {
		req.Platform = "抖音"
	}

	// Demo mode: return canned score for the panda-IP sample
	// script, regardless of the input. The seed is the script
	// length so consecutive demo calls with the same input get
	// the same output (predictable for the operator).
	if h.claude.IsDemoMode(isDemoRequest(c)) {
		raw, _ := h.claude.DemoResponse(agents.DemoOpScore, len(req.Script))
		out, err := parseQualityScore(raw)
		if err != nil {
			h.logger.Error("score demo parse failed", "err", err.Error(), "request_id", c.GetString("request_id"))
			c.JSON(http.StatusBadGateway, gin.H{"error": "AI demo response could not be parsed: " + err.Error()})
			return
		}
		markDemoResponse(c)
		c.JSON(http.StatusOK, out)
		return
	}

	prompt := h.claude.ScoreContentPrompt(req.Title, req.Script, req.Platform)

	ctx, cancel := context.WithTimeout(c.Request.Context(), aiTimeout)
	defer cancel()

	raw, err := h.claude.Complete(ctx, prompt)
	if err != nil {
		if errors.Is(err, agents.ErrNoAPIKey) {
			// No API key: fall back to the rule-based scorer so
			// the frontend still gets a useful answer (and a
			// clear header so it can show a "demo" badge).
			markDemoResponse(c)
			fb, fbErr := qualityResponseFromMap(agents.RuleBasedScore(req.Title, req.Script, req.Platform))
			if fbErr != nil {
				// rule-based map desync — log loudly but
				// still return a clamped zero response so
				// the frontend gets a usable 200. We also
				// set X-Score-Fallback=failed so the UI can
				// surface a degraded banner instead of a
				// silent 0/0/0/0.
				h.logger.Error("score rule-based fallback failed", "err", fbErr.Error(), "request_id", c.GetString("request_id"))
				fb = qualityScoreResponse{Suggestions: []qualitySuggestion{}}
				c.Header(scoreFallbackHeader, scoreFallbackFailed)
			}
			c.JSON(http.StatusOK, fb)
			return
		}
		h.logger.Error("score complete failed", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AI service failed: " + err.Error()})
		return
	}

	out, err := parseQualityScore(raw)
	if err != nil {
		// Parse failure: log it but degrade gracefully to
		// rule-based scoring. Better to give the user a
		// deterministic answer than a 502.
		h.logger.Warn("score parse failed, falling back to rule-based", "err", err.Error(), "request_id", c.GetString("request_id"))
		fb, fbErr := qualityResponseFromMap(agents.RuleBasedScore(req.Title, req.Script, req.Platform))
		if fbErr != nil {
			h.logger.Error("score rule-based fallback failed", "err", fbErr.Error(), "request_id", c.GetString("request_id"))
			fb = qualityScoreResponse{Suggestions: []qualitySuggestion{}}
			// Same X-Score-Fallback header as the no-key path so
			// the frontend can show a degraded banner.
			c.Header(scoreFallbackHeader, scoreFallbackFailed)
		}
		out = fb
	}
	c.JSON(http.StatusOK, out)
}

// parseQualityScore extracts a qualityScoreResponse from a raw
// Claude response. Same lenient approach as parseTopics /
// parsePostmortemStructured: strip a single ```json ... ```
// wrapper, then locate the first JSON object, then unmarshal.
func parseQualityScore(raw string) (qualityScoreResponse, error) {
	if len(raw) > maxAIResultBytes {
		return qualityScoreResponse{}, errors.New("response too large")
	}
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
	if !strings.HasPrefix(cleaned, "{") {
		loc := jsonObjectRE.FindStringIndex(cleaned)
		if loc == nil {
			return qualityScoreResponse{}, errors.New("no JSON object found in response")
		}
		cleaned = cleaned[loc[0]:loc[1]]
	}
	var out qualityScoreResponse
	if err := json.Unmarshal([]byte(cleaned), &out); err != nil {
		return qualityScoreResponse{}, err
	}
	return out, nil
}

// qualityResponseFromMap adapts the map[string]any returned by the
// rule-based fallback into the typed wire response. We do an
// intermediate JSON round-trip so the field name mapping is in one
// place and the rule-based helper stays a plain map.
//
// We do NOT swallow the marshal/unmarshal errors: if the rule-based
// output keys ever drift from the wire struct, the caller MUST hear
// about it (via the returned error) rather than silently get a
// zero-value response. The handler logs + returns the error and
// falls back to a zero-value typed response with clamped scores so
// the frontend still gets a usable 200, but the regression is loud
// for the operator.
func qualityResponseFromMap(m map[string]any) (qualityScoreResponse, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return qualityScoreResponse{}, fmt.Errorf("qualityResponseFromMap: marshal: %w", err)
	}
	var out qualityScoreResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return qualityScoreResponse{}, fmt.Errorf("qualityResponseFromMap: unmarshal: %w", err)
	}
	if out.Suggestions == nil {
		out.Suggestions = []qualitySuggestion{}
	}
	// Clamp scores to [0, 100] so a future heuristic tweak can't
	// return 137 to the frontend.
	out.OverallScore = clamp(out.OverallScore, 0, 100)
	out.HookStrength = clamp(out.HookStrength, 0, 100)
	out.Structure = clamp(out.Structure, 0, 100)
	out.PlatformFit = clamp(out.PlatformFit, 0, 100)
	return out, nil
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ---------------------------------------------------------------------------
// /ai/platform-adapt
// ---------------------------------------------------------------------------

// platformAdaptRequest is the JSON body for POST /ai/platform-adapt.
// source_platform is the platform the user is currently posting to
// (or "original" if no source); the response includes adaptations
// for all three target platforms.
type platformAdaptRequest struct {
	Title          string `json:"title"`
	Angle          string `json:"angle"`
	SourcePlatform string `json:"source_platform"`
}

// platformAdaptDouyin is the shape Claude is asked to return for
// the 抖音 adaptation.
type platformAdaptDouyin struct {
	Title       string   `json:"title"`
	Hashtags    []string `json:"hashtags"`
	Description string   `json:"description"`
}

// platformAdaptBilibili is the shape for 哔哩哔哩.
type platformAdaptBilibili struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

// platformAdaptXiaohongshu is the shape for 小红书.
type platformAdaptXiaohongshu struct {
	Title string   `json:"title"`
	Body  string   `json:"body"`
	Tags  []string `json:"tags"`
}

// platformAdaptations groups the three platform shapes. The JSON
// keys are the Chinese platform names so the frontend can index
// directly without a mapping table.
type platformAdaptations struct {
	Douyin      platformAdaptDouyin      `json:"抖音"`
	Bilibili    platformAdaptBilibili    `json:"哔哩哔哩"`
	Xiaohongshu platformAdaptXiaohongshu `json:"小红书"`
}

// platformAdaptResponse is the wire shape returned by
// /ai/platform-adapt. cross_platform_tips is a flat array of
// actionable "keep X consistent / adapt Y" lines.
type platformAdaptResponse struct {
	Adaptations       platformAdaptations `json:"adaptations"`
	CrossPlatformTips []string            `json:"cross_platform_tips"`
}

// PlatformAdapt — POST /ai/platform-adapt
//
// Body: {title, angle, source_platform}
// 200:  {adaptations: {抖音, 哔哩哔哩, 小红书}, cross_platform_tips}
// 400:  missing title or angle
// 503:  Claude is not configured and no demo flag
func (h *QualityHandler) PlatformAdapt(c *gin.Context) {
	var req platformAdaptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("invalid platform-adapt body", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	req.Angle = strings.TrimSpace(req.Angle)
	req.SourcePlatform = strings.TrimSpace(req.SourcePlatform)
	if req.Title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}
	if req.Angle == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "angle is required"})
		return
	}
	if req.SourcePlatform == "" {
		req.SourcePlatform = "原创"
	}

	// Demo mode: serve pre-canned 3-platform adaptations.
	if h.claude.IsDemoMode(isDemoRequest(c)) {
		raw, _ := h.claude.DemoResponse(agents.DemoOpPlatformAdapt, len(req.Title))
		out, err := parsePlatformAdapt(raw)
		if err != nil {
			h.logger.Error("platform-adapt demo parse failed", "err", err.Error(), "request_id", c.GetString("request_id"))
			c.JSON(http.StatusBadGateway, gin.H{"error": "AI demo response could not be parsed: " + err.Error()})
			return
		}
		markDemoResponse(c)
		c.JSON(http.StatusOK, out)
		return
	}

	prompt := h.claude.PlatformAdaptPrompt(req.Title, req.Angle, req.SourcePlatform)

	ctx, cancel := context.WithTimeout(c.Request.Context(), aiTimeout)
	defer cancel()

	raw, err := h.claude.Complete(ctx, prompt)
	if err != nil {
		if errors.Is(err, agents.ErrNoAPIKey) {
			h.logger.Warn("platform-adapt: no API key configured", "request_id", c.GetString("request_id"))
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "AI service unavailable: ANTHROPIC_API_KEY not configured on the server",
			})
			return
		}
		h.logger.Error("platform-adapt complete failed", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AI service failed: " + err.Error()})
		return
	}

	out, err := parsePlatformAdapt(raw)
	if err != nil {
		h.logger.Error("platform-adapt parse failed", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusBadGateway, gin.H{"error": "AI returned output that could not be parsed: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

// parsePlatformAdapt extracts a platformAdaptResponse from a raw
// Claude response. Same markdown-fence stripping as the other
// parsers.
func parsePlatformAdapt(raw string) (platformAdaptResponse, error) {
	if len(raw) > maxAIResultBytes {
		return platformAdaptResponse{}, errors.New("response too large")
	}
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
	if !strings.HasPrefix(cleaned, "{") {
		loc := jsonObjectRE.FindStringIndex(cleaned)
		if loc == nil {
			return platformAdaptResponse{}, errors.New("no JSON object found in response")
		}
		cleaned = cleaned[loc[0]:loc[1]]
	}
	var out platformAdaptResponse
	if err := json.Unmarshal([]byte(cleaned), &out); err != nil {
		return platformAdaptResponse{}, err
	}
	if out.CrossPlatformTips == nil {
		out.CrossPlatformTips = []string{}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// /ai/publish-checklist
// ---------------------------------------------------------------------------

// publishChecklistRequest is the JSON body for POST
// /ai/publish-checklist. content_item_id is required; the handler
// joins through to Script and ContentItem to gather the fields
// needed for the rule-based checks.
type publishChecklistRequest struct {
	ContentItemID uint `json:"content_item_id"`
}

// publishCheckStatus is the small enum the frontend renders as
// pass / warn / fail badges.
type publishCheckStatus string

const (
	checkPass publishCheckStatus = "pass"
	checkWarn publishCheckStatus = "warn"
	checkFail publishCheckStatus = "fail"
)

// publishCheck is one row in the response. Severity is implicit in
// the status; we keep the type narrow (not a free-form string) so
// the frontend never has to defensively parse it.
type publishCheck struct {
	Name    string             `json:"name"`
	Status  publishCheckStatus `json:"status"`
	Message string             `json:"message"`
}

// publishChecklistResponse is the wire shape for
// /ai/publish-checklist. ready = true only when there are zero
// fail-status checks. overall_score is a 0-100 rollup that the
// frontend can use as a quick health gauge.
type publishChecklistResponse struct {
	Ready        bool           `json:"ready"`
	Checks       []publishCheck `json:"checks"`
	OverallScore int            `json:"overall_score"`
}

// PublishChecklist — POST /ai/publish-checklist
//
// Body: {content_item_id}
// 200:  {ready, checks[], overall_score}
// 400:  missing content_item_id
// 404:  content item not found
// 500:  handler wired without a DB
//
// This endpoint is always available (no Claude, no demo flag) — it
// runs the rule-based checklist inline.
func (h *QualityHandler) PublishChecklist(c *gin.Context) {
	var req publishChecklistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("invalid publish-checklist body", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if req.ContentItemID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content_item_id is required"})
		return
	}
	if h.db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "publish-checklist requires database access"})
		return
	}

	userID := UserIDFromContext(c)
	var ci models.ContentItem
	q := h.db.WithContext(withTimeout(c.Request.Context())).Model(&models.ContentItem{})
	if userID != 0 {
		q = q.Where("user_id = ?", userID)
	}
	if err := q.First(&ci, req.ContentItemID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "content item not found"})
			return
		}
		h.logger.Error("publish-checklist: content item lookup failed", "err", err.Error(), "id", req.ContentItemID, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to look up content item"})
		return
	}

	var script models.Script
	if err := h.db.WithContext(withTimeout(c.Request.Context())).First(&script, ci.ScriptID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// A ContentItem with no backing Script is a data
			// integrity issue. Surface it as 404 with a distinct
			// message rather than proceeding with a zero-value
			// Script, which would yield misleading "no hashtags"
			// / "short script" failures and hide the real bug.
			h.logger.Error("publish-checklist: script missing for content item", "content_item_id", req.ContentItemID, "script_id", ci.ScriptID, "request_id", c.GetString("request_id"))
			c.JSON(http.StatusNotFound, gin.H{"error": "script for content item not found"})
			return
		}
		h.logger.Error("publish-checklist: script lookup failed", "err", err.Error(), "script_id", ci.ScriptID, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to look up script"})
		return
	}

	checks := runPublishChecks(&ci, &script)
	overall := 0
	failCount := 0
	for _, ch := range checks {
		overall += scoreForCheck(ch.Status)
		if ch.Status == checkFail {
			failCount++
		}
	}
	resp := publishChecklistResponse{
		Ready:        failCount == 0,
		Checks:       checks,
		OverallScore: overall,
	}
	c.JSON(http.StatusOK, resp)
}

// scoreForCheck assigns a 0-100 contribution to a single check
// status, used to compute the overall rollup. Pass = full credit,
// warn = half, fail = zero. The maximum is therefore 100 * #checks,
// normalized elsewhere if needed (today the frontend just displays
// the raw sum, which keeps the math simple and the trend visible).
func scoreForCheck(s publishCheckStatus) int {
	switch s {
	case checkPass:
		return 100
	case checkWarn:
		return 50
	default:
		return 0
	}
}

// runPublishChecks runs every rule on the content item + script and
// returns the per-check verdict list. The function is pure (no
// network, no DB) so it can be unit-tested directly.
func runPublishChecks(ci *models.ContentItem, script *models.Script) []publishCheck {
	checks := []publishCheck{
		checkTitleLength(ci.Platform, script.Title),
		checkHasHashtags(script),
		checkScriptLength(script.Content),
		checkHasCTA(script.Content),
		checkScheduled(ci.ScheduledAt),
		checkPlatformSpecific(ci.Platform, script),
	}
	return checks
}

// checkTitleLength verifies the title length is within the
// per-platform convention. 抖音 caps at 22, B站 at 80, 小红书 at
// 20 (rune count, not byte count — Chinese chars count as 1).
func checkTitleLength(platform, title string) publishCheck {
	runes := utf8.RuneCountInString(strings.TrimSpace(title))
	switch platform {
	case "抖音":
		if runes == 0 {
			return publishCheck{Name: "title_length", Status: checkFail, Message: "标题为空,抖音必须有标题"}
		}
		if runes <= 22 {
			return publishCheck{Name: "title_length", Status: checkPass, Message: "标题长度合规(≤22 字)"}
		}
		return publishCheck{Name: "title_length", Status: checkFail, Message: "标题超过 22 字,会被抖音截断"}
	case "哔哩哔哩":
		if runes == 0 {
			return publishCheck{Name: "title_length", Status: checkFail, Message: "标题为空,B 站必须有标题"}
		}
		if runes <= 80 {
			return publishCheck{Name: "title_length", Status: checkPass, Message: "标题长度合规(≤80 字)"}
		}
		return publishCheck{Name: "title_length", Status: checkWarn, Message: "标题超过 80 字,搜索关键词可能埋得太深"}
	case "小红书":
		if runes == 0 {
			return publishCheck{Name: "title_length", Status: checkFail, Message: "标题为空,小红书必须有标题"}
		}
		if runes <= 20 {
			return publishCheck{Name: "title_length", Status: checkPass, Message: "标题长度合规(≤20 字)"}
		}
		return publishCheck{Name: "title_length", Status: checkFail, Message: "标题超过 20 字,小红书会折叠"}
	default:
		// Unknown platform — neutral warn so the checklist still
		// returns something useful.
		return publishCheck{Name: "title_length", Status: checkWarn, Message: "未知平台,无法判断标题长度惯例"}
	}
}

// checkHasHashtags looks for "#" markers in the script or tags
// field. A "tags" field with comma-separated values also counts
// because OPC's Script model stores tags as a single string.
func checkHasHashtags(script *models.Script) publishCheck {
	if strings.Contains(script.Content, "#") {
		return publishCheck{Name: "has_hashtags", Status: checkPass, Message: "脚本内含 # 标签"}
	}
	if strings.TrimSpace(script.Tags) != "" {
		return publishCheck{Name: "has_hashtags", Status: checkPass, Message: "已配置标签"}
	}
	return publishCheck{Name: "has_hashtags", Status: checkWarn, Message: "未发现 hashtag 或 tags,建议至少 1 个话题标签"}
}

// checkScriptLength verifies the script body is over 50 characters
// (a rough proxy for "long enough to make a video"). Short
// scripts fail; medium scripts pass with a note.
func checkScriptLength(content string) publishCheck {
	runes := utf8.RuneCountInString(strings.TrimSpace(content))
	switch {
	case runes == 0:
		return publishCheck{Name: "script_word_count", Status: checkFail, Message: "脚本为空,无法生成视频"}
	case runes < 50:
		return publishCheck{Name: "script_word_count", Status: checkFail, Message: "脚本字数 < 50,可能撑不满一个短视频"}
	case runes < 80:
		return publishCheck{Name: "script_word_count", Status: checkWarn, Message: "脚本字数偏少,建议补充到 80-300 字"}
	default:
		return publishCheck{Name: "script_word_count", Status: checkPass, Message: "脚本长度合适"}
	}
}

// checkHasCTA looks for explicit call-to-action markers in the
// script: "关注", "点赞", "评论", "收藏", "转发", or "@". The
// presence of a CTA is a soft check (warn, not fail) because some
// IP voices deliberately omit CTAs.
func checkHasCTA(content string) publishCheck {
	ctas := []string{"关注", "点赞", "评论", "收藏", "转发", "@", "留言"}
	for _, cta := range ctas {
		if strings.Contains(content, cta) {
			return publishCheck{Name: "has_call_to_action", Status: checkPass, Message: "脚本含 CTA(" + cta + ")"}
		}
	}
	return publishCheck{Name: "has_call_to_action", Status: checkWarn, Message: "未发现明确 CTA,可能影响互动率"}
}

// checkScheduled verifies the content item has a ScheduledAt set
// (and that it's in the future). Past dates are flagged as warn
// because the item should have been published or re-scheduled.
func checkScheduled(scheduled *time.Time) publishCheck {
	if scheduled == nil {
		return publishCheck{Name: "scheduled_time", Status: checkWarn, Message: "未设置发布时间,发布后平台加权较低"}
	}
	if scheduled.Before(time.Now()) {
		return publishCheck{Name: "scheduled_time", Status: checkWarn, Message: "发布时间已过,请重新排期或标记为已发布"}
	}
	return publishCheck{Name: "scheduled_time", Status: checkPass, Message: "发布时间已设置"}
}

// checkPlatformSpecific adds one platform-specific sanity check.
// Today this is just the 抖音 vertical-video hint, but the
// function is structured to grow without changing the dispatcher.
func checkPlatformSpecific(platform string, script *models.Script) publishCheck {
	switch platform {
	case "抖音":
		// 抖音 is vertical video only. The OPC content_item model
		// doesn't store aspect-ratio metadata, so we use the script
		// length as a proxy: scripts > 600 chars almost certainly
		// need a different format than a 15-30s vertical clip.
		runes := utf8.RuneCountInString(script.Content)
		if runes > 600 {
			return publishCheck{Name: "platform_specific", Status: checkWarn, Message: "抖音竖屏视频一般 ≤30s,长脚本需拆条或转横屏"}
		}
		return publishCheck{Name: "platform_specific", Status: checkPass, Message: "抖音竖屏视频长度合适"}
	case "哔哩哔哩":
		return publishCheck{Name: "platform_specific", Status: checkPass, Message: "B 站可长可短,记得加封面和分区"}
	case "小红书":
		if strings.TrimSpace(script.Content) == "" {
			return publishCheck{Name: "platform_specific", Status: checkFail, Message: "小红书必须有正文(body),不能只发标题"}
		}
		return publishCheck{Name: "platform_specific", Status: checkPass, Message: "小红书正文非空,可发布"}
	default:
		return publishCheck{Name: "platform_specific", Status: checkWarn, Message: "未知平台,跳过平台特定检查"}
	}
}
