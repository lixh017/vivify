# OPC Phase 2 — Agent-Agnostic 平台设计（Sub-Spec D）

> **文档版本**：v1.0（初稿，待审）
> **日期**：2026-06-15
> **作者**：OPC 团队（与 Claude 协作）
> **状态**：设计阶段 → 待用户审阅
> **路径**：`docs/superpowers/specs/2026-06-15-opc-phase2-agent-agnostic-design.md`
> **父 spec**：`docs/superpowers/specs/2026-06-03-opc-phase1-ip-design.md` §3.2（Phase 2 6-12 月）、§16（关键路径）
> **依赖 spec**：
> - `docs/superpowers/specs/2026-06-11-opc-phase2-ip-template-design.md` v1.0（Sub-Spec A done）
> - `docs/superpowers/specs/2026-06-12-opc-phase2-capabilities-design.md` v1.0（Sub-Spec B done）
> - `docs/superpowers/specs/2026-06-15-opc-phase2-external-ui-design.md` v1.0（Sub-Spec C done）
> **兄弟 spec**：M1 5 sub-spec 之 **D**（E 待启）

---

## 一、文档目的

给 OPC 平台加 **agent 认证层**（API key），让非-人类 agent（Claude Code / 自研 agent / MCP 客户端 / OpenAI function-calling 客户端）能通过 `X-API-Key` header 调现有 HTTP/MCP 表面，**不动**现有 capability library、不新加 capability-side code。Sub-Spec A/B/C 的所有 HTTP endpoint 现状已支持 agent 调用（仅缺 auth 机制），D M1 把这个 gap 补上。

D M1 走最小范围：1 capability 暴露面（现有 `/api/ai/topics` 复用）+ 1 个新模型（`Agent`）+ 1 个新 middleware（`RequireAgentKey`）+ 4 个 admin endpoint + console 简易管理页。**不**加 OpenAI function-calling 兼容 API、**不**加 SDK library、**不**扩 MCP tool 集合——M2/M3 再说。

Sub-Spec D 是 Phase 2 critical path 外**横向能力**，跟 A/B/C serial 不同。**D M1 完成后，agent 能 用 `X-API-Key` 调 现有 B 能力，平台正式对 agent ecosystem 开放。**

---

## 二、Executive Summary

| 项 | 内容 |
|----|------|
| **目标** | 给 OPC 平台加 agent 认证（API key）。1 个新 `models.Agent` model + bcrypt 加密 + 1 个 `RequireAgentKey` middleware + 4 个 admin endpoint (operator 管 API key) + console `/api-keys` 简易管理页。 |
| **范围（M1）** | 1 capability 暴露（topic 复用现有 `/api/ai/topics`）+ 1 个 X-API-Key auth path + console 管理 UI。 |
| **Agent 种类** | 任何带 `X-API-Key` header 的 HTTP client（MCP server / Claude Code / 自研 agent / OpenAI function-calling 客户端）。M1 不 加 客户端 SDK。 |
| **认证机制** | API key 格式 `opc_agent_<random32>` + bcrypt 加密 存 + `X-API-Key` header 传 + 跟现有 session cookie 兼容（任 一 通过 即可）。 |
| **存储** | 1 个新 GORM 表 `agents`（id / name / hashed_key / created_by / created_at / last_used_at / disabled / scope='all'）。AutoMigrate 加表。 |
| **Cost tracking** | call_log middleware 加 1 列 `actor_type`（`"human"` / `"agent"`），agent 请求 跟 human 一样记 cost_cents。 |
| **不在 M1** | OpenAI function-calling 兼容 API、TypeScript/Python/Go SDK library、per-key 权限 scope（细粒度 capability 控权）、regenerate key 旧 key 1 次显示（spec 列 了 但 M2 优先） |
| **M1 timeline** | 1-2 周（4 task: Agent model + middleware + admin handler + console page） |
| **依赖** | A (IP profile framework, done) + B (capability library, done) + C (User model + 2 middleware, done) + 现有 session auth + 现有 call_log middleware |
| **风险等级** | 🟢 低（auth 复用现有 bcrypt + session pattern；不动 capability 业务） |

---

## 三、背景与现状

### 3.1 Phase 1 + Phase 2 A/B/C 已有 agent 表面

| 表面 | 状态 | 缺什么 |
|------|------|--------|
| MCP server (Phase 1 done, 10 tool) | ✅ 现有 | 无 agent 认证层（任何 MCP client 能连） |
| HTTP API (Phase 1 done, 5+ endpoint) | ✅ 现有 | 无 agent 认证层（仅 human session） |
| CLI `opc-asset` (B M1 done, 5 subcommand) | ✅ 现有 | CLI 跑 在 human laptop, 不 需 agent 认证 |
| Sub-Spec C external web UI (done) | ✅ 现有 | 创作者 human 登录, 不 给 agent |

**关键观察**：D M1 补**单一**缺口——给现有 HTTP + MCP 表面加 `X-API-Key` 认证层，让 agent 能 用 API key 调 一切 capability 端点（`/api/ai/*`）。**不**改 capability 业务、不新加 capability 端点。

### 3.2 Sub-Spec A / B / C 已就绪的复用资产

**A (IP profile 框架) done**:
- `internal/assetgen/` + 4 sub-package, 创作者 跟 agent 调 时 都 走同 一 IP profile (默认 anthropomorphic, M1 1 个)

**B (4 能力 library) M1 done**:
- `internal/capabilities/topic/` + `POST /api/ai/topics` (B M1 端点, 跟 session 兼容, M1 加 1 行 `X-API-Key` check 即可)

**C (User model + 2 role middleware) done**:
- `User` 加 3 列 (Role/Disabled/CreatedBy) + `RequireOperatorRole` + `RequireCreatorRole` 2 个 wrapper
- D M1 直接复用 `RequireOperatorRole` (operator 管 API key)

