---
title: 错误码
description: OPC 的 HTTP 错误码体系 — 4xx / 5xx 分类、错误响应统一格式、上游供应商错误的映射规则。
order: 3
---

OPC 所有 skill 端点、所有管理端点的错误响应都遵循**同一套** HTTP 状态码 + JSON 响应体约定。这样客户端只需要写一套错误处理逻辑, 就能覆盖全站。

## 4xx 通用: 客户端错

4xx 表示**请求本身有问题**, 客户端**改请求**就能解决, 重试没有意义 (除了 429)。

### `400 Bad Request` — 参数不合法

请求体格式、字段类型、必填校验失败。

```json
{
  "error": "invalid_argument",
  "details": {
    "field": "topic",
    "reason": "must be at least 5 characters"
  }
}
```

**客户端做法**: 读 `details.field` 高亮表单对应字段; `details.reason` 给人看。

### `401 Unauthorized` — 未登录

session 缺失或过期。响应体里 `error` 区分两种:

- `not_authenticated`: 完全没带 `opc_session` cookie
- `session_expired`: cookie 有, 但服务端已失效 (空闲超时 / 硬上限)

```json
{ "error": "session_expired" }
```

**客户端做法**: 跳登录页。MCP 客户端要重新走一次 login 流程。

### `403 Forbidden` — 没权限

session 有效, 但当前用户**没有这个 skill / 端点的访问权**。典型场景: 普通成员访问仅 admin 可用的 `credentials.rotate`。

```json
{
  "error": "forbidden",
  "details": { "requiredRole": "admin" }
}
```

**客户端做法**: 显示"无权限"提示页, **不要**自动重试。

### `404 Not Found` — 资源不存在

请求路径里的资源 id (skill id、credential id、user id) 找不到。

```json
{
  "error": "not_found",
  "details": { "resource": "skill", "id": "topic.summarize" }
}
```

**客户端做法**: 检查 id 拼写; 资源被删了要回退到列表页。

### `409 Conflict` — 资源冲突

典型的"已经存在"或"状态不允许"。例如:

- 创建同名 credential
- 删除一个正在被 pipeline 引用的 skill
- 旋转一个已经被旋转的 key

```json
{
  "error": "conflict",
  "details": {
    "reason": "credential_name_taken",
    "conflictingId": "cred_abc123"
  }
}
```

**客户端做法**: 提示用户改名 / 解除引用后重试。**不**是临时错误, 不要自动重试。

### `429 Too Many Requests` — 限流

详见 [限流文档](./rate-limits)。响应里带 `Retry-After` + `X-RateLimit-*` 系列头。

**客户端做法**: 退避 `Retry-After` 秒后重试。

## 5xx 通用: 服务端错

5xx 表示**服务端自己出了问题**, 客户端**改请求解决不了**, 但**重试有可能恢复**。

### `500 Internal Server Error` — 内部异常

未分类的程序 bug。响应体里 `error` 是 `internal_error`, 不会暴露堆栈或内部路径 (避免泄露实现细节)。

```json
{ "error": "internal_error" }
```

**客户端做法**: 通用错误提示, 引导用户上报 issue。生产监控会在 Sentry 之类的地方拿到完整堆栈。

### `502 Bad Gateway` — 上游挂了

OPC 自身服务正常, 但**调用的上游供应商** (Anthropic、 火山引擎、Kling 等) 返回了不可用状态。

```json
{
  "error": "upstream_unavailable",
  "details": {
    "provider": "anthropic",
    "upstreamStatus": 503
  }
}
```

**客户端做法**: 提示"供应商临时不可用, 稍后重试", 退避 5-30 秒后重试一次。

### `503 Service Unavailable` — OPC 维护中

实例主动进入维护模式。响应头会带 `Retry-After`, 通常是一个具体的时间点 (维护结束时间)。

```http
HTTP/1.1 503 Service Unavailable
Retry-After: 3600
```

```json
{
  "error": "maintenance",
  "details": { "until": "2026-06-06T12:00:00Z" }
}
```

**客户端做法**: 顶一个维护横幅, 不要重试, 等 `Retry-After` 到了再让用户操作。

### `504 Gateway Timeout` — 上游超时

上游供应商**没有在规定时间内返回**。默认 60s 上限。

```json
{
  "error": "upstream_timeout",
  "details": {
    "provider": "volcengine",
    "timeoutSeconds": 60
  }
}
```

**客户端做法**: 长任务 (长文生成、视频生成) 用户已经习惯了"等一会儿", 直接重试一次, 二次失败再报错。

## 错误响应统一格式

所有错误响应**严格**遵守下面的 envelope, 任何字段都是可选但**类型固定**:

