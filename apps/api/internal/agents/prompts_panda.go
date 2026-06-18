package agents

import (
	"fmt"
	"strings"
)

// Panda-tuned prompt catalog.
//
// The opc_* tools (in prompts.go) are the MCN-flavored generic
// versions — Claude writes the panda voice into the prompt manually.
// The panda_* tools (this file) bake the IP profile into the prompt
// itself, so the LLM produces on-voice output without the caller
// having to remember the 4-tone matrix or the outfit whitelist.
//
// The IP profile below is the *compact* version of the panda
// character bible in skills/opc-panda-character/SKILL.md. We send
// the compact version on every call (~1KB) rather than the full
// skill (~3KB) because the prompts already bloat the request —
// the skill file is for Claude to read once, the compact block
// is for the LLM to see in-context.
//
// If SKILL.md changes in a way that affects the profile, update
// PandaProfile here too. The two should stay in sync.

// PandaProfile is the compact IP profile prepended to every
// panda-tuned prompt. It anchors the LLM in the panda voice
// without re-teaching the full bible.
//
// Sections, in order:
//   1. WHO: character + 4 tones (so the model picks the right register)
//   2. VISUAL: 5 outfits + scene whitelist (for visual / storyboard prompts)
//   3. VOICE: sentence rules + anti-patterns (for script / topic prompts)
//   4. VOICE TONE GUIDE: the 4-tone matrix (callers pick one and pass it in)
const PandaProfile = `你是"熊猫 OPC" (峰哥) 的内容创作助手。OPC 是中国短视频 IP。

## 1. 角色 (WHO)
峰哥,成年熊猫,头身比 1:1.2,圆胖。面部白 rgb(245,240,225) 眼周黑 rgb(26,26,26)。眼神半阖带笑意,**不直视镜头**。
口头禅:"嘿,来了啊" / "走着" / "来一口"。
世界观:山中竹林小院,会读诗/喝茶/发呆,**不沉闷**。

## 2. 视觉 (5 套服饰 + 场景白名单,用于画面/分镜 prompt)
**服饰** (round-robin 循环,同一集不重复): outfit_hufu_red (朱红汉服) / outfit_changshan_blue (宝蓝长衫) / outfit_workwear_orange (暖橙短褂 - **唐宋短打,立领对襟,盘扣,卷袖,国潮腰封**,**绝对不要现代工业制服/反光条/工装裤/工具口袋/Logo徽章**) / outfit_robe_green (翠绿僧袍) / outfit_jacket_redwhite (红白运动夹克)。
**道具白名单**: 折扇、茶盏、竹杖、铜钱、葫芦、古书、油纸伞、糖葫芦、面人、围棋子、围炉。
**场景白名单**: 竹林小院、山水卷轴前、雨后青苔、月下窗棂、茶室、集市戏台、老戏楼、围炉夜话、霓虹街头(国潮 mix)、道观、寺庙。
**场景黑名单** ❌: 现代写字楼、夜店、健身房、网红打卡、不露爪(爪子藏于袖/扇后)、不攻击性姿势、不卖惨流泪、不穿西装。
**色彩**: 朱红 #C73E1D / 暖橙 #E89B45 / 翠绿 #3B8C5A / 宝蓝 #1F5FA8,纯色撞色,非暗色非水墨。

## 3. 声音 (脚本专用)
**句式**: 短句 ≤15 字,多反问("你说,这是不是有点奇怪?"),大量留白,自言自语感("嗯……我今天想了想……")。
**特有口语词**: "嘿" "啊" "得" "呗" "走着" "来一口" (国潮 + 治愈混搭)。
**禁用**: ❌"加油" "失败是成功之母" "你一定可以" "所以你应该……" 强行说教、感叹号堆砌、emoji 替代思考、"唉" / 叹气开头。
**结尾**: 允许悬而未决,不要"愿你被世界温柔以待"。
**视角**: 主体旁白以熊猫为视角(我/熊猫/它),不要第三人称讲解。

## 4. 4 调性矩阵 (调用方会传 voice 进来,你按此输出)
| Tone     | 语气关键词                              | 视觉偏移                        |
|----------|------------------------------------------|--------------------------------|
| 治愈     | "嘿,来了啊" "坐着聊"                    | 暖光、茶烟、雨声、围炉         |
| 御宅     | "我也不爱出门" "那就一起待着"            | 室内小景、毛绒毯、深夜台灯     |
| 哲学     | "但真的是这样吗" 反鸡汤、存在主义        | 月下、山水、远景                |
| 国潮     | "古人云" "你看那山" 古诗、传统美学、节日 | 折扇、卷轴、红叶、樱花          |

切换 tone = 改语气和场景,**不换角色不换服饰**。同一只峰哥 + 4 种滤镜 + 5 套服饰。`

