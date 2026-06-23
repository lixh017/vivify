# 现代水彩治愈 — Modern Watercolor Style

> **Status:** 🟡 partial — works in 4 shots across EP004–EP005, may need
> tuning per scene. Not stress-tested across as many outfits as
> `guochao-2d`. Use when you want a softer, more pastel register than
> the gouache anchor.

## The anchor

```yaml
style_anchor: |
  soft watercolor illustration, wet-on-wet technique,
  paper texture, gentle color bleeds, soft edges,
  pastel palette (dusty pink + sage green + cream + soft lavender),
  hand-painted feel, slightly imperfect edges,
  children's picture book aesthetic,
  2D flat NEVER 3D NEVER PHOTOREALISTIC,
  NEVER PIXAR NEVER DISNEY NEVER CG,
  NEVER HARSH NEVER HIGH-CONTRAST NEVER SATURATED NEON
```

## When to use

- ✅ **治愈 tone** — softer register than guochao-2d; reads as
  "modern illustration" rather than "trad Chinese".
- ✅ **御宅 tone** — fits the cozy/indoor 御宅 register well
  (interior shots, lamp light, blanket).
- ✅ Modern IPs that need a 治愈-leaning aesthetic but don't want
  to commit to ink outlines.
- ✅ Companion animal / cat IP / small creature IP that reads as
  "storybook soft".

## When NOT to use

- ❌ **国潮 tone** — the soft pastel + wet-on-wet is the opposite of
  the bold ink + saturated vermilion that 国潮 wants. Use
  `guochao-2d` instead.
- ❌ **哲学 tone** — philosophy needs edge and contrast; the soft
  bleeds can wash out the contemplative mood.
- ❌ **Action / fight scenes** — soft watercolor edges make motion
  feel mushy. Use this only for still/quiet scenes.

## Variations

### Variation A — Cozy lamp light

Add this overlay to the base anchor for 围炉夜话 / 室内 / late-night
shots:

```yaml
style_anchor: |
  soft watercolor illustration, wet-on-wet technique,
  paper texture, gentle color bleeds, soft edges,
  warm lamp-lit palette (amber + cream + soft brown + dusty rose),
  cozy interior register, hand-painted feel,
  2D flat NEVER 3D NEVER PHOTOREALISTIC,
  NEVER HARSH NEVER HIGH-CONTRAST NEVER SATURATED NEON
```

### Variation B — Rain-on-window

For 月下窗棂 / 雨后青苔 type shots:

```yaml
style_anchor: |
  soft watercolor illustration, wet-on-wet technique,
  paper texture, gentle color bleeds, soft edges,
  cool rain palette (slate blue + sage + cream + pale silver),
  hand-painted feel, slightly imperfect edges,
  2D flat NEVER 3D NEVER PHOTOREALISTIC,
  NEVER WARM NEVER SUNSET NEVER HIGH-SATURATION
```

## Failure modes

| Failure | Symptom | Fix |
|---------|---------|-----|
| Washes into grey | everything becomes muddy grey-blue | Strengthen palette: prefix with `HIGH-SATURATION PASTEL NOT GREY` |
| Drifts to 3D | character gets soft 3D shading | Strengthen negative: `NEVER CGI NEVER 3D SHADING NEVER RENDERED FIGURINE` |
| Drifts to guochao | ink outlines appear | Remove `brush stroke ink outlines` from anchor (this anchor has none) |
| Loses subject | background bleeds into foreground | Add `SHARP SUBJECT, GENTLY BLEEDING BACKGROUND` |

## Validation history

- EP004 shot 02 (围炉夜话, 治愈): ✅ works as intended
- EP004 shot 05 (月下窗棂, 治愈): ✅ works as intended
- EP005 shot 03 (雨后青苔, 治愈): 🟡 required prompt-side reinforcement
  of palette to avoid grey drift
- EP005 shot 07 (茶室, 治愈): ✅ works as intended

## Cross-references

- For traditional Chinese IPs, use [`guochao-2d.md`](./guochao-2d.md) instead.
- See [INDEX.md](./INDEX.md) for the full list of available anchors.