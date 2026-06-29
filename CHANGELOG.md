# Changelog

All notable changes to this project are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com).

## [Unreleased]

### Added
- (planned)

## [0.2.0] - 2026-06-25 — Phase A: in-process driver pipeline

### Added
- `vivify episode render --use-driver` flag for in-process per-shot asset
  pipeline (replaces subprocess call to render_episode.py)
- `vivify/episode_driver.py` — per-shot image → video → TTS → DB update
- `vivify/asset_orchestrator.generate_asset()` forwards `character_ref`
  kwarg to adapters (was silently dropped before)
- TTS via `mmx speech synthesize` CLI (self-authenticated, no
  MINIMAX_API_KEY env needed); legacy `render_episode.gen_tts` fallback
- Monthly cost-cap in orchestrator (was per-asset only)
- `vivify-providers.yaml` `default_models:` block (resolves provider_id
  "ark" → "doubao-seedance-1-0-pro-fast-251015" so cost gate returns
  real ¥ instead of 0)

### Changed
- `render_episode.py` marked DEPRECATED (escape hatch only — new work in
  `vivify/episode_driver.py`)
- `asset_router._default_model_for()` uses hardcoded provider_id → catalog
  prefix map (was substring match returning 0)

### Fixed
- Cost gate was silently returning ¥0 (model catalog keyed by model name,
  not provider_id — substring match failed for "ark"/"minimax")
- Driver accepted only `shot["n"]`; now also accepts `shot["shot_number"]`
  (DB-shaped dicts from commands/episode._get_shots)

## [0.1.0] - 2026-06-08 — G3 L1 backing

### Added
- Provider adapters (火山方舟 ark + 海螺 minimax via mmx + 4 stubs:
  kling, jimeng, suno, udio)
- YAML provider router (vivify-providers.yaml, auto-created on first use)
- Asset orchestrator pipeline (router → cost-cap → retry → ledger)
- JSONL append-only ledger with FNV-1a 64-bit hash (format `fnv64:` + 16 hex)
- `vivify asset generate / ledger / router` CLI commands
- 95 unit tests (orchestrator, router, ledger, providers, db autoinit)
- `vivify/migrator.py` + `vivify db` command group with migrations table,
  `sql_hash` drift detection, forward-only transactions
- Auto-apply pending migrations on every CLI invocation (fresh install
  creates DB + applies all migrations + reports schema version)
- 12 vivify-* SKILL.md files packaged into `skills/` (mirrors
  `~/.claude/skills/vivify-*`)
- Top-level docs: `README.md` (user-facing) and `SKILL_ARCHITECTURE.md`
  (L4 orchestrate / L3 scenarios / L1 CLI layering)
- Cost-cap command group (`vivify cost status / history / estimate`)
  with per-video soft/hard and monthly hard caps; episode render
  pre-checks caps before spawning `render_episode.py`
- Workflow command group (`vivify workflow config / list / status /
  retry / retry-failed`) with parallel render + smart retry classifier
  (permanent vs transient failures, exp backoff 5s/15s/45s/135s)
- Memory migration (`vivify memory migrate`) bulk-imports cross-IP
  `.md` files into the `lessons` table; idempotent on re-run
- Publish + analytics stub (`vivify publish publish / list / show /
  analytics / refresh`) for 抖音 / 小红书 / B站
- MiniMax 海螺 Hailuo as first-class video provider (no silent fallback);
  `--video-provider {auto,ark,minimax}` and `--video-model` flags
- `render_episode.py --parallel / --max-retries / --retry-only` flags
  for concurrent shot rendering

### Changed
- Project root renamed `opc-character-video-framework` →
  `vivify-character-video-framework`
- All `opc-*` skills renamed to `vivify-*` (12 SKILL.md files + cross-refs)
- Env var `OPC_RENDER_DIR` → `VIVIFY_RENDER_DIR`; default path
  `/tmp/opc-render` → `/tmp/vivify-render`
- DB schema: added `updated_at` column to `episodes` (migration 002)
- `model_router.route_video_model()` now takes a provider filter and
  routes within that provider's models with tier-aware fallback
- `mmx` CLI default timeout raised to 900s for back-to-back renders
- Renamed `铸角 / zhujiao` → `点睛 / Vivify` (package, shell entry,
  DB path, all command files, README)

### Fixed
- `render_episode.py` `sys.exit()` inside `with connect():` skipped
  `conn.commit()`; failed-status UPDATE never reached disk. Explicit
  commit before exit.
- `_ffprobe_duration()` only checked PATH; ffmpeg-installer bundles
  ffprobe at `/root/.openclaw/.../linux-x64/`. Same candidate-path
  search as ffmpeg detection.
- Shot image/video path detection used `.tmp/renders/work/` instead of
  `$OPC_RENDER_DIR/work/`; now reads env.
- `workflow list` and `publish list` FK-vs-string column collisions
  (render_jobs.episode_id / publishes.episode_id is FK int vs
  episodes.episode_id user-visible string) — aliased to
  `fk_episode_id` / `ep_code`.

## [0.0.1] - 2026-05-15 — Initial schema

### Added
- SQLite schema: 8 tables (characters, episodes, shots, assets,
  publishes, render_jobs, lessons, + indexes)
- `vivify character / lesson / asset` CLI commands (register, list,
  show, validate, register-canonical)
- fengge character registration (showcase IP — 峰哥 / Fengge, adult
  panda, 5 outfit variants)
- 6 production panda episodes validated through render_episode.py
  (EP#004 through EP#006 covering 治愈 / 国潮 / 立秋 / 立夏)
- Knowledge persistence architecture: `memory/` (cross-IP), per-IP
  `characters/<ip>/` data layer, `prompt_library/` fragments,
  `validators/` lint scripts
- CLAUDE.md agent instruction manual (13-step new-IP workflow,
  prompt formula reference, quality issue triage, strict forbiddens)
- Validator suite: `lint_character.py`, `lint_prompt.py`,
  `lint_constraints.py`, `canonical_image_check.py`
- Smart auto-route model selection with 3 quality tiers (draft /
  standard / premium), tier-aware fallback chain, special handling
  for `--require-lip-sync` and `audio_url` capability
- Per-tone TTS voice + locked visual style + variable pacing
- Per-shot BGM + QA gate + 4-tone matrix validation
- Content craft layer (title + subtitle + end card)