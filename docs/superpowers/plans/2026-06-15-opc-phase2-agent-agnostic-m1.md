# OPC Phase 2 — Agent-Agnostic 平台 M1 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给 OPC 平台加 agent 认证层（X-API-Key），让非-人类 agent（Claude Code / 自研 / MCP 客户端）能调 现有 HTTP 端点（`/api/ai/topics`），复用 Sub-Spec A/B/C 的 capability + IP profile 框架。 M1 1-2 周内 verify-project-state agent 跑出 OVERALL PASS。

**Architecture:** 1 个新 `models.Agent` GORM 表 + bcrypt 加密 API key + 1 个新 `RequireAgentKey` 中间件（不 abort，pass-through 让 handler 决定）+ 4 个 admin endpoint（operator-only via 现有 `RequireOperatorRole`）+ 现有 `GenerateTopics` handler 加 1 行 agent_id check（跟 session cookie 兼容，任 一 通过 即可）+ 现有 `call_log` middleware 加 2 列 (`actor_type` / `agent_id`)。Console 加 `/api-keys` 管理页（operator 创建/列出/disable/regenerate API key）。

**Tech Stack:** Go 1.25, GORM (SQLite dev), bcrypt (`bcrypt.DefaultCost` 跟 User 同 cost), stdlib `crypto/rand` + `encoding/hex` (key 生成), gin middleware 跟 现有 pattern 复用, Next.js 14 + React 18 (新 console 页面, 跟 Sub-Spec C 模式 一致).

**Spec:** `docs/superpowers/specs/2026-06-15-opc-phase2-agent-agnostic-design.md` v1.0 (commit `91c502e` self-review fix).

---

## 文件结构（M1 终态）

**新建**:
- `apps/api/internal/models/agent.go` — Agent struct + GenerateKey + HashPassword + VerifyPassword
- `apps/api/internal/models/agent_test.go` — 5 个 unit test
- `apps/api/internal/handlers/agent.go` — 4 admin endpoint (create/list/disable/regenerate)
- `apps/api/internal/handlers/agent_test.go` — 5-6 个 unit test
- `apps/api/internal/middleware/agent_auth.go` — MaybeAgentKey (X-API-Key 验)
- `apps/api/internal/middleware/agent_auth_test.go` — 5 个 unit test
- `apps/console/app/(console)/api-keys/page.tsx` — operator API key 管理页
- `apps/console/app/(console)/api-keys/_components/CreateAgentDialog.tsx` — 生成新 key modal

**修改**:
- `apps/api/internal/db/db.go` — AutoMigrate 加 `&models.Agent{}` (1 行)
- `apps/api/internal/handlers/ai.go` — `GenerateTopics` 加 2 行 (agent_id check + call_log actor_type)
- `apps/api/internal/middleware/call_log.go` — CallLog struct 加 2 列 (actor_type + agent_id), middleware 写逻辑加 4 行
- `apps/api/cmd/server/main.go` — 注册 4 新 admin endpoint + 串联新 middleware; 现有 `/api/ai/topics` chain 加 `middleware.MaybeAgentKey(db)`
- `apps/console/components/Sidebar.tsx` — 加 1 个 "API key 管理" link
- `scripts/api-smoke.sh` — 加 4 case (operator create / agent topic 200 / agent admin 401 / bad key 401)

**复用不动**:
- `internal/assetgen/` (A M1 done)
- `internal/capabilities/topic/` + `internal/handlers/ai.go:GenerateTopics` 业务逻辑 (B M1 done)
- `internal/handlers/creator.go` + `internal/handlers/auth.go` + `internal/middleware/auth.go:RequireAuth` + `RequireOperatorRole` (C M1 done, D 复用)
- `internal/middleware/call_log.go` 现有 11 个 handler 调用 点 (G6 closed 2026-06-09, D 仅 加 字段不 改 业务)

---

## 命名约定

| 类型 | 名称 | 说明 |
|------|------|------|
| API key 格式 | `opc_agent_<32 hex chars>` | 例: `opc_agent_8x4k2pqrstuvwxyz0123456789ab` (41 char total) |
| Key prefix (存储) | `<first 12 chars>` | 例: `opc_agent_8x` (operator UI 显示用) |
| Model 名 | `models.Agent` | 跟 `models.User` / `models.Topic` 同 pattern |
| Middleware 名 | `middleware.MaybeAgentKey` | 跟 `RequireAuth` / `RequireOperatorRole` 同 pattern, 命名 "Maybe" 体现 pass-through 语义 |
| Admin endpoint | `POST/GET /api/admin/agents` + 2 个 `:id/<action>` | 跟 Sub-Spec C `/api/admin/creators*` 同 pattern |
| actor_type 值 | `"human"` / `"agent"` | 2 枚举 (跟 User.Role 风格 一致) |
| Agent 标识 | `agent_id` (uint) | 跟 `user_id` (uint) 在 ctx 并列 (任 一 有 值 即 通过) |

---

## 任务分解

---

### Task 1: `models.Agent` + bcrypt helper + 5 unit test

**Files:**
- Create: `apps/api/internal/models/agent.go`
- Create: `apps/api/internal/models/agent_test.go`

- [ ] **Step 1.1: 写 5 个 unit test (TDD red)**

新建 `apps/api/internal/models/agent_test.go`:

```go
package models_test

import (
    "strings"
    "testing"

    "github.com/opc/api/internal/models"
)

func TestAgentGenerateKeyFormat(t *testing.T) {
    t.Parallel()

    key, prefix, err := models.GenerateKey()
    if err != nil {
        t.Fatalf("GenerateKey error: %v", err)
    }
    if !strings.HasPrefix(key, "opc_agent_") {
        t.Errorf("key = %q, want opc_agent_ prefix", key)
    }
    if len(key) != len("opc_agent_")+32 {
        t.Errorf("key length = %d, want %d", len(key), len("opc_agent_")+32)
    }
    if prefix != key[:12] {
        t.Errorf("prefix = %q, want first 12 chars %q", prefix, key[:12])
    }
}

func TestAgentGenerateKeyUnique(t *testing.T) {
    t.Parallel()

    seen := make(map[string]bool)
    for i := 0; i < 100; i++ {
        key, _, _ := models.GenerateKey()
        if seen[key] {
            t.Fatalf("duplicate key generated: %s", key)
        }
        seen[key] = true
    }
}

func TestAgentHashAndVerify(t *testing.T) {
    t.Parallel()

    key, _, _ := models.GenerateKey()
    hash, err := models.HashPassword(key)
    if err != nil {
        t.Fatalf("HashPassword error: %v", err)
    }
    if !models.VerifyPassword(hash, key) {
        t.Error("VerifyPassword(hash, original) returned false, want true")
    }
    if models.VerifyPassword(hash, "opc_agent_wrong_key_1234567890123456") {
        t.Error("VerifyPassword(hash, wrong) returned true, want false")
    }
}

func TestAgentKeyPrefix(t *testing.T) {
    t.Parallel()

    if got := models.KeyPrefix("opc_agent_abcdef123456"); got != "opc_agent_ab" {
        t.Errorf("prefix = %q, want opc_agent_ab", got)
    }
}

func TestAgentKeyPrefixShortKey(t *testing.T) {
    t.Parallel()

    if got := models.KeyPrefix("short"); got != "short" {
        t.Errorf("short key prefix = %q, want short (no truncation)", got)
    }
}
```