**现有 auth 资产 (复用)**:
- `internal/middleware/call_log.go` — 加 1 列 `actor_type` 即可
- `internal/middleware/auth.go:RequireAuth` — 现状 验 session cookie, 不动
- bcrypt (现有, 跟 User.PasswordHash 同 `bcrypt.DefaultCost`)

### 3.3 不在 M1 范围（推到 M2/M3）

- ❌ OpenAI function-calling 兼容 surface (`/v1/tools/invoke` 走 OpenAI 协议)
- ❌ TypeScript / Python / Go SDK library
- ❌ Per-key 权限 scope (M1: 1 key 调 所有 capability；M2: 1 key 仅调 1-2 capability)
- ❌ Regenerate key 旧 key 1 次显示（M1: 旧 key 立刻失效, 不 再 显示 完整 值）
- ❌ OAuth / Magic link 邀请
- ❌ MCN / 子代理 权限继承

---

## 四、设计目标（M1 验收）

| # | 验收 | 验证命令 |
|---|------|---------|
| 1 | `models.Agent` model 存在 + 8 列 (id/name/hashed_key/created_by/created_at/last_used_at/disabled/scope) + GORM AutoMigrate | `go test -tags fts5 ./internal/models/...` PASS |
| 2 | 4 admin endpoint (`/api/admin/agents*`) operator-only + 8-10 unit test PASS | `go test -tags fts5 -run TestAgent ./internal/handlers/...` |
| 3 | `RequireAgentKey` middleware 验 X-API-Key + 5 unit test PASS | `go test -tags fts5 -run TestRequireAgentKey ./internal/middleware/...` |
| 4 | console `/api-keys` 管理页 + 1 个 dialog + 编译 PASS | `pnpm build` exit 0 |
| 5 | 现有 `/api/ai/topics` 加 1 行 agent_id check (session 跟 X-API-Key 任 一 即可) | smoke 测试 agent 调 topic 返 200 |
| 6 | call_log middleware 加 `actor_type` 列 + agent 请求 记 cost | smoke 测试 agent cost 走 同一 路径 |
| 7 | E2E smoke 加 4 case (operator create / agent call topic 200 / agent call admin 403 / bad key 401) | `./scripts/api-smoke.sh` 16+4=20 PASS |
| 8 | verify-project-state agent OVERALL: PASS | `~/.claude/audit/verify-reports/verify-*.md` PASS |

**Out of scope（M1 不做）**:
- ❌ OpenAI function-calling 兼容 API
- ❌ SDK library
- ❌ Per-key scope (M1: 所有 key 调 所有 capability)
- ❌ Regenerate 1 次显示 完整 旧 key
- ❌ 任何 新 capability endpoint
- ❌ 改 capability 业务 (B M1 + Phase 1 老 code 全部 不动)

---

## 五、架构

### 5.1 包结构（M1 终态）

**新建**:
```
apps/api/internal/
├── models/agent.go                      (Agent struct + bcrypt 加密 helper)
├── handlers/agent.go                    (4 admin endpoint: create/list/disable/regenerate)
├── handlers/agent_test.go                (8-10 unit test)
├── middleware/agent_auth.go              (RequireAgentKey + tests)
└── middleware/agent_auth_test.go         (5 unit test)

apps/console/app/(console)/
└── api-keys/                            (NEW)
    ├── page.tsx                          (operator API key 管理页)
    └── _components/
        └── CreateAgentDialog.tsx         (生成新 key modal, 显示 1 次完整 key)
```

**修改**:
```
apps/api/internal/
├── db/db.go                              (AutoMigrate 加 &models.Agent{})
├── handlers/ai.go                        (GenerateTopics 加 1 行 agent_id check + call_log actor_type)
├── middleware/call_log.go                (加 1 列 actor_type; 现有 11 个 handler 调用 点不 动)
└── cmd/server/main.go                    (注册 4 新 endpoint + 新 middleware; 现有 /api/ai/topics 加 1 行)

apps/console/
├── components/Sidebar.tsx                (sidebar 加 1 个 "API key 管理" link)
└── middleware.ts                         (现有 operator auth gate 不 动)
```

**复用不动**:
- `internal/assetgen/` (A M1 done)
- `internal/capabilities/topic/` (B M1 done)
- `internal/handlers/ai.go:GenerateTopics` 业务逻辑 (B M1 done)
- `internal/middleware/auth.go:RequireAuth` (现有 session auth)
- `internal/middleware/call_log.go` 业务逻辑 (仅加 1 列)

### 5.2 关键设计决策

| 决策 | 选项 | 选定 | 理由 |
|------|------|------|------|
| M1 范围 | Agent auth only / + MCP / + OpenAI / 全上 | **Agent auth only** | 用户最小化决定; 1 capability 暴露已够 demo agent workflow |
| API key 存放 | 独立 Agent model / 扩 User model / 仅 API endpoint | **独立 Agent model + console 简易页** | 职责清; User model 不被 agent 逻辑污染 |
| Key 格式 | `opc_agent_<random>` / `<random>` / JWT | **`opc_agent_<random32>`** | 跟 GitHub PAT 风格 一致; 32 char random 防 brute force |
| Key 存储 | bcrypt / plaintext / SHA-256 | **bcrypt (`bcrypt.DefaultCost`)** | 跟 User.PasswordHash 复用 同一 import + 同一 cost; 抗 rainbow table |
| 跟前 session 关系 | 替代 / 兼容 (任 一 通过) | **兼容 (任 一 通过)** | 1 个 agent 同时 带 session cookie 不 应 被 拒绝; 老 frontend 仍能 用 session |
| 显示 完整 key | 创建后 1 次 / 永不 | **创建后 1 次** | 跟 GitHub PAT 风格; 之后 仅 prefix (前 12 char) |
| Disabled | 软删 (deleted_at) / 硬删 (DELETE) | **软删 (bool disabled)** | 跟 Sub-Spec C User.Disabled 一致 |
| Regenerate 旧 key | 1 次显示 / 立刻失效 | **立刻失效** (M1 简化) | M2 加 显示 完整 旧 key |
| Per-key scope | 所有 capability / per-key 限 1-2 | **所有 capability (scope='all' 固定)** | M1 简化; 字段预留 M2 加 1-2 capability |
| call_log actor_type | 新列 (varchar) / 推断 (无 key 写 human) | **新列 actor_type** (明确语义, 易 查询) | agent / human 区分 清晰, 审计 / 计费 友好 |

