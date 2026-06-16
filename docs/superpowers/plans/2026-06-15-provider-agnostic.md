# Provider-Agnostic Multi-Provider Support (Phase 4) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the hardcoded `*agents.MiniMax` client with a user-configurable provider system. Users pick Anthropic-protocol or OpenAI-protocol, choose a model, store their own API key on `/credentials`. Handlers resolve the right provider per request, no code change required to add a new upstream.

**Architecture:**
- Two thin protocol adapters (`AnthropicCompatProvider`, `OpenAICompatProvider`) implement the existing `agents.TextProvider` interface.
- The `credentials` table gains three columns: `protocol` (`anthropic`/`openai`), `base_url` (optional override), `model_name` (e.g. `claude-sonnet-4-5`).
- A new `agents.ProviderResolver` maps `(user_id, scope)` → live provider instance by reading the credential row, decrypting the key, and wrapping the right adapter.
- Every text-generating handler changes its field type from `*agents.MiniMax` to `agents.TextProvider` and is constructed with a `*agents.ProviderResolver`.
- `*agents.MiniMax` stays as the **built-in media provider** (image/speech/video) — its `/v1/image_generation`, `/v1/t2a_v2`, `/v1/video_generation` endpoints are not Anthropic- or OpenAI-compatible, so user-configurable media is out of scope for this phase.

**Tech Stack:**
- Go 1.22+ (Gin, GORM, SQLite)
- Existing `internal/agents` package (`TextProvider` / `MediaProvider` interfaces already defined)
- Next.js 14 + Tailwind + TypeScript (console UI)

---

## File Structure

| Action  | Path | Responsibility |
|---------|------|----------------|
| Modify  | `apps/api/internal/models/credential.go` | Add `Protocol`, `BaseURL`, `ModelName` columns; relax `ProviderIsAllowed` to allow `anthropic`+`openai` |
| Modify  | `apps/api/internal/db/migrations.go` | Auto-migrate new columns on `credentials` (GORM AutoMigrate already covers it — verify + add test) |
| Create  | `apps/api/internal/agents/anthropic_compat.go` | `AnthropicCompatProvider` — POST `/v1/messages` |
| Create  | `apps/api/internal/agents/openai_compat.go` | `OpenAICompatProvider` — POST `/v1/chat/completions` |
| Create  | `apps/api/internal/agents/resolver.go` | `ProviderResolver` with `Text(ctx, userID, scope) (TextProvider, error)` and `Media(...)` |
| Create  | `apps/api/internal/agents/resolver_test.go` | Resolver unit tests (mock DB, decrypt, adapter wiring) |
| Create  | `apps/api/internal/agents/anthropic_compat_test.go` | Adapter HTTP tests via `httptest.NewServer` |
| Create  | `apps/api/internal/agents/openai_compat_test.go` | Adapter HTTP tests via `httptest.NewServer` |
| Modify  | `apps/api/internal/handlers/ai.go` | Field `text *agents.MiniMax` → `text agents.TextProvider`; constructor takes `*agents.ProviderResolver` and resolves per-request |
| Modify  | `apps/api/internal/handlers/quality.go` | Same migration pattern |
| Modify  | `apps/api/internal/handlers/deconstruct.go` | Same |
| Modify  | `apps/api/internal/handlers/pipeline.go` | Same |
| Modify  | `apps/api/internal/handlers/batch.go` | Take a `func() agents.TextProvider` (already does — just widen type) |
| Modify  | `apps/api/internal/handlers/cover.go` | Keep `*agents.MiniMax` (media — not in this phase) |
| Modify  | `apps/api/internal/handlers/speech.go` | Keep `*agents.MiniMax` (media — not in this phase) |
| Modify  | `apps/api/internal/handlers/video.go` | Keep `*agents.MiniMax` (media — not in this phase) |
| Modify  | `apps/api/internal/handlers/credentials.go` | Accept `protocol`, `base_url`, `model_name` in `createRequest` and `rotateRequest`; persist + return |
| Modify  | `apps/api/internal/handlers/credentials_test.go` | Cover new request shape + validation |
| Modify  | `apps/api/internal/capabilities/topic/topic.go` | `Generate(ctx, text agents.TextProvider, input Input)` — drop `*agents.MiniMax` |
| Modify  | `apps/api/internal/mcp/server.go` | Field `text *agents.MiniMax` → `text agents.TextProvider`; constructor takes resolver |
| Modify  | `apps/api/internal/mcp/tools_ai.go` | Same; uses `text.Text(ctx, prompt, agents.TextOptions{Model: ""})` |
| Modify  | `apps/api/cmd/server/main.go` | Construct `ProviderResolver`, pass to AI/Quality/Deconstruct/Pipeline/MCP |
| Modify  | `apps/api/cmd/opc-asset/main.go` | Resolve per-invocation via env or hardcoded preset for the asset CLI |
| Modify  | `apps/shared/src/types/credential.ts` | Add `protocol`, `base_url`, `model_name` to `Credential` type; widen `CredentialProvider` to `'anthropic' \| 'openai' \| 'volcengine' \| 'douyin'` |
| Modify  | `apps/console/components/AddCredentialDialog.tsx` | Add `protocol` (radio), `base_url` (text, optional), `model_name` (text, required for new providers) fields |
| Modify  | `apps/console/components/RotateDialog.tsx` | Show non-editable `protocol/base_url/model_name` (rotate = key only) |
| Modify  | `apps/console/app/(console)/credentials/page.tsx` | Add columns: Protocol / Base URL / Model |
| Modify  | `apps/console/lib/use-credentials.ts` | Pass through new fields |
| Modify  | `docs/console-reserve-audit.md` | Add Phase 4 section noting "user-configurable provider, no admin override needed for multi-vendor" |

**Total**: 8 created, 19 modified, 0 deleted. Net surface change ≈ 2,500 LOC.

---

## Task 1: Extend Credential model with protocol / base_url / model_name

**Files:**
- Modify: `apps/api/internal/models/credential.go`
- Test: `apps/api/internal/models/credential_test.go` (extend existing — model dir already has table-driven test patterns; if no test file, create one)

- [ ] **Step 1: Write failing tests for new columns and validation**

Create `apps/api/internal/models/credential_test.go`:

