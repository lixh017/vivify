package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/opc/api/internal/agents"
	"github.com/opc/api/internal/models"
)

func setupAITestRouter(t *testing.T, override agents.CompleteFunc) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	claude := agents.NewClaudeWithOverride(override)
	r := gin.New()
	h := NewAIHandler(claude)
	h.RegisterRoutes(r)
	return r
}

func doJSON(t *testing.T, r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestGenerateTopicsSuccess: override returns a clean JSON array and
// the handler must parse + return it. This is the happy path.
func TestGenerateTopicsSuccess(t *testing.T) {
	overrideResp := `[
		{"title":"标题A","angle":"角度A","expected_performance":"表现A","hook":"钩子A"},
		{"title":"标题B","angle":"角度B","expected_performance":"表现B","hook":"钩子B"}
	]`
	r := setupAITestRouter(t, func(_ context.Context, _ string) (string, error) {
		return overrideResp, nil
	})
	w := doJSON(t, r, http.MethodPost, "/ai/topics", map[string]any{
		"seed":     "test",
		"platform": "抖音",
		"count":    2,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Topics []struct {
			Title               string `json:"title"`
			Angle               string `json:"angle"`
			ExpectedPerformance string `json:"expected_performance"`
			Hook                string `json:"hook"`
		} `json:"topics"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, w.Body.String())
	}
	if len(resp.Topics) != 2 {
		t.Fatalf("len(topics) = %d, want 2", len(resp.Topics))
	}
	if resp.Topics[0].Title != "标题A" {
		t.Errorf("topics[0].title = %q, want %q", resp.Topics[0].Title, "标题A")
	}
	if resp.Topics[1].Hook != "钩子B" {
		t.Errorf("topics[1].hook = %q, want %q", resp.Topics[1].Hook, "钩子B")
	}
}

// TestGenerateTopicsStripsMarkdownFence: Claude sometimes wraps JSON
// in ```json ... ``` fences. The handler must strip the wrapper.
func TestGenerateTopicsStripsMarkdownFence(t *testing.T) {
	wrapped := "```json\n" +
		`[{"title":"X","angle":"Y","expected_performance":"Z","hook":"H"}]` +
		"\n```"
	r := setupAITestRouter(t, func(_ context.Context, _ string) (string, error) {
		return wrapped, nil
	})
	w := doJSON(t, r, http.MethodPost, "/ai/topics", map[string]any{
		"seed":     "test",
		"platform": "抖音",
		"count":    1,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"title":"X"`) {
		t.Errorf("body missing unwrapped title: %s", w.Body.String())
	}
}

// TestGenerateTopicsNoAPIKey: when the agent returns a "no API key"
// error, the handler must surface 503 with a clear message so the
// frontend can show a useful hint instead of a generic 500.
func TestGenerateTopicsNoAPIKey(t *testing.T) {
	r := setupAITestRouter(t, func(_ context.Context, _ string) (string, error) {
		return "", errAIUnavailable
	})
	w := doJSON(t, r, http.MethodPost, "/ai/topics", map[string]any{
		"seed":     "test",
		"platform": "抖音",
		"count":    2,
	})
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503, body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "API key") && !strings.Contains(w.Body.String(), "ANTHROPIC_API_KEY") {
		t.Errorf("body should mention API key, got: %s", w.Body.String())
	}
}

// TestGenerateTopicsInvalidBody: missing required fields must yield
// 400, not 500.
func TestGenerateTopicsInvalidBody(t *testing.T) {
	r := setupAITestRouter(t, func(_ context.Context, _ string) (string, error) {
		t.Error("claude should not be called for invalid body")
		return "", nil
	})
	w := doJSON(t, r, http.MethodPost, "/ai/topics", map[string]any{
		"seed": "",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", w.Code, w.Body.String())
	}
}

// errAIUnavailable simulates the "no API key" branch.
var errAIUnavailable = &aiTestError{msg: "claude complete: no Anthropic API key configured (set ANTHROPIC_API_KEY)"}

type aiTestError struct{ msg string }

func (e *aiTestError) Error() string { return e.msg }

// TestHumanizeScriptSuccess: override returns a rewritten script and
// the handler must echo it back unchanged in the humanized field.
func TestHumanizeScriptSuccess(t *testing.T) {
	r := setupAITestRouter(t, func(_ context.Context, _ string) (string, error) {
		return "这是改写后的版本...", nil
	})
	w := doJSON(t, r, http.MethodPost, "/ai/humanize", map[string]any{
		"script": "原始 AI 痕迹明显的脚本内容",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Humanized string `json:"humanized"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, w.Body.String())
	}
	if resp.Humanized != "这是改写后的版本..." {
		t.Errorf("humanized = %q, want %q", resp.Humanized, "这是改写后的版本...")
	}
}

// TestHumanizeScriptNoAPIKey: when the agent returns a "no API key"
// error, the handler must surface 503 with a clear message.
func TestHumanizeScriptNoAPIKey(t *testing.T) {
	r := setupAITestRouter(t, func(_ context.Context, _ string) (string, error) {
		return "", errAIUnavailable
	})
	w := doJSON(t, r, http.MethodPost, "/ai/humanize", map[string]any{
		"script": "some script",
	})
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503, body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "API key") && !strings.Contains(w.Body.String(), "ANTHROPIC_API_KEY") {
		t.Errorf("body should mention API key, got: %s", w.Body.String())
	}
}

// TestHumanizeScriptInvalidBody: missing or empty script must yield
// 400, not 500.
func TestHumanizeScriptInvalidBody(t *testing.T) {
	r := setupAITestRouter(t, func(_ context.Context, _ string) (string, error) {
		t.Error("claude should not be called for invalid body")
		return "", nil
	})
	w := doJSON(t, r, http.MethodPost, "/ai/humanize", map[string]any{
		"script": "   ",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", w.Code, w.Body.String())
	}
}

// setupPostmortemRouter builds a router with an in-memory sqlite DB
// seeded with one Topic → Script → ContentItem chain. Tests that
// need to exercise the 404 path skip the seed and post the id of a
// row that does not exist.
func setupPostmortemRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Topic{}, &models.Script{}, &models.ContentItem{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	r := gin.New()
	return r, db
}

// newPostmortemRouterWithOverride wires the router with the given
// override function so each test can supply its own Claude response.
func newPostmortemRouterWithOverride(db *gorm.DB, fn agents.CompleteFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	claude := agents.NewClaudeWithOverride(fn)
	r := gin.New()
	NewAIHandler(claude, db).RegisterRoutes(r)
	return r
}

func seedPostmortemFixture(t *testing.T, db *gorm.DB) (topicID, scriptID, itemID uint) {
	t.Helper()
	topic := models.Topic{
		Title:    "测试选题",
		Angle:    "御宅哲学角度",
		Platform: "抖音",
		Status:   "已发布",
	}
	if err := db.Create(&topic).Error; err != nil {
		t.Fatalf("create topic: %v", err)
	}
	script := models.Script{
		TopicID:  topic.ID,
		Title:    "测试脚本标题",
		Content:  "测试脚本正文",
		Platform: "抖音",
	}
	if err := db.Create(&script).Error; err != nil {
		t.Fatalf("create script: %v", err)
	}
	ci := models.ContentItem{
		ScriptID:           script.ID,
		Platform:           "抖音",
		PlatformURL:        "https://example.com/v/1",
		PerformanceMetrics: "播放 12k 点赞 800",
	}
	if err := db.Create(&ci).Error; err != nil {
		t.Fatalf("create content item: %v", err)
	}
	return topic.ID, script.ID, ci.ID
}

// TestPostmortemSuccess: Claude returns a clean JSON object and the
// handler must echo it as `report` (raw) and `structured` (parsed).
func TestPostmortemSuccess(t *testing.T) {
	r, db := setupPostmortemRouter(t)
	_, _, itemID := seedPostmortemFixture(t, db)
	overrideResp := `{
		"success_factors":["强钩子","情绪共鸣"],
		"reusable_patterns":["开头提问","结尾反转"],
		"insights":["播放完成率 65% 高于均值"],
		"suggestions":["下次可以提前 1 秒抛钩子"]
	}`
	r2 := newPostmortemRouterWithOverride(db, func(_ context.Context, _ string) (string, error) {
		return overrideResp, nil
	})
	_ = r

	w := doJSON(t, r2, http.MethodPost, "/ai/postmortem", map[string]any{
		"content_item_id": itemID,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Report     string `json:"report"`
		Structured struct {
			SuccessFactors   []string `json:"success_factors"`
			ReusablePatterns []string `json:"reusable_patterns"`
			Insights         []string `json:"insights"`
			Suggestions      []string `json:"suggestions"`
		} `json:"structured"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, w.Body.String())
	}
	if resp.Report != overrideResp {
		t.Errorf("report = %q, want raw response", resp.Report)
	}
	if len(resp.Structured.SuccessFactors) != 2 || resp.Structured.SuccessFactors[0] != "强钩子" {
		t.Errorf("success_factors = %v, want [强钩子 情绪共鸣]", resp.Structured.SuccessFactors)
	}
	if len(resp.Structured.ReusablePatterns) != 2 || resp.Structured.ReusablePatterns[1] != "结尾反转" {
		t.Errorf("reusable_patterns = %v, want [... 结尾反转]", resp.Structured.ReusablePatterns)
	}
	if len(resp.Structured.Insights) != 1 {
		t.Errorf("insights = %v, want 1 entry", resp.Structured.Insights)
	}
	if len(resp.Structured.Suggestions) != 1 {
		t.Errorf("suggestions = %v, want 1 entry", resp.Structured.Suggestions)
	}
}

// TestPostmortemStripsMarkdownFence: Claude wraps the JSON in
// ```json ... ``` fences; the handler must strip the wrapper and
// still return parsed structured data.
func TestPostmortemStripsMarkdownFence(t *testing.T) {
	_, db := setupPostmortemRouter(t)
	_, _, itemID := seedPostmortemFixture(t, db)
	wrapped := "```json\n" +
		`{"success_factors":["A"],"reusable_patterns":["B"],"insights":["C"],"suggestions":["D"]}` +
		"\n```"
	r := newPostmortemRouterWithOverride(db, func(_ context.Context, _ string) (string, error) {
		return wrapped, nil
	})

	w := doJSON(t, r, http.MethodPost, "/ai/postmortem", map[string]any{
		"content_item_id": itemID,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"success_factors":["A"]`) {
		t.Errorf("body missing unwrapped success_factors: %s", w.Body.String())
	}
}

// TestPostmortemNotFound: posting a content_item_id that does not
// exist must yield 404, not 500.
func TestPostmortemNotFound(t *testing.T) {
	_, db := setupPostmortemRouter(t)
	r := newPostmortemRouterWithOverride(db, func(_ context.Context, _ string) (string, error) {
		t.Error("claude should not be called when content item is missing")
		return "", nil
	})
	w := doJSON(t, r, http.MethodPost, "/ai/postmortem", map[string]any{
		"content_item_id": 999,
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "not found") {
		t.Errorf("body should mention not found, got: %s", w.Body.String())
	}
}

// TestPostmortemNoAPIKey: when the agent returns a "no API key"
// error, the handler must surface 503 with a clear message.
func TestPostmortemNoAPIKey(t *testing.T) {
	_, db := setupPostmortemRouter(t)
	_, _, itemID := seedPostmortemFixture(t, db)
	r := newPostmortemRouterWithOverride(db, func(_ context.Context, _ string) (string, error) {
		return "", errAIUnavailable
	})

	w := doJSON(t, r, http.MethodPost, "/ai/postmortem", map[string]any{
		"content_item_id": itemID,
	})
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503, body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "API key") && !strings.Contains(w.Body.String(), "ANTHROPIC_API_KEY") {
		t.Errorf("body should mention API key, got: %s", w.Body.String())
	}
}

// TestPostmortemInvalidBody: missing content_item_id must yield 400.
func TestPostmortemInvalidBody(t *testing.T) {
	_, db := setupPostmortemRouter(t)
	r := newPostmortemRouterWithOverride(db, func(_ context.Context, _ string) (string, error) {
		t.Error("claude should not be called for invalid body")
		return "", nil
	})
	w := doJSON(t, r, http.MethodPost, "/ai/postmortem", map[string]any{})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", w.Code, w.Body.String())
	}
}
