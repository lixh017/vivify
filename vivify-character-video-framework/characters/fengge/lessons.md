# peak哥 (fengge) — IP-specific lessons

> Lessons learned across EP001–EP006 of the peak哥 panda showcase IP.
> These are not general — they only apply to this character (or characters with similar silhouette / 2D style anchors).

## Identity at a glance

- 物种: 成年熊猫, 头身比 1:1.2
- 配色: 朱红 / 米白 / 翠绿 / 宝蓝 (no pastels, no pure black/white)
- 默认语气: 半阖眼 + 微微笑意, 不直视镜头
- Default canonical: `characters/fengge/canonical/panda-canonical-zh-red.jpg`
- File: [`characters/fengge/character.yaml`](../../characters/fengge/character.yaml)

---

## ✅ What WORKED (validated in production)

### 1. Three canonical reference images, not one

We ship three jpg files, not one:

```text
canonical/panda-canonical-zh-red.jpg           # 朱红汉服 — default anchor
canonical/panda-canonical-blue-changsan.jpg   # 宝蓝长衫 — 国潮 fallback
canonical/panda-canonical-warm-orange.jpg     # 暖橙短褂 — 治愈 fallback
```

Why three:
- 每个 outfit 是 IP 圣经的一部分, 不能让 Seedream "记得" outfit (它记得的是 silhouette + 颜色)
- 切换 outfit 时不需要重新生成参考图 — 只需要换 round-robin
- Lint pass rate: 1 套 → 3 套从 78% 提升到 95%

### 2. Inline base64 for the reference image, NOT signed URLs

最初我们上传 canonical 到 OSS 然后给 Seedream 传一个 24h 签名的 URL. 24 小时后所有 复现 (reproducibility) 跑都失败了, 因为 URL 过期了.

**Rule**: 永远 inline base64 encode canonical, 直接塞进 prompt 的 `reference_image` 字段. 文件大一点 (< 2MB) 没所谓, 总比 stale URL 强.

### 3. Per-tone voice profile, NOT one-size-fits-all

每个 tone (治愈/御宅/哲学/国潮) 有独立的 `voice_id` + `speed` + `pitch` + `emotion`. 详见 `character.yaml` 第 147–175 行.

关键决策:
- 哲学用 `male-qn-qingse` (音色偏冷), 其它三个用 `male-qn-jingying` (音色偏暖)
- 哲学 emotion = `sad`, 其余都是 `neutral` (sad 在哲学调里不是伤感, 是"留白")
- 速度范围 0.78 (治愈, 最慢) → 0.92 (国潮, 最快), 不超过 1.0

### 4. Style anchor with strong NEVER clauses

`character.yaml` 里的 `style_anchor` (181–190 行) 用了三层防御:

```text
# 正面 (priority 1)
2D flat illustration, gouache paint, paper texture,
cel-shaded (no gradient lighting, no soft 3D shadows),
simplified shapes, anime-influenced lineart,
brush stroke ink outlines, children's book illustration style,
watercolor wash + ink accents,
vibrant saturated palette (vermilion + cream + jade green + royal blue),

# 负面 (priority 2 — 必须 aggressive, 因为 Seedream 对后面的 token 加权更低)
ALWAYS 2D FLAT NEVER 3D NEVER PHOTOREALISTIC,
NEVER PIXAR NEVER DISNEY NEVER CG RENDERED TOY,
NEVER BLURRY NEVER DARK NEVER MUTED DESATURATED
```

Lesson: Seedream 4.0 对 3D 渲染的先验很强, 简单说"2D"是 50% 翻车的. 必须连续三次 "NEVER 3D" + 列举具体的 banned aesthetic (Pixar / Disney / CG toy) 才能压住.

详见 [`prompt-engineering/style-anchors.md`](../prompt-engineering/style-anchors.md).

---

## 🟡 What DIDN'T work (hypothesis / 翻车记录)

### 1. Cleaner uniform 翻车 (EP002 早期)

Outfit `outfit_workwear_orange` (暖橙短褂) 在前 5 个 shot 里被 Seedream 渲染成 **橙色清洁工制服** — 反射条, 工号牌, 拉链口袋全部冒出来了.

