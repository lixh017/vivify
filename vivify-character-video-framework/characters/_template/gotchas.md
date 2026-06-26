# {{name}} — gotchas & debugging notes

> Specific, code-level pitfalls. Each gotcha includes: symptom, root cause, fix, and how to detect it next time.
>
> Generated empty by `vivify character new`. Fill it in when you hit a bug, not before.

## Format

For each gotcha, copy this skeleton:

```markdown
## N. <Short title>

**Symptom**: <what you saw — observable in the rendered output, not the prompt>

**Root cause**: <why it happened — model behavior, parser bug, or config issue>

**Fix**: <what you did — code change, config tweak, or prompt rewrite>

**Detect next time**: <how to catch this BEFORE shipping>
```

---

## 1. (placeholder — first IP-specific gotcha goes here)

_t.b.w._

---

## Quick gotchas (table form, like fengge's gotchas.md §6)

| Gotcha | Fix |
|---|---|
| _t.b.w._ | _t.b.w._ |

---

## How to add a new gotcha

1. 复现 bug — render at least 1 shot that exhibits the issue, save the output
2. 在这个文件加一个 section, 包含: symptom / root cause / fix / how to detect
3. 跑 `bash tests/regression.sh` (待补) 确认 fix 不会回归
4. 在 `lessons.md` 的 status 一栏更新 (✅/🟡/🔴)
5. 如果 fix 是 cross-IP 适用的, 同步到 `memory/prompt-engineering/` 或 `memory/model-capabilities/`

## See also

- `lessons.md` — IP-specific lessons (validated patterns + failed experiments)
- `characters/fengge/gotchas.md` — reference for what kinds of issues belong here
