package mcp

import (
	"context"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/opc/api/internal/models"
)

// toolGetScript implements opc_get_script. Loads a single script by
// its primary key.
func (s *Server) toolGetScript(ctx context.Context, in ToolInput) (*mcpsdk.CallToolResult, ToolOutput, error) {
	if s.db == nil {
		return nil, ToolOutput{}, fmt.Errorf("db not configured")
	}
	id, err := readID(in.Params, "script_id")
	if err != nil {
		return nil, ToolOutput{}, err
	}
	var sc models.Script
	if err := s.db.WithContext(ctx).First(&sc, id).Error; err != nil {
		return nil, ToolOutput{}, fmt.Errorf("get script: %w", err)
	}
	return &mcpsdk.CallToolResult{}, ToolOutput{Script: &sc}, nil
}

// toolCreateScript implements opc_create_script. Inserts a new
// script row tied to an existing topic.
func (s *Server) toolCreateScript(ctx context.Context, in ToolInput) (*mcpsdk.CallToolResult, ToolOutput, error) {
	if s.db == nil {
		return nil, ToolOutput{}, fmt.Errorf("db not configured")
	}
	var payload struct {
		TopicID  uint   `json:"topic_id"`
		Title    string `json:"title"`
		Content  string `json:"content"`
		Platform string `json:"platform"`
	}
	if err := decodeParams(in.Params, &payload); err != nil {
		return nil, ToolOutput{}, fmt.Errorf("invalid create_script payload: %w", err)
	}
	sc := models.Script{
		TopicID:  payload.TopicID,
		Title:    payload.Title,
		Content:  payload.Content,
		Platform: payload.Platform,
	}
	if err := s.db.WithContext(ctx).Create(&sc).Error; err != nil {
		return nil, ToolOutput{}, fmt.Errorf("create script: %w", err)
	}
	return &mcpsdk.CallToolResult{}, ToolOutput{Script: &sc}, nil
}

// scriptPatch is the typed PATCH shape for opc_update_script. All
// fields are pointers so the dispatcher can distinguish "absent"
// (nil) from "explicit zero" (e.g. word_count=0). This replaces the
// previous map[string]any + per-field type-assertion pattern, which
// was both fragile and duplicated the field list.
type scriptPatch struct {
	Title            *string `json:"title,omitempty"`
	Content          *string `json:"content,omitempty"`
	Platform         *string `json:"platform,omitempty"`
	StyleFingerprint *string `json:"style_fingerprint,omitempty"`
	Tags             *string `json:"tags,omitempty"`
	WordCount        *int    `json:"word_count,omitempty"`
}

// scriptPatchSetters maps every patchable field to a closure that
// applies it to a *models.Script. The closure receives the patch
// and is responsible for checking whether the field is present
// (non-nil pointer) before mutating dst. The map is the single
// source of truth for which fields opc_update_script accepts, so
// adding a new field is a one-line change in the struct above
// plus a one-line entry here.
var scriptPatchSetters = map[string]func(*models.Script, *scriptPatch){
	"title": func(s *models.Script, p *scriptPatch) {
		if p.Title != nil {
			s.Title = *p.Title
		}
	},
	"content": func(s *models.Script, p *scriptPatch) {
		if p.Content != nil {
			s.Content = *p.Content
		}
	},
	"platform": func(s *models.Script, p *scriptPatch) {
		if p.Platform != nil {
			s.Platform = *p.Platform
		}
	},
	"style_fingerprint": func(s *models.Script, p *scriptPatch) {
		if p.StyleFingerprint != nil {
			s.StyleFingerprint = *p.StyleFingerprint
		}
	},
	"tags": func(s *models.Script, p *scriptPatch) {
		if p.Tags != nil {
			s.Tags = *p.Tags
		}
	},
	"word_count": func(s *models.Script, p *scriptPatch) {
		if p.WordCount != nil {
			s.WordCount = *p.WordCount
		}
	},
}

// applyScriptPatch iterates scriptPatchSetters and runs each
// closure against dst + patch. Absent fields (nil pointer) are
// left untouched because each setter checks for nil before
// assigning.
func applyScriptPatch(dst *models.Script, patch *scriptPatch) {
	if patch == nil {
		return
	}
	for _, set := range scriptPatchSetters {
		set(dst, patch)
	}
}

// toolUpdateScript implements opc_update_script. Loads the target
// row, applies the typed patch, and saves it in a transaction so
// concurrent updates do not race.
func (s *Server) toolUpdateScript(ctx context.Context, in ToolInput) (*mcpsdk.CallToolResult, ToolOutput, error) {
	if s.db == nil {
		return nil, ToolOutput{}, fmt.Errorf("db not configured")
	}
	id, err := readID(in.Params, "script_id")
	if err != nil {
		return nil, ToolOutput{}, err
	}
	var patch scriptPatch
	if err := decodeParams(in.Params, &patch); err != nil {
		return nil, ToolOutput{}, fmt.Errorf("invalid update_script payload: %w", err)
	}
	var out *models.Script
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.Script
		if err := tx.First(&existing, id).Error; err != nil {
			return err
		}
		applyScriptPatch(&existing, &patch)
		if err := tx.Save(&existing).Error; err != nil {
			return err
		}
		out = &existing
		return nil
	})
	if err != nil {
		return nil, ToolOutput{}, fmt.Errorf("update script: %w", err)
	}
	if out == nil {
		return nil, ToolOutput{}, fmt.Errorf("update script: missing result")
	}
	return &mcpsdk.CallToolResult{}, ToolOutput{Script: out}, nil
}