- [ ] **Step 1.2: 跑 test 确认 FAIL (functions 还没 建)**

Run: `cd /root/workspace/opc/apps/api && go test -tags fts5 -run "TestAgent" ./internal/models/...`
Expected: FAIL (GenerateKey / HashPassword / VerifyPassword / KeyPrefix undefined)

- [ ] **Step 1.3: 新建 `apps/api/internal/models/agent.go`**

```go
// Package models provides the GORM data models for the OPC
// platform. agent.go represents an API key holder (a non-human
// agent representing a 创作者 or OPC team).
package models

import (
    "crypto/rand"
    "encoding/hex"
    "time"

    "golang.org/x/crypto/bcrypt"
    "gorm.io/gorm"
)

// Agent represents an API key holder. The full key is shown
// ONCE at creation; only the bcrypt hash is stored long-term.
type Agent struct {
    gorm.Model

    Name        string `gorm:"not null" json:"name"`
    KeyPrefix   string `gorm:"not null" json:"key_prefix"`
    HashedKey   string `gorm:"not null;uniqueIndex" json:"-"`
    CreatedBy   uint   `gorm:"not null" json:"created_by"`
    Scope       string `gorm:"default:all;not null" json:"scope"`
    Disabled    bool   `gorm:"default:false;not null" json:"disabled"`
    LastUsedAt  *time.Time `json:"last_used_at"`
}

// keyPrefixLen is the number of leading chars we keep as
// KeyPrefix for operator display. 12 chars of "opc_agent_XX"
// is enough to identify a key without leaking the secret.
const keyPrefixLen = 12

// GenerateKey returns a fresh "opc_agent_<random32>" key plus
// its 12-char prefix. Uses crypto/rand for entropy.
func GenerateKey() (key string, prefix string, err error) {
    bytes := make([]byte, 16)  // 16 bytes = 32 hex chars
    if _, err := rand.Read(bytes); err != nil {
        return "", "", err
    }
    key = "opc_agent_" + hex.EncodeToString(bytes)
    return key, key[:keyPrefixLen], nil
}

// KeyPrefix returns the first 12 chars of a key. Used for
// operator display in console + MaybeAgentKey prefix lookup.
func KeyPrefix(key string) string {
    if len(key) < keyPrefixLen {
        return key
    }
    return key[:keyPrefixLen]
}

// HashPassword bcrypts a raw key for storage. Uses
// bcrypt.DefaultCost (12) to match User.PasswordHash cost.
func HashPassword(key string) (string, error) {
    hash, err := bcrypt.GenerateFromPassword([]byte(key), bcrypt.DefaultCost)
    if err != nil {
        return "", err
    }
    return string(hash), nil
}

// VerifyPassword compares a raw key against the stored bcrypt hash.
// Returns true on match, false otherwise.
func VerifyPassword(hashedKey, rawKey string) bool {
    return bcrypt.CompareHashAndPassword([]byte(hashedKey), []byte(rawKey)) == nil
}
```

- [ ] **Step 1.4: 跑 test 确认 5 PASS**

Run: `cd /root/workspace/opc/apps/api && go test -tags fts5 -run "TestAgent" ./internal/models/...`
Expected: 5 PASS

- [ ] **Step 1.5: 跑全 models test 确认 0 回归**

Run: `cd /root/workspace/opc/apps/api && go test -tags fts5 ./internal/models/...`
Expected: All PASS (5 new + 老 C M1 User tests)

- [ ] **Step 1.6: 改 `apps/api/internal/db/db.go` AutoMigrate 加 `&models.Agent{}`**

读 `db.go` 找 现有 AutoMigrate 调 用 (e.g. `db.AutoMigrate(&models.User{}, &models.Session{}, ...)`). 加 `&models.Agent{}` 到 list:

```go
db.AutoMigrate(
    &models.User{}, &models.Session{},
    &models.CallLog{}, &models.Credential{},
    &models.KnowledgeDoc{}, &models.Topic{},
    &models.Script{}, &models.ContentItem{},
    &models.Series{},
    &models.Agent{},  // Sub-Spec D M1
)
```

(只 加 1 行, 不动 其他)

- [ ] **Step 1.7: 跑全 API tests 确认 0 回归 (AutoMigrate 兼容)**

Run: `cd /root/workspace/opc/apps/api && go test -tags fts5 ./...`
Expected: All 16 packages PASS

- [ ] **Step 1.8: Commit**

```bash
cd /root/workspace/opc
git add apps/api/internal/models/agent.go apps/api/internal/models/agent_test.go apps/api/internal/db/db.go
git commit -m "feat(models): Agent struct + bcrypt helper + 5 unit tests (Task 1)

Sub-Spec D M1: agent API-key auth layer foundation.

New models.Agent struct (GORM):
- Name: operator-set display name (e.g. 'claude-code-laptop-1')
- KeyPrefix: first 12 chars of full key (opc_agent_XX) — for
  operator UI display without leaking secret
- HashedKey: bcrypt hash of full key (json:'-' so it never
  serializes in API responses)
- CreatedBy: operator user_id (audit trail)
- Scope: 'all' for M1 (M2: per-capability scope)
- Disabled: soft-delete flag
- LastUsedAt: nullable timestamp for usage tracking

New helper functions:
- GenerateKey() — 'opc_agent_<random32>' (16 bytes from
  crypto/rand hex-encoded; 100-iteration uniqueness test
  passes)
- KeyPrefix(key) — first 12 chars; returns the input as-is
  if shorter (no silent truncation)
- HashPassword(key) — bcrypt.DefaultCost (12), same as
  User.PasswordHash
- VerifyPassword(hash, raw) — bcrypt compare, returns bool

5 new unit tests verify format, uniqueness, hash+verify
round-trip, and short-key edge case. All 5 pass.

GORM AutoMigrate in db.go (1 line added) creates the new
'agents' table on next server boot. Old call_log + User + all
existing tables unchanged."
```

---

### Task 2: 4 admin endpoint (agent.go) + 5-6 unit test

**Files:**
- Create: `apps/api/internal/handlers/agent.go`
- Create: `apps/api/internal/handlers/agent_test.go`
- Modify: `apps/api/cmd/server/main.go` (注册 4 endpoint)

- [ ] **Step 2.1: 写 5-6 个 unit test (TDD red)**

