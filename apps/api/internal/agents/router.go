package agents

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// TaskName is a typed string identifying which OPC AI task is
// being executed. The router maps a TaskName to a (provider, model)
// pair. Defined as constants so a typo at a call site is a compile
// error and `gofmt` keeps the lookup table aligned.
type TaskName string

const (
	TaskTopicGenerate  TaskName = "topic_generate"
	TaskScriptHumanize TaskName = "script_humanize"
	TaskContentScore   TaskName = "content_score"
	TaskPlatformAdapt  TaskName = "platform_adapt"
	TaskDeconstruct    TaskName = "deconstruct"
	TaskViralFormula   TaskName = "viral_formula"
	TaskPostmortem     TaskName = "postmortem"
)

// RouteConfig is the policy for a single task: which provider
// should serve it, and which model on that provider.
//
// We hold Model as a string (not anthropic.Model) on purpose so the
// table can name models that don't live in the Anthropic SDK
// (gemini-2.5-pro, deepseek-chat). The Claude provider casts to
// anthropic.Model at the call site; other providers consume the
// string directly.
type RouteConfig struct {
	Provider string // canonical name (ProviderClaude, ProviderDeepSeek, ProviderGemini)
	Model    string // provider-specific model identifier
}

// DefaultRoutes is the industry-tuned per-task model strategy for
// OPC. Rationale:
//   - Sonnet 4.5 for creative-writing-heavy tasks (best Chinese
//     long-form quality, anti-AI patterns need top model).
//   - Haiku 4.5 for content_score (simple structured scoring,
//     latency-sensitive, ~3x cheaper).
//   - Gemini 2.5 Pro for video deconstruction (multimodal video
//     understanding is its differentiator).
//
// This is the package default; callers can override via
// NewRouterWithOverrides to inject env-based per-task model
// overrides without forking the table.
var DefaultRoutes = map[TaskName]RouteConfig{
	TaskTopicGenerate:  {Provider: ProviderClaude, Model: ModelClaudeSonnet4_5},
	TaskScriptHumanize: {Provider: ProviderClaude, Model: ModelClaudeSonnet4_5},
	TaskContentScore:   {Provider: ProviderClaude, Model: ModelClaudeHaiku4_5},
	TaskPlatformAdapt:  {Provider: ProviderClaude, Model: ModelClaudeSonnet4_5},
	TaskDeconstruct:    {Provider: ProviderGemini, Model: GeminiDefaultModel},
	TaskViralFormula:   {Provider: ProviderClaude, Model: ModelClaudeSonnet4_5},
	TaskPostmortem:     {Provider: ProviderClaude, Model: ModelClaudeSonnet4_5},
}

// Router resolves a TaskName to a (Provider, model) pair at call
// time. It owns the providers (so handlers don't have to thread
// every backend through) and applies env-driven model overrides
// without disturbing the package-level DefaultRoutes table.
//
// The router is safe for concurrent use: providers are read-only
// after construction, routes are copied at construction so callers
// can mutate the input map without affecting the router.
type Router struct {
	mu        sync.RWMutex
	providers map[string]Provider
	routes    map[TaskName]RouteConfig
}

// NewRouter wires the provided providers (by Name()) into a router
// and uses DefaultRoutes as the task → (provider, model) policy.
// Pass NewClaudeProvider, NewDeepSeekProvider, NewGeminiProvider
// (any subset) — missing providers cause RouteFor to fall back to
// the next-best available alternative when possible.
func NewRouter(providers ...Provider) *Router {
	return NewRouterWithOverrides(providers, nil)
}

// NewRouterWithOverrides is like NewRouter but lets the caller
// merge per-task overrides on top of DefaultRoutes. Keys of
// `overrides` use the same TaskName constants. Unknown task names
// in `overrides` are still respected (so future tasks can be
// configured ahead of code support) but won't be picked up until a
// caller asks for them.
func NewRouterWithOverrides(providers []Provider, overrides map[TaskName]RouteConfig) *Router {
	r := &Router{
		providers: make(map[string]Provider, len(providers)),
		routes:    make(map[TaskName]RouteConfig, len(DefaultRoutes)+len(overrides)),
	}
	for _, p := range providers {
		if p == nil {
			continue
		}
		r.providers[p.Name()] = p
	}
	// Copy defaults first so caller-side mutations to DefaultRoutes
	// don't leak into the router. Then layer overrides — non-empty
	// fields replace the default (zero fields leave the default).
	for k, v := range DefaultRoutes {
		r.routes[k] = v
	}
	for k, v := range overrides {
		existing, ok := r.routes[k]
		if !ok {
			r.routes[k] = v
			continue
		}
		if v.Provider != "" {
			existing.Provider = v.Provider
		}
		if v.Model != "" {
			existing.Model = v.Model
		}
		r.routes[k] = existing
	}
	return r
}

