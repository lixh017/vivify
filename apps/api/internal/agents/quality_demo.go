package agents

import (
	"fmt"
	"strings"
)

// RuleBasedScore returns a deterministic 0-100 score breakdown for a
// piece of content based purely on simple heuristics over the
// title/script. The handler uses this when Claude is unavailable and
// the caller did not pass ?demo=true, so the API still returns a
// useful (if conservative) answer instead of 503.
//
// The output shape mirrors the QualityScoreResponse fields consumed
// by the frontend: overall/hook/structure/platform_fit, plus
// suggestions keyed off which axis scored low.
//
// This function is intentionally simple — it exists so demo and
// fallback paths are decoupled from Claude and so unit tests can
// pin its behaviour without spinning up the Anthropic SDK.
//
// NOTE: the messages produced by buildSuggestions are independent
// from the pre-baked demo pool in DemoQualityScores. An operator
// comparing the demo response (served from demo_data.go) against a
// live rule-based fallback for the same input will see different
// numbers AND different suggestion copy. That is intentional — the
// live fallback is meant to be conservative and IP-agnostic, while
// the demo pool is pre-tuned to the panda IP. If you add a new
// suggestion message, do so here; the demo pool is updated
// separately.
func RuleBasedScore(title, script, platform string) map[string]any {
	hook := scoreHook(title)
	structure := scoreStructure(script)
	fit := scorePlatformFit(title, platform)
	overall := (hook*40 + structure*35 + fit*25) / 100
	suggestions := buildSuggestions(title, script, platform, hook, structure, fit)
	rewritten := ""
	if hook < 70 && len(script) > 0 {
		rewritten = suggestRewrittenHook(title, script)
	}
	out := map[string]any{
		"overall_score":  overall,
		"hook_strength":  hook,
		"structure":      structure,
		"platform_fit":   fit,
		"suggestions":    suggestions,
		"rewritten_hook": rewritten,
	}
	return out
}

// scoreHook — 0-100. Looks at the first ~30 chars of the title for
// strong verbs, questions, numeric anchors, or quoted dialogue,
// which are the markers of a good short-form hook.
func scoreHook(title string) int {
	t := strings.TrimSpace(title)
	if t == "" {
		return 20
	}
	score := 50
	// Short, punchy titles (< 18 chars) tend to read better on
	// every platform.
	if len([]rune(t)) <= 18 {
		score += 15
	} else if len([]rune(t)) > 28 {
		score -= 15
	}
	// Has a question mark? Hooks that pose a question outperform.
	if strings.Contains(t, "？") || strings.Contains(t, "?") {
		score += 10
	}
	// Has an exclamation? Same idea.
	if strings.Contains(t, "！") || strings.Contains(t, "!") {
		score += 5
	}
	// Strong verbs that show up a lot in successful scripts.
	// We accumulate (capped) rather than break at the first match
	// so a title with several high-signal keywords — e.g.
	// "看 听 想 翻 写" — gets a stronger hook score. The cap of 3
	// prevents a single character appearing repeatedly in a
	// contrived title from running the score.
	const maxKeywordHits = 3
	const keywordBonus = 5
	keywordHits := 0
	for _, kw := range []string{"看", "听", "想", "翻", "写", "走", "陪", "等", "发现", "原来", "终于"} {
		if keywordHits >= maxKeywordHits {
			break
		}
		if strings.Contains(t, kw) {
			score += keywordBonus
			keywordHits++
		}
	}
	// Dialogue / quotation — hooks that quote something the panda
	// says outperform third-person descriptions.
	if strings.Contains(t, "“") || strings.Contains(t, "\"") || strings.Contains(t, "\"") {
		score += 10
	}
	return clampScore(score)
}

