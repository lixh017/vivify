// Package costume implements the prompt builder for costume IPs
// (古装 — historical period drama characters).
package costume

import (
	"fmt"
	"strings"

	"github.com/opc/api/internal/assetgen"
)

// BuildPrompt assembles a model prompt for the profile + scene +
// outfit + asset type. Type-specific builder registered in the
// parent assetgen registry. Pure function; no IO.
func BuildPrompt(p assetgen.Profile, scene, outfit, assetType string) string {
	if p == nil {
		return ""
	}
	cp, ok := p.(*Profile)
	if !ok {
		return ""
	}
	prefix := fmt.Sprintf(
		"%s, %s, %s, 服饰 %s, 道具 %s, 站在 %s, 身穿 %s, 色调 %s, 时代准确, 无穿越, 国风质感",
		cp.Name_, cp.Era, cp.Role,
		strings.Join(cp.CostumeLayer, "+"),
		strings.Join(cp.PropKit, "+"),
		scene,
		outfit,
		strings.Join(cp.Palette, "/"),
	)
	switch assetType {
	case assetgen.TypeVideo:
		return prefix + "  --duration 5 --resolution 720p --ratio 9:16 --watermark true"
	case assetgen.TypeImage:
		return prefix + "  --ratio 9:16"
	default:
		return prefix
	}
}
