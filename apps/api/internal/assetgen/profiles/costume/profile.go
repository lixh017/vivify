// Package costume registers the costume IP type (古装 — historical
// period drama characters). M1 ships one instance: xianyun_v1
// (纤云 / Tang dynasty 侠女).
package costume

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/opc/api/internal/assetgen"
)

// Profile is the concrete schema for a costume IP (historical
// period drama character). Field names match profile.json keys
// (snake_case → Go PascalCase). The trailing-underscore fields
// (Type_, Version_, Name_) are the actual JSON field names; the
// Type/Version/Name methods return the value (Go disallows a
// field and method sharing a name).
type Profile struct {
	Version_     string   `json:"version"`
	Name_        string   `json:"name"`
	Type_        string   `json:"type"`
	Era          string   `json:"era"`
	Role         string   `json:"role"`
	CostumeLayer []string `json:"costume_layer"`
	PropKit      []string `json:"prop_kit"`
	Palette      []string `json:"palette"`
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
		panic(fmt.Sprintf("costume: failed to unmarshal profile.json: %v", err))
	}
	if defaultInstance.Type_ != "costume" {
		panic(fmt.Sprintf("costume: profile.json has type %q", defaultInstance.Type_))
	}
	assetgen.Register("costume", assetgen.ProfileEntry{
		Schema:      defaultInstance,
		Check:       Check,
		BuildPrompt: BuildPrompt,
	})
}

// DefaultInstance returns the canonical xianyun_v1 纤云 instance.
// Exported so other sub-packages (and tests) can reference the
// "official" costume profile.
func DefaultInstance() *Profile { return defaultInstance }

// SchemaJSON returns the JSON Schema for this type. Exported for
// M2's profile upload endpoint to validate user-uploaded profiles.
func SchemaJSON() []byte { return schemaJSON }
