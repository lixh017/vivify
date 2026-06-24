---
name: vivify-cost-cap
description: Use when spending money on opc AI video asset generation (可灵, 即梦, Suno, TTS, Claude API) and a hard per-video or monthly budget cap must be enforced before the batch runs.
---

# vivify-cost-cap

Phase 1 budget is ¥4-7 万 / month. One failed batch burns the cap. This skill gates every asset-generation call against a per-video and monthly limit, and routes to the cheapest model that can do the job.

## Cost Model (per asset, Phase 1)

| Asset | Unit | Cost (¥) |
|------|------|---------:|
| 可灵 std | per second of video | 0.50 |
| 可灵 pro | per second of video | 2.00 |
| 即梦 静帧 | per image | 0.30 |
| Suno BGM | per track (10s-2min) | 0.20 |
| TTS | per 100 字 | 0.05 |
| Claude API (script gen) | per script | 0.01-0.05 |

## Caps (Phase 1)

- **Default per-video cap**: ¥50
- **Hard per-video cap**: ¥100
- **Monthly hard cap**: ¥60,000 (Phase 1 budget is ¥40k-70k; abort at ¥60k to leave headroom)

## Model Routing (cheapest viable)

- **选题生成** → `haiku` (high volume, low stakes)
- **脚本生成** → `sonnet` (quality matters, volume moderate)
- **反 AI 检测 / 拟人化** → `opus` (only when triggered by `opc_humanize_script` MCP tool, never on the hot path)

## Cost Tracker Schema

`/tmp/vivify-cost-tracker.json` is the single source of truth:

```json
{
  "month": "2026-06",
  "monthly_total_yuan": 12450.0,
  "monthly_cap_yuan": 60000,
  "videos": [
    {
      "id": "vid_2026-06-08_001",
      "title": "为什么我们越来越忙",
      "platform": "抖音",
      "model": "sonnet",
      "totals": { "video_yuan": 32.0, "image_yuan": 0.9, "bgm_yuan": 0.2, "tts_yuan": 0.1, "script_yuan": 0.04 },
      "total_yuan": 33.24,
      "cap_yuan": 50,
      "aborted": false
    }
  ]
}
```

## Pre-Flight Check (Node)

Run before every asset batch. If it throws, abort the batch — do not partially execute.

```js
// scripts/check_cost.mjs
import fs from 'node:fs';
const TRACKER = '/tmp/vivify-cost-tracker.json';
const PER_VIDEO_CAP = Number(process.env.OPC_VIDEO_CAP ?? 50);
const MONTHLY_CAP = Number(process.env.OPC_MONTHLY_CAP ?? 60000);

function loadTracker() {
  try { return JSON.parse(fs.readFileSync(TRACKER, 'utf8')); }
  catch { return { month: new Date().toISOString().slice(0,7), monthly_total_yuan: 0, monthly_cap_yuan: MONTHLY_CAP, videos: [] }; }
}

function quote(plan) {
  // plan = [{ kind: 'video_std'|'video_pro'|'image'|'bgm'|'tts'|'script', qty: number }]
  const PRICE = { video_std: 0.5, video_pro: 2.0, image: 0.3, bgm: 0.2, tts: 0.05, script: 0.03 };
  return plan.reduce((sum, p) => sum + (PRICE[p.kind] ?? 0) * p.qty, 0);
}

export function checkCost(estimatedYuan, videoId) {
  const t = loadTracker();
  if (t.monthly_total_yuan + estimatedYuan > t.monthly_cap_yuan) {
    throw new Error(`MONTHLY CAP BREACH: would push total to ¥${t.monthly_total_yuan + estimatedYuan} > cap ¥${t.monthly_cap_yuan}. Aborting month.`);
  }
  if (estimatedYuan > PER_VIDEO_CAP) {
    throw new Error(`PER-VIDEO CAP BREACH: video ${videoId} would cost ¥${estimatedYuan} > cap ¥${PER_VIDEO_CAP}. Aborting batch.`);
  }
  if (estimatedYuan > 100) {
    throw new Error(`HARD CAP BREACH: video ${videoId} would cost ¥${estimatedYuan} > hard cap ¥100. Manual review required.`);
  }
  return { ok: true, monthly_total_yuan: t.monthly_total_yuan, per_video_remaining: PER_VIDEO_CAP - estimatedYuan };
}
```

## Auto-Pause Logic

When `checkCost` throws:
1. Abort the entire asset batch — do not partially generate.
2. Roll back any in-flight state (no half-written video records).
3. Surface the breach reason to the operator via the dashboard's cost banner.
4. If monthly cap: switch the pipeline to read-only mode until next month.

## PostToolUse Hook (write-tracked)

A PostToolUse hook fires after every asset-generation Write/Edit. It appends to `videos[].totals` and recomputes `monthly_total_yuan`. Implementation in `hooks/cost-tracker.js`:

```js
// ~/.claude/hooks/cost-tracker.js — invoked on PostToolUse:Write|Edit
import fs from 'node:fs';
const TRACKER = '/tmp/vivify-cost-tracker.json';
const PRICE = { video_std: 0.5, video_pro: 2.0, image: 0.3, bgm: 0.2, tts: 0.05, script: 0.03 };

export function track(toolName, filePath) {
  if (!/(video|asset|render|export)\.json$/.test(filePath)) return;
  const t = JSON.parse(fs.readFileSync(TRACKER, 'utf8'));
  const kind = filePath.includes('pro') ? 'video_pro' : 'video_std';
  const cost = PRICE[kind] ?? 0;
  t.monthly_total_yuan += cost;
  t.videos.at(-1).totals.video_yuan = (t.videos.at(-1).totals.video_yuan ?? 0) + cost;
  fs.writeFileSync(TRACKER, JSON.stringify(t, null, 2));
}
```

## Operator Playbook

- First run of the month: set `OPC_MONTHLY_CAP=60000` in `.env`.
- If a single video legitimately needs more, bump `OPC_VIDEO_CAP` per-call and re-run check.
- Monthly reconciliation: export `/tmp/vivify-cost-tracker.json` to the dashboard at month end.
- If `monthly_total_yuan > 50000`: alert the team, throttle opus usage to zero.
