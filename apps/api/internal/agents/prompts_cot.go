package agents

// Chain-of-thought step blocks.
//
// These are the "think before you answer" preambles the per-task
// prompt builders append so Claude enumerates the evidence or
// reasoning before producing the final JSON output. Splitting
// them out of prompts.go keeps the voice blocks (in
// prompts_voice.go) focused on tone and the per-task examples
// (in prompts.go) focused on shape.

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
