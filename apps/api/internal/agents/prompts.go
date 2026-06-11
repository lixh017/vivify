package agents

import "fmt"

// Centralized prompt catalog for the OPC "熊猫" IP.
//
// All prompt builders (claude.go, quality.go, deconstruct.go) draw
// from the constants defined here. To keep this file under the
// project's 800-line limit as the catalog grows, the shared blocks
// are split across sibling files:
//
//   - prompts_voice.go: PandaIPVoice, PandaAntiPatterns,
//     HookFormulaLibrary, PlatformVoice (the IP tone blocks).
//   - prompts_cot.go:   CoTStepsTopics, CoTStepsQuality,
//     CoTStepsDeconstruct (the chain-of-thought preambles).
//
// This file owns the per-task prompt builder methods and the
// JSON-only tail that every prompt appends. claude.go keeps the
// *Claude struct + Provider shim + Complete implementations; the
// prompt builders (which do not need the struct's state) live
// here so the routing-facing file stays focused on transport.

// OutputJSONOnly is the boilerplate tail every prompt appends so
// Claude returns parseable JSON without markdown fences or
// preamble. Putting it in one place means a parser change (e.g.
// "now also allow json fences") only has to happen once.
const OutputJSONOnly = `只用 JSON 输出,不要加任何解释、不要 markdown 代码块、不要开场白。`

// GenerateTopicsPrompt builds the prompt used to ask Claude for N
// differentiated topic ideas for the "熊猫" IP given a seed and
// target platform. The prompt is now IP-voice-anchored, gives a
// 7-pattern hook library, asks for a thinking pass before output,
// and ships with a worked example so the model does not regress to
// generic Chinese short-video topics.
//
// The returned prompt is plain text (no tool-use wrapping). The
// shape requested is a JSON array of objects matching TopicIdea.
func GenerateTopicsPrompt(seed, platform string, count int) string {
	return fmt.Sprintf(
		`%s

## 任务
基于种子概念 "%s",为目标平台 "%s" 一次性生成 %d 个**差异化**的选题。要求:
- 4 个调性维度(治愈/御宅/哲学/国潮)中至少覆盖 3 个
- 每个选题必须能用一句话说清楚"和已有内容比,差别在哪"
- 标题字数严格遵守目标平台上限(抖音 ≤22 / B 站 ≤80 / 小红书 ≤20)

%s

%s

## 输出格式 (JSON array,严格遵守):
[
  {
    "title":               "10-20 字标题,符合平台字数",
    "angle":               "独特角度,1 句话,说明和同主题已有内容的差别",
    "hook":                "前 3 秒的具体台词+画面(画面用'画面:'前缀,旁白用'旁白:'前缀)",
    "expected_performance":"为什么这条有机会爆,给出情绪/数据/时令的具体推理",
    "pattern":             "用了哪个钩子公式(从上面的 7 个公式里挑一个写名字)",
    "voice_tags":          ["治愈","哲学"]  ← 命中了哪几个调性维度
  }
]

## 唯一示例(供参考,不要照抄):
种子 = "夜", 平台 = "抖音":
[
  {
    "title": "凌晨 3 点,熊猫为什么不睡",
    "angle": "御宅+治愈:深夜的孤独感用'反问自己'替代'心疼观众'",
    "hook": "画面: 窗边熊猫对窗发呆,3 秒没有台词。 旁白: 你也还没睡吧。",
    "expected_performance": "深夜流量高峰(00:00-03:00 抖音活跃度高)+ 孤独感共鸣 + 完播率因留白而拉满",
    "pattern": "画面钩子",
    "voice_tags": ["御宅","治愈"]
  }
]

%s`,
		PandaIPVoice,
		seed, platform, count,
		HookFormulaLibrary,
		CoTStepsTopics,
		OutputJSONOnly,
	)
}

