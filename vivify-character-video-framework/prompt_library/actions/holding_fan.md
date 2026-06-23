# 持折扇(缓展) — Holding Fan (Slow Unfurl)

> **Status:** ✅ validated — works in 6/6 attempts in EP003, EP006
> (国潮 tone). Use for 国潮 register; the fan is fengge's signature prop.

## Body part breakdown

| Body part | Position |
|-----------|----------|
| **Head** | Slight lift, looking at the fan in hand (or just past it) |
| **Torso** | Upright, slight turn to display the fan |
| **Right arm** | Elbow at side, forearm raised, holding fan stick in right hand |
| **Right hand** | Grip on the bottom of the fan stick (3 fingers, thumb on top) |
| **Left hand** | Either at the side, or supporting the fan's left edge (mid-fan) |
| **Fan state** | Half-unfurled (1/3 to 1/2 open) — slow unroll in progress |

## Speed / quality of motion

- **Speed**: slow (slow-mo feel, ~3-4 seconds for full unfurl)
- **Motion quality**: deliberate, with slight hesitation at the
  start, then a single smooth unroll
- **Eyes track the fan** during the unfurl

## When to use

- 国潮 tone (primary)
- 治愈 tone (when a 国潮 accent is needed)
- Reveal / showcase shots (the fan is visually distinctive)

## When NOT to use

- 哲学 tone (the prop is too 国潮 for 哲学's austerity — use
  [leaning_window.md](./leaning_window.md) instead)
- Night shots unless scene is 国潮-toned
- Action shots

## Compatible scenes

- [bamboo_courtyard.md](../scenes/bamboo_courtyard.md) ✅
- [scroll_painting.md](../scenes/scroll_painting.md) ✅
- [tea_room.md](../scenes/tea_room.md) 🟡
- [moonside_window.md](../scenes/moonside_window.md) 🟡 (国潮+哲学 rare)
- [moss_after_rain.md](../scenes/moss_after_rain.md) 🟡
- [fireside_chat.md](../scenes/fireside_chat.md) 🟡

## Compatible compositions

- [medium_close_up.md](../compositions/medium_close_up.md) ✅ (most common)
- [detail.md](../compositions/detail.md) ✅ (hand + fan)
- [over_shoulder.md](../compositions/over_shoulder.md) 🟡

## How to use in a per-shot prompt

```text
动作: 站姿, 右手持折扇, 折扇缓展(1/3到1/2展开, 慢动作3-4秒完成), 左手自然下垂或扶扇缘, 视线随扇移动
```

## Failure modes

| Failure | Symptom | Fix |
|---------|---------|-----|
| Fan already open | starts fully unfurled | Add `FAN STARTS 1/4 OPEN, SLOW UNFURLING, NOT FULLY OPEN AT START` |
| Fan becomes Western | lace fan, feather fan | Add `CHINESE FOLDING FAN (折扇), NOT LACE, NOT FEATHER, NOT WESTERN` |
| Both hands on fan | looks clumsy | Add `RIGHT HAND ON STICK, LEFT HAND SUPPORTING EDGE OR AT SIDE` |
| Fan blocks face | fully open fan in front of face | Add `FAN AT SIDE, NOT BLOCKING FACE, NOT COVERING EYES` |
| Snap unfurl | too fast, breaks "slow" | Add `SLOW UNFURL, 3-4 SECONDS, DELIBERATE, NOT SNAP, NOT FLICK` |

## Cross-references

- The fan is in fengge's `character.yaml → props` list.
- For the cup counterpart, see [holding_cup.md](./holding_cup.md).
- For the standing base pose, see [standing_calm.md](./standing_calm.md).