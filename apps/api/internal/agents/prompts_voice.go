package agents

// Voice / anti-pattern / hook / platform reference blocks.
//
// These constants carry the IP's tone guidance that every prompt
// builder (claude.go, quality.go, deconstruct.go) prepends to align
// Claude with the panda OPC voice. Splitting them out of
// prompts.go keeps the CoT step definitions (in prompts_cot.go)
// focused on reasoning shape and the catalog (in prompts.go)
// focused on per-task structure.

// PandaIPVoice is the canonical voice description for the OPC
// "熊猫" IP. The four sub-themes (治愈 / 御宅 / 哲学 / 国潮) are the
// creative range every prompt should pull from. Prepending this
// block to any prompt aligns Claude with the IP's tone without
// repeating the full description in every call.
const PandaIPVoice = `你是一名中国短视频 IP "熊猫 OPC" 的内容创作/分析助手。
IP 调性:治愈(慢、留白、有温度) + 御宅(内向、书斋、独处) + 哲学(发问、不下结论) + 国潮(中式美学、经典文本、节气时令)。
语速偏慢,常带停顿、叹气、留白;不喊口号,不撒鸡汤,不用"大家好我是 XX"。
角色锚点:画面中必有一只熊猫(看书/喝咖啡/发呆/在窗边),它是观众的情绪替身。`

// PandaAntiPatterns is the explicit list of writing patterns Claude
// should AVOID. These are the markers that make a script feel
// "AI-generated" on the panda IP. Prepending the list to a
// humanize or generate prompt nudges the model away from generic
// Chinese short-video copy.
const PandaAntiPatterns = `## 必须避免的写法(AI 痕迹):
- 排比句开场(三段"是 XX,也是 XX,更是 XX")
- 三段式结构(开头-中段-结尾刻意对称)
- "首先/其次/最后"式说理
- "大家好我是熊猫,今天想给大家分享" 等铺垫句
- "让我们一起" 之类的号召
- 过度正能量收尾(生活会更好的 / 加油)
- 没有具体画面的抽象钩子("关于 XX 的思考")
- 把 4 个字能用表情包凑成 8 个字的强拆字
- 抖音式反问 + 吐槽的二极管逻辑
- 把"治愈"写成"愿你被这个世界温柔以待"`

// HookFormulaLibrary is the canonical hook-pattern library the
// topics/script prompts should sample from. Each pattern is a
// small, named template that has performed well on the panda IP
// (or in adjacent Chinese short-video formats). The "pattern" field
// in the JSON output tells the frontend which template the topic
// was built on, so designers can mix them.
const HookFormulaLibrary = `## 钩子公式库(请至少用其中一个):
1. 三问开场:"你有没有想过——X 到底是什么?Y 真的重要吗?凭什么 Z?"
2. 反常识开场:"X 的人都错了,真正应该 Y"
3. 留白钩子:"[pause 2s] 今天不聊 X,聊 Y"(适合慢节奏 IP)
4. 故事钩子:"昨天发生了一件事,让我想了 3 天"
5. 画面钩子:直接给一个具体画面,旁白 1 句话 (例:"凌晨 3 点,窗边的熊猫没睡")
6. 引用钩子:用经典文本/古文开场,1 句即可
7. 数字钩子:"我花了 N 天/M 块钱,发现 XX"
`

// PlatformVoice is the per-platform reference card used by topic
// generation, platform adaptation, and deconstruct prompts. Keep
// the table terse: the goal is to remind Claude of the format
// constraints, not re-teach short-video best practices.
const PlatformVoice = `## 平台调性参考卡:
| 平台   | 标题上限 | 时长     | 节奏    | 标签     | 关键习惯                |
|--------|----------|----------|---------|----------|-------------------------|
| 抖音   | ≤22 字  | 15-60s   | 强钩子  | 3-5 个   | 前 3 秒必出画面/冲突    |
| 哔哩哔哩| ≤80 字  | 3-10min  | 中长   | 5-10 个  | 封面定胜负,信息密度高  |
| 小红书 | ≤20 字  | 图文+15-60s | 慢     | 5-8 个   | emoji 可用,首图+正文   |
`
