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
// deconstructions. The first (and currently only) entry analyses a
// fictional panda IP video "深夜熊猫读庄子 第3集" with 100万+ 播放
// so the demo shows a realistic OpusClip-style breakdown without
// any prompt round-trip.
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
  "overall_score": 87
}`,
}

// DemoViralFormulas is a small pool of pre-canned viral-formula
// extractions. The first (and currently only) entry is "哲学三问开场法"
// which is the formula the panda IP series leans on for the
// 深夜熊猫读庄子 / 熊猫读《心经》 / 熊猫读《道德经》 sub-series.
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
  ]
}`,
}
