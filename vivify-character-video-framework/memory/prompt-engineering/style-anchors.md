# Style anchors — tested & validation status

> Each style anchor is a reusable string that gets appended to every kling_prompt.
> Status legend: ✅ validated (≥ 3 episodes) / 🟡 hypothesis (1–2 episodes) / 🔴 draft (idea only)

---

## ✅ 国潮 2D gouache watercolor — VALIDATED

**Used in**: EP003, EP004, EP005, EP006
**Pass rate**: 95% (5% drift to 3D)
**Source**: derived from official Seedance 2.0 "国潮" style guide + manual tuning

### The exact text (copy-paste safe)

```text
2D flat illustration, gouache paint, paper texture,
cel-shaded (no gradient lighting, no soft 3D shadows),
simplified shapes, anime-influenced lineart,
brush stroke ink outlines, children's book illustration style,
watercolor wash + ink accents,
vibrant saturated palette (vermilion + cream + jade green + royal blue),
ALWAYS 2D FLAT NEVER 3D NEVER PHOTOREALISTIC,
NEVER PIXAR NEVER DISNEY NEVER CG RENDERED TOY,
NEVER BLURRY NEVER DARK NEVER MUTED DESATURATED
```

### Why it works (the 6 lines that matter)

| Line | Purpose |
|---|---|
| `2D flat illustration, gouache paint, paper texture` | 媒介锚定 — 让模型选 gouache, 不是 3D |
| `cel-shaded (no gradient lighting, no soft 3D shadows)` | 排除 soft shadow (3D 标志) |
| `anime-influenced lineart, brush stroke ink outlines` | 加 lineart 权重, 对抗 3D 渲染 |
| `vibrant saturated palette (vermilion + cream + jade green + royal blue)` | 锁色板, 避免 muted / 灰 |
| `ALWAYS 2D FLAT NEVER 3D NEVER PHOTOREALISTIC` | 正面 + 2 个 banned 概念 |
| `NEVER PIXAR NEVER DISNEY NEVER CG RENDERED TOY` | 列举具体 banned aesthetic |

### When this anchor FAILS

1. **当 prompt 里出现 "realistic" / "photo" / "真实感"** — 锚会被覆盖. 永远不要在 shot prompt 里写这些词.
2. **当 reference image 是 3D 渲染的图** — 比如从 CG 工具导出的 reference, 锚会被 reference 覆盖. Reference 必须是 2D 的 (我们用 gouache 画的 canonical).
3. **当 platform 是 治愈 tone 但 scene 是 霓虹街头** — 风格冲突, 5% 漂到 cyberpunk 3D. 这时需要额外的 anchor override (见下方 "tone × scene 矩阵").

### Variations

#### 国潮 + 现代场景 (霓虹街头)
```text
2D flat illustration, gouache paint + neon line art, paper texture,
cel-shaded, simplified shapes, brush stroke ink outlines,
vibrant saturated palette (vermilion + electric cyan + jade green),
ALWAYS 2D FLAT NEVER 3D NEVER PHOTOREALISTIC,
NEVER PIXAR NEVER CYBERPUNK 3D NEVER CG
```

#### 国潮 + 哲学 (月下, 远景)
```text
2D flat illustration, ink wash (水墨) + light gouache, paper texture,
cel-shaded, sparse lineart, monochrome with one accent color (jade green),
ALWAYS 2D FLAT NEVER 3D NEVER PHOTOREALISTIC,
NEVER PHOTOREALISTIC MOON NEVER CGI CLOUD
```

---

## 🟡 治愈 warm 2D — hypothesis (1 episode)

**Used in**: EP002 only (后期修复后)
**Pass rate**: ~85%
**Risk**: 模型容易把 "warm" 渲染成 "sunset orange" + 3D 玩具

```text
2D flat illustration, warm gouache, paper texture,
cel-shaded, soft outlines, children's book style,
warm palette (cream + soft orange + sage green + dusty pink),
ALWAYS 2D FLAT NEVER 3D NEVER PHOTOREALISTIC,
NEVER PIXAR NEVER DISNEY NEVER CG TOY
```

**Why only 1 episode**: 跟国潮锚效果差不多, 但风险更高. 如果要做治愈, **建议直接用国潮锚 + 偏暖色板**, 不另立锚.

---

## 🔴 御宅 otaku cel-shaded anime — draft

**Status**: 未上 production. 想法是给 御宅 tone 一个更 anime 一点的锚.

```text
2D anime cel-shaded, clean lineart, flat color blocks,
Studio Ghibli inspired, soft watercolor background,
vibrant palette (cream + soft teal + warm grey),
ALWAYS 2D NEVER 3D NEVER PHOTOREALISTIC
```

**Risk**: 跟国潮锚重叠度太高, 维护成本不划算. 暂时不立.

---

## 🔴 哲学 ink wash monochrome — draft

**Status**: 早期试过, 容易把 panda 渲染成"水墨熊猫"(齐白石风格), 不是我们想要的.

**Decision**: 哲学 tone 沿用国潮锚 + monochrome 限制, 不另立水墨锚.

---

## How to add a new style anchor

1. 先看 [`prompt_library/styles/`](../../prompt_library/styles/) 是否已经有 fragment
2. 写 anchor, 至少 4 行正面 + 3 行 NEVER
3. 跑 5 个 shot 测试, 翻车率 < 10% 才算 🟡
4. ≥ 3 episodes 后升 ✅
5. 更新这个文件, 在 `INDEX.md` 加一行
