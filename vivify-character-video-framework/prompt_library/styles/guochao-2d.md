# 国潮 2D Gouache Watercolor — Style Anchor

> **Status:** ✅ validated across EP003–EP006 (4 episodes, 30+ shots, 4 voice tones).
> This is the exact `style_anchor` shipped in fengge's `character.yaml`.

## The anchor (copy verbatim)

```yaml
style_anchor: |
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

## When to use

- ✅ **国潮 tone** — this is the default. Anchors lock the
  "vermilion + cream + jade + royal blue" palette and force flat 2D.
- ✅ **治愈 tone** — the watercolor wash + paper texture give the
  cozy register the 治愈 prompt asks for, without sliding into
  pastel/photoreal.
- ✅ **哲学 tone** — the ink outlines and 留白 ("negative space")
  carry the contemplative mood without needing extra register shifts.
- ✅ Panda, fox, crane, monk, scholar, ANY Chinese-tradition IP.

## When NOT to use

- ❌ **Modern IP** — don't use for a 御宅 anime schoolgirl or a cyberpunk
  hacker panda. The gouache + ink outlines read as "trad".
- ❌ **Western题材** — for a European fairy-tale IP, the
  "vermilion + cream + jade + royal blue" palette will fight the
  source material. Use `zhiyu-watercolor` instead.
- ❌ **Photorealistic** — this anchor hard-bans photoreal. Don't fight it.
- ❌ **3D toy aesthetic** — explicit negative. If you want a 3D render,
  you need a different anchor entirely.

## Variations

The fengge anchor above is the validated core. Two safe variations:

### Variation A — Without gold accent (cooler, more water-y)

```yaml
style_anchor: |
  2D flat illustration, gouache paint, paper texture,
  cel-shaded (no gradient lighting, no soft 3D shadows),
  simplified shapes, anime-influenced lineart,
  brush stroke ink outlines, children's book illustration style,
  watercolor wash + ink accents,
  cool muted palette (jade green + cream + ink black + faded vermilion),
  ALWAYS 2D FLAT NEVER 3D NEVER PHOTOREALISTIC,
  NEVER PIXAR NEVER DISNEY NEVER CG RENDERED TOY,
  NEVER BLURRY NEVER DARK NEVER MUTED DESATURATED
```

Use when the scene is 雨后青苔 / 月下窗棂 / 围炉夜话 — the cooler
palette reads more 治愈 / 哲学.

### Variation B — With heavier ink outline (more 国潮 poster)

```yaml
style_anchor: |
  2D flat illustration, gouache paint, paper texture,
  cel-shaded (no gradient lighting, no soft 3D shadows),
  simplified shapes, BOLD ink outlines (1-2mm brush strokes),
  anime-influenced lineart,
  brush stroke ink outlines, children's book illustration style,
  watercolor wash + ink accents,
  vibrant saturated palette (vermilion + cream + jade green + royal blue + gold leaf accents),
  ALWAYS 2D FLAT NEVER 3D NEVER PHOTOREALISTIC,
  NEVER PIXAR NEVER DISNEY NEVER CG RENDERED TOY,
  NEVER BLURRY NEVER DARK NEVER MUTED DESATURATED
```

Use for 集市戏台 / 老戏楼 / 道观 — bolder ink reads as "festive" and
"ornate". Not validated as widely as the core anchor — run a test episode first.

## Failure modes (Seedream drift)

When the anchor fails, it usually drifts in one of these directions:

| Failure | Symptom | Fix |
|---------|---------|-----|
| 3D toy drift | character looks like a Pixar figurine, smooth shading | Strengthen the negative: add `NEVER CGI NEVER C4D NEVER BLENDER NEVER RENDERED 3D CHARACTER` |
| Photoreal drift | fur texture becomes photoreal, eyes go glassy | Add `NEVER FUR TEXTURE NEVER INDIVIDUAL HAIRS NEVER GLOSSY EYES` |
| Muted palette | colors wash out to grey-blue | Strengthen positive: prefix with `HIGH SATURATION VERMILION + CREAM + JADE GREEN + ROYAL BLUE` |
| Western drift | clothing becomes Renaissance / Victorian | Add anti-period: `NEVER RENAISSANCE NEVER VICTORIAN NEVER EUROPEAN CLOTHING` |
| Blurry drift | background dissolves into ink-blot haze | Add `SHARP FOCUS FOREGROUND, GENTLE BOKEH BACKGROUND ONLY` |

## Example output

Validated references are in
[`../../characters/fengge/canonical/`](../../characters/fengge/canonical/) —
all 3 jpgs (`panda-canonical-zh-red.jpg`, `panda-canonical-blue-changsan.jpg`,
`panda-canonical-warm-orange.jpg`) were generated with this anchor and
match across outfits.

## Cross-references

- See [fengge's actual character.yaml § style_anchor](../../characters/fengge/character.yaml) for the line-in-yaml form.
- See [`../../memory/prompt-engineering/style-anchors.md`](../../memory/prompt-engineering/style-anchors.md) for engineering notes.
- See [INDEX.md](./INDEX.md) for the full list of available anchors.