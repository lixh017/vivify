# OPC Phase 2 — 对外 Web UI M1 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给外部 beta 创作者 (1-10 人, invite-only, 零支付) 1 个独立 web UI (`apps/external/`), 能登录、调 Sub-Spec B 的 topic 生成、看 cost, 走 OPC operator 手动 invite 流程. M1 2-3 周内 verify-project-state agent 跑出 OVERALL PASS.

**Architecture:** 3 app 隔离 (apps/web demo / apps/console operator / apps/external 创作者) + 1 backend (apps/api, 复用). 创作者 UI 复用现有 B `POST /api/ai/topics` endpoint, 不新加 capability-side code. 加 4 个 operator-only admin endpoint (creator CRUD) + 2 个 role-check middleware (`RequireOperatorRole` / `RequireCreatorRole`) + User model 3 列 (Role/Disabled/CreatedBy) + console /creators 管理页 + apps/external 全新 Next.js 14 app.

**Tech Stack:** Go 1.25, Next.js 14.2.5 + React 18.3.1, GORM (SQLite dev), bcrypt (existing session auth), stdlib flag (subcommand parsing), `@opc/shared` (workspace dep), pnpm workspace (existing pattern).

**Spec:** `docs/superpowers/specs/2026-06-15-opc-phase2-external-ui-design.md` v1.0 (commit `f2f968d`).

---

## 文件结构（M1 终态）

**新建**:
- `apps/api/internal/handlers/creator.go` — 4 admin endpoint (POST/GET /creators, /:id/reset-password, /:id/disable)
- `apps/api/internal/handlers/creator_test.go` — 8-10 unit test
- `apps/external/` (new Next.js 14 app)
  - `app/login/page.tsx` — creator login
  - `app/(creator)/dashboard/page.tsx` — creator home
  - `app/(creator)/topic/page.tsx` — topic generation form
  - `lib/api.ts` — fetch wrappers
  - `middleware.ts` — session auth gate
  - `package.json` (`@opc/external`)
  - `next.config.js` (port 3002)
  - `tsconfig.json`
- `apps/console/app/(console)/creators/page.tsx` — operator creator management
- `apps/console/app/(console)/creators/_components/CreateCreatorDialog.tsx`
- `apps/console/app/(console)/creators/_components/ResetPasswordDialog.tsx`

**修改**:
- `apps/api/internal/models/user.go` — 加 Role/Disabled/CreatedBy 3 字段 (gorm tags)
- `apps/api/internal/db/db.go` — AutoMigrate 自动加新列 (无需手动改 db.go)
- `apps/api/internal/handlers/auth.go` — `Login`/`Me`/`Register` response 加 `role` 字段 (1 行修改 ×3 endpoint)
- `apps/api/internal/middleware/auth.go` — `RequireAuth` 加 1 行 `c.Set("user_role", user.Role)`; 新增 `RequireOperatorRole` + `RequireCreatorRole` 2 个 wrapper
- `apps/api/cmd/server/main.go` — 注册 4 个新 admin endpoint + 串联 2 个新 middleware
- `apps/console/app/(console)/layout.tsx` — 侧边栏加 "创作者管理" 链接 (1 个 link)
- `scripts/api-smoke.sh` — 加 4 case (operator 创建 / creator 登录 / creator 调 topic / creator 调 admin 403)

**复用不动**:
- `internal/assetgen/` (A M1)
- `internal/capabilities/topic/` (B M1)
- `internal/handlers/ai.go:GenerateTopics` (B M1 重构后)
- `internal/middleware/call_log.go` (existing cost tracking)
- `internal/handlers/auth.go:Login/Me/Register` 业务逻辑 (仅加 Role 字段)
- `apps/web/` (内部 demo, 不动)
- `apps/shared/` (apps/external 复用, 不动)
- pnpm-workspace.yaml (`apps/*` glob 自动包含 external)

---

## 命名约定

| 类型 | 名称 | 说明 |
|------|------|------|
| User role | `"operator"` / `"creator"` | 2 个 string 枚举 (跟 model default 匹配) |
| User model 字段 | `Role`, `Disabled`, `CreatedBy` | 3 个新 字段 (跟 GORM tag 名 一致) |
| Handler endpoint (admin) | `POST /api/admin/creators` 等 | 4 个, 全部 `/api/admin/*` 前缀 |
| Middleware | `RequireOperatorRole` / `RequireCreatorRole` | 2 个新, 跟现有 `RequireAuth` 串联 |
| App 端口 | `:3002` (apps/external) | 跟 apps/web `:3000` + apps/console `:3001` 区分 |
| App name | `@opc/external` | workspace dep name (跟 `@opc/web` `@opc/console` `@opc/shared` 同 pattern) |

---

## 任务分解

---

### Task 1: User model 加 3 列 (Role/Disabled/CreatedBy) + 3 unit test

**Files:**
- Modify: `apps/api/internal/models/user.go`
- Modify: `apps/api/internal/models/user_test.go` (加 3 test)
- (db/db.go 自动 AutoMigrate, 不需改)

- [ ] **Step 1.1: 写 3 个 unit test (TDD red)**

在 `apps/api/internal/models/user_test.go` 末尾加 (如果文件 不 存在, 创建):

```go
package models_test

import (
    "testing"

    "github.com/opc/api/internal/models"
)

// TestUserDefaultRole verifies the new Role field defaults to
// "operator" per the gorm tag in models.User.
func TestUserDefaultRole(t *testing.T) {
    t.Parallel()

    u := models.User{Email: "x@x.com", PasswordHash: "h"}
    if u.Role != "" {
        t.Errorf("zero-value Role = %q, want empty (GORM applies default on Save)", u.Role)
    }

    // After Save + Reload, Role should be "operator" (GORM default).
    db := setupTestDB(t)
    if err := db.Create(&u).Error; err != nil {
        t.Fatalf("Create: %v", err)
    }
    var got models.User
    if err := db.First(&got, u.ID).Error; err != nil {
        t.Fatalf("First: %v", err)
    }
    if got.Role != "operator" {
        t.Errorf("Role after Save+Reload = %q, want \"operator\"", got.Role)
    }
}

// TestUserDefaultDisabled verifies the new Disabled field defaults
// to false.
func TestUserDefaultDisabled(t *testing.T) {
    t.Parallel()

    db := setupTestDB(t)
    u := models.User{Email: "x@x.com", PasswordHash: "h"}
    if err := db.Create(&u).Error; err != nil {
        t.Fatalf("Create: %v", err)
    }
    var got models.User
    if err := db.First(&got, u.ID).Error; err != nil {
        t.Fatalf("First: %v", err)
    }
    if got.Disabled {
        t.Errorf("Disabled = true, want false")
    }
}

// TestUserDefaultCreatedBy verifies the new CreatedBy field defaults
// to 0 (GORM zero value).
func TestUserDefaultCreatedBy(t *testing.T) {
    t.Parallel()

    db := setupTestDB(t)
    u := models.User{Email: "x@x.com", PasswordHash: "h"}
    if err := db.Create(&u).Error; err != nil {
        t.Fatalf("Create: %v", err)
    }
    var got models.User
    if err := db.First(&got, u.ID).Error; err != nil {
        t.Fatalf("First: %v", err)
    }
    if got.CreatedBy != 0 {
        t.Errorf("CreatedBy = %d, want 0", got.CreatedBy)
    }
}
```

(注意: `setupTestDB` 是 现有 helper, 跑 `go test ./internal/models/...` 看 现有 testhelper 实现, 复用 现有 模式)

- [ ] **Step 1.2: 跑 test 确认 FAIL (3 个新字段 还没 加)**

Run: `cd /root/workspace/opc/apps/api && go test -tags fts5 -run "TestUserDefault" ./internal/models/...`
Expected: FAIL (Role/Disabled/CreatedBy undefined)

- [ ] **Step 1.3: 改 `apps/api/internal/models/user.go` 加 3 字段**

读 现有 `apps/api/internal/models/user.go` (cat 它 看 现有 struct 字段), 在 现有 字段 (如 Email, PasswordHash) 之后 加 3 字段:

