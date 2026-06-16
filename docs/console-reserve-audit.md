# Console Reserve Audit — Multi-User + Billing

审计时间: 2026-06-05
审计范围: `apps/api/` (Go) + `apps/console/` (Next.js) + `apps/shared/`
审计目的: 评估"多用户 (org / members)"与"计费 (billing)"两个 Phase 4+ 能力的预留就位情况, 给补齐成本与优先级一个事实基础
不在范围: SSO / OAuth、多组织层级、跨 OPC instance 联邦、实时计费流

---

## TL;DR

**结论: 多用户维度基本就位, 计费维度数据层就位, 业务层完全缺。**

| 维度 | 数据模型 | 鉴权 | 租户隔离 | 业务层 | 状态 |
|------|---------|------|---------|--------|------|
| 多用户 (org / members) | OK (`User`/`Session`/`Credential.UserID`/`CallLog.UserID`/5 业务表) | OK (RequireAuth + cookie + `CtxUserID`) | OK (所有 list 走 `Where("user_id = ?")` + `requireUserID` 守门) | 缺 | 8.5/10 |
| 计费 (billing) | 部分 (cost 字段就位, 价格表残缺) | 缺 (无 quota 拦截) | n/a | 缺 (无支付 / 账单生成) | 4/10 |

**关键风险:**
1. **跨租户 data leak 历史洞**: `handlers/tenant_guard.go:25` (`ErrMissingUserID`) + `multitenant_test.go` 是新近补的 tripwire, 之前 store 用 `if userID != 0 { Where(...) }` 会在绕过 RequireAuth 时静默跨租户读 (`tenant_guard.go:13-19` 自述)。现状已修, 但这条 tripwire 之前是漏的。
2. **call_log.cost 全部为 0**: `middleware/call_log.go:39` (注释明示 `CostCents is left at 0 by the middleware. The handler is responsible for stamping the cost`), 实际没有任何 handler 覆写 cost — billing dashboard 上线时, 所有历史数据全是 0。
3. **Douyin provider 在表里但不在定价表里**: `models/credential.go:22` 接受 `provider=douyin` 写库, `config/cost.go:44-48` 4 个 skill 常量无 douyin — 即未来一旦 douyin 接入, 成本归集会静默归零 (`config/cost.go:99` 注释: "Missing entries return the zero SkillPrice")。
4. **价格变更无历史**: `config/cost.go` 是 in-memory map, 改一行就改全局, 没有 `price_history` 表 — 改了之后历史 call_log 的 cost_cents 仍然是旧价, 但新行是新价, 财务报表对账时会出现"同一天同一 provider 价格不同"。
5. **配额 enforcement 完全没有**: `middleware/call_log.go` 只写日志不拦截, 没有任何 quota / rate limit middleware — 一旦商业化, 用户可以无限刷量。

---

## 一、多用户 (org / members)

### 1.1 数据模型 (✓ 已就位)

**核心表:**

- `users` (`models/user.go:11-18`): `id, email (uniqueIndex), password_hash (json:"-"), name, created_at, updated_at`。
  - `uniqueIndex` on email — 跨用户去重 OK。
  - **无 `deleted_at` 软删除字段**: `grep -rn "gorm.DeletedAt\|DeletedAt" /root/workspace/opc/apps/api/internal/models/` 零结果。这意味着未来加"禁用用户"要么 hard delete (丢外键), 要么新增 `status` 字段 (改 schema)。
- `sessions` (`models/session.go`): `id, user_id, token (uniqueIndex), expires_at, created_at`。
  - 外键逻辑上是 `user_id → users.id`, 但 **模型上没有显式 `references` 声明** — SQLite + gorm 走的是"用户删除时 session 静默悬挂"。`auth/session.go:92-101` 在校验时若发现 user 不存在返回 `ErrSessionNotFound` 兜底, 但 call_log 里 `user_id=0` 会被 skip (见 1.2)。
