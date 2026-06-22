# 峰哥 / Fengge — IP Data Layer

> 峰哥 panda 是 OPC Phase 1 的 showcase IP。本目录包含**这个 IP 自己的所有数据**,与通用 framework 解耦。

## 数据布局

```
characters/fengge/
├── README.md          ← (本文件)IP 概览
├── character.yaml     ← IP 配置(5 服饰 / 12 场景 / 4 tone voice)
├── canonical/         ← 3 张参考图(identity anchor)
├── lessons.md         ← IP-specific lessons(峰哥踩过的坑)
├── gotchas.md         ← IP-specific pitfalls
├── examples/          ← 已出片的剧集(EP#003-006)
├── overrides/         ← 临时覆盖(per-shot 或 per-episode)
│   ├── EP005.bgm.json
│   └── EP007.style.json
├── experiments/       ← A/B 测试数据
│   └── EP003-hook-ab.json
└── analytics/         ← 表现数据(发布后填)
    └── EP003.engagement.json
```

## 与通用 framework 的边界

**属于这个 IP(每个 IP 自己管理)**:
- character.yaml 内容
- canonical/ 参考图
- lessons.md / gotchas.md(IP 踩过的坑)
- overrides/ 实验配置
- experiments/ A/B 数据
- analytics/ 表现数据

**属于通用 framework(所有 IP 共享)**:
- `memory/prompt-engineering/` — Seedance 2.0 公式等通用 prompt 工程
- `memory/model-capabilities/` — 模型能力笔记
- `prompt_library/` — 通用 prompt 片段
- `validators/` — lint 脚本
- `skills/` — agent 操作手册(无 IP 数据)

## 如何扩展这个 IP

### 加新服饰
1. 编辑 `character.yaml` → `outfits:` 段加新条目
2. 跑 `python3 validators/lint_character.py characters/fengge`
3. 修复所有 lint 错误(尤其是 `anti_patterns` 必须有 ≥2 个 negative token)
4. 重新生成 canonical image
5. 跑 `python3 validators/canonical_image_check.py characters/fengge/canonical`

### 加新场景
1. 编辑 `character.yaml` → `scenes:` 段加新条目
2. 同样跑 lint + canonical image check

### 加新 tone(超出治愈/御宅/哲学/国潮 4 个)
1. 编辑 `character.yaml` → `voice_profiles:` 加新 tone
2. 跑 lint
3. 出片测试

### 临时换 BGM / 风格(单集)
1. 在 `overrides/` 创建新 JSON,比如 `EP007.bgm.json`:
   ```json
   {
     "track": "different_bgm",
     "params": {"speed": 0.85}
   }
   ```
2. pipeline 自动读 overrides/ 下的 JSON 覆盖默认

### A/B 测试 hook
1. 在 `experiments/` 创建 `EP007-hook-ab.json`:
   ```json
   {
     "variant_a": {
       "hook_formula": "三问开场",
       "hook_text": "立秋了吗?凉了吗?你睡了吗?"
     },
     "variant_b": {
       "hook_formula": "引用钩子",
       "hook_text": "古人云,秋风起兮"
     }
   }
   ```
2. 用 render_episode.py 出 2 个版本对比

## Lessons & Gotchas

详见:
- `lessons.md` — 峰哥 IP 踩过的坑和验证过的方法
- `gotchas.md` — 具体的 debug pitfall 列表

## 未来迁移

如果峰哥 IP 真的火了,这个目录可以被独立成独立 repo / SDK 包,
不影响通用 framework 的演进。