// scoreStructure — 0-100. Length sweet spot for short-form is
// 80-300 chars (one paragraph, one breath). Paragraph breaks
// (blank lines) signal intentional pacing.
func scoreStructure(script string) int {
	s := strings.TrimSpace(script)
	if s == "" {
		return 30
	}
	score := 50
	runes := len([]rune(s))
	switch {
	case runes < 30:
		score -= 25
	case runes < 80:
		score -= 5
	case runes <= 300:
		score += 20
	case runes <= 600:
		score += 5
	default:
		score -= 10
	}
	// Paragraph breaks = intentional pacing.
	breaks := strings.Count(s, "\n\n")
	if breaks >= 1 {
		score += 8
	}
	if breaks >= 3 {
		score += 4
	}
	// Pauses & white space in the body are signs the writer cares
	// about breath.
	if strings.Contains(s, "...") || strings.Contains(s, "——") {
		score += 5
	}
	return clampScore(score)
}

// scorePlatformFit — 0-100. Each platform has its own title-length
// sweet spot. Other axes (hashtags, body length) are checked by the
// handler layer, which has the full content-item context.
func scorePlatformFit(title, platform string) int {
	score := 60
	runes := len([]rune(strings.TrimSpace(title)))
	switch platform {
	case "抖音":
		// 抖音 caps visible title at ~22 chars.
		switch {
		case runes == 0:
			score = 20
		case runes <= 22:
			score += 25
		case runes <= 28:
			score += 5
		default:
			score -= 20
		}
	case "哔哩哔哩":
		// B站 accepts up to ~80 chars.
		switch {
		case runes == 0:
			score = 20
		case runes <= 80:
			score += 20
		case runes <= 100:
			score += 5
		default:
			score -= 15
		}
	case "小红书":
		// 小红书 caps visible title at ~20 chars.
		switch {
		case runes == 0:
			score = 20
		case runes <= 20:
			score += 25
		case runes <= 26:
			score += 5
		default:
			score -= 20
		}
	default:
		// Unknown platform — give a neutral score so the rest of
		// the breakdown still makes sense.
		score = 60
	}
	return clampScore(score)
}

// buildSuggestions turns the three axis scores into actionable
// suggestions. Each suggestion is shaped {category, severity,
// problem, rewrite} to match the JSON contract the upgraded
// ScoreContentPrompt asks for (see quality.go and the demo pool
// in demo_data.go). We only emit a suggestion when its
// corresponding axis actually scored low — emitting a "fix your
// hook" line on a 90-score hook would be noise.
//
// The older wire contract used {category, message, severity}; the
// field rename + split into problem/rewrite was introduced in the
// AI-quality Phase 1 upgrade. buildSuggestions now returns both
// problem and rewrite so the rule-based fallback path produces the
// same shape as the live Claude path, which means the frontend
// does not need to branch on which mode produced the response.
func buildSuggestions(title, script, platform string, hook, structure, fit int) []map[string]string {
	out := make([]map[string]string, 0, 4)
	if hook < 70 {
		sev := "medium"
		if hook < 50 {
			sev = "high"
		}
		out = append(out, map[string]string{
			"category": "hook",
			"problem":  "前 3 秒钩子偏弱,首句偏向'今天想给大家讲'等铺垫句式,缺少具体画面或对话",
			"rewrite":  "换成画面钩子:'画面: 窗边的熊猫看着雨,3 秒无台词' → 旁白第一句再开口",
			"severity": sev,
		})
	}
	if structure < 70 {
		sev := "medium"
		if structure < 50 {
			sev = "high"
		}
		out = append(out, map[string]string{
			"category": "structure",
			"problem":  "脚本结构松散,缺少换行做呼吸,结尾也没有留出情绪锚点",
			"rewrite":  "在第 2 段和第 3 段之间插入空行,结尾把口号句换成 1 句悬而未决的旁白(例:'今天...就到这里吧')",
			"severity": sev,
		})
	}
	if fit < 70 {
		sev := "low"
		if fit < 50 {
			sev = "high"
		}
		out = append(out, map[string]string{
			"category": "platform_fit",
			"problem":  "标题长度或结构与目标平台惯例不符:" + platformFitProblem(platform, len([]rune(title))),
			"rewrite":  platformFitSuggestion(platform, len([]rune(title))),
			"severity": sev,
		})
	}
	// The "script too short" suggestion shares the structure
	// category with the per-axis structure < 70 branch. A very
	// short script will trip both, producing two identical-category
	// rows in the UI. Demote the length-based one to a different
	// category ("word_count") so the frontend can render a clearer
	// two-axis diagnosis: "your structure is loose" + "your script
	// is too short to anchor it".
	if len(strings.TrimSpace(script)) < 50 {
		out = append(out, map[string]string{
			"category": "word_count",
			"problem":  "脚本字数过少(< 50),完播率可能拉满但信息密度低,长尾搜索吃不动",
			"rewrite":  "在结尾前补 2-3 句具体画面或留白,把字数推到 80-300 字区间",
			"severity": "high",
		})
	}
	return out
}