### 5.3 Agent Model Schema

```go
// Agent represents an API key holder (a non-human agent
// representing a 创作者 or OPC team). The full key is shown
// ONCE at creation; only the bcrypt hash is stored.
type Agent struct {
    gorm.Model

    Name        string `gorm:"not null" json:"name"`
    // M1: 显 1 段 prefix (前 12 char) 给 operator 识别. 不 暴露 完整 key.
    KeyPrefix   string `gorm:"not null" json:"key_prefix"`

    // M1: bcrypt hash. JSON 标签 "-" 防止 序列化 泄露.
    HashedKey   string `gorm:"not null;uniqueIndex" json:"-"`

    // 哪个 operator 创建 此 agent. 审计用.
    CreatedBy   uint   `gorm:"not null" json:"created_by"`

    // M2 字段, M1 固定 "all" (所有 capability 都能 调).
    Scope       string `gorm:"default:all;not null" json:"scope"`

    // 软删 旗.
    Disabled    bool   `gorm:"default:false;not null" json:"disabled"`

    // 跟踪 agent 最近 1 次调 API 时间.
    LastUsedAt  *time.Time `json:"last_used_at"`
}
```

**Key 生成** (创建 时):
```go
// generateKey returns "opc_agent_" + 32 random hex chars.
func generateKey() string {
    bytes := make([]byte, 16)  // 16 bytes = 32 hex chars
    if _, err := rand.Read(bytes); err != nil {
        panic("crypto/rand failed")
    }
    return "opc_agent_" + hex.EncodeToString(bytes)
}
// 例: "opc_agent_8x4k2pqrstuvwxyz0123456789ab" (41 char total)
```

**KeyPrefix 提取** (创建 时 + UI 显示 用):
```go
// keyPrefix returns the first 12 chars of a generated key
// ("opc_agent_XY" — enough to identify without leaking secret).
const keyPrefixLen = 12
func keyPrefix(key string) string {
    if len(key) < keyPrefixLen {
        return key
    }
    return key[:keyPrefixLen]
}
```

### 5.4 Auth 兼容 (Session + API Key)

**设计**: 现有 endpoint (e.g. `/api/ai/topics`) 同时 支持 session cookie 跟 X-API-Key header，**任 一 通过 即可**。

**实现** (`internal/handlers/ai.go:GenerateTopics` 现有 handler 加 2 行):
```go
func (h *AIHandler) GenerateTopics(c *gin.Context) {
    // ... 现有 bind + validate + demo + library call ...

    // 新 (Task D M1): agent_id check (跟 user_id 互斥, 至少 1 个 有 值)
    agentID, _ := c.Get("agent_id")  // 由 RequireAgentKey middleware 设
    if c.GetUint("user_id") == 0 && agentID == nil {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
        return
    }

    // ... 现有 cost stamp ...
}
```

**Middleware 串联** (`cmd/server/main.go`):
```go
// 现状 (B + C done):
r.POST("/api/ai/topics", middleware.RequireAuth(), aiHandler.GenerateTopics)

// D M1 后:
apiGroup := r.Group("/api/ai", middleware.RequireAuth(), middleware.AcceptAgentKey())
//   ↑ AcceptAgentKey 跟 RequireAuth 并联: 任 一 通过 设 ctx, 同时通过 也 OK.
//   失败 一个 不 拒 另 一个.

// 内部: 在 handler 顶部 加 1 行 (c.GetUint("user_id") > 0 || c.Get("agent_id") != nil)
```

> **设计简化**: 不加 `AcceptAgentKey` 复合 middleware. 而是在现有 `RequireAuth` 之后 加 1 个 "弱" middleware `MaybeAgentKey` (设 `agent_id` if header present, 不 abort). 然后 handler 顶部 check `user_id OR agent_id`. 这样 老 frontend 不 破, agent 新 client 加 header 即 可.

**最终 middleware 链** (cmd/server/main.go):
```go
r.POST("/api/ai/topics",
    middleware.RequireAuth(),              // session OR 401
    middleware.MaybeAgentKey(),            // 设 agent_id if X-API-Key present
    aiHandler.GenerateTopics)              // 1 行 check: user_id OR agent_id
```

**4 新 admin endpoint** (operator-only, 走 RequireOperatorRole):
- `POST   /api/admin/agents`                — create (返 1 次完整 key)
- `GET    /api/admin/agents`                — list (返 prefix, 不 返完整)
- `POST   /api/admin/agents/:id/disable`    — soft-delete
- `POST   /api/admin/agents/:id/regenerate` — 生成 新 key, 旧 失效

**4 endpoint 全部 走**:
```go
admin := r.Group("/api/admin", middleware.RequireAuth(), middleware.RequireOperatorRole())
agentH := handlers.NewAgentHandler(db)
agentH.RegisterRoutes(admin)
```

### 5.5 Cost Tracking 集成 (call_log 加 actor_type)

**修改** (`internal/middleware/call_log.go`):
```go
type CallLog struct {
    gorm.Model
    RequestID   string
    UserID      uint    // human 调时 设; agent 调时 0
    AgentID     uint    // NEW: agent 调时 设; human 调时 0
    ActorType   string  // NEW: "human" | "agent" (冗余 字段, 索引 优化)
    Skill       string
    InputTokens  int
    OutputTokens int
    CostCents   int
    // ... 现有 字段 不动
}
```

