package agents

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Provider is the legacy interface (Phase 1). It returns just
// (string, error) and is satisfied by the original DeepSeek /
// Gemini / Claude-wrapper providers the router was built
// around. New code should depend on TextProvider (which
// returns usage for cost attribution) — Provider is kept only
// so the existing router and call sites keep compiling
// through the Phase 4 refactor.
type Provider interface {
	Complete(ctx context.Context, prompt string, opts CompleteOptions) (string, error)
	Name() string
	Available() bool
}

// Canonical provider names. Defined as constants so the router
// and tests cannot disagree on the spelling (e.g. "Claude"
// vs "claude"). The MiniMax / Anthropic / OpenAI / etc.
// surface uses the same vocabulary — see TextProvider.Name
// for the new string set.
const (
	ProviderClaude    = "claude"
	ProviderDeepSeek  = "deepseek"
	ProviderGemini    = "gemini"
	ProviderMiniMax   = "minimax"
	ProviderAnthropic = "anthropic"
	ProviderOpenAI    = "openai"
	ProviderDoubao    = "doubao"
	ProviderQwen      = "qwen"
	ProviderEcho      = "echo"
)

// Canonical model identifiers used by the router and the
// default per-task routing table. Kept in lockstep with
// the Anthropic SDK names; if the upstream SDK renames a
// model, update these constants and the cast in
// claude_provider.go together.
const (
	ModelClaudeSonnet4_5 = "claude-sonnet-4-5"
	ModelClaudeHaiku4_5  = "claude-haiku-4-5"
	ModelClaudeOpus4_5   = "claude-opus-4-5"
	// MiniMax M2.7 + highspeed variant. Both hit the same
	// Anthropic-compatible endpoint; the "highspeed" tier
	// is cheaper per Anthropic-side config (the actual
	// MiniMax billing model is the user's Token Plan
	// subscription, not per-token).
	ModelMiniMaxM27         = "MiniMax-M2.7"
	ModelMiniMaxM27Highspeed = "MiniMax-M2.7-highspeed"
)

// ErrProviderUnavailable is returned by Complete when the
// provider was constructed without the credentials it needs
// AND the caller did not opt into a fallback. Handlers
// should use errors.Is to detect this case rather than
// sniffing err.Error().
var ErrProviderUnavailable = errors.New("provider not configured (missing API key)")

// DefaultProviderTimeout is the per-call timeout applied
// when the caller does not supply one via CompleteOptions.
const DefaultProviderTimeout = 60 * time.Second

// TextProvider is the new (Phase 4) contract for
// text-generation providers. It extends the legacy
// Provider interface with CompleteWithUsage, which returns
// the per-call token usage so handlers can stamp cost onto
// call_log rows without trusting a fixed per-skill price.
//
// All major text-model providers (Anthropic Claude, OpenAI
// GPT, MiniMax M2.7, Doubao, Qwen, etc.) accept roughly
// the same shape — system + messages + max_tokens +
// temperature — and return a text result + usage. The
// provider-specific HTTP wire is hidden behind this
// interface so a handler can swap providers via
// configuration without changing the handler's code.
//
// A new concrete provider (e.g. OpenAI) implements this
// interface and is registered with the ProviderRegistry at
// boot. No handler change required.
type TextProvider interface {
	// Name returns the short, stable identifier the registry
	// matches against. "anthropic" / "minimax" / "openai" /
	// "echo" — lower-case, no whitespace. The X-Text-Provider
	// header value matches this exactly.
	Name() string

	// Available reports whether the provider is configured to
	// make real API calls. Handlers use this to fall back to
	// demo mode when no API key is set.
	Available() bool

	// CompleteWithUsage is the cost-attribution-aware variant.
	// Returns the generated text plus the per-call token
	// usage (so the call_log middleware can stamp cost).
	CompleteWithUsage(ctx context.Context, prompt string, opts CompleteOptions) (string, Usage, error)
}

// MediaProvider is the contract for non-text outputs:
// image generation, speech synthesis, video rendering. One
// provider typically supports a subset of {image, speech,
// video} — providers that don't support a given operation
// should return ErrMediaNotSupported from the matching
// method so the /sandbox and /docs surfaces can show
// "this provider can't do X" rather than a generic 500.
type MediaProvider interface {
	Name() string
	Available() bool
	Image(ctx context.Context, prompt string, opts MiniMaxImageOptions) (*MiniMaxImageResult, error)
	Speech(ctx context.Context, text string, opts MiniMaxSpeechOptions) (*MiniMaxSpeechResult, error)
	Video(ctx context.Context, prompt string, opts MiniMaxVideoOptions) (*MiniMaxVideoResult, error)
}

// VLProvider extends TextProvider for vision-language models
// (Anthropic Claude with image input, GPT-4V, MiniMax with
// vision, Doubao-vl). The text-only path stays on
// TextProvider; DescribeImage is the only divergence.
//
// The shape is a small subset of the Anthropic Messages
// "content blocks" model: an image is sent as base64 inline
// in a user message, the model returns a text response.
type VLProvider interface {
	TextProvider
	DescribeImage(ctx context.Context, prompt string, image []byte, mimeType string, opts CompleteOptions) (string, Usage, error)
}

