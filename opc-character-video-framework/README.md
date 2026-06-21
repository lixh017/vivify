# OPC Character Video Framework

> **End-to-end AI short-drama pipeline for any custom IP character.**
> Drop in 3 reference images + a character profile → get a 抖音-ready MP4.

This is the framework that produced **峰哥 / Fengge** (the panda IP).
It's been refactored to be character-agnostic — add your own IP by
dropping reference images in `characters/<your-name>/canonical/` and
filling in `character.yaml`.

## What it does

For any character with:
- A clean, neutral-pose reference image (3 jpgs recommended)
- A character profile (5 outfits, scenes, voice tones, etc.)

The pipeline produces:
- 4-tone voice matrix (治愈 / 御宅 / 哲学 / 国潮, or define your own)
- 16-21s MP4 with title card, subtitles, end-card CTA
- Cross-shot identity consistency (Seedream `reference_image`)
- Per-tone TTS voice (海螺 MiniMax speech-02-hd with emotion)
- Locked visual style (style anchor from character.yaml)

## Quick start

```bash
# Install deps
pip install pyyaml
export ARK_API_KEY="..."        # 火山方舟 (Seedream + Seedance)
export MINIMAX_API_KEY="..."    # 海螺 TTS

# Render with the bundled 峰哥 panda character (showcase)
python3 render_episode.py \
  --character-dir characters/fengge \
  --storyboard examples/panda-episode-004/STORYBOARD.md \
  --script    examples/panda-episode-004/SCRIPT-douyin.md \
  --voice     治愈 --platform 抖音 \
  --out       /tmp/out
# Output: /tmp/out/panda-episode-004-L3.mp4

# Use the scaffold tool
./make_episode.sh \
  --character fengge \
  --slug panda-episode-007 \
  --tone 治愈 --season 立冬 \
  --outfits "outfit_workwear_orange,outfit_robe_green,outfit_changshan_blue,outfit_workwear_orange" \
  --scenes "围炉夜话,雨后青苔,月下窗棂,茶室"
```

## Add your own character

See **[docs/adding-new-character.md](docs/adding-new-character.md)** for
the step-by-step onboarding. TL;DR:

1. Prepare 3 jpgs of your character in clean, neutral poses (front or
   3/4 view, no props, plain background). Drop them in
   `characters/<your-name>/canonical/`.
2. Copy `characters/_template/character.yaml` to
   `characters/<your-name>/character.yaml`. Fill in:
   - `name`, `species`, `head_body_ratio`, fur colors
   - 5+ outfits with anti-patterns (what to avoid)
   - 5+ scenes
   - Voice profiles for each tone you want
   - `style_anchor` — the visual register you want locked across shots
   - Hook formulas + voice rules
3. Run the pipeline:
   ```bash
   python3 render_episode.py --character-dir characters/<your-name> ...
   ```

That's it. No code changes needed.

## Repository layout

```
opc-character-video-framework/
├── README.md                      (this file)
├── render_episode.py              (character-agnostic pipeline)
├── make_episode.sh                (scaffold tool)
├── character_loader.py            (loads character.yaml)
├── characters/
│   ├── fengge/                    (showcase — panda 峰哥)
│   │   ├── character.yaml         (5 outfits, 12 scenes, 4-tone voice)
│   │   ├── canonical/             (3 reference jpgs)
│   │   └── examples/              (4 demo episodes)
│   └── _template/                 (drop-in for new characters)
│       ├── character.yaml         (minimal schema)
│       └── canonical/             (drop your jpgs here)
└── docs/
    ├── adding-new-character.md    (onboarding guide)
    └── architecture.md            (how the pipeline works)
```

## How the pipeline works

```
make_episode.sh                  ← scaffold STORYBOARD.md + SCRIPT-douyin.md
       ↓
   (human edit)
       ↓
render_episode.py
   ├─ L1: parse storyboard + script (character.yaml: outfits/scenes/voice)
   ├─ L2a: Seedream 4.0 image gen
   │     ├─ canonical reference_image (inline base64, identity preserved)
   │     └─ style_anchor suffix (visual register locked)
   ├─ L2b: Seedance 1.5-pro image→video
   ├─ L3a: 海螺 TTS voiceover (per-tone voice_id + emotion + speed)
   ├─ L3b: BGM synth (Python wave module, character-agnostic)
   ├─ L3e: title card + subtitle overlay + end card (content craft)
   └─ L3f: ffmpeg mux → epXXX-L3.mp4
```

The character.yaml is loaded once at startup. The pipeline references it for:
- Canonical reference image (`canonical.primary`)
- Outfit descriptions (for kling_prompt augmentation)
- Scene whitelists (validated against character config)
- Voice profiles (one per tone)
- Style anchor (locked across all shots)

## Why this design

**Inline base64 reference images** — no TOS uploads, no signed URLs, no 24h TTL.
The jpgs are committed in the repo and sent directly to Seedream.

**One canonical pose, multiple outfits** — instead of generating many
identity-bleeding creative compositions, we use one neutral pose per
outfit. The reference preserves identity, the prompt controls scene/props.

**Style anchor in every prompt** — prevents Seedream from drifting
between 3D-rendered toy aesthetic / photoreal / watercolor across shots.
Every shot gets the same style suffix from character.yaml.

**Per-tone voice profiles** —治愈 / 御宅 / 哲学 / 国潮 each get a
distinct audible register (different voice_id / speed / pitch / emotion)
so the audio matches the visual register.

## Known limitations

- **Cross-episode identity drift**: Seedream `reference_image` is
  best-effort. For pixel-perfect identity across many episodes, train a
  custom LoRA.
- **BGM is synthesized**: Python wave module generates rain + envelope.
  Replace with Suno/Udio API for production.
- **No QA gate**: shots are accepted as Seedream returns them. Add a
  visual audit step (using the IP bible) before mux for production.
- **Pacing is uniform within an episode unless overridden**: each shot
  defaults to the same duration. Override via storyboard `[mm:ss-mm:ss]`
  for variable pacing.

## License

MIT. Use it for whatever.

## Credits

Built on:
- 火山方舟 Seedream 4.0 (image gen) + Seedance 1.5-pro (image→video)
- 海螺 MiniMax speech-02-hd (TTS)
- WQY Zen Hei font (Chinese rendering)
- ffmpeg (mux / drawtext)
- Python 3 (orchestration)

Showcase character 峰哥 / Fengge is OPC's flagship IP (panda, 国潮).

Contributions welcome — fork, send PRs, add your own characters.