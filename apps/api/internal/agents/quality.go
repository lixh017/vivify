package agents

import "fmt"

// Suggestion category values used in the QualitySuggestion JSON
// contract. The Claude prompt's worked example references these
// constants so a rename in the wire shape (or a typo in the
// prompt) breaks compilation rather than silently degrading
// scoring. Keep this list in sync with the values the live
// /ai/score endpoint, the rule-based fallback, and the demo pool
// emit.
const (
	SuggestionCategoryHook      = "hook"
	SuggestionCategoryStructure = "structure"
	SuggestionCategoryPlatform  = "platform_fit"
	SuggestionCategoryWordCount = "word_count"
	SuggestionCategoryCTA       = "cta"
)

// Suggestion severity values used in the QualitySuggestion JSON
// contract. Same rationale as SuggestionCategory*: code-level
// constants so a typo in the prompt is a compile error.
const (
	SuggestionSeverityLow    = "low"
	SuggestionSeverityMedium = "medium"
	SuggestionSeverityHigh   = "high"
)

// ScoreContentPrompt builds the prompt used to ask Claude to
// evaluate a content piece (title + script) and return a JSON
// object matching the QualityScoreResponse shape.
//
// The new version:
//   - anchors the panda IP voice up front
//   - shows the scoring axes in a small table with explicit weights
//   - asks for a chain-of-thought pass before scoring
//   - requires every suggestion to be a "before/after" pair
//   - ships a worked example so the model sees the expected density
//   - reminds Claude about the anti-pattern list
func (c *Claude) ScoreContentPrompt(title, script, platform string) string {
	return fmt.Sprintf(
		`%s

## 任务
对下面这条内容做 0-100 评分,每个轴独立打分,综合分按权重加权。**每条建议必须是"问题→改法"两行,不能空话。**

## 输入
- 平台:   %s
- 标题:   %s
- 脚本:
%s

## 评分轴(独立打分,再加权)
| 轴             | 权重 | 评分要点(只对真实存在的证据打分)|
|----------------|------|----------------------------------|
| hook_strength  | 40%%  | 前 3 秒是否有具体画面/提问/反差/数字;是否避免了"今天想给大家讲" |
| structure      | 35%%  | 段落节奏、是否有 1-2 个情绪转折、结尾是否收束或留白 |
| platform_fit   | 25%%  | 标题字数、脚本长度、是否符合目标平台惯例(见下表) |

%s

%s

## 唯一示例(展示期望的密度)
输入: 标题="熊猫陪你度过雨夜", 脚本="窗边的熊猫看着雨,一句话都没说。今天辛苦了。", 平台="抖音"
输出:
{
  "overall_score":  82,
  "hook_strength":  88,
  "structure":      80,
  "platform_fit":   78,
  "axis_breakdown": {
    "hook":         "画面钩子('窗边的熊猫看着雨')有具体视觉,无 AI 痕迹 +20;无数字/反差 +0;共 +20",
    "structure":    "两段结构 +10;有留白结尾('今天辛苦了'是悬而未决) +20",
    "platform_fit": "标题 8 字 ≤ 22 字 +25;脚本 21 字偏短(< 80) -7;抖音钩子 3 秒内出画面 +10"
  },
  "suggestions": [
    { "category": "%s", "severity": "%s",
      "problem":   "脚本只有 21 字,完播率高但信息密度低,长尾搜索吃不动",
      "rewrite":   "在'今天辛苦了'前加 1-2 句铺垫,例如:'今天的雨...下得有点久' (留白 + 共情)" },
    { "category": "%s", "severity": "%s",
      "problem":   "前 3 秒虽然有画面,但旁白第一句才出'窗边的熊猫',可再前置 0.5 秒",
      "rewrite":   "把'画面: 雨打在窗上,熊猫侧脸'写在脚本最前,代替空镜" }
  ],
  "rewritten_hook": ""
}

## 关键约束
- axis_breakdown 必须把 +分 / -分 的依据写出来,不要写"hook 较强"这种空话
- suggestions 每条同时给 problem + rewrite 两个字段
- 当 hook_strength < 70 时,rewritten_hook 必须是 1 句可直接用的前 3 秒台词(≤ 22 字);否则填空字符串
- 严格避免上方的 AI 痕迹清单

%s`,
		PandaIPVoice,
		platform, title, script,
		PlatformVoice,
		CoTStepsQuality,
		OutputJSONOnly,
		SuggestionCategoryStructure, SuggestionSeverityMedium,
		SuggestionCategoryHook, SuggestionSeverityLow,
	)
}