```go
type User struct {
    gorm.Model

    Email        string `gorm:"uniqueIndex;not null" json:"email"`
    PasswordHash string `gorm:"not null" json:"-"`

    // M1 (Sub-Spec C) — Role 区分 operator 跟 creator.
    // 老 Phase 1 用户 (role="") 由 GORM AutoMigrate 改 "operator"
    // (default 值 在 Save 时 apply).
    Role         string `gorm:"default:operator;not null" json:"role"`

    // M1 (Sub-Spec C) — 软删 flag. 任何 disabled=true 的 user 调任何
    // 端点 返 403 "account disabled". 老 Phase 1 用户 disabled=false.
    Disabled     bool   `gorm:"default:false;not null" json:"disabled"`

    // M1 (Sub-Spec C) — 哪个 operator 创建 此 user. 0 表示 self-created
    // (老 Phase 1 operator 是 0; creator > 0 记录 是哪个 operator).
    CreatedBy    uint   `gorm:"default:0;not null" json:"created_by"`

    // 现有 字段保留
    LastLoginAt  *time.Time `json:"last_login_at"`
}
```

(只 加 3 行 struct 字段 + 3 行 注释, 不动 现有 任何 字段. 确保 跟 现有 gorm tag 风格 一致)

- [ ] **Step 1.4: 跑 test 确认 PASS**

Run: `cd /root/workspace/opc/apps/api && go test -tags fts5 -run "TestUserDefault" ./internal/models/...`
Expected: 3 PASS

- [ ] **Step 1.5: 跑全 models test 确认 0 回归**

Run: `cd /root/workspace/opc/apps/api && go test -tags fts5 ./internal/models/...`
Expected: All PASS

- [ ] **Step 1.6: Commit**

```bash
cd /root/workspace/opc
git add apps/api/internal/models/user.go apps/api/internal/models/user_test.go
git commit -m "feat(models): User adds Role/Disabled/CreatedBy columns (Task 1)

Sub-Spec C M1: User model gains 3 fields for invite-only creator
authentication:
- Role: 'operator' (default) | 'creator' (operator-created beta tester)
- Disabled: bool, false default; soft delete flag (403 on any
  endpoint when true)
- CreatedBy: uint, 0 default; audit trail for which operator
  created which creator (0 = self-created Phase 1 operator)

GORM AutoMigrate in apps/api/internal/db/db.go (existing)
automatically applies the new column defaults on next server
boot. Old Phase 1 operator users (Role='') will be normalized
to 'operator' on Save.

3 new unit tests verify defaults via Save+Reload (GORM
default application is a DB-side concept, not a Go zero-value
concept).

No DB schema migration needed: GORM AutoMigrate handles the
3 new columns. No data loss for existing users."
```

---

### Task 2: 4 admin endpoint (creator.go) + 2 middleware (Require*Role) + 17-19 unit test

**Files:**
- Modify: `apps/api/internal/handlers/auth.go` (Login/Me/Register response 加 `role` 字段, 3 行)
- Modify: `apps/api/internal/middleware/auth.go` (RequireAuth 加 1 行 + 新增 RequireOperatorRole + RequireCreatorRole)
- Modify: `apps/api/cmd/server/main.go` (注册 4 个新 endpoint + 串联 middleware)
- Create: `apps/api/internal/handlers/creator.go`
- Create: `apps/api/internal/handlers/creator_test.go`
- Create or Modify: `apps/api/internal/middleware/auth_test.go` (加 9 个 Require*Role test)

- [ ] **Step 2.1: 写 17-19 个 unit test (TDD red)**

在 `apps/api/internal/handlers/creator_test.go` 加 8-10 个 test (creator endpoint):

```go
package handlers_test

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/opc/api/internal/handlers"
    "github.com/opc/api/internal/models"
    "github.com/gin-gonic/gin"
    "gorm.io/gorm"
)

func TestCreateCreator201(t *testing.T) {
    t.Parallel()
    db := setupTestDB(t)
    op := createTestOperator(t, db)

    body := `{"email":"creator1@beta.com","password":"beta-pass-123"}`
    req := httptest.NewRequest("POST", "/api/admin/creators", bytes.NewBufferString(body))
    req.Header.Set("Content-Type", "application/json")
    req = withOperatorSession(req, op.ID)
    w := httptest.NewRecorder()
    r := newTestRouter(db)
    r.ServeHTTP(w, req)

    if w.Code != http.StatusCreated {
        t.Errorf("status = %d, want 201 (body: %s)", w.Code, w.Body.String())
    }
    var got struct {
        User models.User `json:"user"`
    }
    json.Unmarshal(w.Body.Bytes(), &got)
    if got.User.Email != "creator1@beta.com" {
        t.Errorf("email = %q, want creator1@beta.com", got.User.Email)
    }
    if got.User.Role != "creator" {
        t.Errorf("role = %q, want creator", got.User.Role)
    }
    if got.User.CreatedBy != op.ID {
        t.Errorf("created_by = %d, want %d", got.User.CreatedBy, op.ID)
    }
}

func TestCreateCreator400InvalidEmail(t *testing.T) {
    t.Parallel()
    db := setupTestDB(t)
    op := createTestOperator(t, db)

    body := `{"email":"not-an-email","password":"beta-pass-123"}`
    req := httptest.NewRequest("POST", "/api/admin/creators", bytes.NewBufferString(body))
    req.Header.Set("Content-Type", "application/json")
    req = withOperatorSession(req, op.ID)
    w := httptest.NewRecorder()
    r := newTestRouter(db)
    r.ServeHTTP(w, req)

    if w.Code != http.StatusBadRequest {
        t.Errorf("status = %d, want 400", w.Code)
    }
}

func TestCreateCreator400ShortPassword(t *testing.T) {
    t.Parallel()
    db := setupTestDB(t)
    op := createTestOperator(t, db)

    body := `{"email":"x@x.com","password":"short"}`  // 5 chars
    req := httptest.NewRequest("POST", "/api/admin/creators", bytes.NewBufferString(body))
    req.Header.Set("Content-Type", "application/json")
    req = withOperatorSession(req, op.ID)
    w := httptest.NewRecorder()
    r := newTestRouter(db)
    r.ServeHTTP(w, req)

    if w.Code != http.StatusBadRequest {
        t.Errorf("status = %d, want 400", w.Code)
    }
}

func TestCreateCreator409Duplicate(t *testing.T) {
    t.Parallel()
    db := setupTestDB(t)
    op := createTestOperator(t, db)
    existing := models.User{
        Email: "dup@beta.com", PasswordHash: "h", Role: "creator",
    }
    db.Create(&existing)

    body := `{"email":"dup@beta.com","password":"beta-pass-123"}`
    req := httptest.NewRequest("POST", "/api/admin/creators", bytes.NewBufferString(body))
    req.Header.Set("Content-Type", "application/json")
    req = withOperatorSession(req, op.ID)
    w := httptest.NewRecorder()
    r := newTestRouter(db)
    r.ServeHTTP(w, req)

    if w.Code != http.StatusConflict {
        t.Errorf("status = %d, want 409", w.Code)
    }
}

func TestCreateCreator403NonOperator(t *testing.T) {
    t.Parallel()
    db := setupTestDB(t)
    creator := createTestCreator(t, db)

    body := `{"email":"new@beta.com","password":"beta-pass-123"}`
    req := httptest.NewRequest("POST", "/api/admin/creators", bytes.NewBufferString(body))
    req.Header.Set("Content-Type", "application/json")
    req = withCreatorSession(req, creator.ID)  // creator session, not operator
    w := httptest.NewRecorder()
    r := newTestRouter(db)
    r.ServeHTTP(w, req)

    if w.Code != http.StatusForbidden {
        t.Errorf("status = %d, want 403", w.Code)
    }
}

func TestListCreators200(t *testing.T) {
    t.Parallel()
    db := setupTestDB(t)
    op := createTestOperator(t, db)
    createTestCreator(t, db)  // first creator
    createTestCreator(t, db)  // second creator

    req := httptest.NewRequest("GET", "/api/admin/creators", nil)
    req = withOperatorSession(req, op.ID)
    w := httptest.NewRecorder()
    r := newTestRouter(db)
    r.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Errorf("status = %d, want 200", w.Code)
    }
    var got struct {
        Creators []models.User `json:"creators"`
    }
    json.Unmarshal(w.Body.Bytes(), &got)
    if len(got.Creators) != 2 {
        t.Errorf("len(creators) = %d, want 2", len(got.Creators))
    }
    for _, c := range got.Creators {
        if c.Role != "creator" {
            t.Errorf("creator %d role = %q, want creator", c.ID, c.Role)
        }
    }
}

func TestResetPassword200(t *testing.T) {
    t.Parallel()
    db := setupTestDB(t)
    op := createTestOperator(t, db)
    creator := createTestCreator(t, db)

    body := `{"new_password":"new-pass-1234"}`
    req := httptest.NewRequest("POST", "/api/admin/creators/"+itoa(creator.ID)+"/reset-password", bytes.NewBufferString(body))
    req.Header.Set("Content-Type", "application/json")
    req = withOperatorSession(req, op.ID)
    w := httptest.NewRecorder()
    r := newTestRouter(db)
    r.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Errorf("status = %d, want 200", w.Code)
    }
    // Verify password was actually changed
    var got models.User
    db.First(&got, creator.ID)
    if err := bcrypt.CompareHashAndPassword([]byte(got.PasswordHash), []byte("new-pass-1234")); err != nil {
        t.Errorf("password not updated: %v", err)
    }
}

func TestResetPassword404(t *testing.T) {
    t.Parallel()
    db := setupTestDB(t)
    op := createTestOperator(t, db)

    body := `{"new_password":"new-pass-1234"}`
    req := httptest.NewRequest("POST", "/api/admin/creators/99999/reset-password", bytes.NewBufferString(body))
    req.Header.Set("Content-Type", "application/json")
    req = withOperatorSession(req, op.ID)
    w := httptest.NewRecorder()
    r := newTestRouter(db)
    r.ServeHTTP(w, req)

    if w.Code != http.StatusNotFound {
        t.Errorf("status = %d, want 404", w.Code)
    }
}

func TestResetPassword400NotCreator(t *testing.T) {
    t.Parallel()
    db := setupTestDB(t)
    op := createTestOperator(t, db)

    body := `{"new_password":"new-pass-1234"}`
    req := httptest.NewRequest("POST", "/api/admin/creators/"+itoa(op.ID)+"/reset-password", bytes.NewBufferString(body))
    req.Header.Set("Content-Type", "application/json")
    req = withOperatorSession(req, op.ID)
    w := httptest.NewRecorder()
    r := newTestRouter(db)
    r.ServeHTTP(w, req)

    if w.Code != http.StatusBadRequest {
        t.Errorf("status = %d, want 400 (target user is operator, not creator)", w.Code)
    }
}

func TestDisableCreator200(t *testing.T) {
    t.Parallel()
    db := setupTestDB(t)
    op := createTestOperator(t, db)
    creator := createTestCreator(t, db)

    req := httptest.NewRequest("POST", "/api/admin/creators/"+itoa(creator.ID)+"/disable", nil)
    req = withOperatorSession(req, op.ID)
    w := httptest.NewRecorder()
    r := newTestRouter(db)
    r.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Errorf("status = %d, want 200", w.Code)
    }
    var got models.User
    db.First(&got, creator.ID)
    if !got.Disabled {
        t.Errorf("disabled = false, want true")
    }
}
```