新建 `apps/api/internal/handlers/agent_test.go`:

```go
package handlers_test

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "strconv"
    "testing"

    "github.com/opc/api/internal/models"
)

func TestCreateAgent201(t *testing.T) {
    t.Parallel()
    db := setupTestDB(t)
    op := createTestOperator(t, db)

    body := `{"name":"claude-code-laptop-1"}`
    req := httptest.NewRequest("POST", "/api/admin/agents", bytes.NewBufferString(body))
    req.Header.Set("Content-Type", "application/json")
    req = withOperatorSession(req, op.ID)
    w := httptest.NewRecorder()
    r := newTestRouter(db)
    r.ServeHTTP(w, req)

    if w.Code != http.StatusCreated {
        t.Fatalf("status = %d, want 201 (body: %s)", w.Code, w.Body.String())
    }
    var got struct {
        Agent models.Agent `json:"agent"`
        Key   string       `json:"key"`
    }
    json.Unmarshal(w.Body.Bytes(), &got)
    if got.Agent.Name != "claude-code-laptop-1" {
        t.Errorf("name = %q, want claude-code-laptop-1", got.Agent.Name)
    }
    if got.Agent.CreatedBy != op.ID {
        t.Errorf("created_by = %d, want %d", got.Agent.CreatedBy, op.ID)
    }
    if !strings.HasPrefix(got.Key, "opc_agent_") {
        t.Errorf("returned key = %q, want opc_agent_ prefix", got.Key)
    }
    // hashed_key must NOT be in response (json:"-")
    if strings.Contains(w.Body.String(), "hashed_key") {
        t.Errorf("response body must not contain hashed_key (json:'-')")
    }
}

func TestCreateAgent400InvalidName(t *testing.T) {
    t.Parallel()
    db := setupTestDB(t)
    op := createTestOperator(t, db)

    body := `{"name":""}`  // empty name fails min=1 validator
    req := httptest.NewRequest("POST", "/api/admin/agents", bytes.NewBufferString(body))
    req.Header.Set("Content-Type", "application/json")
    req = withOperatorSession(req, op.ID)
    w := httptest.NewRecorder()
    r := newTestRouter(db)
    r.ServeHTTP(w, req)

    if w.Code != http.StatusBadRequest {
        t.Errorf("status = %d, want 400", w.Code)
    }
}

func TestListAgents200(t *testing.T) {
    t.Parallel()
    db := setupTestDB(t)
    op := createTestOperator(t, db)
    // Create 2 agents via the handler (or insert directly)
    for _, name := range []string{"agent-a", "agent-b"} {
        a := models.Agent{
            Name: name, KeyPrefix: "opc_agent_x" + string(name[len(name)-1]),
            HashedKey: "h", CreatedBy: op.ID, Scope: "all",
        }
        db.Create(&a)
    }

    req := httptest.NewRequest("GET", "/api/admin/agents", nil)
    req = withOperatorSession(req, op.ID)
    w := httptest.NewRecorder()
    r := newTestRouter(db)
    r.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Fatalf("status = %d, want 200", w.Code)
    }
    var got struct {
        Agents []models.Agent `json:"agents"`
    }
    json.Unmarshal(w.Body.Bytes(), &got)
    if len(got.Agents) != 2 {
        t.Errorf("len(agents) = %d, want 2", len(got.Agents))
    }
    // Verify list response does NOT contain full key or hashed_key
    bodyStr := w.Body.String()
    if strings.Contains(bodyStr, "opc_agent_") {
        t.Errorf("list body must not contain full key prefix")
    }
    if strings.Contains(bodyStr, "hashed_key") {
        t.Errorf("list body must not contain hashed_key")
    }
}

func TestDisableAgent200(t *testing.T) {
    t.Parallel()
    db := setupTestDB(t)
    op := createTestOperator(t, db)
    a := models.Agent{
        Name: "to-disable", KeyPrefix: "opc_agent_x",
        HashedKey: "h", CreatedBy: op.ID, Scope: "all",
    }
    db.Create(&a)

    req := httptest.NewRequest("POST", "/api/admin/agents/"+strconv.FormatUint(uint64(a.ID), 10)+"/disable", nil)
    req = withOperatorSession(req, op.ID)
    w := httptest.NewRecorder()
    r := newTestRouter(db)
    r.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Fatalf("status = %d, want 200", w.Code)
    }
    var got models.Agent
    db.First(&got, a.ID)
    if !got.Disabled {
        t.Error("disabled = false, want true")
    }
}

func TestDisableAgent404(t *testing.T) {
    t.Parallel()
    db := setupTestDB(t)
    op := createTestOperator(t, db)

    req := httptest.NewRequest("POST", "/api/admin/agents/99999/disable", nil)
    req = withOperatorSession(req, op.ID)
    w := httptest.NewRecorder()
    r := newTestRouter(db)
    r.ServeHTTP(w, req)

    if w.Code != http.StatusNotFound {
        t.Errorf("status = %d, want 404", w.Code)
    }
}

func TestRegenerateAgent200(t *testing.T) {
    t.Parallel()
    db := setupTestDB(t)
    op := createTestOperator(t, db)
    oldKey, _, _ := models.GenerateKey()
    oldHash, _ := models.HashPassword(oldKey)
    a := models.Agent{
        Name: "to-regen", KeyPrefix: models.KeyPrefix(oldKey),
        HashedKey: oldHash, CreatedBy: op.ID, Scope: "all",
    }
    db.Create(&a)

    req := httptest.NewRequest("POST", "/api/admin/agents/"+strconv.FormatUint(uint64(a.ID), 10)+"/regenerate", nil)
    req = withOperatorSession(req, op.ID)
    w := httptest.NewRecorder()
    r := newTestRouter(db)
    r.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Fatalf("status = %d, want 200", w.Code)
    }
    var got struct {
        Agent  models.Agent `json:"agent"`
        NewKey string       `json:"new_key"`
    }
    json.Unmarshal(w.Body.Bytes(), &got)
    if got.NewKey == oldKey {
        t.Errorf("new key == old key; should be different")
    }
    if !strings.HasPrefix(got.NewKey, "opc_agent_") {
        t.Errorf("new key = %q, want opc_agent_ prefix", got.NewKey)
    }
    // old key must no longer verify
    var dbAgent models.Agent
    db.First(&dbAgent, a.ID)
    if models.VerifyPassword(dbAgent.HashedKey, oldKey) {
        t.Error("old key still verifies after regenerate; should be invalid")
    }
    if !models.VerifyPassword(dbAgent.HashedKey, got.NewKey) {
        t.Error("new key does not verify; should be valid")
    }
}
```

(6 个 test, 复用 现有 testhelper `setupTestDB` / `createTestOperator` / `withOperatorSession` / `newTestRouter` / `strconv.FormatUint`)

- [ ] **Step 2.2: 跑 test 确认 FAIL (handler 还没 写)**