**Middleware 修改** (加 1 段):
```go
// 在 StampClaudeCost 上面 加:
agentID, _ := c.Get("agent_id")
if uid, ok := c.Get("user_id"); ok {
    if aid, ok := agentID.(uint); ok && aid > 0 {
        // agent 调: UserID=0, AgentID=aid, ActorType="agent"
        ref.UserID = 0
        ref.AgentID = aid
        ref.ActorType = "agent"
    } else {
        // human 调: UserID=uid, AgentID=0, ActorType="human"
        ref.UserID = uid.(uint)
        ref.AgentID = 0
        ref.ActorType = "human"
    }
}
```

GORM AutoMigrate 自动加 2 列 (`AgentID`, `ActorType`). 老 call_log row (actor_type="", agent_id=0) 默认 走 "human" 路径 兼容.

---

## 六、组件

### 6.1 `models.Agent` (新建)

```go
// Package models provides the GORM data models for the OPC
// platform. agent.go represents an API key holder (a
// non-human agent representing a 创作者 or OPC team).
package models

import (
    "crypto/rand"
    "encoding/hex"
    "time"

    "golang.org/x/crypto/bcrypt"
    "gorm.io/gorm"
)

type Agent struct {
    gorm.Model

    Name        string     `gorm:"not null" json:"name"`
    KeyPrefix   string     `gorm:"not null" json:"key_prefix"`
    HashedKey   string     `gorm:"not null;uniqueIndex" json:"-"`
    CreatedBy   uint       `gorm:"not null" json:"created_by"`
    Scope       string     `gorm:"default:all;not null" json:"scope"`
    Disabled    bool       `gorm:"default:false;not null" json:"disabled"`
    LastUsedAt  *time.Time `json:"last_used_at"`
}

// GenerateKey returns a fresh opc_agent_<random32> key.
func GenerateKey() (key string, prefix string, err error) {
    bytes := make([]byte, 16)
    if _, err := rand.Read(bytes); err != nil {
        return "", "", err
    }
    key = "opc_agent_" + hex.EncodeToString(bytes)
    return key, keyPrefix(key), nil
}

const keyPrefixLen = 12

func keyPrefix(key string) string {
    if len(key) < keyPrefixLen {
        return key
    }
    return key[:keyPrefixLen]
}

// HashPassword bcrypts the key for storage. Uses bcrypt.DefaultCost
// (12) to match User.PasswordHash.
func HashPassword(key string) (string, error) {
    hash, err := bcrypt.GenerateFromPassword([]byte(key), bcrypt.DefaultCost)
    return string(hash), err
}

// VerifyPassword compares a raw key against the stored bcrypt hash.
func VerifyPassword(hashedKey, rawKey string) bool {
    return bcrypt.CompareHashAndPassword([]byte(hashedKey), []byte(rawKey)) == nil
}
```

### 6.2 `handlers.AgentHandler` (新建, 4 endpoint)

```go
// Package handlers provides HTTP route handlers for the OPC
// platform. agent.go implements the operator-only admin
// endpoints for managing agent API keys (X-API-Key auth path).
package handlers

import (
    "errors"
    "net/http"
    "strconv"
    "time"

    "github.com/gin-gonic/gin"
    "gorm.io/gorm"

    "github.com/opc/api/internal/models"
)

type AgentHandler struct {
    db *gorm.DB
}

func NewAgentHandler(db *gorm.DB) *AgentHandler {
    return &AgentHandler{db: db}
}

func (h *AgentHandler) RegisterRoutes(r gin.IRouter) {
    r.POST("/agents", h.Create)
    r.GET("/agents", h.List)
    r.POST("/agents/:id/disable", h.Disable)
    r.POST("/agents/:id/regenerate", h.Regenerate)
}

// Create handles POST /api/admin/agents.
// Operator-only. Returns 201 + agent + FULL key (1 time only).
func (h *AgentHandler) Create(c *gin.Context) {
    var req struct {
        Name string `json:"name" binding:"required,min=1,max=100"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
        return
    }

    key, prefix, err := models.GenerateKey()
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "key generation failed"})
        return
    }
    hash, err := models.HashPassword(key)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "password hash failed"})
        return
    }

    operatorID := c.GetUint("user_id")
    a := models.Agent{
        Name:      req.Name,
        KeyPrefix: prefix,
        HashedKey: hash,
        CreatedBy: operatorID,
        Scope:     "all",  // M1 固定
    }
    if err := h.db.Create(&a).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "create failed: " + err.Error()})
        return
    }

    // M1: 返 1 次完整 key (跟 GitHub PAT 风格)
    c.JSON(http.StatusCreated, gin.H{
        "agent": a,
        "key":   key,  // 1 次显示
    })
}

// List handles GET /api/admin/agents.
// Operator-only. Returns 200 + array of agents (prefix only, never full key).
func (h *AgentHandler) List(c *gin.Context) {
    var agents []models.Agent
    if err := h.db.Order("created_at DESC").Find(&agents).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "list failed: " + err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"agents": agents})
}

// Disable handles POST /api/admin/agents/:id/disable.
// Operator-only. Soft-delete via Disabled=true.
func (h *AgentHandler) Disable(c *gin.Context) {
    id, err := strconv.ParseUint(c.Param("id"), 10, 64)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
        return
    }
    var a models.Agent
    if err := h.db.First(&a, id).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
            return
        }
        c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup failed"})
        return
    }
    a.Disabled = true
    if err := h.db.Save(&a).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "save failed"})
        return
    }
    c.JSON(http.StatusOK, gin.H{"ok": true, "disabled": true})
}

// Regenerate handles POST /api/admin/agents/:id/regenerate.
// Operator-only. M1: old key 立刻失效, 不 再显示. New key 1 次显示.
func (h *AgentHandler) Regenerate(c *gin.Context) {
    id, err := strconv.ParseUint(c.Param("id"), 10, 64)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
        return
    }
    var a models.Agent
    if err := h.db.First(&a, id).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
            return
        }
        c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup failed"})
        return
    }

    newKey, newPrefix, err := models.GenerateKey()
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "key generation failed"})
        return
    }
    hash, err := models.HashPassword(newKey)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "password hash failed"})
        return
    }

    a.HashedKey = hash
    a.KeyPrefix = newPrefix
    a.Disabled = false  // regen 自动 激活
    a.LastUsedAt = nil   // 重置
    if err := h.db.Save(&a).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "save failed"})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "agent":    a,
        "new_key":  newKey,  // 1 次显示
    })
}
```

### 6.3 `middleware.MaybeAgentKey` (新建, 验 X-API-Key)

```go
// Package middleware provides Gin middleware for the OPC API.
// agent_auth.go implements MaybeAgentKey (X-API-Key header
// validator for non-human agents).
package middleware

