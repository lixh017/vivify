// Package anthropomorphic registers the anthropomorphic IP type
// (拟人动物 — panda, fox, dog, ...) into the assetgen registry.
// M1 ships one instance: fengge_v1 (峰哥 / adult panda) loaded
// from profile.json via //go:embed.
package anthropomorphic

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/opc/api/internal/assetgen"
)

// Profile is the concrete schema for an anthropomorphic animal IP
// (panda, fox, dog, ...). Field names match profile.json keys
// (snake_case → Go PascalCase). The trailing-underscore fields
// (Type_, Version_, Name_) are the actual JSON field names; the
// Type/Version/Name methods return the value (Go disallows a
// field and method sharing a name).
type Profile struct {
	Version_      string   `json:"version"`
	Name_         string   `json:"name"`
	Type_         string   `json:"type"`
	Species       string   `json:"species"`
	BodyShape     string   `json:"body_shape"`
	BodyColor     string   `json:"body_color"`
	EyeColor      string   `json:"eye_color"`
	EyeExpression string   `json:"eye_expression"`
	Palette       []string `json:"palette"`
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
		panic(fmt.Sprintf("anthropomorphic: failed to unmarshal profile.json: %v", err))
	}
	if defaultInstance.Type_ != "anthropomorphic" {
		panic(fmt.Sprintf("anthropomorphic: profile.json has type %q, want anthropomorphic", defaultInstance.Type_))
	}
	assetgen.Register("anthropomorphic", assetgen.ProfileEntry{
		Schema:      defaultInstance,
		Check:       Check,
		BuildPrompt: BuildPrompt,
	})
}

// DefaultInstance returns the canonical fengge_v1 panda instance.
// Exported so other sub-packages (and tests) can reference the
// "official" anthropomorphic profile.
func DefaultInstance() *Profile { return defaultInstance }

// SchemaJSON returns the JSON Schema for this type. Exported for
// M2's profile upload endpoint to validate user-uploaded profiles.
func SchemaJSON() []byte { return schemaJSON }
