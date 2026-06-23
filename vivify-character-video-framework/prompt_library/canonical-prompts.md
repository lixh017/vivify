# Canonical Reference Image Prompts

> How to generate the 3 reference jpgs that Seedream uses as
> `reference_image` for every per-shot prompt. These are the IP's
> identity anchor — if they look wrong, every shot looks wrong.

## Why 3 jpgs (not 1)

- 1 jpg = Seedream preserves identity for 1-2 outfits, then drifts
  when the prompt asks for a different outfit color
- 3 jpgs (front + 3/4 + side) = Seedream can interpolate identity
  across 5+ outfits and 12+ scenes

This is the 3-jpg minimum, not a maximum. For complex IPs, ship 5+.

## Required common elements

Every canonical prompt MUST include these (no exceptions):

1. **`chibi proportions` or specific head_body_ratio** — locks the
   cute/realistic register
2. **`neutral expression`** — no smile, no frown, no eyes wide
3. **`no props`** — no cup, no fan, no scroll, no umbrella. The
   character alone, on a plain background
4. **`plain background`** — solid color (cream / grey / soft sky)
5. **`full body visible`** — head to feet (feet under robe hem is
   fine, but the silhouette should be complete)
6. **`eye patches visible`** — for panda, this is the identity
   marker; for other IPs, use the equivalent (e.g. for a fox, the
   white cheek fur)
7. **`hands visible`** — both hands, not hidden behind back
8. **`[your IP's style_anchor]`** — the same anchor you'll use for
   per-shot prompts

## Front view template

```text
A chibi-proportioned (1:1.2 head-to-body) [species]
in [color description], [face/fur colors] face, [extremity colors] extremities,
neutral expression (no smile, no frown, eyes half-open),
looking straight ahead, both hands relaxed at sides,
no props, full body visible from head to feet,
plain soft cream background,
[INSERT style_anchor HERE]
```

**Example for fengge (panda, 朱红汉服)**:

```text
A chibi-proportioned (1:1.2 head-to-body) adult giant panda
in vermilion red hanfu (wide-sleeved round-collar robe with jade pendant),
cream face (F5F0E1) with black eye patches, near-black hands and feet (1A1A1A),
neutral expression (no smile, no frown, eyes half-open, slight smile optional),
looking straight ahead, both hands relaxed at sides,
no props (no fan, no cup), full body visible from head to feet (feet covered by robe hem),
plain soft cream background,
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

## 3/4 view template

```text
A chibi-proportioned (1:1.2 head-to-body) [species]
in [color description], [face/fur colors] face, [extremity colors] extremities,
3/4 view (turned ~30° to the right, looking at camera, both eyes visible),
neutral expression (no smile, no frown, eyes half-open),
both hands relaxed at sides,
no props, full body visible from head to feet,
plain soft cream background,
[INSERT style_anchor HERE]
```

The 3/4 view is the **most important** of the 3 — it gives Seedream
the depth cue it needs for non-frontal scenes.

## Side view template

```text
A chibi-proportioned (1:1.2 head-to-body) [species]
in [color description], [face/fur colors] face, [extremity colors] extremities,
side view (full profile, 90° turn, one eye visible, full silhouette of the body),
neutral expression (no smile, no frown, eyes half-open),
both hands relaxed at sides,
no props, full body visible from head to feet,
plain soft cream background,
[INSERT style_anchor HERE]
```

## Quality checklist

After generating, validate each jpg with this checklist:

- [ ] **File size > 200KB** (low size = over-compressed = bad for
  Seedream reference)
- [ ] **Eye patches visible** (for panda; for other IPs, the
  equivalent identity marker is visible)
- [ ] **Hands visible** (both hands, not hidden behind back)
- [ ] **Feet hidden under robe hem** (per IP rule, or visible if
  the IP is a different species)
- [ ] **Background is plain** (single color, no scenery, no props)
- [ ] **Expression is neutral** (no smile, no frown, half-open eyes)
- [ ] **Style matches the style_anchor** (run a visual diff against
  the fengge canonicals if you have them)
- [ ] **Three views are consistent** (same proportions, same
  outfit, same face — the only difference is the angle)

Run `python3 validators/canonical_image_check.py characters/<name>/canonical`
to automate steps 1-3, 5, 7.

## How to iterate when first attempts fail

| Failure | Fix |
|---------|-----|
| Eyes not half-open | Add `EYES HALF-OPEN, NOT WIDE OPEN, NOT FULLY CLOSED` |
| 3D drift | Strengthen `style_anchor` negatives |
| Different outfit color | Add explicit hex codes: `VERMILION #C73E1D ROBE, NOT BURGUNDY, NOT ORANGE` |
| Background not plain | Add `PLAIN SOLID CREAM BACKGROUND #E8DEC9, NO SCENERY, NO PROPS, NO FLOOR` |
| Hands too small | Add `BOTH HANDS VISIBLE, FULL SIZE, NOT TINY, NOT HIDDEN` |
| Looking off-frame | Add `LOOKING STRAIGHT AHEAD, NOT OFF-FRAME, NOT AT SCENERY` |
| 3/4 view becomes frontal | Add `TURNED 30° TO THE RIGHT, ONE EYE PARTIALLY HIDDEN, NOT FRONTAL` |
| Side view becomes 3/4 | Add `FULL PROFILE, 90° TURN, ONE EYE ONLY, NOT 3/4, NOT FRONTAL` |
| Smile appears | Add `NEUTRAL EXPRESSION, NO SMILE, NOT HAPPY, NOT SAD, NOT ANGRY` |

## Naming convention

Save the jpgs in `characters/<your-name>/canonical/` with this
naming:

```
canonical/
  <your-name>-canonical-<outfit-id>-front.jpg     # primary
  <your-name>-canonical-<outfit-id>-34.jpg         # 3/4 view
  <your-name>-canonical-<outfit-id>-side.jpg       # side view
```

For fengge:
- `panda-canonical-zh-red.jpg` (vermilion hanfu)
- `panda-canonical-blue-changsan.jpg` (royal blue changshan)
- `panda-canonical-warm-orange.jpg` (warm orange short jacket)

The primary in `character.yaml` should point to the most "neutral
default" outfit. The alternates to the more distinctive ones.

## Cross-references

- [characters/format.md § canonical](./characters/format.md) — how the YAML field references these jpgs
- [characters/fengge-summary.md § What to copy](./characters/fengge-summary.md) — fengge's canonical pattern
- [styles/guochao-2d.md](./styles/guochao-2d.md) — the style_anchor to embed in the prompt
- [../characters/fengge/canonical/](../characters/fengge/canonical/) — the validated reference jpgs