import (
    "net/http"
    "strings"
    "time"

    "github.com/gin-gonic/gin"
    "gorm.io/gorm"

    "github.com/opc/api/internal/models"
)

// MaybeAgentKey inspects the X-API-Key request header. If
// present and valid, sets c.Set("agent_id", <id>) and
// c.Set("agent_scope", <scope>) on the context. Does NOT
// abort if missing or invalid (so it composes with RequireAuth —
// the caller decides whether either is sufficient by checking
// c.GetUint("user_id") > 0 || c.Get("agent_id") != nil in
// the handler).
//
// Use this AFTER RequireAuth in the middleware chain so that
// session-based human callers without X-API-Key are not
// affected.
func MaybeAgentKey(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        key := c.GetHeader("X-API-Key")
        if key == "" {
            c.Next()
            return
        }
        // Quick format check to avoid an unnecessary DB query.
        if !strings.HasPrefix(key, "opc_agent_") || len(key) != 12+32 {
            c.Next()
            return
        }
        prefix := key[:12]  // match stored key_prefix (12 chars)
        var candidates []models.Agent
        if err := db.Where("key_prefix = ?", prefix).Find(&candidates).Error; err != nil {
            c.Next()  // graceful — let RequireAuth 401
            return
        }
        for _, a := range candidates {
            if a.Disabled {
                continue
            }
            // Single bcrypt compare per candidate. bcrypt is
            // already constant-time; no need for explicit
            // subtle.ConstantTimeCompare.
            if !models.VerifyPassword(a.HashedKey, key) {
                continue
            }
            // Stamp last_used_at (fire-and-forget; if fails, log only).
            now := time.Now()
            db.Model(&a).Update("last_used_at", &now)

            c.Set("agent_id", a.ID)
            c.Set("agent_scope", a.Scope)
            c.Set("agent_name", a.Name)
            c.Next()
            return
        }
        c.Next()  // invalid key — let handler's auth check 401
    }
}
```

**注**: 上面 middleware 实现 含 一处 设计 缺陷 (`hashKey` 死代码). 实际 实现 应该 仅 1 个 bcrypt compare 调用, 跟 `models.VerifyPassword` 直接 调. 实施者 需 写 clean code 不要 copy 这 placeholder.

### 6.4 Console `/api-keys` 管理页 (新建)

跟 Sub-Spec C 的 `/creators` 页 模式 几乎一样: table + modal + 1 个 dialog.

**`page.tsx`**: 列 API keys (table: name / key_prefix / created_by / created_at / last_used_at / disabled), "生成新 key" 按钮, per-row "禁用" / "重新生成" 操作.
**`CreateAgentDialog.tsx`**: name 输入 + Submit → 返 1 次完整 key → modal 显示 + 复制 按钮 + "我已保存" 确认 checkbox (operator 必 确认 才 关 modal).

### 6.5 Test 覆盖矩阵

| 层 | 覆盖 | 数量 |
|----|------|------|
| Unit: models.Agent | GenerateKey 格式 验 / HashPassword bcrypt 验 / VerifyPassword 错 key 返 false / KeyPrefix 提取 | 4-5 |
| Unit: handlers.AgentHandler | Create 201 / 400 invalid / 200 List (无 完整 key) / 200 Disable / 200 Regenerate | 5-6 |
| Unit: middleware.MaybeAgentKey | 缺 header pass-through / 无效 key 401 in handler / 有效 key 200 / disabled agent 拒 / 跟 session 兼容 (同时 都有 也 OK) | 5 |
| Integration: E2E | 4 case: operator create / agent call topic 200 / agent call admin 403 / bad key 401 | 4 |
| apps/console 编译 | `pnpm build` exit 0 | 0 |

**Coverage 目标**:
- 父包 `internal/handlers` ≥ 80% (现有 baseline 满足)
- 父包 `internal/middleware` ≥ 80% (现有 baseline 满足)
- NEW: `agent.go` handler ≥ 80%
- NEW: `MaybeAgentKey` middleware ≥ 80%

---

## 七、Schema 设计

### 7.1 Agent Model

```sql
CREATE TABLE agents (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL,
    key_prefix  TEXT NOT NULL,    -- 例 "opc_agent_8x"
    hashed_key  TEXT NOT NULL UNIQUE,  -- bcrypt
    created_by  INTEGER NOT NULL, -- operator user_id
    scope       TEXT NOT NULL DEFAULT 'all',  -- M1 固定
    disabled    BOOLEAN NOT NULL DEFAULT false,
    last_used_at DATETIME,
    created_at  DATETIME,
    updated_at  DATETIME
);
```

GORM AutoMigrate 自动加. 老 Phase 1 + A/B/C 都没这表, 0 影响.

### 7.2 HTTP Wire Schema

**`POST /api/admin/agents`**
- Request: `{"name": "string (1-100 chars)"}`
- Response 201: `{"agent": {id, name, key_prefix, scope, disabled, created_by, created_at}, "key": "opc_agent_<random32>"}`
- Response 400: invalid body
- Response 401/403: not operator (跟 Sub-Spec C 一致)

**`GET /api/admin/agents`**
- Response 200: `{"agents": [{id, name, key_prefix, scope, disabled, created_by, created_at, last_used_at}]}`
- (无 `key` 字段, 也无 `hashed_key` 字段)

**`POST /api/admin/agents/:id/disable`**
- Response 200: `{"ok": true, "disabled": true}`
- Response 404: agent not found

**`POST /api/admin/agents/:id/regenerate`**
- Response 200: `{"agent": {...}, "new_key": "opc_agent_<random32>"}` (新 key 1 次显示, 旧 失效)

### 7.3 现有 endpoint 集成 (调 agent 时, response 不变)

`POST /api/ai/topics` 跟 `GET /api/auth/me` 现有 response shape 不变. Agent 调 时:
- response body 跟 human 一致
- call_log middleware 记录 `actor_type: "agent"` + `agent_id: <id>`
- 跟 human 调 同 一 cost_cents 走 (M1: 同 一 cost table)

---

## 八、Consistency Check 设计

N/A — D 是 auth layer, 无新 一致性 检查. 跟 Sub-Spec A 的 IP profile consistency check 无关.

---

## 九、Prompt 构建

N/A — D 不 加 capability, 复用 B 的 `topic.Generate` library. Agent 调 跟 human 调 走 同 一 handler, prompt 不 变.

---

## 十、数据流

### 10.1 启动期

无 启动期 依赖. GORM AutoMigrate 加 `&models.Agent{}` + call_log 新 2 列. 现有 服务 不 重启 (HMR or rolling restart).

### 10.2 运行时: Operator 创建 Agent

```
Operator 在 console /api-keys → 填 name → Submit
  ↓
