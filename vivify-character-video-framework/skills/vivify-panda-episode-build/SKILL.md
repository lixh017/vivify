---
name: vivify-panda-episode-build
description: Use when building a new episode for an existing IP — user has a topic in mind, wants storyboard + script generated, then rendered to MP4. Orchestrates the vivify CLI to do the actual work. Triggers on "做一集", "出 EP001", "render episode", "make episode", "build episode". Don't trigger for: new IP (use vivify-panda-new-ip), publish-only (use vivify-panda-episode-publish).
---

# vivify-panda-episode-build — Build One Episode

User wants to produce ONE episode. Orchestrates the CLI to:

1. Generate storyboard from topic (LLM)
2. Generate script (LLM)
3. Register in DB (`vivify episode add`)
4. Render (`vivify episode render --parallel N`)
5. Verify output (`vivify episode show`)
6. Report back to user

## When to use

- User: "做一集治愈系熊猫短剧，主题是凌晨 3 点失眠"
- User: "build EP002 for fengge"
- User: "再做一个"
- User: "render an episode about X"

## When NOT to use

- IP doesn't exist yet → `vivify-panda-new-ip` first
- User wants to publish (not build) → `vivify-panda-episode-publish`
- User wants status / cost / history → use `vivify` CLI directly (no scenario)

## Inputs you need

| Q | Ask | Why |
|---|---|---|
| IP id | "用哪个角色？" (default: fengge) | needed for `vivify character add` |
| Topic | "这集讲什么？" (one sentence) | drives storyboard |
| Tone | "治愈 / 御宅 / 哲学 / 国潮" (default: 治愈) | voice + TTS |
| Platform | "抖音 / B站 / 小红书" (default: 抖音) | format constraints |
| Duration | "想多长？" (default: 58s) | timing |
| Provider | "视频用 ark (Seedance) 还是 minimax (Hailuo)?" (default: ark) | cost / availability |
| Cost override | "这集打算花多少？超过 ¥100 要确认" (default: 100) | cost cap |

If user answers fewer than 3 questions, ask the rest yourself.

## Steps (run via `vivify` CLI)

### 1. Generate storyboard

Use the LLM with these inputs to produce `STORYBOARD.md` in the
vivify-character-video-framework's `characters/<ip>/examples/panda-episode-NNN/`
folder. The format must match the Seedance 2.0 公式
(see `vivify-panda-character` skill).

```
## 镜头 1 — 黑屏白字 (钩子) [0:00-0:03]

视觉: ...
构图: ...
灯光: ...
时长: 3s
镜头运动: 静止
可灵 prompt: 竹林, 月光, 黑屏白字, "凌晨三点的竹林", 9:16 构图
```

### 2. Generate script

Use the LLM with same inputs to produce `SCRIPT-douyin.md` matching the
3-段式 (hook + body + CTA) format from `vivify-script-generation`.

```
[0:00-0:03]
画面: 黑屏白字 "凌晨三点的竹林"
旁白: 凌晨三点的竹林...
```

### 3. Register in DB

```bash
./scripts/vivify episode add <ip> <ep_id> \
  --storyboard <path> --script <path> \
  --voice <tone> --platform <platform> --target-dur <duration>
```

### 4. Render

```bash
./scripts/vivify episode render <ip> <ep_id> \
  --storyboard <path> --script <path> \
  --voice <tone> --platform <platform> --target-dur <duration> \
  --video-provider <ark|minimax> \
  --parallel 4
```

> **Cost cap check** runs automatically before render. If estimated
> cost > ¥100, render is BLOCKED. User must pass `--force` or reduce.

### 5. Verify

```bash
./scripts/vivify episode show <ip> <ep_id>
./scripts/vivify episode status <ip> <ep_id>
```

If status=failed, check error_message and consult
`vivify-character-video/references/02-recovery.md`.

### 6. Report to user

> 渲染成功！
> - MP4: .tmp/renders/<ip>-<ep_id>.mp4
> - 成本: ¥X.XX
> - 状态: completed
> - 下一步: `vivify-panda-episode-publish` 发抖音

## Common errors

See `vivify-character-video/references/02-recovery.md`. Top 3:
1. **403** → Seedance 没权限，建议换 minimax
2. **quota_exceeded** → MiniMax Token Plan 满，建议等下月
3. **ARK_API_KEY not set** → 用户没设环境变量

## References

- `references/01-storyboard-format.md` — exact STORYBOARD.md schema
- `references/02-script-format.md` — SCRIPT-douyin.md schema
- `references/03-provider-comparison.md` — ark vs minimax tradeoffs

## Related

- `vivify-character-video` (L4) — dispatched here when IP exists
- `vivify-panda-new-ip` (L3) — pre-step if IP doesn't exist
- `vivify-panda-episode-publish` (L3) — next step after render
- `vivify-cost-cap` (L3) — budget rules enforced inside `vivify episode render`
