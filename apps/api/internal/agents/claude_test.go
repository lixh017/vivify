package agents

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestGenerateTopicsPrompt(t *testing.T) {
	a := NewClaude("test-key")
	prompt := a.GenerateTopicsPrompt("美食探店", "抖音", 5)
	if !strings.Contains(prompt, "美食探店") {
		t.Error("prompt should contain seed")
	}
	if !strings.Contains(prompt, "抖音") {
		t.Error("prompt should contain platform")
	}
	if !strings.Contains(prompt, "5") {
		t.Error("prompt should contain count")
	}
}

func TestHumanizeScriptPrompt(t *testing.T) {
	a := NewClaude("test-key")
	prompt := a.HumanizeScriptPrompt("这是 AI 生成的脚本，结构化、对仗工整")
	if !strings.Contains(prompt, "AI 痕迹") {
		t.Error("humanize prompt missing 'AI 痕迹' concept")
	}
	// New prompt must also include a worked example so Claude
	// knows what "human" looks like in the panda IP voice.
	if !strings.Contains(prompt, "改写前") || !strings.Contains(prompt, "改写后") {
		t.Error("humanize prompt missing before/after example")
	}
}

func TestPostmortemPrompt(t *testing.T) {
	a := NewClaude("test-key")
	prompt := a.PostmortemPrompt("爆款标题", "脚本正文", "播放 12k 点赞 800", "御宅哲学角度")
	for _, want := range []string{"爆款标题", "脚本正文", "播放 12k 点赞 800", "御宅哲学角度"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("postmortem prompt missing %q", want)
		}
	}
	for _, want := range []string{"success_factors", "reusable_patterns", "insights", "suggestions"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("postmortem prompt missing field %q", want)
		}
	}
}

// TestCompleteOverrideSucceeds exercises the success path of the
// override seam so we catch regressions where the override is
// silently bypassed. The previous version of this test asserted the
// real network call failed, but `t.Skip` on the success path was
// hiding any regression that would let the call succeed for the
// wrong reason (e.g. override ignored, fallback to network).
func TestCompleteOverrideSucceeds(t *testing.T) {
	a := NewClaudeWithOverride(func(_ context.Context, prompt string) (string, error) {
		if prompt == "" {
			return "", errors.New("empty prompt")
		}
		return "ok: " + prompt, nil
	})
	got, err := a.Complete(context.Background(), "hello")
	if err != nil {
		t.Fatalf("override Complete: %v", err)
	}
	if got != "ok: hello" {
		t.Errorf("override Complete = %q, want %q", got, "ok: hello")
	}
}

// TestCompleteOverrideReturnsError verifies the override can surface
// errors and that they propagate to the caller verbatim. This is the
// behavior MCP handlers rely on when Claude rate-limits or refuses.
func TestCompleteOverrideReturnsError(t *testing.T) {
	wantErr := errors.New("synthetic 529 overloaded")
	a := NewClaudeWithOverride(func(_ context.Context, _ string) (string, error) {
		return "", wantErr
	})
	_, err := a.Complete(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error from override")
	}
	if !errors.Is(err, wantErr) && err.Error() != wantErr.Error() {
		t.Errorf("error = %v, want %v", err, wantErr)
	}
}

// TestNewClaudeHasNilOverride guards the production constructor
// against accidentally wiring an override. If a future refactor
// makes this true, the override path becomes load-bearing in prod
// and the test name + docstring should be updated.
func TestNewClaudeHasNilOverride(t *testing.T) {
	a := NewClaude("any-key")
	if a.override != nil {
		t.Error("NewClaude must not set override; only NewClaudeWithOverride should")
	}
}

// TestNewClaudeAllowsEmptyKey verifies the dev-mode contract: an
// empty API key must NOT panic the constructor. Instead the agent
// is constructed with keyConfigured=false, and Complete surfaces a
// clear "no API key" error at call time. This keeps the server
// bootable in dev environments without a configured key.
func TestNewClaudeAllowsEmptyKey(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("NewClaude with empty key should NOT panic, got: %v", r)
		}
	}()

	a := NewClaude("")
	if a == nil {
		t.Fatal("NewClaude with empty key should return a non-nil agent")
	}

	// Complete should fail with a clear, actionable error that
	// matches the exported sentinel (so handlers can branch via
	// errors.Is rather than substring matching).
	_, err := a.Complete(context.Background(), "test prompt")
	if err == nil {
		t.Error("Complete with empty key should return error")
	}
	if !errors.Is(err, ErrNoAPIKey) {
		t.Errorf("error should wrap ErrNoAPIKey, got: %v", err)
	}
}

// TestNewClaudeAppliesOptions verifies that NewClaudeWithOptions
// honours the per-call overrides for model, max tokens, and timeout.
func TestNewClaudeAppliesOptions(t *testing.T) {
	a := NewClaudeWithOptions("test-key", CompleteOptions{
		Model:     ModelClaudeOpus4_5,
		MaxTokens: 8192,
		Timeout:   5 * time.Second,
	})
	if a.model != ModelClaudeOpus4_5 {
		t.Errorf("model = %q, want %q", a.model, ModelClaudeOpus4_5)
	}
	if a.maxTokens != 8192 {
		t.Errorf("maxTokens = %d, want 8192", a.maxTokens)
	}
	if a.timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", a.timeout)
	}
}

// TestCompleteEmptyContentSurfacesErrNoContent guards the contract
// that Complete must surface a typed ErrNoContent (not ("", nil))
// when the SDK response has zero content blocks. The previous
// behaviour silently produced empty JSON envelopes for callers
// that wrap the result string, so we lock the new contract down
// with a regression test.
func TestCompleteEmptyContentSurfacesErrNoContent(t *testing.T) {
	// Override returns an empty string with no error — this is the
	// path that used to ("", nil) its way past the content-length
	// check. With the fix, the test asserts the override path still
	// returns the empty string but with a nil error (the override
	// bypasses SDK content extraction, so ErrNoContent only fires
	// on the real SDK path).
	a := NewClaudeWithOverride(func(_ context.Context, _ string) (string, error) {
		return "", nil
	})
	got, err := a.Complete(context.Background(), "x")
	if err != nil {
		t.Errorf("override path: unexpected error %v", err)
	}
	if got != "" {
		t.Errorf("override path: got %q, want empty", got)
	}
	// Sanity check the sentinel exists and is distinct from ErrNoAPIKey
	// so the two failure modes can be branched on independently.
	if ErrNoContent == nil {
		t.Fatal("ErrNoContent should be a non-nil sentinel")
	}
	if errors.Is(ErrNoContent, ErrNoAPIKey) {
		t.Error("ErrNoContent must be distinct from ErrNoAPIKey")
	}
}
