# OPC Phase 2 — 对外 Web UI 设计（Sub-Spec C）

> **文档版本**：v1.0（初稿，待审）
> **日期**：2026-06-15
> **作者**：OPC 团队（与 Claude 协作）
> **状态**：设计阶段 → 待用户审阅
> **路径**：`docs/superpowers/specs/2026-06-15-opc-phase2-external-ui-design.md`
> **父 spec**：`docs/superpowers/specs/2026-06-03-opc-phase1-ip-design.md` §3.2（Phase 2 6-12 月）、§5.1（Phase 2 路线图）
> **依赖 spec**：
> - `docs/superpowers/specs/2026-06-11-opc-phase2-ip-template-design.md` v1.0（Sub-Spec A, IP profile 框架, M1 done）
> - `docs/superpowers/specs/2026-06-12-opc-phase2-capabilities-design.md` v1.0（Sub-Spec B, 4 能力 library, M1 done）
> **兄弟 spec**：M1 5 sub-spec 之 **C**（D/E 待启）

---

## 一、文档目的

为外部 beta 创作者（1-10 人 invite-only 邀请制）暴露一个独立的 web UI（`apps/external/`），让他们能登录、调 `topic` capability 生成选题、看历史、看 cost。M1 是「验证产品」最小可用：1 capability (topic) + invite-only + 零支付。**严格不**扩多 capability、加付费、加 OAuth、加 magic link——M2/M3 再说。

Sub-Spec C 是 Phase 2 critical path 第三步（A → B → C 串行）。M1 完成后，Phase 2 还有 D（agent-agnostic）和 E（反 AI 检测）可并行启。

---

## 二、Executive Summary

| 项 | 内容 |
|----|------|
| **目标** | 给外部 beta 创作者 (1-10 人, invite-only) 1 个独立 web UI，能登录、生成 topic、看历史、看 cost。复用 Sub-Spec A 的 IP profile 框架 + Sub-Spec B 的 topic library + 现有 session auth + 现有 call_log cost tracking。 |
| **范围（M1）** | 1 个新 Next.js 14 app (`apps/external/`) + 4 个 operator-only endpoint (`/api/admin/creators*`) + User model 加 `Role`/`Disabled`/`CreatedBy` 字段 + console 加 `/creators` 管理页 + E2E smoke |
| **目标用户** | 1-10 个友好创作者，operator (OPC 团队) 手动创建账号 + 设初始密码，creator 自己首次登录改密码 |
| **Capability** | 1 capability = topic（默认 IP profile: anthropomorphic；M2 暴露 4 个 IP type 选择） |
| **Auth** | 复用现有 session + `RequireAuth` middleware；新增 `RequireOperatorRole` / `RequireCreatorRole` 2 个 middleware 包装 |
| **支付** | 零支付（invite-only beta） |
| **存储** | User model 加 3 列（Role/Disabled/CreatedBy），GORM AutoMigrate 加。Topic 历史 M1 不持久化（call_log 已记 cost；M2 加 `creator_topic` model） |
| **不在 M1** | 4 capability 暴露、付费 tier、OAuth/magic link、Topic 历史持久化、IP profile 选择器 (默认 anthropomorphic)、MCN 白标、子账号、Stripe 集成 |
| **M1 timeline** | 2-3 周（5 task: User model 加列 + creator handler + middleware + console 创作者管理页 + apps/external 全新 app） |
| **依赖** | A (IP profile framework, done) + B (topic library + HTTP endpoint, done) + 现有 session auth + 现有 call_log middleware |
| **风险等级** | 🟡 中（多 surface：User model 改列 + 新 app + 新 handler + 新 middleware + console 改 — 任 一 破 现有 5 个 console endpoint 或 8 个 API endpoint 算 回归） |

---

## 三、背景与现状

### 3.1 Phase 1 已有 web app 体系

| App | 用途 | 状态 |
|-----|------|------|
| `apps/web/` | OPC 内部 demo (5 pages: 选题/脚本/日历/dashboard/知识库) | Phase 1 done, 12/12 static pages build |
| `apps/console/` | OPC 内部 operator 后台 (8 endpoint: billing/credentials/logs/observability/sandbox + Phase 2 B M1 后新增 docs/members/org/settings) | Phase 1 done + Phase 2 polish 4 轮 verify |
| `apps/external/` | **外部创作者 beta UI** (本次新增) | M1 only |

**关键观察**：3 个 app 职责清晰（demo / operator / external），但**复用同一 backend** (`apps/api`)。M1 跟 Phase 1 + Phase 2 A/B 的集成风险在于：往 User model 加列会触发 GORM AutoMigrate；新 handler 的 session 中间件要确认对老 operator 也兼容。

### 3.2 Sub-Spec A / B 已就绪的复用资产

**A（IP profile 框架）done**:
- `internal/assetgen/profiles/{anthropomorphic,digital_human,costume,info}/` 4 个 sub-package
- `assetgen.LoadProfile(typeName)` / `MustLoadProfile(...)` 公共 API
- 4 类 IP profile 各 1 个 default instance（M1 创作者 暂只能用 anthropomorphic；M2 暴露 4 个选择器）

**B（4 能力 library）M1 done**:
- `internal/capabilities/topic.Generate(ctx, text, input) (*Result, error)` 纯 library
- `internal/handlers/ai.go:GenerateTopics` 已重构调 library
- `POST /api/ai/topics` 现有 HTTP endpoint（5 GenerateTopics test PASS，G1-closed 2026-06-09 行为保留）
- M1 创作者 UI 直接调 现有 endpoint，不新加 B-side code

**现有 auth 资产**:
- `internal/models/{user,session}.go` + GORM AutoMigrate
- `internal/handlers/auth.go:Register/Login/Logout/Me` (4 个 endpoint)
- `internal/middleware/auth.go:RequireAuth` (验 session cookie)
- Session cookie 名: `opc_session`

