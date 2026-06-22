# 细节特写 — Detail Close-Up

> **Status:** ✅ validated — works in 6/6 attempts in EP003, EP004.
> Use for prop reveals, hand-craft moments, and small-but-meaningful
> details. The "this object matters" register.

## Framing description

- **Subject occupies 60-90% of frame** (a single prop or hand+prop)
- **Subject is in sharp focus, full detail visible**
- **No face / no full body** — this is a PROP shot, not a person shot
- **Background is soft bokeh** (the scene is implied, not shown)

## Camera angle

- **Eye level of the prop** (NOT the character's eye level)
- **3/4 angle on the prop** (not dead-on flat)
- **Close** (~30-60cm from the prop)
- **Static or single slow micro-pan** (no large camera motion)

## Depth of field

- **Very shallow DOF** — only the prop is in focus, the hand holding
  it can be soft
- **Aperture equivalent**: ~f/1.4-f/2.0
- **Background bokeh** heavy, soft, atmospheric (mist, firelight,
  steam)

## Best for

- **Prop reveals** (the 茶盏 / 折扇 / 毛笔 / 古书 appears)
- **Hand-craft moments** (writing, pouring, holding)
- **Material focus** (the texture of paper, the grain of wood,
  the curl of steam)
- **Voiceover-supported detail** ("this is the cup" + detail shot)

## When to use (compatible tones)

- 国潮 ✅ (primary — props carry the 国潮 register)
- 治愈 ✅ (texture and tactile)
- 哲学 🟡 (the "small moment" register)
- 御宅 ✅ (the cozy-object register)

## When NOT to use

- Character reveal (use [medium_close_up.md](./medium_close_up.md))
- Scene establishing (use [wide_establishing.md](./wide_establishing.md))
- Action shots (props don't carry action)
- More than 2-3 times per episode (props lose impact)

## Compatible actions

- [holding_cup.md](../actions/holding_cup.md) ✅ (primary — cup + hand)
- [holding_fan.md](../actions/holding_fan.md) ✅ (fan + hand)
- [sitting_by_fire.md](../actions/sitting_by_fire.md) 🟡 (hand on knee)
- [standing_calm.md](../actions/standing_calm.md) 🟡 (prop at chest)
- [leaning_window.md](../actions/leaning_window.md) 🟡 (hand on sill)
- [walking_slow.md](../actions/walking_slow.md) ❌ (motion + tight frame = messy)

## Compatible scenes

- [tea_room.md](../scenes/tea_room.md) ✅ (cup, kettle, paper)
- [fireside_chat.md](../scenes/fireside_chat.md) ✅ (ember, kettle)
- [scroll_painting.md](../scenes/scroll_painting.md) ✅ (brush, ink stone, paper weight)
- [moonside_window.md](../scenes/moonside_window.md) ✅ (cup, paper, ink stone)
- [bamboo_courtyard.md](../scenes/bamboo_courtyard.md) 🟡 (bamboo leaf, stone)
- [moss_after_rain.md](../scenes/moss_after_rain.md) ✅ (droplet, moss)

## How to use in a per-shot prompt

```text
运镜: 细节特写, 单个道具或手+道具占画面60-90%, 道具在3/4角度, 浅景深(f/1.4-2.0), 背景为柔焦氛围(雾/火光/蒸汽)
```

## Failure modes

| Failure | Symptom | Fix |
|---------|---------|-----|
| Prop too small | breaks detail | Add `PROP 60-90% OF FRAME, NOT 30%, NOT TINY` |
| Face in frame | breaks detail register | Add `NO FACE IN FRAME, NO FULL BODY, PROP ONLY OR HAND+PROP` |
| Dead-on flat angle | breaks 3D | Add `3/4 ANGLE ON PROP, NOT FLAT, NOT TOP-DOWN` |
| Deep DOF | background fights prop | Add `VERY SHALLOW DOF f/1.4-2.0, BACKGROUND BOKEH ONLY` |
| Camera motion | breaks the still moment | Add `STATIC OR SLOW MICRO-PAN, NO SWEEP, NO LARGE MOTION` |

## Cross-references

- For the wide counterpart, see [wide_establishing.md](./wide_establishing.md).
- For the prop-cup, see [actions/holding_cup.md](../actions/holding_cup.md).
- For the prop-fan, see [actions/holding_fan.md](../actions/holding_fan.md).