**Root cause**: 我们的初始 `anti_patterns` 只写了 "no business suit", 太弱.

**Fix**: 在 `character.yaml` 第 58 行把 anti_patterns 写满 6 条 negative:

```yaml
anti_patterns: "**NOT a modern cleaner uniform** — no reflective stripes,
no utility pockets, no logo badges, no work number tags, no nylon velcro,
no metal zippers; only 盘扣 (Chinese frog buttons) + cotton/linen fabric
+ 国潮 patterns"
```

详见 [`fengge/gotchas.md`](./gotchas.md).

### 2. 3D drift before Phase 1

EP001 几乎 50% 的 shot 漂到了 3D-rendered aesthetic (像皮克斯玩具), 哪怕我们 prompt 里说了"2D".

**Root cause**: 没有 `style_anchor`, 每一个 shot 单独祈祷.

**Fix**: 把上面那段 6-行 NEVER 加入 `character.yaml.style_anchor`, 强制 render_episode.py 拼到每个 shot 的末尾.

现在 3D drift 率 < 5%.

### 3. Synthetic BGM 弱

最初我们用 MiniMax TTS 自己的 audio + 提一段 5 秒生成的 BGM 拼接. BGM 听起来是 "stock MIDI" — 抖音观众一眼能看出来.

**Lesson**: 短剧 BGM 应该用 **纯人工曲库** (Epidemic Sound, 抖音免费 BGM 库), 不要用 AI 生成. AI 音乐在 5–10 秒片段里不够 "放克" 或 "民乐" 的辨识度.

Current pipeline: 治愈/御宅用 Epidemic Sound lo-fi, 国潮用古风曲库, 哲学用纯 ambient drone.

### 4. Signed URL 24h TTL trap

`render_episode.py` 早期版本把 canonical image 上传到 OSS 然后用 presigned URL 喂给 Seedream. URL 24h 过期后, 任何 `git clone` + replay 都拿不到一样的 reference image.

**Fix**: 全部 inline base64, 见上.

### 5. `panda_visual_anchor: fengge_v1` 被当成 prompt 文字

Bug: 早期我们用 yaml key `panda_visual_anchor: fengge_v1` 当作 canonical 标识, render_episode.py 在拼 prompt 时直接 `str(anchor)` → 把 `"fengge_v1"` 字面量塞进了 Seedream prompt 的末尾.

症状: Seedream 把 `fengge_v1` 渲染成画面上的文字 (像水印/字幕), 5 个 shot 里出现 3 次.

**Fix**: 用 yaml key `canonical.primary: <file_path>`, render_episode.py 只读 `file_path` 不读 key name. 改完从未再翻车.

---

## Best practices for similar characters (panda / 毛绒 / IP 短剧)

| Lesson | Why it matters |
|---|---|
| 3 个 canonical 起步 (default + 2 个 outfit) | Outfit round-robin 不需要 re-render reference |
| Inline base64, 永远不要 signed URL | Reproducibility 100% |
| Per-tone voice profile | 一个 TTS voice 撑不住 4 种 tone |
| Style anchor 6 行 NEVER | Seedream 默认偏 3D, 必须用列举式压住 |
| 抽象情绪 → 具象动作 (see emotion-actions.md) | "沉思" 不会动, "低头 + 手指轻抚杯沿" 才会动 |
| 大头照 单独成 reference | Full-body shot 训练不出稳定的 face embedding |
| Anti-patterns 至少 4 条 negative, 含具体的禁忌元素 | "no business suit" 太弱 |
| 永远加 `保持无字幕, 不要生成水印, 不要生成 Logo` | Seedance 在中文 prompt 里偶尔会自己加字幕 |
| BGM 走曲库, 不走 AI 生成 | 5–10s 片段里 AI 音乐辨识度不够 |
| 每个 episode 跑完写 lessons (就像这个文件) | 下一个 IP 不用从零踩坑 |

---

## Status legend

- ✅ validated — proven in ≥ 3 episodes, lint passes
- 🟡 hypothesis — works but only 1–2 episodes
- 🔴 draft — unverified, but interesting idea
