# OPC Character Video Framework — Agent Instructions

> ⚠️ READ THIS FIRST if you are an AI agent working on this framework.

## What this project does
- Produces IP character short drama videos
- Pipeline: Seedream (image) → Seedance (video) → TTS (voice) → ffmpeg (mux)
- Character-agnostic: any IP via character.yaml config
- Currently showcase IP: 峰哥 (panda)

## Workflow: Adding a new IP character

1. Read `memory/INDEX.md` for relevant **cross-IP** lessons (prompt engineering, model capabilities)
2. Read `prompt_library/INDEX.md` for available prompt templates
3. Read `characters/fengge/` as a worked example (one IP's complete data layer)
4. Ask user 5 questions: species, palette, personality, scenes, tone
5. Generate `characters/<name>/character.yaml` using `prompt_library/characters/format.md`
6. Run: `python3 validators/lint_character.py characters/<name>`
7. Fix all lint errors (especially `anti_patterns` needs ≥2 negative tokens each)
8. Generate 3 canonical reference images (close-up face + full-body × 3 outfits)
   - Use `prompt_library/canonical-prompts.md` for prompts
   - **CRITICAL**: close-up face shot must be SEPARATE from full-body (avoids ID drift)
9. Run `python3 validators/canonical_image_check.py characters/<name>/canonical`
10. Run 1 test episode
11. Run `python3 validators/lint_prompt.py "<prompt>"` on each shot
12. Write `characters/<name>/lessons.md` recording what worked/didn't (this is IP-specific)
13. Iterate

## Workflow: Writing a shot prompt

ALWAYS use the Seedance 2.0 公式 (see memory/prompt-engineering/seedance-formula.md):

```
镜头 N: [运镜] [主体] [动作] [位置] [音频]
```

Add at end:
- Style anchor from prompt_library/styles/
- Constraint words: 保持无字幕, 不要生成水印, 不要生成 Logo

## Workflow: Fixing a quality issue

- ID drift (face changes between shots) → `memory/prompt-engineering/id-drift-prevention.md`
- Style drift (3D vs 2D) → `memory/prompt-engineering/style-anchors.md`
- Wrong outfit rendered → check `characters/<ip>/character.yaml` outfit `anti_patterns` strength (≥2 negative tokens)
- BGM too noisy → `characters/<ip>/lessons.md` BGM section
- **When you find a fix, write it to the appropriate location**:
  - Cross-IP lesson → `memory/<topic>/`
  - IP-specific lesson → `characters/<ip>/lessons.md`

## Strictly forbidden

- ❌ Single-paragraph prompts (must use 分镜 structure)
- ❌ Abstract emotion words without body action (必须用具象动作)
- ❌ Multiple camera movements in one shot (1 only)
- ❌ Using full-body shot as face reference (must have separate 大头照)
- ❌ Writing prompts without consulting prompt_library/

## Tech stack

- Image gen: Seedream 4.0 (`doubao-seedream-4-0-250828`)
- Video gen: Seedance 2.0 (audio sync) / 1.5-pro / 1.0-pro-fast
- Voice: MiniMax speech-02-hd (海螺, returns HEX not base64)
- Mux: ffmpeg with WQY Zen Hei font for Chinese
- Model routing: `model_router.py` (auto tier + fallback)

## File layout

```
vivify-character-video-framework/
├── CLAUDE.md                    ← you are here (agent instructions)
├── render_episode.py            ← DEPRECATED legacy single-shot pipeline (escape hatch only — see file docstring)
├── make_episode.sh              ← scaffold tool
├── model_router.py              ← auto tier + fallback
├── qa_gate.py                   ← heuristic QA checks
├── character_loader.py          ← YAML loader
├── vivify/                      ← Python CLI package (point of entry now)
│   ├── cli.py                   ← `vivify` Click group
│   ├── providers/               ← AI vendor adapters (ark / minimax / stubs)
│   ├── asset_orchestrator.py    ← router → cost-cap → retry → ledger pipeline
│   ├── asset_router.py          ← YAML provider-selection router
│   ├── asset_ledger.py          ← JSONL cost + status ledger
│   ├── commands/                ← CLI subcommand modules (one per group)
│   └── ...                      ← db, migrator, pricing, retry, cost_cap
├── memory/                      ← CROSS-IP knowledge (general, reusable)
│   ├── prompt-engineering/      ← Seedance 2.0 formula, etc.
│   └── model-capabilities/      ← Seedream / Seedance / TTS capabilities
├── prompt_library/              ← reusable prompt fragments (general)
├── validators/                  ← lint scripts (general)
├── tests/                       ← regression tests (general)
├── characters/                  ← per-IP data layer (EACH IP HAS ITS OWN)
│   ├── _template/               ← empty starter template
│   └── fengge/                  ← example IP: peak哥 panda
│       ├── character.yaml        ← IP config
│       ├── canonical/            ← 3 reference images
│       ├── README.md             ← IP data layout
│       ├── lessons.md            ← IP-specific lessons
│       ├── gotchas.md            ← IP-specific pitfalls
│       ├── overrides/            ← per-shot/per-episode overrides (JSON)
│       ├── experiments/          ← A/B test data
│       ├── analytics/            ← performance data
│       └── examples/             ← rendered episodes
└── examples/                    ← cross-IP examples
```

## Architecture principle: skills vs data

**Skills** (general, reusable, stateless): `CLAUDE.md`, `skills/`, `prompt_library/`, `validators/`, `memory/cross-ip/`.
**Data** (per-IP, mutable, stateful): `characters/<ip>/`.
**Never put IP-specific data into `memory/` or skills** — keep the framework general so other IPs can opt in.

## Quick start

```bash
# Render an episode with fengge (legacy single-shot path — still works)
python3 render_episode.py --character-dir characters/fengge \
  --storyboard examples/panda-episode-004/STORYBOARD.md \
  --script examples/panda-episode-004/SCRIPT-douyin.md \
  --voice 治愈 --platform 抖音 --out /tmp/out

# Or via the new `vivify` CLI (preferred — backed by the orchestrator + DB)
./scripts/vivify episode render fengge EP005 \
  --storyboard characters/fengge/examples/panda-episode-005/STORYBOARD.md \
  --script     characters/fengge/examples/panda-episode-005/SCRIPT-douyin.md \
  --voice 治愈 --platform 抖音 --target-dur 58 --parallel 4

# Single-asset generation via the new L1 orchestrator
./scripts/vivify asset generate \
  --scene '{"scene_id":"smoke","type":"image","prompt":"red dot"}' \
  --type image --dry-run           # see which provider would be picked
./scripts/vivify asset generate \
  --scene '{"scene_id":"smoke","type":"image","prompt":"red dot","options":{"size":"1024x1024","out_path":"/tmp/x.jpg"}}' \
  --type image                     # real call → ledger row written

# Read the cost + status ledger
./scripts/vivify asset ledger --last 20
./scripts/vivify asset ledger --filter status=budget-exceeded --as-json

# Inspect / dry-run the provider-selection router
./scripts/vivify asset router --show-config
./scripts/vivify asset router --pick --type video --duration-sec 5
```

## CLI flags (must know)

- `--character-dir` — path to character directory (REQUIRED for new IP)
- `--voice` — 治愈/御宅/哲学/国潮
- `--platform` — 抖音/哔哩哔哩/小红书
- `--quality-tier` — draft/standard/premium (auto-routes model)
- `--require-lip-sync` — forces premium tier + lip sync
- `--target-dur` — total duration (must include 1.5+1.5 for title/end cards)
- `--reference-image` — override default canonical
- `--title` — opening card text
- `--next-episode` — closing card text
- `--qa-skip` — skip QA gate (use only if intentional)

## When user gives vague instructions

> User: "帮我做个熊猫视频"
1. Ask: what tone? what story? what platform?
2. Once answered, read relevant files in this order:
   - `characters/fengge/character.yaml` (example)
   - `characters/fengge/lessons.md` (what worked for this IP)
   - `memory/prompt-engineering/` (cross-IP lessons)
   - `prompt_library/styles/INDEX.md` (style options)
3. Either generate new content or scaffold a new character.

## When something breaks

1. Check `tests/` for regression
2. Run validators
3. If still broken, write the lesson to the appropriate location:
   - **Cross-IP lesson** (affects multiple characters) → `memory/<topic>/lessons.md`
   - **IP-specific lesson** (only affects current IP) → `characters/<ip>/lessons.md`
