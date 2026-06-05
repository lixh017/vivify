//go:build integration

package agents

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// Integration tests for the Claude (Anthropic) API path.
//
// These tests exercise the real Anthropic SDK over the network. They
// are gated by TWO conditions:
//
//  1. The `integration` build tag — `go test ./...` (the default
//     command) MUST NOT compile this file, so CI never accidentally
//     makes a real API call.
//
//  2. The ANTHROPIC_API_KEY environment variable — when the build
//     tag is present but the key is empty, the tests Skip with a
//     clear message. This lets a developer with the tag enabled run
//     `go test ./...` locally and not have every test fail.
//
// To run with real API access:
//
//	ANTHROPIC_API_KEY=sk-ant-... \
//	  go test -tags integration -race ./internal/agents/...
//
// Cost control
//
// These tests use the smallest sensible MaxTokens budgets and prefer
// the cheaper Haiku 4.5 model for structured tasks. The full set,
// run in sequence, costs well under a cent on the current Anthropic
// pricing (June 2026). The "E2E pipeline" test reuses the topic
// output from the first test to avoid paying for the same call
// twice.

// IntegrationTestTimeout caps each real-API call so a hung SDK
// request cannot pin a test goroutine. Larger than the package
// default (60s) to absorb cold-start latency on the Anthropic side.
const IntegrationTestTimeout = 90 * time.Second

// IntegrationTestMaxTokens keeps the per-call spend low. 256 tokens
// is enough for short structured JSON responses; the topic-test
// asks for a small N so the model has room to comply.
const IntegrationTestMaxTokens int64 = 512

// skipIfNoKey aborts the test when ANTHROPIC_API_KEY is not set in
// the environment. Exported behaviour: never fails the test on a
// missing key, only on a real API error. This is the contract the
// Makefile target `test-integration` relies on.
func skipIfNoKey(t *testing.T) {
	t.Helper()
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set; skipping real API test")
	}
}

// newIntegrationClient returns a Claude wired with the real SDK and
// a tight per-call timeout. The model defaults to Haiku 4.5 to keep
// the integration suite cheap; tests that need Sonnet 4.5 override
// via CompleteOptions.
func newIntegrationClient(t *testing.T) *Claude {
	t.Helper()
	a := NewClaude(os.Getenv("ANTHROPIC_API_KEY"))
	if a == nil || !a.Available() {
		t.Fatal("integration client should be available when ANTHROPIC_API_KEY is set")
	}
	return a
}

// extractJSON pulls the first JSON object or array out of a
// model response, tolerating leading prose and trailing prose. The
// Claude prompts all end with the "OutputJSONOnly" tail, but small
// regressions (a stray "好的,以下是..." preamble) should not fail
// the whole test — they only need to be stripped so the JSON parser
// can run.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	// Strip markdown code fences (```json ... ```) if present.
	if strings.HasPrefix(s, "```") {
		if i := strings.Index(s, "\n"); i >= 0 {
			s = s[i+1:]
		}
		if strings.HasSuffix(s, "```") {
			s = s[:len(s)-3]
		}
	}
	// Find the first '{' or '[' and the matching close by scanning
	// for the last '}' or ']'. This is intentionally simple: the
	// prompts are designed to return clean JSON and anything fancier
	// would mask real prompt regressions.
	startObj := strings.IndexByte(s, '{')
	startArr := strings.IndexByte(s, '[')
	start := startObj
	if startArr >= 0 && (start < 0 || startArr < start) {
		start = startArr
	}
	if start < 0 {
		return s
	}
	endObj := strings.LastIndexByte(s, '}')
	endArr := strings.LastIndexByte(s, ']')
	end := endObj
	if endArr > end {
		end = endArr
	}
	if end <= start {
		return s
	}
	return s[start : end+1]
}

