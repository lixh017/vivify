package agents

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// fakeProvider is a minimal Provider stub used by router tests.
// Lives in this test file (not exported) because production code
// never needs a no-op provider — only the router tests do.
type fakeProvider struct {
	name        string
	available   bool
	completeFn  func(ctx context.Context, prompt string, opts CompleteOptions) (string, error)
	lastPrompt  string
	lastOpts    CompleteOptions
	callCounter int
}

func (f *fakeProvider) Name() string    { return f.name }
func (f *fakeProvider) Available() bool { return f.available }
func (f *fakeProvider) Complete(ctx context.Context, prompt string, opts CompleteOptions) (string, error) {
	f.callCounter++
	f.lastPrompt = prompt
	f.lastOpts = opts
	if f.completeFn != nil {
		return f.completeFn(ctx, prompt, opts)
	}
	return "ok:" + f.name, nil
}

func TestRouterRouteForDefaults(t *testing.T) {
	t.Parallel()
	claude := &fakeProvider{name: ProviderClaude, available: true}
	gemini := &fakeProvider{name: ProviderGemini, available: true}
	r := NewRouter(claude, gemini)

	tests := []struct {
		task         TaskName
		wantProvider string
		wantModel    string
	}{
		{TaskTopicGenerate, ProviderClaude, ModelClaudeSonnet4_5},
		{TaskScriptHumanize, ProviderClaude, ModelClaudeSonnet4_5},
		{TaskContentScore, ProviderClaude, ModelClaudeHaiku4_5},
		{TaskPlatformAdapt, ProviderClaude, ModelClaudeSonnet4_5},
		{TaskDeconstruct, ProviderGemini, GeminiDefaultModel},
		{TaskViralFormula, ProviderClaude, ModelClaudeSonnet4_5},
		{TaskPostmortem, ProviderClaude, ModelClaudeSonnet4_5},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(string(tt.task), func(t *testing.T) {
			t.Parallel()
			p, model := r.RouteFor(tt.task)
			if p == nil {
				t.Fatalf("RouteFor(%q) returned nil provider", tt.task)
			}
			if p.Name() != tt.wantProvider {
				t.Errorf("provider = %q, want %q", p.Name(), tt.wantProvider)
			}
			if model != tt.wantModel {
				t.Errorf("model = %q, want %q", model, tt.wantModel)
			}
		})
	}
}

// TestRouterFallsBackWhenProviderMissing covers the dev-mode path:
// GEMINI_API_KEY is unset → no Gemini provider registered → the
// deconstruct task should fall back to whatever provider IS
// available (Claude). The configured model name is preserved so
// logs show what was requested.
func TestRouterFallsBackWhenProviderMissing(t *testing.T) {
	t.Parallel()
	claude := &fakeProvider{name: ProviderClaude, available: true}
	r := NewRouter(claude) // no gemini

	p, model := r.RouteFor(TaskDeconstruct)
	if p == nil {
		t.Fatal("expected fallback provider, got nil")
	}
	if p.Name() != ProviderClaude {
		t.Errorf("fallback provider = %q, want claude", p.Name())
	}
	if model != GeminiDefaultModel {
		t.Errorf("model = %q, want %q (preserved configured model)", model, GeminiDefaultModel)
	}
}

// TestRouterFallsBackWhenProviderUnavailable covers "registered
// but no API key" — the provider exists but Available() returns
// false. The router should still fall back to the first available
// provider rather than handing back the unavailable one.
func TestRouterFallsBackWhenProviderUnavailable(t *testing.T) {
	t.Parallel()
	claude := &fakeProvider{name: ProviderClaude, available: true}
	gemini := &fakeProvider{name: ProviderGemini, available: false}
	r := NewRouter(claude, gemini)

	p, _ := r.RouteFor(TaskDeconstruct)
	if p == nil || p.Name() != ProviderClaude {
		t.Fatalf("expected claude fallback, got %v", p)
	}
}

