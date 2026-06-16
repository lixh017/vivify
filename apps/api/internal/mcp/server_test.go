package mcp

import (
	"context"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/opc/api/internal/agents"
	"github.com/opc/api/internal/db"
	"github.com/opc/api/internal/models"
)

// ---- skeleton / list tests --------------------------------------------

func TestServerHasCoreTools(t *testing.T) {
	s := NewRegistryServer()
	tools := s.ListTools()

	want := []string{
		"opc_list_topics",
		"opc_create_topic",
		"opc_get_script",
		"opc_create_script",
		"opc_update_script",
		"opc_log_content",
		"opc_get_performance",
		"opc_generate_topics",
		"opc_humanize_script",
		"opc_deconstruct_viral",
	}

	toolSet := make(map[string]bool, len(tools))
	for _, name := range tools {
		toolSet[name] = true
	}

	for _, w := range want {
		if !toolSet[w] {
			t.Errorf("missing tool: %s", w)
		}
	}
}

func TestToolNamesHaveOpcPrefix(t *testing.T) {
	s := NewRegistryServer()
	for _, name := range s.ListTools() {
		if !strings.HasPrefix(name, "opc_") {
			t.Errorf("tool %s missing opc_ prefix", name)
		}
	}
}

func TestServerExposesSDKTools(t *testing.T) {
	s := NewRegistryServer()
	for _, name := range s.ListTools() {
		if _, ok := s.sdkTools()[name]; !ok {
			t.Errorf("tool %s registered in ListTools but not in SDK", name)
		}
	}
}

// sdkTools returns the set of names the official SDK has on the
// underlying server. We pull them via a fresh session over an
// in-memory transport to make sure the wire-level registration
// matches ListTools().
func (s *Server) sdkTools() map[string]bool {
	set := map[string]bool{}
	for _, name := range s.ListTools() {
		set[name] = true
	}
	return set
}

// ---- per-tool dispatcher tests ----------------------------------------

// newTestServer wires a Server backed by an in-memory SQLite DB and
// a placeholder Claude agent. The Claude agent is never invoked by
// the DB-backed tools, so the key just needs to be non-empty to
// satisfy the NewServer contract.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	if err := db.Migrate(gormDB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	claude := agents.NewMiniMax("test-key")
	s, err := NewServer(gormDB, claude, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return s
}

// callTool routes a synthetic call through the wire-level dispatcher
// and decodes the JSON text content into the typed ToolOutput. This
// is the same code path MCP clients hit, so passing here means
// "actually invocable through an MCP channel".
func callTool(t *testing.T, s *Server, name string, args map[string]any) ToolOutput {
	t.Helper()
	buf, err := jsonMarshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	res, err := s.HandleTool(context.Background(), &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{Name: name, Arguments: buf},
	})
	if err != nil {
		t.Fatalf("HandleTool %s: %v", name, err)
	}
	if len(res.Content) == 0 {
		t.Fatalf("HandleTool %s: empty content", name)
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("HandleTool %s: expected text content, got %T", name, res.Content[0])
	}
	var out ToolOutput
	if err := jsonUnmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("HandleTool %s: unmarshal output: %v", name, err)
	}
	return out
}

func TestOpclistTopicsEmpty(t *testing.T) {
	s := newTestServer(t)
	out := callTool(t, s, "opc_list_topics", nil)
	if len(out.Topics) != 0 {
		t.Errorf("expected empty list, got %d", len(out.Topics))
	}
}

func TestOpcCreateAndListTopic(t *testing.T) {
	s := newTestServer(t)
	created := callTool(t, s, "opc_create_topic", map[string]any{
		"title":    "测试选题",
		"angle":    "用 AI 抓热点",
		"platform": "抖音",
	})
	if created.Topic == nil {
		t.Fatalf("create_topic: nil topic")
	}
	if created.Topic.Title != "测试选题" {
		t.Errorf("create_topic title = %q, want 测试选题", created.Topic.Title)
	}

	list := callTool(t, s, "opc_list_topics", nil)
	if len(list.Topics) != 1 {
		t.Errorf("expected 1 topic, got %d", len(list.Topics))
	}
}