- 5 业务表 (Topic/Script/ContentItem/KnowledgeDoc/Series) 全部有 `user_id gorm:"index"` + `Unscoped()` 调用点 (e.g. `scripts.go:358`, `knowledge.go:329`) — 是 **hard delete** 实现, 没用 gorm soft delete。
- `credentials` (`models/credential.go:55-65`): `user_id gorm:"index"` + `provider gorm:"index"`。
  - `user_id` 注释: `"UserID is the multi-tenant key — the Phase 2 RequireAuth middleware stamps the caller, and every store method WHERE's on it. There is no admin override path"` (`credential.go:44-47`)。
- `call_logs` (`models/call_log.go:23-106`): `user_id gorm:"index"` + `cost_cents int` + `cost_currency string default CNY`。
  - `cost_cents` / `cost_currency` 注释明示是 **Phase 4 billing 预留** (`call_log.go:18-22`), 字段已就位不需 migrate。

**迁移覆盖:** `db/db.go:99-112` 一次 `AutoMigrate` 跑 9 张表, User/Session 在业务表之前, 注释明示: "User 和 Session 独立于业务实体... 必须在业务表之前就位, 这样后续 PR 引入 OwnerID 外键时不会出现 user 表不存在的迁移错误" (`db.go:95-98`)。

**未就位 (列补齐时需要):**
- 软删除字段 (`DeletedAt`) — 加一行 struct tag 即可, 但需评估历史 hard delete 行为是否要回溯。
- `users.org_id` / `users.role` — 当前 User 表无任何角色字段, 角色假设 (admin/operator/finance) 是隐性的, 见 1.4。
- `users.status` (active/disabled) — 同上, 需 schema 改动。

### 1.2 鉴权 (✓ 已就位)

**链路:**

1. 登录: `handlers/auth.go:220-274` (Login) → `auth.CreateSession(db, u.ID)` (`auth/session.go:52-68`) → 写 cookie `opc_session` (`auth.go:81-92`, path=`/api`, HttpOnly, SameSite=Lax, 默认 Secure, env `OPC_INSECURE_COOKIES=1` 关闭)。
2. 中间件: `middleware/auth.go:77-111` (RequireAuth) → 读 cookie → `auth.ValidateSession(db, token)` (`session.go:74-102`) → `c.Set(CtxUserID, u.ID)` + `c.Set(CtxUser, u)` (`auth.go:107-108`)。
3. 业务读 user_id: `handlers.UserIDFromContext(c)` (re-export `middleware/auth.go:127-136`)。

**关键设计点:**

- Cookie 绑 user_id: 是的, 通过 session 行 (`sessions.user_id`) 间接绑, 不在 cookie 里塞 user_id。Token 是 32 byte 随机 hex (`session.go:34`, 64 字符), 不可猜。
- `requireUserID` 守门: `handlers/tenant_guard.go:25-35` 在每个 `gorm*` store 入口检查 `userID == 0` → 返回 `ErrMissingUserID` 5xx。
  - **这是新加的 tripwire**, 注释 (`tenant_guard.go:5-25`) 写明了"之前 store 用 `if userID != 0 { Where(...) }` 静默跨租户, 现在 0 是 programmer error"。`multitenant_test.go` 是配套的回归测试。
- Test-only 旁路: `middleware/auth.go:163-168` (StubUser) — 单测用, 注释反复警告"Production code MUST NOT use this"。
- Skip list: `middleware/call_log.go:111-119` 显式 skip `/api/auth/`, `/api/health`, `/api/observability/`, `/api/call-logs` 等, 不会因为递归读自己日志产生噪声。
- 路由注册顺序: `cmd/server/main.go:230-249` — RequireAuth 在 CallLog 之前 (`main.go:231-233` 注释: "The call_log middleware is mounted INSIDE this group so a request to /api/observability/* does not log itself"), CallLog 延后读 `CtxUserID` (`call_log.go:166-186` 注释: "UserID is captured LATE (in the defer) so the auth middleware that runs immediately after this one (RequireAuth) has time to set CtxUserID")。

