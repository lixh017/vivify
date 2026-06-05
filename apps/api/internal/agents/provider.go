package agents

import (
	"context"
	"errors"
	"time"
)

// Provider is the small, stable surface every AI backend must
// implement. Keeping it tiny (3 methods) means new providers
// (DeepSeek, Gemini, Qwen, …) can ship without leaking
// vendor-specific concepts into handlers.
//
// Design notes
//   - Complete is a single user-turn completion. Multi-turn
//     conversations are not in scope for Phase 1; the OPC agents
//     are stateless prompt-in / text-out by design.
//   - Name() is the short canonical identifier ("claude" /
//     "deepseek" / "gemini") used by the router to map task → model
//     and by the metrics middleware to label per-provider counters.
//   - Available() reports whether the provider has the credentials
//     it needs to make a real API call. The router falls back to
//     demo data when no available provider can serve a task,
//     preserving the dev-mode contract.
type Provider interface {
	// Complete sends a single user-turn prompt and returns the
	// first text content block. Implementations MUST respect the
	// context deadline and the per-call CompleteOptions.
	Complete(ctx context.Context, prompt string, opts CompleteOptions) (string, error)
	// Name returns the canonical short identifier of the provider.
	// Used by the router and metrics layer; must be stable across
	// releases.
	Name() string
	// Available reports whether the provider has the credentials
	// it needs (API key, etc.) to actually serve a request.
	// Returning false routes the request to a fallback path
	// (demo data, alternate provider) without an error.
	Available() bool
}

// Canonical provider names. Defined as constants so the router and
// tests cannot disagree on the spelling (e.g. "Claude" vs "claude").
const (
	ProviderClaude   = "claude"
	ProviderDeepSeek = "deepseek"
	ProviderGemini   = "gemini"
)

// Canonical model identifiers used by the router and the default
// per-task routing table. Defined here (the vendor-neutral part
// of the package) so router.go and its tests do not need to
// import the Anthropic SDK purely to spell a model name. The
// Claude-specific providers still cast to anthropic.Model at the
// SDK call site.
//
// Kept in lockstep with the values the Anthropic SDK exposes
// under anthropic.ModelClaudeSonnet4_5 etc. If the upstream
// SDK renames a model, update these constants and the cast in
// claude.go / claude_provider.go together.
const (
	ModelClaudeSonnet4_5 = "claude-sonnet-4-5"
	ModelClaudeHaiku4_5  = "claude-haiku-4-5"
	ModelClaudeOpus4_5   = "claude-opus-4-5"
)

// ErrProviderUnavailable is returned by Complete when the provider
// was constructed without the credentials it needs (empty API key)
// AND the caller did not opt into a fallback. Handlers should use
// errors.Is to detect this case rather than sniffing err.Error().
//
// This is the cross-provider analogue of ErrNoAPIKey, which remains
// the Claude-specific sentinel for backwards compatibility with
// existing handlers that branch on it.
var ErrProviderUnavailable = errors.New("provider not configured (missing API key)")

// DefaultProviderTimeout is the per-call timeout applied when the
// caller does not supply one via CompleteOptions. Matches the
// existing Claude default so behavior is unchanged for the migration.
const DefaultProviderTimeout = 60 * time.Second
