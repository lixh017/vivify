// Package assetgen provides the opc asset-generation primitives:
// the canonical panda IP profile, the prompt builders, and the
// brand-consistency check. It is the library side of the opc-asset
// CLI (cmd/opc-asset) and any future platform-side caller that
// wants to generate a brand-on asset on demand (e.g. an "AI
// cover" handler that prefers the canonical prompt over an
// ad-hoc user-supplied one).
//
// The package is intentionally small: a Profile struct, a prompt
// builder, a consistency checker, and a small type system. Anything
// that requires talking to a model (text, image, video) lives in
// the agents package; assetgen is the pure-Go prompt+profile layer
// that callers compose with whatever model they want.
package assetgen

// AssetType distinguishes the two outputs the platform generates.
const (
	TypeImage = "image"
	TypeVideo = "video"
)

// Profile describes a single canonical IP: the panda identity, its
// colors, palette, and the textual anchors that should appear in
// every generated prompt so the model output is on-brand.
//
// Profiles are versioned (FenggeV1, ...) so older prompts can be
// reproduced and consistency checks are anchored to a known schema.
// New profile versions should be added as new top-level vars, not
// mutate FenggeV1, so ledger entries remain interpretable across
// versions.
type Profile struct {
	Version string
	Name    string
	Species string
	Body    string
	Colors  ProfileColors
	Eyes    string
	Palette []string
}

// ProfileColors holds the two RGB anchors that the consistency
// check scores on. We keep them in a sub-struct (not a map) so the
// struct literal is greppable.
type ProfileColors struct {
	BodyWhite string
	EyeBlack  string
}

// FenggeV1 is the original opc panda IP: an adult panda named
// 峰哥 (Feng-ge). Seven visual+textual anchors; the consistency
// check (ConsistencyCheck) scores a prompt on the 5 most
// brand-critical ones (name, species, body color, eye color,
// 国潮 theme), with two minor anchors (no claws, no "AI look")
// contributing the remaining 0.10.
var FenggeV1 = Profile{
	Version: "fengge_v1",
	Name:    "峰哥",
	Species: "成年熊猫",
	Body:    "头身比 1:1.2 圆胖身材",
	Colors: ProfileColors{
		BodyWhite: "rgb(245,240,225)",
		EyeBlack:  "rgb(26,26,26)",
	},
	Eyes: "半阖带笑意, 不直视镜头",
	Palette: []string{
		"朱红#C73E1D", "暖橙#E89B45", "翠绿#3B8C5A", "宝蓝#1F5FA8", "米白#F5F0E1",
	},
}

// GetProfile returns the canonical profile for the given version
// string. Unknown versions return nil so callers can fail loudly
// rather than silently falling back to a default.
func GetProfile(version string) *Profile {
	switch version {
	case "fengge_v1", "":
		return &FenggeV1
	default:
		return nil
	}
}
