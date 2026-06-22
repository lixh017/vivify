# 仰拍 — Low Angle

> **Status:** ✅ validated — works in 4/4 attempts in EP003, EP006
> (used in bamboo_courtyard and scroll_painting). Use sparingly —
> 1-2 per episode max. The dramatic / authoritative register.

## Framing description

- **Subject occupies 30-50% of frame height** (full body or 3/4 body)
- **Subject is in the upper 2/3 of frame** (because of the low angle)
- **Negative space (sky / ceiling) is 40%+ of frame** — this is
  the visual point
- **Subject's face is in the upper third** (rule of thirds)

## Camera angle

- **Camera is BELOW the subject's eye level** (~30-50° below)
- **Camera is close** (~1-2m)
- **Looking UP at the subject** — gives authority / grandeur
- **Static or very slow push-in** (no sweep, no pan)

## Depth of field

- **Medium DOF** — subject in sharp focus, background can be soft
- **Aperture equivalent**: ~f/2.8-f/4
- **Far background** (sky / mist / ceiling) should be the soft
  bokeh element, not distracting

## Best for

- **Authoritative / dramatic shots** (the subject looks powerful)
- **国潮 register** (looking up at a panda + bamboo towering above
  = classic 国潮)
- **Reveal shots** (the first time we see the character in this
  register)
- **"Cultural weight" moments** (the subject is significant)

## When to use (compatible tones)

- 国潮 ✅ (primary)
- 哲学 🟡 (sparingly, for gravitas moments)
- 治愈 🟡 (avoid — 治愈 prefers eye-level intimacy)
- 御宅 🟡 (avoid — 御宅 prefers the cozy register)

## When NOT to use

- More than 1-2 times per episode (dramatic register loses impact)
- Quiet / 治愈 scenes (breaks intimacy)
- Dialogue / voiceover (the upward angle is awkward with the
  voice-over face)
- Sitting shots (low angle of a sitting figure looks awkward unless
  intentional)

## Compatible actions

- [standing_calm.md](../actions/standing_calm.md) ✅ (primary)
- [holding_fan.md](../actions/holding_fan.md) ✅
- [walking_slow.md](../actions/walking_slow.md) ✅
- [leaning_window.md](../actions/leaning_window.md) 🟡
- [sitting_by_fire.md](../actions/sitting_by_fire.md) 🟡 (rare)
- [holding_cup.md](../actions/holding_cup.md) ❌ (cup is awkward from below)

## Compatible scenes

- [bamboo_courtyard.md](../scenes/bamboo_courtyard.md) ✅ (primary)
- [scroll_painting.md](../scenes/scroll_painting.md) ✅
- [moss_after_rain.md](../scenes/moss_after_rain.md) 🟡
- [moonside_window.md](../scenes/moonside_window.md) 🟡 (sky in background)
- [tea_room.md](../scenes/tea_room.md) ❌ (ceiling in background looks weird)
- [fireside_chat.md](../scenes/fireside_chat.md) ❌

## How to use in a per-shot prompt

```text
运镜: 仰拍, 主体占画面高度30-50%, 主体位于上2/3, 留白(天空/雾)≥40%, 镜头在主体眼线下方30-50°, 静态或缓慢推近, 中景深(f/2.8-4)
```

## Failure modes

| Failure | Symptom | Fix |
|---------|---------|-----|
| Too extreme angle | looks like a bug's view | Add `30-50° BELOW EYE LEVEL, NOT WORM'S-EYE, NOT EXTREME` |
| No negative space | breaks the point | Add `40%+ NEGATIVE SPACE (SKY/CEILING/MIST), NOT FULL FRAME, NOT CROWDED` |
| Subject too small | looks lost | Add `SUBJECT 30-50% OF FRAME HEIGHT, NOT 15%, NOT TINY` |
| Distorted face | wide-angle lens look | Add `NORMAL LENS, NOT FISHEYE, NOT WIDE-ANGLE, NO FACE DISTORTION` |
| Multiple uses in episode | dramatic register gets tired | Use this composition only 1-2 times per episode |

## Cross-references

- For the eye-level counterpart, see [medium_close_up.md](./medium_close_up.md).
- For the wide counterpart, see [wide_establishing.md](./wide_establishing.md).
- For 国潮 register examples, see [styles/guochao-2d.md](../styles/guochao-2d.md).