// RouteFor returns the provider + model assigned to a task name.
// When the configured provider is not registered with the router
// (e.g. GEMINI_API_KEY is unset so Gemini wasn't constructed), it
// falls back to the first available provider in a deterministic
// preference order: claude > deepseek > gemini.
//
// The returned Provider may itself report Available()==false (no
// API key in dev mode); callers should branch on that to decide
// whether to invoke Complete or fall back to demo data.
//
// Returns (nil, "") when no provider is registered at all — the
// caller MUST treat this as "demo data only".
func (r *Router) RouteFor(task TaskName) (Provider, string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cfg, ok := r.routes[task]
	if !ok {
		// Unknown task: try the default-known providers in
		// preference order so we degrade gracefully instead of
		// silently 500-ing.
		return r.fallbackProvider(), ""
	}
	if p, ok := r.providers[cfg.Provider]; ok && p.Available() {
		return p, cfg.Model
	}
	// Configured provider missing or unavailable — fall back to
	// any available provider, but keep the configured model name
	// so logs still show what was requested. The caller can
	// override the model via CompleteOptions if the fallback
	// provider doesn't know that model.
	if p := r.fallbackProvider(); p != nil {
		return p, cfg.Model
	}
	// Last resort: hand back the configured provider even if it
	// reported Available()==false, so the caller can surface a
	// proper ErrProviderUnavailable instead of silently swapping
	// to demo data without saying so.
	if p, ok := r.providers[cfg.Provider]; ok {
		return p, cfg.Model
	}
	return nil, cfg.Model
}

// Complete is a convenience wrapper that routes a task to its
// provider and invokes Complete with the right model. CompleteOptions
// supplied by the caller win over the routed model (so a one-off
// override at the call site is still possible).
func (r *Router) Complete(ctx context.Context, task TaskName, prompt string, opts CompleteOptions) (string, error) {
	p, model := r.RouteFor(task)
	if p == nil {
		return "", fmt.Errorf("router: no provider available for task %q: %w", task, ErrProviderUnavailable)
	}
	if opts.Model == "" && model != "" {
		opts.Model = model
	}
	return p.Complete(ctx, prompt, opts)
}

// Providers returns the registered providers in a deterministic
// order (alphabetical by Name). Intended for diagnostics and /readyz
// surface, not for hot-path use.
func (r *Router) Providers() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.providers))
	for n := range r.providers {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Provider, 0, len(names))
	for _, n := range names {
		out = append(out, r.providers[n])
	}
	return out
}

// Routes returns a defensive copy of the task → route table.
// Useful for diagnostics endpoints; the returned map can be
// mutated without affecting the router.
func (r *Router) Routes() map[TaskName]RouteConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[TaskName]RouteConfig, len(r.routes))
	for k, v := range r.routes {
		out[k] = v
	}
	return out
}

// fallbackProvider returns the first Available provider in the
// preferred order. Callers hold the read lock.
func (r *Router) fallbackProvider() Provider {
	for _, name := range []string{ProviderClaude, ProviderDeepSeek, ProviderGemini} {
		if p, ok := r.providers[name]; ok && p.Available() {
			return p
		}
	}
	return nil
}

// ParseOverrides converts an env-style "task=model,task=model" string
// into a map suitable for NewRouterWithOverrides. Used by the config
// loader so operators can pin a single task to a cheaper model
// without recompiling.
//
// Format: TASK_NAME=MODEL_ID[,TASK_NAME=MODEL_ID...]
// Provider is inferred from the task's default. To change provider
// for a task, pass MODEL_ID prefixed with "provider:" e.g.
// "topic_generate=deepseek:deepseek-chat".
//
// Unknown tasks are silently dropped (a typo shouldn't take the
// server down) but logged via the returned warnings slice so the
// operator can spot misconfiguration.
func ParseOverrides(raw string) (map[TaskName]RouteConfig, []string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	out := make(map[TaskName]RouteConfig)
	var warnings []string
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		eq := strings.IndexByte(pair, '=')
		if eq <= 0 || eq == len(pair)-1 {
			warnings = append(warnings, "malformed override (want task=model): "+pair)
			continue
		}
		task := TaskName(strings.TrimSpace(pair[:eq]))
		val := strings.TrimSpace(pair[eq+1:])
		def, known := DefaultRoutes[task]
		if !known {
			warnings = append(warnings, "unknown task in override: "+string(task))
			// Still keep it — see ParseOverrides docstring.
		}
		cfg := RouteConfig{Provider: def.Provider, Model: val}
		if colon := strings.IndexByte(val, ':'); colon > 0 {
			cfg.Provider = strings.TrimSpace(val[:colon])
			cfg.Model = strings.TrimSpace(val[colon+1:])
		}
		out[task] = cfg
	}
	return out, warnings
}
