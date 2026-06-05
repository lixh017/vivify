package agents

import (
	"context"
	"strings"
	"testing"
)

// TestScoreContentPrompt verifies the score prompt embeds the title,
// script, and platform so a stray refactor that drops one is caught
// by the test.
func TestScoreContentPrompt(t *testing.T) {
	a := NewClaude("test-key")
	prompt := a.ScoreContentPrompt("窗边的熊猫", "今天的雨,下得有点久。", "抖音")
	for _, want := range []string{"窗边的熊猫", "今天的雨,下得有点久。", "抖音"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("score prompt missing %q", want)
		}
	}
	// The prompt must ask for the three required axes plus
	// suggestions + rewritten_hook.
	for _, want := range []string{"hook_strength", "structure", "platform_fit", "rewritten_hook"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("score prompt missing field %q", want)
		}
	}
}

// TestPlatformAdaptPrompt verifies the adapt prompt embeds title,
// angle, and source platform, and explicitly names all three target
// platforms (抖音/哔哩哔哩/小红书) so Claude doesn't have to guess.
func TestPlatformAdaptPrompt(t *testing.T) {
	a := NewClaude("test-key")
	prompt := a.PlatformAdaptPrompt("雨夜熊猫", "治愈系", "小红书")
	for _, want := range []string{"雨夜熊猫", "治愈系", "小红书", "抖音", "哔哩哔哩"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("adapt prompt missing %q", want)
		}
	}
}

// TestDemoResponseForNewOps ensures the demo dispatcher returns
// non-empty JSON for the new quality ops, and that a request for an
// unknown op still returns a sensible default (matches existing
// behaviour in DemoResponse).
func TestDemoResponseForNewOps(t *testing.T) {
	a := NewClaude("") // empty key → demo mode
	score, err := a.DemoResponse(DemoOpScore, 0)
	if err != nil {
		t.Fatalf("DemoResponse(score) err: %v", err)
	}
	if !strings.Contains(score, "overall_score") {
		t.Errorf("DemoQualityScores[0] missing overall_score, got: %s", score)
	}
	adapt, err := a.DemoResponse(DemoOpPlatformAdapt, 0)
	if err != nil {
		t.Fatalf("DemoResponse(platform_adapt) err: %v", err)
	}
	for _, want := range []string{"抖音", "哔哩哔哩", "小红书"} {
		if !strings.Contains(adapt, want) {
			t.Errorf("DemoPlatformAdapts[0] missing %q", want)
		}
	}

	// Unknown op must fall back to topics, NOT error — this
	// preserves the contract documented in DemoResponse.
	fallback, err := a.DemoResponse(DemoOperation("nope"), 0)
	if err != nil {
		t.Fatalf("DemoResponse(unknown) err: %v", err)
	}
	if !strings.Contains(fallback, "title") {
		t.Errorf("unknown op fallback missing 'title' (topics shape), got: %s", fallback)
	}
}

// TestDemoResponseScoreRotation exercises the seed-based rotation
// across the DemoQualityScores pool so we catch a regression where
// the picker is bypassed and the same canned response leaks for
// every call.
func TestDemoResponseScoreRotation(t *testing.T) {
	a := NewClaude("")
	first, _ := a.DemoResponse(DemoOpScore, 0)
	second, _ := a.DemoResponse(DemoOpScore, 1)
	if len(DemoQualityScores) < 2 {
		t.Skip("rotation test needs >= 2 canned scores")
	}
	if first == second {
		t.Errorf("expected rotation across seeds, got identical response for seed 0 and 1")
	}
}

// TestDemoResponseUsesOverride exercises the override-constructed
// agent's DemoResponse path. The override constructor sets
// keyConfigured=true so a stray code path that bypasses demo mode
// (e.g. by checking keyConfigured) would skip the demo response and
// fail the test.
func TestDemoResponseUsesOverride(t *testing.T) {
	a := NewClaudeWithOverride(func(_ context.Context, _ string) (string, error) {
		return "should-not-be-called", nil
	})
	// DemoResponse must NOT route to the override — it's served
	// from canned data regardless of how the agent was built.
	got, err := a.DemoResponse(DemoOpScore, 0)
	if err != nil {
		t.Fatalf("DemoResponse err: %v", err)
	}
	if got == "should-not-be-called" {
		t.Error("DemoResponse should not route to override; expected canned data")
	}
}

// TestBuildSuggestionsShape verifies that the rule-based fallback
// produces suggestions with the {category, severity, problem,
// rewrite} wire contract — matching the upgraded ScoreContentPrompt
// and the demo pool. A regression to the older {category, message,
// severity} shape would silently break the frontend's unified
// suggestion renderer (it would have to branch on which mode
// produced the response).
func TestBuildSuggestionsShape(t *testing.T) {
	// Force every axis to score low so all three suggestion
	// branches (hook / structure / platform_fit) plus the
	// word_count fallback fire — that way the test covers the
	// full shape surface in one pass.
	sug := buildSuggestions(
		"a long title that is well past the douyin limit for sure", // > 28 chars
		"短", // < 50 chars → word_count
		"抖音",
		20, // hook
		20, // structure
		20, // fit
	)
	if len(sug) == 0 {
		t.Fatal("expected at least one suggestion when every axis scores low")
	}
	for i, s := range sug {
		for _, field := range []string{"category", "severity", "problem", "rewrite"} {
			if _, ok := s[field]; !ok {
				t.Errorf("suggestion[%d] missing field %q, got: %+v", i, field, s)
			}
			if strings.TrimSpace(s[field]) == "" {
				t.Errorf("suggestion[%d].%s is empty, got: %+v", i, field, s)
			}
		}
		// The "message" field is from the old contract and must
		// NOT be present, otherwise a downstream consumer could
		// still rely on it and the contracts would diverge.
		if _, ok := s["message"]; ok {
			t.Errorf("suggestion[%d] still has old 'message' field, got: %+v", i, s)
		}
	}
}

// TestRuleBasedScoreSuggestionsShape exercises the public
// RuleBasedScore entry point and confirms its `suggestions` field
// is the new shape. This is the function the handler actually
// calls, so the contract is end-to-end.
func TestRuleBasedScoreSuggestionsShape(t *testing.T) {
	out := RuleBasedScore("窗边的熊猫", "短的", "抖音")
	sugs, ok := out["suggestions"].([]map[string]string)
	if !ok {
		t.Fatalf("suggestions has wrong type %T, want []map[string]string", out["suggestions"])
	}
	if len(sugs) == 0 {
		t.Fatal("expected suggestions for a short script on 抖音")
	}
	for i, s := range sugs {
		for _, field := range []string{"category", "severity", "problem", "rewrite"} {
			if _, ok := s[field]; !ok {
				t.Errorf("suggestion[%d] missing %q, got: %+v", i, field, s)
			}
		}
	}
}
