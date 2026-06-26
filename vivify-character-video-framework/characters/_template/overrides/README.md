# Overrides — per-shot / per-episode config overrides

> Override files in this directory are **deep-merged** on top of `character.yaml` when
> rendering a specific episode or shot. They let you tweak a single render without
> mutating the IP bible.

## Naming convention

- `<episode_id>.<aspect>.json` — e.g. `EP007.bgm.json`, `EP007.style.json`
- `<episode_id>.shot-<n>.<aspect>.json` — per-shot override (rare)

Examples:
- `EP001.bgm.json` — replace the BGM track for EP001 only
- `EP002.style.json` — switch to a different style anchor for one episode
- `EP003.shot-04.outfit.json` — force a different outfit on shot 4 only

## Schema (deep-merge)

The override file is a flat JSON object. Any key present is **replaced** in
`character.yaml` for that render. Nested objects are **deep-merged**, not replaced
wholesale. Use this to override one outfit without losing the rest.

### Example 1: BGM swap

```json
{
  "track": "epic_cinematic_03",
  "params": {"speed": 0.85, "loop": true}
}
```

This would replace the `track` and `params` keys wherever they live in the
resolved config. (The exact key path depends on the asset type; check
`validators/lint_constraints.py` for the current shape.)

### Example 2: per-episode style anchor

```json
{
  "character": {
    "style_anchor": "cinematic photorealistic, 35mm film grain, dramatic lighting"
  }
}
```

This deep-merges: only `style_anchor` is replaced, all other `character.*` keys
keep their defaults from `character.yaml`.

### Example 3: per-shot outfit

```json
{
  "shot": "shot-04",
  "outfit": "outfit_jacket_redwhite"
}
```

## Resolution order (highest priority last)

1. `character.yaml` — the IP bible
2. `characters/{{name}}/overrides/*.json` — sorted by filename (later files win)
3. `--flag` CLI options — explicit override always wins
4. Pipeline defaults

## Validation

- The orchestrator logs which override file was applied to each asset
- Run `python3 validators/lint_constraints.py characters/{{name}}` to ensure
  your override doesn't introduce a constraint violation
- Overrides SHOULD NOT weaken `anti_patterns` (e.g. don't override a strong
  anti_patterns with a weaker one — the IP bible is the safety net)

## What NOT to do

- Don't put IP-level config in here (use `character.yaml` instead)
- Don't create override files for every shot — that defeats the purpose
- Don't override `canonical.primary` (use `--reference-image` CLI flag instead)
- Don't commit secrets to override files (use a local-only file + .gitignore)