// TestRouterReturnsNilWhenNoProviders ensures the demo-only mode
// (no providers at all) is signalled by a (nil, "") return so
// callers can branch into DemoResponse without an explicit
// "all providers missing" check at every call site.
func TestRouterReturnsNilWhenNoProviders(t *testing.T) {
	t.Parallel()
	r := NewRouter()
	p, _ := r.RouteFor(TaskTopicGenerate)
	if p != nil {
		t.Errorf("expected nil provider for empty router, got %q", p.Name())
	}
}

// TestRouterReturnsConfiguredEvenIfUnavailableAsLastResort verifies
// that when only the configured-but-unavailable provider is
// registered, the router still hands it back so the caller gets a
// real ErrProviderUnavailable instead of a silent demo-mode swap.
func TestRouterReturnsConfiguredEvenIfUnavailableAsLastResort(t *testing.T) {
	t.Parallel()
	claude := &fakeProvider{name: ProviderClaude, available: false}
	r := NewRouter(claude)
	p, _ := r.RouteFor(TaskTopicGenerate)
	if p == nil {
		t.Fatal("expected provider returned (even unavailable) as last resort")
	}
	if p.Name() != ProviderClaude {
		t.Errorf("provider = %q, want claude", p.Name())
	}
}

// TestRouterCompleteUsesRoutedModel verifies the convenience
// Complete wrapper actually plumbs the routed model into the
// CompleteOptions handed to the provider — a regression here
// would silently make every task hit the provider's default model.
func TestRouterCompleteUsesRoutedModel(t *testing.T) {
	t.Parallel()
	claude := &fakeProvider{name: ProviderClaude, available: true}
	r := NewRouter(claude)
	_, err := r.Complete(context.Background(), TaskContentScore, "hello", CompleteOptions{})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if string(claude.lastOpts.Model) != ModelClaudeHaiku4_5 {
		t.Errorf("routed model = %q, want haiku", claude.lastOpts.Model)
	}
}

// TestRouterCompleteCallerOverridesModel verifies an explicit
// per-call CompleteOptions.Model wins over the routed model so
// one-off experiments work without rewriting the route table.
func TestRouterCompleteCallerOverridesModel(t *testing.T) {
	t.Parallel()
	claude := &fakeProvider{name: ProviderClaude, available: true}
	r := NewRouter(claude)
	_, err := r.Complete(context.Background(), TaskContentScore, "hello", CompleteOptions{
		Model: ModelClaudeOpus4_5,
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if string(claude.lastOpts.Model) != ModelClaudeOpus4_5 {
		t.Errorf("caller model = %q, want opus", claude.lastOpts.Model)
	}
}

// TestRouterCompletePropagatesError ensures a provider error
// reaches the caller verbatim (wrapped, not swallowed).
func TestRouterCompletePropagatesError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("synthetic 429 rate limit")
	claude := &fakeProvider{
		name:      ProviderClaude,
		available: true,
		completeFn: func(_ context.Context, _ string, _ CompleteOptions) (string, error) {
			return "", wantErr
		},
	}
	r := NewRouter(claude)
	_, err := r.Complete(context.Background(), TaskTopicGenerate, "x", CompleteOptions{})
	if err == nil || !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want wraps %v", err, wantErr)
	}
}

// TestRouterCompleteEmptyRouterErrors covers the "no providers
// registered" mode: Complete should refuse, not silently succeed.
func TestRouterCompleteEmptyRouterErrors(t *testing.T) {
	t.Parallel()
	r := NewRouter()
	_, err := r.Complete(context.Background(), TaskTopicGenerate, "x", CompleteOptions{})
	if err == nil {
		t.Fatal("expected error from empty router")
	}
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Errorf("err = %v, want wraps ErrProviderUnavailable", err)
	}
}