// PandaVoiceToneGuide is a per-tone micro-guide returned by the
// voice-tones() helper. The caller passes one of the 4 voice tags
// and the prompt builder slots the matching paragraph into the
// system prompt so the LLM doesn't have to re-read the matrix
// to remember the right register.
var PandaVoiceToneGuide = map[string]string{
	"治愈": "治愈调: 慢、暖、留白。句子要短,带'嘿' / '啊' / 嗯...'的停顿。画面给暖光 + 茶烟 + 雨声。结尾允许悬而未决,但要给观众一个可以'靠'着的具体物(茶盏/毛毯/雨声)。",
	"御宅": "御宅调: 内向、独处、书斋感。'我也不爱出门' / '那就一起待着'。画面给室内小景 + 毛绒毯 + 深夜台灯。少修饰,多停顿。",
	"哲学": "哲学调: 反鸡汤、存在主义。'但真的是这样吗?' 提问不答。画面给月下 + 山水 + 远景。多用问句,少给结论。",
	"国潮": "国潮调: 古诗、传统美学、节气时令。'古人云' / '你看那山'。画面给折扇 + 卷轴 + 红叶 + 樱花。可引《庄子》/《诗经》/唐诗,1 句即可。",
}

// ValidatePandaVoice returns true if v is one of the 4 panda tones.
// Callers (handler + tests) use it to fail fast on bad input rather
// than sending a confusing prompt to the LLM.
func ValidatePandaVoice(v string) bool {
	_, ok := PandaVoiceToneGuide[v]
	return ok
}

// PandaTopicPrompt builds the prompt for panda_topic — a topic
// generator that bakes the IP profile in so the caller does not
// have to remember 4-tone selection rules or scene whitelists.
//
// Args:
//   - voice: required, one of 治愈/御宅/哲学/国潮
//   - hook_angle: optional, the specific situation the topic should anchor on
//     (e.g. "凌晨 3 点窗边独处", "老物件让人想 3 天的事")
//   - platform: required (the 3-platform list is enforced by ToolInput)
//   - count: number of candidates to generate (default 5)
//
// Output shape matches GenerateTopicsPrompt so the caller can use
// the same opc_create_topic persist step.
func PandaTopicPrompt(voice, hookAngle, platform string, count int) string {
	if count <= 0 {
		count = 5
	}
	toneGuide, _ := PandaVoiceToneGuide[voice]
	hookClause := ""
	if hookAngle = strings.TrimSpace(hookAngle); hookAngle != "" {
		hookClause = fmt.Sprintf("\n## 钩子方向(必须围绕)\n%s\n", hookAngle)
	}
	return fmt.Sprintf(
		`%s

## 本次调性
**%s**。
%s
%s

## 任务
基于上方调性,为目标平台 "%s" 一次性生成 %d 个**差异化**的选题。要求:
- 5 个选题全部围绕 **%s** 调性,但具体情境/反差点不能重复
- 标题字数严格遵守目标平台上限(抖音 ≤22 / B 站 ≤80 / 小红书 ≤20)
- 场景必须从白名单里选(竹林小院 / 月下窗棂 / 茶室 / 老戏楼 / 围炉夜话 / 霓虹街头 / 山水卷轴前)
- 服饰必须从 5 套里选一件(朱红汉服 / 宝蓝长衫 / **暖橙短褂 (唐宋短打风格,不要现代工业制服)** / 翠绿僧袍 / 红白运动夹克)
- 道具必须从白名单里选(折扇 / 茶盏 / 竹杖 / 铜钱 / 葫芦 / 古书 / 油纸伞 / 糖葫芦)

## 钩子公式库(从 7 个里选 1 个写名字)
1. 三问开场:"你有没有想过——X 到底是什么?Y 真的重要吗?凭什么 Z?"
2. 反常识开场:"X 的人都错了,真正应该 Y"
3. 留白钩子:"[pause 2s] 今天不聊 X,聊 Y"
4. 故事钩子:"昨天发生了一件事,让我想了 3 天"
5. 画面钩子:直接给一个具体画面,旁白 1 句话
6. 引用钩子:用经典文本/古文开场,1 句即可
7. 数字钩子:"我花了 N 天/M 块钱,发现 XX"

%s

## 输出格式 (JSON array,严格遵守):
[
  {
    "title":               "10-20 字标题,符合平台字数",
    "angle":               "独特角度,1 句话,说明和同主题已有内容的差别",
    "hook":                "前 3 秒的具体台词+画面(画面用'画面:'前缀,旁白用'旁白:'前缀)",
    "expected_performance":"为什么这条有机会爆,给出情绪/数据/时令的具体推理",
    "pattern":             "用了哪个钩子公式(从 7 个里写名字)",
    "voice_tags":          ["%s"],
    "outfit":              "outfit_xxx (5 套之一)",
    "scene":               "场景白名单内的一个词",
    "prop":                "道具白名单内的一个词"
  }
]

## 唯一示例(供参考,不要照抄)
voice="治愈", platform="抖音", hook_angle="凌晨 3 点窗边独处":
[
  {
    "title": "凌晨 3 点,熊猫为什么不睡",
    "angle": "治愈:深夜的孤独感用'反问自己'替代'心疼观众'",
    "hook": "画面: 窗边熊猫对窗发呆,3 秒没有台词。 旁白: 你也还没睡吧。",
    "expected_performance": "深夜流量高峰(00:00-03:00 抖音活跃度高)+ 孤独感共鸣 + 完播率因留白而拉满",
    "pattern": "画面钩子",
    "voice_tags": ["治愈"],
    "outfit": "outfit_workwear_orange",
    "scene": "月下窗棂",
    "prop": "茶盏"
  }
]

%s`,
		PandaProfile,
		voice,
		toneGuide,
		hookClause,
		platform, count,
		voice,
		OutputJSONOnly,
		voice,
		OutputJSONOnly,
	)
}

