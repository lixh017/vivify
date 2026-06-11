package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/opc/api/internal/agents"
	"github.com/opc/api/internal/db"
)

// TestStdioE2EHandshake exercises the full MCP wire protocol that a real
// stdio client (Claude Code, Code CLI) would speak to this server.
//
// It is the Phase 1 DOD #3 verification: an MCP client can call the
// server's tools over the real transport pipeline. We use
// mcpsdk.NewInMemoryTransports, which gives two transports wired through
// net.Pipe() running the *same* newIOConn code path that StdioTransport
// uses (newline-delimited JSON, same frame format). The bytes that flow
// between client and server are byte-for-byte the same shape a
// stdin/stdout client would produce, so any wire-level regression
// (missing initialize, broken tool name, bad arguments encoding) shows
// up here. The only thing this test does not cover is the literal
// os.Stdin / os.Stdout file descriptors, which is what the production
// cmd/server binary does — and that binary is exercised manually when
// Claude Code connects.
//
// The test sequence mirrors what Claude Code does on connect:
//   1. Client.Connect runs the initialize handshake and the
//      notifications/initialized notification.
//   2. ListTools is the discovery step the LLM uses to learn what's
//      callable.
//   3. CallTool on opc_list_topics is the smallest invocation that
//      touches the real handler (DB read) without needing AI.
//
// If any of these steps break, Claude Code will fail to call
// generate_video (or anything else) in production.
func TestStdioE2EHandshake(t *testing.T) {
	// Build a real Server with a real in-memory SQLite DB. We do not
	// need a working MiniMax client for this test — opc_list_topics
	// is DB-only, so the placeholder key stays unused.
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	if err := db.Migrate(gormDB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	mmx := agents.NewMiniMax("test-key-not-used")

	srv, err := NewServer(gormDB, mmx)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	// Two transports, one pipe. t1 is the server side, t2 the client
	// side; the SDK requires the server to Connect first so it can
	// field the client's initialize request.
	serverTransport, clientTransport := mcpsdk.NewInMemoryTransports()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	serverSession, err := srv.SDK().Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer func() {
		_ = serverSession.Close()
	}()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{
		Name:    "opc-verify",
		Version: "0.1.0",
	}, nil)

	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() {
		_ = clientSession.Close()
	}()

	// --- 1. initialize -------------------------------------------------
	// Client.Connect already issued initialize + initialized under the
	// hood. The result is what the server told us about itself; that is
	// the same payload a real Claude Code handshake would inspect.
	initResult := clientSession.InitializeResult()
	if initResult == nil {
		t.Fatal("initialize: nil result")
	}
	if initResult.ServerInfo == nil {
		t.Fatal("initialize: nil serverInfo")
	}
	if initResult.ServerInfo.Name == "" {
		t.Error("initialize: serverInfo.name is empty")
	}
	if initResult.ServerInfo.Version == "" {
		t.Error("initialize: serverInfo.version is empty")
	}
	if !strings.EqualFold(initResult.ServerInfo.Name, "opc-mcp") {
		t.Errorf("initialize: serverInfo.name = %q, want opc-mcp", initResult.ServerInfo.Name)
	}

	// --- 2. tools/list -------------------------------------------------
	listResult, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	if len(listResult.Tools) == 0 {
		t.Fatal("tools/list: returned zero tools")
	}

	// The canonical contract is 10 opc_* tools. We require at least
	// one, but log a precise count mismatch so future regressions are
	// easy to bisect.
	if got, want := len(listResult.Tools), len(toolSpecs); got != want {
		t.Errorf("tools/list: got %d tools, want %d", got, want)
	}

	// Build a name lookup so we can assert the exact set is reachable
	// over the wire — not just that "some tools" came back.
	gotNames := make(map[string]bool, len(listResult.Tools))
	for _, tool := range listResult.Tools {
		if tool == nil || tool.Name == "" {
			t.Errorf("tools/list: nil or unnamed tool entry: %+v", tool)
			continue
		}
		gotNames[tool.Name] = true
	}
	for _, spec := range toolSpecs {
		if !gotNames[spec.name] {
			t.Errorf("tools/list: missing %s over the wire", spec.name)
		}
	}

	// --- 3. tools/call opc_create_topic then opc_list_topics ----------
	// Two-call round-trip: create a topic, then list topics and
	// verify the created row comes back over the wire. This catches
	// schema, encoding, and dispatcher regressions that a single
	// empty-list call would miss. ToolOutput.Topics is `json:"topics,
	// omitempty"`, so an empty DB legitimately omits the key — the
	// real wire contract is exercised by writing then reading.
	createResult, err := clientSession.CallTool(ctx, &mcpsdk.CallToolParams{
		Name: "opc_create_topic",
		Arguments: map[string]any{
			"title":    "AI 抓热点",
			"angle":    "用工具链选爆款",
			"platform": "抖音",
		},
	})
	if err != nil {
		t.Fatalf("tools/call opc_create_topic: %v", err)
	}
	if createResult == nil || len(createResult.Content) == 0 {
		t.Fatal("tools/call opc_create_topic: empty result")
	}
	createText, ok := createResult.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("tools/call opc_create_topic: expected TextContent, got %T", createResult.Content[0])
	}
	if !strings.Contains(createText.Text, "AI 抓热点") {
		t.Errorf("tools/call opc_create_topic: created title not echoed, got %s", createText.Text)
	}

	listTopicsResult, err := clientSession.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "opc_list_topics",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("tools/call opc_list_topics: %v", err)
	}
	if listTopicsResult == nil {
		t.Fatal("tools/call opc_list_topics: nil result")
	}
	if len(listTopicsResult.Content) == 0 {
		t.Fatal("tools/call opc_list_topics: empty content")
	}
	tc, ok := listTopicsResult.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("tools/call opc_list_topics: expected TextContent, got %T", listTopicsResult.Content[0])
	}
	if tc.Text == "" {
		t.Error("tools/call opc_list_topics: empty text payload")
	}

	// HandleTool marshals ToolOutput as JSON text. A real client (or
	// LLM) parses this; require valid JSON object with the topics
	// key now that we know the DB has a row.
	var payload map[string]any
	if err := jsonUnmarshal([]byte(tc.Text), &payload); err != nil {
		t.Fatalf("tools/call opc_list_topics: text is not valid JSON: %v\npayload: %s", err, tc.Text)
	}
	rawTopics, ok := payload["topics"]
	if !ok {
		t.Fatalf("tools/call opc_list_topics: payload missing 'topics' key, got keys: %v\npayload: %s", keysOf(payload), tc.Text)
	}
	topicList, ok := rawTopics.([]any)
	if !ok {
		t.Fatalf("tools/call opc_list_topics: 'topics' is not an array, got %T", rawTopics)
	}
	if len(topicList) == 0 {
		t.Error("tools/call opc_list_topics: expected at least one topic after create")
	}
}

// keysOf is a tiny debug helper so the failure message for a missing
// "topics" key stays readable.
func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