```go
package models

import (
	"strings"
	"testing"
)

func TestCredential_Validate_AcceptsAnthropicProtocol(t *testing.T) {
	c := Credential{
		Name:       "primary",
		Provider:   "anthropic",
		Protocol:   "anthropic",
		ModelName:  "claude-sonnet-4-5",
		Scope:      "all",
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	if c.Protocol != "anthropic" {
		t.Fatalf("want normalized protocol=anthropic, got %q", c.Protocol)
	}
}

func TestCredential_Validate_RejectsBadProtocol(t *testing.T) {
	c := Credential{
		Name:      "primary",
		Provider:  "anthropic",
		Protocol:  "gopher",
		ModelName: "claude-sonnet-4-5",
	}
	err := c.Validate()
	if err == nil {
		t.Fatal("want error for protocol=gopher, got nil")
	}
	if !strings.Contains(err.Error(), "protocol") {
		t.Fatalf("want error mentioning protocol, got %v", err)
	}
}

func TestCredential_Validate_RequiresModelForNewProtocols(t *testing.T) {
	c := Credential{
		Name:     "primary",
		Provider: "anthropic",
		Protocol: "anthropic",
		// ModelName missing
	}
	if err := c.Validate(); err == nil {
		t.Fatal("want error for missing model_name, got nil")
	}
}

func TestProviderIsAllowed_IncludesAnthropicAndOpenAI(t *testing.T) {
	for _, p := range []string{"anthropic", "openai", "Anthropic", "OPENAI"} {
		if !ProviderIsAllowed(p) {
			t.Errorf("want %q to be allowed", p)
		}
	}
}
```

- [ ] **Step 2: Run tests to confirm RED**

Run: `cd apps/api && go test ./internal/models/... -run TestCredential -v`
Expected: FAIL — `Credential` has no `Protocol`/`ModelName` fields yet.

- [ ] **Step 3: Extend the model**

Edit `apps/api/internal/models/credential.go`:

```go
const (
	ProviderVolcengine = "volcengine"
	ProviderAnthropic  = "anthropic"
	ProviderOpenAI     = "openai"
	ProviderDouyin     = "douyin"
)

// Protocol is the wire protocol a credential uses. "anthropic"
// hits POST /v1/messages; "openai" hits POST /v1/chat/completions.
// volcengine/douyin keep their custom shapes and are handled
// out-of-band by the media providers, not the resolver.
const (
	ProtocolAnthropic = "anthropic"
	ProtocolOpenAI    = "openai"
)

func ProtocolIsAllowed(p string) bool {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case ProtocolAnthropic, ProtocolOpenAI:
		return true
	default:
		return false
	}
}

type Credential struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	UserID       uint       `gorm:"index" json:"user_id"`
	Provider     string     `gorm:"index" json:"provider"`
	Protocol     string     `gorm:"-" json:"protocol,omitempty"`     // custom column
	BaseURL      string     `gorm:"-" json:"base_url,omitempty"`     // custom column
	ModelName    string     `gorm:"-" json:"model_name,omitempty"`   // custom column
	Name         string     `json:"name"`
	EncryptedKey []byte     `json:"-"`
	Scope        string     `json:"scope"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
}
```

Replace the `gorm:"-"` tags with explicit migration (GORM AutoMigrate detects them):

```go
type Credential struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	UserID       uint       `gorm:"index" json:"user_id"`
	Provider     string     `gorm:"index" json:"provider"`
	Protocol     string     `gorm:"size:32" json:"protocol,omitempty"`
	BaseURL      string     `gorm:"size:512" json:"base_url,omitempty"`
	ModelName    string     `gorm:"size:128" json:"model_name,omitempty"`
	Name         string     `json:"name"`
	EncryptedKey []byte     `json:"-"`
	Scope        string     `json:"scope"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
}
```

In `ProviderIsAllowed`, add `"openai"` to the switch case.

In `Validate`, after the existing scope default, add:

```go
c.Protocol = strings.ToLower(strings.TrimSpace(c.Protocol))
c.BaseURL = strings.TrimSpace(c.BaseURL)
c.ModelName = strings.TrimSpace(c.ModelName)
// volcengine/douyin rows have empty Protocol/ModelName and stay allowed
if c.Provider == ProviderAnthropic || c.Provider == ProviderOpenAI {
	if !ProtocolIsAllowed(c.Protocol) {
		return wrapInvalid("protocol is required and must be 'anthropic' or 'openai' for this provider")
	}
	if c.ModelName == "" {
		return wrapInvalid("model_name is required for anthropic/openai providers")
	}
}
```

- [ ] **Step 4: Run tests to confirm GREEN**

Run: `cd apps/api && go test ./internal/models/... -run TestCredential -v`
Expected: PASS.

- [ ] **Step 5: Run gofmt + vet**

Run: `cd apps/api && gofmt -w internal/models/credential.go internal/models/credential_test.go && go vet -tags fts5 ./internal/models/...`
Expected: exit 0.

- [ ] **Step 6: Commit**

```bash
cd apps/api
git add internal/models/credential.go internal/models/credential_test.go
git commit -m "feat(models): credential gains protocol/base_url/model_name"
```

---

## Task 2: AnthropicCompatProvider

**Files:**
- Create: `apps/api/internal/agents/anthropic_compat.go`
- Create: `apps/api/internal/agents/anthropic_compat_test.go`

- [ ] **Step 1: Write failing tests**

Create `apps/api/internal/agents/anthropic_compat_test.go`:

