// Package mcp hosts the Model Context Protocol server for OPC.
//
// The server is built on the official Go MCP SDK
// (github.com/modelcontextprotocol/go-sdk) and exposes 10 tools over
// the stdio transport so that MCP-compatible clients (e.g. Claude Code,
// Code CLI) can drive the "熊猫" IP content pipeline. Handlers
// delegate to GORM for persistence and to the agents.MiniMax agent
// for AI-assisted tasks (topic generation, humanization, viral
// deconstruction).
//
// The package is split per concern:
//
//   - server.go         core type, wire-level dispatcher, tool registry
//   - tools_topics.go   opc_list_topics, opc_create_topic
//   - tools_scripts.go  opc_get_script, opc_create_script, opc_update_script
//   - tools_content.go  opc_log_content, opc_get_performance
//   - tools_ai.go       opc_generate_topics, opc_humanize_script,
//     opc_deconstruct_viral
//   - helpers.go        shared parameter decoding and ID parsing
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/opc/api/internal/agents"
)

// toolHandler is the inner dispatcher signature used by every OPC
// tool handler. It is a method value extracted via toolHandlerFor.
type toolHandler func(ctx context.Context, in ToolInput) (*mcpsdk.CallToolResult, ToolOutput, error)

// Server wraps the official Go MCP SDK server with the OPC-specific
// dependencies (GORM DB, MiniMax client). It is the only public
// surface the rest of the application needs to interact with MCP.
type Server struct {
	sdk  *mcpsdk.Server
	db   *gorm.DB
	text *agents.MiniMax
}

// NewServer constructs a fully-configured MCP server. Both db and
// text are required: every OPC tool depends on the GORM handle, and
// the AI-backed tools (opc_generate_topics, opc_humanize_script,
// opc_deconstruct_viral) additionally need the MiniMax text client.
// Use NewRegistryServer for tests that only want to inspect the
// tool list without invoking handlers.
func NewServer(db *gorm.DB, text *agents.MiniMax) (*Server, error) {
	if db == nil {
		return nil, fmt.Errorf("mcp: db is required")
	}
	if text == nil {
		return nil, fmt.Errorf("mcp: text client is required")
	}
	s := &Server{db: db, text: text}
	s.sdk = mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "opc-mcp",
		Version: "v0.1.0",
	}, nil)
	s.registerTools()
	return s, nil
}

// NewRegistryServer constructs an MCP server whose handlers are
// wired but whose dependencies are nil. Calling HandleTool will
// surface "X not configured" errors; ListTools / SDK introspection
// still work. Intended for tests that only need to verify the
// registry shape.
func NewRegistryServer() *Server {
	s := &Server{}
	s.sdk = mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "opc-mcp",
		Version: "v0.1.0",
	}, nil)
	s.registerTools()
	return s
}

// SDK returns the underlying official SDK server. Tests use it to
// drive the server over in-memory transports; main.go is the only
// production caller.
func (s *Server) SDK() *mcpsdk.Server { return s.sdk }

// ServeStdio starts the MCP server on stdin/stdout. The call blocks
// until the client disconnects, the context is cancelled, or the
// transport reports an unrecoverable error. main.go runs this in a
// goroutine alongside the Gin HTTP server.
func (s *Server) ServeStdio(ctx context.Context) error {
	return s.sdk.Run(ctx, &mcpsdk.StdioTransport{})
}

// ---- tool registry (single source of truth) ----------------------------

// toolSpec describes a single OPC tool. The slice is iterated by
// ListTools, registerTools, and dispatch so adding a tool is a
// one-line change.
type toolSpec struct {
	name    string
	handler func(s *Server) toolHandler
}

// toolSpecs is the canonical list of OPC tools, in the order they
// are exposed. The handler is bound at registerTools time (it needs
// a *Server receiver).
var toolSpecs = []toolSpec{
	{name: "opc_list_topics", handler: func(s *Server) toolHandler { return s.toolListTopics }},
	{name: "opc_create_topic", handler: func(s *Server) toolHandler { return s.toolCreateTopic }},
	{name: "opc_get_script", handler: func(s *Server) toolHandler { return s.toolGetScript }},
	{name: "opc_create_script", handler: func(s *Server) toolHandler { return s.toolCreateScript }},
	{name: "opc_update_script", handler: func(s *Server) toolHandler { return s.toolUpdateScript }},
	{name: "opc_log_content", handler: func(s *Server) toolHandler { return s.toolLogContent }},
	{name: "opc_get_performance", handler: func(s *Server) toolHandler { return s.toolGetPerformance }},
	{name: "opc_generate_topics", handler: func(s *Server) toolHandler { return s.toolGenerateTopics }},
	{name: "opc_humanize_script", handler: func(s *Server) toolHandler { return s.toolHumanizeScript }},
	{name: "opc_deconstruct_viral", handler: func(s *Server) toolHandler { return s.toolDeconstructViral }},
}

// ListTools returns the names of every tool exposed by this server.
// All names are guaranteed to start with "opc_".
func (s *Server) ListTools() []string {
	names := make([]string, 0, len(toolSpecs))
	for _, spec := range toolSpecs {
		names = append(names, spec.name)
	}
	return names
}

// emptyObjectSchema is the permissive input schema used for tools
// that take a free-form bag of fields. The dispatcher reads the
// fields it needs and ignores the rest, which keeps the wire format
// loose for clients and lets us evolve individual tools without
// forcing a JSON-schema regen.
var emptyObjectSchema = json.RawMessage(`{"type":"object","additionalProperties":true}`)

// registerTools wires every entry in toolSpecs into the SDK. It is
// the single place that decides which tools are exposed.
func (s *Server) registerTools() {
	for _, spec := range toolSpecs {
		s.sdk.AddTool(&mcpsdk.Tool{Name: spec.name, InputSchema: emptyObjectSchema}, s.HandleTool)
	}
}

// ---- wire-level dispatcher --------------------------------------------

// HandleTool is the wire-level dispatcher registered with the SDK.
// It conforms to mcpsdk.ToolHandler (single result, no typed I/O)
// because the SDK's raw AddTool path is the only way to register
// multiple tools with one shared body. Per-tool tests bypass this
// method and call the per-tool handlers directly.
func (s *Server) HandleTool(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	var in ToolInput
	if len(req.Params.Arguments) > 0 {
		if err := in.UnmarshalJSON(req.Params.Arguments); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	in.Name = req.Params.Name

	result, out, err := s.dispatch(ctx, in)
	if err != nil {
		return nil, err
	}
	if result == nil {
		result = &mcpsdk.CallToolResult{}
	}
	if len(result.Content) == 0 {
		buf, merr := json.Marshal(out)
		if merr != nil {
			// Marshal failures are a programming error, not a
			// client error. Log so the bug is debuggable; the
			// empty result tells the caller something went wrong
			// without leaking internal details.
			log.Printf("mcp: marshal ToolOutput for %s: %v", in.Name, merr)
			return result, nil
		}
		result.Content = []mcpsdk.Content{&mcpsdk.TextContent{Text: string(buf)}}
	}
	return result, nil
}

// dispatch is the inner per-name router shared by the wire-level
// HandleTool and the per-tool unit tests. It returns the SDK
// CallToolResult, the structured ToolOutput, and any handler error.
func (s *Server) dispatch(ctx context.Context, in ToolInput) (*mcpsdk.CallToolResult, ToolOutput, error) {
	for _, spec := range toolSpecs {
		if spec.name != in.Name {
			continue
		}
		return spec.handler(s)(ctx, in)
	}
	return nil, ToolOutput{}, fmt.Errorf("unknown tool: %s", in.Name)
}