1. POST /api/admin/agents (operator session)
   ↓ RequireAuth + RequireOperatorRole (现 有, 跟 C 一致)
2. AgentHandler.Create
   2.1 bind + validate (name 1-100 chars)
   2.2 generateKey() → "opc_agent_<random32>" + prefix (12 char)
   2.3 HashPassword(key) → bcrypt hash
   2.4 INSERT INTO agents (name, key_prefix, hashed_key, created_by, scope='all')
   2.5 Return 201 {agent: {...}, key: "opc_agent_..."}  ← 1 次显示 完整 key
3. console 刷新 table, modal 显示 完整 key + "复制" 按钮 + "我已保存" checkbox
```

### 10.3 运行时: Agent 调 /api/ai/topics

```
$ curl -X POST /api/ai/topics \
    -H "X-API-Key: opc_agent_8x4k2pqrstuvwxyz0123456789ab" \
    -d '{"seed":"...","platform":"...","count":5}'
  ↓
1. POST /api/ai/topics
   ↓ RequireAuth 验 session → 401 (无 cookie)
   ↓ MaybeAgentKey 验 X-API-Key header → bcrypt compare → 成功
   ↓ c.Set("agent_id", <id>); c.Set("agent_scope", "all")
   ↓ maybe 也 c.Set("user_id", nil) (无 session)
2. AIHandler.GenerateTopics (B M1 done, 业务 不动)
   ↓ bind + validate + demo check
   ↓ 1 行 新 check: c.GetUint("user_id") > 0 || c.Get("agent_id") != nil → 通过
   ↓ topic.Generate(ctx, h.text, input) → 5 topics
3. call_log middleware (D M1 加 2 列)
   ↓ actor_type="agent", agent_id=<id>, user_id=0
   ↓ StampClaudeCost + log
4. Response 200 {topics: [5 items]}
```

### 10.4 运行时: Agent 调 Admin Endpoint (拒绝)

```
$ curl -X POST /api/admin/agents \
    -H "X-API-Key: opc_agent_..." \
    -d '{"name":"x"}'
  ↓
1. POST /api/admin/agents
   ↓ RequireAuth 验 session → 401 (无 cookie)
   ↓ RequireOperatorRole 验 user_role → user_role 不 在 ctx (RequireAuth 已 401)... 实际:
   实际 chain: r.Group("/api/admin", RequireAuth, RequireOperatorRole)
   ↓ RequireAuth 401 (无 session) ← 走 这条, 不 到 RequireOperatorRole
2. Response 401
```

**注**: 实测 上, agent 调 admin endpoint 返 401 (RequireAuth 拒) 而非 403 (RequireOperatorRole 拒). 跟 Sub-Spec C 创作者 调 admin 返 403 不同 (创作者 有 session, RequireAuth 通过, RequireOperatorRole 拒). 这是 因为 agent 没 session. M1 接受, M2 可加 RequireOperatorRole 的 agent check.

### 10.5 运行时: 错误 X-API-Key

```
$ curl -X POST /api/ai/topics \
    -H "X-API-Key: opc_agent_bogus0000000000000000000000" \
    -d '{"seed":"x","platform":"抖音","count":5}'
  ↓
1. POST /api/ai/topics
   ↓ RequireAuth 401 (无 session)
2. Response 401
```

(跟 admin 调 一样 走 RequireAuth 拒 路径, 不 到 MaybeAgentKey check. M1 接受. M2 可 加 "session 跟 agent key 任 一 即可" 的 grace 处理, 让无 session + 无效 key 返 401, 但有效 key 返 200. 本 task 不改 老 RequireAuth 行为.)

---

## 十一、错误处理

| 错误 | 检测 | 处理 |
|------|------|------|
| 缺 X-API-Key header | MaybeAgentKey pass-through | (no-op); handler 1 行 check 401 |
| X-API-Key 无效 (不匹配) | bcrypt compare fail | (pass-through); handler 1 行 check 401 |
| X-API-Key 匹配但 agent disabled | MaybeAgentKey 跳过 disabled | (pass-through); handler 1 行 check 401 |
| Agent 调 admin endpoint (无 session) | RequireAuth 401 | 401 "unauthorized" |
| Agent 调 admin endpoint (有 session — race condition) | RequireAuth 通过, RequireOperatorRole 拒 | 403 "operator role required" (跟 C 一致) |
| 创建 agent 重复 name | (不强制 unique, name 仅 display) | 201 (多 key 同 name 允许) |
| bcrypt 失败 (key 极长) | HashPassword | 500 |
| AutoMigrate 失败 (老 DB schema 不兼容) | 启动期 | fatal log + server 不启动 |

---

## 十二、测试策略

### 12.1 Unit Tests

**`apps/api/internal/models/agent_test.go`** (5 case):
```go
func TestAgentGenerateKeyFormat(t *testing.T) {
    key, prefix, err := models.GenerateKey()
    if !strings.HasPrefix(key, "opc_agent_") { t.Errorf(...) }
    if len(prefix) != 12 { t.Errorf(...) }
    if err != nil { t.Errorf(...) }
}

