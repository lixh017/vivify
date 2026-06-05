package agents

import "fmt"

// ScoreContentPrompt builds the prompt used to ask Claude to evaluate
// a content piece (title + script) and return a JSON object matching
// the QualityScoreResponse shape. The prompt lists the scoring axes
// (hook / structure / platform fit) and asks for actionable
// suggestions plus a rewritten hook when the original is weak.
func (c *Claude) ScoreContentPrompt(title, script, platform string) string {
	return fmt.Sprintf(
		`你是 OPC 内容质量评分助手。请对以下内容做 0-100 评分并给出可执行建议。

平台：%s
标题：%s
脚本：
%s

输出 JSON 对象,包含以下字段:
- overall_score(int):综合分
- hook_strength(int):前 3 秒钩子强度
- structure(int):脚本结构(开头-中段-结尾)
- platform_fit(int):与目标平台的契合度
- suggestions(array):每条 {category, message, severity},severity 取 low/medium/high
- rewritten_hook(string):当 hook_strength < 70 时给一个改写版本,否则填空字符串

评分要点:
- hook_strength:是否有强动词/提问/反差/具体数字
- structure:段落节奏、呼吸感、是否有明确 CTA
- platform_fit:标题字数、脚本长度、是否符合该平台惯例

只用 JSON 输出,不要加多余解释。`,
		platform, title, script,
	)
}

// PlatformAdaptPrompt builds the prompt used to ask Claude to take a
// single content concept and adapt it for the three OPC platforms
// (抖音/哔哩哔哩/小红书). The prompt explicitly lists each
// platform's conventions (title length, tone, format) so Claude
// doesn't have to guess.
func (c *Claude) PlatformAdaptPrompt(title, angle, sourcePlatform string) string {
	return fmt.Sprintf(
		`你是 OPC 跨平台内容适配助手。把以下内容改造成适配三个平台的版本。

原始标题:%s
选题角度:%s
来源平台:%s

平台惯例:
- 抖音:标题 ≤22 字,强钩子开头,竖屏短视频,话题标签 3-5 个
- 哔哩哔哩:标题 ≤80 字,可更详细描述内容,标签 5-10 个,接受长标题
- 小红书:标题 ≤20 字,种草/教程语气,正文 200-500 字,标签 5-8 个,emoji 可用

输出 JSON 对象,包含:
- adaptations: 三个键 '抖音'/'哔哩哔哩'/'小红书',每项含 title + 该平台特有字段(hashtags 或 tags + description 或 body)
- cross_platform_tips: string[] 列出需要在所有平台保持一致的核心信息,以及需要适配的本地化点

只用 JSON 输出,不要加多余解释。`,
		title, angle, sourcePlatform,
	)
}