**现有 cost tracking**:
- `internal/middleware/call_log.go` — 每个 API call 记一行 (token + cost_cents)
- G6 closed 2026-06-09 — `StampTextCost` middleware 在 11 个 handler 调用点

### 3.3 不在 M1 范围（推到 M2/M3）

- ❌ 4 个 capability 暴露（仅 topic；script/deconstruct/asset 等 B M2 再说）
- ❌ 付费 tier / Stripe 集成
- ❌ OAuth / Magic link 邀请
- ❌ Topic 历史持久化（call_log 已记 cost；M1 UI 看不看历史无所谓，operator 在 console/observability 看）
- ❌ IP profile 选择器（默认 anthropomorphic；M2 加 4 个下拉）
- ❌ MCN 白标 / 子账号
- ❌ 多语言（先用中文）
- ❌ 移动端适配（先桌面）

---

## 四、设计目标（M1 验收）

| # | 验收 | 验证命令 |
|---|------|---------|
| 1 | `apps/external/` 新 Next.js 14 app 存在 + `pnpm build` exit 0 | `cd apps/external && pnpm build` |
| 2 | `User` model 加 3 列 (`Role`/`Disabled`/`CreatedBy`) + GORM AutoMigrate | `go test -tags fts5 ./internal/models/...` PASS |
| 3 | 4 个 operator-only endpoint (`/api/admin/creators*`) 存在 + 4 case test PASS | `go test -tags fts5 -run TestCreator ./internal/handlers/...` |
| 4 | 2 个新 middleware (`RequireOperatorRole` / `RequireCreatorRole`) + 9 case test PASS | `go test -tags fts5 -run TestRequire ./internal/middleware/...` |
| 5 | `apps/console/app/(console)/creators/page.tsx` 存在 (operator 创作者管理页) | `ls apps/console/app/(console)/creators/` |
| 6 | `apps/external/app/login/page.tsx` 存在 | `ls apps/external/app/login/` |
| 7 | `apps/external/app/(creator)/dashboard/page.tsx` + `topic/page.tsx` 存在 | `ls apps/external/app/(creator)/` |
| 8 | E2E smoke 加 4 case (operator 创建 / creator 登录 / creator 调 topic / creator 调 admin 403) | `./scripts/api-smoke.sh` 16/16 PASS |
| 9 | 老 endpoint 不破 (5 handler + 4 IP type + 1 advisory + 2 topic = 12 case) | 跑 smoke, 0 回归 |
| 10 | verify-project-state agent OVERALL: PASS | `~/.claude/audit/verify-reports/verify-*.md` PASS |

**Out of scope（M1 不做）**:
- ❌ 4 capability 暴露 (script/deconstruct/asset 都没)
- ❌ Topic 历史持久化
- ❌ IP profile 4 个选择器
- ❌ OAuth / magic link / 邮件发送
- ❌ 付费 / Stripe
- ❌ MCN 白标
- ❌ 多语言
- ❌ 移动端适配

---

## 五、架构

### 5.1 包结构（M1 终态）

**新建**:
```
apps/external/                              (NEW Next.js 14 app)
├── app/
│   ├── login/page.tsx                       (creator 登录)
│   └── (creator)/
│       ├── dashboard/page.tsx               (creator home: 历史 + 新建 topic)
│       └── topic/page.tsx                   (生成 topic 表单)
├── lib/
│   └── api.ts                               (跟 apps/shared/api 对接)
├── middleware.ts                            (auth gate, 复用 console 模式)
├── package.json                             ("@opc/external", Next.js 14.2.5)
├── next.config.js
├── tsconfig.json
└── .env.example

apps/api/internal/handlers/
└── creator.go                                (NEW: 4 个 admin endpoint)

apps/console/app/(console)/
└── creators/
    ├── page.tsx                              (NEW: operator 创作者管理页)
    └── _components/
        ├── CreateCreatorDialog.tsx           (NEW: 创建 modal)
        └── ResetPasswordDialog.tsx           (NEW: 重置密码 modal)
```

**修改**:
```
apps/api/internal/
├── models/user.go                            (加 Role/Disabled/CreatedBy 字段)
├── db/db.go                                  (AutoMigrate 自动加新列)
├── handlers/auth.go                          (Register/Login/Me response 加 Role 字段)
├── middleware/auth.go                        (加 RequireOperatorRole + RequireCreatorRole)
└── cmd/server/main.go                        (注册新路由 + 新 middleware)

apps/console/app/(console)/
├── layout.tsx                                (侧边栏加 "创作者管理" 链接)
└── docs/operations/auth.md                   (operator 操作手册: 如何创建 beta 创作者)

package.json (root)                           (workspace packages: 加 @opc/external)
pnpm-workspace.yaml                          (apps/* 已有, 自动包含 external)
```

**复用不动**:
- `apps/api/internal/capabilities/topic/` (B M1 done)
- `apps/api/internal/handlers/ai.go:GenerateTopics` (B M1 done, 1 capability endpoint 复用)
- `apps/api/internal/middleware/call_log.go` (现有 cost tracking)
- `apps/api/internal/assetgen/profiles/anthropomorphic/` (默认 IP profile, A M1 done)
- `apps/shared/` (HTTP client lib, apps/web + apps/console 复用)
- `apps/agents/minimax.go` (LLM client)

### 5.2 关键设计决策