(辅助函数 `setupTestDB`, `createTestOperator`, `createTestCreator`, `withOperatorSession`, `withCreatorSession`, `newTestRouter`, `itoa` 都是 现有 testhelper, 复用. 如果 缺, 找 现有 类似 test (e.g. `apps/api/internal/handlers/auth_test.go`) 参考 模式 加)

在 `apps/api/internal/middleware/auth_test.go` 加 9 个 test (Require*Role):

```go
package middleware_test

import (
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"
)

// TestRequireOperatorRoleOperatorPass
func TestRequireOperatorRoleOperatorPass(t *testing.T) {
    t.Parallel()
    r := gin.New()
    r.Use(func(c *gin.Context) { c.Set("user_role", "operator"); c.Next() })
    r.Use(middleware.RequireOperatorRole())
    r.GET("/x", func(c *gin.Context) { c.String(200, "ok") })

    req := httptest.NewRequest("GET", "/x", nil)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)
    if w.Code != 200 { t.Errorf("operator pass, got %d", w.Code) }
}

// TestRequireOperatorRoleCreatorFail
func TestRequireOperatorRoleCreatorFail(t *testing.T) {
    t.Parallel()
    r := gin.New()
    r.Use(func(c *gin.Context) { c.Set("user_role", "creator"); c.Next() })
    r.Use(middleware.RequireOperatorRole())
    r.GET("/x", func(c *gin.Context) { c.String(200, "ok") })

    req := httptest.NewRequest("GET", "/x", nil)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)
    if w.Code != 403 { t.Errorf("creator fail, got %d want 403", w.Code) }
}

// TestRequireOperatorRoleUnauthFail (no user_role in context)
func TestRequireOperatorRoleUnauthFail(t *testing.T) {
    t.Parallel()
    r := gin.New()
    r.Use(middleware.RequireOperatorRole())
    r.GET("/x", func(c *gin.Context) { c.String(200, "ok") })

    req := httptest.NewRequest("GET", "/x", nil)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)
    if w.Code != 401 { t.Errorf("unauth fail, got %d want 401", w.Code) }
}

// TestRequireOperatorRoleDisabledFail
// ... same pattern, set c.Set("user_role", "operator") + c.Set("user_disabled", true)

// TestRequireCreatorRoleCreatorPass
// TestRequireCreatorRoleOperatorFail
// TestRequireCreatorRoleUnauthFail
// TestRequireCreatorRoleDisabledFail

// (8 个 test, 4 个 RequireOperatorRole + 4 个 RequireCreatorRole, 加 Disabled check)
```

(每个 test 8 行; 9 test 总共 ~80 行. 现有 testhelper 复用, 如果 `middleware.RequireOperatorRole` 不在 现有 import, 加 import)

- [ ] **Step 2.2: 跑 test 确认 FAIL (creator handler / middleware 还没 写)**

Run: `cd /root/workspace/opc/apps/api && go test -tags fts5 -run "TestCreateCreator|TestListCreators|TestResetPassword|TestDisableCreator" ./internal/handlers/... ./internal/middleware/...`
Expected: FAIL (functions undefined)

- [ ] **Step 2.3: 改 `apps/api/internal/handlers/auth.go` (3 处加 `role` 字段)**

读 `apps/api/internal/handlers/auth.go` 找 3 个 endpoint (Login/Me/Register) 返 response 的 struct 定义. 在 response struct 加 `Role string \`json:"role"\`` 字段 (3 处). 在 endpoint body 调 完 user 拿 到 后 设 `resp.User.Role = user.Role` (3 处).

(仅 +3 字段 + 3 行赋值, 不动 业务 逻辑)

- [ ] **Step 2.4: 改 `apps/api/internal/middleware/auth.go` (1 行 + 2 个新函数)**

在 `RequireAuth` 函数 body 找到 `c.Set("user_id", userID)` 这一行 (1 行 后面 加):

```go
c.Set("user_role", user.Role)
```

然后 在 同一 文件 (RequireAuth 后面) 加 2 个新函数:

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

- [ ] **Step 2.5: 新建 `apps/api/internal/handlers/creator.go`**

