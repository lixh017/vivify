# 围炉夜话 — Fireside Chat

> **Status:** ✅ validated — works in 6/6 attempts across EP004–EP006.
> Use for 治愈 / 国潮 tones when the scene calls for warmth, intimacy,
> and a single anchor (the fire / brazier). Strongest 治愈 scene.

## Environment details

- **Clay brazier** (围炉) at center, low (~30cm)
- **Glowing charcoal** (炭火) in the brazier, with soft orange-red embers
- **Iron kettle** or **earthen teapot** on top of the brazier
- **Floor cushions** (蒲团) 2-3 around the brazier
- **Knit blanket** (毛毯) draped over one cushion
- **Background**: dim wooden interior, papered wall
- **Soft smoke wisp** from the brazier

## Lighting

- **Time of day**: late evening, 8-10pm
- **Quality**: warm firelight, flickering
- **Direction**: from the brazier outward, casting long shadows
  away from camera
- **Color temperature**: warm orange (NOT cool)
- **Background falls into soft darkness** — DOF + dim

## Color palette (hex)

- Brazier terracotta: `#7A3B2E`
- Ember orange-red: `#D4622A`
- Charcoal black: `#1A1410`
- Knit blanket cream: `#E8DEC9`
- Papered wall (warm): `#A89880`
- Smoke grey: `#7A7060`

## Mood keywords

`cozy`, `intimate`, `warm`, `tactile`, `low-light`, `companionable`,
`hearth`

## Audio cues (Seedance 2.0 audio sync)

- **Single most important audio**: brazier ember crackle (low, slow,
  sparse pops — NOT a roaring fire)
- Optional: faint kettle low hiss
- **No wind, no music, no voices** — silence + crackle carries the scene

## Camera angle suggestions

- **Default**: medium shot, character sitting on a 蒲团 with the
  brazier in the foreground 1/3, firelight on the face
- **Detail variant**: tight on the kettle spout, with character's
  hand reaching for a cup
- **Wide variant**: brazier center, two cushions (one occupied),
  long shadow cast on wall

## How to use in a per-shot prompt

```text
场景: 围炉夜话, 晚8-10点, 暖色炉火侧光, 围炉 + 炭火 + 铁壶 + 蒲团 + 毛毯, 背景昏暗
配色: 炉 #7A3B2E, 炭火 #D4622A, 毛毯 #E8DEC9
氛围: 温暖、亲密、可触摸
音频: 稀疏的炭火噼啪声 + 微弱水壶低鸣, 无音乐无人声
```

## How to use in character.yaml

```yaml
character:
  scenes:
    - id: fireside_chat
      name: "围炉夜话"
      mood: "cozy, intimate"
      lighting: "late evening 8-10pm, warm firelight from brazier"
      palette:
        - "#7A3B2E"  # brazier terracotta
        - "#D4622A"  # ember orange
        - "#E8DEC9"  # knit blanket cream
```

## Failure modes

| Failure | Symptom | Fix |
|---------|---------|-----|
| Brazier becomes Western fireplace | stone hearth, log fire, mantel | Add `CLAY BRAZIER (围炉), NOT FIREPLACE, NOT MANTEL, NOT LOG FIRE` |
| Light becomes electric | desk lamp, ceiling light | Add `FIRELIGHT ONLY, NO ELECTRIC, NO LAMPS, NO CEILING LIGHT` |
| Roaring fire appears | big flames | Add `LOW EMBER GLOW, NO FLAMES, NO ROARING FIRE` |
| Too bright | daylight visible through window | Add `DEEP EVENING, NO WINDOW DAYLIGHT, FULL DARK EXTERIOR` |
| Multiple braziers | two/three fires | Add `SINGLE BRAZIER, NOT MULTIPLE` |

## Validated against

- EP004 shots 03, 07, 10
- EP005 shot 04
- EP006 shots 02, 06

## Cross-references

- Compatible actions: [sitting_by_fire.md](../actions/sitting_by_fire.md),
  [holding_cup.md](../actions/holding_cup.md),
  [leaning_window.md](../actions/leaning_window.md).
- Compatible compositions: [medium_close_up.md](../compositions/medium_close_up.md),
  [over_shoulder.md](../compositions/over_shoulder.md),
  [detail.md](../compositions/detail.md).