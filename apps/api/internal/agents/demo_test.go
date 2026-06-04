package agents

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestIsDemoModeNoKey verifies the dev-mode contract: an agent
// constructed without an API key auto-falls-back to demo mode even
// when the caller has not asked for it.
func TestIsDemoModeNoKey(t *testing.T) {
	a := NewClaude("")
	if !a.IsDemoMode(false) {
		t.Error("agent without API key should be in demo mode by default")
	}
	if !a.IsDemoMode(true) {
		t.Error("agent without API key + forceDemo should still be in demo mode")
	}
}

// TestIsDemoModeWithKey verifies the opposite: with a real key, demo
// mode only kicks in when explicitly requested.
func TestIsDemoModeWithKey(t *testing.T) {
	a := NewClaude("real-key")
	if a.IsDemoMode(false) {
		t.Error("agent with API key should NOT be in demo mode by default")
	}
	if !a.IsDemoMode(true) {
		t.Error("agent with API key + forceDemo=true should be in demo mode")
	}
}

// TestKeyConfigured verifies the simple boolean accessor.
func TestKeyConfigured(t *testing.T) {
	if NewClaude("real-key").KeyConfigured() != true {
		t.Error("NewClaude with real key should report KeyConfigured=true")
	}
	if NewClaude("").KeyConfigured() != false {
		t.Error("NewClaude with empty key should report KeyConfigured=false")
	}
}

// TestDemoResponseTopicsIsValidJSON ensures the canned topics JSON is
// well-formed and matches the wire shape the handler expects. If a
// future edit breaks the JSON, this test fails before the demo does.
func TestDemoResponseTopicsIsValidJSON(t *testing.T) {
	a := NewClaude("")
	raw, err := a.DemoResponse(DemoOpTopics, 0)
	if err != nil {
		t.Fatalf("DemoResponse(topics) err: %v", err)
	}
	var topics []struct {
		Title               string `json:"title"`
		Angle               string `json:"angle"`
		ExpectedPerformance string `json:"expected_performance"`
		Hook                string `json:"hook"`
	}
	if err := json.Unmarshal([]byte(raw), &topics); err != nil {
		t.Fatalf("canned topics JSON does not parse: %v\n%s", err, raw)
	}
	if len(topics) != 5 {
		t.Errorf("expected 5 canned topics, got %d", len(topics))
	}
	// Spot-check one entry to make sure the fields are populated.
	if topics[0].Title == "" || topics[0].Hook == "" {
		t.Errorf("topic[0] missing title/hook: %+v", topics[0])
	}
}

// TestDemoResponseTopicsCoversIPRange checks that the canned topics
// cover the 4 IP tracks (治愈/御宅/哲学/国潮) — the whole point of
// the demo is to show the IP's range, so missing a track defeats it.
func TestDemoResponseTopicsCoversIPRange(t *testing.T) {
	a := NewClaude("")
	raw, _ := a.DemoResponse(DemoOpTopics, 0)
	tracks := []string{"治愈", "御宅", "哲学", "国潮"}
	for _, track := range tracks {
		if !strings.Contains(raw, track) {
			t.Errorf("canned topics missing track %q (need all 4: 治愈/御宅/哲学/国潮)", track)
		}
	}
}

// TestDemoResponseHumanize ensures the canned humanize response is
// non-empty plain text (no JSON wrapper — the handler echoes it as
// the `humanized` string).
func TestDemoResponseHumanize(t *testing.T) {
	a := NewClaude("")
	raw, err := a.DemoResponse(DemoOpHumanize, 0)
	if err != nil {
		t.Fatalf("DemoResponse(humanize) err: %v", err)
	}
	if strings.TrimSpace(raw) == "" {
		t.Error("canned humanize response should not be empty")
	}
	// Sanity check: the humanized output should contain at least one
	// 停顿/语气词 marker (省略号/括号), otherwise the demo doesn't
	// show off the humanize transform.
	if !strings.Contains(raw, "...") && !strings.Contains(raw, "(") {
		t.Errorf("canned humanize response should contain 停顿/语气词 markers, got: %s", raw)
	}
}

// TestDemoResponseHumanizeVariety verifies the seed rotates which
// canned humanize response is returned, so a demo running multiple
// calls doesn't show the exact same text every time.
func TestDemoResponseHumanizeVariety(t *testing.T) {
	a := NewClaude("")
	first, _ := a.DemoResponse(DemoOpHumanize, 0)
	second, _ := a.DemoResponse(DemoOpHumanize, 1)
	if first == second && len(DemoHumanizedScripts) > 1 {
		t.Error("DemoResponse(humanize) should rotate across seeds when pool has >1 entry")
	}
}

// TestDemoResponsePostmortemIsValidJSON ensures the canned
// postmortem JSON parses into the 4-field shape the handler expects.
func TestDemoResponsePostmortemIsValidJSON(t *testing.T) {
	a := NewClaude("")
	raw, err := a.DemoResponse(DemoOpPostmortem, 0)
	if err != nil {
		t.Fatalf("DemoResponse(postmortem) err: %v", err)
	}
	var parsed struct {
		SuccessFactors   []string `json:"success_factors"`
		ReusablePatterns []string `json:"reusable_patterns"`
		Insights         []string `json:"insights"`
		Suggestions      []string `json:"suggestions"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("canned postmortem JSON does not parse: %v\n%s", err, raw)
	}
	if len(parsed.SuccessFactors) == 0 {
		t.Error("canned postmortem missing success_factors")
	}
	if len(parsed.ReusablePatterns) == 0 {
		t.Error("canned postmortem missing reusable_patterns")
	}
	if len(parsed.Insights) == 0 {
		t.Error("canned postmortem missing insights")
	}
	if len(parsed.Suggestions) == 0 {
		t.Error("canned postmortem missing suggestions")
	}
}

// TestDemoResponseUnknownOpFallsBack guards the default branch:
// passing an unknown op should not panic and should return a usable
// (non-empty) canned response so the demo still works.
func TestDemoResponseUnknownOpFallsBack(t *testing.T) {
	a := NewClaude("")
	raw, err := a.DemoResponse(DemoOperation("bogus"), 0)
	if err != nil {
		t.Fatalf("DemoResponse(bogus) err: %v", err)
	}
	if strings.TrimSpace(raw) == "" {
		t.Error("DemoResponse with unknown op should still return a non-empty fallback")
	}
}

// TestPickDemoIndexZeroDivisor guards the (n == 0) branch — a
// caller that passes an empty pool should not divide by zero.
func TestPickDemoIndexZeroDivisor(t *testing.T) {
	if got := pickDemoIndex(7, 0); got != 0 {
		t.Errorf("pickDemoIndex(7, 0) = %d, want 0", got)
	}
}

// TestPickDemoIndexNegativeSeed verifies the negative-seed branch
// returns a non-negative index (otherwise slice access would panic).
func TestPickDemoIndexNegativeSeed(t *testing.T) {
	if got := pickDemoIndex(-13, 3); got < 0 || got >= 3 {
		t.Errorf("pickDemoIndex(-13, 3) = %d, want in [0,3)", got)
	}
}
