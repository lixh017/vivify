package agents

// Demo mode pre-canned responses for the OPC "熊猫" IP. These are
// returned by Claude.DemoResponse(op) when the server is running
// without an Anthropic API key, or when a caller passes ?demo=true.
//
// The strings are kept as raw JSON (the same shape Claude returns
// in production) so the existing handler parsers (parseTopics,
// parsePostmortemStructured, plain-text humanize) flow through
// unchanged.
//
// Every sample is intentionally written in the panda IP voice
// (治愈 / 御宅 / 哲学 / 国潮) and follows the field shapes the
// upgraded prompts now request. Topics and postmortems now include
// the richer "pattern" / "axis_scores" / "evidence" fields the
// new prompts ask for.

// DemoTopicsJSON is the canned response for /ai/topics demo mode.
// Ten topics spanning 治愈 / 御宅 / 哲学 / 国潮 — at least 2-3
// hits per sub-theme so the demo shows the IP's full creative
// range without leaning on generic Chinese short-video copy.
const DemoTopicsJSON = `[
  {
    "title": "凌晨 3 点,熊猫为什么不睡",
    "angle": "御宅+治愈:深夜的孤独感用'反问自己'替代'心疼观众',把观众从'我在emo'拉到'我不是一个人'",
    "expected_performance": "深夜 00:00-03:00 是抖音活跃高峰,反问开场+具体画面同时拉停留,完播率会显著高于均值",
    "hook": "画面: 窗边熊猫对窗发呆,3 秒无台词。 旁白: 你也还没睡吧。",
    "pattern": "画面钩子",
    "voice_tags": ["御宅","治愈"]
  },
  {
    "title": "熊猫陪你度过雨夜",
    "angle": "治愈系场景:用'不解释+不收束'给观众情绪投射空间,反对'愿你被世界温柔以待'式的口号收尾",
    "expected_performance": "雨夜场景自带高完播;BGM 用雨声+轻钢琴,完播率可达赛道 TOP 10%",
    "hook": "画面: 雨打在窗玻璃,熊猫侧脸。 旁白: 今天辛苦了。",
    "pattern": "画面钩子",
    "voice_tags": ["治愈"]
  },
  {
    "title": "熊猫读《庄子》",
    "angle": "哲学入门:把'逍遥'映射到当代'内卷/精神内耗',让国学对年轻人可消化",
    "expected_performance": "知识+情绪双护城河,长尾搜索流量稳,B 站版本可扩成 3-5 分钟",
    "hook": "画面: 熊猫翻开《庄子》。 旁白: 你有没有想过——逍遥到底是什么?",
    "pattern": "三问开场",
    "voice_tags": ["哲学","国潮"]
  },
  {
    "title": "熊猫逛故宫",
    "angle": "国潮文化:蹭北京 citywalk 热点,故宫红墙做背景视觉冲击",
    "expected_performance": "citywalk 话题自带分发加权,异地观众占比预期 70%+,收藏率 > 点赞率",
    "hook": "画面: 熊猫戴朝珠走在红墙下。 旁白: 这条路,前朝的人走得更急。",
    "pattern": "画面钩子",
    "voice_tags": ["国潮"]
  },
  {
    "title": "熊猫的咖啡日记",
    "angle": "御宅治愈:中女向高互动,手冲咖啡的慢节奏自带留白",
    "expected_performance": "中女(25-40 岁女性)赛道互动率高于均值,小红书/抖音双平台都吃",
    "hook": "画面: 熊猫手冲咖啡,水流一圈一圈。 旁白: 今天,加半糖。",
    "pattern": "画面钩子",
    "voice_tags": ["御宅","治愈"]
  },
  {
    "title": "深夜熊猫的诗",
    "angle": "哲学+文学:用一首'非自己写'的小诗引出情绪,留白+不解释",
    "expected_performance": "小红书爆款潜力高(诗+慢节奏天然适合图文+15 秒);抖音版可拆 30 秒",
    "hook": "画面: 熊猫在窗前写下:月亮是别人家的灯。 旁白: (3 秒无声)",
    "pattern": "引用钩子",
    "voice_tags": ["哲学","治愈"]
  },
  {
    "title": "熊猫翻《论语》",
    "angle": "国学再解读:'温故而知新'放到 2026 还成立吗?用问题+反差切入",
    "expected_performance": "知识类长尾流量,搜索词'论语/国学/熊猫'可拉新,B 站信息密度版本可拉到 5 分钟",
    "hook": "旁白: '温故而知新'——放到 2026 的人,真的在'温'吗?",
    "pattern": "反常识开场",
    "voice_tags": ["国潮","哲学"]
  },
  {
    "title": "熊猫陪夜班司机",
    "angle": "情感+御宅:独处反而不孤独——把'陪伴'重新定义,IP 熊猫作为情绪替身",
    "expected_performance": "情感向 + 御宅 + 视觉奇观(夜景车厢)三合一,完播+评论双高",
    "hook": "画面: 夜班司机驾驶位旁,熊猫安静坐着。 旁白: 你在,就够了。",
    "pattern": "留白钩子",
    "voice_tags": ["御宅","治愈"]
  },
  {
    "title": "熊猫的节气笔记",
    "angle": "国潮+时令:跟着 24 节气更,每集 60 秒,做系列内容",
    "expected_performance": "节气自带日历流量,系列化运营拉关注;一年 24 集素材库,可复用 3 年",
    "hook": "画面: 立夏的窗台,熊猫放着薄荷茶。 旁白: 万物至此皆长大。",
    "pattern": "引用钩子",
    "voice_tags": ["国潮","治愈"]
  },
  {
    "title": "熊猫读《心经》",
    "angle": "哲学+国潮:'色即是空'和当代人的'累'有什么关系?用类比解构",
    "expected_performance": "延续'熊猫读《庄子》'系列的爆款路径,B 站长视频+抖音系列拆条双管齐下",
    "hook": "旁白: 色即是空,空即是色——当代人,为啥这么累?",
    "pattern": "反常识开场",
    "voice_tags": ["哲学","国潮"]
  }
]`