**`/api/auth/me` 改多 user 需要改什么:**
- 当前 `handlers/auth.go:405-420` (Me) 返回 `toUserResponse(u)`, 已是 per-request — 不需要改。
- 若加"我的组织 / 我的成员", 需要新加 `GET /api/orgs` + `GET /api/orgs/:id/members`, **handler 不存在** (零代码)。

**未就位:**
- 注册默认关闭: `auth.go:305` (`if !envBool(registerEnabledEnv) { 403 }`), 注释明示"Phase 2 ships with registration closed by default — the operator is expected to seed the first admin user out-of-band"。第一个用户怎么 seed? 代码里没看到 seed 路径, 需补 `cmd/seed-admin` 或初始化脚本。
- 密码重置 / 邮箱验证: 完全缺。

### 1.3 租户隔离 (✓ 已就位, 但有隐患)

**已 WHERE user_id 的 handler (抽样):**

| 文件 | 行 | 模式 |
|------|-----|------|
| `handlers/topics.go` | 123, 140, 165, 198, 221, 311 | `topic.UserID = UserIDFromContext(c)` / `Where("user_id = ?", f.UserID)` |
| `handlers/scripts.go` | 102, 258, 282, 299, 358 | 同上, 含 `Unscoped()` 显式 hard delete |
| `handlers/knowledge.go` | 93, 247, 268, 285, 329 | 同上 |
| `handlers/quality.go` | 525, 529 | 列表 query 走 user_id |
| `handlers/import_export.go` | 141, 160, 212, 285, 314, 333, 356, 377, 399, 418, 441 | export/import 全程 stamp user_id |
| `handlers/observability.go` | 300, 336, 389, 408, 454 | call_log 聚合全部 WHERE user_id |

**守门:** 9 个表全部 `gorm:"index"` on user_id (`models/*.go`)。`requireUserID` 在 store 层兜底。

**隐患:**
- **observability handler 自身的 skip**: `call_log.go:114` 把 `/api/observability/` 加进 skip list — 注释解释是"避免递归聚合自膨胀", 但也意味着 **observability 读写不计 call_log**, 因此未来要按 observability 调用本身做 quota / 计费时, 数据源会缺一块。
- **多 session 并发**: `sessions` 表按 token 唯一, 但同一 user 多设备登录会创建多行 — 这是设计预期, 不是 leak。
- **没有 `org_id`**: 单层 user_id, 不支持"同一 user 属于多组织"。当前设计假设"一个 user = 一个 tenant", 未来加 org 模型需重新设计 `users ↔ orgs` 多对多。
- **MCP 通道**: `cmd/server/main.go:83-104` 启动 MCP stdio server, **没有走 RequireAuth** (stdio 信任调用方是本机), `user_id` 在 MCP 路径上是 0 — 任何 MCP 调用都会写 `user_id=0` 的 call_log (call_log 写日志不报错, 注释里也提到)。`multitenant_test.go:6-11` 注释里把这个列为"未来 caller bypassed RequireAuth" 的例子。

**跨租户 data leak 风险: 低 (现状) / 中 (历史数据)**
- 现状 tripwire 已就位, `requireUserID` + StubUser 警告 + multitenant test 三件套。
- 历史风险: 在 `tenant_guard.go` 改动前, store 用 `if userID != 0 { Where(...) }`, bypass 路径会跨租户读。改动时间无 git log 可查 (本环境非 git repo, `env.Is directory a git repo: No`), 但 `tenant_guard.go:5-25` 注释把这段历史写明了 — 建议 review 老 PR 确认是否有数据被读出过。

### 1.4 邀请流 / 角色权限 (✗ 未就位)

