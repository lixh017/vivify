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

	"github.com/opc/api/internal/agents"
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