// DemoHumanizedScripts is a pool of pre-canned humanized scripts.
// Each entry already has the 停顿/语气词/留白 quirks the real
// humanize prompt is meant to introduce, so the demo can show the
// "after" state without calling the API. The first entry is the
// default; the others give variety if the demo runs multiple calls
// (we rotate based on a tiny hash of the input).
//
// Each script targets 200-400 Chinese chars (the IP's sweet spot
// for short-form 60-90s video) and lives entirely in the panda IP
// voice — no AI-style排比 / 三段式 / "让我们一起".
var DemoHumanizedScripts = []string{
	`今天的雨...下得有点久。

窗边的熊猫,就那么坐着,看着外面。

不说话。

其实有时候吧,不需要说什么,陪着就够了。

(轻轻叹气)

你今天...也辛苦了。`,

	`你有没有想过——

"逍遥"到底是什么?

(熊猫翻开《庄子》,慢慢念)

"乘天地之正,而御六气之辩,以游无穷者"

嗯...我也不太懂。

但是吧,大概意思是:别太较劲。

风往哪吹,就往哪走一会儿。`,

	`今天的咖啡,加了半糖。

(停顿)

不是因为想喝甜的。

是因为...想对自己好一点。

手冲很慢,水一圈一圈下去。

整个早上,就属于这一杯。

挺好的。`,

	`凌晨 3 点。

窗外的城市,都睡了。

熊猫坐在窗台上,没开灯。

(叹气)

你最近...是不是也常常这样。

明明累了,又不想睡。

不是不困,是...怕一觉醒来,又要重来。`,

	`画面:立夏的窗台,熊猫放着薄荷茶,旁边一卷《月令七十二候集解》。

旁白(慢):

"万物至此皆长大。"

(停顿 2 秒)

嗯...其实人也一样吧。

到了某个节气,就不再装了。

热就是热,累就是累。

不必再向谁解释。`,
}

