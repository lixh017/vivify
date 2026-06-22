# 过肩镜头 — Over-the-Shoulder

> **Status:** ✅ validated — works in 5/5 attempts in EP004, EP005
> (used in tea_room, fireside_chat). Use for dialogue, intimate
> paired-subject shots, and "we're together in this" register.

## Framing description

- **Foreground shoulder/back of head** occupies 25-35% of frame
  (slightly out of focus)
- **Main subject occupies 50-65% of frame** (in focus)
- **The main subject is in the back 2/3 of the frame** (in the
  Z-depth, not lateral)
- **Environment context** visible behind the main subject

## Camera angle

- **Behind-and-above the foreground shoulder** (~30° down on the
  main subject)
- **Slightly off-axis** (not dead-center over the shoulder)
- **Foreground shoulder is on the side** (left or right shoulder
  of the foreground figure, not center)

## Depth of field

- **Shallow DOF** — main subject in sharp focus, foreground
  shoulder is soft (out of focus but still recognizable as
  shoulder/back-of-head)
- **Aperture equivalent**: ~f/2.0-f/2.8
- **Far background** can be soft bokeh

## Best for

- **Dialogue / paired shots** (looking at someone else)
- **"Looking at the same thing"** register (window, fire, scroll)
- **Cozy paired scenes** (tea_room, fireside_chat)
- **"Witness" moments** (the foreground figure witnesses the
  main subject's state)

## When to use (compatible tones)

- 治愈 ✅ (primary — fireside_chat, tea_room)
- 国潮 ✅ (looking at the scroll painting together)
- 哲学 🟡 (looking at the moon together)
- 御宅 🟡 (indoor cozy register)

## When NOT to use

- Solo shots with no foreground figure available
- Action shots (foreground shoulder blocks motion)
- Reveal shots (the shoulder in foreground is a spoiler)

## Compatible actions

- [sitting_by_fire.md](../actions/sitting_by_fire.md) ✅ (primary — main subject sits)
- [leaning_window.md](../actions/leaning_window.md) ✅ (looking out together)
- [standing_calm.md](../actions/standing_calm.md) ✅
- [holding_cup.md](../actions/holding_cup.md) ✅
- [holding_fan.md](../actions/holding_fan.md) 🟡
- [walking_slow.md](../actions/walking_slow.md) ❌ (motion + foreground shoulder = messy)

## Compatible scenes

- [fireside_chat.md](../scenes/fireside_chat.md) ✅ (primary)
- [tea_room.md](../scenes/tea_room.md) ✅
- [moonside_window.md](../scenes/moonside_window.md) ✅
- [bamboo_courtyard.md](../scenes/bamboo_courtyard.md) 🟡
- [moss_after_rain.md](../scenes/moss_after_rain.md) 🟡
- [scroll_painting.md](../scenes/scroll_painting.md) ✅ (looking at scroll together)

## How to use in a per-shot prompt

```text
运镜: 过肩镜头, 前景肩/后脑勺占25-35%(柔焦), 主体占50-65%, 主体在后景, 浅景深(f/2.0-2.8)
```

## Failure modes

| Failure | Symptom | Fix |
|---------|---------|-----|
| Dead-center shoulder | breaks framing | Add `SHOULDER ON LEFT OR RIGHT, NOT DEAD-CENTER, NOT SYMMETRIC` |
| Foreground too sharp | breaks DOF | Add `FOREGROUND SOFTLY OUT OF FOCUS, NOT SHARP, NOT IN-FOCUS` |
| Main subject too small | breaks main subject rule | Add `MAIN SUBJECT 50-65% OF FRAME, NOT 30%, NOT TINY` |
| No environment visible | claustrophobic | Add `ENVIRONMENT VISIBLE BEHIND MAIN SUBJECT, NOT SOLID WALL` |
| Looking past each other | disorienting | Add `EYE-LINE OF FOREGROUND MATCHES MAIN SUBJECT, NOT MISMATCHED` |

## Cross-references

- For the close counterpart, see [medium_close_up.md](./medium_close_up.md).
- For the prop detail variant, see [detail.md](./detail.md).
- For paired-scene examples, see [scenes/tea_room.md](../scenes/tea_room.md) and [scenes/fireside_chat.md](../scenes/fireside_chat.md).