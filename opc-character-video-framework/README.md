# 点睛 / Vivify — Character Video Engineering Platform

> **画龙点睛，把静态 IP 角色点活成短剧。**
> *Vivify* — Latin "to make alive": the international form of the same idea.

An engineering platform for producing AI character short-drama videos.
**点睛 (Diǎn Jīng)** — "dot the eyes of the dragon" — is the final touch that brings
a static painting to life. The framework does the same for IP characters:
takes a static character design and animates it into a living short video.

The showcase IP is **峰哥 / Fengge** (a 治愈系 panda). The framework is
**character-agnostic** — drop in 3 reference images + a profile, get a
抖音-ready MP4, audit-trail in SQLite.

---

## What you get

```
characters/fengge/canonical/*.jpg     ──┐
characters/fengge/character.yaml       ──┤
                                       ├──►  vivify episode render ──►  MP4
examples/.../STORYBOARD.md             ──┤                          │
examples/.../SCRIPT-douyin.md          ──┘                          ▼
                                                         .tmp/data/vivify.db
                                                         (audit trail)
```

For any character:
- 4-tone voice matrix (治愈 / 御宅 / 哲学 / 国潮, configurable)
- 9:16 vertical MP4 with title card, subtitles, end-card CTA
- Cross-shot identity consistency (`mmx video generate --subject-image`
  for character lock, or Seedream `reference_image` for the lighter path)
- Per-tone TTS voice (`mmx speech synthesize` with emotion)
- Cost tracking per shot + per episode, with hard monthly cap
- Auto-retry on transient failures, smart-skip on permanent ones (quota / 403)

---

## Quick start

### 1. Install

```bash
# System deps
pip install pyyaml click

# API keys (already in ~/.bashrc on dev machines)
export ARK_API_KEY="..."          # 火山方舟 (Seedream image gen)
export MINIMAX_API_KEY="..."      # 海螺 TTS + mmx CLI video gen
```

### 2. Bring up the DB

```bash
./scripts/vivify db migrate         # apply pending migrations (idempotent)
./scripts/vivify db inspect         # see table list + row counts
```

### 3. Register a character

```bash
./scripts/vivify character add fengge characters/fengge
./scripts/vivify character list
./scripts/vivify character validate fengge
```

### 4. Render an episode

```bash
./scripts/vivify episode render fengge EP005 \
  --storyboard characters/fengge/examples/panda-episode-005/STORYBOARD.md \
  --script     characters/fengge/examples/panda-episode-005/SCRIPT-douyin.md \
  --voice 治愈 --platform 抖音 --target-dur 58 \
  --video-provider minimax --video-model MiniMax-Hailuo-2.3 \
  --parallel 4
```

Output: `.tmp/renders/fengge-EP005.mp4` plus a full audit trail in the DB.

### 5. Publish + track

```bash
./scripts/vivify publish publish fengge EP005 --platform 抖音 --title "..."
./scripts/vivify publish refresh <id>             # updates view/like/comment
./scripts/vivify publish analytics fengge EP005    # aggregated across platforms
```

> **Note**: publish is a stub today (no real 抖音 API key). See
> [`vivify/commands/publish.py`](vivify/commands/publish.py) — swap
> `_upload_to_platform()` with real API calls when credentials are
> available.

---

## CLI commands

```
vivify
├── character   Register/list/show/validate/refresh IP characters
├── lesson      Manage lessons (cross-IP + per-IP)
├── asset       Manage asset library (canonical images, generated stills)
├── episode     Render lifecycle: add / render / show / shots / status / delete
├── cost        Cost tracking + per-video ¥50/¥100 + monthly ¥60k cap
├── memory      Migrate memory/*.md to DB, list/show cross-IP lessons
├── publish     Publish to 抖音/小红书/B站 (stub), track analytics
├── workflow    Parallel rendering + smart retry (classifier)
└── db          Migrations: status / migrate / inspect / schema-version
```

Full reference: each command has `--help`. Example:

