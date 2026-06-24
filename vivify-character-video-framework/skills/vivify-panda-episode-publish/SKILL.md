---
name: vivify-panda-episode-publish
description: Use when user wants to publish a completed episode to a platform — 抖音 / 小红书 / B站. Records the publish in DB and fetches analytics. Currently a STUB for real platform upload (no 抖音 API credentials in env), but DB records + analytics refresh work. Triggers on "发布", "publish", "发抖音", "post to", "upload to". Don't trigger for: rendering (use vivify-panda-episode-build).
---

# vivify-panda-episode-publish — Publish Completed Episode

User wants to publish a rendered episode to a platform. Records in DB +
(optionally) uploads to the platform.

## When to use

- User: "把 EP005 发到抖音"
- User: "publish to xiaohongshu"
- User: "更新 EP001 的播放数据"

## When NOT to use

- Episode not yet rendered → `vivify-panda-episode-build` first
- User wants to delete a publish → use `vivify episode delete` CLI

## Current status: **PARTIAL STUB**

| Step | Status |
|---|---|
| DB record of publish | ✅ works |
| Real 抖音 upload | ❌ stub (no 抖音 开放平台 credentials) |
| Real 小红书 upload | ❌ stub |
| Real B站 upload | ❌ stub |
| Analytics refresh (view/like/comment) | ⚠️ stub random deltas |
| Aggregated analytics across platforms | ✅ works |

**Be honest with the user**: tell them publish currently writes a DB
record + stub URL, not a real upload. Real upload requires 抖音 开放平台
client_key + access_token that aren't currently in the environment.

## Steps (run via `vivify` CLI)

### 1. Verify episode is rendered

```bash
./scripts/vivify episode show <ip> <ep_id>
```

Status must be `completed`. If not, point user to `vivify-panda-episode-build`.

### 2. Publish

```bash
./scripts/vivify publish publish <ip> <ep_id> \
  --platform {抖音,小红书,哔哩哔哩} \
  --title "..." --description "..." --hashtags "..."
```

This writes a `publishes` row + returns a fake URL (stub).

### 3. (After real platform integration) verify upload

Currently skipped. Would call 抖音 /api/video/status to confirm.

### 4. Refresh analytics (later, manually)

```bash
./scripts/vivify publish refresh <publish_id>
```

Today this just adds random deltas. With real API, it would call the
platform's analytics endpoint.

### 5. View analytics

```bash
./scripts/vivify publish analytics <ip> <ep_id>
./scripts/vivify publish list [--platform 抖音]
```

## When the user says "I have 抖音 credentials"

If user provides `client_key` + `access_token`, swap the stub
implementation:

```python
# File: vivify/commands/publish.py
# Function: _upload_to_platform(platform, video_path, title, ...)
# Replace stub with real HTTP call per platform
```

Real endpoint per platform:
- **抖音**: `https://open.douyin.com/video/create/` + `/video/data/`
- **小红书**: `https://edith.xiaohongshu.com/api/sns/v1/web/notes`
- **B站**: `https://member.bilibili.com/v2/upload/video`

## Cost note

Publish itself is **free** (no API cost). Only video gen costs ¥.
Analytics refresh also free.

## References

- `references/01-real-api-integration.md` — exact endpoint specs per
  platform for when credentials become available

## Related

- `vivify-panda-episode-build` (L3) — pre-step (renders the episode)
- `vivify publish` CLI — what this skill wraps
