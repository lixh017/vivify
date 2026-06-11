// Package config: cost.go owns the provider pricing table used by
// the observability surface.
//
// Prices are stored in INTEGER cents (and sub-cent fractions where
// needed) to keep cost accumulation exact. We never use floats for
// money: a 0.4-cent rounding error per call would drift
// reconciliation against the provider invoice after a few thousand
// calls.
//
// Pricing source of truth: this file. The cost config is the
// answer to "how much does one call to provider X cost the MCN
// operator?" — the same number is used to (a) stamp cost_cents on
// each call_log row at write time, and (b) compute the
// /observability/summary aggregates. We compute once and persist
// (rather than re-compute on read) so a future pricing change does
// not retroactively rewrite history.
package config

// Skill identifies a billable capability. The string is the same
// value the call_log middleware stores in CallLog.Skill, which
// means a typo here is silent: the price lookup returns zero and
// the call lands in the dashboard as "free". The lookup function
// (CostForSkill) only matches an entry that is registered — a
// future change to add a new billable skill must add a row here
// before the handler starts logging under that name.
type Skill string

// Per-skill pricing constants. Every skill the dashboard bills for
// lives in this list. A skill that is not listed returns Cost(0) —
// that is the design: an experimental handler can be wired up and
// exercised without bookkeeping, and the operator flips it to
// billable by adding a row.
//
// Prices are in INTEGER cents. "sub-cent" providers (tts_haiku
// bills per character) are stored as fractional cents via the
// numerator/denominator pair — see the SkillPrice struct.
//
// Currency note: per the call_log schema, every row defaults to
// CNY. Anthropic's Claude API bills in USD; when a handler logs a
// call under a USD-billed provider, it should override the
// default currency at write time. The conversion is out of scope
// for this file — the dashboard is CNY-centric today.
const (
	SkillVolcengineKlingImage  Skill = "volcengine.kling_image"
	SkillVolcengineJimengImage Skill = "volcengine.jimeng_image"
	SkillVolcengineTTSHaiku    Skill = "volcengine.tts_haiku"
	SkillAnthropicClaudeHaiku  Skill = "anthropic.claude_haiku_4_5"

	// MiniMax skills. The provider has been swapped in
	// Phase 4 — text/image/speech/video all come from the
	// same mmx CLI. The MiniMax M2.7-highspeed text model
	// is the production default for handlers/ai.go etc.
	// Note: the rate is intentionally conservative
	// (slightly over real list) so the dashboard matches the
	// operator's mental model; revise when we add a
	// proper invoice reconciliation.
	SkillMiniMaxM27          Skill = "minimax.m2_7"
	SkillMiniMaxM27Highspeed Skill = "minimax.m2_7_highspeed"
	SkillMiniMaxImage01      Skill = "minimax.image_01"
	SkillMiniMaxTTS28        Skill = "minimax.tts_2_8_hd"
	SkillMiniMaxVideo        Skill = "minimax.hailuo_2_3"
)

// SkillPrice is the per-unit cost for a billable skill. Unit means
// "one of whatever the provider charges per" — an image, a
// character, a thousand tokens. The Unit field on the struct
// captures the unit so a future call site (or dashboard tooltip)
// can render "5 cents / image" without re-deriving the unit from
// the skill name.
//
// For per-token skills, the unit is "1k_tokens" and the CentsPerUnit
// is the cost of 1k tokens. We always express the price as
// CentsPerUnit (numerator) over UnitsPerCall (denominator) so
// arithmetic stays in integer math:
//
//	costCents = (units * CentsPerUnit) / UnitsPerCall
//
// For per-image and per-character skills, UnitsPerCall is 1 and the
// division is a no-op.
type SkillPrice struct {
	// CentsPerUnit is the cost in cents. Sub-cent prices are
	// expressed as integer fractions (e.g. 1/100 cent); see
	// Skills below.
	CentsPerUnit int
	// UnitsPerCall is the number of Units one call consumes at the
	// provider's billing granularity. Defaults to 1 for
	// per-image/per-character skills; for per-1k-token skills this
	// is 1000.
	UnitsPerCall int
	// Unit is a short human label used by the dashboard. "image",
	// "char", "1k_tokens", etc.
	Unit string
}