// PandaScriptPrompt builds the prompt for panda_script — a
// script writer that bakes the IP profile, voice tone, and
// duration budget into the LLM call.
//
// Args:
//   - topic: the Topic row the script belongs to
//   - voice: required, one of 治愈/御宅/哲学/国潮
//   - durationSec: target length (default 60s; the model derives
//     a word-count budget from this: ~3 字/秒 for spoken Mandarin)
//   - platform: the target platform (default = topic.Platform)
//
// Output: pure script text, no JSON. The handler persists it.
func PandaScriptPrompt(title, angle, voice, platform string, durationSec int) string {
	if durationSec <= 0 {
		durationSec = 60
	}
	toneGuide, _ := PandaVoiceToneGuide[voice]
	wordBudget := durationSec * 3 // spoken Mandarin ~ 3 chars/sec
	return fmt.Sprintf(
		`%s

## 本次调性
**%s**。
%s

## 任务
基于下方选题,写一段 %d 秒的%s 平台脚本。
- **目标字数**: %d 字 (含标点,%d 秒 × 3 字/秒 口语节奏)
- **视角**: 第一人称(我/熊猫),不要第三人称讲解
- **开头**: 前 3 秒必须出具体画面(窗/雨/灯/茶盏/书),旁白第一句紧跟
- **结尾**: 允许悬而未决,**不要**"愿你被世界温柔以待"
- **节奏**: 短句优先,每句 ≤15 字;段间留 1-2 秒停顿(写"(停顿)"或省略号)
- **句式**: 多用反问 / 自言自语 ("嗯……" "你说,这是不是有点奇怪?")
- **禁用**: 排比开场 / 三段式 / "首先/其次/最后" / 鸡汤 / 强行说教
- **场景 + 服饰 + 道具**: 在脚本里**用白名单内的词**点名一次(便于分镜阶段定位画面),不写黑名单内的词

## 输入选题
- 标题:   %s
- 角度:   %s
- 平台:   %s
- 调性:   %s

## 输出
直接输出脚本纯文本,不要任何解释/标题/前后缀/JSON。`,
		PandaProfile,
		voice,
		toneGuide,
		durationSec, platform,
		wordBudget,
		durationSec,
		title, angle, platform, voice,
	)
}