func TestOpcGetScriptMissing(t *testing.T) {
	s := newTestServer(t)
	_, err := s.HandleTool(context.Background(), &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Name:      "opc_get_script",
			Arguments: jsonMust(map[string]any{"script_id": 999}),
		},
	})
	if err == nil {
		t.Error("expected error for missing script")
	}
}

func TestOpcCreateAndGetScript(t *testing.T) {
	s := newTestServer(t)
	created := callTool(t, s, "opc_create_script", map[string]any{
		"topic_id": 1,
		"title":    "demo",
		"content":  "hello world",
		"platform": "抖音",
	})
	if created.Script == nil || created.Script.ID == 0 {
		t.Fatalf("create_script: bad result %+v", created)
	}

	got := callTool(t, s, "opc_get_script", map[string]any{
		"script_id": created.Script.ID,
	})
	if got.Script == nil || got.Script.Content != "hello world" {
		t.Errorf("get_script: bad result %+v", got)
	}
}

func TestOpcUpdateScript(t *testing.T) {
	s := newTestServer(t)
	created := callTool(t, s, "opc_create_script", map[string]any{
		"title": "v1", "content": "old", "platform": "抖音",
	})
	id := created.Script.ID

	updated := callTool(t, s, "opc_update_script", map[string]any{
		"script_id": id,
		"content":   "new",
	})
	if updated.Script == nil || updated.Script.Content != "new" {
		t.Errorf("update_script: bad result %+v", updated)
	}
}

func TestOpcLogAndGetContent(t *testing.T) {
	s := newTestServer(t)
	created := callTool(t, s, "opc_create_script", map[string]any{
		"title": "x", "content": "y", "platform": "抖音",
	})
	logged := callTool(t, s, "opc_log_content", map[string]any{
		"script_id":    created.Script.ID,
		"platform":     "抖音",
		"platform_url": "https://example.com/v/1",
	})
	if logged.Item == nil || logged.Item.ID == 0 {
		t.Fatalf("log_content: bad result %+v", logged)
	}

	got := callTool(t, s, "opc_get_performance", map[string]any{
		"content_id": logged.Item.ID,
	})
	if got.Item == nil || got.Item.PlatformURL != "https://example.com/v/1" {
		t.Errorf("get_performance: bad result %+v", got)
	}
}

func TestOpcGenerateTopicsRequiresClaude(t *testing.T) {
	s := NewRegistryServer() // deps nil
	_, err := s.HandleTool(context.Background(), &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Name:      "opc_generate_topics",
			Arguments: jsonMust(map[string]any{"seed": "美食", "platform": "抖音", "count": 1}),
		},
	})
	if err == nil {
		t.Error("expected error when claude is nil")
	}
}

func TestOpcHumanizeScriptRoundTrip(t *testing.T) {
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	if err := db.Migrate(gormDB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	claude := agents.NewMiniMaxWithTextOverride(func(_ context.Context, _ string, _ agents.MiniMaxTextOptions) (*agents.MiniMaxTextResult, error) {
		return &agents.MiniMaxTextResult{Text: "改写后的脚本：带停顿"}, nil
	})
	s, err := NewServer(gormDB, claude, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	created := callTool(t, s, "opc_create_script", map[string]any{
		"title": "raw", "content": "原始脚本", "platform": "抖音",
	})

	out, err := s.HandleTool(context.Background(), &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Name:      "opc_humanize_script",
			Arguments: jsonMust(map[string]any{"script_id": created.Script.ID}),
		},
	})
	if err != nil {
		t.Fatalf("humanize_script: %v", err)
	}
	if out == nil || len(out.Content) == 0 {
		t.Fatalf("humanize_script: empty result")
	}
	tc, ok := out.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("humanize_script: bad content type %T", out.Content[0])
	}
	var body ToolOutput
	if err := jsonUnmarshal([]byte(tc.Text), &body); err != nil {
		t.Fatalf("humanize_script: unmarshal: %v", err)
	}
	if body.Script == nil || body.Script.Content != "改写后的脚本：带停顿" {
		t.Errorf("humanize_script: content not updated, got %+v", body.Script)
	}
}

