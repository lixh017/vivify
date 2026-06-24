---
name: vivify-panda-new-ip
description: Use when creating a NEW IP character from scratch — user described a persona but no character.yaml exists yet. Generates character profile + 3 canonical reference images via the vivify CLI. Trigger: "create new character", "新 IP", "做个新角色", "from scratch", "I want a new panda/cat/dragon...". Don't trigger if IP already registered (use vivify-panda-episode-build).
---

# vivify-panda-new-ip — Create a New IP

User said "I want a new character". This skill creates the IP profile +
canonical reference images and registers in DB. **Pure orchestration** —
invokes `vivify` CLI commands, doesn't write characters itself.

## When to use

- User: "我想做个新 IP，治愈系熊猫"
- User: "create new character, fantasy warrior girl"
- User: "做个猫的角色"

## When NOT to use

- IP already exists (`vivify character list` shows it) → skip to
  `vivify-panda-episode-build`
- User only wants to **describe** an IP (not generate images) → just
  read `vivify-panda-character` reference, don't trigger this skill

## Steps (run via `vivify` CLI)

### 1. Gather inputs from user (3 questions)

| # | Ask | Why |
|---|---|---|
| 1 | "What species / creature?" (熊猫/猫/人/龙) | needed for character.yaml |
| 2 | "What's the personality / 口头禅?" | voice + tone |
| 3 | "Pick a tone family: 治愈 / 御宅 / 哲学 / 国潮" | one of the 4 tones |

### 2. Generate `characters/<id>/character.yaml`

Use the LLM to produce a yaml file matching the schema in
`vivify-panda-character` skill. Save to `characters/<id>/character.yaml`.

**Don't write the YAML by hand**. Read `vivify-panda-character` skill for
the schema reference, then have the LLM produce it from the user's 3 answers.

### 3. Generate 3 canonical reference images

```bash
# Three views: close-up face + full-body × 3 outfits (default 5 outfits
# in character.yaml — pick the 3 most distinct)
mkdir -p characters/<id>/canonical
# Generate via Seedream 4.0 with reference_image=none for first 2,
# then use shot 1 as reference for shots 3 and 4
```

> **CRITICAL**: close-up face shot must be SEPARATE from full-body.
> This is the ID-drift fix. See `references/01-id-drift.md` in
> `vivify-panda-character`.

Generate programmatically via Seedream (don't draw by hand). Each image:
- 1024x1792 (9:16 portrait, matches output aspect)
- Inline base64 (no signed URLs, no 24h expiry)
- Use `mmx image generate --prompt "..." --aspect-ratio 9:16 --out ...`
  (cheaper) OR Seedream via CLI

### 4. Register in DB

```bash
./scripts/vivify character add <id> characters/<id>
./scripts/vivify character validate <id>      # must PASS
```

If validate fails:
- Missing `anti_patterns` on outfits → fix character.yaml
- Missing canonical jpgs → re-run step 3
- Image too small → re-generate with higher resolution

### 5. Done

Tell the user:
- IP name
- Where `characters/<id>/` is
- Run `vivify character show <id>` to verify
- Next step: `vivify-panda-episode-build`

## References

- `references/01-anti-patterns.md` — why ≥2 negative tokens per outfit
- `references/02-id-drift.md` — why close-up face must be separate
- `references/03-validation.md` — what `vivify character validate` checks

## Related

- `vivify-character-video` (L4) — dispatched here for new IPs
- `vivify-panda-episode-build` (L3) — next step after creating IP
- `vivify-panda-character` (L3 reference) — IP bible, schema source
