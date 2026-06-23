# Memory Index — Cross-IP knowledge

> 这个目录只放**跨 IP 通用**的知识。每个 IP 自己的数据(包括 lessons、gotchas、overrides)在各自的 `characters/<ip>/` 目录下。

## Cross-IP prompt engineering
- [prompt-engineering/seedance-formula.md](./prompt-engineering/seedance-formula.md) — Seedance 2.0 advanced prompt formula
- [prompt-engineering/style-anchors.md](./prompt-engineering/style-anchors.md) — tested style anchors
- [prompt-engineering/emotion-actions.md](./prompt-engineering/emotion-actions.md) — abstract emotion → concrete body action
- [prompt-engineering/id-drift-prevention.md](./prompt-engineering/id-drift-prevention.md) — how to keep identity stable

## Model capabilities
- [model-capabilities/seedream.md](./model-capabilities/seedream.md) — Seedream 4.0/5.0 features
- [model-capabilities/seedance.md](./model-capabilities/seedance.md) — Seedance 2.0 features
- [model-capabilities/minimax-tts.md](./model-capabilities/minimax-tts.md) — MiniMax TTS gotchas

## How to add new memory
When you discover a **cross-IP** lesson (one that applies to multiple characters), write to the appropriate category file.
**If the lesson is IP-specific**, write it to `characters/<ip>/lessons.md` instead.

Use status markers:
- ✅ validated (proven in production)
- 🟡 hypothesis (works but not stress-tested)
- 🔴 draft (unverified idea)

## What's NOT here
- ❌ Per-character configuration (`characters/<ip>/character.yaml`)
- ❌ Per-IP lessons and gotchas (`characters/<ip>/lessons.md`, `gotchas.md`)
- ❌ Per-shot overrides (`characters/<ip>/overrides/`)
- ❌ A/B test data (`characters/<ip>/experiments/`)
- ❌ Performance analytics (`characters/<ip>/analytics/`)

All of the above live in the per-IP data layer under `characters/<ip>/`.