// DemoPostmortems is a pool of pre-canned postmortem responses.
// Each is a JSON object matching the upgraded prompt's shape:
// success_factors / reusable_patterns / insights / axis_scores /
// suggestions, where the last two use the "claim | evidence |
// confidence" and "改 → 为什么 → 预期" three-segment string form
// the new prompt enforces. The first two are the originals
// (extended); the third is brand new and covers the 御宅 + 哲学
// sub-theme the originals did not.
var DemoPostmortems = []string{
	`{
  "success_factors": [
    "前 3 秒钩子够强:'窗边熊猫看雨'直接拉满治愈感,具象画面比抽象概念停留长 1.8s | 数据:5 秒跳出率仅 12%",
    "BGM 选了雨声+钢琴,氛围承接画面,无声段落(2 秒)让观众'进入'场景 | 数据:无声段落后 8 秒留存率反升 7pct",
    "旁白只有一句'今天辛苦了',留白给观众情绪投射,不强行收束 | 数据:评论高频词'治愈了我'出现 234 次"
  ],
  "reusable_patterns": [
    "治愈系场景 = 自然元素(雨/雪/晚风) + 熊猫静态画面 + 一句话旁白(≤ 12 字),三个固定槽位可替换",
    "完播率拉满靠的是'不解释',让观众自己代入,不要'我希望你们...'这类直球",
    "结尾不要 CTA,情绪没收住前任何 CTA 都是破坏;预告下一集比关注引导更自然"
  ],
  "insights": [
    "播放完成率 68% 远高于赛道均值(45%),说明节奏对了,不需要再加内容 | confidence=high",
    "评论区高频词'治愈了我/想哭/谢谢'验证情绪靶心精准 | confidence=high",
    "收藏率(8.2%) 高于点赞率(6.4%),说明这条有'保留价值'而非纯快消 | confidence=medium"
  ],
  "axis_scores": { "hook": 88, "structure": 80, "platform_fit": 78, "emotional_arc": 92 },
  "suggestions": [
    "改 → 下条做'雪夜熊猫'保持系列感(命名+视觉一致) → 为什么:验证'自然元素'这个公式是否可迁移 → 预期:完播率维持 60%+ 即可视为公式成立",
    "改 → BGM 整体下沉一档(从 -16 LUFS 到 -20 LUFS) → 为什么:目前 BGM 略亮,会从'陪伴'抢戏到'表演' → 预期:评论区情绪词占比再 +10%",
    "改 → 结尾画面再多停 1 秒(从 2 秒到 3 秒) → 为什么:给观众反应时间,留白提升评论区互动 → 预期:评论量 +15%"
  ]
}`,

	`{
  "success_factors": [
    "国潮元素(朝珠+红墙)和熊猫 IP 自带流量叠加,识别度高 1 个数量级 | 数据:点击率比纯熊猫画面高 38%",
    "蹭上 citywalk 热点,平台分发加权,推流池阈值比常规低 30% | 数据:发布 2 小时破 10w",
    "画面构图干净,故宫红墙做背景视觉冲击强,4 秒停留率高于 IP 均值 22% | 数据:封面点击率 14.2%"
  ],
  "reusable_patterns": [
    "国潮模板 = 熊猫 + 高辨识度中国元素(故宫/景德镇/敦煌) + 一句带地名的文案,三个槽位可拆解复用",
    "蹭城市热点(citywalk/特种兵旅游)能拿到额外流量分发,需标题前缀带城市名以吃搜索长尾",
    "标题结构'地名 + IP 名 + 动作'比'IP + 地名'的搜索流量高 2 倍,小红书/抖音通用"
  ],
  "insights": [
    "完播率 55%,中等偏上,适合长尾积累而非短期爆款 | confidence=high",
    "收藏率 8.2% 高于点赞率(6.1%),说明内容有'保留价值',可沉淀为合集 | confidence=high",
    "异地观众占比 73%,验证了文化输出的传播半径,北上广深+成都贡献 60% | confidence=medium"
  ],
  "axis_scores": { "hook": 75, "structure": 78, "platform_fit": 88, "emotional_arc": 70 },
  "suggestions": [
    "改 → 下一条做'熊猫逛颐和园'保持地标系列,封面统一用红墙+熊猫远景 → 为什么:形成 IP 视觉指纹,让用户滑到时 0.5 秒识别 → 预期:系列关注转化率 +20%",
    "改 → 标题前缀加'熊猫城市漫游'做成 IP 子系列,搜索词可带'熊猫+城市' → 为什么:长尾搜索流量,半年后仍可持续带量 → 预期:长尾流量占总播放 40%+",
    "改 → 加一句京味儿旁白(例:'嚯,今儿这红墙可真红') → 为什么:增加地域代入感,提升评论互动 → 预期:评论量 +25%"
  ]
}`,

	`{
  "success_factors": [
    "反问开场'你有没有想过——' + 经典文本《庄子》,知识+情绪双钩子,前 3 秒停留高于纯知识类 40% | 数据:5 秒跳出率 18%",
    "中段用'内卷/精神内耗'做现代类比,把古文映射到观众痛点,跨越知识门槛 | 数据:评论中'当代打工人'出现 189 次",
    "结尾预告'下一集讲惠子',把'看完'变成'追剧',关注转化率高于 IP 均值 60% | 数据:关注转化 3.2%"
  ],
  "reusable_patterns": [
    "哲学三问开场:无答案的深问题(≤ 20 字) + 现代痛点类比 + 软答案(≤ 30 字) + 下集钩子,四步公式可复用到任何经典文本",
    "软答案(不下判断) > 强行结论,IP 调性是'陪你一起想',不是'教你该怎么做'",
    "下集钩子比关注引导转化率高 3-5 倍,做系列化内容时务必在结尾留未解悬念"
  ],
  "insights": [
    "完播率 72%,远高于知识类赛道均值(35%),说明哲学+国潮组合打开了'长视频耐心用户' | confidence=high",
    "B 站版本(5 分钟)复投后,长尾流量 30 天内占总播放 41%,验证'深度内容'长尾价值 | confidence=high",
    "评论区二创率高(用户自发用文本+自配音),说明 IP 形象已成为 meme 模板,可继续做'熊猫读 XX'系列 | confidence=medium"
  ],
  "axis_scores": { "hook": 92, "structure": 85, "platform_fit": 82, "emotional_arc": 90 },
  "suggestions": [
    "改 → 系列化做'熊猫读 XX',每集 60-90 秒,统一片头(熊猫翻书)统一片尾(下集预告) → 为什么:已验证'哲学三问'公式可复用,做系列把单点爆款变成 IP 资产 → 预期:3 个月内关注数翻倍",
    "改 → 中段现代类比部分加 1 个具体场景(例:'996 加班回家路上') → 为什么:把抽象类比锚定到具象画面,提升观众代入 → 预期:评论质量(长评率) +30%",
    "改 → 抖音版拆成 3 条 30 秒系列(开场/类比/软答案),互相引流 → 为什么:抖音用户耐心短,长视频拆条比完整版流量高 2x → 预期:总播放 +150%,涨粉 +80%"
  ]
}`,
}