// ProviderSet is the umbrella struct main.go constructs and
// passes to handlers. It bundles the three provider slots a
// handler might need: text (for /api/ai/* and similar),
// media (for /api/cover, /api/speech, /api/video), and
// optionally vl (for future vision endpoints). Default
// fields hold the fallback provider; per-request selection
// is layered on top via the Registry.
//
// (Named ProviderSet to avoid colliding with the legacy
// Provider interface above.)
type ProviderSet struct {
	Text     TextProvider
	Media    MediaProvider
	VL       VLProvider
	Registry *ProviderRegistry
}

// DefaultText returns the configured default text provider.
// Convenience helper for handlers that take *ProviderSet.
func (p *ProviderSet) DefaultText() TextProvider {
	if p.Text == nil {
		return Echo
	}
	return p.Text
}

// ErrMediaNotSupported is returned by MediaProvider methods
// the concrete provider doesn't support. The /sandbox page
// surfaces this as "this provider can't do X" rather than
// a 500.
var ErrMediaNotSupported = errors.New("media: operation not supported by this provider")

// ProviderRegistry maps provider name → concrete impl. The
// default providers (Echo + the user's chosen real provider)
// are registered at construction; additional providers can
// be added by main.go before any request lands.
//
// The registry is safe for concurrent reads after the
// initial registration phase (i.e. main.go completes before
// the first request lands). Register / SetDefault take the
// write lock; lookup is a plain map read.
type ProviderRegistry struct {
	mu       sync.RWMutex
	text     map[string]TextProvider
	media    map[string]MediaProvider
	vl       map[string]VLProvider
	defaults TextProvider
}

// NewProviderRegistry returns an empty registry. main.go
// is responsible for calling Register* before any handler
// is hit.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		text: make(map[string]TextProvider),
		media: make(map[string]MediaProvider),
		vl: make(map[string]VLProvider),
	}
}

// RegisterText adds (or replaces) a text provider under the
// given name. The first provider registered also becomes
// the default; subsequent RegisterText calls do not change
// the default — pass SetDefaultText explicitly to do that.
func (r *ProviderRegistry) RegisterText(name string, p TextProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.defaults == nil {
		r.defaults = p
	}
	r.text[name] = p
}

// RegisterMedia adds a media provider.
func (r *ProviderRegistry) RegisterMedia(name string, p MediaProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.media[name] = p
}

// RegisterVL adds a vision-language provider.
func (r *ProviderRegistry) RegisterVL(name string, p VLProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.vl[name] = p
}

// SetDefaultText overrides which registered provider is the
// fallback. main.go calls this after registering all known
// providers if the env wants a different default than the
// first-registered.
func (r *ProviderRegistry) SetDefaultText(p TextProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defaults = p
}

// Text looks up a registered text provider by name. Returns
// (nil, false) if not registered, so callers can fall back
// to the default without panicking.
func (r *ProviderRegistry) Text(name string) (TextProvider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.text[name]
	return p, ok
}

// DefaultText returns the fallback text provider. Always
// non-nil if at least one provider was registered; nil
// only on a freshly-constructed registry.
func (r *ProviderRegistry) DefaultText() TextProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.defaults
}

// Media looks up a registered media provider by name.
func (r *ProviderRegistry) Media(name string) (MediaProvider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.media[name]
	return p, ok
}

// DefaultMedia returns the first-registered media provider.
func (r *ProviderRegistry) DefaultMedia() MediaProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.media {
		return p
	}
	return nil
}

// Echo is the no-op provider. It returns the prompt prefixed
// with "[echo:NAME] " so an operator can tell at a glance
// which provider served the response. Useful for dev /
// CI / smoke-testing without burning real tokens, and as
// a regression fallback when every real provider is
// unavailable.
type EchoProvider struct{ Name_ string }

func NewEchoProvider(name string) *EchoProvider {
	if name == "" {
		name = "echo"
	}
	return &EchoProvider{Name_: name}
}

func (e *EchoProvider) Name() string                       { return e.Name_ }
func (e *EchoProvider) Available() bool                   { return true }
func (e *EchoProvider) Complete(ctx context.Context, prompt string, opts CompleteOptions) (string, error) {
	return fmt.Sprintf("[echo:%s] %s", e.Name_, prompt), nil
}
func (e *EchoProvider) CompleteWithUsage(ctx context.Context, prompt string, opts CompleteOptions) (string, Usage, error) {
	return fmt.Sprintf("[echo:%s] %s", e.Name_, prompt), Usage{}, nil
}

// Echo is a package-level convenience handle to a default
// "echo" provider. Handlers that take *Provider can fall
// back to this when no provider is configured; the
// Provider.DefaultText helper already does this.
var Echo = NewEchoProvider("echo")