```bash
./scripts/vivify episode render --help
```

---

## Architecture

```
vivify/                          Click-based CLI (this package)
├── __init__.py                  Branding + version
├── cli.py                       Main entry — registers all subcommands
├── db.py                        SQLite schema (source of truth for fresh DBs)
├── migrator.py                  Lightweight migration runner
├── migrations/                  Numbered SQL files (001, 002, ...)
├── pricing.py                   Cost estimator (¥/sec by model)
├── retry.py                     Error classifier (permanent vs transient)
├── cost_cap.py                  Per-video + monthly hard caps
└── commands/
    ├── character.py             IP registry
    ├── lesson.py                Knowledge base
    ├── asset.py                 Asset library
    ├── episode.py               Render lifecycle
    ├── cost.py                  Cost status / history / estimate
    ├── memory.py                Migrate memory/*.md → DB
    ├── publish.py               Publish + analytics (stub)
    ├── workflow.py              Parallel + retry surface
    └── db.py                    Migrations CLI

scripts/vivify                   Shell entry (no `pip install` needed)

characters/<ip>/                 Per-IP data layer (mutable)
├── character.yaml              IP profile
├── canonical/                  3 reference jpgs (must include close-up face)
├── lessons.md                  IP-specific lessons
├── gotchas.md                  IP-specific pitfalls
├── overrides/                  Per-shot/per-episode overrides
├── experiments/                A/B test data
├── analytics/                  Performance data
└── examples/                   Rendered episode dirs

memory/                          Cross-IP knowledge (legacy, migrated to DB)
├── prompt-engineering/         Seedance/Hailuo prompt patterns
└── model-capabilities/         Seedream/Seedance/MiniMax gotchas

render_episode.py                Standalone rendering pipeline (also called by CLI)
model_router.py                  Model routing + provider filter
character_loader.py              YAML loader
qa_gate.py                       Heuristic image QA
```

### Skills vs Data

**Skills** (general, reusable, stateless): `CLAUDE.md`, `~/.claude/skills/*`,
`prompt_library/`, `validators/`, `memory/`, `examples/`.

**Data** (per-IP, mutable, stateful): `characters/<ip>/`.

**Never** put IP-specific knowledge into skills — keep the framework
general so other IPs can opt in.

### DB schema

8 tables (see `vivify/db.py`):

| Table | Purpose |
|---|---|
| `characters` | IP registry (one row per IP) |
| `episodes` | Render jobs (cost, duration, status, output path) |
| `shots` | Per-storyboard-shot rows (image/video path, QA, cost) |
| `assets` | Asset library (images, videos, audio) with metadata |
| `publishes` | Per-platform publish records + analytics |
| `render_jobs` | Job queue rows (start/end, retries, error) |
| `lessons` | Cross-IP + per-IP knowledge (replaces scattered md files) |
| `migrations` | Migration tracking (added in 002) |

---

## Per-IP data layer (adding a new character)

See `CLAUDE.md` for the full workflow. TL;DR:

1. Read `memory/INDEX.md` for cross-IP lessons
2. Read `prompt_library/INDEX.md` for prompt templates
3. Use `characters/fengge/` as a worked example
4. Generate `character.yaml` using `prompt_library/characters/format.md`
5. Generate 3 canonical reference jpgs (close-up face + full-body × 3 outfits)
   — **close-up face shot must be SEPARATE from full-body** (avoids ID drift)
6. Register: `vivify character add <id> characters/<id>`

---

## Multi-provider video gen (user picks, no fallback)

```bash
# ark (Seedance) — original
./scripts/vivify episode render fengge EP005 \
  --video-provider ark --video-model doubao-seedance-1-5-pro-251215

# minimax (Hailuo via mmx CLI) — character-locked S2V mode
./scripts/vivify episode render fengge EP005 \
  --video-provider minimax --video-model MiniMax-S2V-01

# auto (router decides by tier — DEFAULT)
./scripts/vivify episode render fengge EP005 --quality-tier standard
```

