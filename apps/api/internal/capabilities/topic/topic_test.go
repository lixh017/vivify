// Package topic_test contains unit tests for the topic capability
// library (internal/capabilities/topic). Tests use the
// agents.NewMiniMaxWithTextOverride hook to mock the LLM, so
// no real API key is required.
package topic_test

import (
	"context"
	"strings"
	"testing"

	"github.com/opc/api/internal/agents"
	"github.com/opc/api/internal/capabilities/topic"
)

// ====================
// validate tests (4)
// ====================

func TestValidateEmptySeed(t *testing.T) {
	t.Parallel()
	_, err := topic.Generate(context.Background(), nil, topic.Input{
		Seed: "", Platform: "抖音", Count: 5,
	})
	if err == nil {
		t.Fatal("expected error for empty seed")
	}
	if !strings.Contains(err.Error(), "seed") {
		t.Errorf("error %q should mention 'seed'", err.Error())
	}
}

func TestValidateEmptyPlatform(t *testing.T) {
	t.Parallel()
	_, err := topic.Generate(context.Background(), nil, topic.Input{
		Seed: "个人成长", Platform: "", Count: 5,
	})
	if err == nil {
		t.Fatal("expected error for empty platform")
	}
	if !strings.Contains(err.Error(), "platform") {
		t.Errorf("error %q should mention 'platform'", err.Error())
	}
}

func TestValidateCountZero(t *testing.T) {
	t.Parallel()
	_, err := topic.Generate(context.Background(), nil, topic.Input{
		Seed: "个人成长", Platform: "抖音", Count: 0,
	})
	if err == nil {
		t.Fatal("expected error for count <= 0")
	}
	if !strings.Contains(err.Error(), "count") {
		t.Errorf("error %q should mention 'count'", err.Error())
	}
}

func TestValidateCountTooLarge(t *testing.T) {
	t.Parallel()
	_, err := topic.Generate(context.Background(), nil, topic.Input{
		Seed: "个人成长", Platform: "抖音", Count: 21,
	})
	if err == nil {
		t.Fatal("expected error for count > 20")
	}
	if !strings.Contains(err.Error(), "count") {
		t.Errorf("error %q should mention 'count'", err.Error())
	}
}

// ====================
// parseTopics tests (5) — exercised via Generate mock
// ====================

func TestParseTopicsPerfect(t *testing.T) {
	t.Parallel()
	raw := `[{"title":"T1","angle":"A1","expected_performance":"P1","hook":"H1","pattern":"PAT","voice_tags":["a","b"]}]`
	res, err := runWithMockLLM(t, raw, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Topics) != 1 {
		t.Fatalf("len(Topics) = %d, want 1", len(res.Topics))
	}
	if res.Topics[0].Title != "T1" {
		t.Errorf("Title = %q, want T1", res.Topics[0].Title)
	}
}

func TestParseTopicsMarkdownFence(t *testing.T) {
	t.Parallel()
	raw := "```json\n[{\"title\":\"T1\",\"angle\":\"A1\",\"expected_performance\":\"P1\",\"hook\":\"H1\"}]\n```"
	res, err := runWithMockLLM(t, raw, nil)
	if err != nil {
		t.Fatalf("markdown-fence should be stripped, got: %v", err)
	}
	if len(res.Topics) != 1 {
		t.Errorf("len(Topics) = %d, want 1", len(res.Topics))
	}
}

func TestParseTopicsNoArray(t *testing.T) {
	t.Parallel()
	raw := "no JSON array here, just text"
	_, err := runWithMockLLM(t, raw, nil)
	if err == nil {
		t.Fatal("expected error when no JSON array in response")
	}
	if !strings.Contains(err.Error(), "no JSON array") {
		t.Errorf("error %q should mention 'no JSON array'", err.Error())
	}
}

func TestParseTopicsTooLarge(t *testing.T) {
	t.Parallel()
	big := strings.Repeat("x", 257*1024)
	_, err := runWithMockLLM(t, big, nil)
	if err == nil {
		t.Fatal("expected error for too-large response")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("error %q should mention 'too large'", err.Error())
	}
}

