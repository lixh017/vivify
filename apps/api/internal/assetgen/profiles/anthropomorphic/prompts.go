// Package anthropomorphic implements the prompt builder for
// anthropomorphic animal IPs. Mirrors Phase 1's BuildPrompt
// output 1:1 for the fengge_v1 panda instance.
package anthropomorphic

import (
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
	ap, ok := p.(*Profile)
	if !ok {
		return ""
	}
	return buildAnthropomorphicPrompt(ap, scene, outfit, assetType)
}

func buildAnthropomorphicPrompt(p *Profile, scene, outfit, assetType string) string {
	prefix := "一只" + p.Species + "(" + p.Name_ + "), " + p.BodyShape +
		", 面部白 " + p.BodyColor +
		" 眼周黑 " + p.EyeColor +
		", 眼神" + p.EyeExpression +
		", 身穿" + outfit + "汉服" +
		", 站在" + scene +
		", 暖光氛围, 国潮 + 色彩靓丽(" + strings.Join(p.Palette, "/") +
		"撞色, 非暗色调、非水墨), 不露爪, 不攻击性姿势, 无文字" +
		", 不要 AI 生成感"

	switch assetType {
	case assetgen.TypeVideo:
		return prefix + "  --duration 5 --resolution 720p --ratio 9:16 --watermark true"
	case assetgen.TypeImage:
		return prefix + "  --ratio 9:16"
	default:
		return prefix
	}
}