| 决策 | 选项 | 选定 | 理由 |
|------|------|------|------|
| App 位置 | 新独立 / 扩展 apps/web / 扩展 apps/console | **新独立 apps/external/** | 3 app 职责清 (demo / operator / external); 跟现有 2 app 完全解耦; 未来加 OAuth / 付费 不影响 console |
| 目标用户 (M1) | Beta 创作者 / 免费付费 2 层 / MCN 合作 / 内部多账号 | **Beta 创作者 (1-10 人, invite-only, 零支付)** | 用户最小化决定; 跟 A/B 一致的 minimum scope; 验证产品比验证商业模式优先 |
| 暴露 capability (M1) | 仅 topic / topic+script / topic+script+asset / 仅 dashboard | **仅 topic** | 跟 B M1 一致; 1 capability 也够验证 "创作者能不能用 UI 走完 1 个 workflow" |
| Auth 机制 | Operator 手动 create + password / Email magic link / 邀请码自注册 / OAuth | **Operator 手动 create + initial password** | 5-7 天能上; 零新 auth 代码; 1-10 人 beta 适用 |
| Auth 中间件 | 新建 vs 复用 RequireAuth | **复用 RequireAuth + 加 2 个 role-check wrapper** | 现有 middleware 已成熟; 加 2 个 12 行 wrapper 而非重写 |
| IP profile 范围 (M1) | 1 个固定 (anthropomorphic) / 4 个选择器 | **1 个固定** | 简化 UI; M2 加 4 个下拉 |
| Topic 历史 (M1) | 持久化 (新 model) / 不持久化 (call_log 已记 cost) | **不持久化 (M1)** | call_log 已记 cost; 创作者 UI 简化; M2 加 `creator_topic` model |
| 数据隔离 | 创作者仅看自己 / 全公开 (MVP 1-10 人) | **创作者仅看自己 (M2) / M1 无历史 (跳过)** | M1 不需要; 设计预留 |
| 密码 hashing | 复用 bcrypt (现有) / argon2id | **复用 bcrypt** | 现有 auth 体系已用 bcrypt cost 12; 复用 |
| 国际化 | zh / en / 双语 | **zh only (M1)** | 跟 Phase 1 内部 UI 一致; M2/i18n 再说 |
| Console 集成度 | 完全独立 / 复用 console 路由 | **复用 console 路由: /creators 加 1 个** | console 已有 operator 视角, 加 1 个 page 不破坏 |

### 5.3 User Model Schema (修改后)

```go
// User 现 有 字段 (Phase 1):
//   ID, Email, PasswordHash, CreatedAt, UpdatedAt, LastLoginAt, Disabled

// M1 新增 3 字段:
type User struct {
    gorm.Model

    Email        string `gorm:"uniqueIndex;not null" json:"email"`
    PasswordHash string `gorm:"not null" json:"-"`

    // M1 新增 — Role 区分 operator 跟 creator
    Role         string `gorm:"default:operator;not null" json:"role"`  // "operator" | "creator"

    // M1 新增 — 软删支持
    Disabled     bool   `gorm:"default:false;not null" json:"disabled"`

    // M1 新增 — 创建者审计 (creator 是 哪个 operator 创建; operator 是 0)
    CreatedBy    uint   `gorm:"default:0;not null" json:"created_by"`

    // 现有字段保留
    LastLoginAt  *time.Time `json:"last_login_at"`
}
```

**Role 语义**:
- `"operator"` (默认) — OPC 内部 operator, 走 apps/console, 调 `/api/admin/*`
- `"creator"` — 外部 beta 创作者, 走 apps/external, 调 `/api/ai/*` (现有 B endpoint)

**Disabled 语义**:
- `false` (默认) — 账号可用
- `true` — 软删, 任何端点返 403 "account disabled"

**CreatedBy 语义**:
- `0` — operator 自己创建 (Phase 1 老 operator 全是 0)
- `>0` — 这是个 creator, 记录是哪个 operator 创建的 (审计用)

### 5.4 App 三职责清 (M1 终态)

| App | Auth | 后台 endpoint | UI 视角 |
|-----|------|--------------|---------|
| `apps/web/` | 无 (demo) | 无 (前端 demo) | OPC 团队 demo 选题/脚本/日历/dashboard/知识库 |
| `apps/console/` | operator 登录 | 8 个 admin endpoint + 2 个 topic HTTP | OPC operator 后台: billing/credentials/logs/observability/creators/... |
| `apps/external/` | creator 登录 | **无新加** (复用 /api/ai/*) | 创作者 dashboard: 历史 + 新建 topic |

**关键**: `apps/external/` 本身**不加新 endpoint**, 仅复用 `apps/api` 的现有 + 新加 4 个 admin endpoint (operator-only)。

---

## 六、组件

### 6.1 Operator-Side: `apps/api/internal/handlers/creator.go` (new)

4 个 endpoint, 全部走 `RequireOperatorRole` middleware:

```go
// POST /api/admin/creators — Operator 创建 creator
// Body: {email: string, password: string}
// Response 201: {user: {id, email, role, created_by, created_at}}
// Errors: 400 (invalid body), 409 (email already exists), 401/403 (not operator)
func (h *CreatorHandler) CreateCreator(c *gin.Context) {
    // 1. Bind + validate
    var req struct {
        Email    string `json:"email" binding:"required,email"`
        Password string `json:"password" binding:"required,min=8"`
    }
    if err := c.ShouldBindJSON(&req); err != nil { ... 400 ... }
    // 2. Hash password
    hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
    if err != nil { ... 500 ... }
    // 3. Get operator ID from session
    operatorID := c.GetUint("user_id")
    // 4. Create user
    u := models.User{
        Email: req.Email,
        PasswordHash: string(hash),
        Role: "creator",
        CreatedBy: operatorID,
    }
    if err := h.db.Create(&u).Error; err != nil {
        if strings.Contains(err.Error(), "UNIQUE constraint") {
            c.JSON(http.StatusConflict, gin.H{"error": "email already exists"})
            return
        }
        ... 500 ...
    }
    c.JSON(http.StatusCreated, gin.H{"user": u})
}

// GET /api/admin/creators — Operator 列出所有 creator
// Response 200: {creators: [{id, email, role, created_by, created_at, last_login_at, disabled}]}
func (h *CreatorHandler) ListCreators(c *gin.Context) {
    var creators []models.User
    if err := h.db.Where("role = ?", "creator").Order("created_at DESC").Find(&creators).Error; err != nil {
        ... 500 ...
    }
    c.JSON(http.StatusOK, gin.H{"creators": creators})
}

// POST /api/admin/creators/:id/reset-password — Operator 重置 creator 密码
// Body: {new_password: string}
// Response 200: {ok: true}
// Errors: 404 (not found), 400 (not a creator), 401/403 (not operator)
func (h *CreatorHandler) ResetPassword(c *gin.Context) {
    id := c.Param("id")
    var req struct {
        NewPassword string `json:"new_password" binding:"required,min=8"`
    }
    if err := c.ShouldBindJSON(&req); err != nil { ... 400 ... }
    var u models.User
    if err := h.db.First(&u, id).Error; err != nil { ... 404 ... }
    if u.Role != "creator" { ... 400 "not a creator" ... }
    hash, _ := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
    u.PasswordHash = string(hash)
    h.db.Save(&u)
    c.JSON(http.StatusOK, gin.H{"ok": true})
}

// POST /api/admin/creators/:id/disable — Operator 软删 creator
// Response 200: {ok: true, disabled: true}
func (h *CreatorHandler) DisableCreator(c *gin.Context) {
    id := c.Param("id")
    var u models.User
    if err := h.db.First(&u, id).Error; err != nil { ... 404 ... }
    if u.Role != "creator" { ... 400 ... }
    u.Disabled = true
    h.db.Save(&u)
    c.JSON(http.StatusOK, gin.H{"ok": true, "disabled": true})
}
```

### 6.2 Middleware: `apps/api/internal/middleware/auth.go` (modify)

新增 2 个 wrapper, 跟现有 `RequireAuth` 串联:

```go
// RequireOperatorRole must be used AFTER RequireAuth. Verifies
// the authenticated user has role="operator" (default role for
// OPC internal accounts).
func RequireOperatorRole() gin.HandlerFunc {
    return func(c *gin.Context) {
        role, ok := c.Get("user_role")
        if !ok {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "no user role in context"})
            return
        }
        if role.(string) != "operator" {
            c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "operator role required"})
            return
        }
        c.Next()
    }
}

// RequireCreatorRole must be used AFTER RequireAuth. Verifies
// the authenticated user has role="creator" (Phase 2 external
// beta test accounts).
func RequireCreatorRole() gin.HandlerFunc {
    return func(c *gin.Context) {
        role, ok := c.Get("user_role")
        if !ok {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "no user role in context"})
            return
        }
        if role.(string) != "creator" {
            c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "creator role required"})
            return
        }
        c.Next()
    }
}
```

**配套修改** (`RequireAuth` 加 1 行): 现 有 `RequireAuth` 调 用 时, 调 `session.GetUserRole(userID)` 后 写 `c.Set("user_role", user.Role)`, 让 role-check 能 用.

### 6.3 App `apps/external/` (new Next.js 14)

**`package.json` 关键 deps**:
- `next@14.2.5` (跟 apps/web + apps/console 同步)
- `react@18.3.1` + `react-dom@18.3.1`
- `@opc/shared` (workspace dep, 复用)
- 现有 Tanstack/axios 都 不要, M1 用 最小 stack (fetch + useState + useRouter)

**`app/login/page.tsx` (creator 登录)**:
```tsx
"use client";
import { useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";

export default function LoginPage() {
    const router = useRouter();
    const search = useSearchParams();
    const next = search.get("next") || "/dashboard";
    const [email, setEmail] = useState("");
    const [password, setPassword] = useState("");
    const [err, setErr] = useState("");
    const [loading, setLoading] = useState(false);

    async function submit(e: React.FormEvent) {
        e.preventDefault();
        setLoading(true);
        setErr("");
        const res = await fetch("/api/auth/login", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ email, password }),
            credentials: "include",
        });
        if (res.ok) {
            const data = await res.json();
            // Backend sets opc_session cookie automatically
            router.push(next);
        } else {
            const errBody = await res.json().catch(() => ({}));
            setErr(errBody.error || `login failed (${res.status})`);
        }
        setLoading(false);
    }

    return (
        <form onSubmit={submit}>
            <h1>创作者登录</h1>
            <input
                type="email" value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="email" required
            />
            <input
                type="password" value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="password" required
            />
            {err && <p style={{ color: "red" }}>{err}</p>}
            <button type="submit" disabled={loading}>
                {loading ? "登录中..." : "登录"}
            </button>
        </form>
    );
}
```

**`app/(creator)/dashboard/page.tsx`**:
- 列 历史 (M1: 不持久化, 显示 "M1 暂未持久化 topic 历史; 你的选题记录在 /observability 由 OPC team 监控" placeholder)
- "新建选题" 按钮 → `/topic`
- 显示 当前用户 email + role
- 登出按钮

**`app/(creator)/topic/page.tsx`**:
- 表单: seed (必填), platform (3 个按钮: 抖音/哔哩哔哩/小红书), count (默认 5, max 20)
- IP profile 显示: 固定 anthropomorphic (M1 限定)
- 提交 → `POST /api/ai/topics` (现有 B endpoint, 不新加)
- response 渲染: 5 个 topic 卡片 (title / angle / hook / expected_performance)
- 失败 显示 错误 (401 → 重定向 login, 502 → "服务暂不可用, 请稍后再试")

**`middleware.ts`** (跟 console 模式 一致):
```ts
import { NextRequest, NextResponse } from "next/server";

const SESSION_COOKIE = "opc_session";
const PUBLIC_PATHS = ["/login"];

export function middleware(req: NextRequest) {
    const { pathname } = req.nextUrl;
    if (PUBLIC_PATHS.includes(pathname)) {
        return NextResponse.next();
    }
    const session = req.cookies.get(SESSION_COOKIE);
    if (!session) {
        const url = req.nextUrl.clone();
        url.pathname = "/login";
        url.searchParams.set("next", pathname);
        return NextResponse.redirect(url);
    }
    return NextResponse.next();
}

export const config = {
    matcher: ["/((?!api|_next/static|_next/image|favicon.ico).*)"],
};
```

**注意**: middleware 只验 session cookie **存在**, 不验 role. 创作者 调 /api/admin/creators 仍 会被 server `RequireOperatorRole` 拒 (返 403). UI 不 显式隐藏 /admin link, 因为 创作者 不会 主动 看 /admin 路径.

### 6.4 Console 创作者管理页: `apps/console/app/(console)/creators/page.tsx`

**Operator-only 页面** (`middleware.ts` 加 `RequireOperatorRole`):

**UI 元素**:
- Table: 列 所有 creator (id, email, created_by, created_at, last_login_at, disabled)
- "创建创作者" 按钮 → modal
  - 字段: email, initial password (8+ char)
  - 提交 → `POST /api/admin/creators` → 表刷新
- Per-row 操作:
  - "重置密码" → modal → 调 `POST /api/admin/creators/:id/reset-password`
  - "禁用" → confirm dialog → 调 `POST /api/admin/creators/:id/disable`

**复用 `apps/console` 现有 UI 库** (table, modal, dialog).

### 6.5 Test 覆盖矩阵

| 层 | 覆盖 | 文件 | 数量 |
|----|------|------|------|
| Unit: User model | Role default "operator" / Disabled default false / CreatedBy default 0 | models_test.go | 3 |
| Unit: handler creator.go | CreateCreator 201 / 400 invalid / 409 dup / 403 non-operator; ListCreators 200; ResetPassword 200 / 404 / 400 non-creator; DisableCreator 200 | creator_test.go | 8-10 |
| Unit: middleware | RequireOperatorRole: operator pass / creator fail / unauth fail / disabled fail; RequireCreatorRole: creator pass / operator fail / unauth fail / disabled fail | auth_test.go | 9 |
| Integration: E2E | 5 步: operator 创建 creator → creator 登录 → 生成 topic → 历史 placeholder → 调 admin 拒 403 | (smoke) | 1 |
| apps/external (UI) | 不 写 unit test (无 playwright); 1 个 smoke `curl -I /login` 返 200 | (smoke) | 1 |
| 老 endpoint 不破 | 跑 全 API tests, 0 回归 | 全套 | (12 existing) |

**Coverage 目标**:
- 父包 `internal/handlers` ≥ 80% (现有 baseline 满足)
- 父包 `internal/middleware` ≥ 80% (现有 baseline 满足)
- 父包 `internal/models` ≥ 80% (现有 baseline 满足)
- NEW: `creator.go` handler ≥ 80%
- NEW: 2 个 Require*Role middleware ≥ 80%

---

## 七、Schema 设计

### 7.1 User Model 新增字段 (重复, 便于 reference)

```go
Role      string `gorm:"default:operator;not null" json:"role"`  // "operator" | "creator"
Disabled  bool   `gorm:"default:false;not null" json:"disabled"`
CreatedBy uint   `gorm:"default:0;not null" json:"created_by"`
```

GORM AutoMigrate 检测到新字段自动加列 (SQLite, 开发环境). 老 operator 用户 (`role=""`) 会被 GORM 设 默认 值 "operator". 实 测需验.

### 7.2 HTTP Wire Schema

**`POST /api/admin/creators`**
- Request: `{"email": "string (RFC 5322)", "password": "string (>= 8 chars)"}`
- Response 201: `{"user": {"id": 42, "email": "...", "role": "creator", "created_by": 1, "created_at": "2026-06-15T08:00:00Z", "disabled": false}}`
- Response 400: `{"error": "invalid request body" | "email is required" | "password must be >= 8 chars"}`
- Response 401: `{"error": "unauthorized"}` (RequireAuth)
- Response 403: `{"error": "operator role required"}` (RequireOperatorRole)
- Response 409: `{"error": "email already exists"}` (UNIQUE constraint)

**`GET /api/admin/creators`**
- Response 200: `{"creators": [{...user with role="creator"}]}`

**`POST /api/admin/creators/:id/reset-password`**
- Request: `{"new_password": "string (>= 8 chars)"}`
- Response 200: `{"ok": true}`
- Response 404: `{"error": "creator not found"}`
- Response 400: `{"error": "not a creator"}` (User.Role != "creator")

**`POST /api/admin/creators/:id/disable`**
- Response 200: `{"ok": true, "disabled": true}`

### 7.3 现有 endpoint Response 加 Role 字段

**`GET /api/auth/me`** (修改):
- Response 200: `{"user": {"id": 1, "email": "operator@opc.local", "role": "operator" | "creator", "created_by": 0, ...}}`
- M1 兼容: 老 client 不读 role 字段不影响

**`POST /api/auth/login`** (修改):
- Response 200: 同上 (response 顶部加 `role` 字段)

---

## 八、Consistency Check 设计

N/A — C 是 web UI, 无新的一致性检查。Consistency check 仍属 Sub-Spec A 的 IP profile 范围。

---

## 九、Prompt 构建

N/A — C 不新加 capability, 复用 B 的 `topic.Generate` library (B 内部调 `agents.GenerateTopicsPrompt`)。M1 创作者 UI 提交表单 → `POST /api/ai/topics` → handler 调 library → 现有 prompt.

---

## 十、数据流

### 10.1 启动期

无 启动期 依赖. M1 不加 init() 或 registry. 现有 `internal/db/db.go` 启动时 AutoMigrate User model, 新列 自动加.

### 10.2 运行时: Operator 创建 Creator

```
Operator 在 apps/console/creators → 填 email + password → Submit
  ↓
1. POST /api/admin/creators (operator session)
   ↓ RequireAuth (现有)
   ↓ RequireOperatorRole (新)
2. CreatorHandler.CreateCreator
   2.1 Bind + validate (email, password >= 8)
   2.2 bcrypt hash password
   2.3 INSERT INTO users (email, password_hash, role='creator', created_by=<operator_id>)
   2.4 Return 201 {user: {...}}
3. console /creators 刷新 table, 新 创作者 1 行
```

### 10.3 运行时: Creator 登录 + 生成 Topic

```
Creator 在 apps/external/login → 填 email + password → Submit
  ↓
1. POST /api/auth/login (现有 endpoint, 加 Role 字段)
   ↓ bcrypt compare password
   ↓ Set session cookie (opc_session)
   ↓ Update last_login_at
2. Response 200 {user: {id, email, role: 'creator', ...}}
3. 创作者 重定向 /dashboard
4. /dashboard → 看到 1 个 "新建选题" 按钮
5. 点 /topic → 填 seed / platform / count → Submit
6. POST /api/ai/topics (现有 B endpoint, 走 B refactor 后的 library)
   ↓ RequireAuth
   ↓ call_log middleware (记 cost)
7. response {topics: [{title, angle, hook, expected_performance, ...}]}
8. UI 渲染 5 个 topic 卡片
9. 不 持久化 (M1). call_log 仍 记 1 行 (G6 closed 2026-06-09 行为)
```

### 10.4 运行时: Creator 调 Admin Endpoint (拒绝)

```
Creator 调 POST /api/admin/creators
  ↓
1. POST /api/admin/creators (creator session)
   ↓ RequireAuth (通过, 创作者 有 session)
   ↓ RequireOperatorRole (拒, role='creator' != 'operator')
2. 403 {"error": "operator role required"}
3. UI 不会调 (apps/external 没 admin link), 防御性 depth
```

---

## 十一、错误处理

| 错误 | 检测 | 处理 |
|------|------|------|
| 创作者 调 /api/ai/* 但 session 失效 | middleware | 401 + UI 重定向 /login |
| 创作者 调 /api/admin/* | RequireOperatorRole | 403 (UI 不 调, defense) |
| Operator 调 /api/admin/* 但未登录 | RequireAuth | 401 (重定向 /login) |
| 创建 creator email 重复 | DB UNIQUE | 409 "email already exists" |
| 创作者 disabled=true 调任何端点 | (新加) middleware check | 403 "account disabled" |
| API key 缺失 (LLM call) | minimax.Available() | 502 + UI "服务暂不可用" |
| Topic 生成 失败 (rate limit / timeout) | library wrap | 502 + UI 具体错误 |
| 创作者 提交 invalid 表单 (count > 20) | library validate | 400 + UI 字段错 |
| DB connection 失败 | 各 handler 顶部 check | 500 + log error |
| AutoMigrate 新列失败 (老 DB schema 不兼容) | 启动期 | fatal log + server 不启动 |

---

## 十二、测试策略

### 12.1 Unit Tests (apps/api/internal/)

**`models/user_test.go`** (新增 3 case):
```go
func TestUserDefaultRole(t *testing.T) {
    u := models.User{Email: "x@x.com", PasswordHash: "h"}
    if u.Role != "" { t.Errorf("default Role = %q, want empty (GORM sets default)", u.Role) }
    // After Save + Reload, Role should be "operator"
}

func TestUserDefaultDisabled(t *testing.T) {
    u := models.User{Email: "x@x.com", PasswordHash: "h"}
    if u.Disabled != false { t.Errorf("default Disabled = %v, want false", u.Disabled) }
}

func TestUserDefaultCreatedBy(t *testing.T) {
    u := models.User{Email: "x@x.com", PasswordHash: "h"}
    if u.CreatedBy != 0 { t.Errorf("default CreatedBy = %d, want 0", u.CreatedBy) }
}
```

**`handlers/creator_test.go`** (新增 8-10 case):
```go
func TestCreateCreator201(t *testing.T)        { /* valid email+password → 201 + user.role='creator' */ }
func TestCreateCreator400InvalidEmail(t *testing.T) { /* missing @ → 400 */ }
func TestCreateCreator400ShortPassword(t *testing.T) { /* 7 chars → 400 */ }
func TestCreateCreator409Duplicate(t *testing.T) { /* existing email → 409 */ }
func TestCreateCreator403NonOperator(t *testing.T) { /* creator session → 403 */ }
func TestListCreators200(t *testing.T)         { /* operator session → 200 + array */ }
func TestResetPassword200(t *testing.T)       { /* operator + existing creator → 200 + new hash */ }
func TestResetPassword404(t *testing.T)       { /* operator + nonexistent id → 404 */ }
func TestResetPassword400NotCreator(t *testing.T) { /* operator + user.role='operator' → 400 */ }
func TestDisableCreator200(t *testing.T)       { /* operator + existing creator → 200 + disabled=true */ }
```

**`middleware/auth_test.go`** (新增 9 case):
```go
func TestRequireOperatorRoleOperatorPass(t *testing.T) { /* role='operator' → 200 */ }
func TestRequireOperatorRoleCreatorFail(t *testing.T)  { /* role='creator' → 403 */ }
func TestRequireOperatorRoleUnauthFail(t *testing.T)   { /* no session → 401 */ }
func TestRequireOperatorRoleDisabledFail(t *testing.T)  { /* disabled=true → 403 */ }
func TestRequireCreatorRoleCreatorPass(t *testing.T)   { /* role='creator' → 200 */ }
func TestRequireCreatorRoleOperatorFail(t *testing.T)  { /* role='operator' → 403 */ }
func TestRequireCreatorRoleUnauthFail(t *testing.T)    { /* no session → 401 */ }
func TestRequireCreatorRoleDisabledFail(t *testing.T)   { /* disabled=true → 403 */ }
```

### 12.2 E2E Smoke (`scripts/api-smoke.sh` 加 4 case)

```bash
# === Sub-Spec C M1: external creator flow ===
section "external creator"