The provider filter is explicit and visible in the command. No silent
fallback chains. Cost cap enforces total spend even if user picks
expensive provider.

---

## Cost cap

| Cap | Value | Override |
|---|---|---|
| Per-video soft warn | ¥50 | `--per-video-cap` |
| Per-video hard block | ¥100 | `--force` |
| Monthly hard block | ¥60,000 | `--force` |

```bash
$ vivify cost status
  spent:    ¥0.00 / ¥60000.00
  headroom: ¥60000.00

$ vivify cost estimate --model MiniMax-S2V-01 --shots 50 --video-sec 300
  total: ¥370.50
  ⚠ ABOVE per_video_hard (¥100) — episode render will be blocked without --force
```

Source of truth: `SUM(cost_yuan) FROM episodes WHERE render_completed_at LIKE 'YYYY-MM%'`.

---

## Parallel + smart retry

```bash
# Render with 4 shots in parallel + 3 retries each
./scripts/vivify episode render fengge EP005 --parallel 4 --max-retries 3

# After a partial failure, requeue and re-run only failed shots
./scripts/vivify workflow status
./scripts/vivify workflow retry-failed fengge EP005 -y
./scripts/vivify episode render fengge EP005 --retry-only
```

The retry classifier (`vivify/retry.py`) distinguishes:

- **Permanent** failures (don't retry): quota_exceeded, HTTP 403,
  HTTP 401, HTTP 400, content_policy
- **Transient** failures (retry with backoff 5s → 15s → 45s → 135s):
  timeout, 5xx, 429, network (DNS / reset / refused)
- **Unknown**: retry once

---

## DB migrations

Schema changes don't require manual `ALTER TABLE` — write a numbered
SQL file in `vivify/migrations/`:

```bash
# 1. Add the SQL file
$EDITOR vivify/migrations/003_add_new_column.sql

# 2. Apply (idempotent — only pending migrations are applied)
./scripts/vivify db migrate

# 3. Verify
./scripts/vivify db status
./scripts/vivify db schema-version
```

Convention: `NNN_short_description.sql`. Each migration runs in a
single transaction. Forward-only — to undo, write a corrective migration.

---

## What's NOT here yet

Honest gaps:

- ⚠️ **Real visual QA** — `qa_gate.py` is heuristic (file size,
  aspect ratio, color stats). No vision-model check for "is the
  panda's face the same across shots?"
- ⚠️ **Real publish** — `vivify publish publish` writes a stub URL.
  Need 抖音开放平台 credentials to swap `_upload_to_platform()`.
- ⚠️ **No team/remote DB** — SQLite is local. For team use, swap to
  PostgreSQL (the schema is mostly portable).
- ⚠️ **No HTTP API / Web UI** — CLI only. A FastAPI layer exposing
  the same operations would make this a real platform.
- ⚠️ **No auth** — anyone with shell access owns the DB.

---

## Environment

Required:
- `ARK_API_KEY` (火山方舟) — image gen
- `MINIMAX_API_KEY` (海螺) — TTS, optionally video gen via `mmx`
- `FFMPEG` — auto-detected via `/root/.openclaw/.../ffmpeg-installer/`

Optional:
- `OPC_RENDER_DIR` — where intermediate frames go (default `/tmp/vivify-render`)
- `FFPROBE` — for actual duration detection (auto-detected too)

---

## License

MIT (same as parent framework).

---

## See also

- `CLAUDE.md` — agent-facing instructions (add IP, fix quality, etc.)
- `vivify/README.md` — detailed vivify CLI architecture
- `~/.claude/skills/mmx-video-gen/SKILL.md` — mmx CLI wrapper patterns
- `~/.claude/skills/vivify-cost-cap/SKILL.md` — cost cap rules
- `memory/prompt-engineering/seedance-formula.md` — Seedance 2.0 prompt formula
- `memory/prompt-engineering/id-drift-prevention.md` — character consistency
