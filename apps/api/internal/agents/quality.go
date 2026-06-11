package agents

// Suggestion category values used in the QualitySuggestion JSON
// contract. The MiniMax prompt's worked example references these
// constants so a rename in the wire shape (or a typo in the
// prompt) breaks compilation rather than silently degrading
// scoring. Keep this list in sync with the values the live
// /ai/score endpoint, the rule-based fallback, and the demo pool
// emit.
const (
	SuggestionCategoryHook      = "hook"
	SuggestionCategoryStructure = "structure"
	SuggestionCategoryPlatform  = "platform_fit"
	SuggestionCategoryWordCount = "word_count"
	SuggestionCategoryCTA       = "cta"
)

// Suggestion severity values used in the QualitySuggestion JSON
// contract. Same rationale as SuggestionCategory*: code-level
// constants so a typo in the prompt is a compile error.
const (
	SuggestionSeverityLow    = "low"
	SuggestionSeverityMedium = "medium"
	SuggestionSeverityHigh   = "high"
)
