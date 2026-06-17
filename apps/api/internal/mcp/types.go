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
	Topics     []models.Topic       `json:"topics,omitempty"`
	Topic      *models.Topic        `json:"topic,omitempty"`
	Script     *models.Script       `json:"script,omitempty"`
	Items      []models.ContentItem `json:"items,omitempty"`
	Item       *models.ContentItem  `json:"item,omitempty"`
	Text       string               `json:"text,omitempty"`
	Analysis   *AnalysisResult      `json:"analysis,omitempty"`
	Voice      *VoiceReport         `json:"voice_report,omitempty"`
	Storyboard *StoryboardReport    `json:"storyboard,omitempty"`
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

// VoiceReport is the structured shape returned by panda_voice.
// It is the read-only audit of whether a script is on-panda-IP
// (voice mix, scene whitelist, anti-AI-tells) without rewriting
// anything — for that, use opc_humanize_script.
type VoiceReport struct {
	VoiceMix         map[string]float64 `json:"voice_mix"`
	DominantVoice    string             `json:"dominant_voice"`
	Scenes           []string           `json:"scenes"`
	OutfitReferences []string           `json:"outfit_references"`
	PropReferences   []string           `json:"prop_references"`
	AITells          []string           `json:"ai_tells"`
	IPCompliance     IPCompliance       `json:"ip_compliance"`
	Recommendations  []string           `json:"recommendations"`
	RawResponse      string             `json:"raw_response,omitempty"`
}

// IPCompliance is the pass/fail block within a VoiceReport.
// Passed is true only when Violations is empty. Notes is a
// free-form explanation the model adds when something is borderline.
type IPCompliance struct {
	Passed     bool     `json:"passed"`
	Violations []string `json:"violations"`
	Notes      string   `json:"notes"`
}

// StoryboardShot is one shot in a panda_storyboard response.
// KlingPrompt is the technical prompt the content lead pastes
// directly into the Kling / 即梦 UI.
type StoryboardShot struct {
	ShotID      int    `json:"shot_id"`
	DurationSec int    `json:"duration_sec"`
	Scene       string `json:"scene"`
	Outfit      string `json:"outfit"`
	Prop        string `json:"prop"`
	Action      string `json:"action"`
	Voiceover   string `json:"voiceover"`
	KlingPrompt string `json:"kling_prompt"`
}

// StoryboardReport is the response shape of panda_storyboard.
// Shots is the shot list; Voice echoes the input voice tone so
// downstream tools (e.g. an asset generator) can re-derive the
// visual register without re-asking the caller.
type StoryboardReport struct {
	Voice string          `json:"voice"`
	Shots []StoryboardShot `json:"shots"`
}
