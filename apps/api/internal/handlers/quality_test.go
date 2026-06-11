package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/opc/api/internal/agents"
	"github.com/opc/api/internal/models"
)

// setupQualityTestRouter wires the quality handler with a
// MiniMax text-override so each test can pin the response.
// The DB is nil because the score/adapt endpoints don't touch it.
func setupQualityTestRouter(t *testing.T, fn textOverrideFn) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	m := agents.NewMiniMaxWithTextOverride(toTextResult(fn))
	r := gin.New()
	NewQualityHandler(m).RegisterRoutes(r)
	return r
}

// setupQualityDemoRouter wires the quality handler with a
// MiniMax client WITHOUT an API key so it defaults to demo mode.
// Tests that want to verify the canned panda-IP scores should use
// this.
func setupQualityDemoRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	m := agents.NewMiniMaxForDemo()
	r := gin.New()
	NewQualityHandler(m).RegisterRoutes(r)
	return r
}

// TestScoreDemoNoKey verifies the no-API-key + no-flag path
// returns canned data with the X-Demo-Mode header set.
func TestScoreDemoNoKey(t *testing.T) {
	r := setupQualityDemoRouter(t)
	w := doJSON(t, r, http.MethodPost, "/ai/score", map[string]any{
		"title":    "雨夜窗边的熊猫",
		"script":   "今天的雨,下得有点久。",
		"platform": "抖音",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-Demo-Mode"); got != "true" {
		t.Errorf("X-Demo-Mode header = %q, want %q", got, "true")
	}
	var resp qualityScoreResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, w.Body.String())
	}
	if resp.OverallScore < 1 || resp.OverallScore > 100 {
		t.Errorf("overall_score out of range: %d", resp.OverallScore)
	}
	if resp.HookStrength < 1 || resp.HookStrength > 100 {
		t.Errorf("hook_strength out of range: %d", resp.HookStrength)
	}
}

// TestScoreForcedDemoWithKey verifies ?demo=true overrides even
// when an API key is configured.
func TestScoreForcedDemoWithKey(t *testing.T) {
	m := agents.NewMiniMax("fake-key-for-tests")
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewQualityHandler(m).RegisterRoutes(r)

	w := doJSON(t, r, http.MethodPost, "/ai/score?demo=true", map[string]any{
		"title":    "标题",
		"script":   "脚本",
		"platform": "抖音",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-Demo-Mode"); got != "true" {
		t.Errorf("X-Demo-Mode header = %q, want %q", got, "true")
	}
}

// TestScoreRealPath verifies the override path runs and the
// response is parsed. We echo back a canned JSON and check the
// handler uses it. The suggestion uses the Phase-1
// {category, problem, rewrite, severity} shape so a future
// regression to the old {category, message, severity} contract
// would fail this test.
func TestScoreRealPath(t *testing.T) {
	overrideResp := `{
		"overall_score": 90,
		"hook_strength": 95,
		"structure": 88,
		"platform_fit": 87,
		"suggestions": [{"category":"hook","problem":"开头铺垫句偏多","rewrite":"换成具体画面钩子","severity":"low"}],
		"rewritten_hook": ""
	}`
	r := setupQualityTestRouter(t, func(_ context.Context, _ string) (string, error) {
		return overrideResp, nil
	})
	w := doJSON(t, r, http.MethodPost, "/ai/score", map[string]any{
		"title":    "好标题",
		"script":   "这是一段足够长的脚本用来测试整体评分。",
		"platform": "哔哩哔哩",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	var resp qualityScoreResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, w.Body.String())
	}
	if resp.OverallScore != 90 {
		t.Errorf("overall_score = %d, want 90", resp.OverallScore)
	}
	if len(resp.Suggestions) != 1 || resp.Suggestions[0].Category != "hook" {
		t.Errorf("suggestions = %+v, want one hook suggestion", resp.Suggestions)
	}
	// Pin the Phase-1 wire shape: problem + rewrite must round-trip.
	// The legacy `message` field was removed from the struct; a
	// regression that re-adds it would no longer compile and that
	// compile failure is itself the guard.
	if resp.Suggestions[0].Problem == "" {
		t.Errorf("suggestion.problem is empty, want non-empty (Phase-1 contract)")
	}
	if resp.Suggestions[0].Rewrite == "" {
		t.Errorf("suggestion.rewrite is empty, want non-empty (Phase-1 contract)")
	}
}

// TestScoreFallsBackOnParseError verifies that when Claude returns
// unparseable garbage, the handler degrades to rule-based scoring
// (200 + sensible numbers) rather than 502. This is the contract
// documented in the handler docstring.
func TestScoreFallsBackOnParseError(t *testing.T) {
	r := setupQualityTestRouter(t, func(_ context.Context, _ string) (string, error) {
		return "this is not json at all, sorry", nil
	})
	w := doJSON(t, r, http.MethodPost, "/ai/score", map[string]any{
		"title":    "好标题",
		"script":   "这是一段足够长的脚本用来测试整体评分。",
		"platform": "哔哩哔哩",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (fallback path), body = %s", w.Code, w.Body.String())
	}
	var resp qualityScoreResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, w.Body.String())
	}
	if resp.OverallScore < 1 {
		t.Errorf("fallback overall_score should be > 0, got %d", resp.OverallScore)
	}
	// Phase-1 QA: the X-Score-Fallback header is reserved for the
	// case where the rule-based fallback itself fails. When parse
	// fails but the fallback succeeds (the common case), the
	// header MUST be empty so the frontend does not show a
	// degraded banner for a normal fallback.
	if got := w.Header().Get(scoreFallbackHeader); got != "" {
		t.Errorf("%s = %q, want empty (fallback succeeded)", scoreFallbackHeader, got)
	}
}