**邀请流 (邀请成员加入):**
- `email / token / accepted_at` 字段: **零代码**, 无表无 model。
- `registerEnabled` 是注册闸 (`auth.go:305`), 不是邀请闸 — 开启后任何人可注册, 不是定向邀请。
- 现状成员管理页 `apps/console/app/(console)/members/page.tsx` 是 reserve route, 11 行占位, "多 operator 协作（邀请、角色权限、SSO）会在需要时启动"。

**角色权限 (admin / operator / finance):**
- **User 表无 role 字段** (`models/user.go:11-18` 全部字段列表), 零代码。
- 隐性假设: 单用户模式 = 默认 admin, 所有能力开放。代码里没有任何 `if user.IsAdmin { ... }` 之类的判断 (grep 全空)。
- 未来加 RBAC: 要扩 User 表 + 加 middleware `RequireRole("operator")` + 每个 handler 选角色 (e.g. `billing` 页只对 `finance` 角色开放)。

### 1.5 补齐成本估算 (多用户)

| 项 | 代码量估计 | 备注 |
|------|----------|------|
| `users.deleted_at` 软删除 | 5 行 | struct tag 一行 + 全量 store 加 `Unscoped()` 评估 |
| `users.status` + `users.role` | ~50 行 model + 1 middleware | User 扩 2 字段, `RequireRole` 中间件 ~30 行 |
| 邀请流 (InviteToken 表 + handler + email) | ~400-600 行 Go + ~200 行 TS | `Invite{Email, Token, ExpiresAt, AcceptedAt, OrgID}`, POST /api/invites, GET /api/invites/accept, 邮件发送 (SMTP / 第三方) |
| 成员管理 API (list / remove / change-role) | ~250-400 行 Go | 5 个 endpoint × 50-80 行 |
| 成员管理 UI (`/members` 实现) | ~400-600 行 TS | 当前 11 行占位, 需表格 + 邀请 dialog + 角色选择 |
| `/org` UI 实现 (组织切换) | ~300-500 行 TS | 多组织才需要, **当前 User-=Org 一对一, 暂不需要** |
| 密码重置 / 邮箱验证 | ~300-500 行 Go + ~200 行 TS | token 表 + 邮件 + 限流 |
| **合计 (多用户从 1→N)** | **~1700-2700 行 Go + ~900-1300 行 TS** | 估时 4-6 周 (1 人) |

**复用现状:**
- 鉴权 / cookie / 租户隔离 全部可复用, 不重写。
- `requireUserID` 守门已就位, 不用动。
- `Credential` / `CallLog` 等表已带 user_id, 不需 migrate。

---

## 二、计费 (billing)

### 2.1 成本归集 (△ 部分就位)

**call_log 字段 (✓):** `models/call_log.go:90-96` — `CostCents int` + `CostCurrency string gorm:"size:8;default:CNY"`。注释 (`:18-22`) 明示是"Phase 4 billing 预留, 今天的 middleware 写 cost=0 for every row, 字段已就位所以 schema 不必 migrate"。

**定价表 (△):** `config/cost.go:43-48` 定义 4 个 Skill 常量, `:99-123` 是 `skills map[Skill]SkillPrice` 查表。

| Skill | 单价 | 单位 | 备注 |
|-------|------|------|------|
| `volcengine.kling_image` | 5 cents | image | 有 |
| `volcengine.jimeng_image` | 5 cents | image | 有 |
| `volcengine.tts_haiku` | 1/100 cent | char | 有 (sub-cent 分数) |
| `anthropic.claude_haiku_4_5` | 8/100 cent input + 4/100 cent output | 1k tokens | 有 |