```ts
interface ErrorResponse {
  // 机器可读的稳定错误码, 客户端 switch 靠这个
  error: string

  // 可选: 给程序用的结构化上下文
  details?: {
    // 4xx 常见
    field?: string                  // 400: 哪个字段错
    reason?: string                 // 400/409: 错的原因码
    requiredRole?: 'admin' | 'member'  // 403: 需要的角色
    resource?: string               // 404: 资源类型
    id?: string                     // 404: 资源 id
    conflictingId?: string          // 409: 冲突的资源 id
    scope?: 'ip' | 'user'           // 429: 触限层
    skill?: string                  // 429: 触限 skill
    limit?: number                  // 429: 配额上限
    windowSeconds?: number          // 429: 窗口长度
    retryAfterSeconds?: number      // 429: 重试等待

    // 5xx 常见
    provider?: 'anthropic' | 'volcengine' | 'kling' | string  // 5xx: 上游
    upstreamStatus?: number         // 502: 上游 HTTP 状态
    timeoutSeconds?: number         // 504: 超时阈值
    until?: string                  // 503: 维护结束 ISO 时间
    [key: string]: unknown          // 预留: 未来扩展
  }

  // 永远不会有 message / msg / description 之类的"给人看"字段
  // 客户端需要展示时, 自己用 error 字符串映射中文/英文
}
```

### 几个铁律

1. **`error` 永远是稳定字符串**。客户端代码可以直接 `switch (error)` 判断, 不会因为文案调整而崩溃。
2. **`details` 永远是可选**。服务端不阻塞响应, 缺字段就少给, 不要因为一个细节搞挂整个错误流。
3. **不暴露堆栈、SQL、文件路径**。`details` 里的字段是白名单, 任何反射性的内部状态都不会漏出来。
4. **HTTP 状态码和 `error` 字符串一致**。400 必须配 `invalid_argument` 类的, 不允许 500 + `invalid_argument` 这种混乱组合。

## 上游特定错误映射

OPC 把供应商的错误**统一翻译**成自己的错误码, 客户端**不需要**为每个供应商写适配。

### Anthropic (Claude)

| 上游错误 | OPC 映射 | 说明 |
|----------|----------|------|
| `401 invalid_api_key` | `500 { error: "credential_invalid", details: { provider: "anthropic" } }` | API key 错或被吊销, 提示用户去 `/credentials` 旋转 |
| `429 rate_limit_exceeded` | `429 { error: "rate_limited", details: { provider: "anthropic" } }` | **透传**, 沿用上游的 `Retry-After` |
| `529 overloaded` | `502 { error: "upstream_unavailable", details: { provider: "anthropic", upstreamStatus: 529 } }` | 上游过载, 退避重试 |
| `400 invalid_request` | `400 { error: "invalid_argument", details: { provider: "anthropic", upstreamReason: "..." } }` | 透传, 但 `details` 标记是上游拒绝的 |
| 网络超时 | `504 { error: "upstream_timeout", details: { provider: "anthropic", timeoutSeconds: 60 } }` | |

### 火山引擎 (豆包 / 视觉)

| 上游错误 | OPC 映射 | 说明 |
|----------|----------|------|
| 鉴权失败 (10000+ 业务码) | `500 { error: "credential_invalid", details: { provider: "volcengine" } }` | |
| 余额不足 (5xxx 系列) | `402 { error: "insufficient_balance", details: { provider: "volcengine" } }` | 走支付墙, 不算通用 4xx |
| 限流 | `429 { error: "rate_limited", details: { provider: "volcengine" } }` | 透传 Retry-After |
| 内容审核拒绝 (输入或输出) | `400 { error: "content_blocked", details: { provider: "volcengine", phase: "input" \| "output" } }` | 告诉客户端是哪一侧被拒 |
| 服务端 5xx | `502 { error: "upstream_unavailable", details: { provider: "volcengine" } }` | 退避重试 |

### Kling (媒体)

| 上游错误 | OPC 映射 | 说明 |
|----------|----------|------|
| 任务提交失败 | `502 { error: "upstream_unavailable", details: { provider: "kling" } }` | 退避重试 |
| 任务处理中状态码异常 | **不报错, 轮询直到完成或失败** | 长任务, 客户端不感知 |
| 任务最终失败 | 把 Kling 的 `taskStatus` 错误翻译成 `500 { error: "media_generation_failed", details: { provider: "kling", reason: "..." } }` | |
| 余额不足 | `402 { error: "insufficient_balance", details: { provider: "kling" } }` | |

### 通用映射规则

1. **供应商的 4xx 一律转 OPC 的 4xx**, 除非是供应商的鉴权错 (那是**我们的**错, 转 500)
2. **供应商的 5xx 一律转 OPC 5xx**, 优先 502 (上游挂了) 而不是 500 (我们的错)
3. **`Retry-After` 透传**, 不做加工, 供应商说啥就信啥
4. **`details.provider` 永远存在** (5xx 时), 方便客户端做更精细的提示

客户端写错误处理时, **不需要 import 任何供应商 SDK**, 只看 `error` 字符串和 HTTP 状态码就够了。
