package mcp

import (
	"context"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/opc/api/internal/models"
)

// toolLogContent implements opc_log_content. Inserts a new content
// item row to record that a script was published to a platform.
func (s *Server) toolLogContent(ctx context.Context, in ToolInput) (*mcpsdk.CallToolResult, ToolOutput, error) {
	if s.db == nil {
		return nil, ToolOutput{}, fmt.Errorf("db not configured")
	}
	var payload struct {
		ScriptID    uint   `json:"script_id"`
		Platform    string `json:"platform"`
		PlatformURL string `json:"platform_url"`
	}
	if err := decodeParams(in.Params, &payload); err != nil {
		return nil, ToolOutput{}, fmt.Errorf("invalid log_content payload: %w", err)
	}
	ci := models.ContentItem{
		ScriptID:    payload.ScriptID,
		Platform:    payload.Platform,
		PlatformURL: payload.PlatformURL,
	}
	if err := s.db.WithContext(ctx).Create(&ci).Error; err != nil {
		return nil, ToolOutput{}, fmt.Errorf("log content: %w", err)
	}
	return &mcpsdk.CallToolResult{}, ToolOutput{Item: &ci}, nil
}

// toolGetPerformance implements opc_get_performance. Loads a
// single content item by its primary key so the caller can
// inspect post-publish metrics.
func (s *Server) toolGetPerformance(ctx context.Context, in ToolInput) (*mcpsdk.CallToolResult, ToolOutput, error) {
	if s.db == nil {
		return nil, ToolOutput{}, fmt.Errorf("db not configured")
	}
	id, err := readID(in.Params, "content_id")
	if err != nil {
		return nil, ToolOutput{}, err
	}
	var ci models.ContentItem
	if err := s.db.WithContext(ctx).First(&ci, id).Error; err != nil {
		return nil, ToolOutput{}, fmt.Errorf("get performance: %w", err)
	}
	return &mcpsdk.CallToolResult{}, ToolOutput{Item: &ci}, nil
}