**覆盖缺口:**
- **douyin 缺**: `models/credential.go:22` 接受 `provider=douyin` 写库, 但 `config/cost.go` 无 `SkillVolcengineDouyin*` 常量。`ProviderIsAllowed` 通过 (`credential.go:30-35`), 写库不报错, 读侧未来一旦有 douyin 调用, `PriceFor` 返回 0 (`cost.go:128-131`), dashboard 显示"免费" — 静默归零。
- **deepseek / gemini 缺**: `cmd/server/main.go:65-70` 注册了 `NewDeepSeekProvider` / `NewGeminiProvider` 作为 AI router 备选, 但 `config/cost.go` 无对应定价 — 任何 deepseek/gemini 调用会归零。
- **anthropic sonnet / opus 缺**: 只有 haiku 4.5, 注释 (`cost.go:139-141`) 提到"Sonnet/Opus tier can land without touching CostForSkill", 实际上需要新加常量 + `outputPriceCentsPer1K` map 行, 工作量不大但没做。
- **未注册 skill 归零**: `cost.go:170-173` (`if !ok { return 0 }`), 注释明示这是"experimental handler can be wired up and exercised without bookkeeping, and the operator flips it to billable by adding a row"。问题: 漏加不会报错, 只有 dashboard 上"成本 = 0"。

**`call_log.go` cost 写日志路径 (✗ 关键缺陷):**

`middleware/call_log.go:39-40` 注释:
```
// CostCents is left at 0 by the middleware. The handler is
// responsible for stamping the cost once it knows the billable
// units (image count, token count, character count).
```

+ `call_log.go:197-209` 写 row 时 `CostCents: ref.CostCents` 透明传递。
+ `config/cost.go:184-187` 注释: "The middleware (middleware/call_log.go) does NOT call this — the middleware stamps cost=0 and the handler overrides on the CallLogRef."

**实际 handler 覆写情况:** `grep -rn "ref.CostCents\|\.CostCents =" /root/workspace/opc/apps/api/internal/handlers/` 全部 0 命中 (除注释)。**所有 handler 都没有调 `CostForSkill` / `CostForTokenSkill` 写 cost_cents**。

**结论: 字段就位、定价表残缺、写日志路径上 0 个 handler 在用, 当前 call_log 表里所有 cost_cents = 0。**

**未就位 (列补齐时需要):**
- `price_history` 表 / 时间戳 — `config/cost.go:99-123` 的 `skills` map 是 package-private 静态 map, 改源码即生效, 无历史。商业化必须能解释"为什么 5 月 1 日的 call_log 成本是 5 cents 但 6 月 1 日同 provider 是 4 cents" — 当前无解。
- FX (USD ↔ CNY 汇率) — `cost.go:42-44` 注释:"Anthropic bills in USD; when a handler logs a call under a USD-billed provider, it should override the default currency at write time. The conversion is out of scope for this file"。 当前 Anthropic 行的 `CostCents` 直接以"cents"算 (实际是 sub-cent USD), 没换汇。
- 退款 / 调整 — 无 `call_log.cost_cents` 回写路径, call_log 自述 (`:10-11`) 是 "append-only (no updated_at): the row is a fact about one HTTP call, and editing it after the fact would be an audit-trail violation"。即 cost 写错也不能改, 只能加 adjustment 行 — 但 adjustment 表无。

### 2.2 配额 enforcement (✗ 未就位)

**现状:** `middleware/call_log.go` 是 **observability-only**, 不拦截、不返回 429、不返回 402。整条中间件链 (`cmd/server/main.go:230-233`):

```go
apiGroup := r.Group("/api",
    handlers.NewRequireAuth(gormDB, slog.Default()),
    middleware.CallLog(middleware.CallLogConfig{DB: gormDB, Logger: slog.Default()}),
)
```

中间件顺序: RequestID → RequestLogger → Metrics → Recovery → CORS → RequireAuth → CallLog → handler。

