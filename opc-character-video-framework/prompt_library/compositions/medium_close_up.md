# 中近景 — Medium Close-Up

> **Status:** ✅ validated — the workhorse composition. Used in 60%+
> of all fengge shots. Default unless you have a reason to pick
> something else.

## Framing description

- **Subject occupies 50-70% of frame height** (from head to upper
  torso)
- **Head is the visual center** (rule of thirds: eyes on upper third)
- **Some breathing room above the head** (10-15% of frame)
- **Cut at mid-torso or just below shoulders**
- **1-2 supporting props visible** (cup, fan, paper, etc.)

## Camera angle

- **Eye level** (camera at subject's eye height)
- **3/4 face** (not full-frontal, not profile — slight 30° turn)
- **Subject is looking slightly off-camera** (per the IP rule
  `不直视镜头`)

## Depth of field

- **Shallow DOF** — subject in sharp focus, background softly
  blurred (but not out-of-focus blobs)
- **Aperture equivalent**: ~f/2.0-f/2.8
- **Background bokeh** gentle, not chunky

## Best for

- Action: any prop-based pose (fan, cup, brush, scroll)
- Action: contemplative poses (sitting, leaning)
- Action: dialogue / voiceover register (the face is visible
  enough to anchor emotion)
- Reveal: a soft character intro (without giving the full-body
  silhouette)

## When to use (compatible tones)

- 治愈 ✅
- 国潮 ✅
- 哲学 ✅
- 御宅 ✅ (most common)

## When NOT to use

- Scene-establishing shots (use [wide_establishing.md](./wide_establishing.md))
- Prop-detail reveal (use [detail.md](./detail.md))
- Dramatic power shots (use [low_angle.md](./low_angle.md))

## Compatible actions

- [standing_calm.md](../actions/standing_calm.md) ✅
- [sitting_by_fire.md](../actions/sitting_by_fire.md) ✅
- [holding_cup.md](../actions/holding_cup.md) ✅
- [holding_fan.md](../actions/holding_fan.md) ✅
- [leaning_window.md](../actions/leaning_window.md) ✅
- [walking_slow.md](../actions/walking_slow.md) 🟡 (motion blur risk)

## Compatible scenes

- All 6 scenes in [scenes/](../scenes/) ✅

## How to use in a per-shot prompt

Append to the Seedance 2.0 公式 运镜 field:

```text
运镜: 中近景, 主体占画面高度50-70%, 头部位于上1/3, 3/4面部, 浅景深(f/2.0-2.8), 背景柔焦
```

## Failure modes

| Failure | Symptom | Fix |
|---------|---------|-----|
| Too tight | head fills frame | Add `CUT AT MID-TORSO, NOT CHIN, NOT NECK` |
| Too loose | head is small | Add `HEAD 50-70% OF FRAME HEIGHT, NOT 30%, NOT TINY` |
| Full frontal | breaks IP rule | Add `3/4 FACE, NOT FULL FRONTAL, NOT PROFILE, 30° TURN` |
| Deep DOF | background sharp | Add `SHALLOW DOF f/2.0-2.8, BACKGROUND SOFTLY BLURRED` |
| Eyes at camera | breaks IP rule | Add `GAZE OFF-CAMERA, NOT AT CAMERA, NOT AT VIEWER` |

## Cross-references

- For the wide counterpart, see [wide_establishing.md](./wide_establishing.md).
- For prop details, see [detail.md](./detail.md).
- For the over-shoulder variant, see [over_shoulder.md](./over_shoulder.md).