// TestNewRouterWithOverrides verifies the overrides path: pin a
// task to a cheaper model without forking DefaultRoutes.
func TestNewRouterWithOverrides(t *testing.T) {
	t.Parallel()
	claude := &fakeProvider{name: ProviderClaude, available: true}
	deepseek := &fakeProvider{name: ProviderDeepSeek, available: true}
	overrides := map[TaskName]RouteConfig{
		TaskTopicGenerate: {Provider: ProviderDeepSeek, Model: "deepseek-chat"},
	}
	r := NewRouterWithOverrides([]Provider{claude, deepseek}, overrides)
	p, model := r.RouteFor(TaskTopicGenerate)
	if p == nil || p.Name() != ProviderDeepSeek {
		t.Fatalf("provider = %v, want deepseek", p)
	}
	if model != "deepseek-chat" {
		t.Errorf("model = %q, want deepseek-chat", model)
	}
	// Untouched routes must remain on Claude.
	p2, _ := r.RouteFor(TaskContentScore)
	if p2 == nil || p2.Name() != ProviderClaude {
		t.Errorf("untouched route should remain claude, got %v", p2)
	}
}

// TestParseOverrides exercises the env-string parser with the
// happy paths and the surfacing of warnings.
func TestParseOverrides(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     string
		want      map[TaskName]RouteConfig
		warnCount int
	}{
		{
			name:  "empty input returns nil",
			input: "",
			want:  nil,
		},
		{
			name:  "single override uses task default provider",
			input: "topic_generate=claude-haiku-4-5",
			want: map[TaskName]RouteConfig{
				TaskTopicGenerate: {Provider: ProviderClaude, Model: "claude-haiku-4-5"},
			},
		},
		{
			name:  "provider prefix swaps backend",
			input: "topic_generate=deepseek:deepseek-chat",
			want: map[TaskName]RouteConfig{
				TaskTopicGenerate: {Provider: ProviderDeepSeek, Model: "deepseek-chat"},
			},
		},
		{
			name:      "malformed pair surfaces warning, no entry",
			input:     "garbage",
			want:      map[TaskName]RouteConfig{},
			warnCount: 1,
		},
		{
			name:      "unknown task surfaces warning but still applied",
			input:     "unknown_task=foo:bar",
			warnCount: 1,
			want: map[TaskName]RouteConfig{
				TaskName("unknown_task"): {Provider: "foo", Model: "bar"},
			},
		},
		{
			name:  "two pairs",
			input: "topic_generate=deepseek:deepseek-chat, content_score=gemini:gemini-2.5-flash",
			want: map[TaskName]RouteConfig{
				TaskTopicGenerate: {Provider: ProviderDeepSeek, Model: "deepseek-chat"},
				TaskContentScore:  {Provider: ProviderGemini, Model: "gemini-2.5-flash"},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, warns := ParseOverrides(tt.input)
			if len(warns) != tt.warnCount {
				t.Errorf("warnings = %v, want %d", warns, tt.warnCount)
			}
			if tt.want == nil && got != nil {
				t.Errorf("got = %v, want nil", got)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestRouterRoutesDefensiveCopy guards against the router exposing
// its internal route table by reference — a caller mutating the
// returned map must not affect future RouteFor lookups.
func TestRouterRoutesDefensiveCopy(t *testing.T) {
	t.Parallel()
	claude := &fakeProvider{name: ProviderClaude, available: true}
	r := NewRouter(claude)
	routes := r.Routes()
	routes[TaskTopicGenerate] = RouteConfig{Provider: "evil", Model: "evil-model"}
	p, model := r.RouteFor(TaskTopicGenerate)
	if p == nil || p.Name() != ProviderClaude || model == "evil-model" {
		t.Errorf("mutation of returned Routes leaked into router state: provider=%v model=%q", p, model)
	}
}

// TestProvidersDeterministicOrder verifies Providers() returns a
// stable order so /readyz output and logs don't churn.
func TestProvidersDeterministicOrder(t *testing.T) {
	t.Parallel()
	a := &fakeProvider{name: "z-last", available: true}
	b := &fakeProvider{name: "a-first", available: true}
	c := &fakeProvider{name: "m-middle", available: true}
	r := NewRouter(a, b, c)
	ps := r.Providers()
	if len(ps) != 3 {
		t.Fatalf("Providers() len = %d, want 3", len(ps))
	}
	wantOrder := []string{"a-first", "m-middle", "z-last"}
	for i, name := range wantOrder {
		if ps[i].Name() != name {
			t.Errorf("Providers()[%d] = %q, want %q", i, ps[i].Name(), name)
		}
	}
}

// TestClaudeProviderUnavailableReturnsSentinel was removed in
// Phase 4: the ClaudeProvider shim (and the *Claude struct it
// adapted) is gone. The router now uses MiniMax / DeepSeek /
// Gemini providers only. Equivalent coverage of the dev-mode
// unavailable sentinel lives on the surviving providers.

// TestDeepSeekProviderHappyPath uses a httptest.Server to verify
// the request shape (model + messages) and parse the response. We
// avoid the real DeepSeek endpoint in tests; this catches contract
// drift in the request builder without burning credits.
func TestDeepSeekProviderHappyPath(t *testing.T) {
	t.Parallel()
	var gotPath string
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hello back"}}]}`))
	}))
	defer srv.Close()
	p := NewDeepSeekProviderWithClient("test-key", srv.URL, srv.Client())
	got, err := p.Complete(context.Background(), "hi", CompleteOptions{})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got != "hello back" {
		t.Errorf("got %q, want hello back", got)
	}
	if gotPath != "/" {
		t.Errorf("path = %q, want /", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("auth = %q, want Bearer test-key", gotAuth)
	}
}

// TestDeepSeekProviderHTTPError verifies non-2xx responses surface
// as errors with the status code in the message — operator logs
// need the status code to triage.
func TestDeepSeekProviderHTTPError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited","code":"429"}}`))
	}))
	defer srv.Close()
	p := NewDeepSeekProviderWithClient("test-key", srv.URL, srv.Client())
	_, err := p.Complete(context.Background(), "x", CompleteOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestGeminiProviderHappyPath mirrors the deepseek test for the
// generateContent shape.
func TestGeminiProviderHappyPath(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Expect query string contains key=
		if r.URL.Query().Get("key") != "test-key" {
			t.Errorf("expected key query param, got url=%s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"gemini reply"}]}}]}`))
	}))
	defer srv.Close()
	p := NewGeminiProviderWithClient("test-key", srv.URL, srv.Client())
	got, err := p.Complete(context.Background(), "hi", CompleteOptions{})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got != "gemini reply" {
		t.Errorf("got %q, want gemini reply", got)
	}
}

// TestDeepSeekProviderUnavailable covers the dev-mode contract.
func TestDeepSeekProviderUnavailable(t *testing.T) {
	t.Parallel()
	p := NewDeepSeekProvider("")
	if p.Available() {
		t.Error("Available() must be false for empty key")
	}
	_, err := p.Complete(context.Background(), "x", CompleteOptions{})
	if err == nil || !errors.Is(err, ErrProviderUnavailable) {
		t.Errorf("err = %v, want wraps ErrProviderUnavailable", err)
	}
}

// TestGeminiProviderUnavailable covers the dev-mode contract.
func TestGeminiProviderUnavailable(t *testing.T) {
	t.Parallel()
	p := NewGeminiProvider("")
	if p.Available() {
		t.Error("Available() must be false for empty key")
	}
	_, err := p.Complete(context.Background(), "x", CompleteOptions{})
	if err == nil || !errors.Is(err, ErrProviderUnavailable) {
		t.Errorf("err = %v, want wraps ErrProviderUnavailable", err)
	}
}
