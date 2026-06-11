---
title: 限流
description: OPC 的限流策略 — per-IP + per-user 双层、各 skill 的具体配额、超限响应与申请提额流程。
order: 2
---

OPC 的限流是**保护后端不被滥用 + 保护用户不被多租户挤爆**的双重设计。所有 skill 端点都受同一套框架管控, 具体阈值写在 `apps/console/lib/skills-registry.ts` 里, 改一处全站生效。

## 总体策略: per-IP + per-user 双层

每个请求会**同时**被两层限流检查, 取**先到上限**的那一层作为拒绝依据:

### 第一层: per-IP

- 维度: 客户端 IP (`X-Forwarded-For` 头第一段, 反代已经做过白名单)
- 目的: 防扫描、防爬虫、防脚本小子
- 配额: 默认每分钟 60 次, 不区分 skill
- 共享: 同一 IP 下所有 skill 共享这一个桶
- 实现: 滑动窗口, 60 秒内累计超过 60 次即拒绝

### 第二层: per-user

- 维度: 登录用户 id (从 `opc_session` 解出来)
- 目的: 防止单个用户把昂贵 skill 跑满, 也防止账号被共享
- 配额:**按 skill 单独配置**, 见下表
- 共享: 不跨 skill, 互不影响
- 实现: 滑动窗口, 窗口长度通常 60 秒, 个别昂贵 skill 拉长到 3600 秒

### 两层之间的关系

请求进来时, **先校验鉴权再过限流** (没登录直接 401, 不会消耗限流配额), 然后**两层都过一遍**, 任何一层到达上限就返回 429。两层是**独立计数**的, 也就是说 per-IP 用了 30 次不影响 per-user 的配额, 互不抵扣。

## 各 skill 的具体限流值

下面这张表是当前 (Phase 3) 的默认值, 以 `skills-registry.ts` 为准。`rateLimit` 列是面向操作员的简写, 实际是 `次数 / 时间窗` 的结构化数据。

| 分类 | Skill | per-user 限流 | 备注 |
|------|-------|---------------|------|
| AI | `topic.generate` | 30 / 分钟 | 选题成本低, 配额宽松 |
| AI | `topic.score` | 60 / 分钟 | 评分快, 主要防脚本 |
| AI | `script.generate` | 20 / 分钟 | 长文生成, token 成本中等 |
| AI | `script.score` | 60 / 分钟 | 同 `topic.score` |
| AI | `platform.adapt` | 30 / 分钟 | 平台改写, 中等成本 |
| AI | `explosive.decompose` | 10 / 分钟 | 拆解要拉多条参考, 较贵 |
| AI | `pipeline.run` | 5 / 分钟 | 串多步, 配额保守 |
| AI | `batch.run` | 3 / 分钟 | 批量入口, 兜底用 |
| 媒体 | `media.image` | 10 / 分钟 | 走 Kling / 即梦, 供应商侧也限流, 不能比它高 |
| 媒体 | `media.audio` | 10 / 分钟 | (Phase 4) |
| 媒体 | `media.video` | 5 / 分钟 | (Phase 4) |
| 运营 | `credentials.*` | 60 / 分钟 | 内部端点, 几乎不限 |
| 运营 | `logs.query` | 30 / 分钟 | 数据库读, 中等 |

> 注: 上述数字是 2026 年 Q2 的基线。生产中通过 `apps/console/lib/skills-registry.ts` 单点维护, 改完跑 `pnpm build` 即生效。

### 怎么查当前配额?

控制台右上角在 skill 详情页 (`/docs/skills/[id]`) 直接展示该 skill 的限流值。沙盒页 (`/docs/sandbox`) 调一次失败后, 响应头会告诉你**剩余配额**和**重置时间**, 见下面。

## 超限响应: 429 + `Retry-After`

触限时的响应**完全符合 HTTP 语义**, 客户端可以直接用标准库解析:

```http
HTTP/1.1 429 Too Many Requests
Content-Type: application/json
Retry-After: 17
X-RateLimit-Limit: 30
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1717164000
X-RateLimit-Scope: user

{
  "error": "rate_limited",
  "details": {
    "scope": "user",
    "skill": "script.generate",
    "limit": 30,
    "windowSeconds": 60,
    "retryAfterSeconds": 17
  }
}
```

### 响应头字段

| 头 | 含义 |
|----|------|
| `Retry-After` | **多少秒之后**可以重试, 客户端应当遵守 |
| `X-RateLimit-Limit` | 触发上限的**那一层**的配额 (per-IP 60 / per-user 30) |
| `X-RateLimit-Remaining` | 触限层剩余配额, 触发时为 0 |
| `X-RateLimit-Reset` | Unix 时间戳, 配额桶**完全重置**的时刻 |
| `X-RateLimit-Scope` | `ip` 或 `user`, 告诉你**哪一层**被触发了 |

### 响应体字段

- `error`: 固定字符串 `rate_limited`, 方便程序判断
- `details.scope`: 同 `X-RateLimit-Scope`
- `details.skill`: 触限的具体 skill id (per-user 层才有, per-IP 层为 `null`)
- `details.limit` / `details.windowSeconds`: 该层的配额和窗口长度
- `details.retryAfterSeconds`: 同 `Retry-After` 头, 方便 JS 直接读

### 客户端退避建议

```ts
async function callWithBackoff(url: string, init: RequestInit, attempt = 0) {
  const res = await fetch(url, init)
  if (res.status !== 429) return res

  // 优先读 Retry-After, 没有就指数退避封顶 30s
  const retryAfter = Number(res.headers.get('Retry-After') ?? '0')
  const waitMs = retryAfter > 0
    ? retryAfter * 1000
    : Math.min(30_000, 1000 * 2 ** attempt)
  await new Promise(r => setTimeout(r, waitMs))
  return callWithBackoff(url, init, attempt + 1)
}
```

控制台沙盒页 (`/docs/sandbox`) 内置了这段逻辑, 触发 429 会自动等 `Retry-After` 秒再重试一次。

## 申请提额

默认配额是按"个人/小团队日常使用"调的。如果你跑了实际生产, 配额撑不住, 可以申请提额:

**联系邮箱**: `quota@opc.local` (Phase 3 占位邮箱, 真实域名在生产部署时替换)

**邮件里请提供**:

1. 部署实例的 `instance_id` (在 `/settings` 页右上角能看到)
2. 申请提额的具体 skill id
3. 期望的新配额 (次数 / 时间窗)
4. 一句话业务场景 (例如"每天早上 9 点批量生成 200 条选题")

**审核标准**:

- per-user 配额最大放宽到 `1000 / 分钟` (再高会触发供应商侧限流, 没意义)
- per-IP 配额最大放宽到 `300 / 分钟`
- 提额审批人工 1-3 个工作日

**注意事项**:

- 提额是**整实例生效**, 不是按用户
- 提额会写入 `instance.yaml` 持久化, 重启不丢
- 滥用提额 (例如拿去给别人当代理) 会收回配额并封号
