// Package info implements the prompt builder for info IPs
// (资讯 — news/information host shows).
package info

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
	ip, ok := p.(*Profile)
	if !ok {
		return ""
	}
	prefix := fmt.Sprintf(
		"%s, %s, %s, 字幕 %s, 强调色 %s, 背景色 %s, 在 %s, 演播室专业感, 无 AI 播报感",
		ip.Name_, ip.NewsroomStyle, ip.HostPersona,
		ip.FontTone, ip.AccentColor, ip.BackgroundColor, scene,
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
