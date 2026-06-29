# Experiments — A/B test data

> Store A/B test data here. Each experiment file describes one controlled
> comparison — typically two variants of a hook, opening shot, BGM, or style
> — that the team wants to measure against each other.

## Naming convention

`<episode_id>-<aspect>-ab.json` — e.g. `EP001-hook-ab.json`, `EP003-bgm-ab.json`

Examples:
- `EP001-hook-ab.json` — compare 2 opening hooks on EP001
- `EP002-style-ab.json` — compare 2D vs illustrated style on EP002
- `EP004-bgm-ab.json` — compare 2 BGM tracks for engagement

## Schema

```json
{
  "experiment_id": "EP001-hook",
  "created_at": "2026-06-26T00:00:00Z",
  "hypothesis": "A 3-question hook will outperform a quote hook on {{name}}",
  "primary_metric": "completion_rate",
  "variants": {
    "variant_a": {
      "label": "三问开场",
      "hook_formula": "三问开场",
      "hook_text": "立秋了吗?凉了吗?你睡了吗?",
      "rendered_episode": "characters/{{name}}/examples/EP001-a"
    },
    "variant_b": {
      "label": "引用钩子",
      "hook_formula": "引用钩子",
      "hook_text": "古人云,秋风起兮",
      "rendered_episode": "characters/{{name}}/examples/EP001-b"
    }
  },
  "results": {
    "variant_a": {
      "views": 0,
      "completion_rate": 0.0,
      "engagement_rate": 0.0,
      "winner": null
    },
    "variant_b": {
      "views": 0,
      "completion_rate": 0.0,
      "engagement_rate": 0.0,
      "winner": null
    }
  },
  "conclusion": "t.b.w. — fill in after data collection"
}
```

## Workflow

1. **Define** the hypothesis (what are you testing, why?)
2. **Create** this file with the variants and their rendered_episode paths
3. **Render** both variants (use `render_episode.py` or `vivify episode render`)
4. **Publish** both variants on the same platform, same time window
5. **Collect** metrics into the `results` block (from `analytics/`)
6. **Conclude** with a winner (or "no significant difference")
7. **Promote** the winning pattern into `character.yaml` so future episodes inherit it

## What to test

- Opening hook (3-question vs quote vs story vs ...)
- Style (2D vs illustrated vs 3D-rendered — but 3D usually loses for our IPs)
- BGM (lo-fi vs ambient vs silence)
- Outfit (哪个 outfit 在首集更能立住 IP)
- Voice tone (default tone vs alternative for an episode)

## See also

- `analytics/README.md` — where the metric data comes from
- `overrides/README.md` — if you need to override config to run a variant
- `../lessons.md` — write a lesson when an experiment produces a validated pattern
