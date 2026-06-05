package agents

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestDeconstructPromptEmbedsTranscriptAndMetadata verifies the
// prompt actually contains the transcript and metadata fields so a
// refactor that drops a placeholder is caught immediately.
func TestDeconstructPromptEmbedsTranscriptAndMetadata(t *testing.T) {
	a := NewClaude("test-key")
	prompt := a.DeconstructPrompt("你有没有想过——逍遥到底是什么", DeconstructMetadata{
		Platform:    "抖音",
		DurationSec: 75,
		Views:       1234567,
		Likes:       45678,
	})
	for _, want := range []string{
		"你有没有想过——逍遥到底是什么",
		"抖音",
		"75",
		"1234567",
		"45678",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("deconstruct prompt missing %q", want)
		}
	}
	// The prompt must ask for every top-level field of the wire
	// response. A drift between the prompt and the parser is the
	// most common cause of "AI returned output that could not be
	// parsed" 502s in production.
	for _, want := range []string{
		"hook", "structure", "cta", "emotional_arc",
		"reusable_patterns", "platform_fit_notes", "overall_score",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("deconstruct prompt missing field %q", want)
		}
	}
}

// TestDeconstructPromptMissingMetadataIsHandled verifies the prompt
// renders "(未提供)" placeholders instead of empty trailing lines
// when the caller supplies no metadata.
func TestDeconstructPromptMissingMetadataIsHandled(t *testing.T) {
	a := NewClaude("test-key")
	prompt := a.DeconstructPrompt("一段视频文本", DeconstructMetadata{})
	if !strings.Contains(prompt, "(未提供)") {
		t.Errorf("expected missing metadata to render as (未提供), prompt=\n%s", prompt)
	}
}

// TestViralFormulaPromptEmbedsTranscript verifies the formula
// prompt contains the transcript and asks for every field of the
// wire response.
func TestViralFormulaPromptEmbedsTranscript(t *testing.T) {
	a := NewClaude("test-key")
	prompt := a.ViralFormulaPrompt("熊猫翻开《庄子》:何为逍遥?")
	if !strings.Contains(prompt, "熊猫翻开《庄子》:何为逍遥?") {
		t.Errorf("viral-formula prompt missing transcript")
	}
	for _, want := range []string{
		"formula_name", "variables", "steps",
		"example_application", "variations",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("viral-formula prompt missing field %q", want)
		}
	}
}

