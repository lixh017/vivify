---
name: panda-episode-pipeline
description: Use this skill when rendering a panda IP episode end-to-end — from a storyboard (Markdown with shot prompts) + script (旁白 with timestamp ranges), through image generation (火山 Ark Seedream 4.0), image-to-video (Seedance 1.5-pro), TTS voiceover (海螺 MiniMax speech-02-hd), BGM synthesis (ffmpeg sine + brown noise), and final mix. Triggers on "render panda episode", "出 panda 视频", "panda video pipeline", "出 episode", or any reference to producing a 抖音 熊猫 episode.
---

# Panda Episode Pipeline (L2 + L3 一键出片)

End-to-end renderer that takes a **L1 storyboard + script** and produces a **L3 finished MP4** (video + TTS voiceover + BGM + phone comments overlay). Encodes the recipe proven by the panda EP#001 render (June 2026): 58s 抖音 episode in 12 minutes for ~¥11-19.

## When to Use

- User has a panda IP storyboard + script and wants the finished video
- "Render episode #002 / #003 / ..." on a new storyboard
- Re-rendering an existing storyboard with a different voice / duration / aspect ratio
- Bulk-generating episodes from a series of storyboards (loop the script)

## When NOT to Use

- Panda IP is **not** the target → use a generic video skill (video-editing, fal-ai-media)
- Just the script or storyboard text (no render) → use the panda_voice / panda_script MCP tools
- Final cut in 剪映 / Descript / Premiere → these are manual UI tools, no API
- 抖音 publishing → needs 抖音开放平台 key, out of scope for this skill

## Inputs

The skill expects these files in the working directory:

| File | Format | Required | Example |
|---|---|---|---|
| `STORYBOARD.md` | Markdown with N shots, each with: 视觉 / 构图 / 时长 / 镜头运动 / 可灵 prompt | Yes | `docs/showcase/panda-episode-001/STORYBOARD.md` |
| `SCRIPT-douyin.md` | Markdown with 旁白 timed to each shot | Yes | `docs/showcase/panda-episode-001/SCRIPT-douyin.md` |
| `voice` | One of 治愈 / 御宅 / 哲学 / 国潮 | Yes | `治愈` |
| `platform` | 抖音 / 哔哩哔哩 / 小红书 | Yes | `抖音` |
| `target_duration_sec` | Total episode length (default 58) | No | 58 |

The storyboard shot prompts should follow the panda IP bible at `~/.claude/skills/opc-panda-character/SKILL.md` (5 服饰, 4 调性, 场景白名单, panda_visual_anchor).

## Pipeline (20-40 min total, ¥11-19)

1. **L2a — Image generation** (Seedream 4.0 via 火山 Ark) — ~1 min for 6 images
2. **L2b — Image-to-video** (Seedance 1.5-pro via 火山 Ark) — ~5-8 min per video, serial (7 videos = 30-50 min total)
3. **L2c — Video assembly** (ffmpeg) — ~1 min
4. **L3a — TTS voiceover** (海螺 MiniMax) — ~10s for 6 segments
5. **L3b — BGM synthesis** (ffmpeg) — ~5s
6. **L3c — Phone comments overlay** (ffmpeg drawtext) — ~5s
7. **L3d — Final mix** (ffmpeg) — ~30s

> **Run as a background job**, not interactively. The L2b step alone takes 30-50 minutes. Plan for 1 hour total wall-clock time per episode.

## Required Environment

```bash
# Volcano Ark (image + video) — get from console.volcengine.com/ark
export ARK_API_KEY=...
export ARK_BASE_URL=https://ark.cn-beijing.volces.com/api/v3

# 海螺 MiniMax (TTS) — get from api.minimaxi.com
export MINIMAX_API_KEY=...

# ffmpeg — pre-built in openclaw node_modules (2018 vintage, syntax quirks)
export FFMPEG=/root/.openclaw/extensions/dingtalk-connector/node_modules/@ffmpeg-installer/linux-x64/ffmpeg

# Output dir
export OPC_RENDER_DIR=/tmp/opc-render
```

The script checks for these and exits with a clear message if missing. See `INSTALL.md` for the full setup.

## Usage

```bash
# From the repo root
python3 ~/.claude/skills/panda-episode-pipeline/render_episode.py \
  --storyboard docs/showcase/panda-episode-001/STORYBOARD.md \
  --script     docs/showcase/panda-episode-001/SCRIPT-douyin.md \
  --voice      治愈 \
  --platform   抖音 \
  --target-dur 58 \
  --out        docs/showcase/panda-episode-001/RENDER/panda-ep001-L3.mp4
```

Or in a Claude Code session, just say "render this storyboard" and Claude will run the script with the right env.

## Key Ffmpeg Gotchas (from the 2018 ffmpeg in this env)

- `amix` does **not** support `normalize=0` → omit the option, default behavior is fine
- `amix` does **not** support `duration=first` → omit, default uses longest
- `drawtext` with multiple lines: comma-separate them in a single `-vf` filter chain, don't try to break the filter
- `tpad=stop_mode=clone:stop_duration=N` to hold the last frame for N seconds (used to stretch shot 7 from 8s raw to 12s in the cut)
- `adelay=X|X` for stereo (must give both channels)
- Old ffmpeg 2018 may not have all the newer filter options — see the openclaw binary at `$FFMPEG`

## Cost

| Step | Cost per episode |
|---|---|
| Seedream 4.0 (6 images) | ¥0.10-0.30 |
| Seedance 1.5-pro i2v (7 clips, 720p, 5-8s) | ¥7-15 |
| 海螺 speech-02-hd (132 字) | ¥0.05-0.20 |
| ffmpeg BGM synthesis | ¥0 |
| **Total** | **¥11-19** |

For the 6-month 500万-views target (180 days, ~1 episode/day): **¥2,000-3,400 total**. ~1/100 of outsourcing (¥800-3,000/episode).

## Wall-Clock Time

Real-world timings from the panda EP#001 skill run (June 2026):

- L2a image gen: 1 min (6 images, serial)
- L2b i2v gen: 30-50 min (7 clips, serial — Seedance queues per shot)
- L2c ffmpeg assembly: 1 min
- L3a TTS: 10s
- L3b BGM synth: 5s
- L3c phone comments: 5s
- L3d final mix: 30s
- **Total: ~30-50 min per episode**

Run as `nohup python3 render_episode.py ... &` and check back. Do not run interactively.

## Limitations

- 720p only (Seedance 1.5-pro 1080p is more expensive and the 2018 ffmpeg has codec quirks; can be tuned)
- No 抖音 upload (needs 抖音开放平台 key)
- No A/B test variants (single voice / speed)
- BGM is synthesized, not real piano — for production, swap in licensed music (剪映 / Pixabay)
- Phone comments are static text (no actual scroll animation) — fine for the 8s slot

## Files in This Skill

- `SKILL.md` — this file
- `INSTALL.md` — env setup, API key acquisition, first-run smoke test
- `render_episode.py` — the runner (calls 火山 Ark + 海螺 + ffmpeg)
- `examples/panda-episode-001/` — reference inputs (storyboard + script)
