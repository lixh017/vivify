---
description: Render a panda IP episode end-to-end (L1 script → L2 image+video → L3 voiceover+BGM). Use when you have a storyboard + script and want a finished MP4.
---

# /panda-render — Render panda IP episode

Render a panda IP episode end-to-end. Takes a STORYBOARD.md and SCRIPT-douyin.md and produces a finished L3 MP4 in the same directory.

## Usage

```
/panda-render <storyboard.md> <script.md> [voice] [platform] [target-duration-sec] [output.mp4]
```

Defaults:
- voice: 治愈
- platform: 抖音
- target-duration: 58
- output: `<storyboard-dir>/RENDER/<storyboard-stem>-L3.mp4`

## Examples

```
/panda-render docs/showcase/panda-episode-002/STORYBOARD.md docs/showcase/panda-episode-002/SCRIPT-douyin.md
/pp-render /tmp/ep003/STORYBOARD.md /tmp/ep003/SCRIPT.md 国潮 抖音 32 /tmp/ep003.mp4
```

## What it does

1. Calls `python3 ${CLAUDE_PLUGIN_ROOT}/skills/panda-episode-pipeline/render_episode.py` with the args
2. Pipeline: Seedream 4.0 (image) → Seedance 1.5-pro (i2v) → 海螺 speech-02-hd (TTS) → ffmpeg BGM + mix → final MP4
3. Total: 30-50 min cold / 1-2 min warm (cache hit)
4. Cost: ~¥11-19 per episode (火山 Ark + 海螺)

## Prerequisites

- `ARK_API_KEY` env var (火山引擎 Ark, https://console.volcengine.com/ark/)
- `MINIMAX_API_KEY` env var (海螺 MiniMax, https://api.minimaxi.com)
- `ffmpeg` on PATH (the panda-episode-pipeline skill has a custom path detection if not)