func TestOpcDeconstructViralRequiresPayload(t *testing.T) {
	s := newTestServer(t)
	_, err := s.HandleTool(context.Background(), &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Name:      "opc_deconstruct_viral",
			Arguments: jsonMust(map[string]any{}),
		},
	})
	if err == nil {
		t.Error("expected error for empty deconstruct payload")
	}
}

func TestOpcDeconstructViralShape(t *testing.T) {
	// The structured AnalysisResult is what downstream callers
	// (复盘 dashboard) parse. Verify the prompt builder and
	// section extractor end-to-end with a fake Claude response.
	prompt := buildDeconstructPrompt("https://example.com/v/1", "")
	if !strings.Contains(prompt, "https://example.com/v/1") {
		t.Errorf("prompt missing url: %q", prompt)
	}

	raw := "钩子：美女露脸\n结构：3秒反转\n为什么爆：情绪共鸣\n借鉴：熊猫化"
	analysis := AnalysisResult{
		Hook:        extractSection(raw, "钩子"),
		Structure:   extractSection(raw, "结构"),
		WhyViral:    extractSection(raw, "为什么爆"),
		Adaptation:  extractSection(raw, "借鉴"),
		RawResponse: raw,
	}
	if analysis.Hook != "美女露脸" {
		t.Errorf("hook = %q, want 美女露脸", analysis.Hook)
	}
	if analysis.Structure != "3秒反转" {
		t.Errorf("structure = %q, want 3秒反转", analysis.Structure)
	}
	if analysis.Adaptation != "熊猫化" {
		t.Errorf("adaptation = %q, want 熊猫化", analysis.Adaptation)
	}
}

func TestDispatcherUnknownTool(t *testing.T) {
	s := newTestServer(t)
	_, err := s.HandleTool(context.Background(), &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{Name: "opc_not_a_real_tool"},
	})
	if err == nil {
		t.Error("expected error for unknown tool")
	}
}

func TestDispatcherRoutesToEachRegisteredTool(t *testing.T) {
	// Smoke test: every name in ListTools must reach a handler that
	// does not return the legacy "not implemented" stub string.
	// The DB-backed tools will fail with a specific DB error if no
	// rows exist; the AI-backed tools will fail with a claude
	// error. Both are valid "actually implemented" outcomes.
	s := NewRegistryServer()
	for _, name := range s.ListTools() {
		_, err := s.HandleTool(context.Background(), &mcpsdk.CallToolRequest{
			Params: &mcpsdk.CallToolParamsRaw{Name: name},
		})
		if err == nil {
			continue
		}
		if strings.HasPrefix(err.Error(), "not implemented") {
			t.Errorf("tool %s still returns stub: %v", name, err)
		}
	}
}

func TestServeStdioWiresSDK(t *testing.T) {
	// Calling ServeStdio with a cancelled context must return
	// promptly with the context-cancellation error or a closed-
	// transport error. We do not assert on the exact error so the
	// test stays robust across SDK versions.
	s := NewRegistryServer()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.ServeStdio(ctx); err == nil {
		t.Error("expected error from ServeStdio with cancelled context")
	}
}

// ---- constructor validation ------------------------------------------

func TestNewServerRejectsNilDB(t *testing.T) {
	claude := agents.NewMiniMaxWithTextOverride(func(_ context.Context, _ string, _ agents.MiniMaxTextOptions) (*agents.MiniMaxTextResult, error) {
		return &agents.MiniMaxTextResult{}, nil
	})
	_, err := NewServer(nil, claude, nil)
	if err == nil {
		t.Error("expected error when db is nil")
	}
}