# 1. Operator creates a creator (uses existing operator session)
out=$(curl -s -X POST http://localhost:8081/api/admin/creators \
    -H "Content-Type: application/json" \
    -H "Cookie: opc_session=$OPERATOR_SESSION_TOKEN" \
    -d '{"email":"creator1@beta.com","password":"beta-pass-123"}')
creator_id=$(echo "$out" | jq '.user.id' 2>/dev/null)
if [ -n "$creator_id" ] && [ "$creator_id" != "null" ]; then
    pass "admin: creator created (id=$creator_id)"
else
    fail "admin: creator creation failed (raw: ${out:0:200})"
fi

# 2. Creator logs in
creator_out=$(curl -s -X POST http://localhost:8081/api/auth/login \
    -H "Content-Type: application/json" \
    -d '{"email":"creator1@beta.com","password":"beta-pass-123"}' \
    -c /tmp/opc-creator-cookies.txt)
creator_session=$(grep opc_session /tmp/opc-creator-cookies.txt | awk '{print $7}')
if [ -n "$creator_session" ]; then
    pass "creator: login successful"
else
    fail "creator: login failed (raw: ${creator_out:0:200})"
fi

# 3. Creator generates a topic
out=$(curl -s -X POST http://localhost:8081/api/ai/topics \
    -H "Content-Type: application/json" \
    -H "Cookie: opc_session=$creator_session" \
    -d '{"seed":"个人成长","platform":"抖音","count":5}')