// TestScoreFallsBackOnNoAPIKey: when the agent is real (key set)
// but the underlying call returns an upstream error, the handler
// MUST fall back to the rule-based scorer with the X-Demo-Mode
// header so the frontend can show a "demo" badge.
func TestScoreFallsBackOnUpstreamError(t *testing.T) {
	r := setupQualityTestRouter(t, func(_ context.Context, _ string) (string, error) {
		return "", fmt.Errorf("minimax: simulated upstream error")
	})
	w := doJSON(t, r, http.MethodPost, "/ai/score", map[string]any{
		"title":    "好标题",
		"script":   "这是一段足够长的脚本用来测试整体评分。",
		"platform": "哔哩哔哩",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (fallback path), body = %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-Demo-Mode"); got != "true" {
		t.Errorf("X-Demo-Mode header = %q, want %q (fallback to rule-based)", got, "true")
	}
}

// TestScoreInvalidBody: missing title or script must 400.
func TestScoreInvalidBody(t *testing.T) {
	r := setupQualityTestRouter(t, func(_ context.Context, _ string) (string, error) {
		t.Error("claude should not be called for invalid body")
		return "", nil
	})
	cases := []struct {
		name string
		body map[string]any
	}{
		{"missing title", map[string]any{"script": "x"}},
		{"missing script", map[string]any{"title": "x"}},
		{"empty title", map[string]any{"title": "  ", "script": "x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doJSON(t, r, http.MethodPost, "/ai/score", tc.body)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400, body = %s", w.Code, w.Body.String())
			}
		})
	}
}

// TestScoreStripsMarkdownFence: Claude wraps the JSON in
// ```json ... ``` fences; the handler must strip the wrapper.
func TestScoreStripsMarkdownFence(t *testing.T) {
	wrapped := "```json\n" +
		`{"overall_score":75,"hook_strength":80,"structure":70,"platform_fit":75,"suggestions":[],"rewritten_hook":""}` +
		"\n```"
	r := setupQualityTestRouter(t, func(_ context.Context, _ string) (string, error) {
		return wrapped, nil
	})
	w := doJSON(t, r, http.MethodPost, "/ai/score", map[string]any{
		"title":    "好标题",
		"script":   "这是一段足够长的脚本用来测试 fence strip。",
		"platform": "抖音",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	var resp qualityScoreResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, w.Body.String())
	}
	if resp.OverallScore != 75 {
		t.Errorf("overall_score = %d, want 75", resp.OverallScore)
	}
}

// ---------------------------------------------------------------------------
// /ai/platform-adapt
// ---------------------------------------------------------------------------

