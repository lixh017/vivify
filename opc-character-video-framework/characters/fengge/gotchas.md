# peak哥 — gotchas & debugging notes

> Specific, code-level pitfalls. Each gotcha includes: symptom, root cause, fix, and how to detect it next time.

## 1. Seedream ignores weak anti-patterns

**Symptom**: Outfit 看起来不像你写的, 而是偏向常见误解 (清洁工, 西装, cosplay).

**Root cause**: Seedream 4.0 对 outfit 的先验是 "这个颜色 + 这个物种 ≈ 现实里最常见的服装". 一句 "no business suit" 不够 — 模型把 "suit" 当成关键词避开了, 但还是渲染了 商务休闲 / 办公衬衫.

**Fix**:
1. 在 `anti_patterns` 写**具体的物品**, 不要写抽象类别:
   ```yaml
   # WRONG
   anti_patterns: "no business suit"
   # RIGHT
   anti_patterns: "no reflective stripes, no logo badges, no work number tags,
                   no nylon velcro, no metal zippers, no plastic buttons"
   ```
2. 在 `anti_patterns` 末尾写**正向的具体物**, 告诉模型该长什么样:
   ```yaml
   anti_patterns: "...; only 盘扣 (Chinese frog buttons) + cotton/linen
                   fabric + 国潮 patterns"
   ```
3. 跑 [`validators/lint_character.py`](../../validators/) — 它会 warn 当 `anti_patterns` < 4 条 negative clause.

**Detect next time**: 写完 character.yaml 后, 跑 1 个 shot 测试 outfit, **不要**直接进 episode.

---

## 2. "3D toy drift" — 50% of shots drift to 3D-rendered

**Symptom**: 毛绒 IP (panda / cat / bear) 经常被 Seedream 渲染成 皮克斯 / Disney / CG 玩具风格, 哪怕你 prompt 里写 "2D illustration".

**Root cause**: Seedream 4.0 训练集里 3D-rendered character 占比非常高 (think Pixar shorts, 3D mobile game ads). 模型对 "cute anthropomorphic animal" 的先验是 3D.

**Fix**: 在 `style_anchor` 里**显式列举** banned aesthetic, 至少 3 次 NEVER:

```text
ALWAYS 2D FLAT NEVER 3D NEVER PHOTOREALISTIC,
NEVER PIXAR NEVER DISNEY NEVER CG RENDERED TOY,
NEVER BLURRY NEVER DARK NEVER MUTED DESATURATED
```

当前 fengge 漂移率: < 5% (从 50% 降下来).

**Detect next time**: 跑 EP001 后立刻抽查 5 个 shot, 看有没有 "rendered toy" 感 (光泽皮肤, subsurface scattering, 完美对称). 有就回去加 NEVER clause.

---

## 3. 暖橙 outfit 渲染成清洁工制服

**Symptom**: `outfit_workwear_orange` (暖橙短褂) 出现 反射条 / 工号牌 / 拉链.

**Root cause**: 同上 — "orange short jacket" 在模型记忆里 ≈ 环卫工 / 加油站员工 / 快递员制服.

**Fix**: 见 `lessons.md` 第 41–58 行. 关键是 `anti_patterns` 必须列出 6 条具体物品.

**Detect next time**: 给新 outfit 写完 yaml 后, 跑 1 个 shot 看看 `bash examples/<ip>/test-outfit-shot.sh`.

---

## 4. Subtitle parsing bug — 旁白 总计 误伤

**Symptom**: Episode 最后 1 个 shot 突然消失, 或者旁白空着.

**Root cause**: `render_episode.py` 里有一个 filter:

```python
# WRONG
shots = [s for s in blocks if s.count("旁白") <= 1]
```

当 script 的 footer 写了 "旁白 总计 N 条" 时, 那个 footer 块的 `旁白` 字数 ≥ 2, **被错误地保留**, 同时把最后一个真正的 shot 块 (只有 1 个 "旁白") 给过滤掉了.

**Fix**:

```python
# RIGHT — only count 旁白 in the main shot marker, not in footer summary
def is_shot_block(block):
    return block.lstrip().startswith(("镜头", "【镜头", "分镜", "Shot"))
shots = [s for s in blocks if is_shot_block(s)]
```

**Detect next time**: 写完 SCRIPT 之后, 跑 `bash tests/test_subtitle_parser.sh` (待补). 或者手动数 shot 数, 跟 STORYBOARD 对一遍.

---

## 5. `find_voiceover_for_shot` 返回 first overlap, 不是 closest start

**Symptom**: Shot 5 用的是 Shot 4 的旁白, 差半拍.

**Root cause**: 早期实现:

```python
# WRONG
def find_voiceover_for_shot(shot_start, voiceovers):
    for v in voiceovers:  # iterates in file order
        if v.start < shot_start < v.end:
            return v  # returns FIRST match, not closest
```

当两个旁白重叠 (e.g., Shot 4 的旁白还没说完 Shot 5 已经开始了), 返回了 Shot 4 的那个.

**Fix**:

```python
# RIGHT
def find_voiceover_for_shot(shot_start, voiceovers):
    candidates = [v for v in voiceovers
                  if v.start <= shot_start <= v.end]
    if not candidates:
        return None
    # Pick the one with the largest start ≤ shot_start (most recent)
    return max(candidates, key=lambda v: v.start)
```

**Detect next time**: 渲染后用 `ffprobe` 看每个 shot 的音频起始时间戳, 跟 STORYBOARD 的 `start` 字段对比, 误差 > 200ms 就有问题.

---

## 6. 其他 quick gotchas

| Gotcha | Fix |
|---|---|
| `voice_id` 写错 → MiniMax 返回 silent audio | Lint 在 `validators/lint_voice_id.py` |
| `aspect_ratio` 写 `9:16` 但实际平台是 哔哩哔哩 (16:9) | platform 自动映射, 见 `model_router.py` |
| `reference_image` 传本地路径, 渲染机找不到 | 永远 inline base64, 见 `lessons.md` |
| Seedance duration > 12s 容易超时, 降级到 1.5-pro | `model_router.py` 自动 fallback |
| Lint 报 "shot prompt missing 镜头 N:" 前缀 | 强制 Seedance 公式, 见 [`seedance-formula.md`](../prompt-engineering/seedance-formula.md) |
| `emotion: sad` 在 治愈 tone 里让人物看起来抑郁 | emotion 必须跟 tone 配对, 见 `character.yaml.voice_profiles` |

---

## How to add a new gotcha

1. 复现 bug
2. 在这个文件加一个 section, 包含: symptom / root cause / fix / how to detect
3. 跑 `bash tests/regression.sh` 确认 fix 不会回归
4. 在 `INDEX.md` 的 status 一栏更新 (✅/🟡/🔴)
