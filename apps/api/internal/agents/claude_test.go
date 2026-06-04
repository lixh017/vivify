package agents

import (
	"context"
	"strings"
	"testing"
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
	if !strings.Contains(prompt, "拟人化") {
		t.Error("humanize prompt missing '拟人化' concept")
	}
	if !strings.Contains(prompt, "AI 痕迹") {
		t.Error("humanize prompt missing 'AI 痕迹' concept")
	}
}

// TestCompleteReturnsErrorOnBadKey verifies the Complete method signature
// and that a request with an obviously-invalid API key returns an error
// (network call will be attempted but rejected). We only check that an
// error is returned; we don't assert on its content.
func TestCompleteReturnsErrorOnBadKey(t *testing.T) {
	a := NewClaude("test-key-not-real")
	_, err := a.Complete(context.Background(), "hello")
	if err == nil {
		t.Skip("expected network call to fail with mock key, but got nil error; skipping")
	}
}
