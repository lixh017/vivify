# Anti-Patterns — Why ≥2 Negative Tokens Per Outfit

When generating outfit images, models can drift into wrong clothing.
`anti_patterns` are explicit "never" tokens appended to the prompt to
prevent this.

## Why this matters

Default Seedream/Seedance will:
- Put the panda in a Western suit when you wanted Chinese hanfu
- Add accessories you didn't ask for (glasses, hats)
- Change colors (朱红 → 暗红, 翠绿 → 暗绿)

## Minimum: 2 negative tokens

```yaml
outfit_changshan_blue:
  prompt: 中年男性, 蓝色长衫, 棉麻质感...
  anti_patterns:
    - "不要穿西装"      # 1st: explicit forbid
    - "不要现代服饰"    # 2nd: category-level forbid
    - "不要深色调"      # 3rd: tone forbid
    - "不要光滑面料"    # 4th: texture forbid
    - ...
```

## Recommended: 5-7 negative tokens

Strong protection (validated against 5+ IP generations):
1. Category forbid: "不要西装/不要T恤/不要运动服"
2. Style forbid: "不要现代/不要赛博朋克"
3. Color forbid: "不要冷色调/不要暗色"
4. Texture forbid: "不要皮革/不要塑料"
5. Accessory forbid: "不要眼镜/不要帽子"

## What `vivify character validate` checks

```bash
$ vivify character validate fengge
✓ outfit_changshan_blue: 11 anti_patterns (PASS)
✓ outfit_workwear_orange: 7 anti_patterns (PASS)
✗ outfit_some_loose: 1 anti_pattern (FAIL — must be ≥2)
```

Failure is **blocking**: don't proceed to render until all outfits
pass validation.

## Reference

See `vivify-panda-character` skill for the full outfit schema.
