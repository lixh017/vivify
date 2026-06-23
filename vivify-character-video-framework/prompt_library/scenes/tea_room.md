# 茶室 — Tea Room

> **Status:** ✅ validated — works in 6/6 attempts across EP003, EP004.
> Use for 治愈 / 国潮 / 哲学 tones when the scene calls for stillness,
> focus, and a single tactile prop (tea set).

## Environment details

- **Wooden tea table** (茶台) at center, low (~40cm), aged walnut
- **Yixing teapot** (紫砂壶) + 2 small cups on a bamboo tray
- **Bamboo tea tray** (竹制茶盘) under the cups
- **Linen runner** on the table, natural undyed color
- **Background**: papered wall (宣纸) or simple wooden paneling
- **Soft steam** rising from the teapot spout
- **One window** with diffused daylight (no harsh direct sun)

## Lighting

- **Time of day**: late morning, 10-11am
- **Quality**: diffused, indirect
- **Direction**: side-light from window, character gets 3/4 face lighting
- **No overhead light** — the room should feel "from the side"

## Color palette (hex)

- Aged walnut wood: `#5C4A3A`
- Linen natural: `#E8DEC9`
- Yixing terracotta: `#7A3B2E`
- Bamboo tray: `#9A8870`
- Steam: `#F0EFEC` (almost white)
- Papered wall: `#EFE7D8`

## Mood keywords

`grounded`, `focused`, `tactile`, `slow`, `ceremonial-but-casual`,
`breathing-room`

## Audio cues (Seedance 2.0 audio sync)

- **Single most important audio**: water pouring into a clay teapot
  (low-pitched ceramic sound)
- Steam hiss (very faint)
- Optional: single distant wooden floor creak
- **No music, no human voices** — silence carries the scene

## Camera angle suggestions

- **Default**: medium close-up, character across the table from
  camera, table in lower 1/3
- **Detail variant**: tight on the teapot spout pouring, with
  character's hand entering frame
- **Over-shoulder variant**: looking past the character at the
  papered wall and one window

## How to use in a per-shot prompt

```text
场景: 茶室, 上午10-11点, 侧窗漫射光, 茶台 + 紫砂壶 + 竹茶盘 + 亚麻桌旗, 壶嘴轻烟
配色: 胡桃木 #5C4A3A, 亚麻 #E8DEC9, 紫砂 #7A3B2E
氛围: 沉稳、专注、有触感
音频: 注水入紫砂壶的陶瓷声 + 微弱蒸汽嘶声, 无音乐无人声
```

## How to use in character.yaml

```yaml
character:
  scenes:
    - id: tea_room
      name: "茶室"
      mood: "grounded, focused"
      lighting: "late morning 10-11am, diffused side-light"
      palette:
        - "#5C4A3A"  # aged walnut
        - "#E8DEC9"  # linen
        - "#7A3B2E"  # yixing terracotta
```

## Failure modes

| Failure | Symptom | Fix |
|---------|---------|-----|
| Drifts to cafe | latte art, espresso machine, neon | Add `TRADITIONAL CHINESE TEA CEREMONY, NOT CAFE, NOT ESPRESSO` |
| Drifts to teahouse crowd | many people, bustle | Add `SOLO CHARACTER, EMPTY ROOM, SILENT` |
| Tea set becomes porcelain | white-and-blue porcelain | Add `YIXING PURPLE CLAY (紫砂), NOT PORCELAIN, NOT WHITE-AND-BLUE` |
| Overhead light appears | fluorescent / overhead | Add `SIDE WINDOW LIGHT ONLY, NO OVERHEAD, NO FLUORESCENT` |

## Validated against

- EP003 shot 06 (茶室, 国潮)
- EP003 shot 12 (茶室, 哲学)
- EP004 shot 04 (茶室, 治愈)
- EP004 shot 08 (茶室, 御宅)
- EP005 shot 09 (茶室, 治愈)
- EP006 shot 03 (茶室, 国潮)

## Cross-references

- Compatible actions: [holding_cup.md](../actions/holding_cup.md),
  [sitting_by_fire.md](../actions/sitting_by_fire.md),
  [standing_calm.md](../actions/standing_calm.md).
- Compatible compositions: [detail.md](../compositions/detail.md),
  [over_shoulder.md](../compositions/over_shoulder.md),
  [medium_close_up.md](../compositions/medium_close_up.md).