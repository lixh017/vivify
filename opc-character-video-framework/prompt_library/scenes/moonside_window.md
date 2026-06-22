# 月下窗棂 — Moonside Window

> **Status:** ✅ validated — works in 7/7 attempts across EP003–EP006.
> Use for 哲学 / 治愈 tones when the scene calls for nocturnal intimacy,
> solitude, and 留白 (negative space). The single most-used 哲学 scene.

## Environment details

- **Wooden lattice window** (木格窗棂) in the right 1/3 of frame
- **Moon** (full or 3/4) visible through the window, soft halo
- **Silhouette of distant tree branch** outside the window
- **Interior foreground**: ink stone / brush / paper / 茶盏 on a
  low table in the left 1/3
- **Soft moonlight spill** onto the table
- **NO** electric light, NO lamps

## Lighting

- **Time of day**: night, 11pm-1am
- **Quality**: cool moonlight, very soft
- **Direction**: through the window from camera-right, character
  backlit or in 3/4 silhouette
- **Shadow direction**: cast away from the window
- **Color temperature**: cool blue (NOT warm)

## Color palette (hex)

- Moonlight blue: `#A8B5C8`
- Wood lattice: `#3D2E25`
- Ink stone black: `#1A1A1A`
- Papered wall (cool grey): `#9A9890`
- Distant tree: `#1F2A2A`
- Tea cup highlight: `#D4C7A8`

## Mood keywords

`nocturnal`, `intimate`, `solitude`, `contemplative`, `留白`,
`philosophical`

## Audio cues (Seedance 2.0 audio sync)

- **Single most important audio**: night cricket (low-frequency,
  repeating, sparse — NOT a chorus)
- Optional: distant single night bird
- Optional: paper rustle
- **No music, no human voices, no wind** — quietness carries the scene

## Camera angle suggestions

- **Default**: medium shot, character between camera and window,
  rim-lit by moonlight
- **Profile shot**: character in pure silhouette against the
  window — use sparingly, only one per episode
- **Detail variant**: tight on the moonlit table with character's
  hand entering frame, holding brush or 茶盏

## How to use in a per-shot prompt

```text
场景: 月下窗棂, 深夜11-1点, 月光从木格窗洒入, 室内墨砚/毛笔/茶盏, 无电灯
配色: 月光蓝 #A8B5C8, 木窗 #3D2E25, 墨色 #1A1A1A
氛围: 夜晚、私密、独处、留白
音频: 稀疏的夜虫低鸣, 无音乐无人声无风
```

## How to use in character.yaml

```yaml
character:
  scenes:
    - id: moonlit_window
      name: "月下窗棂"
      mood: "nocturnal, intimate"
      lighting: "night 11pm-1am, cool moonlight, no electric light"
      palette:
        - "#A8B5C8"  # moonlight blue
        - "#3D2E25"  # wood lattice
        - "#1A1A1A"  # ink black
```

## Failure modes

| Failure | Symptom | Fix |
|---------|---------|-----|
| Drifts to daylight | window becomes bright | Add `DEEP NIGHT, NOT DAY, NO SUN, NO WARM LIGHT` |
| Lamps appear | electric desk lamp, candle, lantern | Add `NO ELECTRIC LIGHT, NO CANDLE, NO LANTERN, MOONLIGHT ONLY` |
| Moon becomes sun | harsh bright disc | Add `SOFT HALO MOON, NOT HARSH, NOT WHITE-HOT` |
| Drifts to Western window | stained glass, Victorian | Add `CHINESE LATTICE WINDOW (木格窗棂), NOT STAINED GLASS, NOT VICTORIAN` |
| Too many night sounds | wolf howls, owls, wind | Add `SINGLE SPARSE CRICKET, NOT CHORUS, NOT OWL, NOT WIND` |

## Validated against

- EP003 shot 03, 09
- EP004 shot 02
- EP005 shot 05
- EP006 shot 04, 07

## Cross-references

- See [fengge's character.yaml § scenes → moonlit_window](../../characters/fengge/character.yaml).
- Compatible actions: [leaning_window.md](../actions/leaning_window.md),
  [holding_cup.md](../actions/holding_cup.md),
  [standing_calm.md](../actions/standing_calm.md).
- Compatible compositions: [medium_close_up.md](../compositions/medium_close_up.md),
  [detail.md](../compositions/detail.md).