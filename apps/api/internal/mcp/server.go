// Package mcp hosts the Model Context Protocol server for OPC.
//
// The server is built on the official Go MCP SDK
// (github.com/modelcontextprotocol/go-sdk) and exposes 10 tools over
// the stdio transport so that MCP-compatible clients (e.g. Claude Code,
// Code CLI) can drive the "熊猫" IP content pipeline. Handlers
// delegate to GORM for persistence and to an agents.TextProvider
// (the MiniMax client by default) for AI-assisted tasks (topic
// generation, humanization, viral deconstruction).
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

// textResolver is the subset of *agents.ProviderResolver that
// the MCP server consumes. Defining it locally lets unit tests
// pass a stub that returns a fixed provider, without standing
// up a real *gorm.DB + decryption key. The production
// *agents.ProviderResolver satisfies this interface as-is.
type textResolver interface {
	Text(ctx context.Context, userID uint, scope string) (agents.TextProvider, error)
}

// Server wraps the official Go MCP SDK server with the OPC-specific
// dependencies (GORM DB, text-generation provider, optional
// per-user resolver). It is the only public surface the rest of
// the application needs to interact with MCP.
type Server struct {
	sdk      *mcpsdk.Server
	db       *gorm.DB
	text     agents.TextProvider
	resolver textResolver
}

