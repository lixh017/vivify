---
name: vivify-provider-volcengine
description: Use this skill when calling 火山引擎 Ark for opc asset generation — video (Seedance 1.5-pro / 1.0-pro-fast), image (Seedream 4-0 / 4-5), and TTS. Handles async task creation + polling for video, sync POST for image, and bearer auth from ARK_API_KEY env. Triggers on "火山引擎", "Seedance", "Seedream", "Ark", "调视频", "调图片", "i2v", "t2v", or any opc asset request that needs 火山 vendor.
---

# vivify-provider-volcengine (v2, 2026/06)

真接火山引擎 Ark 调视频/图像/TTS。**不是模板** — 是真用 curl 能跑通的集成。

**Auth 源**: `~/.claude/config/vivify-volcengine.env` (chmod 600) 或 env var `ARK_API_KEY`。
**Base URL**: `https://ark.cn-beijing.volces.com/api/v3` (国内, 已经含 `/api/v3` 前缀)
**协议**: Bearer token; 视频异步 (POST task + GET poll); 图像同步。

## 1. 视频 (Seedance 1.5-pro 主力 / 1.0-pro-fast 便宜)

### 创建任务 (POST task)

```bash
POST {baseUrl}/contents/generations/tasks
Content-Type: application/json
Authorization: Bearer {ARK_API_KEY}

{
  "model": "doubao-seedance-1-5-pro-251215",   // 或 1-0-pro-fast-251015 (便宜)
  "content": [
    {
      "type": "text",
      "text": "<prompt>  --duration 5 --resolution 720p --ratio 9:16 --camerafixed false --watermark true"
    },
    // t2v 时省略, i2v 时必填 (图生视频)
    {
      "type": "image_url",
      "image_url": { "url": "https://..." }
    }
  ]
}

→ HTTP 200
{ "id": "cgt-20260608140817-l579p" }
```

### 轮询任务 (GET poll)

```bash
GET {baseUrl}/contents/generations/tasks/{id}
Authorization: Bearer {ARK_API_KEY}

→ HTTP 200
{
  "id": "cgt-...",
  "status": "running | succeeded | failed",
  "model": "doubao-seedance-1-5-pro-251215",
  "content": {
    "video_url": "https://ark-content-generation-...tos-cn-beijing.volces.com/...mp4?X-Tos-Algorithm=..."
  }
}
```

**轮询间隔**: 5s (短任务) 到 30s (长任务)。超时 600s (10 分钟)。`status === 'succeeded'` 时 `content.video_url` 可下载 (24h 有效, 应立即落盘)。

## 2. 图像 (Seedream 4-0 主力 / 4-5 大图)

### 创建任务 (POST sync)

```bash
POST {baseUrl}/images/generations
Content-Type: application/json
Authorization: Bearer {ARK_API_KEY}

{
  "model": "doubao-seedream-4-0-250828",     // 或 4-5-251128 (要求 size ≥ 3,686,400 px)
  "prompt": "<text>",
  "size": "1024x1792"   // 9:16 竖屏。4-5 要 ≥ 1920x1920
}

→ HTTP 200
{
  "model": "doubao-seedream-4-0-250828",
  "created": 1780898945,
  "data": [{
    "url": "https://ark-content-generation-...tos-cn-beijing.volces.com/...jpeg?X-Tos-Algorithm=..."
  }]
}
```

**image_url 即下即存**。火山 URL 含 X-Tos-Algorithm 签名, 永久有效 (不像视频 24h)。

## 3. TTS (火山独立 endpoint)

```bash
POST https://openspeech.bytedance.com/api/v1/tts
Content-Type: application/json
Authorization: Bearer {ARK_API_KEY}

{ "text": "<中文台词>", "voice_type": "BV001_streaming", "encoding": "mp3", "speed_ratio": 1.0 }
```

## References

- [`references/reference-impl.md`](references/reference-impl.md) — full Node.js 集成 + 真验证历史