// PlatformAdaptPrompt builds the prompt used to ask Claude to take
// a single content concept and adapt it for the three OPC
// platforms (抖音/哔哩哔哩/小红书).
//
// The new version adds the per-platform voice reference table, an
// explicit cross-platform checklist, and a worked example for the
// "雨夜窗边" canonical panda concept.
func (c *Claude) PlatformAdaptPrompt(title, angle, sourcePlatform string) string {
	return fmt.Sprintf(
		`%s

## 任务
把下面这条内容,同步改造为抖音 / 哔哩哔哩 / 小红书三个平台的版本。要求三个版本的**核心信息一致**(情绪锚点 + IP 形象),但形式(字数/标签/正文长度/语气)严格匹配各平台调性。

## 输入
- 原始标题:   %s
- 选题角度:   %s
- 来源平台:   %s

%s

## 必须遵守的平台硬约束
- 抖音:    标题 ≤ 22 字,话题标签 3-5 个,正文 ≤ 100 字
- 哔哩哔哩: 标题 ≤ 80 字,标签 5-10 个,正文 200-500 字
- 小红书:  标题 ≤ 20 字,标签 5-8 个,正文 200-500 字,emoji 可用

## 跨平台必须保持一致
- IP 形象(熊猫)必须在标题或首句出现,不允许替换
- 核心情绪锚点(一句话)三平台通用,作为内化的精神坐标
- 旁白中"我/熊猫"视角,不要换成"我们/大家"

## 唯一示例(供参考,不要照抄)
输入: 原始标题="雨夜窗边的熊猫", 选题角度="深夜治愈:陪着就够了", 来源平台="抖音"
输出:
{
  "adaptations": {
    "抖音": {
      "title":       "雨夜窗边的熊猫",
      "hashtags":    ["#治愈系","#深夜emo","#熊猫日记","#情绪释放"],
      "description": "窗边的熊猫,看着雨,一句话都没说。你今天,也辛苦了。",
      "first_3s":    "雨打窗玻璃特写 → 熊猫侧脸 → 旁白第一句"
    },
    "哔哩哔哩": {
      "title":       "【熊猫日记 EP03】雨夜窗边,我和自己聊了聊",
      "description": "今晚的雨下得有点久。窗边的熊猫,看着外面,不说话。其实有时候,陪着就够了。你今天,也辛苦了。(共 60-80 字;可扩展 200-500 字版做完整短文)",
      "tags":         ["治愈","熊猫","深夜","情绪","解压","慢节奏","氛围感","独处","情绪释放","哲学"],
      "cover_hint":   "熊猫侧脸 + 雨滴特写,色调冷蓝 + 暖灯"
    },
    "小红书": {
      "title":       "雨夜🐼窗边发呆",
      "body":         "今晚的雨下得有点久。\\n\\n窗边的熊猫,就那么坐着,看着外面。\\n\\n不说话。\\n\\n其实有时候吧,不需要说什么,陪着就够了。",
      "tags":         ["#治愈","#独处","#深夜","#情绪","#氛围感","#熊猫","#慢生活"]
    }
  },
  "cross_platform_tips": [
    "核心信息'陪着就够了'在三个平台都保留,作为情绪锚点",
    "IP 角色(熊猫)必须出现在画面或文字中,不要替换成其他形象",
    "小红书可加 emoji,抖音不加,哔哩哔哩看 UP 主风格",
    "BGM 选雨声+轻钢琴,适配三个平台的氛围调性",
    "抖音首 3 秒是关键,封面 1 帧熊猫侧脸比全景更有停留"
  ]
}

## 思考步骤
1. 提炼这条内容的 1 句"情绪锚点"(跨平台通用)
2. 各平台分别:写标题 → 配标签 → 写正文/描述 → 提示封面/首帧
3. cross_platform_tips 给出 3-5 条,区分"必须保持"和"必须适配"

%s`,
		PandaIPVoice,
		title, angle, sourcePlatform,
		PlatformVoice,
		OutputJSONOnly,
	)
}
