package mcp

import (
	"encoding/json"

	"github.com/opc/api/internal/models"
)

// ToolInput is the loose, single-payload input shape every OPC tool
// accepts. JSON tags match the snake_case wire format MCP clients
// send; the dispatcher switches on Name and reads the per-tool
// fields it needs.
type ToolInput struct {
	Name   string                     `json:"-"`
	Params map[string]json.RawMessage `json:"-"`
}

// MarshalJSON flattens the per-tool parameter map back into the
// top-level JSON object so the SDK's schema-less AddTool can
// deserialize the raw arguments.
func (in ToolInput) MarshalJSON() ([]byte, error) {
	out := map[string]json.RawMessage{}
	for k, v := range in.Params {
		out[k] = v
	}
	return json.Marshal(out)
}

// UnmarshalJSON lets a single input struct cover all 10 tools by
// storing the entire argument object as Params.
func (in *ToolInput) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	return json.Unmarshal(data, &in.Params)
}

// ToolOutput is a free-form result envelope. Each tool handler
// populates the fields it needs; the JSON shape matches what the
// corresponding REST endpoint returns so callers do not have to
// branch on transport.
type ToolOutput struct {
	Topics   []models.Topic       `json:"topics,omitempty"`
	Topic    *models.Topic        `json:"topic,omitempty"`
	Script   *models.Script       `json:"script,omitempty"`
	Items    []models.ContentItem `json:"items,omitempty"`
	Item     *models.ContentItem  `json:"item,omitempty"`
	Text     string               `json:"text,omitempty"`
	Analysis *AnalysisResult      `json:"analysis,omitempty"`
}

// AnalysisResult is the structured shape returned by the viral
// deconstruction tool. It mirrors what an analyst would write on
// the "复盘" dashboard.
type AnalysisResult struct {
	Hook        string `json:"hook"`
	Structure   string `json:"structure"`
	WhyViral    string `json:"why_viral"`
	Adaptation  string `json:"adaptation"`
	RawResponse string `json:"raw_response,omitempty"`
}