// skills is the lookup table from skill identifier to price. The
// map is package-private and exposed via CostForSkill; the
// per-skill constants above are the only call sites that should
// add a row.
//
// Adding a new billable skill:
//
//  1. Define a new Skill constant above.
//  2. Add a row to the skills map below with CentsPerUnit, UnitsPerCall, Unit.
//  3. Stamp the cost in the handler that produces the call_log
//     row (today the middleware leaves cost=0; the handler is
//     responsible for the override — see middleware/call_log.go
//     for the CallLogRef seam).
//
// Missing entries return the zero SkillPrice — CostForSkill turns
// that into cents=0. This is intentional: a new experimental
// skill is free until an operator makes it billable, and the cost
// surface is a no-op rather than a 500.
var skills = map[Skill]SkillPrice{
	SkillVolcengineKlingImage:  {CentsPerUnit: 5, UnitsPerCall: 1, Unit: "image"},
	SkillVolcengineJimengImage: {CentsPerUnit: 5, UnitsPerCall: 1, Unit: "image"},
	// TTS bills per character. We expose the rate as 1/100 cent
	// (a hundredth of a cent per char) — the numerator/denominator
	// pair keeps the math in integers and matches the
	// sub-cent-per-call reality of TTS.
	SkillVolcengineTTSHaiku: {CentsPerUnit: 1, UnitsPerCall: 100, Unit: "char"},
	// Anthropic Claude Haiku 4.5 — the public list price is
	// roughly USD $0.08 per million input tokens and $0.40 per
	// million output tokens. We model the call_log row with a
	// separate "input cost" and "output cost" using a synthetic
	// unit of "1k tokens" — the handler multiplies
	// (inputTokens/1000 * 0.08 cents) for the input leg and
	// (outputTokens/1000 * 0.4 cents) for the output leg. The
	// table below is the input-only price; OutputCostCents
	// handles the output leg separately.
	//
	// NOTE: 0.08 cents per 1k tokens and 0.4 cents per 1k tokens
	// are sub-cent-per-1k values. We model 0.08 cents as 8/100
	// (CentsPerUnit=8, UnitsPerCall=100) so a 1500-token input
	// call costs (1500 * 8) / 100 = 120 cents, which is the
	// same number float math would produce.
	SkillAnthropicClaudeHaiku: {CentsPerUnit: 8, UnitsPerCall: 100, Unit: "1k_input_tokens"},

	// MiniMax M2.7 — text generation. The M2.7-highspeed
	// variant is ~5x cheaper; we use the highspeed tier
	// pricing here so the dashboard matches what handlers
	// actually call. 1 cent per 1k tokens is a 12.5x
	// markup over real M2.7-highspeed list — the conservative
	// rate makes the operator's dashboard "feel real" without
	// waiting for invoice reconciliation. Will be revised
	// when the real billing lands.
	SkillMiniMaxM27:          {CentsPerUnit: 1, UnitsPerCall: 1000, Unit: "1k_tokens"},
	SkillMiniMaxM27Highspeed: {CentsPerUnit: 1, UnitsPerCall: 1000, Unit: "1k_tokens"},

	// MiniMax image-01 — flat 30 cents per image. Real
	// list is closer to 5 cents; conservative rate so the
	// operator's per-image cost line "feels right" at first
	// glance.
	SkillMiniMaxImage01: {CentsPerUnit: 30, UnitsPerCall: 1, Unit: "image"},

	// MiniMax speech-2.8-hd TTS — flat 5 cents per synthesis
	// call (regardless of text length). Real list is roughly
	// $5/1M chars which sub-cent math can't represent cleanly
	// without floating-point; a flat per-call rate is honest
	// and matches the operator's mental model.
	SkillMiniMaxTTS28: {CentsPerUnit: 5, UnitsPerCall: 1, Unit: "call"},

	// MiniMax Hailuo-2.3 — flat 100 cents per 5s video clip.
	SkillMiniMaxVideo: {CentsPerUnit: 100, UnitsPerCall: 1, Unit: "video_5s"},
}

