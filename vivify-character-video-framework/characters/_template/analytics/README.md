# Analytics — performance data

> Per-episode performance data. Fill these files in after publishing each
> episode to your target platform (抖音 / 哔哩哔哩 / 小红书). Use this data
> to drive A/B test conclusions and feed back into `character.yaml` tuning.

## Naming convention

`<episode_id>.<platform>.json` — e.g. `EP001.douyin.json`, `EP002.bilibili.json`

Examples:
- `EP001.douyin.json` — EP001's 抖音 performance
- `EP002.bilibili.json` — EP002's 哔哩哔哩 performance
- `EP003.xiaohongshu.json` — EP003's 小红书 performance

## Schema

```json
{
  "episode_id": "EP001",
  "platform": "douyin",
  "published_at": "2026-06-26T00:00:00Z",
  "fetched_at": "2026-06-27T00:00:00Z",
  "metrics": {
    "views": 12345,
    "likes": 678,
    "comments": 42,
    "shares": 17,
    "saves": 23,
    "completion_rate": 0.42,
    "engagement_rate": 0.061,
    "avg_watch_seconds": 18.3,
    "click_through_rate": 0.085
  },
  "demographics": {
    "age_buckets": {
      "18-24": 0.30,
      "25-34": 0.45,
      "35-44": 0.20,
      "45+": 0.05
    },
    "gender_split": {
      "female": 0.62,
      "male": 0.36,
      "other": 0.02
    }
  },
  "experiment_ref": "experiments/EP001-hook-ab.json",
  "notes": "First-episode baseline. 3-question hook variant (variant_a)."
}
```

## Field definitions

| Field | Unit | Notes |
|---|---|---|
| `views` | count | total plays |
| `likes` | count | total likes |
| `comments` | count | total comments |
| `shares` | count | total shares/forwards |
| `saves` | count | total saves (especially relevant on 小红书) |
| `completion_rate` | ratio (0–1) | fraction who watched to end |
| `engagement_rate` | ratio (0–1) | (likes + comments + shares + saves) / views |
| `avg_watch_seconds` | seconds | mean watch time per viewer |
| `click_through_rate` | ratio (0–1) | fraction who clicked through to next episode |
| `age_buckets` | ratios (sum=1.0) | viewer age distribution |
| `gender_split` | ratios (sum=1.0) | viewer gender distribution |

## Workflow

1. **Publish** the episode to the platform
2. **Wait** at least 48h for the algorithm to settle
3. **Fetch** metrics from the platform's analytics dashboard
4. **Write** a JSON file in this directory
5. **Cross-link** to any experiments via `experiment_ref`
6. **Update** `experiments/*.json` with the result in the `results` block
7. **Write** a lesson in `lessons.md` if a clear pattern emerged

## What to look for

- **Completion rate < 0.3** → opening hook isn't holding attention; review hook formula
- **Completion rate > 0.6** → opening + middle + ending all working; template this pattern
- **Engagement rate < 0.02** → content is being watched but not resonating; review tone
- **CTR < 0.05** → ending card isn't compelling; review `--next-episode` copy
- **Demographic skew** → if 18-24 dominates, tone is hitting young audience — verify IP bible aligns

## Privacy

- Do NOT include PII (user IDs, usernames, comments text) in these files
- Aggregate only (counts, ratios, distributions)
- If you need to comment on a specific comment, anonymize first