topic_count=$(echo "$out" | jq '.topics | length' 2>/dev/null)
if [ "$topic_count" = "5" ]; then
    pass "creator: topic HTTP returns 5 topics"
else
    fail "creator: topic HTTP got $topic_count, want 5"
fi

# 4. Creator tries to call admin endpoint (should be 403)
out=$(curl -s -o /dev/null -w "%{http_code}" -X POST \
    http://localhost:8081/api/admin/creators \
    -H "Content-Type: application/json" \
    -H "Cookie: opc_session=$creator_session" \
    -d '{"email":"x@x.com","password":"y-pass-123"}')
if [ "$out" = "403" ]; then
    pass "creator: admin endpoint correctly 403"
else
    fail "creator: admin endpoint returned $out, want 403"
fi
rm -f /tmp/opc-creator-cookies.txt
```

### 12.3 apps/external (UI) Smoke

```bash
# === apps/external UI smoke ===
section "apps/external UI"
status=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:3002/login)
if [ "$status" = "200" ]; then
    pass "apps/external /login returns 200"
else
    fail "apps/external /login returned $status, want 200"
fi
status=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:3002/dashboard)
if [ "$status" = "307" ]; then
    pass "apps/external /dashboard correctly redirects (no session)"
else
    fail "apps/external /dashboard returned $status, want 307 redirect"