// platformFitSuggestion returns a platform-specific title-length
// hint. Kept as a separate function so the test can pin per-platform
// copy without re-running the scoring logic.
func platformFitSuggestion(platform string, runes int) string {
	switch platform {
	case "抖音":
		return "抖音标题超过 22 字会被截断,建议压缩到 22 字以内并加 1 个 emoji"
	case "哔哩哔哩":
		return "B 站标题可以更长(≤80),建议补充内容关键词便于搜索流量"
	case "小红书":
		return "小红书标题超过 20 字会折叠,建议把核心词前置并加 emoji"
	default:
		return "标题长度需根据目标平台调整,主流建议 ≤ 22 字"
	}
}

// platformFitProblem returns the "why this scored low" half of the
// platform_fit suggestion, kept separate from platformFitSuggestion
// (the "what to do about it" half) so the suggestion entry carries
// both problem and rewrite fields in the new wire contract.
func platformFitProblem(platform string, runes int) string {
	switch platform {
	case "抖音":
		return fmt.Sprintf("当前标题 %d 字,抖音显示上限 22 字,超出部分会被截断", runes)
	case "哔哩哔哩":
		return fmt.Sprintf("当前标题 %d 字,B 站虽然接受更长标题,但缺少搜索关键词,长尾流量吃不动", runes)
	case "小红书":
		return fmt.Sprintf("当前标题 %d 字,小红书显示上限 20 字,超出部分会被折叠", runes)
	default:
		return fmt.Sprintf("当前标题 %d 字,主流平台惯例 ≤ 22 字,长度需要按目标平台调整", runes)
	}
}

// suggestRewrittenHook returns a simple, deterministic rewrite of
// the original title that leans on the structural cues that
// scoreHook rewards: shorter, more concrete, and (when possible)
// quoting the panda.
func suggestRewrittenHook(title, script string) string {
	t := strings.TrimSpace(title)
	if t == "" {
		// If there is no title but there is a script, lift the
		// first non-empty line as a candidate hook.
		for _, line := range strings.Split(script, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				return trimToRuneCount(line, 22)
			}
		}
		return "窗边的熊猫,一句话都没说。"
	}
	// Quote it. Quoted dialogue is a reliable signal that the
	// script speaks directly to the viewer.
	if !strings.HasPrefix(t, "“") && !strings.HasPrefix(t, "\"") {
		t = "“" + t + "”"
	}
	return trimToRuneCount(t, 22)
}

// trimToRuneCount shortens s to at most n runes, appending an
// ellipsis when truncation actually happened. Safe for Chinese text
// (works on runes, not bytes).
func trimToRuneCount(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}

// clampScore returns a score clamped to [0, 100]. Pure function so
// it shows up in coverage as a fully-tested helper.
func clampScore(s int) int {
	if s < 0 {
		return 0
	}
	if s > 100 {
		return 100
	}
	return s
}
