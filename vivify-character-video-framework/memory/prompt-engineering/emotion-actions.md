# Emotion → concrete body action mapping

> Source: 火山方舟 (Ark) Seedance 2.0 官方 prompt-engineering 指南, 抽象情绪必须用具象身体动作表达.
> Status: ✅ validated — all fengge EP003–EP006 shots use this table.

## Why this table exists

Seedance 2.0 **不能动 "情绪"** — 它只能动 body. 当你在 prompt 里写 "峰哥 沉思", 模型没有任何视觉信号, 经常渲染出:
- 静止画面 (因为不知道怎么动)
- 完全无关的动作 (因为"沉思" 跟 panda 没视觉关联)
- 错的情绪 (把 "沉思" 渲染成"严肃", 把"喜悦" 渲染成"大笑")

**Rule**: 写 prompt 时, 永远从右列 (具象动作) 选 2–3 个, 写进 shot prompt.

---

## 4 tones × 4 emotions

| 抽象情绪 | 治愈 tone 动作 | 御宅 tone 动作 | 哲学 tone 动作 | 国潮 tone 动作 |
|---|---|---|---|---|
| 沉思 | 低头, 手指轻抚杯沿, 眼睛望下方茶汤 | 抱膝坐在书堆里, 手指无意识翻书页 | 仰望月亮, 背影, 衣摆被风轻吹 | 提笔悬于纸上, 迟迟不落, 眼睛微眯 |
| 喜悦 | 嘴角上扬, 眉眼舒展, 脚步变轻, 哼小曲 | 缩在毛毯里笑出声, 手舞足蹈 | (哲学不做喜悦 — 用"释然"代替) | 折扇轻展, 步伐略快, 眼睛明亮 |
| 紧张 | 频繁看窗外, 手指不停敲桌, 肩膀微微僵硬 | 不停刷手机又放下, 起身又坐下 | (哲学不做紧张 — 用"凝视"代替) | 手握折扇收紧, 脚步放慢, 频繁回头 |
| 释然 | 长长舒一口气, 肩膀放松, 微微仰头 | 把头靠在书堆上, 闭眼, 嘴角微翘 | 转身离开, 不回头, 衣摆随步伐摆动 | 合上折扇, 长揖, 转身缓步离去 |
| 孤独 | 一个人坐在窗边, 手指描窗框上的霜 | 缩在角落, 抱着毛绒玩偶, 灯只亮一盏 | 一个人站在山顶, 风大, 衣摆翻飞 | 一个人对月独酌, 桌上两副碗筷 |
| 治愈 (安心) | 抱着热茶, 闭眼, 头微微后仰 | 把脸埋进毛绒毯子, 深吸一口气 | (哲学不用"治愈" — 用"平静") | 把手贴在胸口, 闭眼, 微微点头 |

---

## 抽象 → 具象 quick reference (单列)

最常用的 8 个抽象词, 全部用具象动作替代:

| 抽象词 | 具象动作 (选 2-3) |
|---|---|
| 沉思 | 低头 + 眼睛望下方 + 手指轻抚杯沿 + 呼吸变慢 |
| 喜悦 | 嘴角上扬 + 眉眼舒展 + 脚步变轻 + 哼小曲 |
| 紧张 | 频繁看窗外 + 手指不停敲桌 + 肩膀微微僵硬 |
| 释然 | 长长舒一口气 + 肩膀放松 + 微微仰头 |
| 孤独 | 一个人坐在窗边 + 手指描窗框 + 灯只亮一盏 |
| 治愈 | 抱着热茶 + 闭眼 + 头微微后仰 |
| 凝视 | 静止不动 + 眼睛锁定一个方向 + 不眨眼 |
| 离开 | 转身 + 缓步 + 不回头 + 衣摆随步伐摆动 |

---

## Anti-patterns (绝对不要写)

| ❌ WRONG | ✅ RIGHT |
|---|---|
| "峰哥 沉思" | "峰哥 低头, 眼睛望下方, 手指轻抚杯沿" |
| "峰哥 看着远方, 心情复杂" | "峰哥 站立, 望向窗外, 手指反复搓衣角" |
| "峰哥 内心很挣扎" | "峰哥 频繁看窗外, 手指不停敲桌, 肩膀微微僵硬" |
| "峰哥 若有所思地喝茶" | "峰哥 双手捧茶杯, 低头看茶汤, 呼吸变慢" |
| "峰哥 强颜欢笑" | "峰哥 嘴角上扬, 但眼睛不跟着笑, 手指握紧扇骨" |

---

## How to use this table in code

`render_episode.py` 里有一个 helper:

```python
# pseudo-code
EMOTION_TO_ACTIONS = {
    "沉思": ["低头", "眼睛望下方", "手指轻抚杯沿"],
    "喜悦": ["嘴角上扬", "眉眼舒展", "脚步变轻"],
    ...
}

def expand_emotion(shot_prompt, tone):
    # Find abstract emotion words and replace with concrete actions
    for emotion, actions in EMOTION_TO_ACTIONS.items():
        if emotion in shot_prompt:
            replacement = " + ".join(actions[:3])
            shot_prompt = shot_prompt.replace(emotion, replacement)
    return shot_prompt
```

(Source: `render_episode.py:expand_emotion`, 待实现. 暂时手动从表里查.)

---

## Validation

- 写完 shot prompt 后, 跑 `validators/lint_prompt.py` 检查是否还含有抽象词.
- 抽象词黑名单: `沉思 / 喜悦 / 紧张 / 释然 / 孤独 / 治愈 / 凝视 / 离开 / 心情复杂 / 若有所思 / 强颜欢笑 / 内心挣扎`