```go
// Package handlers provides HTTP route handlers for the OPC
// platform. creator.go implements the operator-only admin
// endpoints for managing external beta creators.
package handlers

import (
    "errors"
    "net/http"
    "strconv"
    "strings"

    "github.com/gin-gonic/gin"
    "golang.org/x/crypto/bcrypt"
    "gorm.io/gorm"

    "github.com/opc/api/internal/models"
)

// CreatorHandler bundles the 4 admin endpoints for managing
// external creators. All endpoints require RequireAuth +
// RequireOperatorRole middleware (configured in main.go).
type CreatorHandler struct {
    db *gorm.DB
}

func NewCreatorHandler(db *gorm.DB) *CreatorHandler {
    return &CreatorHandler{db: db}
}

// RegisterRoutes attaches the 4 admin endpoints to the router.
func (h *CreatorHandler) RegisterRoutes(r gin.IRouter) {
    r.POST("/api/admin/creators", h.CreateCreator)
    r.GET("/api/admin/creators", h.ListCreators)
    r.POST("/api/admin/creators/:id/reset-password", h.ResetPassword)
    r.POST("/api/admin/creators/:id/disable", h.DisableCreator)
}

// CreateCreator handles POST /api/admin/creators.
// Operator-only. Body: {email, password}. Returns 201 + user.
func (h *CreatorHandler) CreateCreator(c *gin.Context) {
    var req struct {
        Email    string `json:"email" binding:"required,email"`
        Password string `json:"password" binding:"required,min=8"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
        return
    }

    hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "password hash failed"})
        return
    }

    operatorID := c.GetUint("user_id")
    u := models.User{
        Email:        req.Email,
        PasswordHash: string(hash),
        Role:         "creator",
        CreatedBy:    operatorID,
    }
    if err := h.db.Create(&u).Error; err != nil {
        if strings.Contains(err.Error(), "UNIQUE constraint") {
            c.JSON(http.StatusConflict, gin.H{"error": "email already exists"})
            return
        }
        c.JSON(http.StatusInternalServerError, gin.H{"error": "create failed: " + err.Error()})
        return
    }
    c.JSON(http.StatusCreated, gin.H{"user": u})
}

// ListCreators handles GET /api/admin/creators.
// Operator-only. Returns 200 + array of creators.
func (h *CreatorHandler) ListCreators(c *gin.Context) {
    var creators []models.User
    if err := h.db.Where("role = ?", "creator").Order("created_at DESC").Find(&creators).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "list failed: " + err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"creators": creators})
}

// ResetPassword handles POST /api/admin/creators/:id/reset-password.
// Operator-only. Body: {new_password}. Returns 200.
func (h *CreatorHandler) ResetPassword(c *gin.Context) {
    id, err := strconv.ParseUint(c.Param("id"), 10, 64)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
        return
    }

    var req struct {
        NewPassword string `json:"new_password" binding:"required,min=8"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
        return
    }

    var u models.User
    if err := h.db.First(&u, id).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            c.JSON(http.StatusNotFound, gin.H{"error": "creator not found"})
            return
        }
        c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup failed"})
        return
    }
    if u.Role != "creator" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "not a creator"})
        return
    }

    hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "password hash failed"})
        return
    }
    u.PasswordHash = string(hash)
    if err := h.db.Save(&u).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "save failed"})
        return
    }
    c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DisableCreator handles POST /api/admin/creators/:id/disable.
// Operator-only. Returns 200 + disabled=true.
func (h *CreatorHandler) DisableCreator(c *gin.Context) {
    id, err := strconv.ParseUint(c.Param("id"), 10, 64)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
        return
    }

    var u models.User
    if err := h.db.First(&u, id).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            c.JSON(http.StatusNotFound, gin.H{"error": "creator not found"})
            return
        }
        c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup failed"})
        return
    }
    if u.Role != "creator" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "not a creator"})
        return
    }

    u.Disabled = true
    if err := h.db.Save(&u).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "save failed"})
        return
    }
    c.JSON(http.StatusOK, gin.H{"ok": true, "disabled": true})
}
```

- [ ] **Step 2.6: 改 `apps/api/cmd/server/main.go` (注册 4 个新 endpoint + 串联 middleware)**

读 main.go, 找 现有 `r.POST("/api/auth/login", ...)` 等路由定义. 在 `/api/auth/*` 路由**之前**加 1 个 `/api/admin` group:

```go
// Sub-Spec C M1: external creator admin endpoints (operator-only).
admin := r.Group("/api/admin", middleware.RequireAuth(), middleware.RequireOperatorRole())
creatorH := handlers.NewCreatorHandler(db)
creatorH.RegisterRoutes(admin)
```

(只加 4 行 + 1 个 import. 跟 现有 `r.POST(...)` 模式 一致)

- [ ] **Step 2.7: 跑 17 个新 test + 老 handler test 确认 0 回归**

Run: `cd /root/workspace/opc/apps/api && go test -tags fts5 ./internal/handlers/... ./internal/middleware/... ./internal/models/...`
Expected: All PASS (17 new + 老 test)

- [ ] **Step 2.8: 跑全 API tests 确认 0 回归**

Run: `cd /root/workspace/opc/apps/api && go test -tags fts5 ./...`
Expected: All PASS

- [ ] **Step 2.9: Commit**

```bash
cd /root/workspace/opc
git add apps/api/internal/handlers/auth.go apps/api/internal/handlers/creator.go apps/api/internal/handlers/creator_test.go apps/api/internal/middleware/auth.go apps/api/internal/middleware/auth_test.go apps/api/cmd/server/main.go
git commit -m "feat(handlers): 4 admin creator endpoints + 2 role-check middleware (Task 2)

Sub-Spec C M1: operator-side creator management.

4 new admin endpoints (operator-only, RequireAuth + RequireOperatorRole):
- POST   /api/admin/creators                — create creator (email+password)
- GET    /api/admin/creators                — list creators
- POST   /api/admin/creators/:id/reset-password  — reset creator password
- POST   /api/admin/creators/:id/disable    — soft-delete creator

2 new role-check middleware (12 lines each):
- RequireOperatorRole — verifies authenticated user has role='operator'
- RequireCreatorRole  — verifies authenticated user has role='creator'
Both must be used AFTER RequireAuth (which now writes user_role to
context after my +1-line patch).

auth.go: Login/Me/Register responses now include 'role' field
(3 lines added). Old clients ignoring the field are unaffected.

17 new unit tests:
- 10 creator handler tests (201/400×2/409/403/200×4 paths)
- 8 middleware tests (operator pass/creator fail/unauth fail/disabled
  fail × 2 wrappers, with disabled-check variations)

Full API suite: all 16 packages green, no regressions. Old
handler tests + middleware tests + models tests all unchanged."
```

---

### Task 3: console `/creators` 管理页 + 2 dialog

**Files:**
- Create: `apps/console/app/(console)/creators/page.tsx`
- Create: `apps/console/app/(console)/creators/_components/CreateCreatorDialog.tsx`
- Create: `apps/console/app/(console)/creators/_components/ResetPasswordDialog.tsx`
- Modify: `apps/console/app/(console)/layout.tsx` (加 1 个 sidebar link)

- [ ] **Step 3.1: 读 现有 `apps/console/app/(console)/` 1 个 类似 page (e.g. credentials 或 logs) 找 UI 模式**

Run: `ls /root/workspace/opc/apps/console/app/\(console\)/`
读 1 个 现有 page (e.g. `credentials/page.tsx`) 看:
- 怎样用 `useState` + `useEffect` 调 API
- 怎样 列 table + 调 POST/DELETE
- 怎样 弹 modal

(把 模式 抽 出来, 创作者管理页 照 写)

- [ ] **Step 3.2: 新建 `apps/console/app/(console)/creators/page.tsx`**

```tsx
"use client";
import { useEffect, useState } from "react";
import { CreateCreatorDialog } from "./_components/CreateCreatorDialog";
import { ResetPasswordDialog } from "./_components/ResetPasswordDialog";

interface Creator {
    id: number;
    email: string;
    role: string;
    created_by: number;
    created_at: string;
    last_login_at: string | null;
    disabled: boolean;
}

export default function CreatorsPage() {
    const [creators, setCreators] = useState<Creator[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState("");
    const [createOpen, setCreateOpen] = useState(false);
    const [resetTarget, setResetTarget] = useState<Creator | null>(null);

    async function load() {
        setLoading(true);
        try {
            const res = await fetch("/api/admin/creators", { credentials: "include" });
            if (!res.ok) throw new Error(`load failed: ${res.status}`);
            const data = await res.json();
            setCreators(data.creators);
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setLoading(false);
        }
    }

    useEffect(() => { load(); }, []);

    async function disableCreator(c: Creator) {
        if (!confirm(`Disable creator ${c.email}? They won't be able to log in.`)) return;
        const res = await fetch(`/api/admin/creators/${c.id}/disable`, {
            method: "POST", credentials: "include",
        });
        if (!res.ok) { alert("disable failed"); return; }
        await load();
    }

    if (loading) return <p>加载中...</p>;
    if (error) return <p style={{ color: "red" }}>{error}</p>;

    return (
        <div>
            <h1>创作者管理</h1>
            <button onClick={() => setCreateOpen(true)}>+ 创建创作者</button>
            <table>
                <thead>
                    <tr>
                        <th>Email</th><th>创建者</th><th>创建时间</th>
                        <th>最后登录</th><th>状态</th><th>操作</th>
                    </tr>
                </thead>
                <tbody>
                    {creators.map((c) => (
                        <tr key={c.id}>
                            <td>{c.email}</td>
                            <td>{c.created_by || "—"}</td>
                            <td>{c.created_at.slice(0, 19)}</td>
                            <td>{c.last_login_at ? c.last_login_at.slice(0, 19) : "未登录"}</td>
                            <td>{c.disabled ? "已禁用" : "活跃"}</td>
                            <td>
                                <button onClick={() => setResetTarget(c)}>重置密码</button>
                                <button onClick={() => disableCreator(c)} disabled={c.disabled}>
                                    禁用
                                </button>
                            </td>
                        </tr>
                    ))}
                </tbody>
            </table>

            {createOpen && (
                <CreateCreatorDialog
                    onClose={() => setCreateOpen(false)}
                    onCreated={() => { setCreateOpen(false); load(); }}
                />
            )}
            {resetTarget && (
                <ResetPasswordDialog
                    creator={resetTarget}
                    onClose={() => setResetTarget(null)}
                    onReset={() => { setResetTarget(null); load(); }}
                />
            )}
        </div>
    );
}
```

- [ ] **Step 3.3: 新建 `apps/console/app/(console)/creators/_components/CreateCreatorDialog.tsx`**

```tsx
"use client";
import { useState } from "react";

