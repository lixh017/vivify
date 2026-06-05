package agents

import (
	"fmt"
	"strings"
)

// DeconstructMetadata captures optional context about the source
// video: platform, duration, and headline metrics. Keeping the
// shape on the agents side means the handler can build the prompt
// without duplicating the field set.
//
// All fields are optional — the prompt formatter degrades to "(未提供)"
// for empty values so the LLM never sees a half-filled "Platform: "
// line that confuses it.
type DeconstructMetadata struct {
	Platform    string
	DurationSec int
	Views       int
	Likes       int
}

// DeconstructPrompt builds the prompt used to ask Claude to break a
// video transcript down into the OpusClip-style structural pieces:
// hook, structure beats, CTA placement, emotional arc, reusable
// patterns, and per-platform fit notes.
//
// The prompt asks Claude to return a JSON object that mirrors the
// DeconstructResponse shape in the handler so the parser can
// unmarshal directly.
func (c *Claude) DeconstructPrompt(transcript string, meta DeconstructMetadata) string {
	return fmt.Sprintf(
		`你是 OPC 视频拆解助手(对标 OpusClip)。请把以下视频文本/字幕拆解成可复用的结构。

视频元信息:
- 平台:%s
- 时长:%s 秒
- 播放量:%s
- 点赞:%s

视频文本:
%s

请输出一个 JSON 对象,包含以下字段:
- hook: {type, text, analysis, strength}
  - type 取 'question'|'curiosity_gap'|'bold_claim'|'story'|'controversy'
  - text 是视频开头实际的钩子原句
  - analysis 解释这条钩子为什么有效(20-60 字)
  - strength 是 0-100 的整数
- structure: {pattern, beats, pacing, density}
  - pattern 取 'AIDA'|'PAS'|'3-act'|'listicle'|'story-loop'|'hook-problem-solution'
  - beats 是数组,每项 {time_pct, role, description}, role 取 'setup'|'tension'|'payoff'|'callback'
  - pacing 取 'fast'|'medium'|'slow'
  - density 是 0-100,代表每秒信息密度
- cta: {present, type, placement}
  - placement 取 'early'|'mid'|'late'
- emotional_arc: 数组,每项 {time_pct, emotion, intensity(0-100)}
- reusable_patterns: string[],创作者可以复用到自己内容上的具体模式
- platform_fit_notes: {抖音, 哔哩哔哩, 小红书} 三个键,每个都给一句平台建议
- overall_score: 0-100 的整数

只用 JSON 输出,不要加多余解释,不要 markdown 代码块。`,
		fallback(meta.Platform),
		fallbackInt(meta.DurationSec),
		fallbackInt(meta.Views),
		fallbackInt(meta.Likes),
		strings.TrimSpace(transcript),
	)
}

// ViralFormulaPrompt builds the prompt used to ask Claude to extract
// a reusable, named formula from a single video transcript. The
// formula is meant to be re-applied to OPC's panda IP, so the
// example_application field is required.
func (c *Claude) ViralFormulaPrompt(transcript string) string {
	return fmt.Sprintf(
		`你是 OPC 爆款公式提炼助手。请从以下视频文本中提炼一个可复用的"爆款公式"。

视频文本:
%s

请输出一个 JSON 对象,包含以下字段:
- formula_name: string,公式名(例:'三段式情绪共鸣')
- variables: 数组,每项 {name, example, weight},weight 取 'low'|'medium'|'high'
  - name 是公式里的变量占位符(例:theme, hook_question)
  - example 是这条视频里该变量的具体取值
- steps: string[],按顺序复述该公式的执行步骤
- example_application: string,把这个公式套用到 OPC 熊猫 IP 上的一个具体示例
- variations: string[],3-5 个可以套用同一公式的不同主题方向

只用 JSON 输出,不要加多余解释,不要 markdown 代码块。`,
		strings.TrimSpace(transcript),
	)
}

// fallback returns the input when non-empty, otherwise "(未提供)".
// Used by the deconstruct prompt formatter so empty metadata lines
// stay informative instead of trailing blanks.
func fallback(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "(未提供)"
	}
	return s
}

// fallbackInt is like fallback but for ints — a zero value is
// treated as "missing" rather than literally 0.
func fallbackInt(n int) string {
	if n <= 0 {
		return "(未提供)"
	}
	return fmt.Sprintf("%d", n)
}
