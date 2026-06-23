# 站姿 + 半阖眼 — Standing Calm

> **Status:** ✅ validated — fengge's default pose. Works in 12+
> shots across EP003–EP006. The "neutral" pose. Use whenever a shot
> doesn't need a specific action.

## Body part breakdown

| Body part | Position |
|-----------|----------|
| **Head** | Slight tilt (~5°), half-closed eyes (半阖眼), small subtle smile, not looking at camera |
| **Torso** | Upright, slight lean back (~3°), relaxed shoulders (not hunched) |
| **Arms** | One arm holding a prop at chest level (fan/cup), other arm relaxed at side or behind back |
| **Hands** | Visible, holding prop, fingers loose (NOT clenched), claws hidden under sleeves |
| **Legs** | Standing, slight weight on one leg (contrapposto), feet not visible below the robe hem |
| **Tail** | Not visible (under robe) |

## Speed / quality of motion

- **Speed**: still (静)
- **Motion quality**: subtle breathing only — chest rises ~5mm
  on inhale
- **No motion blur**
- **Eye blink**: optional, one slow blink per 5s

## When to use

- Default for any shot where no specific action is called for.
- 哲学 tone (留白 needs stillness)
- 治愈 tone (calm register)
- 国潮 tone (when paired with a prop — fan/cup/杖)

## When NOT to use

- Action shots (御宅 fighting, chase, etc.)
- 集市戏台 / 老戏楼 (these need active pose)
- "Reveal" shots where a stronger action is needed

## Compatible scenes

- [bamboo_courtyard.md](../scenes/bamboo_courtyard.md) ✅
- [moonside_window.md](../scenes/moonside_window.md) ✅
- [scroll_painting.md](../scenes/scroll_painting.md) ✅
- [tea_room.md](../scenes/tea_room.md) ✅
- [moss_after_rain.md](../scenes/moss_after_rain.md) ✅
- [fireside_chat.md](../scenes/fireside_chat.md) 🟡 (sitting is better)

## Compatible compositions

- [medium_close_up.md](../compositions/medium_close_up.md) ✅
- [wide_establishing.md](../compositions/wide_establishing.md) ✅
- [over_shoulder.md](../compositions/over_shoulder.md) ✅
- [low_angle.md](../compositions/low_angle.md) ✅ (gives the character authority)

## How to use in a per-shot prompt

```text
动作: 站姿, 头部微侧, 半阖眼 + 微微笑意, 不直视镜头, 右手持折扇于胸前, 左手自然下垂, 双足被衣摆遮住, 静(无明显动作, 仅胸腔微起伏)
```

## Failure modes

| Failure | Symptom | Fix |
|---------|---------|-----|
| Direct stare at camera | breaks "不直视镜头" rule | Add `LOOKING PAST CAMERA, NOT AT CAMERA, GAZE OFF-FRAME` |
| Symmetric stance | looks rigid, military | Add `WEIGHT ON ONE LEG, CONTRAPPOSTO, NOT SYMMETRIC` |
| Visible claws | breaks IP rule | Add `CLAWS HIDDEN UNDER SLEEVES, NEVER VISIBLE` |
| Eyes too open | breaks 半阖眼 | Add `HALF-CLOSED EYES, NOT WIDE OPEN, NOT FULLY CLOSED` |
| Visible feet | breaks the IP look | Add `FEET HIDDEN UNDER ROBE HEM, NEVER VISIBLE` |

## Cross-references

- See [fengge's character.yaml § expression](../../characters/fengge/character.yaml) — `半阖眼 + 微微笑意,不直视镜头`.
- See [fengge's character.yaml § forbidden.poses](../../characters/fengge/character.yaml) — `露爪`, `攻击性姿势`, `卖惨流泪`.
- This is the baseline for all 6 actions in this directory.