fi
```

(`apps/external` 跑在 :3002, 跟 apps/web :3000 + apps/console :3001 区分)

---

## 十三、实施计划（M1 timeline）

| 周 | 任务 | 交付物 |
|----|------|--------|
| W1 | Task 1: User model 加 3 列 + AutoMigrate + 3 unit test | 老 operator 用户 role 默认 'operator' 验证 |
| W1 | Task 2: 4 个 admin endpoint (creator.go) + 2 个 middleware (Require*Role) + 17-19 unit test | 现有 5+1 handler smoke + 老 9 auth test 0 回归 |
| W2 | Task 3: console /creators 管理页 + 2 个 dialog + 调 4 个 admin endpoint | 完整 operator workflow (建 1 创作者 走通) |
| W2 | Task 4: apps/external (新 Next.js 14) + login + dashboard + topic + middleware | `pnpm build` 0 错, `/login` 返 200, `/dashboard` 返 307 (无 session) |
| W3 | Task 5: scripts/api-smoke.sh 加 4 case + verify-project-state + 状态文件更新 | 16/16 PASS, G15 CLOSED |

**W3 末 verify agent 必跑**, OVERALL PASS 才算 M1 完。

---

## 十四、风险与不做的事

### 14.1 风险

| 风险 | 缓解 |
|------|------|
| User model 加列 破老 operator 用户 (AutoMigrate 失败 / 默认 值 不生效) | Task 1 加 3 unit test 验 默认 值; 老 operator 用户 (role="") 被 GORM 改 "operator" 是 non-destructive |
| 新增 4 个 admin endpoint 命名冲突 (跟现有 8 个 console endpoint) | 全部 前缀 `/api/admin/creators*` (跟现有 `api/billing/credentials/logs/observability/...` 平级) |
| 新增 2 个 middleware 跟现有 RequireAuth 串联 顺序错 | docs 注释 明确 "must be used AFTER RequireAuth"; unit test 验 3 个 串联 顺序 path |
| apps/external 跟 apps/web + apps/console 共享 端口 3000/3001 | apps/external 跑 :3002 (3 个 app 端口 不冲突); `next.config.js` 配 `port: 3002` |
| DB AutoMigrate 新列 在 生产 失败 (e.g. NOT NULL on existing rows) | 现有 字段 都 有 default, GORM 自动 apply default on existing rows; Task 1 unit test 验 default value 正确 |
| Operator 误 改自己 role (从 operator 改 creator) 导致 不能 进 console | 列出 / 改 / 删 endpoint 全部 拒绝 修改 "operator" role 的 user; 只 允许 role='creator' 操作 |
| Topic 不 持久化 创作者 看不到自己 历史 | M1 接受 (call_log 已记 cost + topic 内容); M2 加 creator_topic model; 创作者 M1 UI 显示 "M1 暂未持久化历史" placeholder |
| apps/external 走 fetcher 调 /api/ai/topics 但 session cookie 不共享 (不同 port + 域) | dev 环境 localhost:3002 调 localhost:8081 (同源) cookie 自动带; prod 同域 同 session; 本地 smoke 验证 |
| 老 auth.go Register endpoint 加 Role 字段 后 老 client (apps/web) 登 录 受 影响 | M1 改 Register 也 加 role='operator' 默认; 老 client 不读 role 字段 不 受 影响; 改 GET /api/auth/me 返 role 但不破坏 老 client 读 id/email |

### 14.2 不做的事（M1 范围硬约束）

- ❌ 4 capability 暴露 (script/deconstruct/asset 都没, 等 B M2+)
- ❌ Topic 历史持久化 (M1 不加 creator_topic model, call_log 记录)
- ❌ IP profile 4 个选择器 (M1 默认 anthropomorphic 固定)
- ❌ OAuth / Magic link / 邮件发送
- ❌ 付费 / Stripe
- ❌ MCN 白标
- ❌ 多语言 (先中文)
- ❌ 移动端适配 (先桌面)
- ❌ 改 老 handler endpoint 行为 (除 加 Role 字段 外)
- ❌ 改 老 User model 字段 (除加 3 列 外)
- ❌ 改 apps/web 跟 apps/console 任何 5+ page 行为 (除 console /creators 1 个新 page 外)
- ❌ 重写 session middleware (复用 RequireAuth, 加 2 个 role wrapper)
- ❌ 改 老 auth.go 业务逻辑 (仅加 Role 字段 到 response)

---

## 十五、与 Sub-Spec A / B 既有代码的关系

| 已有 | M1 处置 | 理由 |
|------|--------|------|
| `internal/assetgen/profiles/anthropomorphic/` (A M1) | 不动 | 创作者 UI 默认用 anthropomorphic IP profile |
| `internal/capabilities/topic/topic.go` (B M1) | 不动 | 创作者 UI 调现有 `POST /api/ai/topics`, handler 已重构调 library |
| `internal/handlers/ai.go:GenerateTopics` (B M1) | 不动 | 复用, 创作者 跟 operator 走同一 endpoint |
| `internal/handlers/auth.go` | modify 仅 加 Role 字段 到 response | 老 client 不读 role 字段 不受影响 |
| `internal/models/{user,session}.go` | modify User 加 3 列 | AutoMigrate 自动加, 老 operator 用户 role 改 "operator" |
| `internal/middleware/auth.go:RequireAuth` | modify 加 `c.Set("user_role", user.Role)` 1 行 | 让 2 个新 Require*Role 能查 role |
| `internal/middleware/call_log.go` | 不动 | 老 cost tracking 自动覆盖 创作者 调 /api/ai/* 的 cost |
| `apps/console/` | modify 加 /creators page + 侧边栏 link | 1 个新 page 不破坏老 8 个 endpoint |
| `apps/web/` | 不动 | 内部 demo 不涉及 |
| `apps/shared/` | 不动 | apps/external 复用 @opc/shared/api (现成) |
| pnpm-workspace.yaml | 不动 | `apps/*` glob 自动包含 external |
| `apps/api/cmd/server/main.go` | modify 注册新路由 + 新 middleware | 4 个新 endpoint + 2 个新 middleware 串联 |

---

## 十六、与 Phase 2 其他 sub-spec 的关系

| Sub-spec | 与本 sub-spec 的关系 |
|----------|---------------------|
| A (IP profile) | M1 done ✅。M1 创作者 UI 默认 anthropomorphic; M2 暴露 4 个 IP type 选择器 (本 sub-spec M2) |
| B (4 能力工具化) | M1 done ✅。M1 创作者 UI 仅暴露 topic (B M1); M2 加 script/deconstruct/asset capability (B M2) 时, C M2 加对应 UI 页面 |
| C (对外 web UI) | 本 sub-spec。M1 = invite-only beta + 1 capability; M2 加 4 capability + 付费 |
| D (agent-agnostic 平台) | 跟 C 并行。D 是 agent 调用层, 不依赖 web UI; C 是 web UI, 不调 D |
| E (反 AI 检测 / 拟人化) | 跟 C 弱关联 (拟人化 改 prompt 模板, 不直接影响 C); E M1 是给 4 能力 的 consistency check 加拟人化 token, C M1 调 topic 不受 E M1 影响 |

**关键路径** (按 Phase 1 spec §16):
```
A ✅ → B ✅ → C (本) → C M2 (4 cap + 付费)
       ↓
       D (parallel, 跟 C 互不依赖)
       ↓
       E (parallel, 需 A consistency check)
```

C M1 完成后, 即可启 D (agent-agnostic) 跟 E (反 AI 检测) 并行。

---

## 十七、变更记录

| 版本 | 日期 | 变更 | 作者 |
|------|------|------|------|
| v1.0 | 2026-06-15 | 初稿（基于 brainstorming 4 问澄清：Beta 创作者 invite-only / 仅 topic 1 capability / 新独立 apps/external/ app / Operator 手动 create creator 账号） | OPC + Claude |

---

**下一步**：
- 用户审阅本文档，提出修改意见
- 通过后进入 Spec 自审（检查占位符/矛盾/歧义/范围）
- 然后调用 `writing-plans` skill，把 sub-spec C 拆为可执行的 M1 实施计划（5 task）
- M1 完成后，启 D / E sub-spec 的 brainstorming → spec → plan 循环