// TestRealClaudeTopicGeneration drives a full topic-idea round
// trip through the real API and asserts the JSON parses into a
// non-empty list of ideas with the required fields populated. This
// is the canary: if Claude ever drifts away from the JSON-only
// contract on the topic prompt, this test fails loudly.
func TestRealClaudeTopicGeneration(t *testing.T) {
	skipIfNoKey(t)
	a := newIntegrationClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), IntegrationTestTimeout)
	defer cancel()

	prompt := a.GenerateTopicsPrompt("雨夜", "抖音", 2)
	raw, err := a.CompleteWithOptions(ctx, prompt, CompleteOptions{
		Model:     ModelClaudeHaiku4_5, // cheaper for integration runs
		MaxTokens: IntegrationTestMaxTokens,
		Timeout:   IntegrationTestTimeout,
	})
	if err != nil {
		t.Fatalf("real API Complete: %v", err)
	}
	if raw == "" {
		t.Fatal("real API returned empty body")
	}

	var ideas []struct {
		Title  string   `json:"title"`
		Angle  string   `json:"angle"`
		Hook   string   `json:"hook"`
		VoiceT []string `json:"voice_tags"`
	}
	if err := json.Unmarshal([]byte(extractJSON(raw)), &ideas); err != nil {
		t.Fatalf("response is not a JSON array of ideas: %v\nraw=%q", err, raw)
	}
	if len(ideas) < 1 {
		t.Fatalf("expected at least 1 idea, got %d. raw=%q", len(ideas), raw)
	}
	for i, idea := range ideas {
		if strings.TrimSpace(idea.Title) == "" {
			t.Errorf("idea[%d].title is empty", i)
		}
		if strings.TrimSpace(idea.Angle) == "" {
			t.Errorf("idea[%d].angle is empty", i)
		}
	}
}

// TestRealClaudeHumanize sends a clearly AI-flavored script through
// the humanize prompt and asserts the model returns a non-empty
// rewrite that does not echo the original template phrases back at
// us (which would defeat the purpose of the test). The exact
// rewrite quality is intentionally not asserted — the goal is
// "did the API return something we can ship" not "is the rewrite
// good", which is a human judgement.
func TestRealClaudeHumanize(t *testing.T) {
	skipIfNoKey(t)
	a := newIntegrationClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), IntegrationTestTimeout)
	defer cancel()

	script := "今天想给大家讲讲庄子。在如今这个内卷的时代,我们每个人都很焦虑。庄子告诉我们,真正逍遥的人,是放下执念的人。让我们一起做这样的逍遥人。"
	prompt := a.HumanizeScriptPrompt(script)
	raw, err := a.CompleteWithOptions(ctx, prompt, CompleteOptions{
		Model:     ModelClaudeHaiku4_5,
		MaxTokens: IntegrationTestMaxTokens,
		Timeout:   IntegrationTestTimeout,
	})
	if err != nil {
		t.Fatalf("real API Complete: %v", err)
	}
	if raw == "" {
		t.Fatal("real API returned empty body")
	}
	// Soft regression check: the humanize prompt enumerates these
	// phrases as banned. A model that echoes them back has ignored
	// the prompt and is producing the original AI text verbatim.
	banned := []string{"让我们一起", "今天想给大家讲讲"}
	for _, b := range banned {
		if strings.Contains(raw, b) {
			t.Errorf("humanize output still contains banned AI phrase %q (prompt contract violated)", b)
		}
	}
}