Run: `cd /root/workspace/opc/apps/api && go test -tags fts5 -run "TestCreateAgent|TestListAgents|TestDisableAgent|TestRegenerateAgent" ./internal/handlers/...`
Expected: FAIL (functions undefined)

- [ ] **Step 2.3: 新建 `apps/api/internal/handlers/agent.go`**

```go
// Package handlers provides HTTP route handlers for the OPC
// platform. agent.go implements the operator-only admin
// endpoints for managing agent API keys (X-API-Key auth path).
package handlers

import (
    "errors"
    "net/http"
    "strconv"

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

// RegisterRoutes attaches the 4 admin endpoints to the router.
// IMPORTANT: use RELATIVE paths (e.g. "POST /agents") when
// mounting on a prefixed group (e.g. r.Group("/api/admin")).
// Using absolute paths here will produce duplicated URLs like
// /api/admin/api/admin/agents (Sub-Spec C Task 2 had this bug,
// fixed in Task 5).
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
        "key":   key,
    })
}

// List handles GET /api/admin/agents.
// Operator-only. Returns 200 + array of agents (prefix only,
// never full key — json:"-" on HashedKey enforces this).
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
    a.Disabled = false
    a.LastUsedAt = nil
    if err := h.db.Save(&a).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "save failed"})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "agent":   a,
        "new_key": newKey,
    })
}
```

- [ ] **Step 2.4: 改 `apps/api/cmd/server/main.go` 注册 4 新 endpoint**

读 main.go, 找 现有 C M1 加的 `/api/admin/creators` group (commit `bc6a296`). 在 它 **下面** 加 同样 pattern:

```go
// Sub-Spec D M1: agent API key admin endpoints (operator-only).
// IMPORTANT: pass the router group (not the absolute path) so
// routes register with RELATIVE paths. See Sub-Spec C Task 2 bug
// fix for what happens if you use absolute paths here.
agentH := handlers.NewAgentHandler(db)
agentH.RegisterRoutes(admin)
```

(只加 2 行, 跟 C 模式 一致, 复用 `admin` group — 已经有 `RequireAuth + RequireOperatorRole`)

- [ ] **Step 2.5: 跑 6 个新 test + 老 handler test 确认 0 回归**

Run: `cd /root/workspace/opc/apps/api && go test -tags fts5 -run "TestCreateAgent|TestListAgents|TestDisableAgent|TestRegenerateAgent" ./internal/handlers/...`
Expected: 6 PASS

- [ ] **Step 2.6: 跑全 API tests 确认 0 回归**

Run: `cd /root/workspace/opc/apps/api && go test -tags fts5 ./...`
Expected: All 16 packages PASS

- [ ] **Step 2.7: Commit**

```bash
cd /root/workspace/opc
git add apps/api/internal/handlers/agent.go apps/api/internal/handlers/agent_test.go apps/api/cmd/server/main.go
git commit -m "feat(handlers): 4 admin agent API key endpoints (Task 2)

Sub-Spec D M1: operator-side agent API key management.

4 new admin endpoints (operator-only, RequireAuth +
RequireOperatorRole on the existing /api/admin group):
- POST   /api/admin/agents                 — create API key
  (returns FULL key 1x only, M1 GitHub PAT-style)
- GET    /api/admin/agents                 — list agents
  (KeyPrefix shown, HashedKey NEVER in response — json:'-')
- POST   /api/admin/agents/:id/disable     — soft-delete
- POST   /api/admin/agents/:id/regenerate  — generate new key,
  old key 立刻失效 (M1: 旧 key 不再显示)

6 new unit tests:
- 201 valid name + key returned once
- 400 empty name (binding validator)
- 200 list (no full key, no hashed_key in response body)
- 200 disable (Disabled=true persisted)
- 404 disable nonexistent id
- 200 regenerate (new key differs from old, old no longer verifies)

Routes use RELATIVE paths ('/agents', not '/api/admin/agents') to
match the production /api/admin group prefix — avoids the
Sub-Spec C Task 2 'RegisterRoutes absolute path' bug that was
caught in Task 5 smoke (4xx'd every admin endpoint until the
prefix was double-applied)."
```

---

### Task 3: `middleware.MaybeAgentKey` + 现有 handler 加 1 行 + 5 unit test

