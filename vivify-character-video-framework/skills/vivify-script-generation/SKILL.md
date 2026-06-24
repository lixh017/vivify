---
name: vivify-script-generation
description: Use this skill when generating short-video scripts (15-60s) for the OPC panda IP, destined for Douyin / B站 / 小红书. The script is the foundation that feeds downstream scene-decomposition and asset-generation. Use when the user asks for a video script, 口播稿, 短视频脚本, or 选题落地, and the panda character must be respected.
---

# vivify-script-generation

Phase-1 of OPC's content pipeline. Produces a 3-段式 script (hook → body → CTA) plus a style fingerprint JSON consumed by `vivify-scene-decomposition` and `vivify-platform-adaptation`.

**Depends on**: `vivify-panda-character` (voice / persona / 口头禅).
**Feeds**: `vivify-scene-decomposition` → `vivify-platform-adaptation` → 资产生成.

## 3 段式结构 (15-60s)

| 段 | 时长 | 抖音 | B站 | 小红书 |
|----|------|------|------|--------|
| 钩子 | 3-5s | 15-30 字 | 30-60 字 | 20-40 字 |
| 正文 | 10-45s | 40-100 字 | 150-400 字 | 70-220 字 |
| CTA | 3-8s | 10-20 字 | 20-40 字 | 10-40 字 |
| **合计** | 15-60s | **60-150** | **200-500** | **100-300** |

字数含标点。脚本纯中文，**不含**画面词（归拆解阶段）。
## 钩子公式 (≥6, 必选其一)

1. **反问** — `难道……不才是……吗？` → 例: 难道慢一点，不才是真正的生活吗？
2. **悬念** — `最后一刻你会知道……` → 例: 茶凉之后，我懂了苏轼"也无风雨也无晴"的心。
3. **冲突** — `所有人都说X，我偏……` → 例: 所有人都说年轻人要拼命，我偏要慢。
4. **对比** — `X 的A，和X 的B，竟是同一种` → 例: 写字楼凌晨三点和山里月亮，竟是同一种。
5. **视觉震撼** — `(具体数字/物)+反常识` → 例: 一只熊猫，独饮 7 泡茶。
6. **共鸣** — `如果你也……，请听我说` → 例: 如果你也累了，熊猫想跟你坐一会儿。

## CTA 模板 (选 1)

- **互动**: "你今天……了吗？评论区告诉我。" / "你是哪种？1 还是 2？"
- **转发**: "转给那个也需要……的人。" / "存好，下次用得上。"
- **关注**: "下个视频，熊猫想跟你聊聊……"
- **收藏**: "先收藏，慢慢看 ✨" (小红书高频)

## 风格指纹 JSON

存到 `script.styleFingerprint`（web UI 用）：

```json
{
  "hook_type": "反问 | 悬念 | 冲突 | 对比 | 视觉震撼 | 共鸣",
  "hook_pattern": "字符串模板",
  "sentence_length_mean": 12.4,
  "sentence_length_std": 3.1,
  "emoji_density": 0.02,
  "keyword_density": 0.05,
  "keywords": ["慢", "茶", "治愈"],
  "tone_tags": ["治愈", "反鸡汤", "国潮"],
  "voice_id": "low-warm-male-v1",
  "platform": "douyin | bilibili | xiaohongshu",
  "word_count": 128,
  "duration_sec": 42
}
```

Phase 2 风格库聚类 100+ 条指纹 → "爆款指纹画像"。
## Example 1: 抖音 (132 字 / 反问)

```
难道慢一点，不才是真正的生活吗？
我是一只熊猫，住山上。
今天下山买茶，走了三小时，
路上看见外卖员也在赶路。我们都在急。
但我忽然想：慢，不是懒。
慢，是给灵魂留条缝。
下次你赶路的时候，
允许自己，停下来 3 秒。
评论区告诉我：你今天，停过吗？
```

## Example 2: B 站 (312 字 / 悬念)

```
最后一刻你会知道，李白为什么写"举头望明月"。
我是熊猫，也读诗。《静夜思》二十字，流传千年。
但很少有人问，李白那一夜到底在想什么。
不是思乡。他 25 岁出蜀，写过"仍怜故乡水"，
那时候是想回去。
到了扬州、金陵、华阴，钱不够，朋友散，官也丢。
那一夜，床前有月光。
他抬头看的不是故乡，是远方那个回不去的自己。
所以他没写思乡。他写的是"霜"。
头上有霜、心里有霜、人生有霜，月光再好也照不化。
但他还是写下"低头思故乡"——
回不去也没关系，至少抬头那一刻，月光一样。
如果你也回不了家，记得抬头。
下个视频，熊猫想聊聊：苏东坡的月亮。
```

## Example 3: 小红书 (186 字 / 视觉震撼)

```
一只熊猫，4 套衣服，过 1 个夏天 ☁️
姐妹们，独居不是孤独，
是给自己一个重新认识自己的机会。
我的夏日治愈清单 🌿：
1. 早起一壶冷泡茶
2. 阳台摆一张小桌，吃饭看天
3. 关掉所有推送 2 小时
4. 周末不社交，自己逛街买花
5. 睡前写 3 件今天的小确幸
慢一点，也是一种勇敢。
先收藏，慢慢看 ✨
下期分享：30 块的夏日元气早餐。
```

## Model Prompt

```text
你是 OPC 内容主编，擅长治愈 / 御宅 / 哲学 / 国潮短脚本。
IP: 熊猫 (参考 vivify-panda-character)。
平台: {platform} | 字数: {word_count} | 时长: {duration_sec}s
选题: {topic} | 风格指纹: {style_fingerprint_json}

要求: 严格 3 段式 (hook 3-5s / body / CTA 3-8s);
字数落 {min}-{max} (含标点); 钩子用 {hook_type} 公式，仅一个;
短句为主; emoji: 抖音 0 / B站 0-1 / 小红书 3-8;
CTA 选 1 个模板变体; 末尾附加指纹 JSON。

返回: 纯中文脚本 + 指纹 JSON。
```

## Anti-Patterns

- ❌ 钩子超 5s（丢完播率） / ❌ 字数超平台上限 10%
- ❌ 钩子同时套 2 个公式 / ❌ 写"画面切到……"（归拆解）
- ❌ 网络热梗"绝绝子""yyds" / ❌ 喊口号"加油""努力"

## Cross-References

- IP 人设 → `vivify-panda-character`
- 画面 / 镜头拆解 → `vivify-scene-decomposition`
- 单脚本 → 多平台二次适配 → `vivify-platform-adaptation`
- 指纹入库 → Phase 2 风格库（web UI 页面 2）