// TestPlatformAdaptDemoNoKey verifies the no-API-key + no-flag path
// returns canned 3-platform adaptations.
func TestPlatformAdaptDemoNoKey(t *testing.T) {
	r := setupQualityDemoRouter(t)
	w := doJSON(t, r, http.MethodPost, "/ai/platform-adapt", map[string]any{
		"title":           "雨夜窗边",
		"angle":           "治愈系",
		"source_platform": "小红书",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-Demo-Mode"); got != "true" {
		t.Errorf("X-Demo-Mode header = %q, want %q", got, "true")
	}
	var resp platformAdaptResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, w.Body.String())
	}
	if resp.Adaptations.Douyin.Title == "" {
		t.Errorf("抖音 adaptation title should be non-empty")
	}
	if resp.Adaptations.Bilibili.Title == "" {
		t.Errorf("哔哩哔哩 adaptation title should be non-empty")
	}
	if resp.Adaptations.Xiaohongshu.Title == "" {
		t.Errorf("小红书 adaptation title should be non-empty")
	}
	if len(resp.CrossPlatformTips) == 0 {
		t.Errorf("cross_platform_tips should be non-empty")
	}
}

// TestPlatformAdaptRealPath verifies the override path runs and
// the response is parsed and routed through the typed struct.
func TestPlatformAdaptRealPath(t *testing.T) {
	overrideResp := `{
		"adaptations": {
			"抖音": {"title":"短","hashtags":["#a"],"description":"d"},
			"哔哩哔哩": {"title":"长长长长","description":"d","tags":["t1","t2"]},
			"小红书": {"title":"短","body":"b","tags":["#a"]}
		},
		"cross_platform_tips": ["保持 IP 形象"]
	}`
	r := setupQualityTestRouter(t, func(_ context.Context, _ string) (string, error) {
		return overrideResp, nil
	})
	w := doJSON(t, r, http.MethodPost, "/ai/platform-adapt", map[string]any{
		"title":           "源标题",
		"angle":           "角度",
		"source_platform": "抖音",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	var resp platformAdaptResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, w.Body.String())
	}
	if len(resp.Adaptations.Bilibili.Tags) != 2 {
		t.Errorf("B 站 tags = %+v, want 2", resp.Adaptations.Bilibili.Tags)
	}
}

