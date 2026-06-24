---
name: vivify-panda-character
description: Use this skill when generating 峰哥 panda IP content for OPC Phase 1 — including scripts, video/image assets, voice/script voice rules, and 4-tone adaptation (治愈/御宅/哲学/国潮). Triggers on "panda" / "熊猫" / "峰哥" / "fengge" content creation, 可灵/即梦/Suno prompts, dialogue writing, or any reference to the Phase 1 IP bible.
---

# Panda IP Character Bible (峰哥 / Fengge)

OPC Phase 1 的**唯一主 IP**,所有脚本/资产/平台内容必须围绕此角色。完整设计见 `docs/superpowers/specs/2026-06-03-opc-phase1-ip-design.md` §4.1。

## 1. 角色定位

| 维度 | 设定 |
|------|------|
| **名字** | **峰哥** (Fengge) — 不是"小白" |
| **物种** | 成年熊猫,会站立、双手做手势 |
| **画风** | **国潮 + 色彩靓丽**(非暗色治愈系、非国风水墨;走鲜明对比、暖橙/朱红/翠绿/宝蓝调) |
| **口头禅** | "嘿,来了啊" / "走着" / "来一口" |
| **世界观** | 山中竹林小院,会读诗/喝茶/发呆,但**不沉闷** |

**色彩基调**(关键差异 vs 暗色调治愈风):
- 主色:朱红 `#C73E1D`、暖橙 `#E89B45`、翠绿 `#3B8C5A`、宝蓝 `#1F5FA8`
- 辅色:米白 `#F5F0E1`、墨黑 `#1A1A1A`
- **不允许**:暗灰、莫兰迪、低饱和
- **允许**:纯色对比、撞色(红+绿、蓝+橙)

## 2. 视觉规范(可灵/即梦 prompt 必含)

**毛色 RGB 近似**:黑 `rgb(26,26,26)` 眼周/耳/四肢;白 `rgb(245,240,225)` 脸/躯干。**眼神**:半阖、温润、带笑意(不要直视镜头;不要空洞)。
**体型**:头身比约 1:1.2(圆胖,不瘦)。**衣着**:**默认 5 套服饰之一**(见 §3)。

**5 套服饰**(v1 设定,v1.1 待设计师细化):

| ID | 名称 | 颜色主调 | 适用 tone | 标志性元素 |
|----|------|---------|----------|-----------|
| `outfit_hufu_red` | 朱红汉服 | 朱红 + 米白 | 国潮 | 圆领宽袖汉服、革带、玉佩 |
| `outfit_changshan_blue` | 宝蓝长衫 | 宝蓝 + 银 | 国潮 / 哲学 | 立领长衫、铜扣、折扇 |
| `outfit_workwear_orange` | 暖橙短褂 | 暖橙 + 米白 | 治愈 / 御宅 | **唐宋短打** (立领对襟短褂, 盘扣, 卷袖, 国潮腰封) — **绝不能画成现代清洁工/建筑工制服**! 绝对不要反光条/工具口袋/工装裤/Logo 徽章/工号牌/尼龙搭扣/金属拉链/护目镜挂带;只有盘扣 + 棉麻布料 + 国潮纹样 |
| `outfit_robe_green` | 翠绿僧袍 | 翠绿 + 米白 | 哲学 / 治愈 | 宽袖僧袍、芒鞋、念珠 |
| `outfit_jacket_redwhite` | 红白运动夹克 | 朱红 + 米白 | 御宅 | 立领运动夹克、白 tee、运动鞋 |

**默认 5 套按 round-robin 循环**(同一集视频不重复,跨集不连续)。

**道具白名单**:折扇、茶盏、山水卷轴、竹杖、樱花、红叶、毛绒斗篷、围炉、算盘、铜钱、葫芦、古书、围棋子、油纸伞、面人、糖葫芦。

**场景白名单**:竹林小院、山水卷轴前、雨后青苔、月下窗棂、茶室、集市戏台、老戏楼、围炉夜话、霓虹街头(国潮 mix 现代感)、道观、寺庙、戏台。

**场景黑名单**:`❌` 现代写字楼、夜店、健身房、任何"网红打卡"、不露爪(爪子永远藏于袖/扇后)、不攻击性姿势、不卖惨流泪、不穿西装。

## 3. 4-Tone 适应矩阵(同一只熊猫,4 种取景)

| Tone | 适用场景 | 语气关键词 | 视觉偏移 |
|------|---------|-----------|---------|
| **治愈** | 情绪疏导、每日金句 | "嘿,来了啊"、"坐着聊" | 暖光、茶烟、雨声、围炉 |
| **御宅** | 陪伴向、独处向 | "我也不爱出门"、"那就一起待着" | 室内小景、毛绒毯、深夜台灯 |
| **哲学** | 反鸡汤、存在主义 | "但真的是这样吗" | 月下、山水、远景 |
| **国潮** | 古诗、传统美学、节日 | "古人云"、"你看那山" | 折扇、卷轴、红叶、樱花 |

> 切换 tone = 改语气和场景,**不换角色不换服饰**。同一只峰哥 + 4 种滤镜 + 5 套服饰。

## 4. 声音 / 脚本声音规则

**声音设定**:低沉稳重但**带笑意**的男声(30-40 岁感,治愈系但**不悲情**),普通话为主,语速慢、留白多但**有节奏感**(不是平铺直叙)。

