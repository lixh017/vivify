package agents

// Demo mode pre-canned responses for the OPC video-deconstruction
// surface (POST /ai/deconstruct + POST /ai/viral-formula). Returned
// by Claude.DemoResponse when the server is running without an
// Anthropic API key or when the caller passes ?demo=true.
//
// The strings are kept as raw JSON (same shape Claude returns in
// production) so the existing handler parser flows through
// unchanged.

// DemoDeconstructs is a small pool of pre-canned video
// deconstructions. The first entry analyses a fictional panda IP
// video "深夜熊猫读庄子 第3集" with 100万+ 播放; the second covers
// a panda IP 治愈系 short "凌晨 3 点的窗边" so the demo shows two
// different structural patterns side by side.
var DemoDeconstructs = []string{
	`{
  "hook": {
    "type": "curiosity_gap",
    "text": "你有没有想过——逍遥到底是什么?",
    "analysis": "用第二人称提问+破折号停顿,把观众从滑动状态拉回沉思状态,无答案的问题打开了认知缺口",
    "strength": 92
  },
  "structure": {
    "pattern": "hook-problem-solution",
    "beats": [
      {"time_pct": 0,  "role": "setup",   "description": "熊猫在窗边翻开《庄子》,旁白抛出'逍遥是什么'"},
      {"time_pct": 15, "role": "tension", "description": "原文段落'乘天地之正,而御六气之辩',语速放慢,氛围加深"},
      {"time_pct": 35, "role": "tension", "description": "用现代生活类比'内卷/精神内耗',把古文映射到观众痛点"},
      {"time_pct": 60, "role": "payoff",  "description": "给出一个软答案:别太较劲,风往哪吹就走一会儿"},
      {"time_pct": 85, "role": "callback","description": "熊猫合上书,回到窗边,与开场画面形成闭环"},
      {"time_pct": 95, "role": "callback","description": "结尾留一句'下一集讲惠子',预告下一集的钩子"}
    ],
    "pacing": "slow",
    "density": 38
  },
  "cta": {
    "present": true,
    "type": "subscribe_with_teaser",
    "placement": "late"
  },
  "emotional_arc": [
    {"time_pct": 0,  "emotion": "curiosity",   "intensity": 70},
    {"time_pct": 15, "emotion": "curiosity",   "intensity": 85},
    {"time_pct": 35, "emotion": "reflection",  "intensity": 75},
    {"time_pct": 60, "emotion": "reflection",  "intensity": 90},
    {"time_pct": 80, "emotion": "peace",       "intensity": 88},
    {"time_pct": 100,"emotion": "peace",       "intensity": 80}
  ],
  "reusable_patterns": [
    "用问题开场引发好奇,问题不要给答案",
    "慢节奏留白让观众思考,BGM 选低频钢琴/雨声",
    "古文/经典文本+现代痛点映射,跨越知识门槛",
    "结尾预告下一集,把'看完'变成'追剧'",
    "首尾画面形成视觉闭环(开场和结尾同一构图)"
  ],
  "platform_fit_notes": {
    "抖音":     "适合 60-90 秒,字幕要大,前 3 秒画面要稳,庄子原文可竖排显示加视觉差异",
    "哔哩哔哩": "适合 3-5 分钟版本,展开'内卷'类比 30-60 秒,封面用熊猫+毛笔字效果最好",
    "小红书":   "适合图文+15 秒视频混排,首图放熊猫翻书特写+原文金句,正文 200 字内"
  },
  "overall_score": 87,
  "score_evidence": [
    "完播率 72% 显著高于知识类赛道均值 35%,验证节奏对了",
    "钩子强度 92 来自'无答案的深问题'公式,在 IP 历史 TOP 5 中排第 2",
    "中段用'内卷'类比,评论区 189 次提及'当代打工人',跨越知识门槛成功",
    "结尾下集钩子(惠子)把单点爆款变成系列,关注转化 3.2% 高于 IP 均值 60%"
  ]
}`,
	`{
  "hook": {
    "type": "visual",
    "text": "(无台词,3 秒静音画面:雨打窗玻璃,熊猫侧脸)",
    "analysis": "用静音+具象画面开场,不提问不解释,把观众从'滑动'状态拉进'陪你看雨'的状态,IP 调性命中'不解释'",
    "strength": 88
  },
  "structure": {
    "pattern": "story-loop",
    "beats": [
      { "time_pct": 0,  "role": "setup",    "timestamp_sec": 0,   "description": "雨打窗玻璃,空镜 2 秒,熊猫未入画" },
      { "time_pct": 15, "role": "setup",    "timestamp_sec": 9,   "description": "熊猫入画,侧脸,无台词,BGM 钢琴起" },
      { "time_pct": 35, "role": "tension",  "timestamp_sec": 21,  "description": "旁白:今天的雨...下得有点久,语速放慢" },
      { "time_pct": 60, "role": "payoff",   "timestamp_sec": 36,  "description": "旁白:窗边的熊猫,就那么坐着,看着外面。停顿 2 秒" },
      { "time_pct": 85, "role": "callback", "timestamp_sec": 51,  "description": "旁白:其实有时候吧,不需要说什么,陪着就够了。" },
      { "time_pct": 100,"role": "callback", "timestamp_sec": 60,  "description": "旁白(轻):你今天...也辛苦了。结尾画面静止 3 秒" }
    ],
    "pacing": "slow",
    "density": 22
  },
  "cta": {
    "present":   false,
    "type":      "none",
    "placement": "none"
  },
  "emotional_arc": [
    { "time_pct": 0,   "emotion": "calm",      "intensity": 60 },
    { "time_pct": 15,  "emotion": "calm",      "intensity": 70 },
    { "time_pct": 35,  "emotion": "longing",   "intensity": 75 },
    { "time_pct": 60,  "emotion": "longing",   "intensity": 85 },
    { "time_pct": 85,  "emotion": "warmth",    "intensity": 92 },
    { "time_pct": 100, "emotion": "warmth",    "intensity": 88 }
  ],
  "reusable_patterns": [
    "静音开场:前 2-3 秒不配音,用纯画面+环境音把观众拉进场景",
    "三段式留白:第一段画面(空镜) → 第二段入画+慢旁白 → 第三段一句情绪锚点收束",
    "结尾不 CTA:治愈系内容 CTA 是破坏,情绪未收住前任何引导都多余",
    "旁白不超过 5 句,每句 ≤ 15 字,长句拆成两行用停顿"
  ],
  "platform_fit_notes": {
    "抖音":     "60 秒最合适,字幕要大,前 3 秒空镜不能换 B-roll,否则破坏氛围",
    "哔哩哔哩": "可扩成 3 分钟'雨夜合集',加 30 秒城市空镜+咖啡制作过程",
    "小红书":   "拆成 9 图图文,首图熊猫侧脸+雨滴,正文 200 字内,标签 #治愈系"
  },
  "overall_score": 89,
  "score_evidence": [
    "完播率 68% 显著高于治愈系赛道均值 45%,节奏验证成功",
    "无 CTA 反而带来评论量高于均值 40%(观众主动在评论区'续写'内容)",
    "BGM 钢琴+雨声在 5 秒后留存反升 7pct,说明静音开场为 BGM 铺垫了情感空间"
  ]
}`,
}