// pickDemoIndex returns a deterministic-but-rotating index for a
// given input length. It is intentionally tiny: callers want variety
// across multiple demo calls, not crypto-grade pseudo-randomness.
func pickDemoIndex(seed int, n int) int {
	if n <= 0 {
		return 0
	}
	if seed < 0 {
		seed = -seed
	}
	return seed % n
}

// DemoQualityScores is a small pool of pre-canned content-quality
// scores for the panda-IP sample script. The shape mirrors the
// QualityScoreResponse struct defined in
// internal/handlers/quality.go. The first entry corresponds to a
// "good" script; the second to a "needs work" script so the demo
// shows a range of outputs.
var DemoQualityScores = []string{
	`{
  "overall_score": 82,
  "hook_strength": 88,
  "structure": 80,
  "platform_fit": 78,
  "axis_breakdown": {
    "hook":         "画面钩子'窗边的熊猫看着雨'有具体视觉 +20;无数字/反差 +0;共 +20",
    "structure":    "两段结构 +10;有留白结尾('今天辛苦了'是悬而未决) +20",
    "platform_fit": "标题 8 字 ≤ 22 字 +25;脚本 21 字偏短(< 80) -7;抖音钩子 3 秒内出画面 +10"
  },
  "suggestions": [
    {
      "category":  "structure",
      "severity":  "medium",
      "problem":   "脚本只有 21 字,完播率高但信息密度低,长尾搜索吃不动",
      "rewrite":   "在'今天辛苦了'前加 1-2 句铺垫,例如:'今天的雨...下得有点久' (留白 + 共情)"
    },
    {
      "category":  "hook",
      "severity":  "low",
      "problem":   "前 3 秒虽然有画面,但旁白第一句才出'窗边的熊猫',可再前置 0.5 秒",
      "rewrite":   "把'画面: 雨打在窗上,熊猫侧脸'写在脚本最前,代替空镜"
    }
  ],
  "rewritten_hook": ""
}`,
	`{
  "overall_score": 58,
  "hook_strength": 42,
  "structure": 65,
  "platform_fit": 70,
  "axis_breakdown": {
    "hook":         "无具体画面 +0;无提问/反差/数字 +0;无 AI 痕迹但也无 IP 钩子特征 +5",
    "structure":    "三段式结构 -10;结尾 CTA 弱 -5;有 1 个换行(算节奏) +5",
    "platform_fit": "标题 24 字 > 22 字(抖音截断) -10;无标签 -5;脚本 95 字长度合适 +10"
  },
  "suggestions": [
    {
      "category":  "hook",
      "severity":  "high",
      "problem":   "钩子偏弱,首句是'今天想给大家讲'——典型 AI 铺垫句",
      "rewrite":   "换成具体画面:'画面: 窗边的熊猫看着雨,3 秒无台词'"
    },
    {
      "category":  "hook",
      "severity":  "high",
      "problem":   "首屏无视觉钩子,观众 1.5 秒就会滑走",
      "rewrite":   "首帧直接出熊猫特写+雨滴,旁白从'你也还没睡吧'开始"
    },
    {
      "category":  "structure",
      "severity":  "medium",
      "problem":   "三段式结构过于工整,失去 IP 的'不解释'调性",
      "rewrite":   "删掉中段第二条,只保留一个情绪转折+一个留白结尾"
    },
    {
      "category":  "platform_fit",
      "severity":  "medium",
      "problem":   "标题 24 字偏长,小红书会被截断",
      "rewrite":   "压缩到 18 字内:'雨夜窗边的熊猫,看着你' + 1 个 emoji"
    }
  ],
  "rewritten_hook": "窗边的熊猫,看着雨,一句话都没说。"
}`,
}

