package mcp

import (
	"strings"
	"testing"

	"gorm.io/gorm"

	"github.com/opc/api/internal/agents"
)

func TestServerHasCoreTools(t *testing.T) {
	// Use a nil GORM DB and a nil Claude agent for the skeleton test.
	// Tool listing does not require real dependencies.
	var gormDB *gorm.DB
	var claudeAgent *agents.Claude

	s := NewServer(gormDB, claudeAgent)
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
	var gormDB *gorm.DB
	var claudeAgent *agents.Claude

	s := NewServer(gormDB, claudeAgent)
	for _, name := range s.ListTools() {
		if !strings.HasPrefix(name, "opc_") {
			t.Errorf("tool %s missing opc_ prefix", name)
		}
	}
}
