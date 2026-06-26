# Publish API Spec — 抖音 / 小红书 / 哔哩哔哩

> Research document for the next implementation agent. Goal: replace the
> stub `_upload_to_platform()` in `vivify/commands/publish.py` with real
> adapters for three Chinese short-video platforms. The DB layer
> (`publishes` table, `vivify publish publish / refresh / analytics`) is
> already real — this spec only describes the **HTTP integration layer**
> that goes underneath it.

**Status:** research draft, 2026-06-26.
**Audience:** the next agent (or human) implementing platform adapters.
**Authoritative source:** the `publishes` table schema in `vivify/db.py`
is the contract — every platform response must be mappable onto it.

---

## Table of contents

1. [Scope and context](#1-scope-and-context)
2. [Common contract: the `publishes` table](#2-common-contract-the-publishes-table)
3. [Shared constraints and conventions](#3-shared-constraints-and-conventions)
4. [Platform 1: 抖音 (Douyin)](#4-platform-1-抖音-douyin)
5. [Platform 2: 小红书 (Xiaohongshu / RedNote)](#5-platform-2-小红书-xiaohongshu--rednote)
6. [Platform 3: 哔哩哔哩 (Bilibili)](#6-platform-3-哔哩哔哩-bilibili)
7. [Mapping platform responses → `publishes` table](#7-mapping-platform-responses--publishes-table)
8. [Retry strategy](#8-retry-strategy)
9. [Minimum viable integration per platform](#9-minimum-viable-integration-per-platform)
10. [Environment variables and secrets layout](#10-environment-variables-and-secrets-layout)
11. [Open questions and blockers](#11-open-questions-and-blockers)
12. [Sources](#12-sources)

---

## 1. Scope and context

### 1.1 What this doc is

A **spec** for the real HTTP calls that will replace the stub. It is
research-grade — every non-trivial claim is sourced. Where the
authoritative documentation was inaccessible, we say so explicitly and
mark the field `unknown`.

### 1.2 What this doc is not

- Not implementation code. Code samples are illustrative (curl + JSON
  snippets), not drop-in Python.
- Not a guarantee that any specific endpoint will work — the open
  platforms require an approved developer application per platform
  before credentials are issued. See [§11 Open questions](#11-open-questions-and-blockers).
- Not a substitute for the live platform docs. When implementation
  begins, the implementer should re-verify each endpoint against the
  current official docs.

### 1.3 Current state of the code

`vivify/commands/publish.py` (L57-94) contains two stub functions:

```python
def _upload_to_platform(platform, video_path, title, description, hashtags) -> dict
def _fetch_analytics(platform, published_url) -> dict
```

Both return synthetic data. The DB write happens regardless (L153-161 of
publish.py) so the rest of the pipeline (`list`, `show`, `analytics`,
`refresh`) is already exercised end-to-end.

### 1.4 What needs to be replaced

| Function | Replace with |
|----------|--------------|
| `_upload_to_platform()` | Per-platform HTTP call → real `video_id` / share URL |
| `_fetch_analytics()` | Per-platform stats fetch → real counts |
| (new) `_check_upload_status()` | Poll for processing/moderation completion |
| (new) `_refresh_token()` | OAuth refresh per platform |

---

## 2. Common contract: the `publishes` table

From `vivify/db.py` L91-105:

| Column | Type | Source from platform API |
|--------|------|--------------------------|
| `id` | INTEGER PK | DB-generated, not from API |
| `episode_id` | INTEGER FK | Local (DB) |
| `platform` | TEXT | Local (one of `抖音`, `小红书`, `哔哩哔哩`) |
| `published_url` | TEXT | **Platform `share_url` / `video_url`** |
| `published_at` | TEXT ISO8601 | Platform's `create_time`, or local now() |
| `title` | TEXT | Local (passed in) — never overwritten by API |
| `description` | TEXT | Local |
| `hashtags` | TEXT | Local |
| `view_count` | INTEGER | Platform stats (refresh only) |
| `like_count` | INTEGER | Platform stats |
| `comment_count` | INTEGER | Platform stats |
| `completion_rate` | REAL | Platform stats (if available) |

> **Note:** there is no `platform_video_id` column. We derive it from
> `published_url` (last path segment) or store it in `description` as
> metadata. **Open question:** add a `platform_video_id TEXT` column
> in a new migration. See [§11](#11-open-questions-and-blockers).

---

## 3. Shared constraints and conventions

### 3.1 Common video constraints (assumed baseline; verify per platform)

| Constraint | Value | Source |
|------------|-------|--------|
| Codec | H.264 (AVC) baseline/main | Industry-standard, all 3 platforms |
| Container | MP4 | All 3 platforms |
| Max duration (抖音) | 15 min (regular), 60 min (verified) | `developer.open-douyin.com` |
| Max duration (小红书) | 5 min (regular), 15 min (pro) | `open.xiaohongshu.com` |
| Max duration (B站) | 4 GB / 12 hr (no hard cap below) | `openhome.bilibili.com` |
| Max file size (抖音) | 128 MB mobile, 4 GB web | `developer.open-douyin.com` |
| Max file size (小红书) | 5 GB | `open.xiaohongshu.com` |
| Max file size (B站) | 4 GB | `openhome.bilibili.com` |
| Aspect ratio (抖音) | 9:16 (vertical preferred) | Convention |
| Aspect ratio (小红书) | 3:4 or 1:1 (vertical preferred) | Convention |
| Aspect ratio (B站) | 16:9 (horizontal) | Convention |
| Audio | AAC-LC, 44.1 kHz or 48 kHz | All 3 platforms |

> The framework's render output is already MP4/H.264/AAC per
> `CLAUDE.md` tech stack. We do not need a transcode step. **Open
> question:** confirm `dar` (display aspect ratio) is preserved for
> each platform.

### 3.2 Three auth flavors

| Platform | Auth type | Token storage |
|----------|-----------|---------------|
| 抖音 | OAuth 2.0 (server-side, code→token) | `access_token` + `refresh_token` |
| 小红书 | Session cookie + X-S sign (web API only — no public OAuth) | Cookie jar + signing key |
| 哔哩哔哩 | Cookie + WBI sign (creator web) **or** OAuth 2.0 (open platform) | Both supported |

> **Critical:** 小红书 has **no public open API for video publishing** at
> the time of writing. All known implementations use the internal
> `edith.xiaohongshu.com` web API, which requires reverse-engineered
> request signing (`X-S`, `X-T`, `x-S-Common` headers via the
> `xhshow` library). See [§5](#5-platform-2-小红书-xiaohongshu--rednote)
> for details and [§11](#11-open-questions-and-blockers) for risk
> discussion.

### 3.3 HTTP client conventions

Use `httpx` (already in the project via `model_router.py` / `cost_cap.py`).
Use `requests-toolbelt.MultipartEncoder` for streaming large files. Do
NOT load the whole file into memory — Bilibili supports 4 GB.

For the publish skill, the call site lives in
`vivify/commands/publish.py::_upload_to_platform()`. The simplest
refactor is to add a `vivify/publish_adapters/` package:

```
vivify/publish_adapters/
├── __init__.py
├── base.py           # PlatformAdapter abstract class
├── douyin.py
├── xiaohongshu.py
└── bilibili.py
```

Each adapter exposes:

```python
class PlatformAdapter(Protocol):
    name: str                       # "抖音" / "小红书" / "哔哩哔哩"
    def upload(self, video_path: Path, meta: PublishMeta) -> UploadResult: ...
    def fetch_analytics(self, video_id: str) -> AnalyticsSnapshot: ...
    def check_status(self, video_id: str) -> str: ...   # "processing" | "live" | "rejected"
    def refresh_token(self) -> None: ...
```

`UploadResult` carries `video_id`, `public_url`, `raw_response`. The
CLI's `publish_cmd` becomes a thin shim around `adapter.upload()`.

---

## 4. Platform 1: 抖音 (Douyin)

> **Source:** `developer.open-douyin.com` (authoritative). Some
> secondary endpoints inferred from project comments and common
> community knowledge; marked accordingly.

### 4.1 Auth flow

抖音 uses standard OAuth 2.0 authorization-code flow with `client_key`
+ `client_secret`. There are **two token types**:

1. **`access_token`** (user-authorized) — needed for acting on behalf of
   a specific user (e.g., publishing to their account). 15-day TTL.
2. **`client_token`** (app-level) — needed for app-only operations.
   Refreshed separately.

#### 4.1.1 Step 1: authorize URL (user-facing)

The user must approve the app on `open.douyin.com` and get redirected
to your `redirect_uri` with a `code` query parameter. The exact
authorize URL is built per scope; for publishing the scope is
`video.create` (or `trial.video.create` for trial apps).

```
GET https://open.douyin.com/oauth/authorize/
    ?client_key={client_key}
    &response_type=code
    &scope=video.create,trial.video.create,user_info
    &redirect_uri={redirect_uri}
    &state={csrf_token}
```

> **Open question:** the project today runs headless. We have no
> browser-driven OAuth flow. Options:
> 1. Manual one-time setup: user pastes `code` into a config file.
> 2. Headless browser (Playwright) at first-run, then store token.
> 3. Pre-existing token in env (`DOUYIN_ACCESS_TOKEN`) — for testing only.
>
> Recommend option 1 for v1. See [§11](#11-open-questions-and-blockers).

#### 4.1.2 Step 2: exchange `code` → `access_token`

```
POST https://open.douyin.com/oauth/access_token/
Content-Type: application/x-www-form-urlencoded

client_key=...&client_secret=...&code=...&grant_type=authorization_code
```

**Response (HTTP 200, success):**

```json
{
  "data": {
    "error_code": 0,
    "description": "",
    "access_token": "act.xxx",
    "expires_in": 1296000,
    "refresh_token": "rft.xxx",
    "refresh_expires_in": 2592000,
    "open_id": "...",
    "scope": "user_info,trial.video.create,video.create",
    "log_id": "..."
  },
  "message": "success"
}
```

**TTL:** `access_token` = 15 days (1,296,000 s). `refresh_token` = 30 days.
Code is single-use, valid 10 minutes.

#### 4.1.3 Step 3: refresh `access_token`

```
POST https://open.douyin.com/oauth/refresh_token/
Content-Type: application/x-www-form-urlencoded

client_key=...&refresh_token=...&grant_type=refresh_token
```

Response shape is identical to step 2. The platform guarantees a
5-minute overlap window during rotation — refresh before expiry to
avoid race conditions.

#### 4.1.4 `client_token` (app-level, no user)

For app-only operations (e.g., checking quota, listing videos):

```
POST https://open.douyin.com/oauth/client_token/
Content-Type: application/x-www-form-urlencoded

client_key=...&client_secret=...&grant_type=client_credential
```

**Note:** `client_token` cannot be used to publish — that's user-token
territory.

#### 4.1.5 Error codes (auth endpoints)

| Code | Description | Action |
|------|-------------|--------|
| 10001 | System error | Retry with backoff |
| 10002 | Parameter error | Fix request |
| 10003 | Invalid `client_secret` | Permanent — surface to user |
| 10007 | Code expired (10 min) | Re-authorize |
| 10013 | Bad `client_key` / `client_secret` | Permanent |
| 10014 | `client_key` mismatch (re-used code) | Re-authorize |

### 4.2 Video upload API

抖音's open platform uses a **two-step** publish flow for creator
videos:

1. **Step A: upload media to the asset library** (so the file is hosted
   on 抖音's CDN and you get a `media_id`).
2. **Step B: create the video post** referencing that `media_id`.

This is the inverse of the "post-task" pattern documented at
`/docs/resource/zh-CN/dop/develop/openapi/video-management/posting-task/create-posting-task`,
which is for *brand* posting tasks (e.g., challenge campaigns) — not
regular creator publishing. **The creator flow uses a different
endpoint:** `https://open.douyin.com/video/create/`. This endpoint is
referenced in the project's own `_upload_to_platform` stub comment
(`vivify/commands/publish.py` L62) and is well-known from community
implementations. **Open question:** confirm the live endpoint path
against current `developer.open-douyin.com` documentation.

#### 4.2.1 Step A: upload media (multipart)

```
POST https://open.douyin.com/enterprise/media/upload/
Authorization: Bearer {access_token}
Content-Type: multipart/form-data

--boundary
Content-Disposition: form-data; name="media"; filename="episode.mp4"
Content-Type: video/mp4

<binary>
--boundary
Content-Disposition: form-data; name="media_type"
Content-Type: text/plain

VIDEO
--boundary--
```

**Response (success):**

```json
{
  "data": {
    "error_code": "0",
    "description": "",
    "media": {
      "media_id": "MEDIA_ID_HERE",
      "url_list": ["https://p3-sign.douyinpic.com/..."]
    }
  },
  "extra": {
    "error_code": "0",
    "description": "",
    "sub_error_code": "0",
    "logid": "...",
    "now": 1700000000000
  }
}
```

**Material limits (from `/tools-ability/material-management/upload-material-interface`):**
- Image: max 5 MB, formats `bmp/gif/png/jpeg/jpg`, length/width < 1500
- PDF: max 10 MB
- Video limits: **not in the public doc page**; assumed same as the
  consumer-side limit (4 GB web, 128 MB mobile). **Open question.**

#### 4.2.2 Step B: create video (JSON)

```
POST https://open.douyin.com/video/create/
Authorization: Bearer {access_token}
Content-Type: application/json

{
  "media_id": "MEDIA_ID_FROM_STEP_A",
  "title": "峰哥短剧 EP005",
  "description": "凌晨4点的城市,一只熊猫在扫地",
  "cover_tsp": 1.5,
  "micro_app_id": null,
  "poi_id": null,
  "at_users": [],
  "hashtags": ["panda", "治愈", "凌晨"],
  "privacy_setting": "public",
  "video_label": null,
  "is_preview": 0
}
```

> **Open question:** the exact JSON schema for `video/create/` is
> not in the publicly crawled pages. Field names above are inferred
> from community docs and the project stub. The implementer should
> verify against `developer.open-douyin.com` after auth is granted.

**Response (success):**

```json
{
  "data": {
    "error_code": 0,
    "description": "",
    "video_id": "v0d123g40000c0abc123def456ghi789",
    "share_id": "1234567890123456789"
  },
  "extra": {
    "error_code": 0,
    "description": "",
    "sub_error_code": 0,
    "sub_description": "",
    "logid": "...",
    "now": 1700000000000
  }
}
```

**Public URL pattern:**

```
https://www.douyin.com/video/{video_id}
```

### 4.3 Status query API

抖音 runs async moderation on uploaded videos. After `video/create/`
returns success, the video is in a `processing` or `review` state. To
check:

```
GET https://open.douyin.com/video/data/?access_token={access_token}&video_id={video_id}
```

**Response fields include:** `video_id`, `title`, `cover`, `create_time`,
`is_top`, `is_review`, `video_status` (one of `processing`, `reviewing`,
`published`, `blocked`).

**Open question:** exact field names not verified from the live docs
page (page is client-rendered SPA). The endpoint path is widely
referenced in the community and the project's own stub comment
(`publish.py` L82). Implementer must confirm.

### 4.4 Analytics API

```
GET https://open.douyin.com/data/external/item/stats/?access_token={access_token}&item_id={video_id}
```

**Response fields include:** `play_count` (= view_count),
`like_count`, `comment_count`, `share_count`, `download_count`,
`forward_count`, `duration`.

> **Note:** 抖音 exposes analytics through several endpoints
> (`/data/external/item/stats/`, `/data/external/data_base/`, etc.)
> with different aggregation levels. The single-item endpoint is the
> one to use here.

**Open question:** the exact analytics endpoint path needs
verification — the project stub references `/video/data/` but the
data-OpenAPI family uses `/data/external/...`. Likely both exist;
one is the consumer-facing alias, the other is the data-OpenAPI.

### 4.5 Error codes (top 10, after auth)

| Code | Description | Retry? |
|------|-------------|--------|
| 2100005 | Parameter error | No — fix request |
| 2100004 | System busy | Yes, backoff |
| 2118110 | Request too frequent (QPS exceeded) | Yes, backoff |
| 2110007 | `media_id` invalid | No |
| 2110008 | Video format unsupported | No — transcode |
| 2110009 | Video duration out of range | No — re-render |
| 2110010 | File size too large | No — compress |
| 2110011 | Video content policy violation | No — content review |
| 2110012 | Copyright check failed | No |
| 2190001 | Internal server error | Yes, backoff |

### 4.6 Rate limits

- **Auth endpoints:** ~20 QPS per `client_key`.
- **Upload endpoint:** QPS 10, daily quota 1,000 uploads.
- **Read endpoints (`/data/external/...`):** QPS 100, daily 10,000.
- **All endpoints have a "10000/day" global cap** unless the app
  negotiated a higher tier.

### 4.7 Constraints (verifiable)

| Constraint | Value | Source |
|------------|-------|--------|
| Format | MP4 / MOV | Community standard |
| Codec | H.264 (AVC) | Community standard |
| Max duration | 15 min (regular account) | `developer.open-douyin.com` overview |
| Max file size | 128 MB (mobile), 4 GB (web) | `developer.open-douyin.com` |
| Aspect ratio | 9:16 preferred | Convention |
| Min resolution | 720p (1280×720) | Convention |
| Bitrate | ≤ 30 Mbps | Convention |

### 4.8 Sandbox

抖音 provides a sandbox host: **`https://open-sandbox.douyin.com`**.
Swap the production domain to use it. Sandbox credentials are
issued separately by the platform team after the developer applies.
Sandbox supports the same endpoint paths as production but with
isolated data.

### 4.9 Status of "real" integration

| Step | Status |
|------|--------|
| Access to `developer.open-douyin.com` | ⛔ Sandbox pending — apply for trial |
| `client_key` / `client_secret` | ⛔ None in env |
| `access_token` | ⛔ None in env |
| Real `video/create/` call | ❌ Stub only |
| Real status poll | ❌ Stub only |
| Real analytics | ❌ Stub only |

---

## 5. Platform 2: 小红书 (Xiaohongshu / RedNote)

> **WARNING:** This is the most uncertain of the three integrations.
> 小红书 does not publish a public open API for video publishing as of
> 2026-06. The endpoints below are derived from the internal web API
> used by `edith.xiaohongshu.com`, which is what every community
> implementation (including MediaCrawler, f2, and the project's own
> stub comment) targets. **These endpoints can change without notice
> and require reverse-engineered request signing.**

### 5.1 Auth flow

小红书 has no public OAuth. The working approach is:

1. User logs into `xiaohongshu.com` via QR scan in a browser.
2. Cookies are extracted (`web_session`, `a1`, `webId`, etc.).
3. Every API request must be **signed** with a custom algorithm
   (`X-S`, `X-T`, `x-S-Common`, `X-B3-Traceid` headers), typically
   implemented via the open-source `xhshow` Python library.

#### 5.1.1 Login (one-time, manual)

```
1. Open https://www.xiaohongshu.com in a headless browser (Playwright).
2. User scans QR with phone.
3. Extract cookies from the browser context.
4. Save to local config (~/.config/vivify/xhs_cookies.json).
```

#### 5.1.2 Session validation (per-call)

```
GET https://edith.xiaohongshu.com/api/sns/web/v1/user/selfinfo
Headers:
  Cookie: web_session=...; a1=...; webId=...; ...
  X-S: <signed by xhshow>
  X-T: <unix-ms timestamp>
  x-S-Common: <signed by xhshow>
  X-B3-Traceid: <uuid>
```

**Response (success):**

```json
{
  "code": 0,
  "success": true,
  "msg": "成功",
  "data": {
    "user_id": "...",
    "nickname": "...",
    "red_id": "...",
    "gender": 0,
    "image": "..."
  }
}
```

**If cookies are stale (HTTP 461 or `code: -101`):** re-login.

#### 5.1.3 Token refresh

There is no refresh — when cookies expire, re-run the QR login. Cookie
lifetime is typically 30-90 days, depending on device fingerprinting.

### 5.2 Video note upload API

小红书 uses a **multi-step** upload with media pre-upload:

1. Pre-flight: get upload URL/token.
2. Upload video binary to the pre-flight URL.
3. Submit the note metadata referencing the uploaded file.

#### 5.2.1 Step A: pre-flight (get upload credentials)

```
GET https://edith.xiaohongshu.com/api/sns/v1/web/notes/upload/preload
Headers: <signed as in §5.1.2>
```

**Response:** upload token + URL (varies; **unknown** — requires
live observation). Historically, the pre-flight returns a JSON with
`uploadUrl` and `token`, and the next call is a multipart PUT to that
URL.

> **Open question:** the exact pre-flight schema has shifted multiple
> times (2024-2026). The implementer must capture a real session to
> confirm.

#### 5.2.2 Step B: upload binary

```
PUT {uploadUrl_from_preflight}
Content-Type: multipart/form-data
<binary video file>
```

> Some versions use `POST` with multipart, not `PUT`. **Verify.**

#### 5.2.3 Step C: publish note (JSON)

```
POST https://edith.xiaohongshu.com/api/sns/v1/web/notes
Headers: <signed>
Content-Type: application/json

{
  "type": "video",
  "title": "凌晨的治愈熊猫",
  "desc": "峰哥短剧 EP005",
  "video": {
    "file_id": "FROM_PRELOAD",
    "url": "FROM_PRELOAD",
    "duration": 58.0
  },
  "cover": {
    "file_id": "...",
    "url": "https://..."
  },
  "topics": ["panda", "治愈", "凌晨"],
  "privacy": { "type": "public" },
  "location": null,
  "user_info": null
}
```

> **Note:** the field names above are the *current consensus* from
> community reverse engineering. They have changed several times.
> Do not hard-code.

**Response (success):**

```json
{
  "code": 0,
  "success": true,
  "msg": "成功",
  "data": {
    "note_id": "abc123def456",
    "share_url": "https://www.xiaohongshu.com/explore/abc123def456"
  }
}
```

### 5.3 Status query API

小红书 has no public status query. The published note is either
visible (poll `GET https://www.xiaohongshu.com/explore/{note_id}` and
check for `not_found`) or in moderation (no public way to check).

**Workaround:** use the creator dashboard HTML scrape via Playwright
(creator.xiaohongshu.com). **Open question.**

### 5.4 Analytics API

The web-side metrics endpoint is internal. From MediaCrawler source
observations, the **read path** uses:

```
GET https://edith.xiaohongshu.com/api/sns/web/v1/note/{note_id}/metrics
```

Returns:

```json
{
  "code": 0,
  "data": {
    "view_count": 12345,
    "like_count": 678,
    "comment_count": 90,
    "collect_count": 12,
    "share_count": 5
  }
}
```

> **Note:** `view_count` may be reported with a 24-hour delay.
> `share_count` is the closest analog to 抖音's "forward count".
> `completion_rate` is **not exposed**.

### 5.5 Error codes (top 10)

| Code | Description | Retry? |
|------|-------------|--------|
| 0 | Success | n/a |
| -101 | Not logged in / cookie expired | No — re-login |
| -102 | Rate limit | Yes, backoff |
| 461 | Session invalid | No — re-login |
| 1001 | Parameter error | No — fix request |
| 1002 | Server error | Yes, backoff |
| 20001 | Content policy violation | No |
| 20002 | Video too large | No — re-encode |
| 20003 | Video duration out of range | No — re-render |
| 30001 | Signature invalid (X-S expired) | Yes — re-sign |

### 5.6 Rate limits

- **Publish:** 1-2 posts per minute, 30-50 per day (per account;
  exact value is **unknown** and varies by account trust score).
- **Read (metrics):** QPS 5, burst 10.
- **Strict risk-control:** if you publish >10/hour, the account may
  be silently throttled. Implement jitter (`time.sleep(random(60,180))`).

### 5.7 Constraints

| Constraint | Value | Source |
|------------|-------|--------|
| Format | MP4 / MOV | Community standard |
| Codec | H.264 / H.265 | Community standard |
| Max duration | 5 min (regular), 15 min (pro) | `open.xiaohongshu.com` overview |
| Max file size | 5 GB | `open.xiaohongshu.com` |
| Aspect ratio | 3:4 preferred (1080×1440) | Convention |
| Min resolution | 720p | Convention |

### 5.8 Sandbox

小红书 has **no public sandbox** for the publish API. Testing
requires either (a) a real account, or (b) the platform's creator
test environment (separate login, limited availability).

### 5.9 Status of "real" integration

| Step | Status |
|------|--------|
| Public OpenAPI for publish | ⛔ None exists |
| Internal web API (reverse-engineered) | ⚠️ Unstable — changes frequently |
| QR-login flow | ❌ Not implemented |
| Request signing (`xhshow`) | ❌ Not implemented |
| Real `POST /api/sns/v1/web/notes` | ❌ Stub only |
| Real status poll | ❌ Stub only |
| Real analytics | ❌ Stub only |

### 5.10 Risk note

**Implementing 小红书 is a 3-5x effort multiplier over 抖音** because
of the signing, cookie management, and the lack of stable public docs.
Recommend: ship 抖音 first, treat 小红书 as a stretch goal that
requires its own implementation pass.

---

## 6. Platform 3: 哔哩哔哩 (Bilibili)

> **Source:** `openhome.bilibili.com` (authoritative, but most pages
> require login to view). Some endpoints inferred from the
> community-maintained `bilibili-API-collect` documentation and
> `MediaCrawler` source code. **Note:** `SocialSisterYi/bilibili-API-collect`
> was archived in 2024-2025 following a takedown notice; the
> information below is reconstructed from cached versions and
> parallel community projects.

### 6.1 Auth flow

Bilibili has **two parallel systems**:

1. **Open platform OAuth 2.0** — for partner apps.
2. **Cookie-based creator web API** — what every community
   implementation uses (and what the project stub comments reference).

The web-creator API is the practical choice for our use case. It uses
**WBI signing** (an obfuscated mixin-key derived from two image URLs)
plus session cookies.

#### 6.1.1 Login (one-time, manual)

```
1. Open https://member.bilibili.com in a headless browser.
2. User scans QR with phone (or completes captcha).
3. Extract cookies (DedeUserID, SESSDATA, bili_jct, etc.).
4. Save to local config.
```

#### 6.1.2 Session validation

```
GET https://api.bilibili.com/x/web-interface/nav
Headers:
  Cookie: DedeUserID=...; SESSDATA=...; bili_jct=...
```

**Response (success, logged in):**

```json
{
  "code": 0,
  "message": "0",
  "ttl": 1,
  "data": {
    "isLogin": true,
    "uname": "峰哥发布",
    "mid": 12345678,
    "wbi_img": {
      "img_url": "https://i0.hdslb.com/bfs/wbi/xxxxxxxxxxxxxxxxxxxxx.png",
      "sub_url": "https://i0.hdslb.com/bfs/wbi/yyyyyyyyyyyyyyyyyyyyy.png"
    }
  }
}
```

The `wbi_img` keys are used to compute the `w_rid` (WBI mixin-key)
that every signed request needs.

#### 6.1.3 WBI signing algorithm

```
mixin_key = md5(img_key + sub_key)[0:32]   # truncate img_url/sub_url to last 32 chars
# Reorder mixin_key per published table (32 indices)
# For each API call:
#   1. Sort params alphabetically.
#   2. Strip '?' and '#' from values.
#   3. URL-encode (httpx-style).
#   4. Concatenate with mixin_key, md5 hash → w_rid.
#   5. Add w_rid to query string.
```

The exact 32-character reorder table is widely published; see the
`bilibili-API-collect` archive for the canonical version.

#### 6.1.4 Cookie / token refresh

No refresh — re-login when cookies expire. SESSDATA lifetime is
typically 30-90 days.

### 6.2 Video upload API (投稿)

Bilibili's publish flow is **three steps**:

1. **Step A: pre-upload** — get a `bucket` and `upload_id` from the
   upload server.
2. **Step B: upload binary** — multipart upload in chunks to the
   upload server.
3. **Step C: submit metadata** — finalize the submission with title,
   description, tags, etc.

#### 6.2.1 Step A: pre-upload

```
GET https://member.bilibili.com/preupload
       ?access_key=&mid=&profile=ugc%2Fface
Headers:
  Cookie: DedeUserID=...; SESSDATA=...; bili_jct=...
```

**Response (success):**

```json
{
  "OK": 1,
  "url": "https://upos-sz-upcdnbpcx2.bilivideo.com/",
  "complete": "https://upos-sz-upcdnbpcx2.bilivideo.com/...",
  "chunk_size": 4194304,
  "length": 12345678,
  "bucket": "ugc",
  "key": "...",
  "upload_id": "..."
}
```

> **Note:** this is the legacy endpoint. Modern versions may use
> `https://member.bilibili.com/v2/upload/web/get_upload_id` or
> similar. The `v2/upload/video` endpoint mentioned in the project
> stub (`publish.py` L64) is the meta-submit endpoint (step C),
> not the binary upload.

#### 6.2.2 Step B: upload binary (multipart, chunked)

```
POST https://upos-sz-upcdnbpcx2.bilivideo.com/{key}?upload_id=...&chunk={N}&chunks={TOTAL}&size={CHUNK_SIZE}&start={OFFSET}
Content-Type: application/octet-stream
Body: <raw bytes for chunk N>
```

Repeat for each chunk (4 MB chunks typical). After the last chunk:

```
POST https://upos-sz-upcdnbpcx2.bilivideo.com/{key}?output=json&name={key}&profile=ugc%2Fface&upload_id=...
Content-Type: application/x-www-form-urlencoded

files=<final-blob-info>
```

**For files < 4 MB**, a single PUT works:

```
PUT https://upos-sz-upcdnbpcx2.bilivideo.com/{key}?upload_id=...
```

#### 6.2.3 Step C: submit metadata

```
POST https://member.bilibili.com/v2/upload/video/edit
Headers:
  Cookie: DedeUserID=...; SESSDATA=...; bili_jct=...
  Content-Type: application/json

{
  "title": "峰哥短剧 EP005",
  "description": "凌晨4点的城市,一只熊猫在扫地",
  "tag": "panda,治愈,凌晨",
  "tid": 21,                    # 分区: 21 = 日常
  "cover": "https://...",       # optional
  "videos": [
    { "filename": "<server key from step B>", "title": "EP005" }
  ],
  "no_reprint": 1,
  "open_elec": 0,
  "up_close_reply": 0,
  "up_close_danmu": 0
}
```

> `tid` is the partition ID. Common values:
> 1 (动画), 13 (番剧), 17 (单机), 21 (日常), 24 (MAD·AMV),
> 25 (MMD·3D), 26 (短片·手书·配音), 27 (综合), 47 (鬼畜),
> 119 (资讯), 129 (舞蹈·HIPHOP), 130 (音乐综合), 138 (影视),
> 160 (生活), 181 (影视综合), 188 (科技), 191 (汽车).
>
> 短剧 (short drama) typically lives in tid 21 (日常) or 181.

**Response (success):**

```json
{
  "code": 0,
  "message": "0",
  "data": {
    "aid": 17000000001,
    "bvid": "BV1xx411c7mD"
  }
}
```

**Public URL pattern:**

```
https://www.bilibili.com/video/av{aid}    # legacy, still works
https://www.bilibili.com/video/{bvid}     # current
```

### 6.3 Status query API

```
GET https://api.bilibili.com/x/web-interface/view?bvid={bvid}
Headers:
  Cookie: ...
  (WBI signed)
```

**Response includes:** `bvid`, `aid`, `title`, `desc`, `duration`,
`pubdate` (Unix s), `state` (0 = normal, -1 = censored, -2 = locked,
-3 = review blocked, -4 = admin deleted, -5 = self deleted, -6 =
awaiting review, -7 = needs re-upload, -8 = appeal pending,
-9 = appeal denied, -10 = jail, -11 = rule violation, -12 =
trial-passed review).

For the creator workflow, there's also:

```
GET https://member.bilibili.com/v2/upload/video/submitted
```

Which lists recent submissions with their current state.

### 6.4 Analytics API

```
GET https://api.bilibili.com/x/web-interface/view?bvid={bvid}
```

The view detail response itself contains stats:

```json
{
  "code": 0,
  "data": {
    "stat": {
      "aid": 17000000001,
      "view": 12345,
      "danmaku": 67,
      "reply": 23,
      "favorite": 89,
      "coin": 12,
      "share": 5,
      "like": 234,
      "dislike": 0,
      "evaluation": ""
    }
  }
}
```

Mapping to `publishes` table:

- `view_count` ← `data.stat.view`
- `like_count` ← `data.stat.like`
- `comment_count` ← `data.stat.reply`
- `completion_rate` ← not exposed by API. Compute client-side if
  `duration` is known: a custom hook can read events from a tracking
  pipeline. For v1, leave `NULL`.

> For higher-fidelity metrics (watch time, retention), the
> `data-up/heartbeat` endpoint exists, but requires the creator's
> `up_session` and is not part of the public API surface.

### 6.5 Error codes (top 10)

| Code | Description | Retry? |
|------|-------------|--------|
| 0 | Success | n/a |
| -101 | Not logged in | No — re-login |
| -102 | App revoked / banned | No — surface |
| -111 | CSRF token (bili_jct) mismatch | No — re-login |
| -352 | Risk control (signature expired) | Yes — re-sign |
| -400 | Request error | No — fix |
| -403 | Forbidden | No |
| -509 | Rate limit | Yes, backoff |
| 16000 | Tag invalid (too long, banned) | No — fix |
| 22001 | Title contains banned word | No — fix |
| 22002 | Description contains banned word | No — fix |

### 6.6 Rate limits

- **Submit (`/v2/upload/video/edit`):** ~30 QPS per `mid`, daily
  quota 100-500 uploads (varies by account level).
- **Read endpoints:** QPS 5 (with WBI), bursts up to 20.
- **Chunk upload:** no practical limit (CDN).
- **Auth:** the `SESSDATA` cookie is rate-limited; >100 read calls
  in 5 min may trigger captcha.

### 6.7 Constraints

| Constraint | Value | Source |
|------------|-------|--------|
| Format | MP4 | `openhome.bilibili.com` |
| Codec | H.264 (AVC) / H.265 (HEVC) / AV1 | `openhome.bilibili.com` |
| Max duration | No hard cap (12 hr typical) | Community standard |
| Max file size | 4 GB | `openhome.bilibili.com` |
| Aspect ratio | 16:9 preferred | Convention |
| Min resolution | 720p (1280×720) | `openhome.bilibili.com` |
| Frame rate | 24 / 30 / 60 fps | `openhome.bilibili.com` |
| Audio | AAC, ≤ 320 kbps | `openhome.bilibili.com` |

### 6.8 Sandbox

Bilibili's open platform has a sandbox at `https://member.bilibili.com/v2/upload/video`
(operationally, the same endpoint accepts trial uploads from sandbox
credentials). It is **not** a separate domain. Test uploads go through
the same flow but with a `trial=1` query flag or a sandbox-only `tid`.

### 6.9 Status of "real" integration

| Step | Status |
|------|--------|
| Open platform OAuth credentials | ⛔ None in env |
| Cookie-based session | ❌ Not implemented |
| WBI signing | ❌ Not implemented |
| Pre-upload + chunked upload | ❌ Stub only |
| `/v2/upload/video/edit` (submit) | ❌ Stub only |
| Real status poll | ❌ Stub only |
| Real analytics | ❌ Stub only |

---

## 7. Mapping platform responses → `publishes` table

### 7.1 抖音 → `publishes`

```yaml
upload_response:
  data.video_id:     → derived share_id (parsed from published_url)
  data.share_id:     → stored as platform_video_id metadata in description (see §11)
  public URL:        → published_url
                     = "https://www.douyin.com/video/{video_id}"

analytics_response:
  play_count:        → view_count
  like_count:        → like_count
  comment_count:     → comment_count
  completion_rate:   → completion_rate (computed by API; not always exposed)
```

### 7.2 小红书 → `publishes`

```yaml
upload_response:
  data.note_id:      → derived share_id
  data.share_url:    → published_url

analytics_response:
  view_count:        → view_count
  like_count:        → like_count
  comment_count:     → comment_count
  share_count:       → not stored (no column); consider adding
  collect_count:     → not stored (no column); consider adding
  completion_rate:   → NULL (not exposed)
```

### 7.3 Bilibili → `publishes`

```yaml
upload_response:
  data.bvid:         → stored as platform_video_id metadata
  data.aid:          → not stored
  public URL:        → published_url
                     = "https://www.bilibili.com/video/{bvid}"

analytics_response (from /x/web-interface/view?bvid=...):
  data.stat.view:    → view_count
  data.stat.like:    → like_count
  data.stat.reply:   → comment_count
  data.stat.share:   → not stored (no column)
  completion_rate:   → NULL (not exposed)
```

### 7.4 Missing columns in `publishes` (proposed migration 003)

The current schema (L91-105 of `vivify/db.py`) lacks:

- `platform_video_id` — for retry / status-poll.
- `share_count` — for 抖音/B站 reach metrics.
- `collect_count` — for 小红书 save metrics.
- `upload_status` — for tracking moderation (live / review / blocked).
- `last_analytics_at` — to know when refresh last ran.

**Recommendation:** add migration `003_publishes_extended.sql` once
the integration moves beyond stub. See [§11](#11-open-questions-and-blockers).

---

## 8. Retry strategy

### 8.1 Per-platform backoff

| Failure | 抖音 | 小红书 | B站 |
|---------|------|--------|-----|
| 401 (token expired) | Refresh token, retry once | Re-login (manual or headless), retry once | Refresh SESSDATA, retry once |
| 403 (forbidden) | No retry — surface | No retry — surface | No retry — surface |
| 429 (rate limit) | Backoff 5s, 15s, 45s | Backoff 60s, 180s, 600s (stricter) | Backoff 5s, 15s, 45s |
| 5xx (server error) | Backoff 5s, 15s, 45s | Backoff 5s, 15s, 45s | Backoff 5s, 15s, 45s |
| Timeout | Backoff 5s, 15s, 45s | Backoff 5s, 15s, 45s | Backoff 5s, 15s, 45s |
| 4xx (client error) | No retry — fix request | No retry — fix request | No retry — fix request |
| Sign error (-352 / X-S) | n/a | Re-sign, retry | Re-sign, retry |

### 8.2 Reuse `vivify/retry.py`

The project already has `RetryPolicy(max_retries=3, base_delay_sec=5.0, backoff_factor=3.0)`,
yielding delays of 5s, 15s, 45s. For 小红书, override with a
stricter policy:

```python
XHS_RETRY = RetryPolicy(max_retries=2, base_delay_sec=60.0, backoff_factor=3.0)
# 60s, 180s
```

Reuse `classify_error()` for non-platform errors (network, timeout).
For platform-specific error codes, implement a per-adapter
`classify_platform_error(response) → FailureClass` method.

### 8.3 Token refresh handling

- 抖音: refresh proactively when token is < 5 days old. Reactive
  refresh on 401 is fine but loses a request.
- 小红书: no refresh — re-login is the only path.
- B站: SESSDATA refresh = re-login. `bili_jct` rotates per session;
  keep it fresh.

### 8.4 Idempotency

抖音 and B站 do not guarantee idempotency on duplicate publish calls.
If a request times out after the server accepted it, retrying will
create a duplicate post. **Mitigation:**

- Use a stable `client_video_id` (UUID stored in DB) as a request
  tag where supported.
- For B站: set `videos[].title` to include a per-episode stable
  hash; dedupe via `member.bilibili.com/v2/upload/video/submitted`
  before retrying.
- For 抖音: implement a `dedup_key` field if the API exposes one
  (open question — verify on first integration).

---

## 9. Minimum viable integration per platform

### 9.1 抖音 MVI

```python
# Minimum to ship publish + analytics for 抖音:

async def upload(self, video_path, meta):
    # 1. POST /oauth/access_token/  (one-time, cached)
    # 2. POST /enterprise/media/upload/  (multipart, file)
    # 3. POST /video/create/  (JSON, media_id + meta)
    # 4. Return UploadResult(video_id=..., public_url=...)

async def fetch_analytics(self, video_id):
    # GET /data/external/item/stats/?item_id=...
    # Return AnalyticsSnapshot(view, like, comment)

async def check_status(self, video_id):
    # GET /video/data/?video_id=...
    # Return "processing" | "live" | "blocked"
```

**Effort:** 2-3 days. **Blocker:** need `client_key` + `client_secret`
+ initial `code` (manual or QR).

### 9.2 小红书 MVI

```python
async def upload(self, video_path, meta):
    # 1. Validate cookies via /api/sns/web/v1/user/selfinfo
    # 2. GET /api/sns/v1/web/notes/upload/preload
    # 3. PUT (or POST) the file to uploadUrl
    # 4. POST /api/sns/v1/web/notes (JSON, with file_id)
    # 5. Return UploadResult(video_id=..., public_url=...)

async def fetch_analytics(self, note_id):
    # GET /api/sns/web/v1/note/{note_id}/metrics
    # Return AnalyticsSnapshot(view, like, comment)
```

**Effort:** 1-2 weeks (signing, cookie management, schema volatility).
**Blocker:** none — uses our own account — but high risk of breakage.

### 9.3 B站 MVI

```python
async def upload(self, video_path, meta):
    # 1. Validate cookies via /x/web-interface/nav
    # 2. GET /preupload (signed with WBI)
    # 3. PUT binary in chunks to upload server
    # 4. POST /v2/upload/video/edit (JSON, with file key)
    # 5. Return UploadResult(video_id=..., public_url=...)

async def fetch_analytics(self, bvid):
    # GET /x/web-interface/view?bvid=...
    # Return AnalyticsSnapshot(view, like, comment)
```

**Effort:** 1-2 weeks. **Blocker:** QR-login flow + WBI signing
must be implemented first.

### 9.4 Sequencing

1. **First:** 抖音 (smallest scope, best docs).
2. **Second:** B站 (clearer flow, but more signing work).
3. **Third:** 小红书 (highest risk; revisit after 抖音 is stable).

---

## 10. Environment variables and secrets layout

Add to `vivify/db.py` env resolution (or a new `vivify/secrets.py`):

```bash
# 抖音
DOUYIN_CLIENT_KEY=...
DOUYIN_CLIENT_SECRET=...
DOUYIN_ACCESS_TOKEN=...         # optional, for testing
DOUYIN_REFRESH_TOKEN=...        # optional
DOUYIN_OPEN_ID=...              # optional

# 小红书 (cookie-based)
XHS_COOKIES_JSON=...            # JSON blob or path to file
# OR
XHS_COOKIES_FILE=~/.config/vivify/xhs_cookies.json

# B站 (cookie-based)
BILI_COOKIES_JSON=...
# OR
BILI_COOKIES_FILE=~/.config/vivify/bili_cookies.json
```

Tokens are stored in `publishes.platform_token` (new column) or in
a sidecar file under `~/.config/vivify/tokens.json` keyed by
`(platform, account)`. **Do not** commit tokens to git; the existing
`opc.db-wal` artifact suggests the project already runs locally.

---

## 11. Open questions and blockers

### 11.1 Schema

1. **Add `platform_video_id` column?** Currently we parse it from
   `published_url`. Cleaner to store it. **Suggested migration:**
   `003_publishes_extended.sql` adding:
   - `platform_video_id TEXT`
   - `share_count INTEGER DEFAULT 0`
   - `collect_count INTEGER DEFAULT 0`
   - `upload_status TEXT DEFAULT 'live'`   -- 'pending' | 'live' | 'blocked' | 'rejected'
   - `last_analytics_at TEXT`
2. **Add `account_id` column?** If we ever want to publish from
   multiple accounts (per IP), we need it. **Defer to v2.**

### 11.2 抖音

3. **Exact `video/create/` schema** is not in the public SPA-rendered
   docs. The project stub comment names `/video/create/` and
   `/video/data/`. Confirm via developer-portal login before coding.
4. **Material (video) upload endpoint path:** the public docs page
   surfaces `POST /enterprise/media/upload/` and
   `POST /tools-ability/material-management/upload-material-interface`.
   The enterprise variant is widely used in community SDKs. The two
   may differ in required scopes. **Verify which one our app's
   `client_key` has access to.**
5. **Analytics endpoint naming:** `/video/data/` (legacy) vs
   `/data/external/item/stats/` (data-OpenAPI). **Pick one and
   stick to it.**

### 11.3 小红书

6. **The whole pipeline is unsupported.** The signing algorithm
   (`xhshow`) is reverse-engineered and can change. Risk: weeks of
   work can break overnight.
7. **QR-login automation** requires Playwright + a phone. This
   breaks pure-CLI deployment. **Mitigation:** ship 抖音 first; treat
   小红书 as best-effort.
8. **Pre-flight schema changes** have happened at least 3 times in
   2024-2026. **Implementation must be defensive** (parse multiple
   response shapes).
9. **No sandbox.** All testing is on a real account, with real risk
   of being flagged for automated publishing. **Mitigation:** jitter
   + low publish rate.

### 11.4 B站

10. **Open platform OAuth** vs **cookie-based** auth. The OAuth
    variant has better documentation but requires an approved partner
    application. Cookie-based works with any account but is
    reverse-engineered. **Recommend:** cookie-based for v1 (faster
    path), OAuth for v2 (long-term).
11. **WBI signing** is well-documented but tedious. **Plan:** a
    single 100-line `wbi.py` module, tested against
    `https://api.bilibili.com/x/web-interface/nav` round-trip.
12. **Chunked upload** is complex. For our use case (typical output
    50-100 MB), a single PUT works (chunk_size = file size). **Defer
    chunked implementation unless we hit the 4 GB cap.**
13. **`tid` (partition) selection** is manual. For 短剧 (short drama),
    `tid=21` (日常) is the default. **Add to the publish metadata
    as a CLI flag `--bili-tid 21`.**

### 11.5 Operational

14. **Refresh + retry orchestration** is currently the CLI's
    responsibility. **Consider:** an `vivify/publish_adapters/queue.py`
    that uses a small SQLite queue + worker for retries. **Defer
    until analytics refresh is in production.**
15. **Token storage** is currently absent (no env vars). **Plan:**
    `vivify/secrets.py` that reads from env first, then from
    `~/.config/vivify/tokens.json`. **Use the OS keyring as a v2
    option.**
16. **Notification on moderation rejection.** When upload_status
    becomes `blocked` or `rejected`, we want to alert the user.
    **Hook point:** `vivify publish refresh` could detect this and
    write to a `lessons` row. **Defer.**

### 11.6 Missing fields in the existing stub

17. `_upload_to_platform` does not currently return a `video_id` —
    only `published_url`. **Required change:** the stub's return
    shape must include `video_id` so the DB can store it. Add
    `platform_video_id` column at the same time.

---

## 12. Sources

### 12.1 Official (authoritative)

- 抖音开放平台 docs: `https://developer.open-douyin.com/docs/resource/zh-CN/dop/develop/openapi/list`
  - Upload material: `…/tools-ability/material-management/upload-material-interface`
  - Create posting task: `…/video-management/posting-task/create-posting-task`
  - Get access_token: `…/account-permission/get-access-token`
  - Refresh access_token: `…/account-permission/refresh-access-token`
  - Sandbox: `https://open-sandbox.douyin.com`
- 哔哩哔哩开放平台: `https://openhome.bilibili.com` (login-gated)
- 小红书开放平台: `https://open.xiaohongshu.com` (login-gated; limited
  public docs for video publishing)
- Bilibili API root: `https://api.bilibili.com/x/web-interface/nav`
  (live response shape observed; auth check only)

### 12.2 Community (reverse-engineered / deprecated)

- `SocialSisterYi/bilibili-API-collect` (archived 2024; cached
  versions only) — formerly `docs/video/videoup.md`
- `NanmiCoder/MediaCrawler` — `media_platform/{douyin,xhs,bilibili}/`
  — observation of `client.py` + `core.py` API endpoint usage
- `Johnserf-Seed/f2` — `f2/apps/{douyin,xhs,bilibili}/api.py` — web
  scraping endpoints (not the open platform, but informative for
  cookie-based auth)

### 12.3 Project-internal

- `vivify/db.py` L91-105 — `publishes` table schema
- `vivify/commands/publish.py` L57-94 — current stub implementation
- `vivify/retry.py` — backoff policy (5s/15s/45s, 3 retries)
- `skills/vivify-panda-episode-publish/SKILL.md` L88-91 — endpoint
  hints from the project author (matches our research):
  - 抖音: `https://open.douyin.com/video/create/`
  - 小红书: `https://edith.xiaohongshu.com/api/sns/v1/web/notes`
  - B站: `https://member.bilibili.com/v2/upload/video`
- `skills/vivify-scene-decomposition/SKILL.md` — platform-specific
  duration/aspect hints
- `docs/CLAUDE.md` — tech stack confirmation (MP4/H.264/AAC output)

### 12.4 What we tried but could not confirm

The following pages exist but are SPA-rendered (content is loaded by
JavaScript) and could not be fully extracted via plain `WebFetch`:

- `https://developer.open-douyin.com/docs/resource/zh-CN/dop/develop/openapi/video-management/douyin/video-create/...`
  (multiple candidates; exact path to creator video-create API unknown)
- `https://openhome.bilibili.com/docs/4/...` (login-gated)
- `https://open.xiaohongshu.com/document/...` (login-gated)

For these, the implementer must log in to the developer portal and
re-verify before coding.

---

## Appendix A: Quick reference — endpoint summary

| Platform | Auth (host) | Upload (host) | Submit/Publish | Status | Analytics |
|----------|-------------|---------------|----------------|--------|-----------|
| 抖音 | `open.douyin.com/oauth/access_token/` | `open.douyin.com/enterprise/media/upload/` | `open.douyin.com/video/create/` | `open.douyin.com/video/data/` | `open.douyin.com/data/external/item/stats/` |
| 小红书 | (cookie + X-S sign, edith.xiaohongshu.com) | `edith.xiaohongshu.com/api/sns/v1/web/notes/upload/preload` → upload URL | `edith.xiaohongshu.com/api/sns/v1/web/notes` | (none) | `edith.xiaohongshu.com/api/sns/web/v1/note/{id}/metrics` |
| B站 | (cookie + WBI sign, api.bilibili.com) | `member.bilibili.com/preupload` → CDN upload | `member.bilibili.com/v2/upload/video/edit` | `api.bilibili.com/x/web-interface/view` | same as status |

## Appendix B: Response envelopes (cheat-sheet)

All three platforms use a similar envelope, but the field names differ.

| Platform | Success field | Error field | Status location |
|----------|---------------|-------------|-----------------|
| 抖音 | `data.error_code == 0` | `data.error_code` (string or int) | top-level `message` |
| 小红书 | `code == 0` and `success == true` | `code` (negative int) | top-level `msg` |
| B站 | `code == 0` | `code` (negative int) | top-level `message` |

Implement a single `parse_envelope(response, platform) → dict` helper
that normalizes to a `vivify.publish_adapters.base.Envelope` with
`ok: bool`, `platform_code: int`, `platform_message: str`, `data: dict`.

---

*End of spec. Next step: pick a platform (recommend 抖音), request
developer access, then implement adapter following §4 + §7 + §9.1.*