// DemoPlatformAdapts is a small pool of pre-canned 3-platform
// adaptations for the panda IP. The first entry is the default;
// the second provides variety if the demo runs multiple calls.
var DemoPlatformAdapts = []string{
	`{
  "adaptations": {
    "抖音": {
      "title":       "雨夜窗边的熊猫",
      "hashtags":    ["#治愈系", "#熊猫日记", "#深夜emo", "#情绪释放"],
      "description": "窗边的熊猫,看着雨,一句话都没说。",
      "first_3s":    "雨打窗玻璃特写 → 熊猫侧脸 → 旁白第一句"
    },
    "哔哩哔哩": {
      "title":       "【熊猫日记 EP03】雨夜窗边,我和自己聊了聊",
      "description": "今晚的雨下得有点久。窗边的熊猫,看着外面,不说话。其实有时候,陪着就够了。",
      "tags":         ["治愈", "熊猫", "深夜", "情绪", "解压", "慢节奏", "氛围感", "独处"],
      "cover_hint":   "熊猫侧脸 + 雨滴特写,色调冷蓝 + 暖灯"
    },
    "小红书": {
      "title":       "雨夜🐼窗边发呆",
      "body":         "今晚的雨下得有点久。\\n\\n窗边的熊猫,就那么坐着,看着外面。\\n\\n不说话。其实有时候吧,不需要说什么,陪着就够了。",
      "tags":         ["#治愈", "#独处", "#深夜", "#情绪", "#氛围感"]
    }
  },
  "cross_platform_tips": [
    "核心信息'陪着就够了'在三个平台都保留,作为情绪锚点",
    "IP 角色(熊猫)必须出现在画面或文字中,不要替换成其他形象",
    "小红书可加 emoji,抖音不加,哔哩哔哩看 UP 主风格",
    "BGM 选雨声+轻钢琴,适配三个平台的氛围调性"
  ]
}`,
	`{
  "adaptations": {
    "抖音": {
      "title":       "熊猫读《庄子》",
      "hashtags":    ["#国潮", "#哲学", "#熊猫", "#庄子", "#文化"],
      "description": "熊猫翻开《庄子》:何为逍遥?",
      "first_3s":    "熊猫手翻书页特写 → 书名大字 → 旁白第一句"
    },
    "哔哩哔哩": {
      "title":       "【熊猫读庄子】乘天地之正,而御六气之辩——什么是真正的逍遥?",
      "description": "熊猫翻开《庄子》,慢慢念:'乘天地之正,而御六气之辩,以游无穷者'。我也不太懂,但大概意思是:别太较劲。",
      "tags":         ["哲学", "庄子", "国学", "熊猫", "逍遥", "慢节奏", "知识", "文化"],
      "cover_hint":   "熊猫+毛笔字效果,标题'逍遥'用书法字体"
    },
    "小红书": {
      "title":       "熊猫翻《庄子》🐼",
      "body":         "你有没有想过——'逍遥'到底是什么?\\n\\n熊猫翻开《庄子》,慢慢念:乘天地之正,而御六气之辩,以游无穷者。\\n\\n我也不太懂,但是吧,大概意思是:别太较劲。",
      "tags":         ["#哲学", "#庄子", "#国学", "#熊猫", "#治愈"]
    }
  },
  "cross_platform_tips": [
    "原文'乘天地之正,而御六气之辩'必须保留,这是 IP 的内容护城河",
    "三个平台标题都要带'熊猫'关键词,强化 IP 识别",
    "哔哩哔哩适合展开哲学讨论,小红书偏情绪共鸣,抖音偏钩子",
    "B 站可用长视频(3-5 分钟),抖音拆成 15-30 秒系列"
  ]
}`,
}
