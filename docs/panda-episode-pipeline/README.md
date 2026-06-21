# Panda Episode Pipeline

> End-to-end pipeline for producing **峰哥 / Fengge** panda IP short-drama
> episodes. From scaffolding to HTTP-served MP4, one command at a time.

## What this does

Scaffolds + renders 16-60s panda IP short-drama episodes for 抖音 / B 站 /
小红书. Each episode is:

- 4-12 shots × 4s each
- Same panda identity across all shots (Seedream `reference_image`)
- 1 of 4 voice tones (治愈 / 御宅 / 哲学 / 国潮)
- 5-outfit rotation + 12-scene whitelist (IP-bible enforced)
- TTS voiceover (海螺 MiniMax, per-tone emotion) + BGM + final mux

## Quick start

```bash
# 1. Set API keys (see INSTALL.md for where to get them)
export ARK_API_KEY="..."        # 火山方舟 (Seedream + Seedance)
export MINIMAX_API_KEY="..."    # 海螺 TTS

# 2. Scaffold a new episode from IP-bible vocabulary
./scripts/make_episode.sh \
  --slug panda-episode-007 \
  --tone 治愈 \
  --season 立冬 \
  --outfits "outfit_workwear_orange,outfit_robe_green,outfit_changshan_blue,outfit_workwear_orange" \
  --scenes "围炉夜话,雨后青苔,月下窗棂,茶室"

# 3. Edit the scaffolded files:
#    docs/showcase/panda-episode-007/STORYBOARD.md — fill [TODO] visual lines
#    docs/showcase/panda-episode-007/SCRIPT-douyin.md — fill [TODO] voiceover

# 4. Render
python3 docs/panda-episode-pipeline/render_episode.py \
  --storyboard docs/showcase/panda-episode-007/STORYBOARD.md \
  --script    docs/showcase/panda-episode-007/SCRIPT-douyin.md \
  --voice     治愈 \
  --platform  抖音 \
  --out       docs/showcase/panda-episode-007/RENDER
# Output: docs/showcase/panda-episode-007/RENDER/ep007-L3.mp4

# 5. (optional) View in HTTP showcase
#    Already served on http://localhost:19788/showcase/panda-episode-007/RENDER/
```

## Architecture

```
make_episode.sh              ← scaffold STORYBOARD.md + SCRIPT-douyin.md
       ↓
   (human edit)
       ↓
render_episode.py            ← full pipeline, see below
       ↓
   RENDER/
   ├── shot-0{1..N}.jpg     ← Seedream 4.0 (image gen)
   ├── shot-0{1..N}.mp4     ← Seedance 1.5-pro (image→video)
   ├── vo-0{1..N}.mp3       ← 海螺 TTS (per-tone voice profile)
   ├── bgm.wav              ← Python wave synth (rain + noise + envelope)
   └── epXXX-L3.mp4         ← ffmpeg mux (video + audio + subtitles)
```

## Identity anchor system

The panda's identity is locked via **inline base64 reference images** in
`opc-panda-plugin/reference/`:

| File | Outfit | Use |
|---|---|---|
| `panda-canonical-zh-red.jpg` | 朱红汉服 | **default anchor** for every shot |
| `panda-canonical-blue-changsan.jpg` | 宝蓝长衫 | outfit swatch |
| `panda-canonical-warm-orange.jpg` | 暖橙唐宋短打 | outfit swatch |

When `render_episode.py` runs, it reads `panda-canonical-zh-red.jpg`,
base64-encodes it inline, and sends it to Seedream as `reference_image`.
Seedream preserves the panda's face / eye-patch shape / ear placement /
body proportion across all shots, regardless of outfit or scene changes.

**Why this works without external infra**: jpgs are committed in git.
Render is fully self-contained, no signed URLs, no 24h TTL, no
internet roundtrip for the reference.

## 4-Tone voice matrix

| Tone | Visual register | TTS profile | Hook style |
|---|---|---|---|
| **治愈** | 暖光 / 茶烟 / 雨声 / 围炉 | speed 0.78, pitch -2, emotion neutral | 留白 / 画面钩子 |
| **御宅** | 室内小景 / 毛绒毯 / 深夜台灯 | speed 0.85, pitch -1, emotion neutral | 留白 / 故事钩子 |
| **哲学** | 月下 / 山水 / 远景 / 反问 | speed 0.82, pitch -3, voice male-qn-qingse, emotion sad | 三问 / 反常识 |
| **国潮** | 折扇 / 卷轴 / 红叶 / 樱花 | speed 0.92, pitch 0, emotion neutral | 引用 / 反常识 |

