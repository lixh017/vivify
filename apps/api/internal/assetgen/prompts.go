package assetgen

import "strings"

// BuildPrompt returns a Chinese-language prompt that requests the
// profile's panda in the given scene wearing the given outfit. The
// type-specific tail carries the 5 color anchors, the anti-AI
// tokens (no claws, no AI-generation look), the "national trend"
// theme (国潮), and the technical flags (--duration/--resolution
// for video, aspect-ratio for image).
//
// The function is pure: it does not call out to any model or read
// from disk. The CLI and the platform side both use the same
// builder so every prompt is reproducible from the same
// {profile, scene, outfit, type} tuple, and consistency checks
// can be re-run on historical prompts without re-fetching anything.
func BuildPrompt(p Profile, scene, outfit, assetType string) string {
	// Shared body — the visual anchors that must appear in every
	// prompt for the model to recognize the IP.
	prefix := "一只" + p.Species + "(" + p.Name + "), " + p.Body +
		", 面部白 " + p.Colors.BodyWhite +
		" 眼周黑 " + p.Colors.EyeBlack +
		", 眼神" + p.Eyes +
		", 身穿" + outfit + "汉服" +
		", 站在" + scene +
		", 暖光氛围, 国潮 + 色彩靓丽(" + strings.Join(p.Palette, "/") +
		"撞色, 非暗色调、非水墨), 不露爪, 不攻击性姿势, 无文字"

	switch assetType {
	case TypeVideo:
		// Video model accepts technical flags inline. 5s at 720p
		// vertical 9:16 is the opc default (matches the Shorts /
		// Reels / 抖音 vertical-first feed).
		return prefix + "  --duration 5 --resolution 720p --ratio 9:16 --watermark true"
	case TypeImage:
		// Image model ignores the duration/--watermark flags but
		// benefits from a stated ratio.
		return prefix + "  --ratio 9:16"
	default:
		// Unknown type — return just the prefix. The consistency
		// check will still work (it doesn't care about the tail),
		// but the caller is responsible for handling the unknown
		// type before reaching this function.
		return prefix
	}
}