**未来加 quota checkpoint 在哪改:**
- **优先方案: 新增中间件 `middleware/quota.go`**, 在 `main.go:232` 之后 / 业务 handler 之前挂载: `middleware.Quota(quotaCfg)`。
- 检查点: `ref := CallLogFromContext(c)` → 读 `ref.Skill` (已有) → 查 `quotas` 表 (user_id + skill + period + used + limit) → 超额 return 402 Payment Required。
- 计数: 在 `call_log.go:213` `Create(&row)` 之后 (`defer func()` 末尾), 触发 `quota.TrackUsage(userID, skill, costCents)` 异步 (channel / goroutine, 不阻塞 request finalizer)。
- 或者: 复用 `CallLogRef` 的 cost 字段, 在 middleware 跑完 handler 后再 charge (post-pay 模型) — 这样失败不拒服务, 但超额也用了。
- **更早期方案: handler 入口拒绝** (pre-pay), 拿不到 cost 时按 skill 默认单价估算 — 适合"套餐内 N 次" 而不是"按用量"。

**未就位:**
- `quotas` 表 / `subscriptions` 表 — 零代码。
- 套餐 / 软限 / 硬限概念 — 零代码。
- 超额返回码 / 错误体 — 零代码, 需在 `handlers/` 加 `ErrQuotaExceeded` 哨兵 + handler 包装。

### 2.3 支付 (✗ 完全缺)

**现状:** 零代码, 零模型, 零 env。

**未来加需要什么:**
- **支付方式**: 微信支付 / 支付宝 / Stripe。微信 + 支付宝需要商户号 + 异步回调 (webhook) + 签名验签, 走 `POST /api/billing/webhook/wechat`, `POST /api/billing/webhook/alipay`, IP 白名单 + signature header。
- **订单表**: `orders {id, user_id, sku_id, amount_cents, currency, status, provider, provider_txn_id, paid_at, created_at}`。
- **订阅表 (套餐)**: `subscriptions {id, user_id, plan_id, period_start, period_end, status, auto_renew}`。
- **凭证表**: 同上, 选 `volcengine` / `anthropic` 走 `credential_id` 关联调用。
- **API 端点**: `GET /api/billing/plans`, `POST /api/billing/orders`, `POST /api/billing/orders/:id/pay`, `POST /api/billing/webhook/:provider`。
- **前端**: `/billing` 页面从 reserve route (11 行占位) 变真实页面, 含套餐选择 + 支付跳转 + 订单历史。
- **合规**: 发票抬头 (中国增值税) / 个人 / 企业 — 单独 `invoices` 表。

**估时:** 微信 + 支付宝 + Stripe 各 ~2-3 周 (3-4 后端 + 1 前端 + 1 合规), 合计 **8-12 周**。

### 2.4 账单生成 (✗ 完全缺)

**现状:** 零代码, 零 cron, 零 PDF 模板。

**未来加需要什么:**
- **账单聚合 query**: `SELECT user_id, period, SUM(cost_cents) FROM call_logs WHERE user_id = ? AND created_at BETWEEN ? AND ? GROUP BY user_id` — 复用 `observability.go:300` 现有 `Where("user_id = ? AND created_at >= ? AND created_at < ?", ...)` 模板即可, ~30 行。
- **账单行 / 账单头表**: `bills {id, user_id, period_start, period_end, subtotal_cents, tax_cents, total_cents, currency, status, issued_at, due_at, paid_at}`, `bill_lines {id, bill_id, skill, units, unit_price_cents, cost_cents, description}`。
- **PDF 生成**: 微信电子发票 / 自渲染 PDF, 走 `text/template` + chromedp / wkhtmltopdf, 或直接打 PDF 库 (`gofpdf`)。
- **cron 触发**: `cmd/billing-nightly/main.go` (新 binary), 或 `cron` 标签的 goroutine in-process, 每月 1 号 0 点跑聚合。
- **邮件发送**: 复用未来邀请流的 SMTP 设施。
- **API**: `GET /api/billing/bills`, `GET /api/billing/bills/:id`, `GET /api/billing/bills/:id.pdf`。

**估时:** 账单生成 + PDF + cron + 邮件 ~ **3-5 周**。

### 2.5 补齐成本估算 (计费)

