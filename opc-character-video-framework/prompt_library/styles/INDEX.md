# Visual Style Anchors — Index

> `style_anchor` is the visual register that gets appended to every
> kling_prompt. It locks Seedream into a single aesthetic across all
> shots so you don't drift between 3D-rendered toy / photoreal /
> watercolor between scenes.

## Available anchors

| Anchor | Status | Tones it serves | When to use |
|--------|--------|-----------------|-------------|
| [guochao-2d.md](./guochao-2d.md) | ✅ validated | 国潮, 治愈, 哲学 | Default for traditional-Chinese IPs (pandas, foxes, cranes, monks). Validated across EP003–EP006 (4 episodes, 30+ shots). |
| [zhiyu-watercolor.md](./zhiyu-watercolor.md) | 🟡 partial | 治愈, 御宅 | Modern-soft watercolor. Use for 治愈-leaning IPs that want a quieter, more pastel register. |
| [ink-brush.md](./ink-brush.md) | 🔴 draft | 哲学, 国潮 | 水墨写意 — high-contrast ink + sparse color. Single-shot validated only. |
| cyberpunk-neon.md (coming soon) | 🔴 draft | 国潮 + 现代 | Neon + 国潮 hybrid. Tested once in `memory/xiaohu-example`. |
| ink-line.md (coming soon) | 🔴 draft | 哲学, 治愈 | Pure black-and-white line illustration. Single-shot only. |
| ukiyo-e.md (coming soon) | 🔴 draft | 国潮 | 木刻浮世绘 — Edo-period woodblock. Single-shot only. |

## How to choose

1. If your IP is 国潮-toned (traditional Chinese aesthetic with
   modern packaging), start with **guochao-2d** — it has the most
   validation history.
2. If your IP is 治愈-toned (cozy, pastel, intimate), try **zhiyu-watercolor**.
3. If your IP is 哲学-toned (contemplative, austere), try **ink-brush** —
   but expect to iterate.
4. Anything marked 🔴 is single-shot unverified. Plan to run the
   `validators/lint_prompt.py` and a full test episode before shipping.

## How to apply

Copy the `style_anchor` block from the chosen file into
`character.yaml`:

```yaml
character:
  style_anchor: |
    2D flat illustration, gouache paint, paper texture,
    cel-shaded (no gradient lighting, no soft 3D shadows),
    ...
```

The full anchor (positive + negative) is appended to every Seedream
prompt automatically by `render_episode.py`. Do not truncate.

## Anti-patterns in style_anchor

- ❌ `soft 3D` / `3D render` / `Pixar` / `Disney` — these attract the
  wrong style
- ❌ `photorealistic` / `cinematic photo` — opposite register
- ❌ Ambiguous negatives (`no bad quality`) — Seedream ignores these
- ✅ Strong unambiguous negatives: `NEVER 3D`, `NEVER PIXAR`, `NEVER DISNEY`

## Validation status legend

- ✅ validated — 8+ production shots with consistent output
- 🟡 partial — 2-7 shots, some drift between scenes
- 🔴 draft — single-shot, not stress-tested

See also:
- [fengge's actual anchor](../characters/fengge/character.yaml) — battle-tested
- [`../../memory/prompt-engineering/style-anchors.md`](../../memory/prompt-engineering/style-anchors.md) — engineering notes