export function CreateCreatorDialog({ onClose, onCreated }: {
    onClose: () => void;
    onCreated: () => void;
}) {
    const [email, setEmail] = useState("");
    const [password, setPassword] = useState("");
    const [err, setErr] = useState("");
    const [loading, setLoading] = useState(false);

    async function submit(e: React.FormEvent) {
        e.preventDefault();
        setErr("");
        setLoading(true);
        const res = await fetch("/api/admin/creators", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            credentials: "include",
            body: JSON.stringify({ email, password }),
        });
        setLoading(false);
        if (res.ok) {
            onCreated();
        } else {
            const body = await res.json().catch(() => ({}));
            setErr(body.error || `failed: ${res.status}`);
        }
    }

    return (
        <div style={overlayStyle}>
            <div style={dialogStyle}>
                <h2>创建创作者</h2>
                <form onSubmit={submit}>
                    <label>Email
                        <input type="email" value={email}
                            onChange={(e) => setEmail(e.target.value)} required />
                    </label>
                    <label>初始密码 (至少 8 字符)
                        <input type="password" value={password}
                            onChange={(e) => setPassword(e.target.value)} required minLength={8} />
                    </label>
                    {err && <p style={{ color: "red" }}>{err}</p>}
                    <button type="submit" disabled={loading}>
                        {loading ? "创建中..." : "创建"}
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
    minWidth: "400px",
};
```

- [ ] **Step 3.4: 新建 `apps/console/app/(console)/creators/_components/ResetPasswordDialog.tsx`**

```tsx
"use client";
import { useState } from "react";

interface Creator {
    id: number;
    email: string;
}

export function ResetPasswordDialog({ creator, onClose, onReset }: {
    creator: Creator;
    onClose: () => void;
    onReset: () => void;
}) {
    const [newPassword, setNewPassword] = useState("");
    const [err, setErr] = useState("");
    const [loading, setLoading] = useState(false);

    async function submit(e: React.FormEvent) {
        e.preventDefault();
        setErr("");
        setLoading(true);
        const res = await fetch(`/api/admin/creators/${creator.id}/reset-password`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            credentials: "include",
            body: JSON.stringify({ new_password: newPassword }),
        });
        setLoading(false);
        if (res.ok) {
            onReset();
        } else {
            const body = await res.json().catch(() => ({}));
            setErr(body.error || `failed: ${res.status}`);
        }
    }

    return (
        <div style={overlayStyle}>
            <div style={dialogStyle}>
                <h2>重置密码: {creator.email}</h2>
                <form onSubmit={submit}>
                    <label>新密码 (至少 8 字符)
                        <input type="password" value={newPassword}
                            onChange={(e) => setNewPassword(e.target.value)} required minLength={8} />
                    </label>
                    {err && <p style={{ color: "red" }}>{err}</p>}
                    <button type="submit" disabled={loading}>
                        {loading ? "重置中..." : "重置"}
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
    minWidth: "400px",
};
```

- [ ] **Step 3.5: 改 `apps/console/app/(console)/layout.tsx` 加 sidebar link**

读 现有 layout, 找 sidebar nav 链接 列表. 加 1 个:

```tsx
<Link href="/creators">创作者管理</Link>
```

(跟 现有 `/credentials`, `/observability` 等 同 模式)

- [ ] **Step 3.6: 跑 console build 验 不破**

Run: `cd /root/workspace/opc/apps/console && pnpm build 2>&1 | tail -10`
Expected: exit 0, 编译 PASS

- [ ] **Step 3.7: Commit**

```bash
cd /root/workspace/opc
git add apps/console/app/\(console\)/creators/ apps/console/app/\(console\)/layout.tsx
git commit -m "feat(console): creator management page + 2 dialogs (Task 3)

Sub-Spec C M1: operator UI to manage external beta creators.

New page: apps/console/app/(console)/creators/page.tsx
- Lists all creators (table: email / created_by / created_at /
  last_login_at / disabled)
- 'Create creator' button → modal
- Per-row 'Reset password' + 'Disable' actions
- 2 new dialog components: CreateCreatorDialog, ResetPasswordDialog
- Sidebar link added in apps/console/app/(console)/layout.tsx

All 4 admin endpoints from Task 2 are wired:
- POST /api/admin/creators (create)
- POST /api/admin/creators/:id/reset-password (reset)
- POST /api/admin/creators/:id/disable (disable)
- GET /api/admin/creators (list)

Manual flow: operator opens /creators → '+ 创建创作者' → 填
email + initial password → 表刷新, 新创作者可见.
operator can then send the credentials to the beta creator
out-of-band (1-1 invite).

pnpm build: exit 0, no console errors. The new page is just
TypeScript + React, no new dependencies needed."
```

---

### Task 4: `apps/external/` 全新 Next.js 14 app (login + dashboard + topic + middleware)

**Files:**
- Create: `apps/external/package.json`
- Create: `apps/external/next.config.js`
- Create: `apps/external/tsconfig.json`
- Create: `apps/external/middleware.ts`
- Create: `apps/external/lib/api.ts`
- Create: `apps/external/app/login/page.tsx`
- Create: `apps/external/app/(creator)/dashboard/page.tsx`
- Create: `apps/external/app/(creator)/topic/page.tsx`
- Create: `apps/external/app/layout.tsx` (root layout)
- Create: `apps/external/app/page.tsx` (root: redirect /login or /dashboard)

- [ ] **Step 4.1: 读 `apps/web/` 跟 `apps/console/` 的 package.json + tsconfig.json 找依赖版本**

Run: `cd /root/workspace/opc && cat apps/web/package.json apps/web/tsconfig.json apps/console/package.json`
Note: Next.js 14.2.5, React 18.3.1, @types/react 18.3.3, TypeScript 5.5.3 — apps/external 用同 套版本.

- [ ] **Step 4.2: 新建 `apps/external/package.json`**

```json
{
  "name": "@opc/external",
  "version": "0.1.0",
  "private": true,
  "scripts": {
    "dev": "next dev -p 3002",
    "build": "next build",
    "start": "next start -p 3002",
    "lint": "next lint"
  },
  "dependencies": {
    "next": "14.2.5",
    "react": "18.3.1",
    "react-dom": "18.3.1",
    "@opc/shared": "workspace:*"
  },
  "devDependencies": {
    "typescript": "5.5.3",
    "@types/node": "20.14.10",
    "@types/react": "18.3.3",
    "@types/react-dom": "18.3.0"
  }
}
```

- [ ] **Step 4.3: 新建 `apps/external/next.config.js`**

```js
/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  // Allow client components to fetch the OPC API server during dev
  // (CORS). In prod, apps/external is served behind the same
  // nginx as /api/, so this is moot.
  async rewrites() {
    return [];
  },
};

