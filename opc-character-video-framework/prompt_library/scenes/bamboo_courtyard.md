# 竹林小院 — Bamboo Courtyard

> **Status:** ✅ validated — works in 8/8 attempts across EP003–EP006.
> This is fengge's default scene. Use it whenever you need a "home
> base" environment that reads as intimate, grounded, and 国潮.

## Environment details

- **Low wooden fence** (粗木篱笆, weathered, 0.5m high) at the back edge of frame
- **3-5 bamboo stalks** (毛竹) in mid-ground, slightly leaning inward
- **Stone path** (青石板小径) curving from front-left to back-right
- **Stone lantern** (石灯笼) in one corner, lightly mossy
- **Distant mist** — soft fog in the back third of frame
- **No characters other than the IP** in frame (unless the scene is
  集市戏台 etc.)

## Lighting

- **Time of day**: afternoon, 3pm
- **Quality**: soft daylight, no harsh shadows
- **Direction**: side backlight (3/4 from behind-right, so the
  character gets a subtle rim light)
- **Sky**: hazy white-blue, not pure blue

## Color palette (hex)

- Bamboo green: `#7BA05B`
- Stone grey: `#A8A29E`
- Wooden fence (warm brown): `#8B6F47`
- Stone lantern: `#B8B2A1`
- Mist: `#E8E6E0`
- Background sky: `#D4DCE0`

## Mood keywords

`warm`, `intimate`, `grounded`, `daylight`, `home`, `breathing-room`

## Audio cues (Seedance 2.0 audio sync)

- Soft bamboo leaves rustling (high-frequency whisper)
- Distant single bird call (NOT a chorus)
- Optional: faint wooden door creak at start
- **No human voices, no music** — scene provides texture only

## Camera angle suggestions

- **Default**: medium shot, character centered on the stone path,
  bamboo in the back third
- **Wide establishing**: stone lantern in foreground, character
  walking away down the path
- **Close-up variant**: shallow DOF, bamboo stalks out of focus in
  background, character with rim light

## How to use in a per-shot prompt

Append this to the Seedance 2.0 公式 after the 位置 field:

```text
场景: 竹林小院, 下午3点, 侧面逆光, 青石板小径 + 3-5根毛竹 + 木篱笆 + 石灯笼, 远处薄雾
配色: 竹绿 #7BA05B, 青石 #A8A29E, 木篱 #8B6F47
氛围: 温暖、私密、有呼吸感
音频: 竹叶沙沙声 + 远处一声鸟鸣, 无人声
```

## How to use in character.yaml

```yaml
character:
  scenes:
    - id: bamboo_courtyard
      name: "竹林小院"
      mood: "warm, intimate"
      lighting: "afternoon 3pm, side backlight, soft daylight"
      palette:
        - "#7BA05B"  # bamboo green
        - "#A8A29E"  # stone grey
        - "#8B6F47"  # wooden fence brown
```

## Failure modes

| Failure | Symptom | Fix |
|---------|---------|-----|
| Bamboo becomes tropical | banana leaves, palm trees | Add `BAMBOO FOREST (毛竹), NOT TROPICAL, NOT PALM` |
| Lighting becomes noon harsh | sharp shadows on face | Add `AFTERNOON 3PM SOFT DAYLIGHT, NO NOON SHADOWS` |
| Background becomes solid black | background not rendered | Add `HAZY WHITE-BLUE SKY IN BACKGROUND, NEVER SOLID BLACK` |
| Stone path becomes cobblestone | Western cobble | Add `CHINESE STONE FLAGS (青石板), NOT COBBLESTONE, NOT BRICK` |

## Validated against

- EP003 shots 02, 04, 08
- EP004 shots 01, 06
- EP005 shots 02, 07
- EP006 shot 01

## Cross-references

- See [fengge's character.yaml § scenes → bamboo_courtyard](../../characters/fengge/character.yaml).
- Compatible actions: [standing_calm.md](../actions/standing_calm.md),
  [walking_slow.md](../actions/walking_slow.md), [holding_cup.md](../actions/holding_cup.md).
- Compatible compositions: [medium_close_up.md](../compositions/medium_close_up.md),
  [wide_establishing.md](../compositions/wide_establishing.md).