# 捧茶盏 — Holding Cup

> **Status:** ✅ validated — works in 8/8 attempts across EP003–EP006
> (used in tea_room, fireside_chat, moonside_window). The "cozy prop"
> pose. Universally compatible.

## Body part breakdown

| Body part | Position |
|-----------|----------|
| **Head** | Slight tilt, eyes at the cup or just past it (NOT camera) |
| **Torso** | Upright or slight forward lean (depending on sit/stand) |
| **Both hands** | Cradling the tea cup (both hands wrapping the cup body) |
| **Cup position** | At chest level, slightly below chin, NOT at lips |
| **Cup content** | Visible (light amber tea) |
| **Steam** | Optional thin wisp from the cup |

## Speed / quality of motion

- **Speed**: very still + one slow lift (~5s lift from waist to chest)
- **Motion quality**: careful, deliberate, with weight (the cup
  is a real object)
- **Optional micro-motion**: thumb micro-rotation on the cup

## When to use

- 治愈 tone (primary)
- 国潮 tone (with the right cup — yixing 紫砂, not porcelain)
- 哲学 tone (paired with moonside_window)
- 御宅 tone (with indoor cozy register)

## When NOT to use

- Action / chase shots
- Reveal shots (cup is too small a prop)
- Shots that need a stronger 国潮 register (use [holding_fan.md](./holding_fan.md))

## Compatible scenes

- [tea_room.md](../scenes/tea_room.md) ✅ (primary)
- [fireside_chat.md](../scenes/fireside_chat.md) ✅
- [moonside_window.md](../scenes/moonside_window.md) ✅
- [bamboo_courtyard.md](../scenes/bamboo_courtyard.md) 🟡
- [moss_after_rain.md](../scenes/moss_after_rain.md) 🟡

## Compatible compositions

- [medium_close_up.md](../compositions/medium_close_up.md) ✅
- [detail.md](../compositions/detail.md) ✅ (hand + cup)
- [over_shoulder.md](../compositions/over_shoulder.md) ✅

## How to use in a per-shot prompt

```text
动作: 双手捧茶盏, 茶盏位于胸前(略低于下巴, 不在唇边), 茶杯内容物可见(淡琥珀色茶汤), 偶尔缓慢举杯动作
```

## Failure modes

| Failure | Symptom | Fix |
|---------|---------|-----|
| Cup becomes coffee mug | Western ceramic mug | Add `SMALL CHINESE TEA CUP, NOT MUG, NOT COFFEE, NOT WESTERN CERAMIC` |
| Cup at lips | looks like drinking | Add `CUP AT CHEST LEVEL, NOT AT LIPS, NOT DRINKING, JUST HOLDING` |
| One hand | looks unbalanced | Add `BOTH HANDS CRADLING CUP, NOT ONE-HANDED, NOT GRIPPING HANDLE` |
| No steam | cup looks cold | Add `THIN WISP OF STEAM FROM CUP, NOT NO STEAM, NOT BOILING` |
| Cup becomes porcelain | white-and-blue | Add `YIXING PURPLE CLAY (紫砂) OR SIMPLE CERAMIC, NOT PORCELAIN, NOT WHITE-AND-BLUE` |

## Cross-references

- The 茶盏 is in fengge's `character.yaml → props` list.
- For the fan counterpart, see [holding_fan.md](./holding_fan.md).
- For the seated base, see [sitting_by_fire.md](./sitting_by_fire.md).