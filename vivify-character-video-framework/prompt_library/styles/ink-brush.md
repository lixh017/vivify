# 水墨写意 — Ink Brush Style

> **Status:** 🔴 draft — single-shot validated in EP006 shot 09 (philosophy
> tone). Has NOT been stress-tested across outfits or other tones. Use
> only when 哲学 tone + sparse register is the goal. Plan a full test
> episode before shipping.

## The anchor

```yaml
style_anchor: |
  Chinese ink wash painting (水墨写意),
  high-contrast black ink + sparse color accents,
  rice paper texture, ink bleeds and splatters,
  calligraphic brush strokes, 留白 negative space,
  zen / contemplative register,
  minimal palette (ink black + cream + sparse vermilion accent),
  2D FLAT NEVER 3D NEVER PHOTOREALISTIC,
  NEVER COLORFUL NEVER SATURATED NEVER BUSY COMPOSITION,
  NEVER PHOTOREALISTIC INK NEVER OIL PAINTING
```

## When to use

- 🟡 **哲学 tone** — single-shot validated. The 留白 + sparse ink register
  reads as contemplative, but it does NOT carry well across many shots.
- 🟡 **国潮 tone** as a contrast — for one standout shot in an otherwise
  gouache episode. Don't use it for the whole episode.
- ✅ Crane / scholar / monk IP that wants a stark, austere register.

## When NOT to use

- ❌ **治愈 tone** — too austere. Use `zhiyu-watercolor` instead.
- ❌ **御宅 tone** — too sparse. Doesn't carry the cozy register.
- ❌ **Whole episode** — the ink register gets visually monotonous
  after 5+ shots. Use as accent, not default.
- ❌ **Action / fight scenes** — ink bleed destroys motion clarity.

## Variations

### Variation A — Sparer (more 哲学)

```yaml
style_anchor: |
  Chinese ink wash painting (水墨写意),
  high-contrast black ink, NO color (pure ink + rice paper),
  rice paper texture, ink bleeds and splatters,
  calligraphic brush strokes, 留白 negative space (≥60% white space),
  zen / austere register,
  2D FLAT NEVER 3D NEVER PHOTOREALISTIC,
  NEVER COLORFUL NEVER SATURATED NEVER BUSY COMPOSITION
```

### Variation B — Ink + gold leaf (more 国潮 poster)

```yaml
style_anchor: |
  Chinese ink wash painting (水墨写意) + gold leaf accents,
  high-contrast black ink + sparse gold + sparse vermilion,
  rice paper texture, ink bleeds and splatters,
  calligraphic brush strokes, 留白 negative space,
  ornate register,
  2D FLAT NEVER 3D NEVER PHOTOREALISTIC,
  NEVER BUSY NEVER CROWDED NEVER LOW-CONTRAST
```

## Failure modes

| Failure | Symptom | Fix |
|---------|---------|-----|
| Subject disappears | character loses definition against ink background | Add `STRONG CHARACTER OUTLINE, INK FILLS BACKGROUND` |
| Drifts to oil painting | looks like Western oil | Strengthen: `NEVER OIL NEVER ACRYLIC NEVER CANVAS TEXTURE` |
| Drifts to calligraphy | pure stroke with no figure | Add `RECOGNIZABLE CHARACTER, NOT PURE STROKES` |
| Drifts to watercolor | soft bleeds appear | Strengthen: `NEVER SOFT WATERCOLOR NEVER PASTEL` |

## Validation history

- EP006 shot 09 (山水卷轴前, 哲学): ✅ single-shot success

## How to validate before shipping

If you want to use this anchor for a whole episode:

1. Run a 5-shot test episode with this anchor.
2. Validate all 5 shots against the IP bible (face/eyes visible, palette locked).
3. If ≥1 shot drifts to oil/calligraphy, fall back to `guochao-2d`
   with palette variation.
4. If ≥3 shots pass, ship with this anchor and run `validators/lint_prompt.py`
   per shot.

## Cross-references

- For a more reliable Chinese-trad register, use [`guochao-2d.md`](./guochao-2d.md).
- For a softer 治愈 register, use [`zhiyu-watercolor.md`](./zhiyu-watercolor.md).
- See [INDEX.md](./INDEX.md) for the full list of available anchors.