// NewServer wires the MCP server. text is the default provider
// used when no resolver is configured (or when userID=0). The
// resolver is consulted per-tool-call to honor the caller's
// configured credentials; it can be nil for callers that don't
// need per-user routing (e.g. tests). *agents.ProviderResolver
// satisfies the resolver slot directly.
func NewServer(db *gorm.DB, text agents.TextProvider, resolver textResolver) (*Server, error) {
	if db == nil {
		return nil, fmt.Errorf("mcp: db is required")
	}
	if text == nil {
		return nil, fmt.Errorf("mcp: text client is required")
	}
	s := &Server{db: db, text: text, resolver: resolver}
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
//
// description is surfaced to MCP clients (Claude Code, Cursor)
// as the tool's "what is this" hint — the model uses it to
// decide which tool to call. Keep descriptions short, action-
// oriented, and tell the model when NOT to use the tool (e.g.
// "does not call the LLM — use opc_generate_topics for that"
// saves a round-trip on misroutes). The 10 generic tools here
// are MCN-flavored; the 4 panda-tuned tools (panda_topic etc.)
// bake the OPC IP profile into the description so the model
// knows to use them when the user is producing panda content.
type toolSpec struct {
	name        string
	description string
	handler     func(s *Server) toolHandler
}

// toolSpecs is the canonical list of OPC tools, in the order they
// are exposed. The handler is bound at registerTools time (it needs
// a *Server receiver). Description strings are what Claude Code
// surfaces as the "what is this" tooltip; the model uses them
// to decide which tool to call, so keep them concrete.
var toolSpecs = []toolSpec{
	{
		name: "opc_list_topics",
		description: "List all topics in the OPC database, newest first. " +
			"Returns every topic regardless of platform or status. " +
			"Use this to browse what the team has queued before generating " +
			"new ones — duplicates are cheap to avoid at the prompt level " +
			"and expensive to avoid at the LLM level.",
		handler: func(s *Server) toolHandler { return s.toolListTopics },
	},
	{
		name: "opc_create_topic",
		description: "Create a new topic (a candidate content idea) with " +
			"title, angle, and platform. Does NOT generate the topic — " +
			"the caller supplies the values. For AI-generated topics, " +
			"use opc_generate_topics first and then call this with the " +
			"chosen result. Returns the created topic row including its " +
			"assigned id, which is the input to opc_create_script.",
		handler: func(s *Server) toolHandler { return s.toolCreateTopic },
	},
	{
		name: "opc_get_script",
		description: "Fetch a single script by id. Returns title, content, " +
			"platform, and timestamps. Use after opc_create_script to " +
			"verify a write, or to load a script for the " +
			"opc_humanize_script / opc_update_script round-trip.",
		handler: func(s *Server) toolHandler { return s.toolGetScript },
	},
	{
		name: "opc_create_script",
		description: "Create a script attached to a topic (the topic_id " +
			"argument). The script is the long-form text the host will " +
			"voice, animate, or post. Returns the new script id, which is " +
			"the input to opc_log_content after publishing.",
		handler: func(s *Server) toolHandler { return s.toolCreateScript },
	},
	{
		name: "opc_update_script",
		description: "Update an existing script's content (or title). " +
			"Used to apply edits from opc_humanize_script, or to record " +
			"manual review changes. Only the fields you pass are updated; " +
			"absent fields are preserved.",
		handler: func(s *Server) toolHandler { return s.toolUpdateScript },
	},
	{
		name: "opc_log_content",
		description: "Log a published content item — the script id, the " +
			"platform (抖音 / B站 / 小红书), and the public URL. " +
			"This is the bridge between 'I posted the video' and " +
			"'I want to see its performance' — opc_get_performance reads " +
			"from the rows this tool writes. Call once per actual publish, " +
			"not on draft / scheduled posts.",
		handler: func(s *Server) toolHandler { return s.toolLogContent },
	},
	{
		name: "opc_get_performance",
		description: "Fetch the performance record for a content item by " +
			"id (NOT script id — the content item id from opc_log_content). " +
			"Returns views, likes, comments, and engagement rate. " +
			"Use this to feed the postmortem flow: when a piece " +
			"performs notably (high OR low), call opc_deconstruct_viral " +
			"to extract the pattern.",
		handler: func(s *Server) toolHandler { return s.toolGetPerformance },
	},
	{
		name: "opc_generate_topics",
		description: "Generate 5 candidate topics for a given seed + " +
			"platform via the LLM. The seed is the human-provided " +
			"concept (e.g. 'panda IP, healing + edgeness, 抖音'); " +
			"the platform tunes the output voice. Returns a JSON array " +
			"of {title, angle, expected_performance, hook, pattern, " +
			"voice_tags}. The caller is expected to pick 1 and " +
			"follow up with opc_create_topic — this tool does NOT " +
			"persist anything.",
		handler: func(s *Server) toolHandler { return s.toolGenerateTopics },
	},
	{
		name: "opc_humanize_script",
		description: "Take a script id, return a rewritten version that " +
			"sounds less AI-generated. The rewrite drops 'as an AI " +
			"language model' / 'in conclusion' / bullet-list tells and " +
			"adds the kind of micro-roughness a human writer leaves in. " +
			"Returns the rewritten text — you call opc_update_script " +
			"to persist it. Skip on first-draft scripts; this is most " +
			"useful when the script is intended for a platform whose " +
			"audience can detect AI tells (B站 especially).",
		handler: func(s *Server) toolHandler { return s.toolHumanizeScript },
	},
	{
		name: "opc_deconstruct_viral",
		description: "Take a viral piece of content (by URL or raw text), " +
			"return a structured analysis of why it worked: hook, structure, " +
			"emotional mechanism, and a recommended adaptation for the " +
			"OPC IP. Use this AFTER opc_get_performance surfaces a " +
			"viral hit, or to study a competitor's video before writing " +
			"the team's next piece. The output is meant to be read, not " +
			"persisted — promote the takeaways into a new topic via " +
			"opc_generate_topics or a new script via opc_create_script.",
		handler: func(s *Server) toolHandler { return s.toolDeconstructViral },
	},
	// ---- panda-tuned tools -------------------------------------------
	// The 4 panda_* tools are the MCN-flavored opc_* tools with
	// the OPC panda IP profile (4 voice tones, 5 outfits, scene
	// whitelist, voice rules) baked into the LLM prompt. Use
	// them INSTEAD of the opc_* equivalents when producing panda
	// content. The opc_* tools stay available for non-panda work.
	{
		name: "panda_topic",
		description: "Generate 5 candidate topics for the panda IP, with the " +
			"OPC panda IP profile (4 voice tones / 5 outfits / scene whitelist) " +
			"baked into the prompt. voice is required (治愈/御宅/哲学/国潮); " +
			"hook_angle is an optional concrete situation to anchor the topics " +
			"(e.g. '凌晨 3 点独处', '陌生人善意'). platform tunes the output " +
			"format. Does NOT persist — caller picks one and uses " +
			"opc_create_topic. Prefer this over opc_generate_topics when " +
			"producing panda content; opc_generate_topics is the MCN-flavored " +
			"default that does not know about panda voices.",
		handler: func(s *Server) toolHandler { return s.toolPandaTopic },
	},
	{
		name: "panda_script",
		description: "Write a script for an existing panda topic, with the " +
			"OPC IP profile, voice tone, and duration budget baked in. " +
			"Loads the topic by id, generates the script via the LLM, " +
			"persists it, and returns the new script id. voice is " +
			"required (治愈/御宅/哲学/国潮); duration_sec defaults to 60; " +
			"platform defaults to the topic's platform. Use this INSTEAD " +
			"of opc_create_script for panda content — the LLM is told " +
			"about voice rules, anti-patterns, and 留白 endings so the " +
			"first draft is already on-voice.",
		handler: func(s *Server) toolHandler { return s.toolPandaScript },
	},
	{
		name: "panda_voice",
		description: "Read-only voice-tone audit of a script (by script_id) " +
			"or raw text. Returns a structured VoiceReport: voice_mix, " +
			"dominant_voice, scenes, outfit_references, prop_references, " +
			"ai_tells, ip_compliance (with violations), and 2-4 " +
			"recommendations. Does NOT modify the script — for rewriting, " +
			"use opc_humanize_script. Use this AFTER panda_script or " +
			"opc_create_script to verify a script is on-panda-IP before " +
			"sending it to opc_humanize_script.",
		handler: func(s *Server) toolHandler { return s.toolPandaVoice },
	},
	{
		name: "panda_storyboard",
		description: "Turn a script into a shot list with Kling / 即梦 " +
			"prompts. Loads the script by id, asks the LLM to split it " +
			"into shots, and returns the shot list (one shot per ~4s of " +
			"narration, with scene/outfit/prop/action/voiceover/kling_prompt " +
			"per shot). voice is required so the visual register matches " +
			"the script. The kling_prompt field is the 4-line technical " +
			"prompt the content lead pastes directly into the video model. " +
			"Output is returned, not persisted — caller can render it as " +
			"Markdown or save the JSON alongside the script.",
		handler: func(s *Server) toolHandler { return s.toolPandaStoryboard },
	},
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
// the single place that decides which tools are exposed. The
// Description field is what Claude Code shows the model — it is
// the tool's "what is this" hint and the model uses it to decide
// which tool to call. Keep descriptions concrete (when to use,
// when NOT to use) and short (1-3 sentences).
func (s *Server) registerTools() {
	for _, spec := range toolSpecs {
		s.sdk.AddTool(&mcpsdk.Tool{
			Name:        spec.name,
			Description: spec.description,
			InputSchema: emptyObjectSchema,
		}, s.HandleTool)
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