| 项 | 代码量估计 | 备注 |
|------|----------|------|
| 定价表补齐 (douyin / deepseek / gemini / sonnet) | ~50 行 Go | 加 Skill 常量 + map 行 + `outputPriceCentsPer1K` 行 |
| Handler 覆写 cost (10 个 AI / cover / batch handler) | ~200-300 行 Go | 每 handler 3-5 行 + import |
| `price_history` 表 + 加载 | ~200 行 Go | pricing 改成数据库读, 启动时缓存 |
| FX 汇率缓存 | ~150 行 Go | 每日拉一次, redis / sqlite 存 `fx_rates` 表 |
| 配额中间件 + `quotas` 表 | ~400-600 行 Go | `middleware/quota.go` + 计数逻辑 |
| 支付 (微信/支付宝/Stripe) | ~1500-2500 行 Go + ~500 行 TS | 订单表 + 3 个 webhook + 前端 |
| 账单生成 + PDF + cron | ~800-1200 行 Go + ~200 行 TS | bills + bill_lines + pdf 模板 |
| `/billing` UI 实现 (当前 11 行占位) | ~600-1000 行 TS | 套餐 / 订单 / 账单 / 发票 |
| **合计 (计费从 0→商业化)** | **~3900-6000 行 Go + ~1300-1700 行 TS** | 估时 12-20 周 (2-3 人) |

---

## 三、优先级建议

### 3.1 半年内不接 (P3 / 暂缓)

- **多组织层级 / 跨 OPC 联邦**: 商业模型未知, 强行做易返工。当前 `User-=Tenant` 一对一够用。
- **实时计费流 (秒级用量推送)**: call_log 已是事实, dashboard 月度聚合不需要 SSE, REST 轮询够用。
- **SSO / OAuth**: 客户未提, 微信扫码登录 + 邮箱密码已覆盖中小客户。
- **退款 / 调整流**: 商业化上线前不必须有, 但要预留 `adjustments` 表 (或 `bill_lines.type = 'adjustment'`), 不需写代码只需 schema 留位。

### 3.2 3 个月内要接 (P1 / 商业化前置)

- **handler 覆写 cost (10 个 AI / cover / batch)**: 不接的话 dashboard 永远显示 ¥0, 计费无意义。**估时 1 周。**
- **定价表补齐 (douyin / deepseek / gemini)**: 不接的话新接入的 provider 都归零, 财务报表对账不上。**估时 2-3 天。**
- **`/billing` 页面"用量明细" tab**: 不接商业化也接得住, 因为 cost_cents 字段已就位, 复用 `/observability` 现有数据。只缺前端聚合视图。**估时 1-2 周 (前端)。**
- **价格变更历史 (`price_history` 表)**: 不接, 改一次价格历史数据 cost 含义全变, 财务对不上账。**估时 1 周。**

### 3.3 1 个月内要接 (P0 / 已漏的洞)

- **call_log 当前 cost 全 0 的历史数据回填**: **不能回填** (call_log 是 audit-trail, append-only), 只能:
  1. 立刻接 P1 第一项 (handler 覆写 cost), 让新行有 cost。
  2. 历史行的 cost 字段保留为 0, 在 `/billing` UI 上显示"该时段用量明细缺失" 而非"免费"。
- **handler 覆写 cost (前置)**: 实际就是 P1 第一项, 排到 P0 是因为: 推迟越久, 0 cost 的行越多, 后面"对账"越痛。
- **配额 enforcement 中间件**: 商业化前必须有, 否则用户可无限刷量。**估时 2-3 周。**
- **observability skip 自身 的隐患**: 当前 `/api/observability/*` 不进 call_log, 未来计费时要重新评估是否要记 — 建议在 P0 阶段顺手补一行 (把 observability 写出 call_log, 但 exclude 自己读的行) — **估时 半天。**

---

## 四、不在本次审计范围 (按要求跳过)

