package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/opc/api/internal/agents"
)

// setupDeconstructTestRouter wires the deconstruct handler with a
// MiniMax text-override so each test can pin the response.
func setupDeconstructTestRouter(t *testing.T, fn textOverrideFn) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	m := agents.NewMiniMaxWithTextOverride(toTextResult(fn))
	r := gin.New()
	NewDeconstructHandler(m).RegisterRoutes(r)
	return r
}

// setupDeconstructDemoRouter wires the handler with a MiniMax
// client WITHOUT an API key so it defaults to demo mode.
func setupDeconstructDemoRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	m := agents.NewMiniMaxForDemo()
	r := gin.New()
	NewDeconstructHandler(m).RegisterRoutes(r)
	return r
}

// ---------------------------------------------------------------------------
// /ai/deconstruct
// ---------------------------------------------------------------------------

// TestDeconstructDemoNoKey verifies the no-API-key + no-flag path
// returns the canned panda-IP analysis with X-Demo-Mode set.
func TestDeconstructDemoNoKey(t *testing.T) {
	r := setupDeconstructDemoRouter(t)
	w := doJSON(t, r, http.MethodPost, "/ai/deconstruct", map[string]any{
		"transcript": "你有没有想过——逍遥到底是什么?",
		"metadata": map[string]any{
			"platform":     "抖音",
			"duration_sec": 75,
			"views":        1000000,
			"likes":        45000,
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-Demo-Mode"); got != "true" {
		t.Errorf("X-Demo-Mode = %q, want %q", got, "true")
	}
	var resp deconstructResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, w.Body.String())
	}
	if resp.Hook.Text == "" {
		t.Error("hook.text empty — demo should return the panda-IP hook text")
	}
	if resp.OverallScore < 1 || resp.OverallScore > 100 {
		t.Errorf("overall_score out of range: %d", resp.OverallScore)
	}
	if len(resp.ReusablePatterns) == 0 {
		t.Error("reusable_patterns empty — demo should show OpusClip-style patterns")
	}
	// All three target platforms must have a note.
	if resp.PlatformFitNotes.Douyin == "" || resp.PlatformFitNotes.Bilibili == "" || resp.PlatformFitNotes.Xiaohongshu == "" {
		t.Errorf("platform_fit_notes incomplete: %+v", resp.PlatformFitNotes)
	}
}

// TestDeconstructForcedDemoWithKey verifies ?demo=true overrides
// even when an API key is configured.
func TestDeconstructForcedDemoWithKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := agents.NewMiniMax("fake-key-for-tests")
	r := gin.New()
	NewDeconstructHandler(m).RegisterRoutes(r)

	w := doJSON(t, r, http.MethodPost, "/ai/deconstruct?demo=true", map[string]any{
		"transcript": "任意文本",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-Demo-Mode"); got != "true" {
		t.Errorf("X-Demo-Mode = %q, want %q", got, "true")
	}
}

// TestDeconstructRealPath verifies the override path runs and the
// response is parsed into the wire shape.
func TestDeconstructRealPath(t *testing.T) {
	overrideResp := `{
		"hook": {"type":"question","text":"今天的雨,为什么这么久?","analysis":"用提问+具体场景","strength":85},
		"structure": {
			"pattern": "hook-problem-solution",
			"beats": [{"time_pct":0,"role":"setup","description":"开场"}],
			"pacing": "slow",
			"density": 40
		},
		"cta": {"present": true, "type":"subscribe", "placement":"late"},
		"emotional_arc": [{"time_pct":0,"emotion":"curiosity","intensity":70}],
		"reusable_patterns": ["问题开场", "慢节奏留白"],
		"platform_fit_notes": {"抖音":"60s 内","哔哩哔哩":"3-5 分钟","小红书":"图文+短视频"},
		"overall_score": 80
	}`
	r := setupDeconstructTestRouter(t, func(_ context.Context, _ string) (string, error) {
		return overrideResp, nil
	})
	w := doJSON(t, r, http.MethodPost, "/ai/deconstruct", map[string]any{
		"transcript": "今天的雨,下得有点久。",
		"metadata":   map[string]any{"platform": "抖音"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-Demo-Mode"); got == "true" {
		t.Errorf("X-Demo-Mode = %q, want unset (real path)", got)
	}
	var resp deconstructResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, w.Body.String())
	}
	if resp.Hook.Strength != 85 {
		t.Errorf("hook.strength = %d, want 85", resp.Hook.Strength)
	}
	if resp.OverallScore != 80 {
		t.Errorf("overall_score = %d, want 80", resp.OverallScore)
	}
}

