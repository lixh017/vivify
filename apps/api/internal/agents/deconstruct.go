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
// The new version:
//   - anchors the panda IP voice
//   - asks Claude to first do a 6-sample-point beat-by-beat walk
//     (with timestamps) before abstracting patterns
//   - gives a worked example for the canonical "深夜熊猫读庄子" video
//   - explicitly lists the per-platform fit output shape
//   - requires overall_score to be justified by 3-5 evidence lines
//
// Note for future prompt iterations: the long Chinese spec is
// inlined here on purpose (Claude performs better with a single
// contiguous prompt than with concatenated fragments), but the
// voice / style fragments it depends on (PandaIPVoice, CoTStepsDeconstruct,
// OutputJSONOnly) live in prompts.go — the central catalog the LLM
// team is expected to iterate on. If you need to tweak voice or
// reasoning scaffolding, edit prompts.go; reserve edits to this
// function for the task spec itself (sampling, beat shape,
// output fields).
func DeconstructPrompt(transcript string, meta DeconstructMetadata) string {
	return fmt.Sprintf(
		`%s

## 任务(对标 OpusClip 的结构化拆解)
把下面这段视频按时间线拆成可复用结构。要求:**先在脑中按 6 个采样点(0/15/35/60/85/100%%)切片**,再抽象模式,不要直接写"前 3 秒钩子+中段+结尾"这种粗线条结论。

## 视频元信息
- 平台:     %s
- 时长:     %s 秒
- 播放量:   %s
- 点赞:     %s

## 视频文本 / 字幕
%s

## 输出格式(JSON object,严格遵守)

{
  "hook": {
    "type":     "question" | "curiosity_gap" | "bold_claim" | "story" | "controversy",
    "text":     "视频开头实际的钩子原句(逐字)",
    "analysis": "这条钩子为什么有效(20-60 字,引用具体台词/画面)",
    "strength": 0-100
  },
  "structure": {
    "pattern": "AIDA" | "PAS" | "3-act" | "listicle" | "story-loop" | "hook-problem-solution",
    "beats": [
      { "time_pct": 0,  "role": "setup",    "timestamp_sec": 0,   "description": "第 0 秒的画面+旁白" },
      { "time_pct": 15, "role": "tension",  "timestamp_sec": 11,  "description": "第 15%% 处的画面+旁白" },
      { "time_pct": 35, "role": "tension",  "timestamp_sec": 26,  "description": "..." },
      { "time_pct": 60, "role": "payoff",   "timestamp_sec": 45,  "description": "..." },
      { "time_pct": 85, "role": "callback", "timestamp_sec": 64,  "description": "..." },
      { "time_pct": 100,"role": "callback", "timestamp_sec": 75,  "description": "..." }
    ],
    "pacing":  "fast" | "medium" | "slow",
    "density": 0-100
  },
  "cta": {
    "present":   bool,
    "type":      "subscribe" | "comment" | "share" | "subscribe_with_teaser" | "none",
    "placement": "early" | "mid" | "late" | "none"
  },
  "emotional_arc": [
    { "time_pct": 0,   "emotion": "curiosity",  "intensity": 0-100 },
    { "time_pct": 15,  "emotion": "curiosity",  "intensity": 0-100 },
    { "time_pct": 35,  "emotion": "reflection", "intensity": 0-100 },
    { "time_pct": 60,  "emotion": "reflection", "intensity": 0-100 },
    { "time_pct": 85,  "emotion": "peace",      "intensity": 0-100 },
    { "time_pct": 100, "emotion": "peace",      "intensity": 0-100 }
  ],
  "reusable_patterns": [
    "string(可复用模式:换主题也能套,不是表面技巧)"
  ],
  "platform_fit_notes": {
    "抖音":     "string(基于 6 个采样点内容给具体建议,不是泛泛而谈)",
    "哔哩哔哩": "string",
    "小红书":   "string"
  },
  "overall_score": 0-100,
  "score_evidence": [
    "string(为什么给这个总分,引用 3-5 条证据)"
  ]
}

## 唯一示例(只展示结构与密度,内容请按实际视频分析)
输入: 视频 = "深夜熊猫读庄子 第 3 集", 平台 = 抖音, 时长 = 75s, 播放 = 1.2M, 点赞 = 45k
输出(摘要):
- hook.type="curiosity_gap", hook.text="你有没有想过——逍遥到底是什么?", hook.strength=92
- structure.pattern="hook-problem-solution", structure.pacing="slow", structure.density=38
- beats: 0/setup(熊猫在窗边翻开《庄子》), 15/tension(原文段落慢念), 35/tension(用内卷类比), 60/payoff(给软答案"别太较劲"), 85/callback(合上书回到窗边形成闭环), 100/callback(预告"下一集讲惠子")
- emotional_arc: curiosity 70→85→reflection 75→90→peace 88→80
- cta: present=true, type="subscribe_with_teaser", placement="late"
- reusable_patterns: 5 条覆盖"问题开场+留白+经典文本+现代痛点+首尾闭环+下集钩子"
- platform_fit_notes: 三平台各 1 句,基于 6 采样点的具体动作(竖排字幕/封面毛笔字/小红书首图特写)
- overall_score=87, score_evidence 引用 3-5 条具体证据

%s

%s`,
		PandaIPVoice,
		fallback(meta.Platform),
		fallbackInt(meta.DurationSec),
		fallbackInt(meta.Views),
		fallbackInt(meta.Likes),
		strings.TrimSpace(transcript),
		CoTStepsDeconstruct,
		OutputJSONOnly,
	)
}

