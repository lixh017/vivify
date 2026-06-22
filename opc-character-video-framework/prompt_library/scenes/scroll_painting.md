# 山水卷轴前 — Scroll Painting

> **Status:** ✅ validated — works in 5/5 attempts across EP003, EP006.
> Use for 国潮 / 哲学 tones when the scene calls for cultural depth,
> 留白, and a "scholar / contemplator" register. Strongest 国潮 scene.

## Environment details

- **Large vertical scroll** (立轴) hanging in the back third of frame
- **Scroll subject**: ink landscape with distant mountains + mist
  (山水), with 1-2 small figures
- **Wooden scroll rollers** (天杆) at top and bottom
- **Low writing table** (书案) in the foreground with: ink stone,
  brush rest, paper weight (镇纸)
- **Optional**: chair with cushion, no character sitting
- **Floor**: aged wooden plank

## Lighting

- **Time of day**: afternoon, 2-4pm
- **Quality**: soft daylight, no harsh direct sun
- **Direction**: side light from camera-left, so the scroll gets
  gentle wash and the table gets soft shadow
- **No overhead light**

## Color palette (hex)

- Aged wood: `#5C4A3A`
- Scroll paper (rice paper): `#EFE5C9`
- Scroll ink: `#1A1A1A`
- Ink stone: `#2A2520`
- Brush rest wood: `#7A5A3E`
- Distant mist: `#D4D0C0`

## Mood keywords

`classical`, `contemplative`, `scholarly`, `留白`, `cultural-depth`,
`国潮`

## Audio cues (Seedance 2.0 audio sync)

- **Single most important audio**: faint brush on paper rustle (only
  if character is writing)
- Optional: distant single bird call
- Optional: very faint paper-weight click
- **No music, no voices, no wind**

## Camera angle suggestions

- **Default**: wide shot, scroll occupying back 2/3 of frame,
  character's back/side in foreground
- **Detail variant**: tight on the ink stone + brush, with the
  scroll out of focus in the background
- **Reverse angle**: looking past the character AT the scroll, so
  the scroll is in full view

## How to use in a per-shot prompt

```text
场景: 山水卷轴前, 下午2-4点, 侧光, 立轴山水 + 书案 + 砚台 + 笔架 + 镇纸, 旧木地板
配色: 旧木 #5C4A3A, 宣纸 #EFE5C9, 墨色 #1A1A1A
氛围: 古典、沉思、有文化纵深
音频: 微弱的笔触宣纸声(若书写), 无音乐无人声
```

## How to use in character.yaml

```yaml
character:
  scenes:
    - id: scroll_painting
      name: "山水卷轴前"
      mood: "classical, contemplative"
      lighting: "afternoon 2-4pm, soft side-light, no overhead"
      palette:
        - "#5C4A3A"  # aged wood
        - "#EFE5C9"  # rice paper
        - "#1A1A1A"  # ink black
```

## Failure modes

| Failure | Symptom | Fix |
|---------|---------|-----|
| Drifts to Western art gallery | framed canvas, museum | Add `CHINESE VERTICAL SCROLL (立轴), NOT FRAMED CANVAS, NOT MUSEUM` |
| Drifts to calligraphy studio | many brushes, ink splatters | Add `CLEAN SCHOLAR'S DESK, NOT MESSY STUDIO, NOT INK-SPLATTERED` |
| Mountains become Alps | snowy peaks | Add `INK LANDSCAPE MOUNTAINS, NOT PHOTOREALISTIC, NOT ALPS, NOT SNOWY` |
| Scroll subject becomes Western painting | oil landscape, Renaissance | Add `INK PAINTING (水墨), NOT OIL, NOT WESTERN, NOT RENAISSANCE` |
| Character blocks scroll | full-body in front of scroll | Add `CHARACTER IN FOREGROUND 1/3, SCROLL IN BACKGROUND 2/3` |

## Validated against

- EP003 shot 05
- EP006 shot 09 (the ink-brush anchor test — see [ink-brush.md](../styles/ink-brush.md))
- EP006 shots 11, 12
- EP003 shot 10

## Cross-references

- Compatible actions: [standing_calm.md](../actions/standing_calm.md),
  [holding_fan.md](../actions/holding_fan.md).
- Compatible compositions: [wide_establishing.md](../compositions/wide_establishing.md),
  [detail.md](../compositions/detail.md),
  [over_shoulder.md](../compositions/over_shoulder.md).