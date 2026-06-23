# Prompt Library — Index

> Reusable prompt fragments for IP authors. Copy-paste into
> `characters/<your-ip>/character.yaml` and into per-shot prompts.

Every fragment here has been written or stress-tested against the
**峰哥 / Fengge** showcase IP (EP003–EP006, 5 outfits, 12 scenes,
4 voice tones). Fragments are marked:

- ✅ **validated** — proven across 8+ production shots
- 🟡 **partial** — works in 2-7 shots, may need tuning per scene
- 🔴 **draft** — single-shot unverified, use with caution

## Categories

| Category | Path | Purpose | Count |
|----------|------|---------|-------|
| Visual styles | [`styles/`](./styles/INDEX.md) | `style_anchor` text appended to every kling_prompt to lock visual register (2D gouache, watercolor, ink, cyberpunk neon, line art, ukiyo-e) | 6 |
| Scenes | [`scenes/`](./scenes/) | Reusable environment descriptions — bamboo courtyard, tea room, fireside chat, etc. Use in `character.yaml → scenes` and per-shot prompts. | 6 |
| Actions | [`actions/`](./actions/) | Body-action templates — standing calm, sitting by fire, holding fan, walking, etc. Use when writing per-shot Seedance prompts. | 6 |
| Compositions | [`compositions/`](./compositions/) | Shot framing — medium close-up, wide establishing, over-shoulder, low-angle, detail. Append to `运镜` field. | 5 |
| Characters | [`characters/`](./characters/) | `character.yaml` schema reference + fengge IP summary. Required reading before authoring a new IP. | 2 |
| Canonical prompts | [`canonical-prompts.md`](./canonical-prompts.md) | How to generate the 3 canonical reference jpgs (front / 3-4 / side) for any new IP. | 1 |

## How to use

1. **Picking a style**: read [`styles/INDEX.md`](./styles/INDEX.md). Pick the
   anchor that matches your IP's tone. The validated one (国潮 2D gouache)
   has been stress-tested across 30+ shots.
2. **Building scenes**: copy a scene fragment from [`scenes/`](./scenes/)
   into `character.yaml → scenes`. Then append the same fragment to each
   per-shot prompt that uses it.
3. **Writing a shot prompt**: read [`characters/format.md`](./characters/format.md)
   for the per-shot structure (运镜 / 主体 / 动作 / 位置 / 音频).
4. **Generating canonical refs**: follow [`canonical-prompts.md`](./canonical-prompts.md)
   for the 3 neutral-pose jpgs that Seedream will use as `reference_image`.

## Quick links to specific files

### styles/
- [styles/INDEX.md](./styles/INDEX.md)
- [styles/guochao-2d.md](./styles/guochao-2d.md) — 国潮 2D gouache watercolor ✅
- [styles/zhiyu-watercolor.md](./styles/zhiyu-watercolor.md) — 治愈 watercolor 🟡
- [styles/ink-brush.md](./styles/ink-brush.md) — 水墨写意 🔴

### scenes/
- [scenes/bamboo_courtyard.md](./scenes/bamboo_courtyard.md) — 竹林小院 ✅
- [scenes/tea_room.md](./scenes/tea_room.md) — 茶室 ✅
- [scenes/moonside_window.md](./scenes/moonside_window.md) — 月下窗棂 ✅
- [scenes/fireside_chat.md](./scenes/fireside_chat.md) — 围炉夜话 ✅
- [scenes/scroll_painting.md](./scenes/scroll_painting.md) — 山水卷轴前 ✅
- [scenes/moss_after_rain.md](./scenes/moss_after_rain.md) — 雨后青苔 ✅

### actions/
- [actions/standing_calm.md](./actions/standing_calm.md) — 站姿 + 半阖眼 ✅
- [actions/sitting_by_fire.md](./actions/sitting_by_fire.md) — 抱膝坐于炉旁 ✅
- [actions/holding_fan.md](./actions/holding_fan.md) — 持折扇(缓展) ✅
- [actions/holding_cup.md](./actions/holding_cup.md) — 捧茶盏 ✅
- [actions/walking_slow.md](./actions/walking_slow.md) — 缓步竹林 ✅
- [actions/leaning_window.md](./actions/leaning_window.md) — 倚窗望雨 ✅

### compositions/
- [compositions/medium_close_up.md](./compositions/medium_close_up.md) — 中近景 ✅
- [compositions/wide_establishing.md](./compositions/wide_establishing.md) — 全景建立镜头 ✅
- [compositions/over_shoulder.md](./compositions/over_shoulder.md) — 过肩镜头 ✅
- [compositions/low_angle.md](./compositions/low_angle.md) — 仰拍 ✅
- [compositions/detail.md](./compositions/detail.md) — 细节特写 ✅

### characters/
- [characters/format.md](./characters/format.md) — character.yaml schema documentation ✅
- [characters/fengge-summary.md](./characters/fengge-summary.md) — 峰哥 IP summary ✅

### Top-level
- [canonical-prompts.md](./canonical-prompts.md) — how to generate canonical reference images ✅

## Cross-references

- Pipeline overview: [`../README.md`](../README.md)
- Onboarding guide: [`../docs/adding-new-character.md`](../docs/adding-new-character.md)
- fengge IP bible (real-world example): [`../characters/fengge/character.yaml`](../characters/fengge/character.yaml)
- Prompt-engineering memory: [`../memory/prompt-engineering/`](../memory/prompt-engineering/)