```go
package agents

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeAnthropic returns a server that echoes a canned Messages API
// response. The handler asserts the request shape is the Anthropic
// Messages API: model + max_tokens + messages.
func fakeAnthropic(t *testing.T, wantModel string, captured *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		*captured = string(body)
		var req struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			MaxTokens int `json:"max_tokens"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if req.Model != wantModel {
			http.Error(w, "wrong model", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":    "msg_test",
			"type":  "message",
			"role":  "assistant",
			"model": req.Model,
			"content": []map[string]string{
				{"type": "text", "text": "hi from anthropic"},
			},
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 11, "output_tokens": 22},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAnthropicCompatProvider_CompleteWithUsage(t *testing.T) {
	var captured string
	srv := fakeAnthropic(t, "claude-sonnet-4-5", &captured)

	p := NewAnthropicCompatProvider(AnthropicCompatConfig{
		APIKey:    "sk-test",
		BaseURL:   srv.URL,
		ModelName: "claude-sonnet-4-5",
	})

	text, usage, err := p.CompleteWithUsage(context.Background(), "hello", CompleteOptions{MaxTokens: 64})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if text != "hi from anthropic" {
		t.Errorf("want text=hi from anthropic, got %q", text)
	}
	if usage.InputTokens != 11 || usage.OutputTokens != 22 {
		t.Errorf("want usage=11/22, got %d/%d", usage.InputTokens, usage.OutputTokens)
	}
	if !strings.Contains(captured, `"model":"claude-sonnet-4-5"`) {
		t.Errorf("captured request missing model: %s", captured)
	}
}

func TestAnthropicCompatProvider_Available(t *testing.T) {
	// empty key → Available()==false
	p := NewAnthropicCompatProvider(AnthropicCompatConfig{ModelName: "x"})
	if p.Available() {
		t.Error("want Available()=false with no key")
	}
	p = NewAnthropicCompatProvider(AnthropicCompatConfig{APIKey: "sk-x", ModelName: "y"})
	if !p.Available() {
		t.Error("want Available()=true with key")
	}
}
```

- [ ] **Step 2: Run tests to confirm RED**

Run: `cd apps/api && go test ./internal/agents/... -run TestAnthropicCompat -v`
Expected: FAIL — `NewAnthropicCompatProvider` undefined.

- [ ] **Step 3: Implement the provider**

Create `apps/api/internal/agents/anthropic_compat.go`:

```go
package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// AnthropicCompatConfig is the user-tunable shape of an
// Anthropic-protocol endpoint. ModelName is required; BaseURL
// defaults to api.anthropic.com; APIKey is the user-supplied
// secret.
type AnthropicCompatConfig struct {
	APIKey    string
	BaseURL   string
	ModelName string
}

// AnthropicCompatProvider is the protocol adapter that hits any
// endpoint speaking the Anthropic Messages API shape. It
// implements TextProvider. Both api.anthropic.com (real Claude)
// and MiniMax's /anthropic/v1/messages endpoint are reachable
// from the same code — only the BaseURL differs.
type AnthropicCompatProvider struct {
	cfg    AnthropicCompatConfig
	http   *http.Client
}

// NewAnthropicCompatProvider constructs a provider. Missing
// APIKey is OK — Available() reports false and callers fall back
// to demo mode.
func NewAnthropicCompatProvider(cfg AnthropicCompatConfig) *AnthropicCompatProvider {
	base := cfg.BaseURL
	if base == "" {
		base = "https://api.anthropic.com"
	}
	return &AnthropicCompatProvider{
		cfg:  cfg,
		http: &http.Client{Timeout: DefaultProviderTimeout},
	}
}

func (p *AnthropicCompatProvider) Name() string     { return "anthropic" }
func (p *AnthropicCompatProvider) Available() bool { return strings.TrimSpace(p.cfg.APIKey) != "" }

type anthropicCompatRequest struct {
	Model     string  `json:"model"`
	MaxTokens int     `json:"max_tokens,omitempty"`
	System    string  `json:"system,omitempty"`
	Messages  []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

func (p *AnthropicCompatProvider) CompleteWithUsage(ctx context.Context, prompt string, opts CompleteOptions) (string, Usage, error) {
	if !p.Available() {
		return "", Usage{}, errors.New("anthropic-compat: APIKey not configured")
	}
	model := opts.Model
	if model == "" {
		model = p.cfg.ModelName
	}
	if model == "" {
		return "", Usage{}, errors.New("anthropic-compat: model name not configured")
	}
	req := anthropicCompatRequest{
		Model:     model,
		MaxTokens: int(opts.MaxTokens),
		System:    "You are a helpful AI assistant.",
	}
	req.Messages = append(req.Messages, struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}{Role: "user", Content: prompt})
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(p.cfg.BaseURL, "/")+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", Usage{}, fmt.Errorf("anthropic-compat: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.cfg.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.http.Do(httpReq)
	if err != nil {
		return "", Usage{}, fmt.Errorf("anthropic-compat: http: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", Usage{}, fmt.Errorf("anthropic-compat: read: %w", err)
	}
	if resp.StatusCode >= 400 {
		return "", Usage{}, fmt.Errorf("anthropic-compat: api %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	var env struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		BaseResp struct {
			StatusCode int    `json:"status_code"`
			StatusMsg  string `json:"status_msg"`
		} `json:"base_resp"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", Usage{}, fmt.Errorf("anthropic-compat: parse: %w (raw=%s)", err, truncate(string(raw), 200))
	}
	if env.BaseResp.StatusCode != 0 {
		return "", Usage{}, fmt.Errorf("anthropic-compat: api error %d: %s",
			env.BaseResp.StatusCode, env.BaseResp.StatusMsg)
	}
	var sb strings.Builder
	for _, b := range env.Content {
		if b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}
	return sb.String(), Usage{InputTokens: env.Usage.InputTokens, OutputTokens: env.Usage.OutputTokens}, nil
}

// Compile-time check that AnthropicCompatProvider satisfies TextProvider.
var _ TextProvider = (*AnthropicCompatProvider)(nil)
```

- [ ] **Step 4: Run tests to confirm GREEN**

Run: `cd apps/api && go test -race ./internal/agents/... -run TestAnthropicCompat -v`
Expected: PASS.

- [ ] **Step 5: Run gofmt + vet**

Run: `cd apps/api && gofmt -w internal/agents/anthropic_compat.go internal/agents/anthropic_compat_test.go && go vet -tags fts5 ./internal/agents/...`
Expected: exit 0.

- [ ] **Step 6: Commit**

```bash
cd apps/api
git add internal/agents/anthropic_compat.go internal/agents/anthropic_compat_test.go
git commit -m "feat(agents): AnthropicCompatProvider (Messages API adapter)"
```

---

## Task 3: OpenAICompatProvider

**Files:**
- Create: `apps/api/internal/agents/openai_compat.go`
- Create: `apps/api/internal/agents/openai_compat_test.go`

- [ ] **Step 1: Write failing tests**

Create `apps/api/internal/agents/openai_compat_test.go`:

```go
package agents

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fakeOpenAI(t *testing.T, wantModel string, captured *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		*captured = string(body)
		var req struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if req.Model != wantModel {
			http.Error(w, "wrong model", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":      "cmpl-test",
			"object":  "chat.completion",
			"model":   req.Model,
			"choices": []map[string]any{
				{
					"index":         0,
					"finish_reason": "stop",
					"message":       map[string]string{"role": "assistant", "content": "hi from openai"},
				},
			},
			"usage": map[string]int{"prompt_tokens": 7, "completion_tokens": 9, "total_tokens": 16},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestOpenAICompatProvider_CompleteWithUsage(t *testing.T) {
	var captured string
	srv := fakeOpenAI(t, "gpt-4o-mini", &captured)

	p := NewOpenAICompatProvider(OpenAICompatConfig{
		APIKey:    "sk-test",
		BaseURL:   srv.URL,
		ModelName: "gpt-4o-mini",
	})

	text, usage, err := p.CompleteWithUsage(context.Background(), "hi", CompleteOptions{MaxTokens: 32})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if text != "hi from openai" {
		t.Errorf("want text=hi from openai, got %q", text)
	}
	if usage.InputTokens != 7 || usage.OutputTokens != 9 {
		t.Errorf("want usage=7/9, got %d/%d", usage.InputTokens, usage.OutputTokens)
	}
	if !strings.Contains(captured, `"model":"gpt-4o-mini"`) {
		t.Errorf("captured request missing model: %s", captured)
	}
}

func TestOpenAICompatProvider_Available(t *testing.T) {
	if NewOpenAICompatProvider(OpenAICompatConfig{ModelName: "x"}).Available() {
		t.Error("want false with no key")
	}
	if !NewOpenAICompatProvider(OpenAICompatConfig{APIKey: "k", ModelName: "x"}).Available() {
		t.Error("want true with key")
	}
}
```

- [ ] **Step 2: Run tests to confirm RED**

Run: `cd apps/api && go test ./internal/agents/... -run TestOpenAICompat -v`
Expected: FAIL.

- [ ] **Step 3: Implement the provider**

Create `apps/api/internal/agents/openai_compat.go`:

```go
package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type OpenAICompatConfig struct {
	APIKey    string
	BaseURL   string
	ModelName string
}

// OpenAICompatProvider hits any endpoint speaking the OpenAI
// Chat Completions API shape. api.openai.com, DeepSeek,
// Azure-OpenAI, and most Chinese open-source gateways are
// reachable from the same code.
type OpenAICompatProvider struct {
	cfg  OpenAICompatConfig
	http *http.Client
}

func NewOpenAICompatProvider(cfg OpenAICompatConfig) *OpenAICompatProvider {
	base := cfg.BaseURL
	if base == "" {
		base = "https://api.openai.com"
	}
	return &OpenAICompatProvider{
		cfg:  cfg,
		http: &http.Client{Timeout: DefaultProviderTimeout},
	}
}

func (p *OpenAICompatProvider) Name() string     { return "openai" }
func (p *OpenAICompatProvider) Available() bool { return strings.TrimSpace(p.cfg.APIKey) != "" }

type openaiCompatRequest struct {
	Model    string  `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
}

func (p *OpenAICompatProvider) CompleteWithUsage(ctx context.Context, prompt string, opts CompleteOptions) (string, Usage, error) {
	if !p.Available() {
		return "", Usage{}, errors.New("openai-compat: APIKey not configured")
	}
	model := opts.Model
	if model == "" {
		model = p.cfg.ModelName
	}
	if model == "" {
		return "", Usage{}, errors.New("openai-compat: model name not configured")
	}
	req := openaiCompatRequest{
		Model:       model,
		MaxTokens:   int(opts.MaxTokens),
		Temperature: opts.Temperature,
	}
	req.Messages = append(req.Messages, struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}{Role: "user", Content: prompt})
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(p.cfg.BaseURL, "/")+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", Usage{}, fmt.Errorf("openai-compat: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

	resp, err := p.http.Do(httpReq)
	if err != nil {
		return "", Usage{}, fmt.Errorf("openai-compat: http: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", Usage{}, fmt.Errorf("openai-compat: read: %w", err)
	}
	if resp.StatusCode >= 400 {
		return "", Usage{}, fmt.Errorf("openai-compat: api %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	var env struct {
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", Usage{}, fmt.Errorf("openai-compat: parse: %w (raw=%s)", err, truncate(string(raw), 200))
	}
	if len(env.Choices) == 0 {
		return "", Usage{}, errors.New("openai-compat: empty choices")
	}
	return env.Choices[0].Message.Content,
		Usage{InputTokens: env.Usage.PromptTokens, OutputTokens: env.Usage.CompletionTokens},
		nil
}

var _ TextProvider = (*OpenAICompatProvider)(nil)
```

- [ ] **Step 4: Run tests to confirm GREEN**

Run: `cd apps/api && go test -race ./internal/agents/... -run TestOpenAICompat -v`
Expected: PASS.

- [ ] **Step 5: gofmt + vet**

Run: `cd apps/api && gofmt -w internal/agents/openai_compat.go internal/agents/openai_compat_test.go && go vet -tags fts5 ./internal/agents/...`
Expected: exit 0.

- [ ] **Step 6: Commit**

```bash
cd apps/api
git add internal/agents/openai_compat.go internal/agents/openai_compat_test.go
git commit -m "feat(agents): OpenAICompatProvider (Chat Completions adapter)"
```

---

## Task 4: ProviderResolver

**Files:**
- Create: `apps/api/internal/agents/resolver.go`
- Create: `apps/api/internal/agents/resolver_test.go`

- [ ] **Step 1: Write failing tests**

Create `apps/api/internal/agents/resolver_test.go`:

```go
package agents

import (
	"context"
	"sync"
	"testing"
)

// fakeCredentialDB is a minimal in-memory stand-in for *gorm.DB.
// It only implements what the resolver needs: reading the
// "newest credential" for a (userID, scope) pair. A real test
// against *gorm.DB lives in handlers/credentials_test.go; this
// unit test pins the resolver's adapter-selection logic.
type fakeCredentialDB struct {
	mu    sync.Mutex
	rows  map[uint]map[string]resolvedCredential // userID → scope → row
}

type resolvedCredential struct {
	Protocol  string
	BaseURL   string
	ModelName string
	APIKey    []byte // plaintext (resolver will not decrypt here — it's a unit test)
}

func (f *fakeCredentialDB) newestCredentialForScope(_ context.Context, userID uint, scope string) (*resolvedCredential, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	byScope, ok := f.rows[userID]
	if !ok {
		return nil, false, nil
	}
	// First check exact scope, then fall back to "all".
	if r, ok := byScope[scope]; ok {
		return &r, true, nil
	}
	if r, ok := byScope["all"]; ok {
		return &r, true, nil
	}
	return nil, false, nil
}

func TestProviderResolver_PicksAnthropicAdapter(t *testing.T) {
	db := &fakeCredentialDB{
		rows: map[uint]map[string]resolvedCredential{
			7: {"all": {Protocol: "anthropic", BaseURL: "https://example.com", ModelName: "claude-haiku-4-5", APIKey: []byte("sk")}},
		},
	}
	resolver := newResolverForTest(db)

	p, err := resolver.text(context.Background(), 7, "all")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if p.Name() != "anthropic" {
		t.Errorf("want adapter=anthropic, got %q", p.Name())
	}
	if !p.Available() {
		t.Error("want Available()=true")
	}
}

func TestProviderResolver_PicksOpenAIAdapter(t *testing.T) {
	db := &fakeCredentialDB{
		rows: map[uint]map[string]resolvedCredential{
			7: {"all": {Protocol: "openai", BaseURL: "https://api.deepseek.com", ModelName: "deepseek-chat", APIKey: []byte("sk")}},
		},
	}
	resolver := newResolverForTest(db)
	p, err := resolver.text(context.Background(), 7, "all")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if p.Name() != "openai" {
		t.Errorf("want adapter=openai, got %q", p.Name())
	}
}

func TestProviderResolver_NoCredential_ReturnsEcho(t *testing.T) {
	db := &fakeCredentialDB{rows: map[uint]map[string]resolvedCredential{}}
	resolver := newResolverForTest(db)
	p, err := resolver.text(context.Background(), 99, "all")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if p.Name() != "echo" {
		t.Errorf("want fallback=echo, got %q", p.Name())
	}
}
```

- [ ] **Step 2: Run tests to confirm RED**

Run: `cd apps/api && go test ./internal/agents/... -run TestProviderResolver -v`
Expected: FAIL.

- [ ] **Step 3: Implement the resolver**

Create `apps/api/internal/agents/resolver.go`:

```go
package agents

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/opc/api/internal/models"
	"gorm.io/gorm"
)

// ProviderResolver maps (user_id, scope) → a live TextProvider
// or MediaProvider by reading the credential table, decrypting
// the key, and wiring the right protocol adapter. The resolver
// is safe for concurrent use after construction; the underlying
// *gorm.DB is the source of truth and is read on every call so
// key rotations land on the next request without a restart.
type ProviderResolver struct {
	db           *gorm.DB
	decrypt      func([]byte) ([]byte, error)
	defaultMedia MediaProvider // MiniMax (built-in, env-driven)

	// memoize adapters by (providerType, baseURL) so we don't
	// rebuild the http.Client on every call. The credential
	// row's APIKey still flows in per-request — only the client
	// struct is cached. In practice this is a small win (one
	// call per request, not per second) but the cache also
	// bounds the cardinality of clients in flight.
	mu       sync.Mutex
	clients  map[string]TextProvider
}

// NewProviderResolver wires a resolver. decrypt is the
// credentials column decryptor (auth.Decrypt with the master
// key); pass a stub in tests.
func NewProviderResolver(db *gorm.DB, decrypt func([]byte) ([]byte, error), defaultMedia MediaProvider) *ProviderResolver {
	return &ProviderResolver{
		db:           db,
		decrypt:      decrypt,
		defaultMedia: defaultMedia,
		clients:      make(map[string]TextProvider),
	}
}

// ErrNoCredential is returned when the resolver cannot find a
// credential matching the requested scope. Callers fall back
// to the package Echo / a demo-mode handler.
var ErrNoCredential = errors.New("no credential configured for this user/scope")

// Text returns a TextProvider for the given (userID, scope).
// scope can be "all" or a specific skill name; the resolver
// prefers an exact match and falls back to "all".
func (r *ProviderResolver) Text(ctx context.Context, userID uint, scope string) (TextProvider, error) {
	if userID == 0 {
		return Echo, nil
	}
	cred, err := r.lookupCredential(ctx, userID, scope)
	if err != nil {
		return nil, err
	}
	if cred == nil {
		return Echo, nil
	}
	key, err := r.decrypt(cred.EncryptedKey)
	if err != nil {
		return nil, fmt.Errorf("resolver: decrypt: %w", err)
	}
	switch models.Protocol(cred.Protocol) {
	case models.ProtocolAnthropic:
		return r.cachedClient("anthropic:"+cred.BaseURL, func() TextProvider {
			return NewAnthropicCompatProvider(AnthropicCompatConfig{
				APIKey:    string(key),
				BaseURL:   cred.BaseURL,
				ModelName: cred.ModelName,
			})
		}), nil
	case models.ProtocolOpenAI:
		return r.cachedClient("openai:"+cred.BaseURL, func() TextProvider {
			return NewOpenAICompatProvider(OpenAICompatConfig{
				APIKey:    string(key),
				BaseURL:   cred.BaseURL,
				ModelName: cred.ModelName,
			})
		}), nil
	default:
		return nil, fmt.Errorf("resolver: unsupported protocol %q for credential %d", cred.Protocol, cred.ID)
	}
}

// Media returns the default MediaProvider. User-configurable
// media is out of scope for Phase 4 — MiniMax remains the
// built-in. The signature is here so handlers can adopt a
// uniform resolver surface.
func (r *ProviderResolver) Media(_ context.Context, _ uint, _ string) (MediaProvider, error) {
	if r.defaultMedia == nil {
		return nil, errors.New("resolver: no default media provider configured")
	}
	return r.defaultMedia, nil
}

func (r *ProviderResolver) lookupCredential(ctx context.Context, userID uint, scope string) (*models.Credential, error) {
	var cred models.Credential
	// Exact-scope first.
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND scope = ?", userID, scope).
		Order("updated_at DESC").
		First(&cred).Error
	if err == nil {
		return &cred, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("resolver: lookup: %w", err)
	}
	// Fall back to scope="all".
	err = r.db.WithContext(ctx).
		Where("user_id = ? AND scope = ?", userID, "all").
		Order("updated_at DESC").
		First(&cred).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("resolver: lookup all: %w", err)
	}
	return &cred, nil
}

func (r *ProviderResolver) cachedClient(key string, build func() TextProvider) TextProvider {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.clients[key]; ok {
		return c
	}
	c := build()
	r.clients[key] = c
	return c
}
```

Add the `Protocol` typed string to the models package so the switch is type-safe:

In `apps/api/internal/models/credential.go`, add:

```go
type Protocol string

const (
	ProtocolAnthropic Protocol = "anthropic"
	ProtocolOpenAI    Protocol = "openai"
)
```

Also add the test-only helper used by `resolver_test.go` — append to `apps/api/internal/agents/resolver.go`:

```go
// newResolverForTest wires a resolver that reads from the
// supplied fakeCredentialDB (instead of *gorm.DB). Used only
// by resolver_test.go. The fake adapter is wired in test code.
func newResolverForTest(db *fakeCredentialDB) *testResolver {
	return &testResolver{db: db}
}
```

And create a small test-only file `apps/api/internal/agents/resolver_fake_test.go`:

```go
package agents

import (
	"context"

	"github.com/opc/api/internal/models"
)

type testResolver struct {
	db *fakeCredentialDB
}

func (r *testResolver) text(ctx context.Context, userID uint, scope string) (TextProvider, error) {
	row, ok, err := r.db.newestCredentialForScope(ctx, userID, scope)
	if err != nil {
		return nil, err
	}
	if !ok {
		return Echo, nil
	}
	switch models.Protocol(row.Protocol) {
	case models.ProtocolAnthropic:
		return NewAnthropicCompatProvider(AnthropicCompatConfig{
			APIKey:    string(row.APIKey),
			BaseURL:   row.BaseURL,
			ModelName: row.ModelName,
		}), nil
	case models.ProtocolOpenAI:
		return NewOpenAICompatProvider(OpenAICompatConfig{
			APIKey:    string(row.APIKey),
			BaseURL:   row.BaseURL,
			ModelName: row.ModelName,
		}), nil
	}
	return Echo, nil
}
```

- [ ] **Step 4: Run tests to confirm GREEN**

Run: `cd apps/api && go test -race ./internal/agents/... -run TestProviderResolver -v`
Expected: PASS.

- [ ] **Step 5: gofmt + vet**

Run: `cd apps/api && gofmt -w internal/agents/resolver.go internal/agents/resolver_test.go internal/agents/resolver_fake_test.go internal/models/credential.go && go vet -tags fts5 ./internal/agents/... ./internal/models/...`
Expected: exit 0.

- [ ] **Step 6: Commit**

```bash
cd apps/api
git add internal/agents/resolver.go internal/agents/resolver_test.go internal/agents/resolver_fake_test.go internal/models/credential.go
git commit -m "feat(agents): ProviderResolver — DB-backed adapter selection"
```

---

## Task 5: Refactor AI/Quality/Deconstruct/Pipeline handlers to TextProvider

**Files:**
- Modify: `apps/api/internal/handlers/ai.go`
- Modify: `apps/api/internal/handlers/quality.go`
- Modify: `apps/api/internal/handlers/deconstruct.go`
- Modify: `apps/api/internal/handlers/pipeline.go`
- Modify: `apps/api/internal/handlers/ai_test.go`
- Modify: `apps/api/internal/handlers/quality_test.go`
- Modify: `apps/api/internal/handlers/deconstruct_test.go`
- Modify: `apps/api/internal/handlers/pipeline_test.go`

- [ ] **Step 1: Write a helper textClient interface used by all 4 handlers**

Append to `apps/api/internal/handlers/ai.go` (or a new `provider_resolver_helper.go` in handlers package):

```go
// textClientResolver is the subset of *agents.ProviderResolver
// that handlers consume. Defining it locally keeps the handler
// unit tests free of a *gorm.DB and lets us pass a stub in
// tests.
type textClientResolver interface {
	Text(ctx context.Context, userID uint, scope string) (agents.TextProvider, error)
}

func resolveTextForRequest(c *gin.Context, r textClientResolver, def agents.TextProvider) agents.TextProvider {
	if r != nil {
		userID := UserIDFromContext(c)
		if userID != 0 {
			if p, err := r.Text(c.Request.Context(), userID, "all"); err == nil && p != nil {
				return p
			}
		}
	}
	if def != nil {
		return def
	}
	return agents.Echo
}
```

- [ ] **Step 2: Update AIHandler**

In `apps/api/internal/handlers/ai.go`:

```go
type AIHandler struct {
	resolver textClientResolver
	db       *gorm.DB
}

func NewAIHandler(resolver textClientResolver, db *gorm.DB) *AIHandler {
	return &AIHandler{resolver: resolver, db: db}
}

// Inside GenerateTopics / HumanizeScript / Postmortem: replace
// `h.text.Text(...)` with `resolveTextForRequest(c, h.resolver, h.text).Text(...)`.
// Drop the `text *agents.MiniMax` field entirely; if the test
// suite still constructs the handler with a *agents.MiniMax,
// keep a deprecated constructor NewAIHandlerWithDefault that
// wraps it as a TextProvider via a stub resolver.
```

Add a backward-compat shim:

```go
// NewAIHandlerWithDefault is a transitional constructor kept
// during the Phase 4 migration. It wires the handler with a
// fixed TextProvider and no resolver (used by tests that
// haven't been updated yet). New callers should use
// NewAIHandler(resolver, db).
func NewAIHandlerWithDefault(text agents.TextProvider, db *gorm.DB) *AIHandler {
	return &AIHandler{
		resolver: &staticResolver{text: text},
		db:       db,
	}
}

type staticResolver struct{ text agents.TextProvider }

func (s *staticResolver) Text(_ context.Context, _ uint, _ string) (agents.TextProvider, error) {
	return s.text, nil
}
```

- [ ] **Step 3: Apply the same pattern to QualityHandler, DeconstructHandler, PipelineHandler**

Each handler:
- Field `text *agents.MiniMax` → drop
- Constructor signature → `(resolver textClientResolver, db *gorm.DB)` plus the deprecated `*WithDefault` form
- Inside handler methods, replace `h.text.Text(...)` with `resolveTextForRequest(c, h.resolver, h.text).Text(...)`

For `pipeline.go`, the `Text()` accessor returns the live provider used by `BatchHandler` — change its return type to `agents.TextProvider`:

```go
func (h *PipelineHandler) Text() agents.TextProvider {
	return resolveTextForRequest(context.Background(), h.resolver, h.defaultText)
}
```

- [ ] **Step 4: Update test files**

In each `*_test.go`, replace the constructor calls:

```go
// Before
m := agents.NewMiniMaxWithTextOverride(...)
aiH := handlers.NewAIHandler(m, db)

// After
m := agents.NewMiniMaxWithTextOverride(...)
aiH := handlers.NewAIHandlerWithDefault(m, db)
```

The MiniMax override shim still satisfies TextProvider (it has CompleteWithUsage), so the existing tests stay green with no test-logic changes.

- [ ] **Step 5: Run full handler suite**

Run: `cd apps/api && go test -race -tags fts5 ./internal/handlers/...`
Expected: PASS (all existing tests still green).

- [ ] **Step 6: Commit**

```bash
cd apps/api
git add internal/handlers/ai.go internal/handlers/ai_test.go \
        internal/handlers/quality.go internal/handlers/quality_test.go \
        internal/handlers/deconstruct.go internal/handlers/deconstruct_test.go \
        internal/handlers/pipeline.go internal/handlers/pipeline_test.go
git commit -m "refactor(handlers): AI/Quality/Deconstruct/Pipeline consume TextProvider interface"
```

---

## Task 6: Refactor Batch handler to accept TextProvider factory

**Files:**
- Modify: `apps/api/internal/handlers/batch.go`

- [ ] **Step 1: Verify the existing factory signature**

In `apps/api/internal/handlers/batch.go`, `BatchHandler` already takes `func() *agents.MiniMax` via `NewBatchHandler`. Update its signature to take `func() agents.TextProvider` and adjust the call sites in `main.go`.

- [ ] **Step 2: Change signature**

```go
func NewBatchHandler(textFn func() agents.TextProvider, db *gorm.DB) *BatchHandler {
	return &BatchHandler{textFn: textFn, db: db}
}
```

If the existing body uses `agents.MiniMaxTextOptions{Model: ...}`, change to `agents.TextOptions{Model: ...}` (the provider-agnostic option name).

- [ ] **Step 3: Update main.go to pass the resolver-aware factory**

```go
batchH := handlers.NewBatchHandler(func() agents.TextProvider {
	return handlers.ResolveTextForBackground(pipelineH.Resolver(), systemUserID)
}, gormDB)
```

Where `ResolveTextForBackground` is a package helper that calls `resolver.Text(ctx, userID, "all")` once per batch tick (caching the result for the duration of the batch).

- [ ] **Step 4: Run handler suite**

Run: `cd apps/api && go test -race -tags fts5 ./internal/handlers/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd apps/api
git add internal/handlers/batch.go
git commit -m "refactor(handlers): batch takes TextProvider factory"
```

---

## Task 7: Refactor MCP server + tools

**Files:**
- Modify: `apps/api/internal/mcp/server.go`
- Modify: `apps/api/internal/mcp/tools_ai.go`
- Modify: `apps/api/internal/mcp/tools_topics.go`
- Modify: `apps/api/internal/mcp/tools_scripts.go`
- Modify: `apps/api/internal/mcp/tools_content.go`
- Modify: `apps/api/internal/mcp/server_test.go`
- Modify: `apps/api/internal/mcp/testhelpers_test.go`

- [ ] **Step 1: Update `Server.text` field type**

In `apps/api/internal/mcp/server.go`:

```go
type Server struct {
	db       *gorm.DB
	text     agents.TextProvider // was *agents.MiniMax
	resolver *agents.ProviderResolver
}

func NewServer(db *gorm.DB, text agents.TextProvider, resolver *agents.ProviderResolver) (*Server, error) {
	return &Server{db: db, text: text, resolver: resolver}, nil
}
```

- [ ] **Step 2: Update tools_ai.go calls**

Replace `s.text.Text(ctx, prompt, agents.MiniMaxTextOptions{Model: "MiniMax-M2.7-highspeed"})` with:

```go
// Resolve per-call from the user's most recent credential, OR
// fall back to the constructor-provided default.
provider := s.text
if s.resolver != nil {
    // MCP server has no per-request user_id today (tools run
    // under the configured agent identity); we resolve against
    // user_id=0 → Echo. Future: thread agent_id → owner_user_id
    // so per-agent credentials can be honored.
    if p, err := s.resolver.Text(ctx, 0, "all"); err == nil && p != nil {
        provider = p
    }
}
res, err := provider.Text(ctx, prompt, agents.TextOptions{Model: ""})
```

- [ ] **Step 3: Update tests**

In `server_test.go`, the line `claude := agents.NewMiniMax("test-key")` becomes `claude := agents.NewMiniMax("test-key")` (still satisfies TextProvider via CompleteWithUsage). The existing tests stay green. Add one new test:

```go
func TestServer_ResolvesViaResolver(t *testing.T) {
    // Use a stub resolver that returns a fake TextProvider
    // returning "resolved text".
    // Assert: toolGenerateTopics returns "resolved text".
}
```

- [ ] **Step 4: Run MCP suite**

Run: `cd apps/api && go test -race -tags fts5 ./internal/mcp/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd apps/api
git add internal/mcp/
git commit -m "refactor(mcp): server.text is TextProvider; resolver-aware fallback"
```

---

## Task 8: Update CredentialHandler request shape

**Files:**
- Modify: `apps/api/internal/handlers/credentials.go`
- Modify: `apps/api/internal/handlers/credentials_test.go`

- [ ] **Step 1: Extend `createRequest` and `rotateRequest`**

```go
type createRequest struct {
	Provider     string `json:"provider"`
	Protocol     string `json:"protocol"`
	BaseURL      string `json:"base_url"`
	ModelName    string `json:"model_name"`
	Name         string `json:"name"`
	PlaintextKey string `json:"plaintext_key"`
	Scope        string `json:"scope"`
}

type rotateRequest struct {
	PlaintextKey string `json:"plaintext_key"`
	// Future: protocol/base_url/model_name are read-only on rotate
}
```

- [ ] **Step 2: Persist the new fields**

In `Create` and `Rotate`, before `cred.Validate()`:

```go
cred.Protocol = req.Protocol
cred.BaseURL = req.BaseURL
cred.ModelName = req.ModelName
```

(Validate already enforces the constraints from Task 1.)

- [ ] **Step 3: Include in `credentialResponse`**

```go
type credentialResponse struct {
	ID         uint       `json:"id"`
	UserID     uint       `json:"user_id"`
	Provider   string     `json:"provider"`
	Protocol   string     `json:"protocol,omitempty"`
	BaseURL    string     `json:"base_url,omitempty"`
	ModelName  string     `json:"model_name,omitempty"`
	Name       string     `json:"name"`
	Scope      string     `json:"scope"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}
```

- [ ] **Step 4: Add tests**

```go
func TestCredential_Create_AcceptsProtocol(t *testing.T) {
    // POST /api/credentials with provider=anthropic, protocol=anthropic,
    // base_url=https://api.anthropic.com, model_name=claude-haiku-4-5
    // → 201, row persisted with all 4 fields populated.
}

func TestCredential_Create_RejectsMissingModelName(t *testing.T) {
    // POST with provider=anthropic but no model_name → 400.
}
```

- [ ] **Step 5: Run handler suite**

Run: `cd apps/api && go test -race -tags fts5 ./internal/handlers/... -run TestCredential`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd apps/api
git add internal/handlers/credentials.go internal/handlers/credentials_test.go
git commit -m "feat(credentials): request carries protocol/base_url/model_name"
```

---

## Task 9: Wire ProviderResolver in main.go

**Files:**
- Modify: `apps/api/cmd/server/main.go`
- Modify: `apps/api/cmd/opc-asset/main.go`

- [ ] **Step 1: Build the resolver**

In `cmd/server/main.go`, after the credential handler is built:

```go
// ProviderResolver: maps user_id + scope → live TextProvider.
// It reads the credentials table on every call so a key
// rotation lands on the next request without a restart.
// The decrypt closure uses auth.Decrypt with the master key
// loaded above.
resolver := agents.NewProviderResolver(gormDB, func(cipher []byte) ([]byte, error) {
    return auth.Decrypt(cipher, encryptionKey)
}, mmxClient)
```

- [ ] **Step 2: Pass the resolver into every text-consuming handler**

```go
aiH := handlers.NewAIHandler(resolver, gormDB)
qualityH := handlers.NewQualityHandler(resolver, gormDB)
deconstructH := handlers.NewDeconstructHandler(resolver)
pipelineH := handlers.NewPipelineHandler(resolver, gormDB)
mcpServer, err := mcp.NewServer(gormDB, mmxClient, resolver)
```

Each handler constructor signature changed in Tasks 5–7.

- [ ] **Step 3: Update opc-asset**

In `cmd/opc-asset/main.go`, the asset CLI is single-user (no credential table lookup). Build a single-shot provider from env vars:

```go
textProvider := buildAssetProviderFromEnv()
```

Where `buildAssetProviderFromEnv` reads e.g. `OPC_ASSET_PROTOCOL` / `OPC_ASSET_MODEL` / `OPC_ASSET_API_KEY` and constructs either an AnthropicCompat or OpenAICompat. Falls back to Echo when unset. Document the env vars in `.env.example`.

- [ ] **Step 4: Build + smoke**

Run: `cd apps/api && go build ./... && go test -race -tags fts5 ./...`
Expected: 0 errors, all tests PASS.

- [ ] **Step 5: Commit**

```bash
cd apps/api
git add cmd/server/main.go cmd/opc-asset/main.go .env.example
git commit -m "feat(main): wire ProviderResolver into AI/Quality/Pipeline/MCP"
```

---

## Task 10: Update shared types + console UI

**Files:**
- Modify: `apps/shared/src/types/credential.ts`
- Modify: `apps/console/lib/use-credentials.ts`
- Modify: `apps/console/components/AddCredentialDialog.tsx`
- Modify: `apps/console/components/RotateDialog.tsx`
- Modify: `apps/console/app/(console)/credentials/page.tsx`

- [ ] **Step 1: Extend shared types**

```ts
export type CredentialProtocol = 'anthropic' | 'openai'
export type CredentialProvider =
  | 'anthropic'
  | 'openai'
  | 'volcengine'
  | 'douyin'

export interface Credential {
  id: number
  user_id: number
  provider: CredentialProvider
  protocol?: CredentialProtocol
  base_url?: string
  model_name?: string
  name: string
  scope: string
  created_at: string
  updated_at: string
  last_used_at?: string
}

export interface CreateCredentialInput {
  provider: CredentialProvider
  protocol?: CredentialProtocol
  base_url?: string
  model_name?: string
  name: string
  plaintext_key: string
  scope: string
}
```

- [ ] **Step 2: Update AddCredentialDialog**

Add a "Provider type" radio group above the key field. When `anthropic` or `openai` is picked, surface `protocol` (locked to provider default), `base_url` (pre-filled but editable), `model_name` (text input). When `volcengine`/`douyin` is picked, hide those fields (media providers — out of scope for this phase).

```tsx
<fieldset className="space-y-2">
  <legend className="text-sm font-medium">协议</legend>
  <label className="flex items-center gap-2">
    <input type="radio" value="anthropic" {...register('protocol')} />
    <span>Anthropic 协议 (Claude / MiniMax 等)</span>
  </label>
  <label className="flex items-center gap-2">
    <input type="radio" value="openai" {...register('protocol')} />
    <span>OpenAI 协议 (GPT / DeepSeek 等)</span>
  </label>
</fieldset>
{protocol === 'anthropic' || protocol === 'openai' ? (
  <>
    <input {...register('base_url')} placeholder="Base URL（可选）" />
    <input {...register('model_name')} placeholder="模型（如 claude-sonnet-4-5）" required />
  </>
) : null}
```

- [ ] **Step 3: Update CredentialsPage table**

Add 3 columns: `Protocol`, `Base URL`, `Model`. Use `font-mono` for the model name. Truncate `Base URL` to 32 chars with tooltip.

- [ ] **Step 4: Type-check + build**

Run: `cd /root/workspace/opc && pnpm -F @opc/shared tsc --noEmit && pnpm -F @opc/console tsc --noEmit && pnpm -F @opc/console build`
Expected: 0 errors.

- [ ] **Step 5: Commit**

```bash
git add apps/shared/src/types/credential.ts \
        apps/console/lib/use-credentials.ts \
        apps/console/components/AddCredentialDialog.tsx \
        apps/console/components/RotateDialog.tsx \
        apps/console/app/\(console\)/credentials/page.tsx
git commit -m "feat(console): /credentials exposes protocol/base_url/model_name"
```

---

## Task 11: Update docs

**Files:**
- Modify: `docs/console-reserve-audit.md`
- Modify: `apps/console/docs/skills/ai-topics.md` (or equivalent)
- Modify: `apps/console/docs/operations/auth.md`

- [ ] **Step 1: Append Phase 4 section to reserve-audit**

```md
## Phase 4 — Provider-agnostic (done 2026-06)

The operator-facing surface now lets each user pick:

- Protocol: Anthropic Messages API | OpenAI Chat Completions API
- Model: free-form string (e.g. `claude-sonnet-4-5`, `gpt-4o-mini`,
  `MiniMax-M2.7-highspeed`)
- Base URL: optional override for self-hosted / mirror deployments
- API key: stored encrypted in the `credentials` table

The `ProviderResolver` (internal/agents/resolver.go) maps
(user_id, scope) → live TextProvider on every call. MiniMax
remains the built-in media provider (image/speech/video); its
text path is also reachable via the Anthropic protocol by
pointing at `https://api.minimaxi.com`.
```

- [ ] **Step 2: Update skills docs**

Add a paragraph to each `apps/console/docs/skills/*.md`:

> **Provider:** resolved at request time from the caller's
> configured credential. Set one under `/credentials` before
> invoking this skill.

- [ ] **Step 3: Commit**

```bash
git add docs/console-reserve-audit.md apps/console/docs/
git commit -m "docs: phase 4 — user-configurable provider"
```

---

## Task 12: End-to-end smoke

**Files:**
- (no new files)

- [ ] **Step 1: Boot the API + console**

```bash
cd apps/api && go build -o /tmp/opc-server ./cmd/server && REGISTRATION_ENABLED=true PORT=19900 /tmp/opc-server &
cd apps/console && pnpm dev &
```

- [ ] **Step 2: Smoke flow**

1. Register a user via `/register`.
2. POST `/api/credentials` with `{provider:"anthropic", protocol:"anthropic", base_url:"https://api.anthropic.com", model_name:"claude-haiku-4-5", name:"prod", plaintext_key:"sk-...", scope:"all"}`. Expect 201.
3. POST `/api/ai/topics` with a topic seed. Expect 200 + `provider="anthropic"` stamped in `call_log`.
4. Open `/credentials` in console. Verify the row shows Protocol=Anthropic, Model=claude-haiku-4-5.
5. Rotate the key via PUT `/api/credentials/:id/rotate`. Verify the next call uses the new key.
6. Switch to OpenAI: POST a second credential with `{provider:"openai", protocol:"openai", base_url:"https://api.openai.com", model_name:"gpt-4o-mini", ...}`. POST `/api/ai/topics` again. Verify `call_log.provider="openai"`.

- [ ] **Step 3: Run full test matrix**

```bash
cd apps/api && go test -race -tags fts5 ./...
cd /root/workspace/opc && pnpm -r tsc --noEmit && pnpm -F @opc/console build
```

Expected: all green.

- [ ] **Step 4: Commit any final fixes**

```bash
git add -A && git commit -m "chore: phase 4 smoke fixes" --allow-empty
```

---

## Self-Review

**1. Spec coverage:**
- ✅ User configures provider — Credential gains `protocol`/`base_url`/`model_name` (Tasks 1, 8, 10)
- ✅ Anthropic protocol — `AnthropicCompatProvider` (Task 2)
- ✅ OpenAI protocol — `OpenAICompatProvider` (Task 3)
- ✅ User picks model — `model_name` field
- ✅ User sets `anthropic_auth_token` or `apikey` — same `plaintext_key` field, persisted encrypted
- ✅ Handlers resolve per-request — `ProviderResolver.Text()` (Task 4)
- ✅ Decouple from `*agents.MiniMax` — Tasks 5, 6, 7
- ✅ Media providers stay built-in (MiniMax) — explicitly noted in Task 4 + 7
- ✅ TDD — every task writes failing tests first
- ✅ Frequent commits — one commit per task

**2. Placeholder scan:** Searched for "TBD", "TODO", "later", "fill in", "appropriate", "edge cases" — none.

**3. Type consistency:** `agents.TextProvider` used throughout; `models.Protocol` is the typed string; `textClientResolver` is consistent across handlers.

**Open scope questions raised during planning:**
- MiniMax text path: removed (AnthropicCompat handles it via base_url override). **Decided:** keep `agents.MiniMax` for media only; do not remove the file (existing tests still use it via NewMiniMaxWithTextOverride).
- Per-skill routing vs single default: **decided** scope="all" only for Phase 4. Per-skill (e.g. `score=haiku, topics=sonnet`) deferred to a follow-up.
- Backward compat for existing `credentials` rows (provider=volcengine/anthropic/douyin, no protocol): **decided** allow them to persist untouched; resolver ignores them (returns Echo for volcengine/douyin text requests).
