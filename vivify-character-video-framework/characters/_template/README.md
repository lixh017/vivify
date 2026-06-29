# {{name}} / {{english_name}} — IP Data Layer

> {{name}} is a new IP added via `vivify character new {{name}}`. This directory contains **this IP's own data**, decoupled from the general framework.

## Quick facts

| Field | Value |
|---|---|
| Name | {{name}} ({{english_name}}) |
| Species | {{species}} |
| Personality | {{personality_1}}, {{personality_2}}, {{personality_3}} |
| Primary palette | `{{palette_primary}}` |
| Secondary palette | `{{palette_secondary}}` |
| Default voice | {{default_voice}} |
| Generated | by `vivify character new` (seed: `{{idempotency_seed}}`) |

## 数据布局

```
characters/{{name}}/
├── README.md          ← (this file) IP overview
├── character.yaml     ← IP config (3 outfits / 5 scenes / 4-tone voice)
├── canonical/         ← 3 reference jpgs (identity anchor)
├── lessons.md         ← IP-specific lessons (write after first render)
├── gotchas.md         ← IP-specific pitfalls (write after first render)
├── examples/          ← rendered episode dirs (write after first episode)
├── overrides/         ← per-shot / per-episode overrides (JSON)
├── experiments/       ← A/B test data
└── analytics/         ← performance data (fill after publishing)
```

## 与通用 framework 的边界

**属于这个 IP (每个 IP 自己管理)**:
- `character.yaml` 内容
- `canonical/` 参考图
- `lessons.md` / `gotchas.md` (IP 踩过的坑)
- `overrides/` 实验配置
- `experiments/` A/B 数据
- `analytics/` 表现数据
- `examples/` 已出片剧集

**属于通用 framework (所有 IP 共享)**:
- `memory/prompt-engineering/` — Seedance 2.0 公式等通用 prompt 工程
- `memory/model-capabilities/` — 模型能力笔记
- `prompt_library/` — 通用 prompt 片段
- `validators/` — lint 脚本
- `skills/` — agent 操作手册 (无 IP 数据)

## First-time setup (after `vivify character new`)

The generator created this directory with a parameterized `character.yaml`. To finish bringing up the IP:

1. **Review** `character.yaml` — fill in any placeholders that weren't covered by the 5 questions (outfit descriptions, voice guides, hook formulas).
2. **Lint** the config: `python3 validators/lint_character.py characters/{{name}}`
3. **Generate 3 canonical jpgs** — see `prompt_library/canonical-prompts.md`.
   - **CRITICAL**: close-up face shot must be SEPARATE from full-body (avoids ID drift — see `characters/fengge/lessons.md`).
4. **Run** `python3 validators/canonical_image_check.py characters/{{name}}/canonical`
5. **Render 1 test episode** to validate end-to-end.
6. **Write** `lessons.md` and `gotchas.md` with what you learned.

## How to extend this IP

### 加新服饰
1. 编辑 `character.yaml` → `outfits:` 段加新条目
2. 跑 `python3 validators/lint_character.py characters/{{name}}`
3. 修复所有 lint 错误 (尤其是 `anti_patterns` 必须有 ≥2 个 negative token)
4. 重新生成 canonical image
5. 跑 `python3 validators/canonical_image_check.py characters/{{name}}/canonical`

### 加新场景
1. 编辑 `character.yaml` → `scenes:` 段加新条目
2. 同样跑 lint + canonical image check

### 加新 tone (超出治愈/御宅/哲学/国潮 4 个)
1. 编辑 `character.yaml` → `voice_profiles:` 加新 tone
2. 跑 lint
3. 出片测试

### 临时换 BGM / 风格 (单集)
1. 在 `overrides/` 创建新 JSON, 比如 `EP001.bgm.json`:
   ```json
   {
     "track": "different_bgm",
     "params": {"speed": 0.85}
   }
   ```
2. pipeline 自动读 overrides/ 下的 JSON 覆盖默认

### A/B 测试 hook
1. 在 `experiments/` 创建 `EP001-hook-ab.json`:
   ```json
   {
     "variant_a": {
       "hook_formula": "三问开场",
       "hook_text": "{{opening_a}}"
     },
     "variant_b": {
       "hook_formula": "引用钩子",
       "hook_text": "{{opening_b}}"
     }
   }
   ```
2. 用 render_episode.py 出 2 个版本对比

## Lessons & Gotchas

详见 (创建出片经验后填充):
- `lessons.md` — {{name}} IP 踩过的坑和验证过的方法
- `gotchas.md` — 具体的 debug pitfall 列表

## Lessons from fengge (for reference)

While you build {{name}}'s own `lessons.md`, here are the cross-IP-validated lessons from the showcase IP that almost certainly apply to you too:

- 3 canonical jpgs, not 1 (default + 2 outfit alternates)
- Inline base64 for the reference image, never signed URLs (reproducibility)
- Per-tone voice profile — one TTS voice can't carry 4 tones
- Style anchor with 6+ NEVER clauses (Seedream default-prior is 3D)
- Anti-patterns must list SPECIFIC items ("no business suit" is too weak)
- Always append `保持无字幕, 不要生成水印, 不要生成 Logo` to prompts
- BGM from human-licensed libraries, not AI-generated (5–10s clips lack identity)

See `characters/fengge/lessons.md` for the full list with citations.