// TestDeconstructStripsMarkdownFence: Claude wraps the JSON in
// ```json ... ``` fences; the handler must strip the wrapper.
func TestDeconstructStripsMarkdownFence(t *testing.T) {
	wrapped := "```json\n" + `{
		"hook": {"type":"question","text":"x","analysis":"y","strength":50},
		"structure": {"pattern":"AIDA","beats":[],"pacing":"medium","density":50},
		"cta": {"present":false,"type":"","placement":"late"},
		"emotional_arc": [],
		"reusable_patterns": [],
		"platform_fit_notes": {"抖音":"","哔哩哔哩":"","小红书":""},
		"overall_score": 60
	}` + "\n```"
	r := setupDeconstructTestRouter(t, func(_ context.Context, _ string) (string, error) {
		return wrapped, nil
	})
	w := doJSON(t, r, http.MethodPost, "/ai/deconstruct", map[string]any{
		"transcript": "测试 fence strip 的文本。",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	var resp deconstructResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, w.Body.String())
	}
	if resp.OverallScore != 60 {
		t.Errorf("overall_score = %d, want 60", resp.OverallScore)
	}
}

// TestDeconstructClampsOutOfRangeScores verifies the parser clamps
// hallucinated >100 scores so the frontend never has to defend
// against them.
func TestDeconstructClampsOutOfRangeScores(t *testing.T) {
	overrideResp := `{
		"hook": {"type":"question","text":"x","analysis":"y","strength":150},
		"structure": {"pattern":"AIDA","beats":[],"pacing":"medium","density":-10},
		"cta": {"present":false,"type":"","placement":"late"},
		"emotional_arc": [],
		"reusable_patterns": [],
		"platform_fit_notes": {"抖音":"","哔哩哔哩":"","小红书":""},
		"overall_score": 200
	}`
	r := setupDeconstructTestRouter(t, func(_ context.Context, _ string) (string, error) {
		return overrideResp, nil
	})
	w := doJSON(t, r, http.MethodPost, "/ai/deconstruct", map[string]any{
		"transcript": "text",
	})
	var resp deconstructResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Hook.Strength != 100 {
		t.Errorf("hook.strength = %d, want clamped to 100", resp.Hook.Strength)
	}
	if resp.Structure.Density != 0 {
		t.Errorf("structure.density = %d, want clamped to 0", resp.Structure.Density)
	}
	if resp.OverallScore != 100 {
		t.Errorf("overall_score = %d, want clamped to 100", resp.OverallScore)
	}
}

// TestDeconstructInvalidBody: missing transcript must 400. Table-
// driven so every variant is covered.
func TestDeconstructInvalidBody(t *testing.T) {
	r := setupDeconstructTestRouter(t, func(_ context.Context, _ string) (string, error) {
		t.Error("claude should not be called for invalid body")
		return "", nil
	})
	cases := []struct {
		name string
		body map[string]any
	}{
		{"missing transcript", map[string]any{}},
		{"empty transcript", map[string]any{"transcript": ""}},
		{"whitespace transcript", map[string]any{"transcript": "   "}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doJSON(t, r, http.MethodPost, "/ai/deconstruct", tc.body)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400, body = %s", w.Code, w.Body.String())
			}
		})
	}
}

// TestDeconstructRejectsHugeTranscript verifies the cap kicks in
// for absurdly large payloads.
func TestDeconstructRejectsHugeTranscript(t *testing.T) {
	r := setupDeconstructTestRouter(t, func(_ context.Context, _ string) (string, error) {
		t.Error("claude should not be called for oversize transcript")
		return "", nil
	})
	w := doJSON(t, r, http.MethodPost, "/ai/deconstruct", map[string]any{
		"transcript": strings.Repeat("x", maxTranscriptBytes+1),
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400, body = %s", w.Code, w.Body.String())
	}
}

// TestDeconstructHandlesUpstreamError: a MiniMax Text call that
// returns an upstream error should surface as 503 (consistent with
// the rest of the AI surface).
func TestDeconstructHandlesUpstreamError(t *testing.T) {
	r := setupDeconstructTestRouter(t, func(_ context.Context, _ string) (string, error) {
		return "", fmt.Errorf("minimax: simulated upstream error")
	})
	w := doJSON(t, r, http.MethodPost, "/ai/deconstruct", map[string]any{
		"transcript": "text",
	})
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503, body = %s", w.Code, w.Body.String())
	}
}

