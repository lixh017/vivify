# Validators

Linters for character configs and prompts in the AI character video
framework. All validators are standalone — they import only stdlib,
PyYAML, and Pillow, and they can be run independently.

## When to run

Run validators **before any render**, ideally inside CI or as a
pre-commit gate. A single failure should block the render pipeline.

| Stage        | Validator                            |
|--------------|--------------------------------------|
| Config edit  | `lint_character.py`                  |
| Config edit  | `lint_constraints.py`                |
| Asset add    | `canonical_image_check.py`           |
| Prompt write | `lint_prompt.py`                     |

## Usage

Each validator takes either a directory or a string and exits with:

- `0` — all checks passed
- `1` — one or more checks failed
- `2` — invalid arguments / load error

### lint_character.py

```bash
python3 validators/lint_character.py characters/fengge
```

Checks:

- Required fields present (`name`, `species`, `canonical`, `outfits`,
  `scenes`, `voice_profiles`)
- Each outfit has `anti_patterns`
- `style_anchor` contains strong negative anchors
  (`NEVER` / `no ` / `不要` / `NOT`)
- `style_anchor` is at least 30 chars
- At least 3 outfits, 5 scenes
- `voice_profiles` has at least 1 tone
- `canonical.primary` file exists and is >= 50 KB
- `canonical.alternates` are jpg files that exist

### lint_prompt.py

```bash
python3 validators/lint_prompt.py "镜头 1: 中景, 固定机位. 峰哥抱膝坐于炉旁, 手握茶盏, 暖光照面. 4s, 720p."
```

Checks (Seedance 2.0 best practices):

- Uses 分镜 structure (`镜头 N:` markers)
- Specifies exactly ONE camera movement
- Uses special-character syntax (`() <>` `{}` `【】`)
- Includes `no watermark / no logo / no text` constraints
- Emotion-to-action mapping (physical actions paired with emotions)
- Duration in 4-15s
- Resolution in 480p/720p/1080p

### lint_constraints.py

```bash
python3 validators/lint_constraints.py characters/fengge
```

Checks constraint *strength*:

- Each outfit's `anti_patterns` has >= 2 negative tokens
  (`NEVER` / `no ` / `不要` / `NOT` / `avoid`)
- `style_anchor` has >= 5 `NEVER` clauses
- Outfit descriptions are free of vague AI-tell words
  (`modern`, `realistic`, `human`, `generic`, `ordinary`, `normal`)

### canonical_image_check.py

```bash
python3 validators/canonical_image_check.py characters/fengge/canonical
```

Checks each image in the directory:

- File size >= 100 KB
- Resolution >= 1024x1024
- Aspect ratio is square (1:1) or 9:16 vertical
- File is JPEG or PNG

## Example output

### Passing run

```
$ python3 validators/lint_character.py characters/fengge

=== Linting characters/fengge ===

[required fields]
  [PASS] character.name present
  [PASS] character.species present
  [PASS] character.canonical present
  [PASS] character.outfits present
  [PASS] character.scenes present
  [PASS] character.voice_profiles present

[outfits]
  [PASS] outfits has >= 3 entries — found 5
  [PASS] outfit 'outfit_hufu_red' has anti_patterns
  ...

=== RESULT: ALL CHECKS PASSED (characters/fengge) ===
```

### Failing run

```
[outfits]
  [PASS] outfits has >= 3 entries — found 5
  [FAIL] outfit 'outfit_default_1' has anti_patterns — (empty)
  [WARN] outfits without anti_patterns (AI will generate wrong outfits) — outfit_default_1

=== RESULT: 1 CHECK(S) FAILED (characters/fengge) ===
```

## Common errors and fixes

| Error                                                | Fix                                                                                  |
|------------------------------------------------------|--------------------------------------------------------------------------------------|
| `outfit 'X' has anti_patterns — (empty)`             | Add a `no <thing>; no <thing>;` string to the outfit. Generic rules leak through.    |
| `style_anchor length < 30 chars`                     | Expand the style anchor to describe visual register in detail.                      |
| `style_anchor contains negative anchors — matched: none` | Add `NEVER` / `no ` / `不要` / `NOT` clauses — positive-only anchors leak defaults. |
| `voice_profiles has >= 1 tone — found 0`            | Add at least one tone block under `voice_profiles:`.                                 |
| `canonical.primary size >= 50 KB — 12.0 KB`          | Re-render at higher fidelity; tiny files usually indicate broken generations.        |
| `canonical.alternates are jpg files — bad.jpg (not .jpg)` | Convert to JPEG, or update the `alternates:` list to point at jpg files.       |
| `outfit 'X' anti_patterns negative tokens = 1`       | Add at least 2 negative phrases per outfit.                                          |
| `style_anchor has >= 5 'NEVER' clauses — found 2`    | Be explicit about what NOT to render. AI defaults to 3D / photoreal otherwise.      |
| `outfit 'X' description contains vague AI-tell words` | Replace `modern` / `realistic` / `human` with specific historical / cultural terms.  |
| `resolution < 1024x1024`                             | Re-export at >= 1024 on the short side.                                              |
| `aspect ratio is square or 9:16 — 16:9`              | Re-crop to 1:1 or 9:16 before using as a Seedream reference.                         |
| Prompt has no `镜头 N:` marker                       | Add `镜头 1: ...` headers for each shot in the 分镜 block.                          |
| Prompt has multiple camera movements                 | Pick one (推/拉/横移/跟拍/固定) per shot — combining them confuses motion models.    |
| Prompt missing `no watermark / no logo / no text`    | Add at least one of each. Seedance 2.0 will otherwise inject studio watermarks.     |
| Prompt emotion-only (`happy`, `sad`)                 | Pair emotions with physical actions (握住茶盏, 低头, 叹息).                          |
| Prompt duration out of 4-15s range                   | Adjust to fit Seedance 2.0's supported window.                                       |
| Prompt resolution missing                            | Add `720p` or `1080p` explicitly.                                                    |