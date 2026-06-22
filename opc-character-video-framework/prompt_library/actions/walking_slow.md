# 缓步竹林 — Walking Slowly in Bamboo

> **Status:** ✅ validated — works in 5/5 attempts in EP003, EP004.
> Use for transition shots, contemplative walks, scene-establishing
> movement. Always slow.

## Body part breakdown

| Body part | Position |
|-----------|----------|
| **Head** | Forward, slight downward gaze, half-closed eyes |
| **Torso** | Upright with natural sway (~3° side-to-side as steps land) |
| **Right arm** | Slight forward swing, then back |
| **Left arm** | Slight back swing, then forward (opposite to right) |
| **Hands** | Loose, optional single prop (杖 or 折扇 closed) |
| **Legs** | Slow steps, feet NOT visible below robe hem |
| **Hip sway** | Minimal — this is a "scholar's walk" not a sashay |

## Speed / quality of motion

- **Speed**: slow, ~1 step per 2 seconds
- **Motion quality**: deliberate, no hurry, with breath rhythm
  (step on exhale)
- **No hesitation** — the walk is continuous
- **Eyes blink** every 4-5s

## When to use

- Scene transitions (e.g., from bamboo_courtyard to tea_room)
- Establishing shots
- 哲学 tone (the "contemplator walking" is a classic)
- 国潮 tone (with a 山竹杖 or closed fan)

## When NOT to use

- Action / chase shots
- Indoor scenes (use [standing_calm.md](./standing_calm.md) or
  [sitting_by_fire.md](./sitting_by_fire.md))
- 御宅 tone (the walk is too composed for 御宅's "inner restlessness")

## Compatible scenes

- [bamboo_courtyard.md](../scenes/bamboo_courtyard.md) ✅ (primary)
- [moss_after_rain.md](../scenes/moss_after_rain.md) ✅
- [scroll_painting.md](../scenes/scroll_painting.md) 🟡
- [tea_room.md](../scenes/tea_room.md) ❌ (indoor)

## Compatible compositions

- [wide_establishing.md](../compositions/wide_establishing.md) ✅ (primary)
- [medium_close_up.md](../compositions/medium_close_up.md) 🟡
- [over_shoulder.md](../compositions/over_shoulder.md) ✅ (looking back at character)
- [low_angle.md](../compositions/low_angle.md) ✅ (dramatic, bamboo towering)

## How to use in a per-shot prompt

```text
动作: 缓步, 1步/2秒, 头部微低, 半阖眼, 双臂自然摆动(不持物或持山竹杖), 袍摆微动, 脚步被衣摆遮住
```

## Failure modes

| Failure | Symptom | Fix |
|---------|---------|-----|
| Walking too fast | breaks "缓步" | Add `1 STEP PER 2 SECONDS, NOT WALKING PACE, NOT POWER WALK` |
| Modern walking | Western stride, hands in pockets | Add `SCHOLAR'S WALK, NATURAL SWING, NOT HANDS IN POCKETS, NOT MODERN STRIDE` |
| Visible feet | breaks IP look | Add `FEET HIDDEN UNDER ROBE, NEVER VISIBLE, ROBE COVERS ANKLES` |
| Head up, looking around | breaks contemplative | Add `GAZE SLIGHTLY DOWNWARD, NOT LOOKING AROUND, NOT ALERT` |
| Hip sway too much | looks like runway | Add `MINIMAL HIP SWAY, SCHOLAR'S WALK, NOT SASHAY, NOT RUNWAY` |

## Cross-references

- The 山竹杖 is in fengge's `character.yaml → props` list.
- For the standing counterpart, see [standing_calm.md](./standing_calm.md).
- For the seated counterpart, see [sitting_by_fire.md](./sitting_by_fire.md).