- **SSO / OAuth**: 微信扫码登录、企业微信、飞书、钉钉、Okta。零代码, 未来加需新 middleware + provider 表。
- **多组织层级**: `users.org_id` / `orgs.parent_id` 树状结构, 跨组织数据继承 / 隔离策略。零代码, 涉及租户隔离整套重审。
- **跨 OPC instance 联邦**: 多 OPC 节点互信、跨节点调用路由。零代码, 涉及 auth / 路由 / 数据同步全栈。
- **实时计费流**: WebSocket / SSE 推送每秒用量。call_log 表本身已支持分钟级查询, 不需要流式基础设施。

---

## 附录: 文件清单 (引用过)

### Go (后端)

- `apps/api/cmd/server/main.go` (338 行) — 路由注册, 中间件顺序
- `apps/api/internal/db/db.go:99-112` — 9 张表 AutoMigrate
- `apps/api/internal/models/user.go` (22 行) — User 表
- `apps/api/internal/models/session.go` (21 行) — Session 表
- `apps/api/internal/models/credential.go` (101 行) — Credential + ProviderIsAllowed
- `apps/api/internal/models/call_log.go` (123 行) — CallLog + cost 字段
- `apps/api/internal/models/{topic,script,content_item,knowledge_doc,series}.go` — 5 业务表
- `apps/api/internal/auth/session.go` (115 行) — CreateSession / ValidateSession / DeleteSession
- `apps/api/internal/middleware/auth.go` (169 行) — RequireAuth + UserIDFromContext + StubUser
- `apps/api/internal/middleware/call_log.go` (458 行) — CallLog 中间件
- `apps/api/internal/handlers/tenant_guard.go` (35 行) — ErrMissingUserID + requireUserID
- `apps/api/internal/handlers/middleware.go` (54 行) — NewRequireAuth re-export
- `apps/api/internal/handlers/auth.go` (420 行) — Login / Logout / Register / Me
- `apps/api/internal/handlers/multitenant_test.go` (提及) — 跨租户回归测试
- `apps/api/internal/handlers/{topics,scripts,knowledge,quality,observability,import_export}.go` — user_id 守门样例
- `apps/api/internal/config/cost.go` (206 行) — 定价表
- `apps/api/internal/config/cost_test.go` (187 行) — 定价测试

### TS (前端)

- `apps/console/app/(console)/org/page.tsx` (28 行) — /org reserve route
- `apps/console/app/(console)/members/page.tsx` (21 行) — /members reserve route
- `apps/console/app/(console)/billing/page.tsx` (21 行) — /billing reserve route

### 关键 grep 证据

```text
# 软删除 / role 字段
grep -rn "gorm.DeletedAt\|DeletedAt" /root/workspace/opc/apps/api/internal/models/   # 0 命中
grep -rn "user.role\|IsAdmin\|IsOperator" /root/workspace/opc/apps/api/internal/      # 0 命中

# 成本覆写
grep -rn "ref.CostCents\|\.CostCents =" /root/workspace/opc/apps/api/internal/handlers/  # 仅注释, 0 代码

# 配额 / 支付 / 账单
grep -rn "quota\|payment\|invoice\|subscription" /root/workspace/opc/apps/api/internal/   # 0 业务命中
```

---

## Phase 4 — Provider-agnostic (done 2026-06)

The operator-facing surface now lets each user pick:

- Protocol: Anthropic Messages API | OpenAI Chat Completions API
- Model: free-form string (e.g. `claude-sonnet-4-5`, `gpt-4o-mini`,
  `MiniMax-M2.7-highspeed`)
- Base URL: optional override for self-hosted / mirror deployments
- API key: stored encrypted in the `credentials` table

The `ProviderResolver` (`internal/agents/resolver.go`) maps
`(user_id, scope)` → live `TextProvider` on every call. MiniMax
remains the built-in media provider (image/speech/video); its
text path is also reachable via the Anthropic protocol by
pointing at `https://api.minimaxi.com`.

For MCN operators running this for many creators, this means
**one OPC deployment can serve users on different providers**
without any code change. The `/credentials` page in the console
is the operator's control surface; the `ProviderResolver` is the
back-end enforcement.

---

(报告完)
