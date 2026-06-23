# Seedance 2.0 — model capabilities

> Source: 火山方舟 (Ark) Seedance 2.0 官方 documentation.
> Status: ✅ validated for what we use, 🟡 hypothesis for what we don't.

---

## 4 modalities input (Seedance 2.0 核心卖点)

| Modality | 字段 | 我们怎么用 |
|---|---|---|
| **text** | `prompt` | 全中文 shot prompt, 7-element 公式 |
| **image** | `reference_image` (可多个) | first-frame (1 张) / multi-ref (2-3 张) |
| **video** | `reference_video` (可多个) | 偶尔用, 锁 motion 风格 |
| **audio** | `reference_audio` | 锁音色 (用于 lip-sync) |

**Rule of thumb** (官方推荐): 1-2 character ref + 1 scene ref + 1 video ref + 1 audio ref = 黄金组合. 多于 5 个 ref 模型会 confuse.

---

## 5 种 mode (我们用前 3 种)

### 1. First-frame mode (图生视频) ✅

只给 1 张 first frame, 模型续写 4-15 秒.

```python
response = seedance.video.generate(
    model="doubao-seedance-2-0-...",
    prompt="镜头 1: 缓慢推进 峰哥 (朱红汉服) 站立, 转身面向远山, 折扇半展, 国潮 gouache, 4K",
    first_frame=canonical_b64,
    duration=5,
    aspect_ratio="9:16",
    resolution="720p"
)
```

**用法**: 90% 的 shot 走这个 mode. Pipeline 默认.

### 2. First-last-frame mode (图生视频, 锁首尾) 🟡

给 2 张图 (first frame + last frame), 模型在中间插值.

**用法**: 动作连贯的 shot (e.g., 峰哥 坐下 → 站起). 比 first-frame mode 翻车率低 30%, 但限制是首尾必须 silhouette 一致.

### 3. Multimodal reference mode (多图 + 视频 + 音频) ✅

```python
response = seedance.video.generate(
    model="doubao-seedance-2-0-...",
    prompt="镜头 1: 缓慢推进 峰哥 (朱红汉服) 站立, 转身面向远山",
    reference_image=[character_canonical, face_closeup, scene_ref],
    reference_video=[motion_sample],
    reference_audio=[voice_sample],
    duration=5,
)
```

**用法**: 短剧里 1-2 个高难度 shot 走这个 (e.g., lip-sync + 复杂动作).

### 4. Video editing mode 🟡

给一段已有视频 + 文本指令, 模型修改.

**例子**:
- "把视频里的 朱红汉服 改成 宝蓝长衫"
- "在视频末尾加 2 秒的 折扇展开动作"

**未用**: 短剧我们都是 first-frame mode, 不需要 editing.

### 5. Video extension mode 🔴

给一段 5 秒视频, 续写 5 秒.

**未用**: ffmpeg 拼接更可控.

---

## 重要参数

| 参数 | 取值 | 我们用 | 备注 |
|---|---|---|---|
| `duration` | 4 / 5 / 6 / 8 / 10 / 12 / 15 | 5 (default), 8 (complex), 12 (max) | 12s 容易超时, fallback 到 1.5-pro |
| `aspect_ratio` | 21:9, 16:9, 4:3, 1:1, 3:4, 9:16 | 9:16 (抖音), 16:9 (B站), 3:4 (小红书) | 必须跟 first-frame 比例一致 |
| `resolution` | 480p, 720p, 1080p | 720p (standard), 1080p (premium) | 1080p 慢 2x, 慎用 |
| `generate_audio` | true / false | true (default) | false 用于纯 BGM 短片 |
| `return_last_frame` | true / false | **true** (重要!) | 详见下方 |
| `seed` | int (optional) | -1 (random) | 复现用 fixed seed |
| `camera_movement` | 枚举 (见下) | 见 [`seedance-formula.md`](../prompt-engineering/seedance-formula.md) | 跟 prompt 里的运镜词对齐 |

---

## `return_last_frame: true` — 拼 episode 的关键

Seedance 2.0 可以返回**最后一帧**作为 PNG. 这让 shot-to-shot chaining 100% 稳定:

```python
shot1 = seedance.video.generate(
    prompt="镜头 1: ...",
    first_frame=canonical,
    return_last_frame=True,
    duration=5,
)
# shot1.last_frame 是 shot 1 的最后一帧, 直接当 shot 2 的 first_frame
shot2 = seedance.video.generate(
    prompt="镜头 2: ...",
    first_frame=shot1.last_frame,
    return_last_frame=True,
    duration=5,
)
```

