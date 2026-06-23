# 全景建立镜头 — Wide Establishing Shot

> **Status:** ✅ validated — works in 8+ shots across EP003–EP006.
> Use for opening shots, scene transitions, and 国潮 register
> (国潮 loves the wide scenic composition).

## Framing description

- **Subject occupies 15-25% of frame height** (full body visible
  if standing, or visible seated + scene context)
- **Scene/environment occupies 60-75% of frame** (this is the
  reason for the wide)
- **Negative space (留白) is 30%+ of frame** (sky, mist, scroll,
  empty courtyard)
- **Subject placed using rule of thirds** (left or right third,
  not center)

## Camera angle

- **Eye level or slightly low** (looking slightly UP at the subject)
- **Far enough back to capture environment** (~5-8m)
- **Static** (no camera motion during the shot) OR a single slow
  push-in (1m over 5s)

## Depth of field

- **Deep DOF** — both subject AND environment in focus
- **Aperture equivalent**: ~f/5.6-f/8
- **Far background** can have atmospheric haze (mist) but should
  still be readable

## Best for

- **Scene establishment** (this is the primary use — tell the
  viewer WHERE we are)
- **国潮 register** (the scenic wide + 留白 + scroll/mountain
  composition is signature 国潮)
- **Philosophical register** (the 留白 is the 哲学 register)
- **Transition between two scenes** (show the journey)

## When to use (compatible tones)

- 国潮 ✅ (primary)
- 哲学 ✅ (primary)
- 治愈 🟡 (works but less common — 治愈 prefers close-up)
- 御宅 🟡 (only for indoor wide establishing)

## When NOT to use

- Dialogue / voiceover register (subject is too small to anchor
  emotion — use [medium_close_up.md](./medium_close_up.md))
- Prop-detail reveal (use [detail.md](./detail.md))
- Action shots (subject too small for motion to read)

## Compatible actions

- [walking_slow.md](../actions/walking_slow.md) ✅ (best — the walk
  is visible at wide scale)
- [standing_calm.md](../actions/standing_calm.md) ✅ (still figure
  in scenic context)
- [sitting_by_fire.md](../actions/sitting_by_fire.md) 🟡
- [holding_fan.md](../actions/holding_fan.md) 🟡 (fan gets too small)
- [holding_cup.md](../actions/holding_cup.md) ❌ (cup is invisible)
- [leaning_window.md](../actions/leaning_window.md) 🟡

## Compatible scenes

- [bamboo_courtyard.md](../scenes/bamboo_courtyard.md) ✅
- [scroll_painting.md](../scenes/scroll_painting.md) ✅
- [moss_after_rain.md](../scenes/moss_after_rain.md) ✅
- [moonside_window.md](../scenes/moonside_window.md) 🟡 (exterior + window)
- [tea_room.md](../scenes/tea_room.md) 🟡 (interior wide)
- [fireside_chat.md](../scenes/fireside_chat.md) 🟡 (interior wide)

## How to use in a per-shot prompt

```text
运镜: 全景建立镜头, 主体占画面高度15-25%, 场景占60-75%, 留白≥30%, 主体位于左/右1/3, 深景深(f/5.6-8), 静态或缓慢推近
```

## Failure modes

| Failure | Symptom | Fix |
|---------|---------|-----|
| Subject too big | breaks wide | Add `SUBJECT 15-25% OF FRAME HEIGHT, NOT 50%, NOT MEDIUM-CLOSE-UP` |
| Subject dead center | breaks rule of thirds | Add `SUBJECT IN LEFT OR RIGHT THIRD, NOT CENTER, NOT DEAD-CENTER` |
| No 留白 | frame feels full/cluttered | Add `NEGATIVE SPACE 30%+ OF FRAME, NOT ZERO, NOT FULL FRAME` |
| Shallow DOF | environment is blurred out | Add `DEEP DOF f/5.6-8, ENVIRONMENT IN FOCUS, NOT SHALLOW, NOT BOKEH BG` |
| Camera moves during shot | breaks establishing | Add `STATIC CAMERA OR SINGLE SLOW PUSH-IN, NOT MULTIPLE MOVES, NOT SWEEP` |

## Cross-references

- For the close counterpart, see [medium_close_up.md](./medium_close_up.md).
- For the over-shoulder variant, see [over_shoulder.md](./over_shoulder.md).
- For 国潮 register examples, see [scenes/scroll_painting.md](../scenes/scroll_painting.md).