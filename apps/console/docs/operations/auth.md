---
title: 鉴权
description: OPC 控制台与 skill API 的鉴权机制 — session cookie 体系、登录端点、续期策略与跨域注意事项。
order: 1
---

OPC 的所有 skill API、文档站以及未来的客户端 SDK 都使用同一套基于 **HTTP-only cookie 的会话体系**。本节说明这套机制的形态、生命周期,以及从 MCP 客户端接入时需要关心的细节。

## 鉴权方式: `opc_session` cookie

OPC 鉴权最终落到浏览器 (或 MCP 客户端进程) 上的一个 cookie:

- **名称**: `opc_session`
- **类型**: `HttpOnly` —— 禁止 JS 读取, 防止 XSS 偷 cookie
- **Secure**: 生产强制, 本地 dev 可关
- **SameSite**: `Lax` —— 允许顶层 GET 跨站携带, 但表单 POST 等敏感动作必须同站发起
- **Path**: `/` —— 全站生效, 不区分 skill

cookie 内容本身是**不透明**的: 它是一段被服务端签名的 session id, 不直接存放用户身份或权限信息。任何客户端都不应该解析它, 也不应该手工拼装。续期、撤销、轮转全部由后端控制。

## 登录: `POST /api/auth/login`

OPC 使用经典的 email + password 登录, 没有走 OAuth 也没有 magic-link, 目的是让单用户 / 小团队私有部署**零外部依赖**就能跑起来。

**请求**:

```http
POST /api/auth/login
Content-Type: application/json
```

```json
{
  "email": "operator@example.com",
  "password": "your-password-here"
}
```

**成功响应 (200)**:

```json
{
  "user": {
    "id": "u_abc123",
    "email": "operator@example.com",
    "role": "admin"
  }
}
```

`Set-Cookie: opc_session=...; HttpOnly; SameSite=Lax; ...` 会随响应一起下发, 浏览器自动接管。

**失败响应**:

- `401`: 邮箱或密码错误。响应体里**不区分**这两种情况, 避免账号枚举。
- `429`: 同一 IP 在一分钟内尝试次数过多, 临时封禁 (见限流文档)。
- `400`: 请求体格式不合法 (缺字段、不是 JSON、字段类型错)。

**当前会话查询**: `GET /api/auth/me` —— 用于前端在每次加载时确认 session 还有效。返回 200 + user 对象, 或者 401 让前端跳回登录页。

**登出**: `POST /api/auth/logout` —— 服务端清除服务端 session 记录并下发一条过期的 `opc_session` cookie, 之后该 cookie 即便被冒用也不再有效。

## 会话有效期与续期策略

OPC 的 session 寿命是**滑动 + 上限**的混合策略:

- **绝对寿命 (硬上限)**: 14 天, 无论中间怎么活跃, 超过就必须重新登录。
- **空闲超时**: 7 天, 期间没有任何请求就过期。
- **滑动续期**: 任何**成功通过鉴权**的请求, 只要 session 还剩余寿命的 1/3 以内, 就会在响应里**追加一条新的 `Set-Cookie`**, 把硬上限和空闲超时都重新拉满。

这意味着只要你在使用中, cookie 永远不会在操作中途突然失效; 但长期挂着的客户端也会被强制要求重登。

### 续期在客户端的体感

成功请求 → 检查 `Set-Cookie` 头 → 浏览器自动替换 cookie。客户端代码**无需做任何事**。唯一的注意点是: 如果你用 `fetch` 走 `credentials: 'include'`, 跨域时**不能**自己手写 `Cookie` 头, 让浏览器根据 `Set-Cookie` 自动维护即可。

### 续期失败会怎样?

session 过期后, 受保护接口会返回 `401 Unauthorized`, 响应体形如:

```json
{ "error": "session_expired" }
```

前端控制台 (apps/console) 的 fetch 封装会自动跳到 `/login`。MCP 客户端需要自己监听 401 状态码并提示用户重新登录。

## 跨域 cookie 注意事项

OPC 假设**控制台 UI 和 skill API 部署在同一个一级域名下** (例如 `app.example.com` 和 `api.example.com` —— 这两个不算同站, cookie 默认不会跨域带上)。这种情况下需要额外配置:

### 1. `SameSite=None; Secure`

如果前端和 API 是**真跨域** (不同的 eTLD+1), `opc_session` 必须设成 `SameSite=None; Secure` 才能被跨站请求带上。生产环境必须走 HTTPS, 否则浏览器直接拒绝该 cookie。

