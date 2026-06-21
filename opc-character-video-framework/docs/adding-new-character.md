# Adding a New Character

> Step-by-step guide to onboard your own IP character into the framework.
> Expected time: **30-60 minutes** for a well-prepared character.

## 1. Prepare 3 reference images

These jpgs are sent to Seedream as `reference_image` for every shot.
Seedream will preserve the character's **face / identity features**
from these images across all generated scenes and outfits.

### Requirements

- **Format**: JPEG, 1024×1024 or 1024×1792
- **Pose**: front or 3/4 view, **neutral** (not laughing, not crying)
- **Background**: simple, plain (cream / white / solid color)
- **Props**: NONE held in hands
- **Lighting**: soft, even
- **Style**: 2D 国潮 illustration style recommended (matches the
  framework's default style_anchor); can also be 3D / watercolor /
  your style of choice — just update character.yaml accordingly

### Example prompts for generating these (Seedream)

```
Studio portrait of an adult character named <NAME>,
neutral pose, slight 3/4 turn, calm expression,
wearing <default outfit>, no props held in hands,
plain background, consistent 2D illustration style,
NOT photorealistic, NOT 3D rendered toy aesthetic
```

Generate 3 jpgs with different default outfits:
- `primary.jpg` (the default anchor — sent for every shot)
- `alternate-1.jpg` (different outfit, same pose)
- `alternate-2.jpg` (different outfit, same pose)

These alternates are useful as **outfit swatches** when reviewing the IP
bible — they show designers how each outfit looks on the locked identity.

## 2. Set up the directory structure

```bash
mkdir -p characters/<your-name>/canonical
cp primary.jpg     characters/<your-name>/canonical/primary.jpg
cp alternate-1.jpg characters/<your-name>/canonical/alternate-1.jpg
cp alternate-2.jpg characters/<your-name>/canonical/alternate-2.jpg
cp characters/_template/character.yaml characters/<your-name>/character.yaml
```

## 3. Fill in character.yaml

Open `characters/<your-name>/character.yaml` and fill in:

### Required fields

| Field | Example | Notes |
|---|---|---|
| `character.name` | `"小狐"` | Chinese display name |
| `character.english_name` | `"XiaoHu"` | Pinyin / English |
| `character.species` | `"成年狐狸"` | 物种 |
| `character.head_body_ratio` | `"1:1.2"` | 头:身 |
| `character.fur_face` | `"F5F0E1"` | 面部 hex(无 #) |
| `character.fur_extremity` | `"C73E1D"` | 四肢 hex |
| `character.expression` | `"半阖眼 + 微微笑意"` | 标志性表情 |

### Outfits (5 recommended)

```yaml
outfits:
  - id: outfit_default_1
    name: "<name>"
    color: "<color hex>"
    description: "<brief visual description>"
    best_for: ["<tone>"]
    anti_patterns: "<what NOT to render — e.g. 'no exposed modern elements'>"
```

### Scenes (5-12 recommended)

```yaml
scenes:
  - id: scene_id
    name: "<name>"
    mood: "<mood>"
```

### Voice profiles (1 per tone you want)

```yaml
voice_profiles:
  治愈:
    voice_id: "male-qn-jingying"   # 海螺 voice ID
    speed: 0.85
    pitch: 0
    emotion: "neutral"             # neutral | happy | sad | angry | fearful | disgust | surprised
    vol: 1.0
    guide: "one-line tone description for the LLM"
```

### Style anchor (lock visual register)

```yaml
style_anchor: |
  consistent 2D 国潮 illustration style,
  soft watercolor wash + ink accents,
  vibrant saturated palette,
  NOT photorealistic, NOT 3D rendered toy aesthetic
```

### Forbidden scenes / poses

```yaml
forbidden:
  scenes:
    - "现代写字楼"
  poses:
    - "露爪"
    - "攻击性姿势"
```

## 4. Test the character

```bash
export ARK_API_KEY="..."
export MINIMAX_API_KEY="..."
python3 render_episode.py \
  --character-dir characters/<your-name> \
  --storyboard <path to a test storyboard> \
  --script    <path to a test script> \
  --voice     <your tone> \
  --platform  抖音 \
  --out       /tmp/test
```

The pipeline will:
1. Load your character.yaml
2. Print: `character: <name> from characters/<your-name>`
3. Print: `outfits: 5  scenes: 12  tones: <your tones>`
4. Use `canonical/primary.jpg` as the Seedream reference
5. Append your `style_anchor` to every shot's prompt
6. Use your `voice_profiles[tone]` for TTS

## 5. Iterate

Things that often need tweaking:

| Symptom | Fix |
|---|---|
| Face drifts across shots | Re-generate canonical jpg with stronger identity pose |
| Outfit renders as something wrong | Add more `anti_patterns` to the outfit |
| Style drifts to 3D-rendered | Add stronger `NOT 3D rendered` to style_anchor |
| Voice doesn't match tone | Adjust `speed` / `emotion` / try different `voice_id` |
| Scene looks modern | Add specific `forbidden.scenes` list |

## Reference: 峰哥 / Fengge

The bundled showcase character. See `characters/fengge/character.yaml`
for the full reference implementation:

- 5 outfits with anti-patterns
- 12 scenes across multiple moods
- 4-tone voice matrix (治愈/御宅/哲学/国潮)
- Style anchor with strong "NOT" clauses
- Hook formulas + voice rules + banned words

Use it as a template when designing your own.