package config

import "testing"

// TestCostForSkill_PerImageSkills verifies the per-image billing
// math: a single image call costs 5 cents, three image calls cost
// 15 cents, fractional inputs (e.g. 0 images) cost 0.
//
// We do not test float arithmetic — the entire point of the
// cents/integer model is that 5 cents * 3 is exactly 15, not
// 14.999... .
func TestCostForSkill_PerImageSkills(t *testing.T) {
	cases := []struct {
		name  string
		skill Skill
		units int
		want  int
	}{
		{"kling one image", SkillVolcengineKlingImage, 1, 5},
		{"kling three images", SkillVolcengineKlingImage, 3, 15},
		{"jimeng one image", SkillVolcengineJimengImage, 1, 5},
		{"jimeng zero images (defensive)", SkillVolcengineJimengImage, 0, 0},
		{"kling negative (defensive)", SkillVolcengineKlingImage, -1, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CostForSkill(tc.skill, tc.units)
			if got != tc.want {
				t.Errorf("CostForSkill(%q, %d) = %d, want %d", tc.skill, tc.units, got, tc.want)
			}
		})
	}
}

// TestCostForSkill_TTSSubCent verifies the TTS rate stays exact
// at sub-cent granularity. 100 characters cost 1 cent (1/100 cent
// per char), 350 characters cost 3 cents (integer division
// rounds down — the platform does not bill a partial cent).
//
// This is the test that justifies the "integer math, no floats"
// decision: a float implementation would round 350 * 0.01 to
// 3.499... and either round up to 3 or 4 depending on language
// rounding rules. The integer version is exactly 3.
func TestCostForSkill_TTSSubCent(t *testing.T) {
	cases := []struct {
		chars int
		want  int
	}{
		{1, 0},   // 1 char * 1/100 = 0.01 cents, integer floor is 0
		{100, 1}, // 100 chars * 1/100 = exactly 1 cent
		{199, 1}, // 199/100 floor = 1
		{200, 2}, // 200/100 = 2
		{350, 3}, // 350/100 = 3
		{10000, 100},
	}
	for _, tc := range cases {
		got := CostForSkill(SkillVolcengineTTSHaiku, tc.chars)
		if got != tc.want {
			t.Errorf("CostForSkill(tts_haiku, %d) = %d, want %d", tc.chars, got, tc.want)
		}
	}
}

// TestCostForSkill_UnknownSkillIsFree verifies the contract: a
// skill that is not in the price table costs 0 cents. This is
// what lets the operator experiment with a new skill without
// booking a 500 on every dashboard load.
func TestCostForSkill_UnknownSkillIsFree(t *testing.T) {
	got := CostForSkill("nonexistent.skill", 100)
	if got != 0 {
		t.Errorf("unknown skill should be free, got %d", got)
	}
}

// TestCostForTokenSkill_Asymmetric verifies the per-1k-token
// pricing model. Claude Haiku bills input at 0.08 cents / 1k
// and output at 0.4 cents / 1k; the cost for 1500 input + 500
// output tokens is:
//
//	input:  (1500 * 8) / 100   = 120 cents
//	output: (500  * 4) / 1000  =   2 cents
//	total:                    = 122 cents
//
// The integer form of the rates is 8/100 (input) and 4/1000
// (output). The denominator split is deliberate: input is billed
// per 100 tokens in the source data, output is billed per 1000
// tokens; using a single denominator would be wrong on one side.
func TestCostForTokenSkill_Asymmetric(t *testing.T) {
	cases := []struct {
		name    string
		skill   Skill
		inToks  int
		outToks int
		want    int
	}{
		{
			name:    "1k input + 1k output",
			skill:   SkillAnthropicClaudeHaiku,
			inToks:  1000,
			outToks: 1000,
			// (1000 * 8)/100 + (1000 * 4)/1000 = 80 + 4 = 84
			want: 84,
		},
		{
			name:    "1500 input + 500 output",
			skill:   SkillAnthropicClaudeHaiku,
			inToks:  1500,
			outToks: 500,
			// (1500 * 8)/100 + (500 * 4)/1000 = 120 + 2 = 122
			want: 122,
		},
		{
			name:    "zero tokens",
			skill:   SkillAnthropicClaudeHaiku,
			inToks:  0,
			outToks: 0,
			want:    0,
		},
		{
			name:    "negative tokens clamp to zero",
			skill:   SkillAnthropicClaudeHaiku,
			inToks:  -100,
			outToks: -100,
			want:    0,
		},
		{
			name:    "unknown token skill is free",
			skill:   Skill("openai.gpt4o"),
			inToks:  5000,
			outToks: 1000,
			want:    0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CostForTokenSkill(tc.skill, tc.inToks, tc.outToks)
			if got != tc.want {
				t.Errorf("CostForTokenSkill(%q, in=%d, out=%d) = %d, want %d",
					tc.skill, tc.inToks, tc.outToks, got, tc.want)
			}
		})
	}
}

// TestOutputPriceCentsPer1KTokens_TableDriven guards the per-1k
// output price map. A future addition (Sonnet, Opus) only has to
// add a row and the test will catch a typo.
func TestOutputPriceCentsPer1KTokens_TableDriven(t *testing.T) {
	cases := []struct {
		skill Skill
		want  int
	}{
		{SkillAnthropicClaudeHaiku, 4},
		{Skill("anthropic.sonnet_4_5"), 0}, // unknown: free
		{SkillVolcengineKlingImage, 0},     // image: no output leg
	}
	for _, tc := range cases {
		got := OutputPriceCentsPer1KTokens(tc.skill)
		if got != tc.want {
			t.Errorf("OutputPriceCentsPer1KTokens(%q) = %d, want %d", tc.skill, got, tc.want)
		}
	}
}

// TestPriceFor_KnownAndUnknown verifies the boolean return is the
// right "table contains this skill" signal. A future caller
// (e.g. a pricing audit tool) can use PriceFor to surface "this
// skill is not billable" without re-deriving from the map keys.
func TestPriceFor_KnownAndUnknown(t *testing.T) {
	p, ok := PriceFor(SkillAnthropicClaudeHaiku)
	if !ok {
		t.Fatal("expected PriceFor(anthropic.claude_haiku_4_5) to be known")
	}
	if p.Unit != "1k_input_tokens" {
		t.Errorf("Unit = %q, want %q", p.Unit, "1k_input_tokens")
	}
	if p.CentsPerUnit != 8 {
		t.Errorf("CentsPerUnit = %d, want 8", p.CentsPerUnit)
	}
	if p.UnitsPerCall != 100 {
		t.Errorf("UnitsPerCall = %d, want 100", p.UnitsPerCall)
	}

	if _, ok := PriceFor(Skill("does.not.exist")); ok {
		t.Error("expected unknown skill to return ok=false")
	}
}
