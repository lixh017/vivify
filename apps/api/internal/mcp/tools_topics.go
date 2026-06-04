package mcp

import (
	"context"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm/clause"

	"github.com/opc/api/internal/models"
)

// toolListTopics implements opc_list_topics. Returns the most
// recent topics, optionally filtered by platform / status.
func (s *Server) toolListTopics(ctx context.Context, in ToolInput) (*mcpsdk.CallToolResult, ToolOutput, error) {
	if s.db == nil {
		return nil, ToolOutput{}, fmt.Errorf("db not configured")
	}
	var filter struct {
		Platform string `json:"platform"`
		Status   string `json:"status"`
	}
	if err := decodeParams(in.Params, &filter); err != nil {
		return nil, ToolOutput{}, fmt.Errorf("invalid list_topics payload: %w", err)
	}
	q := s.db.WithContext(ctx).Model(&models.Topic{}).Order(
		clause.OrderByColumn{Column: clause.Column{Name: "created_at"}, Desc: true},
	)
	if filter.Platform != "" {
		q = q.Where("platform = ?", filter.Platform)
	}
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	var topics []models.Topic
	if err := q.Limit(defaultListLimit).Find(&topics).Error; err != nil {
		return nil, ToolOutput{}, fmt.Errorf("list topics: %w", err)
	}
	return &mcpsdk.CallToolResult{}, ToolOutput{Topics: topics}, nil
}

// toolCreateTopic implements opc_create_topic. Inserts a new topic
// row with the provided title / angle / platform.
func (s *Server) toolCreateTopic(ctx context.Context, in ToolInput) (*mcpsdk.CallToolResult, ToolOutput, error) {
	if s.db == nil {
		return nil, ToolOutput{}, fmt.Errorf("db not configured")
	}
	var payload struct {
		Title    string `json:"title"`
		Angle    string `json:"angle"`
		Platform string `json:"platform"`
	}
	if err := decodeParams(in.Params, &payload); err != nil {
		return nil, ToolOutput{}, fmt.Errorf("invalid create_topic payload: %w", err)
	}
	topic := models.Topic{
		Title:    payload.Title,
		Angle:    payload.Angle,
		Platform: payload.Platform,
	}
	if err := s.db.WithContext(ctx).Create(&topic).Error; err != nil {
		return nil, ToolOutput{}, fmt.Errorf("create topic: %w", err)
	}
	return &mcpsdk.CallToolResult{}, ToolOutput{Topic: &topic}, nil
}