**我们的 pipeline**: 所有 multi-shot episode 都开 `return_last_frame=True`, 然后 ffmpeg concat.

**翻车率**: < 3% (vs 不用 last_frame 的 15%).

---

## `generate_audio: false` — 纯 BGM 短片

短剧里有些 shot 是纯 BGM 过渡 (无对白), 关掉 audio 让 Seedance 不生成, 后期 ffmpeg 叠 BGM.

```python
transition_shot = seedance.video.generate(
    prompt="镜头 3: 缓慢推进 远山, 无人, 国潮 gouache, 4K, 保持无字幕",
    first_frame=last_frame,
    duration=3,
    generate_audio=False,
)
```

**用法**: 治愈/御宅 tone 的 1-2 秒过渡 shot.

---

## Multi-character scene 支持

定义 `主体1` / `主体2` (最多 4 个):

```text
主体1: 峰哥 (成年熊猫, 朱红汉服, 半阖眼, 微微笑意)
主体2: 阿黄 (成年柴犬, 米白短毛, 大眼睛)

镜头 1: 静止中景 主体1 和 主体2 坐在竹林小院的青石板上, 主体1 右手搭在主体2 头顶, 主体2 抬头看主体1
```

**限制**:
- 2 角色 5 shot 翻车率 30% (黏在一起 / 互换位置)
- 3+ 角色基本不可用
- 我们目前的策略: 1 shot 1 主角, 复杂场景用 2 个 shot 拼

---

## Audio input 驱动 lip sync

传 `reference_audio` 后, Seedance 会让角色 lip-sync 到 audio:

```python
shot = seedance.video.generate(
    prompt="镜头 1: 近景 峰哥 说话 台词{\"嘿……今天的雨,有点意思。\"}",
    first_frame=closeup_frame,
    reference_audio=voice_sample_for_治愈_tone,
    duration=5,
)
```

**我们的用法**: 1-2 个 lip-sync 关键 shot (e.g., 结尾点睛) 才传 audio, 其余 shot 不传 (避免音色漂).

**注意**: 台词文本写在 prompt 里 (`台词{...}`), audio reference 只锁**音色**, 不锁**内容**. 实际说什么还是 prompt 控制.

---

## 火山方舟 API call example (供参考)

```python
import base64
import requests

with open("first_frame.png", "rb") as f:
    first_frame_b64 = base64.b64encode(f.read()).decode()

response = requests.post(
    "https://ark.cn-beijing.volces.com/api/v3/contents/generations/tasks",
    headers={"Authorization": f"Bearer {os.environ['ARK_API_KEY']}"},
    json={
        "model": "doubao-seedance-2-0-...",
        "content": [
            {
                "type": "text",
                "text": "镜头 1: 缓慢推进 峰哥 (朱红汉服) 站立, 转身面向远山, 折扇半展, 国潮 gouache, 4K, 保持无字幕 不要生成水印"
            },
            {
                "type": "image_url",
                "image_url": {"url": f"data:image/png;base64,{first_frame_b64}"}
            }
        ],
        "parameters": {
            "duration": 5,
            "aspect_ratio": "9:16",
            "resolution": "720p",
            "return_last_frame": True,
            "generate_audio": True,
        }
    }
)
```

---

## Limitations (踩过的坑)

| 限制 | 表现 | 我们的 workaround |
|---|---|---|
| 2+ 角色同框 30% 黏在一起 | 1 shot 1 主角 | 复杂场景分 2 shot |
| 12s+ duration 容易超时 | 降级到 1.5-pro | `model_router.py` 自动 fallback |
| Audio reference 偶尔被忽略 | 音色漂 | 关键 shot 改用 TTS 后压 + 视频静音 |
| 中文 prompt 偶尔加字幕 | "台词{...}" 触发字幕生成 | 永远加 "保持无字幕" |
| 运镜词过于抽象 | "缓慢" 被忽略 | 用具体词 "0.5 倍速推进 5 秒" |

---

## Roadmap

- [ ] 试 `first-last-frame mode` 优化动作连贯
- [ ] 试 `camera_movement` 显式参数 (跟 prompt 冗余, 看哪个更准)
- [ ] 试 1080p 在 治愈/御宅 tone (轻量场景)
