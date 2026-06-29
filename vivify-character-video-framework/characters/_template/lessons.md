# {{name}} — IP-specific lessons

> Lessons learned from rendering episodes of the {{name}} ({{english_name}}) IP.
> These are not general — they only apply to this character.
>
> This file is generated empty by `vivify character new`. Fill it in after each
> episode, like a lab notebook. Future you (and any other agent working on this
> IP) will thank present you.

## Identity at a glance

- 物种: {{species}}, 头身比 1:1.2
- 配色: {{palette_primary}} / {{palette_secondary}} / (调色板待补)
- 默认语气: {{expression_default}}
- Personality: {{personality_1}}, {{personality_2}}, {{personality_3}}
- Default canonical: `characters/{{name}}/canonical/primary.jpg` (to be generated)
- File: [`characters/{{name}}/character.yaml`](../../characters/{{name}}/character.yaml)

---

## Post-render checklist

After each new episode, fill in at least ONE of the sections below. The format
follows `characters/fengge/lessons.md` — copy that file's structure as a model.

- [ ] Episode EP___ 渲染完毕
- [ ] ✅ 至少 1 条 "What WORKED" 写进下面
- [ ] 🟡 至少 1 条 "What DIDN'T work" 写进下面 (hypothesis 或 翻车记录)
- [ ] best practices 表更新 (新 row)
- [ ] 1 句总结加到 fengge `lessons.md` 末尾的 "经验池" (cross-IP 时)

---

## ✅ What WORKED (validated in production)

<!-- Add a new H3 section for each validated lesson. Format:
### N. <Short title>

<Why it worked>

**Evidence**: EP___ rendered cleanly using this approach; lint pass rate ___% → ___%.
-->

### 1. (placeholder — first working pattern goes here)

_t.b.w._

---

## 🟡 What DIDN'T work (hypothesis / 翻车记录)

<!-- Add a new H3 section for each failed experiment. Format:
### N. <Short title>

**Symptom**: <what you saw>
**Root cause**: <why it happened>
**Fix**: <what you did>
-->

### 1. (placeholder — first failed pattern goes here)

_t.b.w._

---

## Best practices for similar characters

| Lesson | Why it matters |
|---|---|
| _t.b.w._ | _t.b.w._ |

---

## Status legend

- ✅ validated — proven in ≥ 3 episodes, lint passes
- 🟡 hypothesis — works but only 1–2 episodes
- 🔴 draft — unverified, but interesting idea

## See also

- `gotchas.md` — IP-specific code-level pitfalls (debugging notes)
- `characters/fengge/lessons.md` — cross-reference for the showcase IP
- `memory/prompt-engineering/` — general prompt-engineering lessons
