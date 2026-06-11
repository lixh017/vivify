package agents

import "time"

// Usage is the per-call token accounting returned by an
// upstream text provider. Surfaced so handlers can stamp it
// into call_log for cost attribution.
//
// Both fields are 0 if the upstream response omits Usage
// (e.g. older SDK versions, certain proxy errors). Handlers
// should treat zero as "unknown" rather than as a real count.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Sum returns the total token count (input + output).
// Convenience for the cost-attribution seam; not used
// elsewhere.
func (u Usage) Sum() int { return u.InputTokens + u.OutputTokens }

// CompleteOptions tunes a single text completion call. Zero
// values fall through to the provider's defaults. The
// post-Phase-4 handlers do not need most of these (the
// MiniMax native shape is MiniMaxTextOptions) but the
// Provider interface and the legacy routers still consume
// it, so it stays as the vendor-neutral transport-level
// option type.
type CompleteOptions struct {
	Model     string
	MaxTokens int64
	Timeout   time.Duration
}
