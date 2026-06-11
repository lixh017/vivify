package assetgen

import "strings"

// BuildPrompt returns a model prompt for the given profile +
// scene + outfit + asset type. The dispatcher routes to the
// type-specific builder registered in the registry. Unknown
// types return an empty string (callers should check).
//
// The function is pure: no IO, no model call. Same builder is
// used by opc-asset CLI and any platform-side handler.
func BuildPrompt(p Profile, scene, outfit, assetType string) string {
	if p == nil {
		return ""
	}
	entry, ok := registry[p.Type()]
	if !ok {
		return ""
	}
	return entry.BuildPrompt(p, scene, outfit, assetType)
}

// joinPalette is a public helper for sub-package prompt builders
// to format a palette slice into the standard "C1/C2/C3" form
// used in opc prompts.
func joinPalette(colors []string) string {
	return strings.Join(colors, "/")
}