// TestPlatformAdaptInvalidBody: missing title or angle must 400.
func TestPlatformAdaptInvalidBody(t *testing.T) {
	r := setupQualityTestRouter(t, func(_ context.Context, _ string) (string, error) {
		t.Error("claude should not be called for invalid body")
		return "", nil
	})
	cases := []struct {
		name string
		body map[string]any
	}{
		{"missing title", map[string]any{"angle": "x"}},
		{"missing angle", map[string]any{"title": "x"}},
		{"empty title", map[string]any{"title": " ", "angle": "x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doJSON(t, r, http.MethodPost, "/ai/platform-adapt", tc.body)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400, body = %s", w.Code, w.Body.String())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// /ai/publish-checklist
// ---------------------------------------------------------------------------

// setupChecklistRouter builds a router + sqlite DB seeded with one
// Topic → Script → ContentItem chain. Returns the id of the
// content item so tests can post it. userID is used to scope the
// query; pass 0 for "no scoping" (test mode default).
func setupChecklistRouter(t *testing.T, userID uint, schedule *time.Time) (*gin.Engine, *gorm.DB, uint) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Topic{}, &models.Script{}, &models.ContentItem{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	topic := models.Topic{Title: "测试选题", Angle: "测试", Platform: "抖音", Status: "已发布", UserID: userID}
	if err := db.Create(&topic).Error; err != nil {
		t.Fatalf("create topic: %v", err)
	}
	script := models.Script{
		TopicID:  topic.ID,
		Title:    "测试标题",
		Content:  "窗边的熊猫,就那么坐着,看着外面的雨。今天的雨下得有点久。其实有时候吧,不需要说什么,陪着就够了。#话题 关注我们",
		Platform: "抖音",
		Tags:     "",
		UserID:   userID,
	}
	if err := db.Create(&script).Error; err != nil {
		t.Fatalf("create script: %v", err)
	}
	ci := models.ContentItem{
		ScriptID:    script.ID,
		Platform:    "抖音",
		PlatformURL: "https://example.com/v/1",
		UserID:      userID,
		ScheduledAt: schedule,
	}
	if err := db.Create(&ci).Error; err != nil {
		t.Fatalf("create ci: %v", err)
	}

	m := agents.NewMiniMaxForDemo()
	r := gin.New()
	NewQualityHandler(m, db).RegisterRoutes(r)
	return r, db, ci.ID
}

// TestPublishChecklistAllPass: a well-formed item yields a
// ready=true response with mostly pass checks.
func TestPublishChecklistAllPass(t *testing.T) {
	scheduled := time.Now().Add(24 * time.Hour)
	r, _, itemID := setupChecklistRouter(t, 0, &scheduled)
	w := doJSON(t, r, http.MethodPost, "/ai/publish-checklist", map[string]any{
		"content_item_id": itemID,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	var resp publishChecklistResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if !resp.Ready {
		t.Errorf("ready = false, want true; checks=%+v", resp.Checks)
	}
	if resp.OverallScore == 0 {
		t.Errorf("overall_score = 0, want > 0")
	}
	// Spot-check: every check should have a non-empty name and
	// message. This guards against a regression that silently
	// drops a check.
	for i, ch := range resp.Checks {
		if ch.Name == "" {
			t.Errorf("check[%d].name is empty", i)
		}
		if ch.Message == "" {
			t.Errorf("check[%d].message is empty", i)
		}
	}
}

// TestPublishChecklistNotFound: posting a non-existent id yields 404.
func TestPublishChecklistNotFound(t *testing.T) {
	r, _, _ := setupChecklistRouter(t, 0, nil)
	w := doJSON(t, r, http.MethodPost, "/ai/publish-checklist", map[string]any{
		"content_item_id": 9999,
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", w.Code, w.Body.String())
	}
}

// TestPublishChecklistInvalidBody: missing content_item_id must 400.
func TestPublishChecklistInvalidBody(t *testing.T) {
	r, _, _ := setupChecklistRouter(t, 0, nil)
	w := doJSON(t, r, http.MethodPost, "/ai/publish-checklist", map[string]any{})
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400, body = %s", w.Code, w.Body.String())
	}
}

// TestPublishChecklistNoSchedule: nil ScheduledAt must yield a
// warn (not fail) status, because the row is still draftable — the
// checklist is informational.
func TestPublishChecklistNoSchedule(t *testing.T) {
	r, _, itemID := setupChecklistRouter(t, 0, nil)
	w := doJSON(t, r, http.MethodPost, "/ai/publish-checklist", map[string]any{
		"content_item_id": itemID,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	var resp publishChecklistResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	found := false
	for _, ch := range resp.Checks {
		if ch.Name == "scheduled_time" {
			found = true
			if ch.Status != checkWarn {
				t.Errorf("scheduled_time status = %q, want %q (no schedule set)", ch.Status, checkWarn)
			}
		}
	}
	if !found {
		t.Errorf("expected scheduled_time check in response, got %+v", resp.Checks)
	}
}

// TestPublishChecklistRulesUnit pins the rule output to a known
// shape. Pure function test — no router, no DB.
func TestPublishChecklistRulesUnit(t *testing.T) {
	ci := &models.ContentItem{
		Platform: "抖音",
	}
	script := &models.Script{
		Title:   "短标题",
		Content: "足够长的脚本内容,带有 #话题 关注 CTA 标识。",
	}
	checks := runPublishChecks(ci, script)
	// We expect exactly 6 checks, in the documented order, so a
	// future refactor that adds/removes a check fails loudly.
	if len(checks) != 6 {
		t.Fatalf("len(checks) = %d, want 6: %+v", len(checks), checks)
	}
	wantOrder := []string{
		"title_length",
		"has_hashtags",
		"script_word_count",
		"has_call_to_action",
		"scheduled_time",
		"platform_specific",
	}
	for i, want := range wantOrder {
		if checks[i].Name != want {
			t.Errorf("checks[%d].name = %q, want %q", i, checks[i].Name, want)
		}
	}
}

// TestPublishChecklistOverLengthTitle: a 抖音 title > 22 chars
// must fail the title_length check.
func TestPublishChecklistOverLengthTitle(t *testing.T) {
	ci := &models.ContentItem{Platform: "抖音"}
	// 25-rune title to trigger the > 22 branch.
	script := &models.Script{Title: strings.Repeat("熊", 25), Content: "足够长"}
	checks := runPublishChecks(ci, script)
	var got publishCheck
	for _, ch := range checks {
		if ch.Name == "title_length" {
			got = ch
			break
		}
	}
	if got.Status != checkFail {
		t.Errorf("over-length 抖音 title status = %q, want %q: %+v", got.Status, checkFail, got)
	}
}

// TestPublishChecklistShortScript: a script under 50 chars must
// fail the script_word_count check.
func TestPublishChecklistShortScript(t *testing.T) {
	ci := &models.ContentItem{Platform: "小红书"}
	script := &models.Script{Title: "ok", Content: "短"}
	checks := runPublishChecks(ci, script)
	var got publishCheck
	for _, ch := range checks {
		if ch.Name == "script_word_count" {
			got = ch
			break
		}
	}
	if got.Status != checkFail {
		t.Errorf("short script status = %q, want %q: %+v", got.Status, checkFail, got)
	}
}

// TestPublishChecklistScriptMissing: when a ContentItem points at a
// Script id that doesn't exist, the handler must return 404 (not
// 200 with misleading "no hashtags" / "short script" fails from a
// zero-value Script).
func TestPublishChecklistScriptMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Topic{}, &models.Script{}, &models.ContentItem{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	// Insert a content item with a non-existent ScriptID (no
	// Script row in the DB).
	ci := models.ContentItem{
		ScriptID: 9999,
		Platform: "抖音",
	}
	if err := db.Create(&ci).Error; err != nil {
		t.Fatalf("create ci: %v", err)
	}
	m := agents.NewMiniMaxForDemo()
	r := gin.New()
	NewQualityHandler(m, db).RegisterRoutes(r)

	w := doJSON(t, r, http.MethodPost, "/ai/publish-checklist", map[string]any{
		"content_item_id": ci.ID,
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (script missing), body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "script") {
		t.Errorf("body should mention script, got: %s", w.Body.String())
	}
}

// TestClamp pins the package-private clamp helper. The
// qualityResponseFromMap test only exercises the happy path, so
// this table-driven test guards the > 100 and < lo branches added
// after the original clamp was extracted. If a future refactor
// silently inverts the bounds, this test fails loudly.
func TestClamp(t *testing.T) {
	cases := []struct {
		name            string
		v, lo, hi, want int
	}{
		{"in range", 50, 0, 100, 50},
		{"below lo", -5, 0, 100, 0},
		{"above hi", 150, 0, 100, 100},
		{"at lo", 0, 0, 100, 0},
		{"at hi", 100, 0, 100, 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := clamp(tc.v, tc.lo, tc.hi); got != tc.want {
				t.Errorf("clamp(%d, %d, %d) = %d, want %d", tc.v, tc.lo, tc.hi, got, tc.want)
			}
		})
	}
}

// TestCheckHasCTAMessageShape pins the user-facing message
// checkHasCTA produces. The matched CTA token is interpolated into
// the message, so a future CTA like "@" would render as "脚本含
// CTA(@)" — fine, but worth pinning. If a future refactor drops
// the matched token from the message (e.g. for a clean "CTA 存在"
// copy), this test will fail and force a deliberate update.
func TestCheckHasCTAMessageShape(t *testing.T) {
	cases := []struct {
		name       string
		content    string
		wantStatus publishCheckStatus
		wantSubstr string
	}{
		{"chinese CTA 关注", "欢迎大家关注我们频道", checkPass, "关注"},
		{"at mention", "请 @ 我们", checkPass, "@"},
		{"no CTA", "今天讲讲窗边的熊猫", checkWarn, "未发现明确 CTA"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := checkHasCTA(tc.content)
			if got.Status != tc.wantStatus {
				t.Errorf("status = %q, want %q (msg=%q)", got.Status, tc.wantStatus, got.Message)
			}
			if !strings.Contains(got.Message, tc.wantSubstr) {
				t.Errorf("message = %q, want substring %q", got.Message, tc.wantSubstr)
			}
		})
	}
}