func TestAgentGenerateKeyUnique(t *testing.T) {
    // Generate 100 keys, all unique.
}

func TestAgentHashAndVerify(t *testing.T) {
    key, _, _ := models.GenerateKey()
    hash, _ := models.HashPassword(key)
    if !models.VerifyPassword(hash, key) { t.Errorf("verify failed") }
    if models.VerifyPassword(hash, "wrong-key") { t.Errorf("verify should fail") }
}

func TestAgentKeyPrefix(t *testing.T) {
    if got := models.KeyPrefix("opc_agent_abcdef123456"); got != "opc_agent_ab" {
        t.Errorf("prefix = %q, want opc_agent_ab", got)
    }
}

func TestAgentKeyPrefixShortKey(t *testing.T) {
    if got := models.KeyPrefix("short"); got != "short" {
        t.Errorf("short key prefix = %q, want short", got)
    }
}
```

**`apps/api/internal/handlers/agent_test.go`** (5-6 case):
```go
func TestCreateAgent201(t *testing.T)            { /* valid name + operator → 201 + agent + full key */ }
func TestCreateAgent400InvalidName(t *testing.T)  { /* empty name → 400 */ }
func TestListAgents200(t *testing.T)              { /* operator → 200 + array (无 full key) */ }
func TestDisableAgent200(t *testing.T)            { /* operator → 200 + disabled=true */ }
func TestRegenerateAgent200(t *testing.T)         { /* operator → 200 + new key 1 次显示 */ }
```

**`apps/api/internal/middleware/agent_auth_test.go`** (5 case):
```go
func TestMaybeAgentKeyNoHeader(t *testing.T)        { /* agent_id 不 设, pass-through */ }
func TestMaybeAgentKeyInvalidKey(t *testing.T)       { /* 无效 key → agent_id 不 设 */ }
func TestMaybeAgentKeyValidKey(t *testing.T)         { /* 有效 key → agent_id 设, next 调 */ }
func TestMaybeAgentKeyDisabledAgent(t *testing.T)    { /* disabled=true → 跳 */ }
func TestMaybeAgentKeyWithSessionCookie(t *testing.T) { /* 两者 都 有, 双 通过 */ }
```

### 12.2 E2E Smoke (`scripts/api-smoke.sh` 加 4 case)

```bash
# === Sub-Spec D M1: agent API key auth ===
section "agent API key"

# 1. Operator creates an agent (uses existing operator session)
out=$(curl -s -X POST http://localhost:8081/api/admin/agents \
    -H "Content-Type: application/json" \
    -H "Cookie: opc_session=$OPERATOR_SESSION_TOKEN" \
    -d '{"name":"claude-code-laptop-1"}')
api_key=$(echo "$out" | jq -r '.key' 2>/dev/null)
agent_id=$(echo "$out" | jq -r '.agent.id' 2>/dev/null)
if [ -n "$api_key" ] && [ "$api_key" != "null" ]; then
    pass "admin: agent created (id=$agent_id, key returned 1x)"
else
    fail "admin: agent creation failed (raw: ${out:0:200})"
fi

# 2. Agent calls /api/ai/topics with X-API-Key
out=$(curl -s -X POST http://localhost:8081/api/ai/topics \
    -H "Content-Type: application/json" \
    -H "X-API-Key: $api_key" \
    -d '{"seed":"个人成长","platform":"抖音","count":5}')
topic_count=$(echo "$out" | jq '.topics | length' 2>/dev/null)
if [ "$topic_count" = "5" ]; then
    pass "agent: topic HTTP returns 5 topics"
else
    fail "agent: topic HTTP got $topic_count, want 5"
fi

# 3. Agent tries to call admin endpoint (should be 401 — no session)
status=$(curl -s -o /dev/null -w "%{http_code}" -X POST \
    http://localhost:8081/api/admin/agents \
    -H "Content-Type: application/json" \
    -H "X-API-Key: $api_key" \
    -d '{"name":"x"}')
if [ "$status" = "401" ]; then
    pass "agent: admin endpoint correctly 401 (no session)"
else
    fail "agent: admin endpoint returned $status, want 401"
fi

# 4. Bad API key (should be 401)
status=$(curl -s -o /dev/null -w "%{http_code}" -X POST \
    http://localhost:8081/api/ai/topics \
    -H "Content-Type: application/json" \
    -H "X-API-Key: opc_agent_bogus0000000000000000000000" \
    -d '{"seed":"x","platform":"抖音","count":5}')
if [ "$status" = "401" ]; then
    pass "agent: bad API key correctly 401"
else
    fail "agent: bad API key returned $status, want 401"
