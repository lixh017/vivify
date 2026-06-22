# 雨后青苔 — Moss After Rain

> **Status:** ✅ validated — works in 6/6 attempts across EP005, EP006.
> Use for 治愈 / 哲学 tones when the scene calls for fresh, quiet,
> dewy, post-rain register. Strongest 治愈 scene for outdoor shots.

## Environment details

- **Wet moss** (青苔) covering stones in the foreground 1/3
- **Droplets** still on leaves, with soft light catch
- **3-4 small stones** half-buried in the moss
- **Bamboo leaves** above, with droplets catching the light
- **Wooden bridge or path edge** (optional) in the back 1/3
- **Distant mist** softening the background
- **NO** running water, NO puddle reflections (those read as too busy)

## Lighting

- **Time of day**: early morning, 6-7am (right after rain stops)
- **Quality**: soft diffused, slightly cool
- **Direction**: from above (overcast sky)
- **Sky**: pale grey-white, NOT blue
- **Droplets catch** the soft light (key visual)

## Color palette (hex)

- Moss green: `#5A7A4A`
- Wet stone grey: `#5A5A55`
- Bamboo leaf: `#7BA05B`
- Droplet highlight: `#D4DCE0`
- Wood (aged bridge): `#6B4F3A`
- Sky: `#D4D8D2`

## Mood keywords

`fresh`, `quiet`, `dewy`, `post-rain`, `breathing-room`, `grounded`

## Audio cues (Seedance 2.0 audio sync)

- **Single most important audio**: last droplets falling from bamboo
  leaves (sparse, slow, soft)
- Optional: distant single bird (early morning)
- **No music, no voices, no thunder, no wind**

## Camera angle suggestions

- **Default**: detail shot, tight on moss + droplets, character
  entering frame from one side
- **Wide variant**: low angle, character walking across the bridge,
  moss in foreground
- **Top-down variant**: looking down at moss + character's hand
  reaching down

## How to use in a per-shot prompt

```text
场景: 雨后青苔, 清晨6-7点, 阴天漫射光, 湿青苔 + 水滴 + 竹叶 + 旧木桥, 远处薄雾
配色: 青苔 #5A7A4A, 湿石 #5A5A55, 竹叶 #7BA05B
氛围: 清新、安静、雨后、可呼吸
音频: 稀疏的竹叶水滴声 + 远处一声晨鸟, 无音乐无人声无雷
```

## How to use in character.yaml

```yaml
character:
  scenes:
    - id: moss_after_rain
      name: "雨后青苔"
      mood: "fresh, quiet"
      lighting: "early morning 6-7am, overcast soft light"
      palette:
        - "#5A7A4A"  # moss green
        - "#5A5A55"  # wet stone grey
        - "#7BA05B"  # bamboo leaf
```

## Failure modes

| Failure | Symptom | Fix |
|---------|---------|-----|
| Drifts to rainforest | tropical leaves, vines | Add `TEMPERATE CHINESE GARDEN, NOT TROPICAL, NOT RAINFOREST` |
| Puddle reflections appear | big reflective surface | Add `NO LARGE PUDDLES, NO REFLECTIONS, ONLY DROPLETS ON LEAVES` |
| Sun appears | blue sky, sharp light | Add `OVERCAST EARLY MORNING, NO SUN, NO BLUE SKY` |
| Drifts to still-raining | heavy rain visible | Add `RAIN JUST STOPPED, DRIPPING ONLY, NO ACTIVE RAINFALL` |
| Drifts to moss-only (no character) | pure nature shot | Add `CHARACTER IN FRAME, NOT NATURE-ONLY` |

## Validated against

- EP005 shots 01, 06, 08
- EP006 shots 05, 08, 10

## Cross-references

- Compatible actions: [walking_slow.md](../actions/walking_slow.md),
  [standing_calm.md](../actions/standing_calm.md),
  [leaning_window.md](../actions/leaning_window.md) (if window is
  visible).
- Compatible compositions: [detail.md](../compositions/detail.md),
  [low_angle.md](../compositions/low_angle.md),
  [wide_establishing.md](../compositions/wide_establishing.md).