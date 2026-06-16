// Package topic implements the topic generation capability.
//
// Generate is the library entry point. It is reused by:
//   - internal/handlers/ai.go:GenerateTopics (HTTP POST /api/ai/topics)
//   - cmd/opc-asset/main.go:runTopic (CLI subcommand)
//
// The library is pure: no HTTP context, no middleware, no IO
// besides the LLM call (which is provided by the caller via
// agents.TextProvider). The HTTP handler adds demo-mode + call_log
// cost stamping on top; the CLI adds file output + stderr cost
// summary on top.
package topic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/opc/api/internal/agents"
)

// Topic is the canonical shape of one generated topic.
// Field names match the existing HTTP wire shape (snake_case
// JSON tags) so library + HTTP API stay in sync.
type Topic struct {
	Title               string   `json:"title"`
	Angle               string   `json:"angle"`
	ExpectedPerformance string   `json:"expected_performance"`
	Hook                string   `json:"hook"`
	Pattern             string   `json:"pattern,omitempty"`
	VoiceTags           []string `json:"voice_tags,omitempty"`
}

// Input captures the variables the operator can tweak.
type Input struct {
	Seed     string
	Platform string
	Count    int
}

// Result is what Generate returns: parsed topics + cost data
// (so callers can stamp it to call_log or write to a CLI ledger)
// + the prompt that was sent to the model.
//
// Prompt is added in Sub-Spec E M1 as an additive field. The
// HTTP handler does NOT consume it today — it builds the prompt
// locally via agents.GenerateTopicsPrompt so the same code path
// can run before the demo short-circuit (which doesn't call
// topic.Generate at all). The field is exposed for:
//
//   - M2's planned output-side anti-AI check (Sub-Spec E M2:
//     check res.Text, the model's response, against the prompt
//     for tighter grounding).
//   - Operator observability: a future "show the prompt the
//     model saw" panel can read res.Prompt without re-running
//     the LLM.
//   - Test coverage: pins the invariant that the library
//     exposes what it builds (see TestGenerateReturnsPromptUsed).
//
// The library has always built the prompt internally; this just
// exposes it. Additive change — existing callers that only
// read Topics or Cost are unaffected.
type Result struct {
	Topics []Topic
	Cost   CostInfo
	Prompt string // exact prompt sent to the LLM (Sub-Spec E M1)
}

// CostInfo carries the per-call token counts. Callers compute
// the actual cost in CNY via config.CostForTokenSkill (HTTP
// handler uses call_log middleware; CLI prints to stderr in M1).
type CostInfo struct {
	InputTokens  int
	OutputTokens int
}

// maxAIResultBytes is the upper bound on raw LLM response size.
// A single 20-topic batch is well under 256KB in practice; this
// guard prevents a runaway model from blowing up memory.
const maxAIResultBytes = 256 * 1024

// maxCount hard-caps how many topics one call may request. Keeps
// token usage bounded; a "give me 500" request would burn budget.
const maxCount = 20

// Generate is the library entry point. It validates input,
// builds the prompt, calls the LLM, parses the JSON array
// response, and returns topics + token counts.
//
// text is the Phase-4 TextProvider interface; both *agents.MiniMax
// and the resolver-managed AnthropicCompat / OpenAICompat
// providers satisfy it. The library uses the legacy MiniMax
// Text(...) entry point so the existing test seam (which mocks
// via NewMiniMaxWithTextOverride) keeps working unchanged.
func Generate(ctx context.Context, text agents.TextProvider, input Input) (*Result, error) {
	if err := validate(input); err != nil {
		return nil, err
	}
	if text == nil {
		return nil, errors.New("topic: text provider is nil")
	}
	prompt := agents.GenerateTopicsPrompt(input.Seed, input.Platform, input.Count)
	// CompleteWithUsage is the provider-neutral entry point on
	// TextProvider. *agents.MiniMax implements it (it falls
	// through to Text internally), and so do the AnthropicCompat /
	// OpenAICompat providers. Model is left empty so the
	// configured credential's ModelName (or the provider's own
	// default) is used.
	body, usage, err := text.CompleteWithUsage(ctx, prompt, agents.CompleteOptions{})
	if err != nil {
		return nil, fmt.Errorf("topic: LLM call failed: %w", err)
	}
	topics, err := parseTopics(body)
	if err != nil {
		return nil, fmt.Errorf("topic: parse failed: %w", err)
	}
	if len(topics) > input.Count {
		topics = topics[:input.Count]
	}
	return &Result{
		Topics: topics,
		Cost:   CostInfo{InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens},
		Prompt: prompt,
	}, nil
}

// validate enforces the 4 input rules documented in spec §11.
// TrimSpace is applied so that whitespace-only seed/platform
// fail the empty check (avoids "    " slipping through).
func validate(input Input) error {
	if strings.TrimSpace(input.Seed) == "" {
		return errors.New("seed is required")
	}
	if strings.TrimSpace(input.Platform) == "" {
		return errors.New("platform is required")
	}
	if input.Count <= 0 {
		return errors.New("count must be > 0")
	}
	if input.Count > maxCount {
		return fmt.Errorf("count must be <= %d", maxCount)
	}
	return nil
}

// jsonArrayRE finds the first '[' so we know where the array starts.
// The matching closing ']' is found by bracket-balancing, since a
// non-greedy regex like `\[.*?\]` truncates at the first inner ']'
// (e.g. inside `"voice_tags":["a","b"]`) which is wrong.
var jsonArrayRE = regexp.MustCompile(`\[`)

// extractJSONArray pulls the first balanced top-level JSON array
// from s. It returns "" if no balanced array exists.
func extractJSONArray(s string) string {
	start := jsonArrayRE.FindStringIndex(s)
	if start == nil {
		return ""
	}
	depth := 0
	inStr := false
	escape := false
	for i := start[0]; i < len(s); i++ {
		c := s[i]
		if escape {
			escape = false
			continue
		}
		if inStr {
			if c == '\\' {
				escape = true
				continue
			}
			if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return s[start[0] : i+1]
			}
		}
	}
	return ""
}

// parseTopics extracts []Topic from raw LLM output. Lenient about
// markdown fences because LLM often wraps JSON in ```json ... ```
// blocks despite explicit instructions.
func parseTopics(raw string) ([]Topic, error) {
	if len(raw) > maxAIResultBytes {
		return nil, errors.New("response too large")
	}
	cleaned := strings.TrimSpace(raw)
	// Strip a single ```json ... ``` wrapper if present.
	if strings.HasPrefix(cleaned, "```") {
		if i := strings.Index(cleaned, "\n"); i >= 0 {
			cleaned = cleaned[i+1:]
		}
		if strings.HasSuffix(cleaned, "```") {
			cleaned = cleaned[:len(cleaned)-3]
		}
		cleaned = strings.TrimSpace(cleaned)
	}
	match := extractJSONArray(cleaned)
	if match == "" {
		return nil, errors.New("no JSON array found in response")
	}
	var topics []Topic
	if err := json.Unmarshal([]byte(match), &topics); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return topics, nil
}
