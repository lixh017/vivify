package agents

// Demo mode pre-canned responses for the OPC "熊猫" IP. These are
// returned by Claude.DemoResponse(op) when the server is running
// without an Anthropic API key, or when a caller passes ?demo=true.
//
// The strings are kept as raw JSON (the same shape Claude returns in
// production) so the existing handler parsers (parseTopics,
// parsePostmortemStructured, plain-text humanize) flow through
// unchanged.

// DemoTopicsJSON is the canned response for /ai/topics demo mode.
// Five topics spanning 治愈 / 御宅 / 哲学 / 国潮 to cover the IP's
// full creative range during a demo without needing real prompts.
const DemoTopicsJSON = `[
  {
    "title": "熊猫陪你度过雨夜",
    "angle": "雨天治愈系场景",
    "expected_performance": "配雨声+轻音乐,完播率高",
    "hook": "画面:窗边熊猫看雨,旁白:今天辛苦了"
  },
  {
    "title": "熊猫读《庄子》",
    "angle": "哲学入门",
    "expected_performance": "知识类长尾流量",
    "hook": "熊猫翻开书,问:何为逍遥?"
  },
  {
    "title": "熊猫逛故宫",
    "angle": "国潮文化",
    "expected_performance": "蹭北京citywalk热点",
    "hook": "熊猫戴朝珠,走在红墙下"
  },
  {
    "title": "熊猫的咖啡日记",
    "angle": "御宅治愈",
    "expected_performance": "中女向高互动",
    "hook": "熊猫手冲咖啡,旁白:今天加半糖"
  },
  {
    "title": "深夜熊猫的诗",
    "angle": "哲学+文学",
    "expected_performance": "小红书爆款潜力",
    "hook": "熊猫在窗前写下:月亮是别人家的灯"
  }
]`

// DemoHumanizedScripts is a small pool of pre-canned humanized
// scripts. Each entry already has the 停顿/语气词/留白 quirks the
// real humanize prompt is meant to introduce, so the demo can show
// the "after" state without calling the API. The first entry is the
// default; the others give variety if the demo runs multiple calls
// (we rotate based on a tiny hash of the input).
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
}

// DemoPostmortems is a small pool of pre-canned postmortem responses.
// Each is a JSON object matching the shape the real prompt asks for
// (success_factors / reusable_patterns / insights / suggestions) so
// the existing parsePostmortemStructured parser just works.
var DemoPostmortems = []string{
	`{
  "success_factors": [
    "前 3 秒钩子够强:窗边熊猫看雨直接拉满治愈感",
    "BGM 选了雨声+钢琴,氛围承接画面",
    "旁白只有一句'今天辛苦了',留白给观众情绪投射"
  ],
  "reusable_patterns": [
    "治愈系场景=自然元素(雨/雪/晚风)+熊猫静态画面+一句话旁白",
    "完播率拉满靠的是'不解释',让观众自己代入",
    "结尾不要 CTA,情绪没收住前任何 CTA 都是破坏"
  ],
  "insights": [
    "播放完成率 68% 远高于赛道均值(45%),说明节奏对了",
    "评论区高频词:'治愈了我','想哭','谢谢',验证情绪靶心",
    "5 秒跳出率仅 12%,前 3 秒钩子工作良好"
  ],
  "suggestions": [
    "下条可以试'雪夜熊猫'保持系列感",
    "BGM 可以再下沉一档,目前略亮",
    "结尾画面可以再多停 1 秒,给观众反应时间"
  ]
}`,

	`{
  "success_factors": [
    "国潮元素(朝珠+红墙)和熊猫 IP 自带流量叠加",
    "蹭上 citywalk 热点,平台分发加权",
    "画面构图干净,故宫红墙做背景视觉冲击强"
  ],
  "reusable_patterns": [
    "国潮模板=熊猫+具有辨识度的中国元素(故宫/景德镇/敦煌)+一句文案",
    "蹭城市热点(citywalk/特种兵旅游)能拿到额外流量",
    "标题用地名+IP 名,搜索流量长尾稳"
  ],
  "insights": [
    "完播率 55%,中等偏上,适合长尾积累",
    "收藏率 8.2% 高于点赞率,说明内容有'保留价值'",
    "异地观众占比 73%,验证了文化输出的传播半径"
  ],
  "suggestions": [
    "下一条可以做'熊猫逛颐和园'保持地标系列",
    "标题前缀'熊猫城市漫游'可以做成 IP 子系列",
    "可以加一句京味儿旁白增强代入感"
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