// TestDemoDeconstructIsValidJSON ensures the canned deconstruction
// parses into the shape the handler expects. The handler's parser
// shares the same wire struct, so a regression here fails the demo
// loudly here instead of at request time.
func TestDemoDeconstructIsValidJSON(t *testing.T) {
	a := NewClaude("")
	raw, err := a.DemoResponse(DemoOpDeconstruct, 0)
	if err != nil {
		t.Fatalf("DemoResponse(deconstruct) err: %v", err)
	}
	// Use a structural local type to avoid coupling the agents
	// package to the handlers package's wire shape.
	var parsed struct {
		Hook struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			Strength int    `json:"strength"`
		} `json:"hook"`
		Structure struct {
			Pattern string `json:"pattern"`
			Beats   []struct {
				TimePct int    `json:"time_pct"`
				Role    string `json:"role"`
			} `json:"beats"`
			Density int `json:"density"`
		} `json:"structure"`
		CTA struct {
			Present   bool   `json:"present"`
			Placement string `json:"placement"`
		} `json:"cta"`
		EmotionalArc     []map[string]any  `json:"emotional_arc"`
		ReusablePatterns []string          `json:"reusable_patterns"`
		PlatformFitNotes map[string]string `json:"platform_fit_notes"`
		OverallScore     int               `json:"overall_score"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("canned deconstruct JSON does not parse: %v\n%s", err, raw)
	}
	if parsed.Hook.Type == "" || parsed.Hook.Text == "" {
		t.Errorf("hook block missing type/text: %+v", parsed.Hook)
	}
	if parsed.Hook.Strength < 1 || parsed.Hook.Strength > 100 {
		t.Errorf("hook strength out of range: %d", parsed.Hook.Strength)
	}
	if parsed.Structure.Pattern == "" || len(parsed.Structure.Beats) == 0 {
		t.Errorf("structure block incomplete: %+v", parsed.Structure)
	}
	if len(parsed.EmotionalArc) == 0 {
		t.Error("emotional_arc must be non-empty so the demo renders a curve")
	}
	if len(parsed.ReusablePatterns) < 3 {
		t.Errorf("reusable_patterns < 3 (got %d) — the whole point of the demo is to show patterns", len(parsed.ReusablePatterns))
	}
	// All three target platforms must have a note. A demo that
	// silently drops 哔哩哔哩 would be embarrassing live.
	for _, p := range []string{"抖音", "哔哩哔哩", "小红书"} {
		if parsed.PlatformFitNotes[p] == "" {
			t.Errorf("platform_fit_notes missing %q", p)
		}
	}
	if parsed.OverallScore < 1 || parsed.OverallScore > 100 {
		t.Errorf("overall_score out of range: %d", parsed.OverallScore)
	}
}

// TestDemoViralFormulaIsValidJSON ensures the canned viral-formula
// parses into the shape the handler expects.
func TestDemoViralFormulaIsValidJSON(t *testing.T) {
	a := NewClaude("")
	raw, err := a.DemoResponse(DemoOpViralFormula, 0)
	if err != nil {
		t.Fatalf("DemoResponse(viral_formula) err: %v", err)
	}
	var parsed struct {
		FormulaName string `json:"formula_name"`
		Variables   []struct {
			Name    string `json:"name"`
			Example string `json:"example"`
			Weight  string `json:"weight"`
		} `json:"variables"`
		Steps              []string `json:"steps"`
		ExampleApplication string   `json:"example_application"`
		Variations         []string `json:"variations"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("canned viral-formula JSON does not parse: %v\n%s", err, raw)
	}
	if parsed.FormulaName == "" {
		t.Error("formula_name must be non-empty")
	}
	if len(parsed.Variables) == 0 {
		t.Error("variables must be non-empty so the demo shows the formula's parameters")
	}
	for i, v := range parsed.Variables {
		if v.Name == "" || v.Example == "" {
			t.Errorf("variables[%d] missing name/example: %+v", i, v)
		}
		switch v.Weight {
		case "low", "medium", "high":
			// ok
		default:
			t.Errorf("variables[%d].weight = %q, want low/medium/high", i, v.Weight)
		}
	}
	if len(parsed.Steps) < 2 {
		t.Errorf("steps < 2 (got %d) — a formula needs a recipe", len(parsed.Steps))
	}
	if parsed.ExampleApplication == "" {
		t.Error("example_application must be non-empty — it's how the user knows the formula is reusable")
	}
	if n := len(parsed.Variations); n < 3 || n > 5 {
		t.Errorf("variations count = %d, spec requires 3-5", n)
	}
}

// TestDemoResponseDeconstructAndFormulaSeparate verifies the two new
// ops do not share storage — a bug where DemoOpViralFormula returned
// the deconstruct JSON would silently 502 the second endpoint, and
// the failure would be hard to spot in production.
func TestDemoResponseDeconstructAndFormulaSeparate(t *testing.T) {
	a := NewClaude("")
	d, _ := a.DemoResponse(DemoOpDeconstruct, 0)
	f, _ := a.DemoResponse(DemoOpViralFormula, 0)
	if d == f {
		t.Error("deconstruct and viral-formula demos must be distinct strings")
	}
	if !strings.Contains(d, "hook") {
		t.Errorf("deconstruct demo missing 'hook': %s", d)
	}
	if !strings.Contains(f, "formula_name") {
		t.Errorf("viral-formula demo missing 'formula_name': %s", f)
	}
}

// TestFallbackHelpers exercises the small helpers used by the
// deconstruct prompt formatter. Trivial but worth pinning so a
// future refactor that swaps "(未提供)" for "?" doesn't silently
// change every prompt the operator sees.
func TestFallbackHelpers(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"empty", "", "(未提供)"},
		{"whitespace", "   ", "(未提供)"},
		{"value", "抖音", "抖音"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := fallback(tc.in); got != tc.want {
				t.Errorf("fallback(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	intCases := []struct {
		name string
		in   int
		want string
	}{
		{"zero", 0, "(未提供)"},
		{"negative", -1, "(未提供)"},
		{"positive", 1234567, "1234567"},
	}
	for _, tc := range intCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := fallbackInt(tc.in); got != tc.want {
				t.Errorf("fallbackInt(%d) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
