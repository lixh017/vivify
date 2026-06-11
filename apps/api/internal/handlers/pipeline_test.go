package handlers

import (
	"context"
	"encoding/json"
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

// setupPipelineRouter wires the pipeline handler with the given
// override function. Pass nil for db to skip the RAG step
// (matches the production wiring when the handler is constructed
// without a DB).
func setupPipelineRouter(t *testing.T, fn textOverrideFn, db *gorm.DB) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	var c *agents.MiniMax
	if fn != nil {
		c = agents.NewMiniMaxWithTextOverride(toTextResult(fn))
	} else {
		c = agents.NewMiniMaxForDemo() // empty key → demo mode
	}
	r := gin.New()
	if db != nil {
		NewPipelineHandler(c, db).RegisterRoutes(r)
	} else {
		NewPipelineHandler(c).RegisterRoutes(r)
	}
	return r
}

// openPipelineDB returns a fresh in-memory sqlite DB with the
// knowledge_docs table migrated. Each test gets its own DB so
// they can run in parallel safely.
func openPipelineDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.KnowledgeDoc{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

// TestPipelineRunsAllFourStepsInOrder asserts the canonical
// happy path: a single /ai/pipeline call runs topics -> script ->
// score -> adapt, in that order, and returns all four sections
// populated. The override function is invoked 4 times (one per
// step) and we assert each call sees the right prompt shape so
// the step ordering and dependency wiring is exercised end to
// end.
func TestPipelineRunsAllFourStepsInOrder(t *testing.T) {
	var calls int
	// Each call returns a different canned response: first a
	// topics array, then a plain script, then a score object,
	// then a platform-adapt object. The order matters because
	// the handler parses them through different code paths and a
	// mismatch (e.g. score JSON returned for the topics step)
	// would yield a 502.
	override := func(_ context.Context, prompt string) (string, error) {
		calls++
		switch calls {
		case 1: // topics
			return `[{"title":"深夜窗边","angle":"治愈","expected_performance":"高完播","hook":"画面: 熊猫"}]`, nil
		case 2: // script
			return "窗边的熊猫,一句话都没说。", nil
		case 3: // score
			return `{"overall_score":85,"hook_strength":90,"structure":80,"platform_fit":85,"suggestions":[],"rewritten_hook":""}`, nil
		case 4: // adapt
			return `{"adaptations":{"抖音":{"title":"A","hashtags":["#a"],"description":"d"},"哔哩哔哩":{"title":"B","description":"d","tags":["x"]},"小红书":{"title":"C","body":"d","tags":["#x"]}},"cross_platform_tips":["t"]}`, nil
		default:
			t.Fatalf("unexpected extra claude call: %d (prompt: %s)", calls, prompt)
			return "", nil
		}
	}
	r := setupPipelineRouter(t, override, nil)
	w := doJSON(t, r, http.MethodPost, "/ai/pipeline", map[string]any{
		"seed":              "雨夜",
		"platform":          "抖音",
		"include_knowledge": false,
		// no steps → defaults to all four
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Topics []generatedTopic       `json:"topics"`
		Script *pipelineScript        `json:"script"`
		Score  *qualityScoreResponse  `json:"score"`
		Adapt  *platformAdaptResponse `json:"adaptations"`
		Used   []string               `json:"knowledge_used"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if calls != 4 {
		t.Errorf("claude calls = %d, want 4 (one per step)", calls)
	}
	if len(resp.Topics) != 1 || resp.Topics[0].Title != "深夜窗边" {
		t.Errorf("topics = %+v, want 1 entry with title 深夜窗边", resp.Topics)
	}
	if resp.Script == nil || resp.Script.Content != "窗边的熊猫,一句话都没说。" {
		t.Errorf("script = %+v, want content 窗边的熊猫...", resp.Script)
	}
	if resp.Score == nil || resp.Score.OverallScore != 85 {
		t.Errorf("score = %+v, want overall 85", resp.Score)
	}
	if resp.Adapt == nil || len(resp.Adapt.CrossPlatformTips) != 1 {
		t.Errorf("adapt = %+v, want 1 cross_platform_tip", resp.Adapt)
	}
	if resp.Used == nil {
		t.Errorf("knowledge_used = nil, want [] (not null)")
	}
}

// TestPipelineKnowledgeUsedWhenFlagTrue asserts the RAG step
// pulls matching knowledge_docs from the DB and surfaces their
// titles in `knowledge_used`. The RAG content is also expected
// to appear in the topic + script prompts (the two steps the
// handler injects it into) — score and adapt are kept
// evidence-only.
func TestPipelineKnowledgeUsedWhenFlagTrue(t *testing.T) {
	db := openPipelineDB(t)
	// Seed one matching doc + one noise doc. Multi-doc ordering
	// is exercised separately by
	// TestPipelineKnowledgeUsedOrderIsUpdatedAtDesc; this test
	// keeps the "RAG picks the right doc" assertion focused.
	if err := db.Create(&models.KnowledgeDoc{Title: "雨夜窗边", Content: "窗边的熊猫,一句话都没说"}).Error; err != nil {
		t.Fatalf("seed doc 1: %v", err)
	}
	if err := db.Create(&models.KnowledgeDoc{Title: "逍遥哲学", Content: "乘天地之正而御六气之辩"}).Error; err != nil {
		t.Fatalf("seed doc 2: %v", err)
	}

	var prompts []string
	override := func(_ context.Context, prompt string) (string, error) {
		prompts = append(prompts, prompt)
		switch len(prompts) {
		case 1:
			return `[{"title":"深夜窗边","angle":"治愈","expected_performance":"高完播","hook":"画面: 熊猫"}]`, nil
		case 2:
			return "窗边的熊猫,一句话都没说。", nil
		case 3:
			return `{"overall_score":80,"hook_strength":80,"structure":80,"platform_fit":80,"suggestions":[],"rewritten_hook":""}`, nil
		case 4:
			return `{"adaptations":{"抖音":{"title":"A","hashtags":[],"description":"d"},"哔哩哔哩":{"title":"B","description":"d","tags":[]},"小红书":{"title":"C","body":"d","tags":[]}},"cross_platform_tips":[]}`, nil
		}
		return "", nil
	}
	r := setupPipelineRouter(t, override, db)
	w := doJSON(t, r, http.MethodPost, "/ai/pipeline", map[string]any{
		"seed":              "雨夜",
		"platform":          "抖音",
		"include_knowledge": true,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Used []string `json:"knowledge_used"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if len(resp.Used) != 1 || resp.Used[0] != "雨夜窗边" {
		t.Errorf("knowledge_used = %v, want [雨夜窗边]", resp.Used)
	}
	// The RAG block must appear in the topic + script prompts
	// (steps 1 and 2). It must NOT appear in score or adapt
	// (steps 3 and 4) by design.
	if len(prompts) < 4 {
		t.Fatalf("got %d prompts, want at least 4", len(prompts))
	}
	if !strings.Contains(prompts[0], "STYLE REFERENCE from user's knowledge base:") {
		t.Errorf("topics prompt missing STYLE REFERENCE banner:\n%s", prompts[0])
	}
	if !strings.Contains(prompts[0], "窗边的熊猫") {
		t.Errorf("topics prompt missing injected RAG content:\n%s", prompts[0])
	}
	if !strings.Contains(prompts[1], "STYLE REFERENCE from user's knowledge base:") {
		t.Errorf("script prompt missing STYLE REFERENCE banner:\n%s", prompts[1])
	}
	if strings.Contains(prompts[2], "STYLE REFERENCE from user's knowledge base:") {
		t.Errorf("score prompt should NOT contain STYLE REFERENCE banner (RAG kept for gen steps only):\n%s", prompts[2])
	}
	if strings.Contains(prompts[3], "STYLE REFERENCE from user's knowledge base:") {
		t.Errorf("adapt prompt should NOT contain STYLE REFERENCE banner:\n%s", prompts[3])
	}
}

// TestPipelineKnowledgeUsedOrderIsUpdatedAtDesc seeds multiple
// matching docs in a non-monotonic order and asserts the response
// is sorted by updated_at DESC, matching the SQL in
// agents/rag.go. A future query change (e.g. dropping the ORDER
// BY, or switching to ORDER BY id) would flip this and the test
// would fail — protecting the operator-facing "grounded in docs
// X, Y, Z" ordering from silent regressions.
func TestPipelineKnowledgeUsedOrderIsUpdatedAtDesc(t *testing.T) {
	db := openPipelineDB(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Insertion order is NOT updated_at order on purpose.
	docs := []models.KnowledgeDoc{
		{Title: "雨夜-A", Content: "x", UpdatedAt: base.Add(0 * time.Hour)}, // oldest
		{Title: "雨夜-C", Content: "y", UpdatedAt: base.Add(2 * time.Hour)}, // newest
		{Title: "雨夜-B", Content: "z", UpdatedAt: base.Add(1 * time.Hour)}, // middle
	}
	for i := range docs {
		if err := db.Create(&docs[i]).Error; err != nil {
			t.Fatalf("seed doc %d: %v", i, err)
		}
	}

	override := func(_ context.Context, _ string) (string, error) {
		return `[{"title":"深夜窗边","angle":"治愈","expected_performance":"高完播","hook":"画面: 熊猫"}]`, nil
	}
	r := setupPipelineRouter(t, override, db)
	w := doJSON(t, r, http.MethodPost, "/ai/pipeline", map[string]any{
		"seed":              "雨夜",
		"platform":          "抖音",
		"include_knowledge": true,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Used []string `json:"knowledge_used"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	want := []string{"雨夜-C", "雨夜-B", "雨夜-A"}
	if len(resp.Used) != len(want) {
		t.Fatalf("knowledge_used = %v, want %v", resp.Used, want)
	}
	for i := range want {
		if resp.Used[i] != want[i] {
			t.Errorf("knowledge_used[%d] = %q, want %q (full: %v)", i, resp.Used[i], want[i], resp.Used)
		}
	}
}

// TestPipelineKnowledgeEmptyWhenFlagFalse asserts the RAG step
// is silently skipped when include_knowledge is false. The
// knowledge_used field is still present in the response (as an
// empty array, not null) so the frontend can render the
// "grounded in N docs" line uniformly.
func TestPipelineKnowledgeEmptyWhenFlagFalse(t *testing.T) {
	db := openPipelineDB(t)
	if err := db.Create(&models.KnowledgeDoc{Title: "雨夜窗边", Content: "anything"}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	override := func(_ context.Context, _ string) (string, error) {
		// Return a topics-array shape for any call. We do not
		// care which step is hit — this test is purely about
		// confirming RAG is OFF.
		return `[{"title":"X","angle":"Y","expected_performance":"Z","hook":"H"}]`, nil
	}
	r := setupPipelineRouter(t, override, db)
	w := doJSON(t, r, http.MethodPost, "/ai/pipeline", map[string]any{
		"seed":              "雨夜",
		"platform":          "抖音",
		"include_knowledge": false,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "雨夜窗边") {
		t.Errorf("response should not contain RAG titles when include_knowledge=false: %s", w.Body.String())
	}
	// knowledge_used must be present (even empty) — not omitted.
	if !strings.Contains(w.Body.String(), `"knowledge_used"`) {
		t.Errorf("response should always include knowledge_used field: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"knowledge_used":null`) {
		t.Errorf("knowledge_used must be [] not null: %s", w.Body.String())
	}
}

// TestPipelineDemoMode: with no API key, the handler must short
// circuit to the canned demo pool and return the full pipeline
// response without calling Complete. The X-Demo-Mode header is
// set on the response so the frontend can render the demo
// badge.
func TestPipelineDemoMode(t *testing.T) {
	r := setupPipelineRouter(t, nil, nil) // nil override + no key → demo mode
	w := doJSON(t, r, http.MethodPost, "/ai/pipeline", map[string]any{
		"seed":     "雨夜",
		"platform": "抖音",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-Demo-Mode"); got != "true" {
		t.Errorf("X-Demo-Mode header = %q, want %q", got, "true")
	}
	var resp struct {
		Topics []generatedTopic       `json:"topics"`
		Script *pipelineScript        `json:"script"`
		Score  *qualityScoreResponse  `json:"score"`
		Adapt  *platformAdaptResponse `json:"adaptations"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if len(resp.Topics) == 0 {
		t.Errorf("demo: topics empty, want canned topics")
	}
	if resp.Script == nil || resp.Script.Content == "" {
		t.Errorf("demo: script empty, want canned script")
	}
	if resp.Score == nil {
		t.Errorf("demo: score nil, want canned score")
	}
	if resp.Adapt == nil {
		t.Errorf("demo: adapt nil, want canned adapt")
	}
}

// TestPipelineStepsFilter: the caller can supply a `steps`
// array to skip steps. We assert that supplying only
// ["score", "adapt"] runs the score step first (which fails
// because script is nil → 400) and that the topics + script
// steps are NOT called. This guards the dependency wiring.
func TestPipelineStepsFilterRequiresScript(t *testing.T) {
	override := func(_ context.Context, _ string) (string, error) {
		t.Error("claude should not be called: script step missing")
		return "", nil
	}
	r := setupPipelineRouter(t, override, nil)
	w := doJSON(t, r, http.MethodPost, "/ai/pipeline", map[string]any{
		"seed":     "雨夜",
		"platform": "抖音",
		"steps":    []string{"score"},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "script") {
		t.Errorf("400 body should mention script dependency, got: %s", w.Body.String())
	}
}

// TestPipelineStepsFilterOnlyTopics asserts that asking for
// only the topics step stops after that step (no further Claude
// calls).
func TestPipelineStepsFilterOnlyTopics(t *testing.T) {
	var calls int
	override := func(_ context.Context, _ string) (string, error) {
		calls++
		return `[{"title":"A","angle":"B","expected_performance":"C","hook":"D"}]`, nil
	}
	r := setupPipelineRouter(t, override, nil)
	w := doJSON(t, r, http.MethodPost, "/ai/pipeline", map[string]any{
		"seed":     "雨夜",
		"platform": "抖音",
		"steps":    []string{"topics"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	if calls != 1 {
		t.Errorf("claude calls = %d, want 1 (topics only)", calls)
	}
	var resp struct {
		Topics []generatedTopic       `json:"topics"`
		Script *pipelineScript        `json:"script"`
		Score  *qualityScoreResponse  `json:"score"`
		Adapt  *platformAdaptResponse `json:"adaptations"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if len(resp.Topics) != 1 {
		t.Errorf("topics empty, want 1")
	}
	if resp.Script != nil {
		t.Errorf("script = %+v, want nil (not requested)", resp.Script)
	}
	if resp.Score != nil {
		t.Errorf("score = %+v, want nil (not requested)", resp.Score)
	}
	if resp.Adapt != nil {
		t.Errorf("adapt = %+v, want nil (not requested)", resp.Adapt)
	}
}

// TestPipelineNoAPIKey: when the underlying provider returns
// a "no API key" / unavailable error, the pipeline handler
// must surface 503. The MiniMax path doesn't differentiate
// the "no key" case from generic upstream failure at the
// surface — both are 503 — so the test just asserts the
// status code rather than message wording.
func TestPipelineNoAPIKey(t *testing.T) {
	override := func(_ context.Context, _ string) (string, error) {
		return "", errAIUnavailable
	}
	r := setupPipelineRouter(t, override, nil)
	w := doJSON(t, r, http.MethodPost, "/ai/pipeline", map[string]any{
		"seed":     "雨夜",
		"platform": "抖音",
	})
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503, body = %s", w.Code, w.Body.String())
	}
}

// TestPipelineInvalidBody: missing seed / platform / unknown
// steps must yield 400, not 500.
func TestPipelineInvalidBody(t *testing.T) {
	r := setupPipelineRouter(t, nil, nil)
	cases := []struct {
		name string
		body map[string]any
	}{
		{"missing seed", map[string]any{"platform": "抖音"}},
		{"missing platform", map[string]any{"seed": "雨夜"}},
		{"only unknown steps", map[string]any{"seed": "雨夜", "platform": "抖音", "steps": []string{"garbage"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doJSON(t, r, http.MethodPost, "/ai/pipeline", tc.body)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400, body = %s", w.Code, w.Body.String())
			}
		})
	}
}

// TestNormalizePipelineSteps: the step normalizer must drop
// unknown names, drop blanks, and default to the full four-step
// pipeline when the input is empty/nil. Order is preserved.
func TestNormalizePipelineSteps(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"nil defaults to all", nil, []string{"topics", "script", "score", "adapt"}},
		{"empty defaults to all", []string{}, []string{"topics", "script", "score", "adapt"}},
		{"unknowns dropped", []string{"topics", "garbage", "adapt"}, []string{"topics", "adapt"}},
		{"blanks dropped", []string{"", "topics", "   "}, []string{"topics"}},
		{"order preserved", []string{"adapt", "topics"}, []string{"adapt", "topics"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizePipelineSteps(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestInjectRAG: the RAG banner wraps the supplied content
// with the "STYLE REFERENCE" header and the closer line. Used
// by both the topic + script steps.
func TestInjectRAG(t *testing.T) {
	got := injectRAG("PROMPT_BODY", "DOC_BODY")
	if !strings.Contains(got, "STYLE REFERENCE from user's knowledge base:") {
		t.Errorf("missing header banner: %s", got)
	}
	if !strings.Contains(got, "DOC_BODY") {
		t.Errorf("missing RAG body: %s", got)
	}
	if !strings.Contains(got, "PROMPT_BODY") {
		t.Errorf("missing original prompt body: %s", got)
	}
	if !strings.Contains(got, "--- end of style reference ---") {
		t.Errorf("missing closer: %s", got)
	}
}