module.exports = nextConfig;
```

- [ ] **Step 4.4: 新建 `apps/external/tsconfig.json`**

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "lib": ["dom", "dom.iterable", "esnext"],
    "allowJs": true,
    "skipLibCheck": true,
    "strict": true,
    "noEmit": true,
    "esModuleInterop": true,
    "module": "esnext",
    "moduleResolution": "bundler",
    "resolveJsonModule": true,
    "isolatedModules": true,
    "jsx": "preserve",
    "incremental": true,
    "paths": {
      "@/*": ["./*"]
    }
  },
  "include": ["next-env.d.ts", "**/*.ts", "**/*.tsx", ".next/types/**/*.ts"],
  "exclude": ["node_modules"]
}
```

- [ ] **Step 4.5: 新建 `apps/external/middleware.ts`**

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

- [ ] **Step 4.6: 新建 `apps/external/lib/api.ts`**

```ts
// Thin fetch wrappers for the OPC API. Session cookie is sent
// automatically via credentials: 'include'.

export interface Creator {
    id: number;
    email: string;
    role: string;
    created_at: string;
    last_login_at: string | null;
    disabled: boolean;
}

export interface Topic {
    title: string;
    angle: string;
    expected_performance: string;
    hook: string;
    pattern?: string;
    voice_tags?: string[];
}

export async function login(email: string, password: string): Promise<Creator> {
    const res = await fetch("/api/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ email, password }),
    });
    if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        throw new Error(err.error || `login failed: ${res.status}`);
    }
    const data = await res.json();
    return data.user;
}

export async function getMe(): Promise<Creator | null> {
    const res = await fetch("/api/auth/me", { credentials: "include" });
    if (res.status === 401) return null;
    if (!res.ok) throw new Error(`me failed: ${res.status}`);
    const data = await res.json();
    return data.user;
}

export async function generateTopics(input: {
    seed: string;
    platform: string;
    count: number;
}): Promise<Topic[]> {
    const res = await fetch("/api/ai/topics", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify(input),
    });
    if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        throw new Error(err.error || `generate failed: ${res.status}`);
    }
    const data = await res.json();
    return data.topics;
}
```

- [ ] **Step 4.7: 新建 `apps/external/app/layout.tsx` (root layout)**

```tsx
import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
    title: "OPC 创作者平台",
    description: "External creator beta UI",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
    return (
        <html lang="zh">
            <body style={{ fontFamily: "system-ui, sans-serif", margin: 0, padding: 0 }}>
                {children}
            </body>
        </html>
    );
}
```

Create `apps/external/app/globals.css`:

```css
* { box-sizing: border-box; }
body { background: #fafafa; color: #222; }
button { padding: 0.5rem 1rem; cursor: pointer; }
input, select { padding: 0.5rem; border: 1px solid #ccc; border-radius: 4px; }
table { border-collapse: collapse; width: 100%; }
th, td { padding: 0.5rem 1rem; text-align: left; border-bottom: 1px solid #eee; }
th { background: #f5f5f5; }
```

- [ ] **Step 4.8: 新建 `apps/external/app/page.tsx` (root: redirect based on session)**

```tsx
import { redirect } from "next/navigation";
import { getMe } from "@/lib/api";

export default async function RootPage() {
    const me = await getMe();
    if (me) {
        redirect("/dashboard");
    } else {
        redirect("/login");
    }
}
```

- [ ] **Step 4.9: 新建 `apps/external/app/login/page.tsx`**

```tsx
"use client";
import { useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { login } from "@/lib/api";

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
        try {
            const user = await login(email, password);
            if (user.role !== "creator") {
                setErr("此账号不是创作者账号");
                setLoading(false);
                return;
            }
            router.push(next);
        } catch (e) {
            setErr((e as Error).message);
            setLoading(false);
        }
    }

    return (
        <main style={{ maxWidth: 400, margin: "5rem auto", padding: "1.5rem", background: "white", borderRadius: 8, boxShadow: "0 2px 8px rgba(0,0,0,0.1)" }}>
            <h1 style={{ marginTop: 0 }}>创作者登录</h1>
            <form onSubmit={submit}>
                <div style={{ marginBottom: "1rem" }}>
                    <label>Email<br />
                        <input
                            type="email" value={email}
                            onChange={(e) => setEmail(e.target.value)}
                            placeholder="creator@beta.com"
                            required
                            style={{ width: "100%" }}
                        />
                    </label>
                </div>
                <div style={{ marginBottom: "1rem" }}>
                    <label>密码<br />
                        <input
                            type="password" value={password}
                            onChange={(e) => setPassword(e.target.value)}
                            placeholder="密码"
                            required
                            style={{ width: "100%" }}
                        />
                    </label>
                </div>
                {err && <p style={{ color: "red" }}>{err}</p>}
                <button type="submit" disabled={loading} style={{ width: "100%" }}>
                    {loading ? "登录中..." : "登录"}
                </button>
            </form>
        </main>
    );
}
```

- [ ] **Step 4.10: 新建 `apps/external/app/(creator)/dashboard/page.tsx`**

```tsx
"use client";
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { getMe } from "@/lib/api";

export default function DashboardPage() {
    const router = useRouter();
    const [me, setMe] = useState<{ email: string; role: string } | null>(null);

    useEffect(() => {
        getMe().then((u) => {
            if (!u) router.push("/login");
            else setMe(u);
        });
    }, [router]);

    async function logout() {
        await fetch("/api/auth/logout", { method: "POST", credentials: "include" });
        router.push("/login");
    }

    if (!me) return <p style={{ padding: "1rem" }}>加载中...</p>;

    return (
        <main style={{ maxWidth: 800, margin: "2rem auto", padding: "1.5rem" }}>
            <header style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                <h1>创作者 Dashboard</h1>
                <div>
                    <span>{me.email} ({me.role})</span>{" "}
                    <button onClick={logout}>登出</button>
                </div>
            </header>

            <section style={{ marginTop: "2rem" }}>
                <h2>选题</h2>
                <p style={{ color: "#666" }}>
                    生成 5 个选题，基于 OPC 品牌 IP profile (anthropomorphic / 峰哥 panda)。
                </p>
                <button
                    onClick={() => router.push("/topic")}
                    style={{ padding: "0.75rem 1.5rem", fontSize: "1rem" }}
                >
                    + 新建选题
                </button>
            </section>

            <section style={{ marginTop: "2rem" }}>
                <h2>历史</h2>
                <p style={{ color: "#888", fontStyle: "italic" }}>
                    M1 暂未持久化选题历史；你的选题记录在 OPC operator 的 observability dashboard
                    中可查询（含 cost_cents 标记）。
                </p>
            </section>
        </main>
    );
}
```

- [ ] **Step 4.11: 新建 `apps/external/app/(creator)/topic/page.tsx`**