func TestParseTopicsInvalidJSON(t *testing.T) {
	t.Parallel()
	raw := `[{"title":broken json}]`
	_, err := runWithMockLLM(t, raw, nil)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Errorf("error %q should mention 'invalid JSON'", err.Error())
	}
}

// ====================
// Generate tests (5)
// ====================

func TestGeneratePerfect(t *testing.T) {
	t.Parallel()
	raw := `[{"title":"T1","angle":"A","expected_performance":"P","hook":"H"},
			{"title":"T2","angle":"A","expected_performance":"P","hook":"H"},
			{"title":"T3","angle":"A","expected_performance":"P","hook":"H"},
			{"title":"T4","angle":"A","expected_performance":"P","hook":"H"},
			{"title":"T5","angle":"A","expected_performance":"P","hook":"H"}]`
	res, err := runWithMockLLM(t, raw, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Topics) != 5 {
		t.Errorf("len(Topics) = %d, want 5", len(res.Topics))
	}
	if res.Cost.InputTokens != 100 || res.Cost.OutputTokens != 50 {
		t.Errorf("Cost = %+v, want {100, 50}", res.Cost)
	}
}

func TestGenerateTrimsToCount(t *testing.T) {
	t.Parallel()
	var rawBuilder strings.Builder
	rawBuilder.WriteString("[")
	for i := 0; i < 8; i++ {
		if i > 0 {
			rawBuilder.WriteString(",")
		}
		rawBuilder.WriteString(`{"title":"T` + fmtInt(i) + `","angle":"A","expected_performance":"P","hook":"H"}`)
	}
	rawBuilder.WriteString("]")
	res, err := runWithMockLLM(t, rawBuilder.String(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Topics) != 5 {
		t.Errorf("len(Topics) = %d, want 5 (library should trim)", len(res.Topics))
	}
}

func TestGenerateLLMError(t *testing.T) {
	t.Parallel()
	_, err := runWithMockLLM(t, "", &llmError{msg: "API rate limit"})
	if err == nil {
		t.Fatal("expected error from LLM call")
	}
	if !strings.Contains(err.Error(), "rate limit") {
		t.Errorf("error %q should mention 'rate limit'", err.Error())
	}
}

func TestGenerateParseError(t *testing.T) {
	t.Parallel()
	_, err := runWithMockLLM(t, "garbage", nil)
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestGenerateAcceptsFewerTopicsThanCount(t *testing.T) {
	t.Parallel()
	raw := `[{"title":"T1","angle":"A","expected_performance":"P","hook":"H"},
			{"title":"T2","angle":"A","expected_performance":"P","hook":"H"}]`
	res, err := runWithMockLLM(t, raw, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Topics) != 2 {
		t.Errorf("len(Topics) = %d, want 2 (LLM returned fewer, no error)", len(res.Topics))
	}
}

// ====================
// Test helpers
// ====================

// runWithMockLLM runs topic.Generate with a mocked LLM that
// returns raw text (or an error). Returns the result and any error.
func runWithMockLLM(t *testing.T, raw string, returnErr error) (*topic.Result, error) {
	t.Helper()
	text := agents.NewMiniMaxWithTextOverride(
		func(ctx context.Context, prompt string, opts agents.MiniMaxTextOptions) (*agents.MiniMaxTextResult, error) {
			if returnErr != nil {
				return nil, returnErr
			}
			return &agents.MiniMaxTextResult{
				Text:         raw,
				InputTokens:  100,
				OutputTokens: 50,
			}, nil
		},
	)
	return topic.Generate(context.Background(), text, topic.Input{
		Seed: "个人成长", Platform: "抖音", Count: 5,
	})
}

// llmError is a simple error type for TestGenerateLLMError.
type llmError struct{ msg string }

func (e *llmError) Error() string { return e.msg }

// fmtInt formats an int as a string (helper to avoid importing fmt).
func fmtInt(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}