fi
```

### 12.3 console 编译

```bash
cd /root/workspace/opc/apps/console
pnpm build 2>&1 | tail -10
```

Expected: exit 0, /api-keys 路由编译

---

## 十三、实施计划（M1 timeline）

| 周 | 任务 | 交付物 |
|----|------|--------|
| W1 | Task 1: `models.Agent` + bcrypt helper + 5 unit test | AutoMigrate 加 agents 表, 老 Phase 1 不破 |
| W1 | Task 2: 4 admin endpoint (creator.go) + `RequireOperatorRole` 复用 (现 C 已 加) + 5-6 unit test | 4 endpoint 跟 C 的 creator 模式 一致, operator 仅 |
| W1 | Task 3: `middleware.MaybeAgentKey` + 现有 handler 加 1 行 check + 5 unit test | agent 调 /api/ai/topics 走 通, 跟 session 兼容 |
| W2 | Task 4: console /api-keys 页 + CreateAgentDialog + call_log 加 2 列 | 完整 operator workflow (创 + 列表 + disable + regenerate) |
| W2 | Task 5: scripts/api-smoke.sh 加 4 case + verify + state files | 16+4=20 PASS, G16 CLOSED |

**W2 末 verify agent 必跑**, OVERALL PASS 才算 M1 完.

---

## 十四、风险与不做的事

### 14.1 风险

| 风险 | 缓解 |
|------|------|
| API key 泄露 (operator 复制 保存 不 慎) | 立即 regenerate (旧 失效) + 1 次显示 机制 (modal 强制 "我已保存" checkbox) |
| X-API-Key 跟 session 互相 冲突 | 设计 兼容: handler check `user_id OR agent_id`, 任 一 通过 即 可. 老 frontend 不 破 |
| bcrypt 跟 User.PasswordHash 共用 cost, 未来 加 cost 不一致 | M1 复用 `bcrypt.DefaultCost`; M2 加 per-table cost 配置 (在 models 包) |
| 老 call_log row 缺 actor_type 跟 agent_id 字段 | GORM AutoMigrate 自动加 (default "" / 0), 老 row 走 "human" 路径 兼容 |
| Operator 用 API key 创 creator (误用 agent 创 human 账号) | (no M1 defense; future 加 audit log) |
| API key 误存为 完整 plaintext 在 log | (no M1 defense; M2 加 log redaction) |
| agent_id check 在 handler 缺漏 (老 handler 没 改) | 老 handler (除 GenerateTopics) 不 需 改; call_log middleware 自动 记 agent_id (如有); M2 全面 audit |
| Agent 调 admin endpoint 返 401 而非 403 (跟 human 创作者 调 admin 返 403 不一致) | 接受 (RequireAuth 先 401 拒); M2 可 加 "agent 走 RequireOperatorRole 也 拒 返 403" 强化 UX |

### 14.2 不做的事（M1 范围硬约束）

- ❌ OpenAI function-calling 兼容 API
- ❌ TypeScript / Python / Go SDK library
- ❌ Per-key 权限 scope (M1: 所有 key 调 所有 capability)
- ❌ Regenerate 1 次显示 完整 旧 key
- ❌ 任何 新 capability endpoint
- ❌ 改 capability 业务 (B M1 + Phase 1 老 code 全部 不动)
- ❌ 改 Sub-Spec C 老 endpoint 行为 (C 的 4 admin endpoint 仍 operator-only via session)
- ❌ 重写 call_log middleware (仅 加 2 列)
- ❌ 重写 RequireAuth (仅 加 1 行 c.Set "agent_id")

---

## 十五、与 Sub-Spec A / B / C 既有代码的关系

| 已有 | D M1 处置 | 理由 |
|------|--------|------|
| `internal/assetgen/` (A M1) | 不动 | agent 跟 human 调 走 同 一 IP profile (anthropomorphic, M1 1 个) |
| `internal/capabilities/topic/` (B M1) | 不动 | agent 调 走 现有 `POST /api/ai/topics` 复用 library |
| `internal/handlers/ai.go:GenerateTopics` (B M1) | 加 2 行 (agent_id check + actor_type) | 1 capability 暴露, 跟 human 走 同 一 handler |
| `internal/handlers/creator.go` (C M1) | 不动 | operator-only via session, agent 调 返 401 |
| `internal/middleware/auth.go:RequireAuth` (C M1) | 不动 | session auth 仍 走 RequireAuth |
| `internal/middleware/auth.go:RequireOperatorRole` (C M1) | 不动 | D 的 4 admin endpoint 复用 |
| `internal/middleware/call_log.go` (现 有) | 加 2 列 (agent_id + actor_type) | agent 调 跟 human 一样 记 cost |
| `internal/models/user.go` (C M1) | 不动 | D 新建 Agent model (跟 User 隔表) |
| `apps/console/app/(console)/creators/` (C M1) | 复用 UI 模式 (table + modal) | D 加 /api-keys 页 跟 /creators 模式 一致 |
| `apps/console/components/Sidebar.tsx` (C M1) | 加 1 个 "API key 管理" link | 跟 /creators link 同 模式 |
| `apps/api/cmd/server/main.go` (C M1) | 加 4 新 endpoint + 1 个新 middleware | 跟 C 模式 一致 |

---

## 十六、与 Phase 2 其他 sub-spec 的关系

| Sub-spec | 与本 sub-spec 的关系 |
|----------|---------------------|
| A (IP profile) | M1 done ✅。Agent 调 走 同一 IP profile |
| B (4 能力工具化) | M1 done ✅。Agent 通过 X-API-Key 调 现有 `/api/ai/topics` |
| C (对外 web UI) | M1 done ✅。C 是 human 创作者, D 是 non-human agent — 互不 冲突 |
| D (agent-agnostic) | 本 sub-spec。M1 = 仅加 agent auth (API key) |
| E (反 AI 检测) | 跟 D 弱 关联。E M1 是给 capability 的 consistency check 加拟人化 token; D M1 调 capability 不 受 E M1 影响 |

**关键路径** (按 Phase 1 spec §16):
```
A ✅ → B ✅ → C ✅ ─┐
                  ├─ D (本) ─ D M2 (OpenAI + SDK + per-key scope)
       ↓          │
       E (parallel, 需 A consistency check)
```

D M1 完成后, 启 E (反 AI 检测) 即可。E M1 独立.

---

## 十七、变更记录

| 版本 | 日期 | 变更 | 作者 |
|------|------|------|------|
| v1.0 | 2026-06-15 | 初稿（基于 brainstorming 2 问澄清：仅加 agent auth (API key) / 独立 Agent model + console 简易页） | OPC + Claude |

---

**下一步**：
- 用户审阅本文档，提出修改意见
- 通过后进入 Spec 自审（检查占位符/矛盾/歧义/范围）
- 然后调用 `writing-plans` skill，把 sub-spec D 拆为可执行的 M1 实施计划（5 task, 1-2 周）
- M1 完成后，启 E (反 AI 检测) 的 brainstorming → spec → plan 循环