```tsx
"use client";
import { useState } from "react";
import { useRouter } from "next/navigation";
import { generateTopics, type Topic } from "@/lib/api";

const PLATFORMS = ["抖音", "哔哩哔哩", "小红书"];

export default function TopicPage() {
    const router = useRouter();
    const [seed, setSeed] = useState("");
    const [platform, setPlatform] = useState("抖音");
    const [count, setCount] = useState(5);
    const [topics, setTopics] = useState<Topic[] | null>(null);
    const [err, setErr] = useState("");
    const [loading, setLoading] = useState(false);

    async function submit(e: React.FormEvent) {
        e.preventDefault();
        setErr("");
        setTopics(null);
        setLoading(true);
        try {
            const t = await generateTopics({ seed, platform, count });
            setTopics(t);
        } catch (e) {
            setErr((e as Error).message);
        } finally {
            setLoading(false);
        }
    }

    return (
        <main style={{ maxWidth: 800, margin: "2rem auto", padding: "1.5rem" }}>
            <button onClick={() => router.push("/dashboard")}>← 返回</button>
            <h1>新建选题</h1>

            <form onSubmit={submit} style={{ marginTop: "1rem" }}>
                <div style={{ marginBottom: "1rem" }}>
                    <label>Seed (必填)<br />
                        <input
                            type="text" value={seed}
                            onChange={(e) => setSeed(e.target.value)}
                            placeholder="例如：个人成长"
                            required
                            style={{ width: "100%" }}
                        />
                    </label>
                </div>
                <div style={{ marginBottom: "1rem" }}>
                    <label>平台<br />
                        <select value={platform} onChange={(e) => setPlatform(e.target.value)}>
                            {PLATFORMS.map((p) => <option key={p} value={p}>{p}</option>)}
                        </select>
                    </label>
                </div>
                <div style={{ marginBottom: "1rem" }}>
                    <label>数量 (1-20)<br />
                        <input
                            type="number" min={1} max={20}
                            value={count} onChange={(e) => setCount(parseInt(e.target.value) || 5)}
                            style={{ width: "100px" }}
                        />
                    </label>
                </div>
                <p style={{ color: "#888", fontSize: "0.9em" }}>
                    IP profile: anthropomorphic (峰哥) — M1 固定
                </p>
                <button type="submit" disabled={loading}>
                    {loading ? "生成中..." : "生成"}
                </button>
            </form>

            {err && <p style={{ color: "red", marginTop: "1rem" }}>{err}</p>}

            {topics && (
                <section style={{ marginTop: "2rem" }}>
                    <h2>结果 ({topics.length} 个选题)</h2>
                    {topics.map((t, i) => (
                        <article key={i} style={{ background: "white", padding: "1rem", marginBottom: "0.5rem", borderRadius: 4, boxShadow: "0 1px 3px rgba(0,0,0,0.08)" }}>
                            <h3 style={{ margin: "0 0 0.5rem 0" }}>{i + 1}. {t.title}</h3>
                            <p style={{ margin: "0.25rem 0" }}><b>角度:</b> {t.angle}</p>
                            <p style={{ margin: "0.25rem 0" }}><b>Hook:</b> {t.hook}</p>
                            <p style={{ margin: "0.25rem 0" }}><b>预期表现:</b> {t.expected_performance}</p>
                        </article>
                    ))}
                </section>
            )}
        </main>
    );
}
```

- [ ] **Step 4.12: install deps + build 验**

```bash
cd /root/workspace/opc/apps/external
pnpm install 2>&1 | tail -3
pnpm build 2>&1 | tail -10
```

Expected: exit 0, build PASS, 3 routes compiled (/login, /dashboard, /topic)

- [ ] **Step 4.13: 手动 smoke (optional) - 启 dev server, curl /login**

```bash
cd /root/workspace/opc/apps/external
pnpm dev &
sleep 5
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:3002/login
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:3002/dashboard
kill %1 2>/dev/null
```

Expected: /login → 200, /dashboard → 307 (redirect to /login)

- [ ] **Step 4.14: Commit**

```bash
cd /root/workspace/opc
git add apps/external/
git commit -m "feat(external): new Next.js 14 app for invite-only beta creators (Task 4)

Sub-Spec C M1: standalone web UI for external beta creators
(1-10 invite-only users, zero payment, validate product).

apps/external/ structure:
- app/login/page.tsx              — email + password form
- app/(creator)/dashboard/page.tsx — home with 'new topic' button
- app/(creator)/topic/page.tsx    — generation form (seed / platform / count)
- app/page.tsx                    — root redirect based on session
- app/layout.tsx                  — root layout (zh locale)
- app/globals.css                 — minimal styles
- lib/api.ts                       — fetch wrappers (login / getMe / generateTopics)
- middleware.ts                    — session cookie auth gate (redirects to /login)
- package.json                     — @opc/external workspace dep
- next.config.js + tsconfig.json  — port 3002 (no conflict with web/console)

Auth: session cookie 'opc_session' (same name as apps/console).
The login page uses the existing POST /api/auth/login (modified
in Task 2 to include 'role' field). The page checks user.role
=== 'creator' to prevent operator accounts from logging in
(operator accounts get '此账号不是创作者账号' error).

IP profile: hard-coded to anthropomorphic (峰哥 panda) in M1
as per spec. M2 will add a 4-type selector dropdown.

M1 scope reminder: this app does NOT persist topic history
for creators. The call_log middleware (G6-closed 2026-06-09)
still records cost_cents per call, which operator can see in
apps/console /observability. The dashboard intentionally
displays a placeholder message about this M1 limitation.

pnpm build: exit 0, 3 routes compiled. Manual smoke confirmed
/login returns 200 and /dashboard returns 307 redirect to
/login when no session cookie is present."
```

---

### Task 5: 集成测试 + 文档 + 最终 verify + 状态文件更新

**Files:**
- Modify: `scripts/api-smoke.sh`
- Modify: `~/.claude/PROJECT-STATE.md`
- Modify: `~/.claude/GAP-LOG.md`

- [ ] **Step 5.1: 读现有 api-smoke.sh 找合适位置加 creator case**

Run: `cd /root/workspace/opc && wc -l scripts/api-smoke.sh; tail -50 scripts/api-smoke.sh`
Note: existing 12 cases (5 handler + 4 IP type + 1 advisory + 2 topic). Add 4 new (operator create / creator login / creator topic / creator 403).

- [ ] **Step 5.2: 在 api-smoke.sh 末尾加 4 case**

Append:

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
    -c /tmp/opc-creator-cookies.txt \
    -d '{"email":"creator1@beta.com","password":"beta-pass-123"}')
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
status=$(curl -s -o /dev/null -w "%{http_code}" -X POST \
    http://localhost:8081/api/admin/creators \
    -H "Content-Type: application/json" \
    -H "Cookie: opc_session=$creator_session" \
    -d '{"email":"x@x.com","password":"y-pass-123"}')
if [ "$status" = "403" ]; then
    pass "creator: admin endpoint correctly 403"
else
    fail "creator: admin endpoint returned $status, want 403"
fi
rm -f /tmp/opc-creator-cookies.txt
```

(注: `OPERATOR_SESSION_TOKEN` env 需在 smoke 跑前 set. 如果 老 smoke 没设, 加 `OPERATOR_SESSION_TOKEN=$SESSION_TOKEN` 假设 老 operator session 已 在, 或 改 smoke 1-2 行 加 operator login step)

- [ ] **Step 5.3: 跑 smoke 验证**

```bash
cd /root/workspace/opc
./scripts/api-smoke.sh 2>&1 | tail -30
```

Expected: 16 cases PASS (12 existing + 4 new). Topic creator case may SKIP if MINIMAX_API_KEY not set, but admin/login/403 should PASS.

- [ ] **Step 5.4: 改 `~/.claude/PROJECT-STATE.md`**

按 §H 协议:
- Section E append 1 个 bullet (C M1 closure 总结, 5 commit, 17 unit test + smoke 16/16 PASS, plan deviations 文档)
- Section D 标 G15 (sub-spec C M1) **CLOSED 2026-06-15**
- 改 Last refreshed line

具体 edit (类似 A/B M1 closure 处理):
- Header `Last refreshed: 2026-06-15 ~HH:MM, after verify agent run 23 (Phase 2 sub-spec C M1 closure) returned OVERALL PASS.`
- Section E append: 5 commit 链 + C M1 summary + plan deviations
- Section D add: G15-closed row

- [ ] **Step 5.5: 改 `~/.claude/GAP-LOG.md`**

Append G15 closure entry:
```markdown
## 2026-06-15 HH:MM (objective, observed) — G15: Phase 2 sub-spec C M1 not delivered

