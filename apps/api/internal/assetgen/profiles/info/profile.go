// Package info registers the info IP type (资讯 — news/information
// host shows). M1 ships one instance: opc_daily_v1 (OPC 每日资讯
// / 演播室双主播).
package info

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/opc/api/internal/assetgen"
)

// Profile is the concrete schema for an info IP (news/information
// host shows). Field names match profile.json keys
// (snake_case → Go PascalCase). The trailing-underscore fields
// (Type_, Version_, Name_) are the actual JSON field names; the
// Type/Version/Name methods return the value (Go disallows a
// field and method sharing a name).
type Profile struct {
	Version_        string `json:"version"`
	Name_           string `json:"name"`
	Type_           string `json:"type"`
	NewsroomStyle   string `json:"newsroom_style"`
	HostPersona     string `json:"host_persona"`
	FontTone        string `json:"font_tone"`
	AccentColor     string `json:"accent_color"`
	BackgroundColor string `json:"background_color"`
}

// Type implements assetgen.Profile.
func (p *Profile) Type() string { return p.Type_ }

// Version implements assetgen.Profile.
func (p *Profile) Version() string { return p.Version_ }

// Name implements assetgen.Profile.
func (p *Profile) Name() string { return p.Name_ }

//go:embed profile.json
var profileJSON []byte

//go:embed schema.json
var schemaJSON []byte

// defaultInstance is loaded from profile.json at init() time.
// M1 has one instance per type; M2 may add per-version lookup.
var defaultInstance *Profile

func init() {
	if err := json.Unmarshal(profileJSON, &defaultInstance); err != nil {
		panic(fmt.Sprintf("info: failed to unmarshal profile.json: %v", err))
	}
	if defaultInstance.Type_ != "info" {
		panic(fmt.Sprintf("info: profile.json has type %q", defaultInstance.Type_))
	}
	assetgen.Register("info", assetgen.ProfileEntry{
		Schema:      defaultInstance,
		Check:       Check,
		BuildPrompt: BuildPrompt,
	})
}

// DefaultInstance returns the canonical opc_daily_v1 OPC 每日资讯
// instance. Exported so other sub-packages (and tests) can
// reference the "official" info profile.
func DefaultInstance() *Profile { return defaultInstance }

// SchemaJSON returns the JSON Schema for this type. Exported for
// M2's profile upload endpoint to validate user-uploaded profiles.
func SchemaJSON() []byte { return schemaJSON }