### 2. CORS 预检

`/api/auth/login` 等端点需要后端显式放行:

- `Access-Control-Allow-Origin`: 限定为前端域名, **不要**写 `*`
- `Access-Control-Allow-Credentials: true`
- `Access-Control-Allow-Headers`: 至少包含 `Content-Type`
- `Access-Control-Allow-Methods`: 至少 `GET, POST, OPTIONS`

### 3. 客户端 fetch 必填项

```ts
fetch('https://api.example.com/api/skills/...', {
  credentials: 'include', // 关键: 允许跨域发送 cookie
  headers: { 'Content-Type': 'application/json' },
  // 注意: 不要在这里写 Cookie 头, 让浏览器自动从 opc_session 里取
})
```

### 4. 反向代理兜底

生产环境里**更推荐**的做法是用 nginx / caddy 把 `app.example.com/api/*` 反代到 `127.0.0.1:3000/api/*`, 这样前端和后端就变成**同源**, cookie 走默认的 `Lax` 就够了, 跨域配置整套可以省掉。本地开发 (`pnpm dev`) 默认就是这种同源模式。

## MCP 客户端接入

MCP 客户端走 stdio 调 OPC 的 skill 端点, 走的是普通的 HTTP fetch。和浏览器不同的是, 客户端进程**没有浏览器的 cookie jar 自动管理**, 有两种处理方式:

### 方式 A: 把 cookie 当 Bearer (推荐)

第一次登录拿到 cookie 之后, 客户端从 `Set-Cookie` 里解析出 `opc_session=...` 这段, 在后续所有请求里手动加上头:

```ts
const sessionCookie = 'opc_session=eyJ1c2VySWQiOi...'

await fetch('https://api.example.com/api/skills/script.generate', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'Cookie': sessionCookie,
  },
  body: JSON.stringify({ topic: '...' }),
})
```

session 过期 (401) 时, 客户端要重新跑一次 `POST /api/auth/login` 拿新 cookie。

### 方式 B: 起一个本地持久 cookie jar

更稳的做法是引入一个内存 + 持久化 (例如 `~/.opc/session.json`) 的 cookie jar, 把 `Set-Cookie` 头解析后存起来, 每次请求自动带上。这样续期、过期、重登都由客户端自己管, 体验和浏览器一致。

OPC 官方 SDK (在 `apps/console/lib/api-client.ts` 同一套) 默认会提供这个 cookie jar 的实现, MCP 集成时直接复用即可。

## Provider resolution (Phase 4)

每个 AI 技能在执行时, 上游 LLM provider **不再**由环境变量决定, 而是按调用方配置的凭据当场解析:

1. 在 `/credentials` 页面(或 `POST /api/credentials`)新增一条凭据, 选择 `provider=anthropic` 或 `provider=openai`, 填入 protocol-specific 字段(`api_key`、`model_name`、可选 `base_url`)。
2. 凭据以 **AES-GCM** 加密存储在 `credentials` 表, 密钥来自 `ENCRYPTION_KEY` 环境变量。
3. 每次请求到达后, 解析器 (`internal/agents/resolver.go`) 读取调用方对应的行, 解密 key, 按 `protocol` 字段挂载对应的协议适配器 (Anthropic Messages API 或 OpenAI Chat Completions API)。
4. **Key 轮转下次请求即生效** — 不需要重启服务, 也不需要改任何代码。

这意味着 MCN 运营方在同一个 OPC 部署里可以为不同 operator 配不同 provider (例如一部分用 Anthropic Sonnet、一部分用 GPT-4o-mini、一部分指向 MiniMax), `/credentials` 是控制面, `ProviderResolver` 是后端执行点。

### MiniMax 兼容示例

MiniMax 提供 Anthropic 协议兼容的文本接口。配置一条凭据即可让所有 AI 技能走它:

| 字段 | 值 |
|------|----|
| `name` | MiniMax (Anthropic 兼容) |
| `provider` | `anthropic` |
| `protocol` | `anthropic` |
| `model_name` | `MiniMax-M2.7-highspeed` |
| `base_url` | `https://api.minimaxi.com` |
| `api_key` | `<your MiniMax key>` |

媒体类技能 (封面、TTS、视频) 仍走 MiniMax 自带客户端, **不**经过本解析器 — 文本类 skill (选题 / 人化 / 评分 / 拆解 / 平台适配 / 公式 / pipeline / batch) 才会按上述凭据解析。