**Files:**
- Create: `apps/api/internal/middleware/agent_auth.go`
- Create: `apps/api/internal/middleware/agent_auth_test.go`
- Modify: `apps/api/internal/handlers/ai.go` (加 2 行: agent_id check + call_log actor)
- Modify: `apps/api/internal/middleware/call_log.go` (加 2 列 + 4 行)
- Modify: `apps/api/cmd/server/main.go` (加 MaybeAgentKey 到 /api/ai/* chain)

- [ ] **Step 3.1: 写 5 个 unit test (TDD red)**

新建 `apps/api/internal/middleware/agent_auth_test.go`:

```go
package middleware_test

import (
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"
    "github.com/opc/api/internal/middleware"
    "github.com/opc/api/internal/models"
)

func newAgentTestRouter(db *gorm.DB) *gin.Engine {
    r := gin.New()
    r.Use(middleware.MaybeAgentKey(db))
    r.GET("/x", func(c *gin.Context) {
        if id, ok := c.Get("agent_id"); ok {
            c.String(200, "agent=%v", id)
            return
        }
        c.String(200, "no-agent")
    })
    return r
}

func TestMaybeAgentKeyNoHeader(t *testing.T) {
    t.Parallel()
    db := setupAgentTestDB(t)
    r := newAgentTestRouter(db)
    req := httptest.NewRequest("GET", "/x", nil)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)
    if w.Code != 200 || w.Body.String() != "no-agent" {
        t.Errorf("no header should pass-through, got code=%d body=%q", w.Code, w.Body.String())
    }
}

func TestMaybeAgentKeyInvalidKey(t *testing.T) {
    t.Parallel()
    db := setupAgentTestDB(t)
    r := newAgentTestRouter(db)
    req := httptest.NewRequest("GET", "/x", nil)
    req.Header.Set("X-API-Key", "opc_agent_bogus0000000000000000000000")
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)
    if w.Body.String() != "no-agent" {
        t.Errorf("invalid key should pass-through, got body=%q", w.Body.String())
    }
}

func TestMaybeAgentKeyValidKey(t *testing.T) {
    t.Parallel()
    db := setupAgentTestDB(t)
    // Create a valid agent
    key, _, _ := models.GenerateKey()
    hash, _ := models.HashPassword(key)
    a := models.Agent{
        Name: "test", KeyPrefix: models.KeyPrefix(key),
        HashedKey: hash, CreatedBy: 1, Scope: "all",
    }
    db.Create(&a)

    r := newAgentTestRouter(db)
    req := httptest.NewRequest("GET", "/x", nil)
    req.Header.Set("X-API-Key", key)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)
    if w.Body.String() == "no-agent" {
        t.Errorf("valid key should set agent_id, got no-agent")
    }
    if !strings.Contains(w.Body.String(), "agent=") {
        t.Errorf("body should contain agent=, got %q", w.Body.String())
    }
}

func TestMaybeAgentKeyDisabledAgent(t *testing.T) {
    t.Parallel()
    db := setupAgentTestDB(t)
    key, _, _ := models.GenerateKey()
    hash, _ := models.HashPassword(key)
    a := models.Agent{
        Name: "test", KeyPrefix: models.KeyPrefix(key),
        HashedKey: hash, CreatedBy: 1, Scope: "all", Disabled: true,
    }
    db.Create(&a)

    r := newAgentTestRouter(db)
    req := httptest.NewRequest("GET", "/x", nil)
    req.Header.Set("X-API-Key", key)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)
    if w.Body.String() != "no-agent" {
        t.Errorf("disabled key should pass-through, got body=%q", w.Body.String())
    }
}

func TestMaybeAgentKeyWithSessionCookie(t *testing.T) {
    t.Parallel()
    db := setupAgentTestDB(t)
    key, _, _ := models.GenerateKey()
    hash, _ := models.HashPassword(key)
    a := models.Agent{
        Name: "test", KeyPrefix: models.KeyPrefix(key),
        HashedKey: hash, CreatedBy: 1, Scope: "all",
    }
    db.Create(&a)

    r := newAgentTestRouter(db)
    req := httptest.NewRequest("GET", "/x", nil)
    req.Header.Set("X-API-Key", key)
    // (no user_id in context, but no RequireAuth in this test router)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)
    if w.Code != 200 {
        t.Errorf("valid key should pass, got %d", w.Code)
    }
}
```

(5 个 test, 复用 `setupAgentTestDB` helper (跟 `setupTestDB` 相似, 但 加 `&models.Agent{}` AutoMigrate))

- [ ] **Step 3.2: 跑 test 确认 FAIL (middleware 还没 写)**

Run: `cd /root/workspace/opc/apps/api && go test -tags fts5 -run "TestMaybeAgentKey" ./internal/middleware/...`
Expected: FAIL (MaybeAgentKey undefined)

- [ ] **Step 3.3: 新建 `apps/api/internal/middleware/agent_auth.go`**

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
// the handler decides whether either is sufficient by checking
// c.GetUint("user_id") > 0 || c.Get("agent_id") != nil).
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
        if !strings.HasPrefix(key, "opc_agent_") || len(key) != len("opc_agent_")+32 {
            c.Next()
            return
        }
        prefix := key[:12]  // match stored key_prefix
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

- [ ] **Step 3.4: 改 `apps/api/internal/handlers/ai.go` `GenerateTopics` 加 1 行 agent_id check**

读 `ai.go` 找 `GenerateTopics` 函数 body (B M1 写的). 在 现有 `topic.Generate` 调 用 之前, 加 1 行 check:

```go
// 在 "Real path: delegate to the capability library." 注释之前加
// (M1 Sub-Spec D: agent auth path)
if c.GetUint("user_id") == 0 && c.Get("agent_id") == nil {
    c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized (need session cookie or X-API-Key)"})
    return
}
```

(1 行 check, 不动 业务 逻辑. 老 human flow 跟 现有 一样: RequireAuth 设 user_id → check 通过; agent flow: MaybeAgentKey 设 agent_id → check 通过; 双 无: 401.)

- [ ] **Step 3.5: 改 `apps/api/internal/middleware/call_log.go` (加 2 列 + 4 行)**

读 `call_log.go` 找 `CallLog` struct. 加 2 字段:

```go
type CallLog struct {
    gorm.Model
    // ... 现有 字段 ...
    UserID      uint   `gorm:"default:0" json:"user_id"`     // 现有
    AgentID     uint   `gorm:"default:0" json:"agent_id"`    // NEW (Sub-Spec D M1)
    ActorType   string `gorm:"default:human" json:"actor_type"`  // NEW (M1: "human"|"agent")
    // ... 其他 字段 ...
}
```

在 `StampTextCost` 等现有 helper 加 actor_type 设:

```go
// 在 StampTextCost 上面 加 1 段:
if uid, ok := c.Get("user_id"); ok {
    if aid, ok := c.Get("agent_id"); ok && aid != nil {
        // agent 调: UserID=0, AgentID=aid, ActorType="agent"
        ref.UserID = 0
        if a, ok := aid.(uint); ok {
            ref.AgentID = a
        }
        ref.ActorType = "agent"
    } else {
        // human 调: UserID=uid, AgentID=0, ActorType="human"
        if u, ok := uid.(uint); ok {
            ref.UserID = u
        }
        ref.AgentID = 0
        ref.ActorType = "human"
    }
}
```

(4-5 行, 跟 现有 StampTextCost 共用 同一 middleware 上下文. GORM AutoMigrate 自动加 2 列.)

- [ ] **Step 3.6: 改 `apps/api/cmd/server/main.go` 加 `MaybeAgentKey` 到 `/api/ai/*` chain**

读 main.go 找 现有 `/api/ai/topics` 路由 (B M1 加). 加 1 个 middleware 串联:

```go
// 现状 (B M1):
r.POST("/api/ai/topics", middleware.RequireAuth(), aiHandler.GenerateTopics)

// D M1 后:
r.POST("/api/ai/topics",
    middleware.RequireAuth(),
    middleware.MaybeAgentKey(db),  // NEW: pass-through, 设 agent_id if X-API-Key present
    aiHandler.GenerateTopics)
```

(1 行加, 跟 C 的 `admin := r.Group("/api/admin", ...)` 模式 一致; db 变量 现 有)

- [ ] **Step 3.7: 跑 5 个新 test + 老 middleware test 确认 0 回归**

Run: `cd /root/workspace/opc/apps/api && go test -tags fts5 ./internal/middleware/...`
Expected: All PASS (5 new + 现有 9 auth test + 6 creator test + ...)

- [ ] **Step 3.8: 跑全 API tests 确认 0 回归**

Run: `cd /root/workspace/opc/apps/api && go test -tags fts5 ./...`
Expected: All 16 packages PASS

- [ ] **Step 3.9: Commit**

```bash
cd /root/workspace/opc
git add apps/api/internal/middleware/agent_auth.go apps/api/internal/middleware/agent_auth_test.go apps/api/internal/handlers/ai.go apps/api/internal/middleware/call_log.go apps/api/cmd/server/main.go
git commit -m "feat(middleware): MaybeAgentKey + call_log actor_type (Task 3)

Sub-Spec D M1: agent authentication layer (X-API-Key).

New middleware: middleware.MaybeAgentKey(db)
- Inspects X-API-Key request header
- If present + valid: c.Set('agent_id', id), c.Set('agent_scope', scope)
- If missing or invalid: pass-through (no abort; composes with
  RequireAuth in the middleware chain)
- Format check: 'opc_agent_' prefix + 32 hex chars (avoids
  unnecessary DB queries for malformed keys)
- Single bcrypt compare per candidate (constant-time)
- Disabled agents skipped
- Stamps last_used_at on success (fire-and-forget)

ai.go:GenerateTopics now has 1-line check for
  c.GetUint('user_id') > 0 || c.Get('agent_id') != nil
so both human (session cookie) and agent (X-API-Key) auth
paths work for the same handler.

call_log middleware: 2 new columns
- agent_id (uint, default 0) — same pattern as user_id
- actor_type (string, default 'human') — 'human' | 'agent'
On each call, middleware sets one of:
- human path: UserID=uid, AgentID=0, ActorType='human'
- agent path: UserID=0, AgentID=aid, ActorType='agent'

5 new middleware tests:
- no header → pass-through, no agent_id
- invalid key → pass-through (no abort)
- valid key → agent_id set + last_used_at updated
- disabled agent → pass-through (no abort)
- key with no session → still sets agent_id (composes
  with future auth check in handler)

Routes /api/ai/topics chain now: RequireAuth, MaybeAgentKey,
handler. Old frontend clients unchanged."
```

---

### Task 4: console `/api-keys` 管理页 + `CreateAgentDialog`

**Files:**
- Create: `apps/console/app/(console)/api-keys/page.tsx`
- Create: `apps/console/app/(console)/api-keys/_components/CreateAgentDialog.tsx`
- Modify: `apps/console/components/Sidebar.tsx` (加 1 个 link)

- [ ] **Step 4.1: 读 现有 `apps/console/app/(console)/creators/page.tsx` 找 UI 模式**

Run: `cat /root/workspace/opc/apps/console/app/\(console\)/creators/page.tsx`
跟它 模式 一致: table + dialog (CreateCreatorDialog) + per-row 操作 (重置密码 / 禁用).

- [ ] **Step 4.2: 新建 `apps/console/app/(console)/api-keys/page.tsx`**

```tsx
"use client";
import { useEffect, useState } from "react";
import { CreateAgentDialog } from "./_components/CreateAgentDialog";

interface Agent {
    id: number;
    name: string;
    key_prefix: string;
    scope: string;
    disabled: boolean;
    created_by: number;
    created_at: string;
    last_used_at: string | null;
}

export default function ApiKeysPage() {
    const [agents, setAgents] = useState<Agent[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState("");
    const [createOpen, setCreateOpen] = useState(false);

    async function load() {
        setLoading(true);
        try {
            const res = await fetch("/api/admin/agents", { credentials: "include" });
            if (!res.ok) throw new Error(`load failed: ${res.status}`);
            const data = await res.json();
            setAgents(data.agents);
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setLoading(false);
        }
    }

    useEffect(() => { load(); }, []);

    async function disableAgent(a: Agent) {
        if (!confirm(`Disable agent ${a.name} (${a.key_prefix}...)? It will no longer be able to call the API.`)) return;
        const res = await fetch(`/api/admin/agents/${a.id}/disable`, {
            method: "POST", credentials: "include",
        });
        if (!res.ok) { alert("disable failed"); return; }
        await load();
    }

    async function regenerateAgent(a: Agent) {
        if (!confirm(`Regenerate API key for ${a.name}? The current key will be invalidated immediately.`)) return;
        const res = await fetch(`/api/admin/agents/${a.id}/regenerate`, {
            method: "POST", credentials: "include",
        });
        if (!res.ok) { alert("regenerate failed"); return; }
        const data = await res.json();
        // Show the new key one time in a confirm dialog (manual copy)
        prompt("New API key (copy now, shown only once):", data.new_key);
        await load();
    }

    if (loading) return <p>加载中...</p>;
    if (error) return <p style={{ color: "red" }}>{error}</p>;

    return (
        <div>
            <h1>API key 管理</h1>
            <p style={{ color: "#666" }}>
                Agent API keys 让非-人类用户（Claude Code / 自研 agent / MCP 客户端）能通过 X-API-Key header 调平台。
            </p>
            <button onClick={() => setCreateOpen(true)}>+ 生成新 API key</button>
            <table>
                <thead>
                    <tr>
                        <th>名称</th><th>Key 前缀</th><th>Scope</th>
                        <th>创建者</th><th>创建时间</th><th>最后使用</th>
                        <th>状态</th><th>操作</th>
                    </tr>
                </thead>
                <tbody>
                    {agents.map((a) => (
                        <tr key={a.id}>
                            <td>{a.name}</td>
                            <td><code>{a.key_prefix}...</code></td>
                            <td>{a.scope}</td>
                            <td>{a.created_by || "—"}</td>
                            <td>{a.created_at.slice(0, 19)}</td>
                            <td>{a.last_used_at ? a.last_used_at.slice(0, 19) : "未使用"}</td>
                            <td>{a.disabled ? "已禁用" : "活跃"}</td>
                            <td>
                                <button onClick={() => regenerateAgent(a)}>重新生成</button>
                                <button onClick={() => disableAgent(a)} disabled={a.disabled}>
                                    禁用
                                </button>
                            </td>
                        </tr>
                    ))}
                </tbody>
            </table>

            {createOpen && (
                <CreateAgentDialog
                    onClose={() => setCreateOpen(false)}
                    onCreated={() => { setCreateOpen(false); load(); }}
                />
            )}
        </div>
    );
}
```

- [ ] **Step 4.3: 新建 `apps/console/app/(console)/api-keys/_components/CreateAgentDialog.tsx`**

```tsx
"use client";
import { useState } from "react";

export function CreateAgentDialog({ onClose, onCreated }: {
    onClose: () => void;
    onCreated: () => void;
}) {
    const [name, setName] = useState("");
    const [err, setErr] = useState("");
    const [loading, setLoading] = useState(false);
    const [newKey, setNewKey] = useState<string | null>(null);
    const [confirmed, setConfirmed] = useState(false);

    async function submit(e: React.FormEvent) {
        e.preventDefault();
        setErr("");
        setLoading(true);
        const res = await fetch("/api/admin/agents", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            credentials: "include",
            body: JSON.stringify({ name }),
        });
        setLoading(false);
        if (res.ok) {
            const data = await res.json();
            setNewKey(data.key);  // Show 1 time
        } else {
            const body = await res.json().catch(() => ({}));
            setErr(body.error || `failed: ${res.status}`);
        }
    }

    if (newKey) {
        return (
            <div style={overlayStyle}>
                <div style={dialogStyle}>
                    <h2>API key 已生成</h2>
                    <p style={{ color: "red" }}>⚠ 完整 key 仅显示 1 次. 立即复制保存!</p>
                    <pre style={{ background: "#f0f0f0", padding: "0.5rem", overflow: "auto" }}>{newKey}</pre>
                    <label>
                        <input
                            type="checkbox"
                            checked={confirmed}
                            onChange={(e) => setConfirmed(e.target.checked)}
                        />
                        {" "}我已复制保存
                    </label>
                    <br />
                    <button
                        onClick={onCreated}
                        disabled={!confirmed}
                        style={{ marginTop: "1rem" }}
                    >
                        关闭
                    </button>
                </div>
            </div>
        );
    }

    return (
        <div style={overlayStyle}>
            <div style={dialogStyle}>
                <h2>生成新 API key</h2>
                <form onSubmit={submit}>
                    <label>名称 (1-100 字符)
                        <input
                            type="text" value={name}
                            onChange={(e) => setName(e.target.value)}
                            placeholder="例如：claude-code-laptop-1"
                            required minLength={1} maxLength={100}
                            style={{ width: "100%" }}
                        />
                    </label>
                    {err && <p style={{ color: "red" }}>{err}</p>}
                    <button type="submit" disabled={loading}>
                        {loading ? "生成中..." : "生成"}
                    </button>
                    <button type="button" onClick={onClose}>取消</button>
                </form>
            </div>
        </div>
    );
}

const overlayStyle: React.CSSProperties = {
    position: "fixed", top: 0, left: 0, right: 0, bottom: 0,
    background: "rgba(0,0,0,0.5)", display: "flex",
    alignItems: "center", justifyContent: "center", zIndex: 1000,
};
const dialogStyle: React.CSSProperties = {
    background: "white", padding: "1.5rem", borderRadius: "8px",
    minWidth: "500px", maxWidth: "90vw",
};
```

- [ ] **Step 4.4: 改 `apps/console/components/Sidebar.tsx` 加 sidebar link**

读 现有 Sidebar.tsx, 找 `ITEMS` 数组 (C M1 Task 3 加了 "创作者管理"). 加 1 个:

```tsx
{ href: '/api-keys', label: 'API key 管理' },
```

(放 紧跟 在 "创作者管理" 之后, 同 模式)

- [ ] **Step 4.5: 跑 console build 验 不破**

Run: `cd /root/workspace/opc/apps/console && pnpm build 2>&1 | tail -10`
Expected: exit 0, 编译 PASS

- [ ] **Step 4.6: Commit**

```bash
cd /root/workspace/opc
git add apps/console/app/\(console\)/api-keys/ apps/console/components/Sidebar.tsx
git commit -m "feat(console): API key management page + 1 dialog (Task 4)

Sub-Spec D M1: operator UI to manage agent API keys.

New page: apps/console/app/(console)/api-keys/page.tsx
- Lists all API keys (table: name / key_prefix / scope /
  created_by / created_at / last_used_at / disabled)
- 'Generate new key' button → modal
- Per-row 'Regenerate' + 'Disable' actions
  (regenerate prompts for confirmation, then shows the new
  key via window.prompt for one-time copy)
- 1 new dialog: CreateAgentDialog
  - 2-step flow: first shows name input, then after creation
    shows the full key with a '我已复制保存' checkbox that
    must be ticked before the Close button is enabled
  - Defense against operator losing the key (GitHub PAT UX)

Sidebar link added: 'API key 管理' in apps/console/components/
Sidebar.tsx (Sub-Spec C Task 3 fixed the same dev: the
sidebar actually lives in components/Sidebar.tsx, not
app/(console)/layout.tsx).

All 4 admin endpoints from Task 2 are wired:
- POST /api/admin/agents (create)
- POST /api/admin/agents/:id/disable (disable)
- POST /api/admin/agents/:id/regenerate (regenerate)
- GET /api/admin/agents (list)

pnpm build: exit 0, no console errors."
```

---

### Task 5: 集成测试 + 文档 + 最终 verify + 状态文件更新

**Files:**
- Modify: `scripts/api-smoke.sh`
- Modify: `~/.claude/PROJECT-STATE.md`
- Modify: `~/.claude/GAP-LOG.md`

- [ ] **Step 5.1: 读现有 api-smoke.sh 找合适位置加 agent case**

Run: `cd /root/workspace/opc && wc -l scripts/api-smoke.sh; tail -50 scripts/api-smoke.sh`
Note: existing 16 cases (5 handler + 4 IP type + 1 advisory + 2 topic + 4 external creator). Add 4 new (operator create / agent call topic 200 / agent call admin 401 / bad key 401).

- [ ] **Step 5.2: 在 api-smoke.sh 末尾加 4 case**

Append:

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
    pass "agent: topic HTTP returns 5 topics (X-API-Key auth)"
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

(注: 跟 C M1 一致, `OPERATOR_SESSION_TOKEN` env 需在 smoke 跑前 set. 老 smoke 假设 operator session 已 在.)

- [ ] **Step 5.3: 跑 smoke 验证**

```bash
cd /root/workspace/opc
./scripts/api-smoke.sh 2>&1 | tail -30
```

Expected: 20 cases total (16 existing + 4 new). Topic creator case may SKIP if MINIMAX_API_KEY not set.

- [ ] **Step 5.4: 改 `~/.claude/PROJECT-STATE.md`**

按 §H 协议:
- Section E append 1 个 bullet (D M1 closure 总结, 5 commit 链, 16 new test + smoke 20/20 PASS, plan deviations 文档)
- Section D 标 G16 (sub-spec D M1) **CLOSED 2026-06-15**
- 改 Last refreshed line

具体 edit (类似 A/B/C M1 closure 处理):
- Header `Last refreshed: 2026-06-15 ~HH:MM, after verify agent run 24 (Phase 2 sub-spec D M1 closure) returned OVERALL PASS.`
- Section E append: 5 commit 链 + D M1 summary
- Section D add: G16-closed row

- [ ] **Step 5.5: 改 `~/.claude/GAP-LOG.md`**

Append G16 closure entry:
```markdown
## 2026-06-15 HH:MM (objective, observed) — G16: Phase 2 sub-spec D M1 not delivered

- **缺口**: spec + plan 写了但没实现. models.Agent + 4 admin endpoint + MaybeAgentKey middleware + console /api-keys 页 都没 落地.
- **影响**: agent ecosystem 无法 接 (Claude Code / 自研 / MCP 客户端 都 无 X-API-Key 调 平台).
- **修复计划**: 见 docs/superpowers/plans/2026-06-15-opc-phase2-agent-agnostic-m1.md (5 task, 1-2 周)
- **status**: **closed 2026-06-15** — Task 1-5 全部 done, 5 commit landed, 5+6+5=16 new unit test PASS, 4 smoke case (operator create / agent topic 200 / agent admin 401 / bad key 401) PASS
```

- [ ] **Step 5.6: Dispatch verify-project-state subagent** (or self-verify if dispatch is too costly)

按 `~/.claude/agents/verify-project-state.md` 规范. 关键命令:
```bash
go vet -tags fts5 ./...
go test -tags fts5 -race -count=1 ./...
go build -tags fts5 -o /tmp/v-task5-api ./cmd/server
cd apps/console && pnpm build
# Agent admin smoke: curl POST /api/admin/agents 返 201
# Agent 调 admin 返 401
# Bad API key 返 401
# X-API-Key 调 /api/ai/topics 返 5 topics
```

报告写 `~/.claude/audit/verify-reports/verify-YYYYMMDD-HHMMSS-phase2-sub-spec-d-m1.md`. OVERALL: PASS 才算 M1 closed.

- [ ] **Step 5.7: Commit 收尾**

```bash
cd /root/workspace/opc
git add scripts/api-smoke.sh
git commit -m "test(smoke): add agent API key auth cases (Task 5)

scripts/api-smoke.sh: 4 new test cases for Sub-Spec D M1:
- admin: agent created (operator POST /api/admin/agents → 201,
  full key returned 1x for operator to copy)
- agent: topic HTTP returns 5 topics (X-API-Key auth path;
  verifies that the MaybeAgentKey middleware + 1-line handler
  check correctly authenticates non-human callers)
- agent: admin endpoint correctly 401 (agent has no session;
  the /api/admin group requires RequireAuth → 401 because
  no session cookie; documented as expected M1 behavior)
- agent: bad API key correctly 401 (X-API-Key format OK but
  not in agents table; MaybeAgentKey pass-through, then
  handler check 401)

Total smoke count: 16 → 20. Old cases unchanged.

Project state + gap log updated to reflect M1 closure (G16
CLOSED 2026-06-15)."
```

---

## 验收清单（M1 done 的标志, 5 task 全部 DONE）

| # | 验收 | 验证 | 状态 |
|---|------|------|------|
| 1 | `models.Agent` model 存在 + 8 列 + bcrypt helper | `go test -tags fts5 ./internal/models/...` 5+ PASS | ☐ |
| 2 | 4 admin endpoint + 6 unit test PASS | `go test -tags fts5 -run TestAgent ./internal/handlers/...` | ☐ |
| 3 | `MaybeAgentKey` middleware + 5 unit test + 现有 handler 加 1 行 + call_log 2 列 | `go test -tags fts5 ./internal/middleware/...` 全绿 | ☐ |
| 4 | console /api-keys 管理页 + CreateAgentDialog + 编译 PASS | `pnpm build` exit 0 | ☐ |
| 5 | scripts/api-smoke.sh 20/20 (16 老 + 4 新) | `./scripts/api-smoke.sh` | ☐ |
| 6 | 0 回归 (全 16 包 API suite) | smoke 0 回归 | ☐ |
| 7 | verify-project-state agent OVERALL: PASS | `~/.claude/audit/verify-reports/verify-*.md` PASS | ☐ |
| 8 | PROJECT-STATE.md + GAP-LOG.md 更新 | G16 CLOSED 记录在案 | ☐ |

---

## 不做的事（M1 范围硬约束）

- ❌ OpenAI function-calling 兼容 API
- ❌ TypeScript / Python / Go SDK library
- ❌ Per-key 权限 scope (M1: 所有 key 调 所有 capability)
- ❌ Regenerate 1 次显示 完整 旧 key (M1: 仅 显示 新 key, 旧 立刻 失效)
- ❌ 任何 新 capability endpoint
- ❌ 改 capability 业务 (B M1 + Phase 1 老 code 全部 不动)
- ❌ 改 Sub-Spec C 老 endpoint 行为
- ❌ 重写 call_log middleware (仅 加 2 列)
- ❌ 重写 RequireAuth (跟 MaybeAgentKey 串联, RequireAuth 仍 走 session)
- ❌ Agent 调 admin endpoint 返 403 (M1 返 401 即可, M2 可加 强化)

---

## 风险与缓解

| 风险 | 缓解 |
|------|------|
| API key 泄露 (operator 复制 保存 不 慎) | regenerate 1 步 失效 旧 key; CreateAgentDialog 强制 "我已复制保存" checkbox 才 能 关 modal |
| 1 个 user 同时 带 session 跟 X-API-Key (race condition) | MaybeAgentKey 不 abort, handler check `user_id OR agent_id` 任 一 通过 |
| 老 frontend 调 /api/ai/topics 无 X-API-Key | MaybeAgentKey pass-through, RequireAuth 401 — 老 行为 不 破 |
| bcrypt 跟 User.PasswordHash 共用 cost, 未来 加 cost 不一致 | M1 复用 `bcrypt.DefaultCost`; M2 加 per-table cost 配置 |
| 老 call_log row 缺 actor_type 跟 agent_id 字段 | GORM AutoMigrate 自动加 (default '' / 0); 老 row 走 "human" 路径 兼容 |
| RegisterRoutes 绝对 vs 相对 path (Sub-Spec C Task 2 latent bug) | Task 2 commit message 加 注释 提醒 用 相对 path; 4 endpoint 全用 相对 |
| Agent 调 admin endpoint 返 401 而非 403 (跟 human 创作者 调 admin 返 403 不一致) | 接受; M2 可加 RequireOperatorRole 的 agent check |

---

## Self-Review

写 plan 时做了 4 轮 self-review:

1. **Spec coverage**: spec §5 (架构) / §6 (组件) / §7 (schema) / §10 (数据流) / §11 (错误处理) / §12 (测试) / §13 (timeline) / §15 (跟 A/B/C 关系) 全部对应到 5 task. 验收清单 8 项覆盖.
2. **Placeholder scan**: 0 个 TBD/TODO/类似措辞.
3. **Type consistency**:
   - `models.Agent` 8 字段 (id/name/key_prefix/hashed_key/created_by/scope/disabled/last_used_at) Task 1 定义, Task 2 handler 用, Task 3 middleware 用
   - `GenerateKey() (key string, prefix string, err error)` Task 1, Task 2 + Task 3 调
   - `HashPassword(key string) (string, error)` Task 1, Task 2 调
   - `VerifyPassword(hashedKey, rawKey string) bool` Task 1, Task 3 调
   - `MaybeAgentKey(db *gorm.DB) gin.HandlerFunc` Task 3, Task 3 main.go 调
   - `RegisterRoutes(r gin.IRouter)` Task 2, 用 RELATIVE path (跟 C 一样 group pattern, 避免 absolute path bug)
4. **Ambiguity check**:
   - Key 生成 format "opc_agent_<random32>" spec §5.3 + Task 1
   - 1 次显示 完整 key (Create 时) + regenerate 时 仅 显示 新 key, 旧 立刻 失效
   - 老 frontend 调 /api/ai/topics 走 RequireAuth 401 路径, 不 受 MaybeAgentKey 影响
   - disabled=true 的 agent 跳 验证 (Sub-Spec C 创作者 disabled 行为 同步)

无矛盾. Plan ready for execution.