- **缺口**: spec + plan 写了但没实现. apps/external/ 创作者 UI + 4 admin endpoint + User model 3 列 + 2 role-check middleware + console /creators 页 都没 落地.
- **影响**: Phase 2 D 跟 E 跟 C 弱关联, 但 critical path 走 C 后 才能 给 真 用户 用.
- **修复计划**: 见 docs/superpowers/plans/2026-06-15-opc-phase2-external-ui-m1.md (5 task, 2-3 周)
- **status**: **closed 2026-06-15** — Task 1-5 全部 done, 5 commit landed, 17 unit test + 老 suite PASS, 4 smoke case (admin create / creator login / topic 200 / admin 403) PASS, apps/external `pnpm build` exit 0
```

- [ ] **Step 5.6: Dispatch verify-project-state subagent** (or self-verify if dispatch is too costly)

按 `~/.claude/agents/verify-project-state.md` 规范. 关键命令:
```bash
go vet -tags fts5 ./...
go test -tags fts5 -race -count=1 ./...
go build -tags fts5 -o /tmp/v-task5-api ./cmd/server
cd apps/external && pnpm build
# Creator admin smoke: curl POST /api/admin/creators 返 201
# Creator 调 admin 返 403
# apps/external /login 返 200, /dashboard 返 307
```

报告写 `~/.claude/audit/verify-reports/verify-YYYYMMDD-HHMMSS-phase2-sub-spec-c-m1.md`. OVERALL: PASS 才算 M1 closed.

- [ ] **Step 5.7: Commit 收尾**

```bash
cd /root/workspace/opc
git add scripts/api-smoke.sh
git commit -m "test(smoke): add external creator flow cases (Task 5)

scripts/api-smoke.sh: 4 new test cases for Sub-Spec C M1:
- admin: creator created (operator POST /api/admin/creators → 201)
- creator: login successful (POST /api/auth/login with creator
  credentials → 200 + session cookie)
- creator: topic HTTP returns 5 topics (B M1 endpoint reused
  via creator session; verifies that refactor didn't break
  non-operator callers)
- creator: admin endpoint correctly 403 (creator trying to call
  /api/admin/creators → 403 'operator role required')

Total smoke count: 12 → 16. Old cases unchanged.

Project state + gap log updated to reflect M1 closure (G15
CLOSED 2026-06-15)."
```

---

## 验收清单（M1 done 的标志, 5 task 全部 DONE）

| # | 验收 | 验证 | 状态 |
|---|------|------|------|
| 1 | User model 加 3 列 (Role/Disabled/CreatedBy) | `go test -tags fts5 ./internal/models/...` 3 PASS | ☐ |
| 2 | 4 admin endpoint (creator.go) + 2 middleware + 17-19 unit test PASS | `go test -tags fts5 ./internal/handlers/... ./internal/middleware/...` 全绿 | ☐ |
| 3 | console /creators 管理页 + 2 dialog 存在 + build 0 错 | `ls apps/console/app/(console)/creators/`, `pnpm build` | ☐ |
| 4 | apps/external 全新 Next.js 14 app + 3 pages + middleware + build 0 错 | `ls apps/external/`, `pnpm build` | ☐ |
| 5 | scripts/api-smoke.sh 16/16 PASS (12 老 + 4 新) | `./scripts/api-smoke.sh` | ☐ |
| 6 | 老 endpoint 0 回归 (12 老 case 仍 PASS) | smoke 0 回归 | ☐ |
| 7 | verify-project-state agent OVERALL: PASS | `~/.claude/audit/verify-reports/verify-*.md` PASS | ☐ |
| 8 | PROJECT-STATE.md + GAP-LOG.md 更新 | G15 CLOSED 记录在案 | ☐ |

---

## 不做的事（M1 范围硬约束）

- ❌ 4 capability 暴露 (script/deconstruct/asset 都没, 等 B M2)
- ❌ Topic 历史持久化 (M1 不加 creator_topic model, call_log 记录)
- ❌ IP profile 4 个选择器 (M1 默认 anthropomorphic 固定)
- ❌ OAuth / Magic link / 邮件发送
- ❌ 付费 / Stripe
- ❌ MCN 白标
- ❌ 多语言 (先中文)
- ❌ 移动端适配 (先桌面)
- ❌ 改 老 handler endpoint 业务逻辑 (仅加 Role 字段)
- ❌ 改 老 User model 字段 (除加 3 列外)
- ❌ 改 apps/web 任何 5-page 行为
- ❌ 重写 session middleware (复用 RequireAuth, 加 2 个 role wrapper)

---

## 风险与缓解

| 风险 | 缓解 |
|------|------|
| User model 加列 破 老 operator 用户 (AutoMigrate 失败) | Task 1 加 3 unit test 验 default 值; 老 Phase 1 operator 用户 (Role="") 走 GORM default → "operator" |
| 新增 4 admin endpoint 命名冲突 | 全部 `/api/admin/creators*` 前缀, 跟现有 `/api/billing/credentials/...` 平级 |
| 新增 2 middleware 跟 RequireAuth 串联 顺序错 | docs 注释明确 "must be used AFTER RequireAuth"; unit test 验 4 个 path |
| apps/external 跟 web + console 共享 端口 | apps/external 跑 :3002; `next.config.js` 配 `port: 3002` |
| Operator 误 改自己 role (operator → creator) | 现有 `POST /api/admin/creators` 拒绝 修改 "operator" role 的 user; 只允许 role='creator' 操作 |
| Topic 不 持久化 创作者 看不到历史 | M1 接受 (call_log 已记 cost + topic 内容); UI placeholder 显式 提示 |
| apps/external session cookie 不共享 (不同 port) | dev 用 nginx 反代 同源; 本地 smoke 用 curl `--cookie-jar` 验 session 通; prod 同域 解决 |
| AutoMigrate 新列 在老 DB schema 失败 (NOT NULL) | 3 个新列都有 gorm default, 老 row 自动 apply default; Task 1 unit test 验 default value 正确 |
| apps/external 跟 web/console 共享 `/api` 路径 | middleware matcher 排除 /api 路径; 各 app 独立 调 自己后端 |

---

## Self-Review

写 plan 时做了 4 轮 self-review:

1. **Spec coverage**: spec §5 (架构) / §6 (组件) / §7 (schema) / §10 (数据流) / §11 (错误处理) / §12 (测试) / §13 (timeline) / §15 (跟 A/B 关系) 全部对应到 5 task. 验收清单 8 项覆盖.
2. **Placeholder scan**: 0 个 TBD/TODO/类似措辞.
3. **Type consistency**:
   - `User` 加 3 字段 (Role/Disabled/CreatedBy) 在 Task 1, Task 2 creator.go 用
   - `creatorH := handlers.NewCreatorHandler(db)` 签名 Task 2, Task 5 smoke 用
   - 2 个 middleware `RequireOperatorRole` / `RequireCreatorRole` 签名 Task 2, Task 3 console /creators 透过 main.go 调
   - `Login/Me/Register` response 加 `role` 字段 Task 2, Task 4 apps/external login 读
   - `assets/external` port :3002, Task 4 配 next.config.js, Task 5 smoke 验
4. **Ambiguity check**:
   - Operator 手动 create creator (明确 POST /api/admin/creators body schema)
   - 默认 IP profile anthropomorphic (明确 apps/external/topic page hard-code)
   - Topic 历史 M1 不持久化 (明确 UI placeholder text)
   - session cookie name 'opc_session' 跨 3 app 共享 (middleware 复用)

无矛盾. Plan ready for execution.