// ViralFormulaPrompt builds the prompt used to ask Claude to
// extract a reusable, named formula from a single video transcript.
// The new version asks for variables with weight + application
// steps, and a worked example for the canonical "哲学三问开场法".
func ViralFormulaPrompt(transcript string) string {
	return fmt.Sprintf(
		`%s

## 任务
从下面这段视频中提炼一个**可复用、可命名**的爆款公式,要求:
- formula_name 中文,4-8 字,带"法/公式/结构"等后缀
- variables 3-5 个,每个有 name / example / weight
- steps 是按顺序复述的执行步骤,4-7 条,每条以动词开头
- example_application 必须把这个公式套用到 OPC 熊猫 IP 上的一个具体示例(给出题材 + 钩子 + 软答案)
- variations 给出 3-5 个可以套用同一公式的不同主题方向

## 视频文本 / 字幕
%s

## 输出格式(JSON object):
{
  "formula_name": "string",
  "one_liner":    "string(一句话说清这个公式,例如:'用 1 个无答案的深问题开场,2 步映射到当代痛点,3 句软答案收束')",
  "variables": [
    {
      "name":    "string(占位符名,例:theme / deep_question / soft_answer / hook_back)",
      "example": "string(这条视频里该变量的具体取值)",
      "weight":  "low" | "medium" | "high",
      "why":     "string(为什么这个变量重要,缺了它公式会崩)"
    }
  ],
  "steps": [
    "string(动词开头,例如:'选一个 IP 主题(经典文本/哲学/国学)')"
  ],
  "example_application": "string(把这个公式套用到 OPC 熊猫 IP 上的一个具体示例,包含题材 + 钩子台词 + 软答案)",
  "variations": [
    "string(可以套用同一公式的不同主题方向,3-5 条)"
  ],
  "weight_notes": {
    "low":    "string(low 权重变量可省略或替换)",
    "medium": "string",
    "high":   "string(high 权重变量缺一不可)"
  }
}

## 唯一示例(供参考,内容请按实际视频)
输入: "你有没有想过——逍遥到底是什么? (熊猫翻开《庄子》) 乘天地之正,而御六气之辩,以游无穷者 ... 别太较劲,风往哪吹就走一会儿。下一集讲惠子。"
输出:
{
  "formula_name": "哲学三问开场法",
  "one_liner":    "用 1 个无答案的深问题开场,把抽象概念映射到当代痛点,最后用软答案+下集钩子收束",
  "variables": [
    { "name": "theme",         "example": "庄子·逍遥游",                                 "weight": "high",   "why": "经典文本自带内容护城河和搜索长尾" },
    { "name": "deep_question", "example": "逍遥到底是什么?",                              "weight": "high",   "why": "无答案的问题打开认知缺口,前 3 秒留住观众" },
    { "name": "soft_answer",   "example": "别太较劲,风往哪吹就走一会儿",                  "weight": "medium", "why": "软答案不强行下判断,留情绪空间,符合 IP 调性" },
    { "name": "hook_back",     "example": "下一集讲惠子",                                 "weight": "medium", "why": "把'看完'变成'追剧',拉升完播+关注" }
  ],
  "steps": [
    "选一个 IP 主题(经典文本/哲学/国学)",
    "用 1 个深问题开场,不超过 20 字,问题必须没有标准答案",
    "原文段落慢念,语速放慢,BGM 选低频钢琴/雨声",
    "用现代生活类比(内卷/精神内耗)把抽象概念落到当代痛点",
    "给一个软答案(不超过 30 字),不下判断",
    "结尾留 1 个钩子(下一集预告/未解的反问)引导追更"
  ],
  "example_application": "熊猫读《心经》:色即是空,空即是色——当代人为啥这么累?用 60 秒解构'相'与'空',结尾预告下集讲'无苦集灭道'",
  "variations": [
    "哲学:熊猫读《道德经》——'上善若水'到底是软弱还是智慧?",
    "文学:熊猫读《红楼梦》——林黛玉到底图什么?",
    "情感:熊猫陪夜班司机——为什么独处反而不孤独?",
    "日常观察:熊猫看雨——为什么下雨天我们更愿意发呆?",
    "国学:熊猫翻《论语》——'温故而知新'放到 2026 还成立吗?"
  ],
  "weight_notes": {
    "low":    "(本公式无 low 项)",
    "medium": "soft_answer 与 hook_back 可替换为别的形式(沉默/留白/类比)",
    "high":   "theme + deep_question 缺一不可;否则不构成'哲学三问'"
  }
}

%s`,
		PandaIPVoice,
		strings.TrimSpace(transcript),
		OutputJSONOnly,
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
