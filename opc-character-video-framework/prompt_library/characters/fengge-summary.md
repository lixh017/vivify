# 峰哥 / Fengge — IP Reference Summary

> One-page overview of the showcase IP. What makes it work, what
> doesn't, what to copy if you're authoring a similar 国潮-toned IP.
> For the full config, see [`../../characters/fengge/character.yaml`](../../characters/fengge/character.yaml).

## TL;DR

**峰哥 (Fengge)** is an adult panda IP in 国潮 register. Visual
register: 2D gouache + ink outlines + saturated vermilion/cream/jade/royal
blue palette. 4 voice tones (治愈 / 御宅 / 哲学 / 国潮) each with a
distinct audible register. 5 outfits that round-robin per episode.
12 scenes (3 primary + 9 supporting). 7 hook formulas for opening
lines. Strongest IP for traditional-Chinese + cozy-intimate content.

## What makes this IP work

### 1. Visual register is locked, not aspirational

The `style_anchor` is ~10 lines of **strong positive + strong
negative** anchors. It explicitly bans 3D, Pixar, Disney, photoreal,
blurry, dark, muted. Without those negatives, Seedream drifts to
3D-rendered-toy aesthetic on every third shot. The anchor survives
30+ production shots in EP003–EP006 without drift.

See [../styles/guochao-2d.md](../styles/guochao-2d.md) for the full
validated anchor.

### 2. The expression rule `半阖眼 + 微微笑意,不直视镜头`

This is the IP's soul. It:
- Prevents the "looking at viewer" creep that breaks fourth wall
- Conveys 留白 (contemplation, half-distance)
- Reads as "scholar / sage / elder" without being preachy
- Works in ALL 4 voice tones

**If you copy one thing from fengge, copy this expression rule.**

### 3. The 4-tone voice matrix is concrete, not vague

Each tone has a `voice_id` + `speed` + `pitch` + `emotion` + `vol` +
a human-readable `guide`. The 治愈/御宅/哲学/国潮 split maps to:
- 治愈: slow + warm + lamp light + tea
- 御宅: introspective + indoor + 11pm + blanket
- 哲学: questioning + moonlight + 留白 + reverse-chicken-soup
- 国潮: classical poetry + 折扇 + scroll + seasonal

The `guide` field is critical: it's how the human author knows
**what kind of sentences to write** in this tone. Without it, the
voice_id is just a number.

### 4. 5 outfits that round-robin

Outfits are visual variety, not wardrobe change. Same pose, same
scene, different robe color → different episode mood:
- 朱红汉服 (vermilion) — 国潮 / 集市戏台
- 宝蓝长衫 (royal blue) — 国潮 / 哲学 / 卷轴
- 暖橙短褂 (warm orange) — 治愈 / 御宅 / 围炉
- 翠绿僧袍 (jade green) — 哲学 / 治愈 / 寺庙
- 红白运动夹克 (red-white) — 御宅 / 现代感

The `anti_patterns` field on each outfit is what stops Seedream
from rendering the 暖橙短褂 as a modern cleaner uniform (the
strongest `anti_patterns` in the file).

### 5. 12 scenes is the sweet spot

Not 5 (too repetitive), not 30 (too thin). 12 gives:
- 3 home-base scenes (bamboo_courtyard, tea_room, fireside_chat)
- 3 outdoor scenes (moss_after_rain, moonside_window, scroll_painting)
- 6 supporting scenes (market_stage, old_theater, neon_street,
  taoist_temple, buddhist_temple, stage)

The 3 home-base scenes carry 80% of the IP. The supporting scenes
are flavor for episodes that need a different venue.

### 6. Forbidden list is specific, not generic

```yaml
forbidden:
  scenes:
    - "现代写字楼"
    - "夜店"
    - "健身房"
    - "网红打卡点"
  poses:
    - "露爪"          # claws always hidden
    - "攻击性姿势"
    - "卖惨流泪"
    - "穿西装"
```

Every item is specific. "现代写字楼" is one phrase, not "bad
modern scenes". The IP bible is meant to be scannable, not legal.

### 7. Hook formulas + voice rules = repeatable content craft

7 hook formulas (三问开场, 反常识开场, 留白钩子, 故事钩子,
画面钩子, 引用钩子, 数字钩子) + voice rules (短句 ≤15 字, 多用
反问, 大量留白, signature words like 嘿/啊/得/呗) means **any
human author can write 治愈/国潮 copy in 5 minutes** without
reinventing the register.

## What doesn't work (lessons learned)

- **3D rendering** — without `NEVER 3D NEVER PIXAR`, ~30% of
  shots drift to soft 3D shading. The negative anchors are not
  optional.
- **Western clothing** — even with `mandarin collar` in the
  prompt, Seedream will sometimes add Renaissance / Victorian
  cues. The `forbidden.poses → 穿西装` rule helps; for
  scene-level, scene-specific anti-patterns are needed.
- **Photoreal fur** — Seedream loves adding individual fur
  textures. The 2D gouache anchor bans this; without the
  anchor, shots look like a Pixar panda.
- **High-saturation rainbow** — without the `palette` line in
  the anchor, Seedream goes neon. The `vermilion + cream + jade
  green + royal blue` line is the palette lock.
- **Multiple camera movements per shot** — the framework bans
  this; the per-shot prompt should have ONE 运镜 direction only.

## Quantitative snapshot (as of EP006)

| Dimension | Count |
|-----------|-------|
| Episodes produced | 4 (EP003–EP006) |
| Outfits | 5 |
| Scenes | 12 (6 heavily used) |
| Voice tones | 4 |
| Hook formulas | 7 |
| Signature words | 6 |
| Banned words | 5 |
| Validation status | ✅ across the board |

## What to copy vs. what to adapt

| Element | Copy? | Why |
|---------|-------|-----|
| `style_anchor` (国潮 2D gouache) | ✅ copy verbatim if your IP is 国潮 | Validated, lock-and-forget |
| `expression: 半阖眼 + 微微笑意` | ✅ copy if your IP is contemplative | Carries the 留白 register |
| 4-tone voice matrix | 🟡 adapt | The tones (治愈/御宅/哲学/国潮) are fengge-specific; your IP may need different ones |
| `forbidden.poses → 露爪` | 🟡 adapt | Specific to fengge's panda; replace with your IP's hard no |
| `outfit_workwear_orange.anti_patterns` | 🟡 copy the structure | Strong anti_patterns are a model; the content is fengge-specific |
| Hook formulas | ✅ copy if your IP needs hooks | The 7 formulas are tone-agnostic |
| `voice_rules` | 🟡 adapt | The `signature_words` and `banned_words` are fengge-specific |

## Cross-references

- Full config: [`../../characters/fengge/character.yaml`](../../characters/fengge/character.yaml)
- Canonical jpgs: [`../../characters/fengge/canonical/`](../../characters/fengge/canonical/)
- Style anchor: [../styles/guochao-2d.md](../styles/guochao-2d.md)
- Format reference: [./format.md](./format.md)
- Canonical prompt guide: [../canonical-prompts.md](../canonical-prompts.md)