**句式规则**:
- 短句为主(每句 ≤ 15 字)
- 多用反问("你说,这是不是有点奇怪?")
- 大量留白(段间停顿 1-2 秒)
- 自言自语感("嗯……我今天想了想……")
- **特有口语词**: "嘿" "啊" "得" "呗" "走着" "来一口" (国潮 + 治愈的混搭语气)

**禁用词**:
- `❌` "加油"、"你一定可以" 等鸡汤
- `❌` "失败是成功之母" 等毒鸡汤
- `❌` 任何"所以你应该……"的强行说教
- `❌` 感叹号堆砌、emoji 替代思考
- `❌` 沉闷的"唉"、"叹气"开头(国潮不要这种调)

**对话示例**(国潮 tone,穿朱红汉服):
> "嘿,来了啊。
> 我刚沏了壶茶,你要是路过,正好。
> 外面风大——你听,竹子在响。
> 我以前觉得竹子是在跟风。
> 后来发现,它在跟**自己**的根说话。"

## 5. 可灵 / 即梦 Prompt 模板

```text
一只成年熊猫(峰哥),头身比 1:1.2 圆胖身材,
面部白 rgb(245,240,225) 眼周黑 rgb(26,26,26),
眼神半阖带笑意不直视镜头,
身穿[outfit_hufu_red | outfit_changshan_blue | outfit_workwear_orange | outfit_robe_green | outfit_jacket_redwhite],
手持[折扇|茶盏|竹杖|铜钱|葫芦]道具,
站在[竹林小院|月下窗棂|茶室|集市戏台|老戏楼],
[暖光|月光|霓虹]氛围,
**国潮 + 色彩靓丽**(朱红/暖橙/翠绿/宝蓝撞色,非暗色调、非水墨),
不露爪,不攻击性姿势,无文字,
9:16 竖屏构图
```

> 每次生成必含:毛色 RGB、眼神描述、outfit ID、道具、场景白名单内词、撞色指示。
> **`panda_visual_anchor: fengge_v1`**(给 IP 一致性 check 用)必嵌入每条 prompt。

## 6. Cross-References

本 skill 是以下 skill 的**唯一来源**——它们必须引用本文件,不可自行定义角色:

- `vivify-script-generation` — 脚本生成时调用本 skill 的声音/句式规则
- `vivify-asset-router` (v2) + `vivify-provider-***` (v2) — 可灵/即梦 prompt 必须使用本 skill 的视觉规范
- `vivify-scene-decomposition` — 拆解爆款/选题时,角色一致性以本 skill 为准

修改本 skill 前先检查下游 3+ 个 skill 的引用;修改后必须同步更新它们。

**注意**:v1 的 `vivify-asset-generation` skill 引用"小白"。**v2 升级时此引用必须改"峰哥"**。

## 6. 角色一致性 (Reference Image System)

为保证**短剧多集同只熊猫**,使用 Seedream `reference_image` 机制。

**Canonical images** (committed in `vivify-panda-plugin/reference/`):

| 文件 | 服饰 | 用途 |
|---|---|---|
| `panda-canonical-zh-red.jpg` | 朱红汉服 | **默认主参考**,所有 shot 默认走这张保 identity |
| `panda-canonical-blue-changsan.jpg` | 宝蓝长衫 | 备选 outfit swatch |
| `panda-canonical-warm-orange.jpg` | 暖橙短褂 | 备选 outfit swatch |

**Prototype 准则 (3 张图共用一套)**:
- **姿势**: 站姿正面 / 3/4 侧脸,**中性**(不要明显的"笑露舌"或"闭眼")
- **表情**: 半阖眼 + 微微笑意,不直视镜头
- **背景**: 简洁米色渐变,**不**要月亮 / 樱花 / 茶案 / 道具 — 让身份锚点干净
- **不持道具**: 道具交给 prompt,reference 只保 identity
- **3 张同 pose 同表情** — 只是 outfit 不同,这样 reference 切换不会引入姿势漂移

**机制**: inline base64 (no URL, no expiry)
- `render_episode.py` 读本地 jpg → base64 → `data:image/jpeg;base64,...` → Seedream API body
- Seedream 接受 `reference_image` 既支持 URL 也支持 base64 (火山方舟官方支持)
- **没有 24h TTL 问题** — jpgs 在 git 里,渲染完全 self-contained,不需要外网 round-trip 拿 reference
- 渲染大 body (>64KB) 自动走 tmpfile,避开 argv E2BIG

**为什么 outfit/scene 不在 reference 里**:
- Seedream reference_image 主要保 identity,不一定保 outfit
- outfit/scene 在 prompt 里(5 套 round-robin + 场景白名单)
- 同只 panda 穿不同 outfit/不同场景 = 短剧自然

**Story 调整**:
- 同只 panda + 不同 outfit + 不同 scene + 不同 voice tone = 不同 episode
- 由 `panda_topic` / `panda_script` / `panda_storyboard` 三个 MCP tool 控制 storyline
- L1 步骤不变(同只 panda),只换故事背景

**真实 LoRA** (未来):
- 短剧 ≥ 5 集时考虑训练 Seedream LoRA
- 目前 inline-base64 reference_image 是 best-effort 一致性
