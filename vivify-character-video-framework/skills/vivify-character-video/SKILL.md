---
name: vivify-character-video
description: Use when the user wants to create an IP character short-drama video — 熊猫 / panda / 短剧 / 抖音 / "做个视频" / "出个 episode". Top-level orchestrator: gathers user intent, dispatches to L3 panda scenario skills (vivify-panda-new-ip / vivify-panda-episode-build / vivify-panda-episode-publish). Pure orchestration — never calls APIs directly. Trigger keywords: panda, 熊猫, 峰哥, fengge, 短剧, 抖音, 出片, 渲染, 视频.
---

# vivify-character-video — Top-Level Orchestrator

User said "I want a video". This skill figures out **what kind** and routes
to the right scenario skill. Nothing else.

## When to use

- User: "帮我做个熊猫视频"
- User: "我想出个治愈系的短剧"
- User: "make a panda short drama"
- User: "渲染 EP001"

## Decision tree

```
User wants a video
├─ Is the IP (character) already created?
│   ├─ YES  → vivify-panda-episode-build  (storyboard → script → render)
│   └─ NO   → vivify-panda-new-ip        (gather persona, generate profile + canonical images)
│
├─ After render, does the user want to publish?
│   └─ YES  → vivify-panda-episode-publish (currently stub; real 抖音 API not available)
│
└─ At any point: cost / status / questions?
    └─ vivify cost status / vivify episode status <char> <ep>
       (read-only CLI; no scenario skill needed)
```

## What to ask the user (always, before invoking any L3 skill)

Read `references/01-questions.md` for the 5-question template.
Always ask — don't assume.

## What NOT to do here

- ❌ Don't call Ark / MiniMax / Suno APIs directly. Use `vivify` CLI.
- ❌ Don't write storyboards yourself. That's `vivify-panda-episode-build`.
- ❌ Don't generate canonical images. That's `vivify-panda-new-ip`.
- ❌ Don't handle 403/quota errors yourself. The CLI surfaces them; you interpret for the user.

## After completion

Tell the user:
- MP4 path (from `vivify episode show`)
- Cost (from `vivify cost status`)
- How to view past renders (from `vivify episode list`)
- How to publish (from `vivify-panda-episode-publish`)

## References

- `references/01-questions.md` — exact wording for the 5 user questions
- `references/02-recovery.md` — common errors and what to tell the user
- `references/03-output-formats.md` — how to present results to the user

## Examples

- `examples/panda-insomnia-60s.md` — full happy-path (治愈系, 60s, 抖音)

## Related skills

- `vivify-panda-new-ip` (L3) — create character + canonical images
- `vivify-panda-episode-build` (L3) — storyboard → script → render via CLI
- `vivify-panda-episode-publish` (L3) — publish to platform (stub today)
- `vivify-cost-cap` (L3) — budget enforcement rules
- `vivify-panda-character` (L3 reference) — IP bible, only read