// TestDeconstructHandlesParseError: when Claude returns garbage,
// the handler must 502 (not silently OK with zero values). This is
// distinct from the score handler which falls back to rule-based —
// there is no rule-based deconstruct, so 502 is the correct signal.
func TestDeconstructHandlesParseError(t *testing.T) {
	r := setupDeconstructTestRouter(t, func(_ context.Context, _ string) (string, error) {
		return "this is not json", nil
	})
	w := doJSON(t, r, http.MethodPost, "/ai/deconstruct", map[string]any{
		"transcript": "text",
	})
	if w.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502, body = %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// /ai/viral-formula
// ---------------------------------------------------------------------------

// TestViralFormulaDemoNoKey verifies the no-API-key + no-flag path
// returns the canned formula with X-Demo-Mode set.
func TestViralFormulaDemoNoKey(t *testing.T) {
	r := setupDeconstructDemoRouter(t)
	w := doJSON(t, r, http.MethodPost, "/ai/viral-formula", map[string]any{
		"transcript": "你有没有想过——逍遥到底是什么?",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-Demo-Mode"); got != "true" {
		t.Errorf("X-Demo-Mode = %q, want %q", got, "true")
	}
	var resp viralFormulaResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, w.Body.String())
	}
	if resp.FormulaName == "" {
		t.Error("formula_name empty — demo should return the panda-IP formula")
	}
	if len(resp.Variables) == 0 {
		t.Error("variables empty")
	}
	if len(resp.Steps) == 0 {
		t.Error("steps empty")
	}
	if resp.ExampleApplication == "" {
		t.Error("example_application empty")
	}
	if n := len(resp.Variations); n < 3 || n > 5 {
		t.Errorf("variations count = %d, spec requires 3-5", n)
	}
}

// TestViralFormulaRealPath verifies the override path runs and the
// response is parsed correctly.
func TestViralFormulaRealPath(t *testing.T) {
	overrideResp := `{
		"formula_name": "测试公式",
		"variables": [{"name":"theme","example":"庄子","weight":"high"}],
		"steps": ["选主题","写钩子","展开"],
		"example_application": "熊猫读《论语》",
		"variations": ["哲学","文学","情感"]
	}`
	r := setupDeconstructTestRouter(t, func(_ context.Context, _ string) (string, error) {
		return overrideResp, nil
	})
	w := doJSON(t, r, http.MethodPost, "/ai/viral-formula", map[string]any{
		"transcript": "测试文本",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	var resp viralFormulaResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, w.Body.String())
	}
	if resp.FormulaName != "测试公式" {
		t.Errorf("formula_name = %q, want %q", resp.FormulaName, "测试公式")
	}
	if len(resp.Variables) != 1 || resp.Variables[0].Name != "theme" {
		t.Errorf("variables = %+v, want one entry name=theme", resp.Variables)
	}
}

// TestViralFormulaInvalidBody covers the validation surface.
func TestViralFormulaInvalidBody(t *testing.T) {
	r := setupDeconstructTestRouter(t, func(_ context.Context, _ string) (string, error) {
		t.Error("claude should not be called for invalid body")
		return "", nil
	})
	cases := []struct {
		name string
		body map[string]any
	}{
		{"missing transcript", map[string]any{}},
		{"empty transcript", map[string]any{"transcript": "  "}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doJSON(t, r, http.MethodPost, "/ai/viral-formula", tc.body)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400, body = %s", w.Code, w.Body.String())
			}
		})
	}
}

// TestViralFormulaRejectsHugeTranscript guards the same cap as
// /ai/deconstruct.
func TestViralFormulaRejectsHugeTranscript(t *testing.T) {
	r := setupDeconstructTestRouter(t, func(_ context.Context, _ string) (string, error) {
		t.Error("claude should not be called for oversize transcript")
		return "", nil
	})
	w := doJSON(t, r, http.MethodPost, "/ai/viral-formula", map[string]any{
		"transcript": strings.Repeat("x", maxTranscriptBytes+1),
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400, body = %s", w.Code, w.Body.String())
	}
}

// TestViralFormulaHandlesUpstreamError: upstream errors through
// the live path must surface as 503.
func TestViralFormulaHandlesUpstreamError(t *testing.T) {
	r := setupDeconstructTestRouter(t, func(_ context.Context, _ string) (string, error) {
		return "", fmt.Errorf("minimax: simulated upstream error")
	})
	w := doJSON(t, r, http.MethodPost, "/ai/viral-formula", map[string]any{
		"transcript": "text",
	})
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503, body = %s", w.Code, w.Body.String())
	}
}

// TestStripJSONFenceHelper pins the helper used by both parsers
// so a refactor that moves the fence-strip logic in one direction
// doesn't silently regress the other.
func TestStripJSONFenceHelper(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"no fence", `{"x":1}`, `{"x":1}`},
		{"json fence", "```json\n{\"x\":1}\n```", `{"x":1}`},
		{"plain fence", "```\n{\"x\":1}\n```", `{"x":1}`},
		{"leading whitespace", "   {\"x\":1}   ", `{"x":1}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripJSONFence(tc.in); got != tc.want {
				t.Errorf("stripJSONFence(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
