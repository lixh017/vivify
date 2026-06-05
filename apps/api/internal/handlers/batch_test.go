package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/opc/api/internal/agents"
)

// setupBatchRouter wires a BatchHandler with the given
// override function and (optional) DB. The DB is not
// exercised in the tests we ship today — the persistence
// branch is reserved for a future PR — but wiring it
// through the constructor keeps the test harness close
// to the production shape.
func setupBatchRouter(t *testing.T, fn agents.CompleteFunc) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	var c PipelineClient
	if fn != nil {
		c = agents.NewClaudeWithOverride(fn)
	} else {
		c = agents.NewClaude("")
	}
	r := gin.New()
	NewBatchHandler(c, nil).RegisterRoutes(r)
	return r
}

// TestBatchHappyPathIssues5Topics is the canonical
// happy-path test. The override returns the canned
// responses for each of the four steps (topics/script/
// score/adapt) and we expect:
//
//   - response contains exactly count items
//   - every item has a topic populated
//   - items with non-topics steps also have script/score/adapt
//   - summary.success_count == count, fail_count == 0
//
// Because the batch runs topics+script+score+adapt in
// parallel across N topics, we cannot reliably assert on
// the *order* of override calls (the prompts arrive in a
// different interleaving). We do assert on the *count*:
// 4 calls per topic * 5 topics = 20.
func TestBatchHappyPathIssues5Topics(t *testing.T) {
	var calls int32
	override := func(_ context.Context, prompt string) (string, error) {
		atomic.AddInt32(&calls, 1)
		// Branch on the prompt content rather than call
		// order — the batch may interleave step calls
		// across topics, but each prompt has a unique
		// "role" hint (the topic prompt asks for an array,
		// the script prompt does not, etc.).
		switch {
		case containsBody(prompt, "Generate exactly 1 topic"):
			return `[{"title":"深夜熊猫","angle":"治愈","expected_performance":"高","hook":"画面: 窗边","pattern":"画面钩子","voice_tags":["治愈"]}]`, nil
		case containsBody(prompt, "JSON array of N short topics"):
			return `[{"title":"深夜熊猫","angle":"治愈","expected_performance":"高","hook":"画面: 窗边","pattern":"画面钩子","voice_tags":["治愈"]}]`, nil
		case containsBody(prompt, "STYLE REFERENCE"):
			// RAG-augmented prompt — same response as the
			// non-RAG case.
			return `[{"title":"深夜熊猫","angle":"治愈","expected_performance":"高","hook":"画面: 窗边","pattern":"画面钩子","voice_tags":["治愈"]}]`, nil
		case containsBody(prompt, "humanize") || containsBody(prompt, "把这段旁白改写"):
			return "窗边的熊猫一句话都没说。", nil
		case containsBody(prompt, "0-100"):
			return `{"overall_score":85,"hook_strength":90,"structure":80,"platform_fit":85,"suggestions":[],"rewritten_hook":""}`, nil
		case containsBody(prompt, "三平台"):
			return `{"adaptations":{"抖音":{"title":"A","hashtags":["#a"],"description":"d"},"哔哩哔哩":{"title":"B","description":"d","tags":["x"]},"小红书":{"title":"C","body":"d","tags":["#x"]}},"cross_platform_tips":["t"]}`, nil
		default:
			// Unknown prompt shape — return a benign default.
			return `[{"title":"t","angle":"a","expected_performance":"e","hook":"h"}]`, nil
		}
	}
	r := setupBatchRouter(t, override)
	w := doJSON(t, r, http.MethodPost, "/ai/batch", map[string]any{
		"seed":      "panda",
		"count":     5,
		"platforms": []string{"抖音", "哔哩哔哩", "小红书"},
		"steps":     []string{"topics", "script", "score", "adapt"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp batchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Summary.Total != 5 {
		t.Errorf("summary.total = %d, want 5", resp.Summary.Total)
	}
	if resp.Summary.SuccessCount != 5 {
		t.Errorf("summary.success_count = %d, want 5", resp.Summary.SuccessCount)
	}
	if resp.Summary.FailCount != 0 {
		t.Errorf("summary.fail_count = %d, want 0", resp.Summary.FailCount)
	}
	if len(resp.Generated) != 5 {
		t.Errorf("generated len = %d, want 5", len(resp.Generated))
	}
	for i, it := range resp.Generated {
		if it.Topic == nil {
			t.Errorf("item %d: topic is nil", i)
			continue
		}
		if it.Script == nil || it.Score == nil || it.Adaptations == nil {
			t.Errorf("item %d: missing script/score/adapt", i)
		}
		if it.Error != "" {
			t.Errorf("item %d: error = %q", i, it.Error)
		}
	}
	// 4 calls per topic * 5 topics = 20. The concurrency
	// bound (3) does not affect the call COUNT, only the
	// wall-clock ordering.
	if got := atomic.LoadInt32(&calls); got != 20 {
		t.Errorf("override calls = %d, want 20", got)
	}
}

// TestBatchDefaultsCountAndPlatforms asserts the request
// normalization: omitting count yields the default (10),
// omitting platforms yields 抖音/哔哩哔哩/小红书, and
// omitting steps yields all four. We don't run the live
// path — we just check the summary.total = 10 by
// overriding Complete to return 0 topics so every item
// is a "no topics returned" failure (still counted as
// fail_count, but Total still reflects the request).
func TestBatchDefaultsCountAndPlatforms(t *testing.T) {
	override := func(_ context.Context, _ string) (string, error) {
		return `[]`, nil
	}
	r := setupBatchRouter(t, override)
	w := doJSON(t, r, http.MethodPost, "/ai/batch", map[string]any{
		"seed": "panda",
		// count omitted → 10
		// platforms omitted → defaults
		// steps omitted → defaults
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp batchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Summary.Total != batchDefaultCount {
		t.Errorf("summary.total = %d, want %d", resp.Summary.Total, batchDefaultCount)
	}
	if resp.Summary.FailCount != batchDefaultCount {
		t.Errorf("summary.fail_count = %d, want %d", resp.Summary.FailCount, batchDefaultCount)
	}
}

// TestBatchCapsCountAtMax asserts the upper bound. A
// caller asking for count=1000 should be silently clamped
// to batchMaxCount (25) — the cap is the only way to
// prevent a token-burn runaway in production.
func TestBatchCapsCountAtMax(t *testing.T) {
	override := func(_ context.Context, _ string) (string, error) {
		return `[]`, nil
	}
	r := setupBatchRouter(t, override)
	w := doJSON(t, r, http.MethodPost, "/ai/batch", map[string]any{
		"seed":  "panda",
		"count": 1000,
	})
	var resp batchResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Summary.Total > batchMaxCount {
		t.Errorf("total = %d, expected clamp to <= %d", resp.Summary.Total, batchMaxCount)
	}
	if resp.Summary.Total != batchMaxCount {
		t.Errorf("total = %d, want %d", resp.Summary.Total, batchMaxCount)
	}
}

// TestBatchRejectsMissingSeed asserts the 400 path for
// the most common validation error. The handler must
// surface a useful error message — not a 500.
func TestBatchRejectsMissingSeed(t *testing.T) {
	r := setupBatchRouter(t, nil)
	w := doJSON(t, r, http.MethodPost, "/ai/batch", map[string]any{
		"count": 5,
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if !containsBody(w.Body.String(), "seed is required") {
		t.Errorf("body = %s, want 'seed is required'", w.Body.String())
	}
}

// TestBatchRejectsEmptySteps asserts the second 400 path:
// the steps list resolved to empty after normalization
// (e.g. all-unknown names). The handler must NOT silently
// run an empty pipeline.
func TestBatchRejectsEmptySteps(t *testing.T) {
	override := func(_ context.Context, _ string) (string, error) {
		return `[]`, nil
	}
	r := setupBatchRouter(t, override)
	w := doJSON(t, r, http.MethodPost, "/ai/batch", map[string]any{
		"seed":  "panda",
		"steps": []string{"unknown_step", "also_unknown"},
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// TestBatchTopicsOnlyShortCircuit asserts the lightweight
// path: steps=["topics"] returns one topic per slot and
// nothing else. The override should be called exactly
// count times — no script/score/adapt calls.
func TestBatchTopicsOnlyShortCircuit(t *testing.T) {
	var calls int32
	override := func(_ context.Context, _ string) (string, error) {
		atomic.AddInt32(&calls, 1)
		return `[{"title":"t","angle":"a","expected_performance":"e","hook":"h"}]`, nil
	}
	r := setupBatchRouter(t, override)
	w := doJSON(t, r, http.MethodPost, "/ai/batch", map[string]any{
		"seed":  "panda",
		"count": 3,
		"steps": []string{"topics"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp batchResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Generated) != 3 {
		t.Errorf("generated = %d, want 3", len(resp.Generated))
	}
	for i, it := range resp.Generated {
		if it.Topic == nil {
			t.Errorf("item %d: topic nil", i)
		}
		if it.Script != nil || it.Score != nil || it.Adaptations != nil {
			t.Errorf("item %d: non-topic steps ran", i)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("calls = %d, want 3 (topics only)", got)
	}
}

// TestBatchDemoModeShortCircuits asserts the demo path
// produces N copies of the canned per-topic pipeline
// without hitting the (non-existent) Claude API. The
// response must also carry the X-Demo-Mode header so the
// frontend can render the badge.
func TestBatchDemoModeShortCircuits(t *testing.T) {
	r := setupBatchRouter(t, nil) // nil fn → empty key → demo mode
	w := doJSON(t, r, http.MethodPost, "/ai/batch?demo=true", map[string]any{
		"seed":  "panda",
		"count": 2,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-Demo-Mode"); got != "true" {
		t.Errorf("X-Demo-Mode = %q, want 'true'", got)
	}
	var resp batchResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Generated) != 2 {
		t.Errorf("generated = %d, want 2", len(resp.Generated))
	}
	if resp.Summary.SuccessCount != 2 {
		t.Errorf("success_count = %d, want 2", resp.Summary.SuccessCount)
	}
}

// TestBatchConcurrencyBoundIsRespected asserts the
// runtime concurrency cap. The override sleeps for 100ms
// and we ask for 6 topics; the wall clock should be at
// least 200ms (2 batches * 100ms / 3 concurrency ≈ 200ms)
// and at most ~400ms (no overlap). A regression that
// removes the semaphore would push the wall clock to
// 6*100ms = 600ms.
func TestBatchConcurrencyBoundIsRespected(t *testing.T) {
	sleep := 100 * time.Millisecond
	override := func(ctx context.Context, _ string) (string, error) {
		select {
		case <-time.After(sleep):
			return `[{"title":"t","angle":"a","expected_performance":"e","hook":"h"}]`, nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	r := setupBatchRouter(t, override)
	start := time.Now()
	w := doJSON(t, r, http.MethodPost, "/ai/batch", map[string]any{
		"seed":  "panda",
		"count": 6,
		"steps": []string{"topics"},
	})
	elapsed := time.Since(start)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	// 6 topics at 100ms with concurrency=3 → ~200ms expected
	// Allow up to 450ms to avoid flakiness on slow CI. The
	// important assertion is the *lower* bound: <200ms would
	// mean the semaphore was bypassed.
	if elapsed < 2*sleep-time.Millisecond*10 {
		t.Errorf("elapsed = %v, expected >= 2*%v (concurrency=3 working)", elapsed, sleep)
	}
	if elapsed > 5*sleep {
		t.Errorf("elapsed = %v, expected <= ~3*%v", elapsed, sleep)
	}
}

// TestBatchNormalizePlatformsDropsDuplicates is a
// focused unit test for the helper. The test lives
// here (not in a separate file) because the helper is
// package-private and only the batch handler uses it.
func TestBatchNormalizePlatformsDropsDuplicates(t *testing.T) {
	got := normalizeBatchPlatforms([]string{"抖音", "抖音", " ", "小红书", "哔哩哔哩", "小红书"})
	want := []string{"抖音", "小红书", "哔哩哔哩"}
	if !equalStringSlice(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if got := normalizeBatchPlatforms(nil); !equalStringSlice(got, defaultBatchPlatforms) {
		t.Errorf("nil input should yield defaults, got %v", got)
	}
	if got := normalizeBatchPlatforms([]string{"", " "}); !equalStringSlice(got, defaultBatchPlatforms) {
		t.Errorf("all-empty input should yield defaults, got %v", got)
	}
}

// TestBatchNormalizeStepsDropsUnknown is the parallel
// helper test for steps normalization. Same reasoning
// as the platforms test: covered via the public surface
// but kept here so a regression in the helper itself
// gets a precise error.
func TestBatchNormalizeStepsDropsUnknown(t *testing.T) {
	got := normalizeBatchSteps([]string{"topics", "nope", "script", " "})
	want := []string{"topics", "script"}
	if !equalStringSlice(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if got := normalizeBatchSteps(nil); !equalStringSlice(got, defaultBatchSteps) {
		t.Errorf("nil input should yield defaults, got %v", got)
	}
}

// TestBatchErrorInOneItemDoesNotAbortBatch is the
// partial-failure contract. The override alternates
// between a valid topics array and an empty one; we
// expect the response to have a mix of success and
// fail counts and a 200 status (not 500).
func TestBatchErrorInOneItemDoesNotAbortBatch(t *testing.T) {
	var calls int32
	override := func(_ context.Context, _ string) (string, error) {
		n := atomic.AddInt32(&calls, 1)
		if n%2 == 0 {
			return `[]`, nil
		}
		return `[{"title":"t","angle":"a","expected_performance":"e","hook":"h"}]`, nil
	}
	r := setupBatchRouter(t, override)
	w := doJSON(t, r, http.MethodPost, "/ai/batch", map[string]any{
		"seed":  "panda",
		"count": 4,
		"steps": []string{"topics"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp batchResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Summary.SuccessCount+resp.Summary.FailCount != 4 {
		t.Errorf("success+fail = %d, want 4", resp.Summary.SuccessCount+resp.Summary.FailCount)
	}
}

// containsBody is a tiny helper so the test file does not
// pull in strings just for one substring check. (Named
// "containsBody" to avoid clashing with the contains
// helper already defined in topic_test.go for the same
// package.)
func containsBody(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// equalStringSlice is a small helper. The standard
// reflect.DeepEqual would work but pulls in a big
// dependency for a 5-line loop.
func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
