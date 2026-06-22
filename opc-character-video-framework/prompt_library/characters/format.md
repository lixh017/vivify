# character.yaml — Schema Reference

> Complete field-by-field documentation for `characters/<name>/character.yaml`.
> Required reading before authoring a new IP. The fengge file at
> [`../../characters/fengge/character.yaml`](../../characters/fengge/character.yaml)
> is the canonical example.

## Top-level structure

```yaml
character:           # required
  name: ...
  english_name: ...
  species: ...
  head_body_ratio: ...
  fur_face: ...         # or palette: ... for non-fur IPs
  fur_extremity: ...
  expression: ...
  canonical: ...        # required
  outfits: [...]        # required, 5+ items
  scenes: [...]         # required, 5+ items
  forbidden: ...        # required
  props: [...]          # required
  voice_profiles: ...   # required
  style_anchor: ...     # required
  hook_formulas: ...    # required
  voice_rules: ...      # required
```

## Required fields

### `name` (string)
Display name in Chinese. Used in title cards.

```yaml
name: "峰哥"
```

### `english_name` (string)
Pinyin or English. Used in filenames and logs.

```yaml
english_name: "Fengge"
```

### `species` (string)
Free-form species description. Be specific (helps Seedream with
identity).

```yaml
species: "成年熊猫"  # not just "熊猫", not "bear"
```

### `head_body_ratio` (string)
Head-to-body ratio. The "chibi vs realistic" knob.

```yaml
head_body_ratio: "1:1.2"  # chibi-cute, fengge default
# head_body_ratio: "1:2"   # more realistic
# head_body_ratio: "1:3"   # fully realistic
```

### `fur_face` / `fur_extremity` (hex string, no `#`)
For furry IPs. Hex color of face and extremity (hands, feet, ears).

```yaml
fur_face: "F5F0E1"        # cream
fur_extremity: "1A1A1A"   # near-black
```

For non-fur IPs, use a `palette:` field instead:

```yaml
palette:
  primary: "#4A6B8A"
  secondary: "#E8DEC9"
  accent: "#C73E1D"
```

### `expression` (string)
The signature expression register. Used in [standing_calm.md](../actions/standing_calm.md).

```yaml
expression: "半阖眼 + 微微笑意,不直视镜头"
```

### `canonical` (object)
The 3 reference jpgs that Seedream will use as `reference_image`.

```yaml
canonical:
  primary: "canonical/your-char-front.jpg"     # required
  alternates:                                   # optional, but recommended
    - "canonical/your-char-side.jpg"
    - "canonical/your-char-back.jpg"
```

See [../canonical-prompts.md](../canonical-prompts.md) for how to
generate these.

### `outfits` (array of 5+)
Each outfit is a swappable wardrobe. Round-robin per episode.

```yaml
outfits:
  - id: outfit_xxx       # unique id, no spaces
    name: "朱红汉服"      # Chinese name
    color: "朱红 #C73E1D" # color description
    description: "vermilion red hanfu, wide-sleeved round-collar robe, jade pendant"
    best_for: ["国潮"]    # which tones this outfit serves
    anti_patterns: "no exposed modern elements; mandarin collar, jade accessories"
```

**Critical**: `anti_patterns` is the most important field. This is
what tells Seedream what to AVOID. See fengge's
`outfit_workwear_orange.anti_patterns` for the strongest example
(it explicitly bans modern cleaner-uniform cues).

### `scenes` (array of 5+)
Each scene is a reusable environment. See [../scenes/](../scenes/)
for the validated library.

```yaml
scenes:
  - id: bamboo_courtyard
    name: "竹林小院"
    mood: "warm, intimate"
```

### `forbidden` (object)
The hard no-list. Tells Seedream what to avoid.

```yaml
forbidden:
  scenes:
    - "现代写字楼"
    - "夜店"
  poses:
    - "露爪"      # claws always hidden
    - "攻击性姿势"
```

### `props` (array of strings)
Whitelist of allowed props. **Anything not on this list is forbidden**.

```yaml
props:
  - 折扇
  - 茶盏
  - 山竹杖
  - 古书
  - ...
```

### `voice_profiles` (object)
One profile per tone. Maps tone name → TTS voice_id + emotion +
speed.

```yaml
voice_profiles:
  治愈:
    voice_id: "male-qn-jingying"  # 海螺 voice id
    speed: 0.78                    # 0.5-2.0
    pitch: -2                      # -12 to +12
    emotion: "neutral"             # neutral|happy|sad|angry|fearful|disgust|surprised
    vol: 1.0                       # 0.5-2.0
    guide: "..."                   # human-readable guidance
```

The `guide` field is for the human author, not Seedream. It describes
how to write copy in this tone.

### `style_anchor` (multiline string)
The visual register lock. See [../styles/](../styles/) for the
library.

```yaml
style_anchor: |
  2D flat illustration, gouache paint, paper texture,
  cel-shaded (no gradient lighting, no soft 3D shadows),
  simplified shapes, anime-influenced lineart,
  ...
```

**Critical**: include BOTH positive anchors (what you want) AND
strong negative anchors (`NEVER 3D`, `NEVER PIXAR`).

### `hook_formulas` (array of 3+)
Templates for opening lines. fengge ships 7.

```yaml
hook_formulas:
  - name: "三问开场"
    formula: '"你有没有想过——X 到底是什么?Y 真的重要吗?凭什么 Z?"'
```

### `voice_rules` (object)
Sentence style + signature words + banned words.

```yaml
voice_rules:
  sentence_style:
    - "短句 ≤15 字"
    - "多用反问"
  signature_words:
    - "嘿"
    - "啊"
  banned_words:
    - "加油"
    - "失败是成功之母"
```

## Optional fields

None of the fields are truly optional, but you can ship with
minimum 5 outfits, 5 scenes, 3 hook_formulas, 1 voice_profile, and
a single canonical image.

## Common mistakes

1. **Empty `anti_patterns`**: this is the most important field for
   style lock. Don't ship without it.
2. **No `forbidden` block**: Seedream will default to common tropes
   (Western clothing, modern items, etc.). Always specify.
3. **Props not whitelisted**: if it's not in `props:`, don't put it
   in the prompt. The pipeline does not enforce this — it relies on
   the author.
4. **One canonical image only**: Seedream can preserve identity
   across 1-2 outfits with 1 image, but 3 outfits + 3 images is
   far more reliable.
5. **`style_anchor` without negatives**: Seedream needs the strong
   `NEVER 3D NEVER PIXAR` etc. to lock the style.
6. **Voice profile `guide` field too vague**: the guide is for the
   human author. Be specific ("慢、暖、留白" beats "soft tone").
7. **Banned words too generic**: `no bad words` is meaningless.
   `加油` / `愿你被世界温柔以待` are specific and useful.

## Validation

Run `python3 validators/lint_character.py characters/<name>` to
check schema. Run `python3 validators/canonical_image_check.py
characters/<name>/canonical` to verify the 3 reference jpgs.

## Cross-references

- [fengge's actual character.yaml](../../characters/fengge/character.yaml) — real example
- [../canonical-prompts.md](../canonical-prompts.md) — how to generate the 3 jpgs
- [../styles/INDEX.md](../styles/INDEX.md) — pick a style_anchor
- [../scenes/](../scenes/) — pick scenes
- [../actions/](../actions/) — pick action templates
- [../compositions/](../compositions/) — pick composition templates