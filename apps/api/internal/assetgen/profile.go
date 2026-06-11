// Package assetgen provides the opc asset-generation primitives:
// the canonical IP profiles (4 hard-boundary sub-packages under
// profiles/), the prompt builders, and the brand-consistency check.
// It is the library side of the opc-asset CLI (cmd/opc-asset) and
// any future platform-side caller that wants to generate a
// brand-on asset on demand.
//
// The package is intentionally small: a Profile interface, a
// ProfileEntry struct that bundles (schema, check, prompt builder)
// per IP type, a registry + dispatcher, and a small type system.
// Anything that requires talking to a model (text, image, video)
// lives in the agents package; assetgen is the pure-Go
// prompt+profile layer that callers compose with whatever model
// they want.
package assetgen

// AssetType distinguishes the two outputs the platform generates.
const (
	TypeImage = "image"
	TypeVideo = "video"
)

// Profile is the public interface every IP type implements.
// Type() returns the IP type discriminator ("anthropomorphic",
// "digital_human", "costume", "info") used by the registry to
// route ConsistencyCheck and BuildPrompt calls to the right
// sub-package's implementation.
//
// Implementations live in internal/assetgen/profiles/<type>/
// sub-packages; the parent package never knows about their
// concrete structs.
type Profile interface {
	Type() string
	Version() string
	Name() string
}

// ProfileEntry is the unit of registration: a concrete schema
// (which is itself a Profile), the consistency check function
// bound to it, and the prompt builder. Sub-package init() calls
// Register(<type>, entry) at startup; the parent dispatcher
// looks up by Type() and invokes the right function.
type ProfileEntry struct {
	Schema      Profile
	Check       ConsistencyFn
	BuildPrompt PromptFn
}

// ConsistencyFn scores a prompt against the profile's brand
// anchors. Returns ConsistencyResult with Score in [0, 1] and
// Fails listing missing anchor names. Pure: no IO, no model call.
type ConsistencyFn func(Profile, string) ConsistencyResult

// PromptFn assembles a model prompt for the given profile +
// scene + outfit + asset type. Pure: no IO, no model call.
type PromptFn func(Profile, string, string, string) string