**Switching tone = change voice + visual register, NOT change character.**
Same panda + 4 voice profiles + 5 outfits + 12 scenes = infinite episodes
with consistent identity.

## Environment

```bash
# Required
ARK_API_KEY=...          # 火山方舟 — Seedream 4.0 + Seedance 1.5-pro
MINIMAX_API_KEY=...      # 海螺 — speech-02-hd TTS
PLUGIN_DIR=/path/to/opc-panda-plugin  # default: $REPO/opc-panda-plugin

# Optional
FFMPEG=/path/to/ffmpeg   # default: openclaw bundled, fallback /usr/bin/ffmpeg
```

ARK and MINIMAX keys are sourced from `~/.claude/config/opc-volcengine.env`
in dev environments — see INSTALL.md.

## Existing episodes (reference examples)

| Episode | Tone | Season | Outfits | Scenes |
|---|---|---|---|---|
| [panda-episode-003](../../showcase/panda-episode-003/) | 国潮 | 立夏 | 4 套全展示 | 月下窗棂 / 竹林小院 / 山水卷轴前 / 围炉夜话 |
| [panda-episode-004](../../showcase/panda-episode-004/) | 治愈 | 立秋 | 2 套循环 | 围炉夜话 / 雨后窗棂 / 茶室小景 / 竹林秋月 |
| [panda-episode-005](../../showcase/panda-episode-005/) | 御宅 | 冬至 | 2 套循环 | 竹林小院 / 围炉古书 / 月下窗棂 / 屋檐青苔 |
| [panda-episode-006](../../showcase/panda-episode-006/) | 哲学 | 春分 | 2 套循环 | 山水卷轴前 / 月下窗棂 / 古松下 / 雨后小径 |

## IP bible

The full character bible (5 套服饰 + 12 场景白名单 + 7 钩子公式 +
禁用词) lives in skill `opc-panda-character`. `make_episode.sh` reads
from it; you don't need to memorize it.

```
~/.claude/skills/opc-panda-character/SKILL.md   # canonical IP bible
```

## Plugin distribution

The pipeline is bundled in the `opc-panda-plugin` Claude Code plugin:

```bash
# One-line install (production)
curl -fsSL https://raw.githubusercontent.com/opc-project/opc-panda-plugin/main/scripts/install.sh | bash

# Local dev install
bash /root/workspace/opc/opc-panda-plugin/scripts/install.sh
```

After install:
- `opc-mcp` MCP server (stdio JSON-RPC) is registered with your agent
- `opc-panda-character` + `panda-episode-pipeline` skills are copied to
  your agent's skills directory
- Canonical reference images are bundled

## Known limitations

- **Cross-episode identity drift**: Seedream `reference_image` is best-effort.
  Across 4 episodes the panda's face is "similar" but not pixel-identical.
  For true identity lock, train a Seedream LoRA (≥5 episodes of training data).
- **BGM is synthesized**: Python wave module generates rain + noise + envelope.
  No traditional instruments (古筝 / 箫). Replace with Suno/Udio for production.
- **No QA gate**: shots are accepted as Seedream returns them. Add a visual
  audit step (using `panda_voice_analysis` skill) before mux for production.
- **Pacing is uniform**: 4s per shot. Vary per-tone (哲学 wants slower) for
  more emotional impact.

## Render command reference

```bash
python3 render_episode.py \
  --storyboard STORYBOARD.md \      # required
  --script SCRIPT-douyin.md \       # required
  --voice {治愈,御宅,哲学,国潮} \   # required
  --platform {抖音,哔哩哔哩,小红书} \# required
  --out PATH \                       # required (dir → epXXX-L3.mp4 inside)
  --target-dur 58 \                  # default
  --image-model doubao-seedream-4-0-250828 \
  --video-model doubao-seedance-1-5-pro-251215 \
  --reference-image PATH/OR/URL     # override default canonical
```

## See also

- `INSTALL.md` — environment setup
- `SKILL.md` — operational recipe (for Claude agents)
- `examples/` — older episode examples
- `../../showcase/index.md` — HTTP-served showcase index