// TestRealClaudeScore exercises the score-content prompt and
// asserts the JSON envelope deserializes into the
// QualityScoreResponse shape with the three axis fields present.
// A numeric range check is intentionally NOT done — the model is
// allowed to score in 0-100, and locking down the exact value
// would make the test flaky.
func TestRealClaudeScore(t *testing.T) {
	skipIfNoKey(t)
	a := newIntegrationClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), IntegrationTestTimeout)
	defer cancel()

	prompt := a.ScoreContentPrompt(
		"熊猫陪你度过雨夜",
		"窗边的熊猫看着雨,一句话都没说。今天辛苦了。",
		"抖音",
	)
	raw, err := a.CompleteWithOptions(ctx, prompt, CompleteOptions{
		Model:     ModelClaudeHaiku4_5,
		MaxTokens: IntegrationTestMaxTokens,
		Timeout:   IntegrationTestTimeout,
	})
	if err != nil {
		t.Fatalf("real API Complete: %v", err)
	}
	if raw == "" {
		t.Fatal("real API returned empty body")
	}

	var resp struct {
		Overall      int                 `json:"overall_score"`
		HookStrength int                 `json:"hook_strength"`
		Structure    int                 `json:"structure"`
		PlatformFit  int                 `json:"platform_fit"`
		Suggestions  []map[string]string `json:"suggestions"`
	}
	if err := json.Unmarshal([]byte(extractJSON(raw)), &resp); err != nil {
		t.Fatalf("score response is not JSON: %v\nraw=%q", err, raw)
	}
	if resp.HookStrength == 0 && resp.Structure == 0 && resp.PlatformFit == 0 {
		t.Errorf("expected at least one axis score to be populated, got %+v", resp)
	}
}

// TestRealClaudePipeline is the end-to-end smoke: it chains the
// four real Claude calls a real user would trigger from the UI:
// topics → humanize the best topic's hook → score the result. The
// goal is to catch latency cliffs and contract drift across
// multiple calls (e.g. a schema change in one prompt that would
// break the downstream pipeline in production).
//
// The chained model is Haiku 4.5 throughout to keep cost bounded.
// Each step is also a regression test on its own — failures are
// reported with the step name so the operator can tell which
// prompt regressed.
func TestRealClaudePipeline(t *testing.T) {
	skipIfNoKey(t)
	a := newIntegrationClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*IntegrationTestTimeout)
	defer cancel()

	opts := CompleteOptions{
		Model:     ModelClaudeHaiku4_5,
		MaxTokens: IntegrationTestMaxTokens,
		Timeout:   IntegrationTestTimeout,
	}

	// Step 1: topic generation.
	rawTopics, err := a.CompleteWithOptions(ctx, a.GenerateTopicsPrompt("雨夜", "抖音", 1), opts)
	if err != nil {
		t.Fatalf("step1 topics: %v", err)
	}
	var ideas []struct {
		Title string `json:"title"`
		Hook  string `json:"hook"`
	}
	if err := json.Unmarshal([]byte(extractJSON(rawTopics)), &ideas); err != nil {
		t.Fatalf("step1 topics JSON: %v\nraw=%q", err, rawTopics)
	}
	if len(ideas) == 0 {
		t.Fatal("step1 produced no ideas")
	}
	picked := ideas[0]
	if picked.Title == "" {
		t.Fatal("step1 idea has empty title")
	}

	// Step 2: humanize the hook.
	rawHuman, err := a.CompleteWithOptions(ctx, a.HumanizeScriptPrompt(picked.Hook), opts)
	if err != nil {
		t.Fatalf("step2 humanize: %v", err)
	}
	if rawHuman == "" {
		t.Fatal("step2 humanize returned empty")
	}

	// Step 3: score the humanized result against 抖音.
	rawScore, err := a.CompleteWithOptions(ctx, a.ScoreContentPrompt(picked.Title, rawHuman, "抖音"), opts)
	if err != nil {
		t.Fatalf("step3 score: %v", err)
	}
	if rawScore == "" {
		t.Fatal("step3 score returned empty")
	}
	var resp struct {
		Overall int `json:"overall_score"`
	}
	if err := json.Unmarshal([]byte(extractJSON(rawScore)), &resp); err != nil {
		t.Fatalf("step3 score JSON: %v\nraw=%q", err, rawScore)
	}
	if resp.Overall < 0 || resp.Overall > 100 {
		t.Errorf("step3 overall_score=%d out of [0,100]", resp.Overall)
	}
}