// DemoViralFormulas is a small pool of pre-canned viral-formula
// extractions. The first entry is "哲学三问开场法" used by the
// 深夜熊猫读庄子 / 熊猫读《心经》 / 熊猫读《道德经》 sub-series.
// The second is "治愈系留白收束法" used by the 雨夜 / 雪夜 / 咖啡日记
// short-form 治愈系 content.
var DemoViralFormulas = []string{
	`{
  "formula_name": "哲学三问开场法",
  "variables": [
    {"name": "theme",         "example": "庄子·逍遥游",                                   "weight": "high"},
    {"name": "deep_question", "example": "逍遥到底是什么?",                                "weight": "high"},
    {"name": "soft_answer",   "example": "别太较劲,风往哪吹就走一会儿",                    "weight": "medium"},
    {"name": "hook_back",     "example": "下一集讲惠子",                                   "weight": "medium"}
  ],
  "steps": [
    "选一个 IP 主题(经典文本/哲学/国学)",
    "用 1 个深问题开场,不超过 20 字,问题必须没有标准答案",
    "用软答案 + 故事/类比展开,把抽象概念落到当代生活痛点",
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
}`,
	`{
  "formula_name": "治愈系留白收束法",
  "one_liner":    "静音画面 + 慢旁白 + 一句情绪锚点结尾,全程不解释、不 CTA、不收束",
  "variables": [
    { "name": "ambient_element", "example": "雨/雪/晚风/夜灯",                            "weight": "high",   "why": "自然元素自带情绪基线,观众进入场景几乎零成本" },
    { "name": "panda_pose",      "example": "窗边对窗发呆/手冲咖啡/翻书静坐",            "weight": "high",   "why": "静态画面比动态画面更易情绪投射,熊猫是观众的情绪替身" },
    { "name": "ambient_sound",   "example": "雨声 + 低频钢琴",                            "weight": "high",   "why": "BGM 是氛围 80%,人声只承担最后一句情绪锚点" },
    { "name": "one_line_anchor", "example": "今天辛苦了 / 陪着就够了",                    "weight": "medium", "why": "结尾不收束,但需要 1 句'共情锚点'给观众带走的情绪" },
    { "name": "no_cta",          "example": "全文不提'关注/评论/点赞'",                   "weight": "medium", "why": "CTA 会破坏情绪;观众主动续写才是治愈系内容的复利" }
  ],
  "steps": [
    "选一个自然元素(雨/雪/夜灯/风)作为画面 + BGM 主轴",
    "熊猫静态入画,前 2-3 秒静音,只用环境音",
    "旁白 3-5 句,每句 ≤ 15 字,中间加停顿('...''(叹气)')",
    "结尾 1 句情绪锚点(≤ 8 字),不下判断不号召",
    "画面静止 2-3 秒再黑场,给观众'带走'情绪的时间",
    "全程不出现 CTA、关注引导、'让我们一起'等口号"
  ],
  "example_application": "熊猫的雪夜:画面空镜 2 秒(雪落在窗)→ 熊猫入画侧脸 → 旁白'雪...下了一夜了' → 停顿 → '你那边,冷吗?' → 静止 3 秒黑场",
  "variations": [
    "城市夜归:凌晨出租车后座,熊猫看窗外霓虹,旁白'你在路上,我在窗里'",
    "独居咖啡:周日早上手冲,旁白'整个早上,属于这一杯',结尾不提'周末'",
    "旧物陪伴:熊猫翻看旧明信片,旁白'有些东西,不用,就很好'",
    "雨天书店:熊猫在书店靠窗位,旁白'翻到哪页,就看哪页',BGM 雨声",
    "深夜阳台:熊猫站在阳台,旁白'城市睡了,我没',无结尾画面静黑"
  ],
  "weight_notes": {
    "low":    "no_cta 可适度放开(系列化内容最后 1 集可加片尾预告,但仍不提关注)",
    "medium": "one_line_anchor 可省略,但有锚点会让评论区情绪浓度高 30%",
    "high":   "ambient_element + panda_pose + ambient_sound 缺一不可;否则不构成'治愈'"
  }
}`,
}