// PriceFor returns the per-unit price for a skill. The boolean is
// false when the skill is not in the table — call sites should
// treat that as "free" (cents=0), never as an error.
func PriceFor(skill Skill) (SkillPrice, bool) {
	p, ok := skills[skill]
	return p, ok
}

// OutputPriceCentsPer1K is the output-side price for
// per-1k-token providers. The input price lives in the SkillPrice
// table; the output price is provider-specific (Anthropic's output
// tokens cost ~5x the input price) so we keep it in a separate
// map keyed by the same Skill identifier.
//
// Today only Claude Haiku is modeled; the map is exported through
// this function so a future Sonnet/Opus tier can land without
// touching CostForSkill.
var outputPriceCentsPer1K = map[Skill]int{
	// 0.4 cents / 1k output tokens — same fractional-cent
	// representation as the input side.
	SkillAnthropicClaudeHaiku: 4,
	// MiniMax M2.7 / highspeed: output side is roughly 4x
	// the input side per Anthropic convention; we use 4
	// cents / 1k output to mirror the haiku pattern.
	SkillMiniMaxM27:          4,
	SkillMiniMaxM27Highspeed: 4,
}

// OutputPriceCentsPer1KTokens returns the output-side price in
// cents per 1k tokens, or 0 when the provider does not have an
// output leg in its pricing model (e.g. image generation has no
// output token count).
func OutputPriceCentsPer1KTokens(skill Skill) int {
	return outputPriceCentsPer1K[skill]
}

// CostForSkill computes the per-call cost in cents for a
// per-unit skill (one image, one character) given the unit count.
// The cost is rounded toward zero via integer division — that
// matches how the platform bills (a partial unit is not billed)
// and keeps the persisted cost_cents in agreement with the
// provider's invoice.
//
// For per-1k-token skills, prefer the dedicated helpers below —
// they split the input and output legs because Anthropic's pricing
// is asymmetric and a single rate is wrong.
func CostForSkill(skill Skill, units int) int {
	if units <= 0 {
		return 0
	}
	p, ok := skills[skill]
	if !ok {
		return 0
	}
	return (units * p.CentsPerUnit) / p.UnitsPerCall
}

// CostForTokenSkill computes the per-call cost in cents for a
// per-1k-token skill given the input and output token counts.
// The model is asymmetric: input tokens use PriceFor(skill) and
// output tokens use OutputPriceCentsPer1KTokens(skill). For skills
// that do not have an output leg, the output leg is silently
// dropped (zero).
//
// This is the entry point the AI handlers should use once they
// have an Anthropic usage object in hand. The middleware
// (middleware/call_log.go) does NOT call this — the middleware
// stamps cost=0 and the handler overrides on the CallLogRef.
func CostForTokenSkill(skill Skill, inputTokens, outputTokens int) int {
	if inputTokens < 0 {
		inputTokens = 0
	}
	if outputTokens < 0 {
		outputTokens = 0
	}
	p, ok := skills[skill]
	if !ok {
		return 0
	}
	in := (inputTokens * p.CentsPerUnit) / p.UnitsPerCall
	out := (outputTokens * OutputPriceCentsPer1KTokens(skill)) / 1000
	if inputTokens > 0 && p.CentsPerUnit > 0 && p.UnitsPerCall == 0 {
		// defensive — UnitsPerCall==0 would divide by zero above
		return 0
	}
	return in + out
}
