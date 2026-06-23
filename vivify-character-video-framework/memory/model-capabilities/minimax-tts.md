# MiniMax TTS (speech-02-hd) — model capabilities & gotchas

> Source: 海螺 AI 官方 API documentation + manual testing on fengge EP001–EP006.
> Status: ✅ validated for what we use.

---

## TL;DR

| 项 | 值 |
|---|---|
| Provider | 海螺 AI (MiniMax) |
| Model | `speech-02-hd` |
| Auth | `MINIMAX_API_KEY` (env var, 永远不 hardcode) |
| Returns | **HEX string**, NOT base64 (踩坑) |
| Latency | ~1.5s for 10s audio |

---

## Per-tone voice_id mapping (peak哥)

| Tone | voice_id | speed | pitch | emotion | vol | 用途 |
|---|---|---|---|---|---|---|
| 治愈 | `male-qn-jingying` | 0.78 | -2 | neutral | 1.0 | 默认, 最慢最暖 |
| 御宅 | `male-qn-jingying` | 0.85 | -1 | neutral | 0.95 | 略快, 室内感 |
| 哲学 | `male-qn-qingse` | 0.82 | -3 | sad | 1.0 | 音色偏冷, 留白 |
| 国潮 | `male-qn-jingying` | 0.92 | 0 | neutral | 1.05 | 最快, 古诗感 |

> **为什么 哲学 单独用 `qingse`**: `jingying` 在 emotion=sad 时听起来像 "忧郁少年", 不够"存在主义". `qingse` 自带冷感, 配 sad 是"空"不是"哀".

Source: [`characters/fengge/character.yaml`](../../characters/fengge/character.yaml) 第 147–175 行.

---

## Parameters (官方文档)

| 参数 | 类型 | 范围 | 默认 | 备注 |
|---|---|---|---|---|
| `voice_id` | string | enum | - | 必须, 写错会 silent audio |
| `text` | string | ≤ 5000 字 | - | 必须, 支持中英 |
| `speed` | float | 0.5 – 2.0 | 1.0 | < 0.8 慢, > 1.2 快 |
| `pitch` | int | -12 – 12 | 0 | 整数, 不是 float |
| `emotion` | enum | neutral / happy / sad / angry / fearful / disgust / surprised | neutral | 7 选 1 |
| `vol` | float | 0.1 – 2.0 | 1.0 | < 1.0 轻, > 1.0 响 |
| `sample_rate` | int | 8000 / 16000 / 24000 / 32000 | 24000 | 我们用 24000 |
| `bitrate` | int | 32000 / 64000 / 128000 | 128000 | 我们用 128000 |
| `format` | enum | mp3 / wav / pcm | mp3 | 我们用 mp3 |
| `channel` | int | 1 / 2 | 1 | 单声道, 视频够用 |

---

## 🚨 GOTCHA: Returns HEX, NOT base64

**症状**: 第一次集成时, `base64.b64decode(audio_hex)` 抛 `binascii.Error: Invalid base64-encoded string`.

**Root cause**: 海螺 API 返回的 `audio` 字段是 **HEX-encoded** (16 进制字符串), 不是 base64. 文档里写得不明显.

**Fix**:

```python
# WRONG
audio_bytes = base64.b64decode(response["audio"])

# RIGHT
audio_bytes = bytes.fromhex(response["audio"])
```

**Detect next time**: 写完 TTS 调用, 立刻 `play(audio_bytes)`, 听一下. 如果是噪音 / 静音, 大概率是 HEX 没解码对.

---

## API call example

```python
import os
import requests

response = requests.post(
    "https://api.MiniMax.chat/v1/tts/async",
    headers={
        "Authorization": f"Bearer {os.environ['MINIMAX_API_KEY']}",
        "Content-Type": "application/json",
    },
    json={
        "model": "speech-02-hd",
        "voice_id": "male-qn-jingying",  # 治愈
        "text": "嘿……今天的雨,有点意思。",
        "speed": 0.78,
        "pitch": -2,
        "emotion": "neutral",
        "vol": 1.0,
        "sample_rate": 24000,
        "bitrate": 128000,
        "format": "mp3",
        "channel": 1,
    }
)
response.raise_for_status()

data = response.json()

# GOTCHA: HEX, not base64
audio_bytes = bytes.fromhex(data["audio"])

with open("output.mp3", "wb") as f:
    f.write(audio_bytes)
```

---

## Emotion mapping (踩过的坑)

`emotion` 在 TTS 里跟视觉里的"情绪" **不是一回事**. TTS emotion 控的是**音色起伏**, 不是 body language.

| Emotion | 音色表现 | 视觉表现 (我们 prompt 怎么写) |
|---|---|---|
| neutral | 平, 留白多 | 任何 tone 默认 |
| happy | 上扬, 略快 | 不要用 — 我们用 0.85+ speed + 嘴角上扬 替代 |
| sad | 略下沉, 慢 | 哲学 tone 用, 配合 ink-wash scene |
| angry | 紧, 快 | **不用** |
| fearful | 抖, 不稳 | **不用** |
| disgust | 收紧 | **不用** |
| surprised | 突高 | **不用** |

**Rule**: 4 个 tone 里只用 `neutral` 和 `sad`. 其余 emotion 会让 panda 听起来"戏剧化", 跟治愈/哲学/国潮 调性冲突.

---

## Latency & cost

| Metric | Value |
|---|---|
| Latency (10s audio) | ~1.5s |
| Latency (60s audio) | ~4s |
| Cost | ¥0.0001 / 字 (中文) |
| Rate limit | 60 req/min (default), 可申请提到 600 |

**我们的 budget**: 1 episode (90s 旁白) ≈ ¥0.5, 几乎可忽略.

---

## Sync with video (lip-sync)

TTS audio 跟 Seedance 视频对齐用 `ffmpeg`:

```bash
# 1. TTS 生成的 mp3 (10s 旁白)
# 2. Seedance 生成的 mp4 (5s 视频, audio 是 lip-sync 失败的, 静音)
# 3. ffmpeg 替换 audio

ffmpeg -i seedance_output.mp4 -i tts_output.mp3 \
  -c:v copy -c:a aac -map 0:v:0 -map 1:a:0 \
  -shortest final_shot.mp4
```

**关键**: 旁白长度可以 > 视频长度 (`-shortest` 自动截断), 也可以 < 视频长度 (用 `-af apad` 补静音).

---

## BGM strategy (跟 TTS 无关但常一起用)

TTS 只生成 voice. BGM 走**人工曲库**:

| Tone | BGM 来源 | 风格 |
|---|---|---|
| 治愈 | Epidemic Sound "lo-fi cozy" | 钢琴 + 雨声 |
| 御宅 | Epidemic Sound "study lo-fi" | lo-fi hip hop + 翻书声 |
| 哲学 | Epidemic Sound "ambient drone" | 低频 drone + 风 |
| 国潮 | 抖音免费 BGM 库"古风" | 古琴 / 笛箫 |

**不要**用 AI 生成 BGM — 5-10 秒片段里 AI 音乐辨识度不够, 一听就是 stock MIDI.

---

## How to add a new tone

1. 在 `character.yaml.voice_profiles` 加一个 tone
2. 选 voice_id (从海螺官方 voice 列表)
3. 调 speed / pitch / emotion / vol, 录 1 句 sample
4. 跑 `validators/lint_voice_id.py` 验证 voice_id 存在
5. 写 1 个 test shot 听效果
6. 满意后更新 [`characters/fengge/character.yaml`](../../characters/fengge/character.yaml) + 本文件
