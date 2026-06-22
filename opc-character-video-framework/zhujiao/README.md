# 铸角 / ZhuJiao — Character Video Engineering CLI

> 铸角 (zhù jiǎo, "casting characters") is the engineering platform CLI
> for the OPC character video framework. It replaces loose scripts +
> markdown files with a proper CLI backed by a SQLite state database.

## Quick start

```bash
# Register the showcase character (peak哥)
./scripts/zhujiao character add fengge characters/fengge

# Browse
./scripts/zhujiao character list
./scripts/zhujiao character show fengge

# Validate
./scripts/zhujiao character validate fengge

# Asset library
./scripts/zhujiao asset register-canonical fengge
./scripts/zhujiao asset list --character fengge --type image

# Lessons (replaces memory/ + characters/<ip>/lessons.md)
./scripts/zhujiao lesson add \
  --title "3D drift fix" \
  --body "Add NEVER 3D NEVER PIXAR to style_anchor..." \
  --character fengge --status validated --source EP003
./scripts/zhujiao lesson list
./scripts/zhujiao lesson search "3D"
```

## Why this exists

The previous setup — skills + memory/ + characters/<ip>/lessons.md +
scattered /tmp renders — worked for demo, but couldn't scale. There was:

- ❌ No way to query "what did I render last week?"
- ❌ No way to compare two versions of an episode
- ❌ No asset library (assets only existed as files)
- ❌ No state tracking (every render was isolated)
- ❌ Skills had IP data mixed in (architecture anti-pattern)

铸角 fixes all of these with a CLI + SQLite.

## Architecture

```
zhujiao/
├── __init__.py
├── __main__.py              # python -m zhujiao
├── cli.py                   # Click entry point
├── db.py                    # SQLite schema + connection
└── commands/
    ├── character.py         # IP character management
    ├── lesson.py            # lessons registry (replaces memory/ + md files)
    └── asset.py             # asset library (replaces scattered files)

data/
└── zhujiao.db               # SQLite state (auto-created)
```

## Commands

```
zhujiao character
├── list                    List all registered IPs
├── add <id> <dir>          Register IP from directory
├── show <id>               Show IP details + counts
├── validate <id>           Run all validators
└── refresh <id>            Re-read character.yaml

zhujiao asset
├── list [--type image|video|audio]
├── show <id>
├── register <path> --type <t>
└── register-canonical <character-id>

zhujiao lesson
├── list [--character X]
├── show <id>
├── add --title --body [--character X]
└── search <query>
```

## What it replaces

| Old | New |
|---|---|
| `characters/<ip>/character.yaml` | `characters/<ip>/character.yaml` (unchanged) + DB row |
| `characters/<ip>/lessons.md` | `lessons` table rows (per IP) |
| `characters/<ip>/gotchas.md` | `lessons` table rows (category=ip-specific) |
| `memory/prompt-engineering/*.md` | `lessons` table rows (character_id=NULL = cross-IP) |
| `/tmp/opc-render/work/` (rendition intermediates) | DB records on each `episode.render_*` |
| `docs/showcase/<episode>/` (final outputs) | DB record + filesystem |
| git commit messages ("Render EP004 with X model") | DB row: `model_used`, `cost_yuan`, etc. |
| `validators/*.py` (separate scripts) | `zhujiao character validate` wrapper |
| `make_episode.sh` | future: `zhujiao episode scaffold` |

## Coming soon (not yet implemented)

- `zhujiao episode` group (list, show, shots, diff, status, render)
- `zhujiao publish` (post to 抖音/小红书, fetch analytics)
- `zhujiao workflow` (job queue, retry, parallel)
- `zhujiao compare <ep-a> <ep-b>` (visual diff between 2 versions)
- HTTP API (FastAPI) for external integration
- Web UI

## Database schema

```sql
-- One row per IP character
characters (id, name, english_name, species, dir_path, ...)

-- One row per episode render
episodes (id, character_id, episode_id, season, tone, platform,
          status, cost_yuan, model_used, duration_sec, ...)

-- One row per shot per episode
shots (id, episode_id, shot_number, outfit_id, scene_id,
       image_path, video_path, kling_prompt, voiceover_text,
       qa_passed, cost_yuan, ...)

-- Asset library (image, video, audio) with metadata + tags
assets (id, asset_type, character_id, asset_path, width, height,
        duration_sec, tags, used_in_episodes, ...)

-- Publish tracking (view/like/comment counts per platform)
publishes (id, episode_id, platform, view_count, like_count, ...)

-- Render job queue (for retry + parallel rendering)
render_jobs (id, episode_id, status, retries, ...)

-- Lessons (cross-IP + per-IP) with status + full-text search
lessons (id, character_id, title, body, category, status, ...)
```

## Migration guide

If you have existing markdown lessons in `characters/<ip>/lessons.md`
or `memory/prompt-engineering/`, you can migrate them:

```bash
# Manual one-by-one
./scripts/zhujiao lesson add \
  --title "$(head -1 /path/to/lesson.md)" \
  --body "$(cat /path/to/lesson.md | tail -n +3)" \
  --character fengge --status validated

# (Future: bulk migration script)
```

## Future: web UI + HTTP API

The CLI is the foundation. A thin FastAPI layer exposing the same
operations via HTTP, plus a Next.js web UI, would make this a real
platform that non-engineers can use. Not built yet — say when needed.

## License

MIT (same as parent framework).