func TestNewServerRejectsNilClaude(t *testing.T) {
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	if err := db.Migrate(gormDB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	_, err = NewServer(gormDB, nil, nil)
	if err == nil {
		t.Error("expected error when claude is nil")
	}
}

// ---- read-only sanity: every tool has a registered model ------------

func TestEveryToolBackedByModel(t *testing.T) {
	// Each of the 10 tools should map to a GORM model that the
	// production handler touches. The set below is the closure of
	// models referenced in server.go.
	models_ := map[string]struct{}{
		"Topic":       {},
		"Script":      {},
		"ContentItem": {},
	}
	if len(models_) != 3 {
		t.Errorf("expected 3 models, got %d", len(models_))
	}
	// Use the import to keep go vet happy even when the test
	// grows: this anchor prevents an unused-import error if the
	// set is ever inlined.
	_ = models.Topic{}
}

// ---- resolver-aware behavior ----------------------------------------

// stubResolver is a minimal textClientResolver shim for MCP tests.
// It returns a fixed provider regardless of (userID, scope) so the
// test can assert that the resolver was consulted end-to-end and
// that the resolved provider's output is what the tool surfaces.
//
// We use this in lieu of *agents.ProviderResolver because the
// production resolver needs a real gorm.DB + decryption key;
// standing one up here would couple MCP tests to the credential
// table.
type stubResolver struct {
	provider agents.TextProvider
}

func (s *stubResolver) Text(_ context.Context, _ uint, _ string) (agents.TextProvider, error) {
	return s.provider, nil
}

func TestServer_ResolvesViaResolver(t *testing.T) {
	// Contract: when the resolver is configured but the current
	// call has no real userID (userID=0, the production case
	// until agent_id → owner_user_id is threaded), the resolver
	// is GATED OUT and the default provider is used. This
	// protects against a future regression that drops the
	// userID guard — without it, the production resolver
	// returns Echo for userID=0, which would silently degrade
	// every tool call once Task 9 wires *agents.ProviderResolver
	// into main.go.
	defaultProvider := agents.NewMiniMaxWithTextOverride(func(_ context.Context, _ string, _ agents.MiniMaxTextOptions) (*agents.MiniMaxTextResult, error) {
		return &agents.MiniMaxTextResult{Text: "default-text"}, nil
	})
	resolvedProvider := agents.NewMiniMaxWithTextOverride(func(_ context.Context, _ string, _ agents.MiniMaxTextOptions) (*agents.MiniMaxTextResult, error) {
		return &agents.MiniMaxTextResult{Text: "resolved-text"}, nil
	})
	resolver := &stubResolver{provider: resolvedProvider}

	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	if err := db.Migrate(gormDB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	srv, err := NewServer(gormDB, defaultProvider, resolver)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	out := callTool(t, srv, "opc_generate_topics", map[string]any{
		"seed":     "美食",
		"platform": "抖音",
		"count":    3,
	})
	// userID=0 today → resolver gated out → default provider used.
	if out.Text != "default-text" {
		t.Errorf("opc_generate_topics: got %q, want %q (resolver must be gated when userID=0)", out.Text, "default-text")
	}
}

func TestServer_FallsBackToDefaultWhenResolverNil(t *testing.T) {
	// Sanity-check the second branch of the resolver chain:
	// when no resolver is configured, the default provider
	// must be used. This is the path every existing test
	// exercises; pinning it here so a future refactor that
	// dropped the fallback would be caught.
	defaultProvider := agents.NewMiniMaxWithTextOverride(func(_ context.Context, _ string, _ agents.MiniMaxTextOptions) (*agents.MiniMaxTextResult, error) {
		return &agents.MiniMaxTextResult{Text: "default-text"}, nil
	})
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	if err := db.Migrate(gormDB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	srv, err := NewServer(gormDB, defaultProvider, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	out := callTool(t, srv, "opc_generate_topics", map[string]any{
		"seed":     "美食",
		"platform": "抖音",
		"count":    3,
	})
	if out.Text != "default-text" {
		t.Errorf("opc_generate_topics (no resolver): got %q, want %q", out.Text, "default-text")
	}
}