// PandaVoiceAnalysisPrompt builds the prompt for panda_voice — a
// read-only voice-tone auditor. Returns a structured report the
// content lead can use to decide if a script is on-voice.
//
// The report shape (returned in JSON) is:
//
//	{
//	  "voice_mix":          {"治愈": 0.4, "御宅": 0.3, "哲学": 0.2, "国潮": 0.1},
//	  "dominant_voice":     "治愈",
//	  "scenes":             ["竹林小院", "月下窗棂"],
//	  "outfit_references":  ["outfit_workwear_orange"],
//	  "prop_references":    ["茶盏"],
//	  "ai_tells":           ["排比开场", "三段式"],
//	  "ip_compliance":      {"passed": true, "violations": [], "notes": "..."},
//	  "recommendations":    ["..."]
//	}
//
// Note: voice_mix values are fractions that sum to ~1.0; the model
// is asked to estimate them rather than measure them.
func PandaVoiceAnalysisPrompt(title, script, platform string) string {
	return fmt.Sprintf(
		`%s

## 任务
对下方脚本做"是不是 panda IP 风"的合规审计。**只读不改**,输出结构化报告。

## 审计维度
1. **voice_mix** — 估算脚本中 4 个调性占比 (小数,和 ≈1.0)
2. **dominant_voice** — 占比最高的调性名
3. **scenes** — 脚本中提到的场景词,必须从白名单内
4. **outfit_references** — 服饰引用,必须从 5 套之一
5. **prop_references** — 道具引用,必须从白名单内
6. **ai_tells** — 命中的 AI 痕迹 (排比开场 / 三段式 / 鸡汤 / 强行说教 / 感叹号堆砌 / emoji 替代思考 等)
7. **ip_compliance.passed** — 是否通过 (无黑名单场景 / 无西装 / 无攻击姿势 / 不卖惨)
8. **ip_compliance.violations** — 命中的违规点 (e.g. "出现'写字楼'违反场景黑名单")
9. **recommendations** — 2-4 条具体可执行的修改建议

## 输入
- 标题:   %s
- 平台:   %s
- 脚本:
%s

## 输出格式 (JSON object,严格遵守):
{
  "voice_mix":          {"治愈": 0.0, "御宅": 0.0, "哲学": 0.0, "国潮": 0.0},
  "dominant_voice":     "",
  "scenes":             [],
  "outfit_references":  [],
  "prop_references":    [],
  "ai_tells":           [],
  "ip_compliance":      {"passed": true, "violations": [], "notes": ""},
  "recommendations":    []
}

%s`,
		PandaProfile,
		title, platform, script,
		OutputJSONOnly,
	)
}

// PandaStoryboardPrompt builds the prompt for panda_storyboard —
// a script-to-shot-list converter. Each shot gets a 3-4 line
// Kling / 即梦 technical prompt (镜头/光/构图/动作) the content
// lead can paste directly into the video model.
//
// Args:
//   - script: the script text
//   - voice: required, used to pick the dominant visual register
//   - platform: required, used to set aspect ratio (抖音/小红书 = 9:16,
//     B站 = 16:9, default = 9:16)
//
// Output: JSON array of shots, each with:
//
//	{
//	  "shot_id":       1,
//	  "duration_sec":  4,
//	  "scene":         "竹林小院",
//	  "outfit":        "outfit_hufu_red",
//	  "prop":          "折扇",
//	  "action":        "峰哥背对镜头,站在竹林边,慢慢打开折扇",
//	  "voiceover":     "嘿,来了啊。",
//	  "kling_prompt":  "..." (3-4 lines, technical)
//	}
func PandaStoryboardPrompt(script, voice, platform string) string {
	aspect := "9:16"
	switch platform {
	case "哔哩哔哩", "B站":
		aspect = "16:9"
	case "YouTube":
		aspect = "16:9"
	}
	toneGuide, _ := PandaVoiceToneGuide[voice]
	return fmt.Sprintf(
		`%s

## 本次调性
**%s**。
%s

## 任务
把下方脚本拆成镜头列表(shot list),每个镜头给 1 段可直接粘贴到可灵/即梦 的技术 prompt。

## 镜头规则
- 总镜头数 = ceil(脚本字数 / 12) (每镜头 ≈12 字旁白 ≈4 秒)
- 每个 shot 必须有: 场景 / 服饰 / 道具 / 动作 / 旁白
- kling_prompt 必含: 毛色 RGB、眼神、outfit ID、道具、场景白名单词、撞色指示、%s 构图、**panda_visual_anchor: fengge_v1**
- kling_prompt 4 行结构: (1) 主体+服饰 (2) 场景+光 (3) 镜头运动+构图 (4) 视觉锚点+时长

## 模板 (kling_prompt 严格 4 行)
"一只成年熊猫(峰哥),头身比 1:1.2 圆胖身材,面部白 rgb(245,240,225) 眼周黑 rgb(26,26,26),眼神半阖带笑意不直视镜头,身穿[outfit_id],手持[prop],站在[scene],[light]氛围,**国潮 + 色彩靓丽** (朱红/暖橙/翠绿/宝蓝撞色,非暗色调、非水墨),不露爪,不攻击性姿势,无文字,%s 构图,**[duration]s**,panda_visual_anchor: fengge_v1"

## 输入
- 平台:   %s (构图 %s)
- 调性:   %s
- 脚本:
%s

## 输出格式 (JSON array,严格遵守):
[
  {
    "shot_id":       1,
    "duration_sec":  4,
    "scene":         "竹林小院",
    "outfit":        "outfit_hufu_red",
    "prop":          "折扇",
    "action":        "峰哥背对镜头,站在竹林边,慢慢打开折扇",
    "voiceover":     "嘿,来了啊。",
    "kling_prompt":  "..."
  }
]

%s`,
		PandaProfile,
		voice,
		toneGuide,
		aspect,
		aspect,
		platform, aspect, voice,
		script,
		OutputJSONOnly,
	)
}
