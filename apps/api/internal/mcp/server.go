// Package mcp hosts the Model Context Protocol server for OPC.
//
// The server exposes 10 tools to MCP-compatible clients (e.g. Claude Code
// CLI) so that AI agents can manage the "熊猫" IP content pipeline.
// Tool handlers will be filled in by Tasks 19-25; for now this package
// provides only the skeleton (tool list + stub handlers).
package mcp

import (
	"gorm.io/gorm"

	"github.com/opc/api/internal/agents"
)

// Server holds the dependencies required by MCP tool handlers.
// During Task 16 only the tool list is exercised; real handler logic
// arrives in Task 19+.
type Server struct {
	db     *gorm.DB
	claude *agents.Claude
	tools  []string
}

// NewServer constructs a Server. db and claude may be nil during the
// skeleton phase; handlers that need them will assert non-nil in
// their full implementations.
func NewServer(db *gorm.DB, claude *agents.Claude) *Server {
	return &Server{
		db:     db,
		claude: claude,
		tools: []string{
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
		},
	}
}

// ListTools returns the names of every tool exposed by this server.
// All names are guaranteed to start with "opc_".
func (s *Server) ListTools() []string {
	return s.tools
}

// HandleTool is a stub dispatcher used while full handlers are being
// written in Tasks 19-25. The signature is intentionally generic so
// callers can hand it an MCP-shaped payload without further branching.
func (s *Server) HandleTool(name string) string {
	return "not implemented: " + name
}
