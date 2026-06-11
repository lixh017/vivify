// Package digital_human registers the digital_human IP type
// (数字人 — virtual YouTubers, virtual idols, news anchors).
// M1 ships one instance: lina_v1 (莉娜 / 28yo East-Asian female).
package digital_human

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/opc/api/internal/assetgen"
)

// Profile is the concrete schema for a digital human IP (virtual
// anchor, virtual idol, virtual YouTuber). Field names match
// profile.json keys (snake_case → Go PascalCase). The
// trailing-underscore fields (Type_, Version_, Name_) are the
// actual JSON field names; the Type/Version/Name methods return
// the value (Go disallows a field and method sharing a name).
type Profile struct {
	Version_    string `json:"version"`
	Name_       string `json:"name"`
	Type_       string `json:"type"`
	Age         int    `json:"age"`
	Gender      string `json:"gender"`
	Ethnicity   string `json:"ethnicity"`
	FaceShape   string `json:"face_shape"`
	HairStyle   string `json:"hair_style"`
	VoiceTone   string `json:"voice_tone"`
	SkinTone    string `json:"skin_tone"`
	AccentColor string `json:"accent_color"`
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
		panic(fmt.Sprintf("digital_human: failed to unmarshal profile.json: %v", err))
	}
	if defaultInstance.Type_ != "digital_human" {
		panic(fmt.Sprintf("digital_human: profile.json has type %q", defaultInstance.Type_))
	}
	assetgen.Register("digital_human", assetgen.ProfileEntry{
		Schema:      defaultInstance,
		Check:       Check,
		BuildPrompt: BuildPrompt,
	})
}

// DefaultInstance returns the canonical lina_v1 莉娜 instance.
// Exported so other sub-packages (and tests) can reference the
// "official" digital_human profile.
func DefaultInstance() *Profile { return defaultInstance }

// SchemaJSON returns the JSON Schema for this type. Exported for
// M2's profile upload endpoint to validate user-uploaded profiles.
func SchemaJSON() []byte { return schemaJSON }