// HumanizeScriptPrompt builds the prompt used to ask Claude to
// rewrite a script so it does not feel AI-generated. The new
// version anchors the IP voice up front, enumerates the anti-pattern
// list explicitly, and gives a before/after example so Claude sees
// exactly what "human" looks like in the panda IP.
func HumanizeScriptPrompt(script string) string {
	return fmt.Sprintf(
		`%s

## 任务
把下面这段脚本改写得"不像 AI 写的"。先想清楚它当前的最大 AI 痕迹在哪里(排比?铺垫?口号?抽象?),再针对性改写,不要通篇平均用力。

## 改写要求(保留内容,只改语气与节奏):
- 留下具体画面(窗、雨、书、咖啡)而不是抽象概念
- 加入停顿/犹豫/转折:用 "..."、"(停顿)"、"(叹气)" 等显式标记
- 删掉"首先/其次/最后""大家好""让我们一起""加油"等模板句
- 短句优先,长句拆成两行;不写排比、不写三段式
- 结尾允许悬而未决,不强行收束成"愿你被世界温柔以待"
- 主体旁白以熊猫为视角(我/熊猫/它),不要第三人称讲解
- 如果原脚本超过 250 字,保留 60-80%% 的核心信息,不要全塞回去

%s

## 唯一示例(必须学这个感觉):
改写前(典型 AI 风):
"今天想给大家讲讲庄子。在如今这个内卷的时代,我们每个人都很焦虑。庄子告诉我们,真正逍遥的人,是放下执念的人。让我们一起做这样的逍遥人。"

改写后(熊猫 IP 风):
"你有没有想过——'逍遥'到底是什么?
(熊猫翻开《庄子》,慢慢念)
'乘天地之正,而御六气之辩,以游无穷者。'
嗯...其实我也没太懂。
但是吧,大概意思是:别太较劲。
风往哪吹,就往哪走一会儿。"

## 待改写脚本:
%s

## 输出
直接输出改写后的纯文本脚本,不要加任何解释/标题/前后缀。`,
		PandaIPVoice,
		PandaAntiPatterns,
		script,
	)
}

// PostmortemPrompt builds the prompt used to ask Claude to
// deconstruct a viral content item: why it performed well, what
// patterns are reusable, what the metrics suggest, and what to try
// next. The new version asks Claude to score the four dimensions
// (hook / structure / platform_fit / emotional_arc) and to
// separate "evidence" from "speculation" in the insights.
func PostmortemPrompt(title, script, metrics, topicAngle string) string {
	return fmt.Sprintf(
		`%s

## 任务
对一条已经发布的"熊猫"内容做结构化复盘:先把每条结论的"证据"列清楚,再下判断,不要凭空表扬。

## 输入
- 标题: %s
- 脚本: %s
- 表现数据: %s
- 选题角度: %s

## 思考步骤
1. 先从数据里挑 2 个最异常的指标(显著高于或低于赛道均值),解释原因
2. 回到脚本,定位具体哪一段/哪一句台词和这两个指标对应
3. 提炼 3-5 条"换主题也能复用"的模式(必须是模式,不是表面技巧)
4. 给出 3 条下次可执行的改进,每条都要说"改哪里→为什么→预期效果"

%s

## 输出格式(JSON object,严格遵守以下 shape):
{
  "success_factors":   ["string", ...]   ← 3-5 条,每条都是一句"具体证据 + 结论",例:"前 3 秒'窗边的熊猫看着雨'有具象画面,完播率 68%% 验证钩子有效"
  "reusable_patterns": ["string", ...]   ← 3-5 条,必须是"换主题也能套"的模式,例:"治愈场景 = 自然元素(雨/雪/晚风) + 熊猫静态画面 + 一句话旁白"
  "insights":          ["string", ...]   ← 3-5 条,每条都是"claim | evidence | confidence"三段式,例:"完播率高于均值 23pct | 播放完成率 68%% vs 赛道 45%% | confidence=high"
  "axis_scores": {
    "hook":          0-100,
    "structure":     0-100,
    "platform_fit":  0-100,
    "emotional_arc": 0-100
  },
  "suggestions":       ["string", ...]   ← 3-5 条,每条都是"改哪里 → 为什么 → 预期效果"三段式,例:"结尾画面再多停 1 秒 → 给观众反应时间,留白提升评论区互动 → 预期评论量 +15pct"
}

%s`,
		PandaIPVoice,
		title, script, metrics, topicAngle,
		CoTStepsQuality,
		OutputJSONOnly,
	)
}

// ScoreContentPrompt builds the prompt used to ask Claude to
// evaluate a content piece (title + script) and return a JSON
// object matching the QualityScoreResponse shape. Lives here
// (alongside the other per-task prompt builders) so the prompt
// catalog is in one place; quality.go owns the wire shape and
// parsers.
//
// The new version:
//   - anchors the panda IP voice up front
//   - shows the scoring axes in a small table with explicit weights
//   - asks for a chain-of-thought pass before scoring
//   - requires every suggestion to be a "before/after" pair
//   - ships a worked example so the model sees the expected density
//   - reminds Claude about the anti-pattern list
func ScoreContentPrompt(title, script, platform string) string {
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

// PlatformAdaptPrompt builds the prompt used to ask Claude to
// take a single content concept and adapt it for the three OPC
// platforms (抖音/哔哩哔哩/小红书). Moved from quality.go in
// Phase 3 QA cleanup so the prompt catalog is in one place.
//
// The new version adds the per-platform voice reference table, an
// explicit cross-platform checklist, and a worked example for the
// "雨夜窗边" canonical panda concept.
func PlatformAdaptPrompt(title, angle, sourcePlatform string) string {
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
      "body":         "今晚的雨下得有点久。\n\n窗边的熊猫,就那么坐着,看着外面。\n\n不说话。\n\n其实有时候吧,不需要说什么,陪着就够了。",
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
