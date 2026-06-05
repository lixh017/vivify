package agents

// Centralized prompt catalog for the OPC "熊猫" IP.
//
// All prompt builders (claude.go, quality.go, deconstruct.go) draw
// from the constants defined here. Splitting them out keeps the
// individual prompt strings focused on per-task structure and
// examples, while the shared panda-IP voice, anti-patterns, hook
// library, and platform reference card live in one place that is
// easy to evolve.
//
// The strings are deliberately Chinese-heavy because the IP's voice
// is Chinese; embedding them as raw string literals (rather than
// loading from a JSON/YAML catalog) keeps the build hermetic and
// lets `gofmt` keep everything in a single file.

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

// OutputJSONOnly is the boilerplate tail every prompt appends so
// Claude returns parseable JSON without markdown fences or
// preamble. Putting it in one place means a parser change (e.g.
// "now also allow json fences") only has to happen once.
const OutputJSONOnly = `只用 JSON 输出,不要加任何解释、不要 markdown 代码块、不要开场白。`

// CoTStepsTopics is the chain-of-thought preamble the topics prompt
// asks Claude to perform before producing the final array. Explicit
// steps reduce the "five generic topics in the same voice" failure
// mode and surface reasoning the user can audit if a topic is
// rejected.
const CoTStepsTopics = `## 思考步骤(在生成前先在脑内完成,不要输出思考):
1. 拆解种子概念:核心意象 + 情绪锚点 + 至少 2 个反差方向
2. 在 治愈/御宅/哲学/国潮 四个维度中各选 1 个落点(可以重复,但要说明)
3. 为每个选题匹配最合适的钩子公式(从上述 7 个里选)
4. 想清楚"为什么这条可能爆"——给出数据/情绪/时令的具体推理
5. 写出标题+角度+钩子台词+预期表现+对应公式
`

// CoTStepsQuality is the chain-of-thought preamble for the quality
// scoring prompt. Asks Claude to enumerate the evidence for each
// score before settling on a number, so suggestions are anchored
// to specific lines in the script.
const CoTStepsQuality = `## 思考步骤:
1. 先看标题,数一下字数、是否提问、是否有具体画面词(只列出真实存在的证据)
2. 再看脚本,标注:第几段是钩子 / 中段有几个转折 / 结尾是否有 CTA / 有无明显 AI 痕迹
3. 对照平台惯例,标出该作品符合/违反平台调性的位置
4. 综合三轴分数,加权得出 overall_score(权重:hook 40% / structure 35% / platform_fit 25%)
5. 每条建议必须给出'问题→改法'两行,不能只说'建议加强'这种空话
`

// CoTStepsDeconstruct is the chain-of-thought preamble for the
// video-deconstruction prompt. Asks Claude to walk through the
// video second-by-second before abstracting patterns, so the
// beat-by-beat analysis is anchored to timestamps.
const CoTStepsDeconstruct = `## 思考步骤:
1. 把视频按 0%/15%/35%/60%/85%/100% 六个采样点切片,每个点写一句画面+一句旁白
2. 在每个采样点上识别钩子/铺垫/转折/收束中的哪个
3. 写出情感曲线(从 6 个采样点向量化)
4. 抽象出 3-5 个真正可复用的模式(可复用的意思是换主题也能套)
5. 给出每个平台的具体建议(基于 6 个采样点的内容,不是泛泛而谈)
`
