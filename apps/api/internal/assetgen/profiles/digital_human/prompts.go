// Package digital_human implements the prompt builder for digital
// human IPs (virtual anchors, virtual idols).
package digital_human

import (
	"fmt"

	"github.com/opc/api/internal/assetgen"
)

// BuildPrompt assembles a model prompt for the profile + scene +
// outfit + asset type. Type-specific builder registered in the
// parent assetgen registry. Pure function; no IO.
func BuildPrompt(p assetgen.Profile, scene, outfit, assetType string) string {
	if p == nil {
		return ""
	}
	dh, ok := p.(*Profile)
	if !ok {
		return ""
	}
	prefix := fmt.Sprintf(
		"%s, %d 岁 %s %s, %s, 肤色 %s, 发型 %s, 声线 %s, 身穿 %s, 在 %s, 演播室灯光, 写实质感, 镜头特写, 表情自然, 无恐怖谷",
		dh.Name_, dh.Age, dh.Ethnicity, dh.Gender, dh.FaceShape,
		dh.SkinTone, dh.HairStyle, dh.VoiceTone, outfit, scene,
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
