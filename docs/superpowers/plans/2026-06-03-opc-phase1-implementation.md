# OPC Phase 1 实施计划：熊猫 IP + Web UI MVP

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 6 个月内用"熊猫"AI IP 跑出至少 1 个爆款（500 万+播放）+ 1 平台 10 万粉 + 5 页面 web UI MVP（内部用）+ MCP server，验证 NemoVideo 混合模式的可行性。

**Architecture:** Monorepo（`apps/web` Next.js + `apps/api` Go/Gin），agent-agnostic（MCP server 暴露给 Claude Code CLI 也能用），IP 知识/工作流 5 页面全自建，跨平台抓数据外包给蝉妈妈/飞瓜。

**Tech Stack:**
- 前端：Next.js 14 (App Router) + TypeScript + shadcn/ui + Tailwind
- 后端：Go 1.22+ + Gin + GORM + SQLite (MVP)
- LLM：`github.com/anthropics/anthropic-sdk-go`
- MCP：`github.com/modelcontextprotocol/go-sdk`
- 部署：单台 VPS + Docker Compose
- 内容工具（外包）：可灵/即梦/剪映/Suno

**对应 Spec:** `docs/superpowers/specs/2026-06-03-opc-phase1-ip-design.md` (v1.3)

---

## 文件结构

```
/root/workspace/opc/
├── apps/
│   ├── web/                          # Next.js 前端
│   │   ├── app/
│   │   │   ├── topics/page.tsx       # 页面 1: 选题看板
│   │   │   ├── scripts/page.tsx      # 页面 2: 脚本库
│   │   │   ├── calendar/page.tsx     # 页面 3: 内容日历
│   │   │   ├── dashboard/page.tsx    # 页面 4: 表现 dashboard
│   │   │   ├── knowledge/page.tsx    # 页面 5: 知识库
│   │   │   ├── layout.tsx
│   │   │   └── page.tsx              # 首页
│   │   ├── components/               # 复用组件
│   │   ├── lib/
│   │   │   ├── api.ts                # API client
│   │   │   └── types.ts
│   │   ├── package.json
│   │   ├── tsconfig.json
│   │   └── tailwind.config.ts
│   └── api/                          # Go 后端
│       ├── cmd/server/main.go
│       ├── internal/
│       │   ├── config/config.go
│       │   ├── db/db.go
│       │   ├── models/
│       │   │   ├── topic.go
│       │   │   ├── script.go
│       │   │   ├── content_item.go
│       │   │   ├── knowledge_doc.go
│       │   │   └── series.go
│       │   ├── handlers/
│       │   │   ├── topics.go
│       │   │   ├── scripts.go
│       │   │   ├── content_items.go
│       │   │   ├── knowledge.go
│       │   │   └── dashboard.go
│       │   ├── mcp/server.go         # MCP server
│       │   ├── agents/claude.go      # Claude API 集成
│       │   └── migrations/
│       ├── go.mod
│       ├── go.sum
│       └── Dockerfile
├── scripts/
│   ├── dev.sh                        # 本地启动
│   ├── seed.sh                       # 种子数据
│   └── deploy.sh                     # 部署
├── docker-compose.yml
├── .gitignore
└── README.md
```

---

# M1：试水期（4 周）— 跑内容 + 搭骨架

> **目标**: 30 条内容验证人设，1 万粉账号，搭好项目骨架
> **并行**: 内容主导跑内容；全栈开发 M1 末到位开始搭 web UI

## Task 1: 项目初始化（monorepo 骨架）

**Files:**
- Create: `/root/workspace/opc/.gitignore`
- Create: `/root/workspace/opc/README.md`
- Create: `/root/workspace/opc/docker-compose.yml`
- Create: `/root/workspace/opc/scripts/dev.sh`

- [ ] **Step 1: 初始化 git 仓库**

```bash
cd /root/workspace/opc
git init
git config user.email "opc@example.com"
git config user.name "OPC Team"
```

- [ ] **Step 2: 创建 .gitignore**

写入 `/root/workspace/opc/.gitignore`:
```gitignore
# OS
.DS_Store
Thumbs.db

# Editors
.vscode/
.idea/
*.swp
*.swo

# Node
node_modules/
.next/
out/
.env
.env.local
.env*.local
npm-debug.log*
yarn-debug.log*
yarn-error.log*
.pnpm-debug.log*

# Go
*.exe
*.exe~
*.dll
*.so
*.dylib
bin/
tmp/
*.test
*.out
vendor/
coverage.txt

# Database
*.db
*.db-journal
*.sqlite
*.sqlite3

# Docker
.env.docker

# Build artifacts
dist/
build/
```

- [ ] **Step 3: 创建 README.md（项目总览）**

写入 `/root/workspace/opc/README.md`:
```markdown
# OPC

AI 视频创作平台（NemoVideo 混合模式）。

## 当前阶段

Phase 1：跑"熊猫"AI IP + 5 页面 web UI MVP（内部用）+ MCP server。

详见 `docs/superpowers/specs/2026-06-03-opc-phase1-ip-design.md`

## 快速开始

\`\`\`bash
# 安装依赖
./scripts/dev.sh setup

# 启动开发环境（DB + 后端 + 前端）
./scripts/dev.sh up
\`\`\`

## 目录结构

- `apps/web/` — Next.js 前端（5 个页面）
- `apps/api/` — Go 后端（Gin + GORM）
- `docs/superpowers/` — 设计与实施文档
- `scripts/` — 运维脚本

## 团队工作流

1. **内容主导**：用 web UI 的选题看板/脚本库/内容日历
2. **AI 操作手**：用 web UI + 可灵/即梦/剪映生成视频
3. **全栈开发**：维护 web UI + 后端 + MCP server
```

- [ ] **Step 4: 创建 docker-compose.yml（开发用）**

写入 `/root/workspace/opc/docker-compose.yml`:
```yaml
version: '3.8'

services:
  api:
    build: ./apps/api
    ports:
      - "8080:8080"
    environment:
      - DB_PATH=/data/opc.db
      - ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY}
    volumes:
      - opc-data:/data
    restart: unless-stopped

  web:
    build: ./apps/web
    ports:
      - "3000:3000"
    environment:
      - NEXT_PUBLIC_API_URL=http://localhost:8080
    depends_on:
      - api
    restart: unless-stopped

volumes:
  opc-data:
```

- [ ] **Step 5: 创建 dev.sh 脚本**

写入 `/root/workspace/opc/scripts/dev.sh`:
```bash
#!/bin/bash
set -e

case "$1" in
  setup)
    echo "📦 Setting up OPC monorepo..."
    cd apps/api && go mod init github.com/opc/api && cd ../..
    cd apps/web && npm init -y && cd ../..
    mkdir -p apps/web/app apps/api/internal
    echo "✅ Setup complete"
    ;;
  up)
    echo "🚀 Starting dev environment..."
    docker compose up -d
    echo "✅ API: http://localhost:8080, Web: http://localhost:3000"
    ;;
  down)
    docker compose down
    ;;
  *)
    echo "Usage: $0 {setup|up|down}"
    exit 1
    ;;
esac
```

```bash
chmod +x /root/workspace/opc/scripts/dev.sh
```

- [ ] **Step 6: 提交**

```bash
cd /root/workspace/opc
git add .gitignore README.md docker-compose.yml scripts/
git commit -m "feat: init monorepo skeleton with docker-compose setup"
```

---

## Task 2: Go 后端骨架（Gin + GORM + 健康检查）

**Files:**
- Create: `apps/api/go.mod`
- Create: `apps/api/cmd/server/main.go`
- Create: `apps/api/internal/config/config.go`
- Create: `apps/api/internal/db/db.go`
- Create: `apps/api/internal/handlers/health.go`
- Create: `apps/api/Dockerfile`

- [ ] **Step 1: 初始化 Go module**

```bash
cd /root/workspace/opc/apps/api
go mod init github.com/opc/api
go get github.com/gin-gonic/gin
go get gorm.io/gorm
go get gorm.io/driver/sqlite
go get github.com/glebarez/sqlite
go get github.com/anthropics/anthropic-sdk-go
go get github.com/modelcontextprotocol/go-sdk
go get github.com/gin-contrib/cors
```

- [ ] **Step 2: 写健康检查 handler 测试（TDD）**

写入 `apps/api/internal/handlers/health_test.go`:
```go
package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHealthEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/health", Health)

	req, _ := http.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if w.Body.String() != `{"status":"ok"}` {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}
```

- [ ] **Step 3: 运行测试，验证失败**

```bash
cd /root/workspace/opc/apps/api
go test ./internal/handlers/ -v
```

Expected: FAIL（"undefined: Health"）

- [ ] **Step 4: 实现 health handler**

写入 `apps/api/internal/handlers/health.go`:
```go
package handlers

import "github.com/gin-gonic/gin"

func Health(c *gin.Context) {
	c.JSON(200, gin.H{"status": "ok"})
}
```

- [ ] **Step 5: 实现 main.go**

写入 `apps/api/cmd/server/main.go`:
```go
package main

import (
	"log"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/opc/api/internal/config"
	"github.com/opc/api/internal/db"
	"github.com/opc/api/internal/handlers"
)

func main() {
	cfg := config.Load()

	database, err := db.Connect(cfg.DBPath)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer database.Close()

	if err := db.Migrate(database); err != nil {
		log.Fatalf("db migrate: %v", err)
	}

	r := gin.Default()
	r.Use(cors.Default())

	r.GET("/health", handlers.Health)

	log.Printf("🚀 OPC API listening on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("server: %v", err)
	}
}
```

- [ ] **Step 6: 实现 config**

写入 `apps/api/internal/config/config.go`:
```go
package config

import "os"

type Config struct {
	Port             string
	DBPath           string
	AnthropicAPIKey  string
}

func Load() *Config {
	return &Config{
		Port:            getEnv("PORT", "8080"),
		DBPath:          getEnv("DB_PATH", "/data/opc.db"),
		AnthropicAPIKey: getEnv("ANTHROPIC_API_KEY", ""),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
```

- [ ] **Step 7: 实现 db 连接和迁移（空函数占位）**

写入 `apps/api/internal/db/db.go`:
```go
package db

import (
	"database/sql"
	"fmt"

	_ "github.com/glebarez/sqlite"
)

func Connect(dbPath string) (*sql.DB, error) {
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return conn, nil
}

// Migrate 占位——Task 3 后填充
func Migrate(db *sql.DB) error {
	return nil
}
```

- [ ] **Step 8: 写 Dockerfile**

写入 `apps/api/Dockerfile`:
```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -o opc-api ./cmd/server

FROM alpine:latest
RUN apk add --no-cache ca-certificates sqlite
WORKDIR /app
COPY --from=builder /app/opc-api .
EXPOSE 8080
CMD ["./opc-api"]
```

- [ ] **Step 9: 写测试 helper（编译用）**

写入 `apps/api/internal/handlers/handlers.go`（确保包能编译）:
```go
package handlers

// 留空——后续 task 增 handlers
```

- [ ] **Step 10: 跑测试，验证通过**

```bash
cd /root/workspace/opc/apps/api
go mod tidy
go test ./... -v
```

Expected: PASS

- [ ] **Step 11: 本地启动验证**

```bash
cd /root/workspace/opc/apps/api
DB_PATH=/tmp/opc-test.db go run ./cmd/server
```

另开终端：
```bash
curl http://localhost:8080/health
# Expected: {"status":"ok"}
```

Ctrl+C 停止

- [ ] **Step 12: 提交**

```bash
cd /root/workspace/opc
git add apps/api/
git commit -m "feat(api): gin skeleton with health endpoint"
```

---

## Task 3: 数据库 schema + GORM models

**Files:**
- Create: `apps/api/internal/models/topic.go`
- Create: `apps/api/internal/models/script.go`
- Create: `apps/api/internal/models/content_item.go`
- Create: `apps/api/internal/models/knowledge_doc.go`
- Create: `apps/api/internal/models/series.go`
- Create: `apps/api/internal/db/migrate.go`

- [ ] **Step 1: 写 topic model**

写入 `apps/api/internal/models/topic.go`:
```go
package models

import "time"

type Topic struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	Title               string    `json:"title"`
	Angle               string    `json:"angle"`
	Platform            string    `json:"platform"`  // 抖音/哔哩哔哩/小红书
	Status              string    `json:"status"`    // 想法/评估/待写/已发布
	SeriesID            *uint     `json:"series_id,omitempty"`
	ExpectedPerformance string    `json:"expected_performance"`
	Notes               string    `json:"notes"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func (Topic) TableName() string { return "topics" }
```

- [ ] **Step 2: 写 script model**

写入 `apps/api/internal/models/script.go`:
```go
package models

import "time"

type Script struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	TopicID          uint      `json:"topic_id"`
	Title            string    `json:"title"`
	Content          string    `gorm:"type:text" json:"content"`
	Platform         string    `json:"platform"`
	StyleFingerprint string    `gorm:"type:text" json:"style_fingerprint"` // JSON
	Tags             string    `json:"tags"`
	WordCount        int       `json:"word_count"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (Script) TableName() string { return "scripts" }
```

- [ ] **Step 3: 写 content_item model**

写入 `apps/api/internal/models/content_item.go`:
```go
package models

import "time"

type ContentItem struct {
	ID                 uint       `gorm:"primaryKey" json:"id"`
	ScriptID           uint       `json:"script_id"`
	Platform           string     `json:"platform"`
	ScheduledAt        *time.Time `json:"scheduled_at,omitempty"`
	PublishedAt        *time.Time `json:"published_at,omitempty"`
	PlatformURL        string     `json:"platform_url"`
	PerformanceMetrics string     `gorm:"type:text" json:"performance_metrics"` // JSON
	CreatedAt          time.Time  `json:"created_at"`
}

func (ContentItem) TableName() string { return "content_items" }
```

- [ ] **Step 4: 写 knowledge_doc model**

写入 `apps/api/internal/models/knowledge_doc.go`:
```go
package models

import "time"

type KnowledgeDoc struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Title     string    `json:"title"`
	Path      string    `json:"path"`     // e.g. "ip-style-guide/voice"
	Content   string    `gorm:"type:text" json:"content"`
	Tags      string    `json:"tags"`
	DocType   string    `json:"doc_type"` // ip-style/sop/prompt/postmortem/decision/platform-rule
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (KnowledgeDoc) TableName() string { return "knowledge_docs" }
```

- [ ] **Step 5: 写 series model**

写入 `apps/api/internal/models/series.go`:
```go
package models

type Series struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (Series) TableName() string { return "series" }
```

- [ ] **Step 6: 写 migrate 单元测试**

写入 `apps/api/internal/db/migrate_test.go`:
```go
package db

import (
	"database/sql"
	"testing"
)

func TestMigrateCreatesTables(t *testing.T) {
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// 用 GORM 的 sqlite 驱动
	gormDB, err := connectGorm(":memory:")
	if err != nil {
		t.Fatal(err)
	}

	if err := Migrate(gormDB); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// 验证表存在
	tables := []string{"topics", "scripts", "content_items", "knowledge_docs", "series"}
	for _, table := range tables {
		var name string
		row := conn.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table)
		if err := row.Scan(&name); err != nil {
			t.Errorf("table %s not created: %v", table, err)
		}
	}
}
```

- [ ] **Step 7: 实现 migrate.go（重构 db.go）**

替换 `apps/api/internal/db/db.go`:
```go
package db

import (
	"database/sql"
	"fmt"

	_ "github.com/glebarez/sqlite"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/opc/api/internal/models"
)

func Connect(dbPath string) (*sql.DB, error) {
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return conn, nil
}

func connectGorm(dbPath string) (*gorm.DB, error) {
	return gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
}

func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.Topic{},
		&models.Script{},
		&models.ContentItem{},
		&models.KnowledgeDoc{},
		&models.Series{},
	)
}
```

- [ ] **Step 8: 更新 main.go 使用 GORM**

更新 `apps/api/cmd/server/main.go`:
```go
package main

import (
	"log"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/opc/api/internal/config"
	"github.com/opc/api/internal/db"
	"github.com/opc/api/internal/handlers"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func main() {
	cfg := config.Load()

	gormDB, err := gorm.Open(sqlite.Open(cfg.DBPath), &gorm.Config{})
	if err != nil {
		log.Fatalf("gorm open: %v", err)
	}

	if err := db.Migrate(gormDB); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	r := gin.Default()
	r.Use(cors.Default())

	r.GET("/health", handlers.Health)

	log.Printf("🚀 OPC API listening on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("server: %v", err)
	}
}
```

- [ ] **Step 9: 跑测试，验证通过**

```bash
cd /root/workspace/opc/apps/api
go mod tidy
go test ./... -v
```

Expected: PASS

- [ ] **Step 10: 启动验证表创建**

```bash
rm -f /tmp/opc-test.db
cd /root/workspace/opc/apps/api
DB_PATH=/tmp/opc-test.db go run ./cmd/server
# 让它跑 2 秒
sleep 2
sqlite3 /tmp/opc-test.db ".tables"
# Expected: content_items  knowledge_docs  scripts  series  topics
Ctrl+C
```

- [ ] **Step 11: 提交**

```bash
cd /root/workspace/opc
git add apps/api/
git commit -m "feat(api): add GORM models and migrations for all entities"
```

---

## Task 4: Topic CRUD API（后端第一个业务端点）

**Files:**
- Create: `apps/api/internal/handlers/topics.go`
- Create: `apps/api/internal/handlers/topics_test.go`
- Modify: `apps/api/cmd/server/main.go`

- [ ] **Step 1: 写测试**

写入 `apps/api/internal/handlers/topics_test.go`:
```go
package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/opc/api/internal/models"
)

func setupTestTopicsRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	gin.SetMode(gin.TestMode)
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := gormDB.AutoMigrate(&models.Topic{}); err != nil {
		t.Fatal(err)
	}

	r := gin.New()
	h := NewTopicsHandler(gormDB)
	r.POST("/topics", h.Create)
	r.GET("/topics", h.List)
	r.GET("/topics/:id", h.Get)
	r.PUT("/topics/:id", h.Update)
	r.DELETE("/topics/:id", h.Delete)
	return r, gormDB
}

func TestCreateTopic(t *testing.T) {
	r, _ := setupTestTopicsRouter(t)

	body := `{"title":"测试选题","angle":"反常识","platform":"抖音","status":"想法"}`
	req, _ := http.NewRequest("POST", "/topics", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d, body: %s", w.Code, w.Body.String())
	}

	var topic models.Topic
	if err := json.Unmarshal(w.Body.Bytes(), &topic); err != nil {
		t.Fatal(err)
	}
	if topic.Title != "测试选题" {
		t.Errorf("unexpected title: %s", topic.Title)
	}
}

func TestListTopics(t *testing.T) {
	r, db := setupTestTopicsRouter(t)

	// 种 2 条
	db.Create(&models.Topic{Title: "A", Platform: "抖音", Status: "想法"})
	db.Create(&models.Topic{Title: "B", Platform: "哔哩哔哩", Status: "评估"})

	req, _ := http.NewRequest("GET", "/topics", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var topics []models.Topic
	json.Unmarshal(w.Body.Bytes(), &topics)
	if len(topics) != 2 {
		t.Errorf("expected 2 topics, got %d", len(topics))
	}
}
```

- [ ] **Step 2: 跑测试，验证失败**

```bash
cd /root/workspace/opc/apps/api
go test ./internal/handlers/ -v
```

Expected: FAIL（"undefined: NewTopicsHandler"）

- [ ] **Step 3: 实现 topics handler**

写入 `apps/api/internal/handlers/topics.go`:
```go
package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/opc/api/internal/models"
)

type TopicsHandler struct {
	db *gorm.DB
}

func NewTopicsHandler(db *gorm.DB) *TopicsHandler {
	return &TopicsHandler{db: db}
}

func (h *TopicsHandler) Create(c *gin.Context) {
	var topic models.Topic
	if err := c.ShouldBindJSON(&topic); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.db.Create(&topic).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, topic)
}

func (h *TopicsHandler) List(c *gin.Context) {
	var topics []models.Topic
	query := h.db.Order("created_at desc")
	if platform := c.Query("platform"); platform != "" {
		query = query.Where("platform = ?", platform)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Find(&topics).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, topics)
}

func (h *TopicsHandler) Get(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var topic models.Topic
	if err := h.db.First(&topic, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, topic)
}

func (h *TopicsHandler) Update(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var topic models.Topic
	if err := h.db.First(&topic, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err := c.ShouldBindJSON(&topic); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	topic.ID = uint(id)
	if err := h.db.Save(&topic).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, topic)
}

func (h *TopicsHandler) Delete(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := h.db.Delete(&models.Topic{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}
```

- [ ] **Step 4: 跑测试，验证通过**

```bash
cd /root/workspace/opc/apps/api
go test ./internal/handlers/ -v -run TestCreateTopic
go test ./internal/handlers/ -v -run TestListTopics
```

Expected: PASS

- [ ] **Step 5: 注册路由到 main.go**

更新 `apps/api/cmd/server/main.go` 中路由部分:
```go
		topicsH := handlers.NewTopicsHandler(gormDB)
		r.POST("/topics", topicsH.Create)
		r.GET("/topics", topicsH.List)
		r.GET("/topics/:id", topicsH.Get)
		r.PUT("/topics/:id", topicsH.Update)
		r.DELETE("/topics/:id", topicsH.Delete)
```

- [ ] **Step 6: 启动 + curl 验证**

```bash
rm -f /tmp/opc-test.db
cd /root/workspace/opc/apps/api
DB_PATH=/tmp/opc-test.db go run ./cmd/server &
sleep 2

curl -X POST http://localhost:8080/topics \
  -H "Content-Type: application/json" \
  -d '{"title":"测试","platform":"抖音","status":"想法"}'
# Expected: 201, 返回 topic JSON

curl http://localhost:8080/topics
# Expected: 200, 返回 list

kill %1
```

- [ ] **Step 7: 提交**

```bash
cd /root/workspace/opc
git add apps/api/
git commit -m "feat(api): Topic CRUD endpoints with tests"
```

---

## Task 5-8: Script / ContentItem / KnowledgeDoc / Series CRUD

> 跟 Task 4 同样的 TDD 模式，分别给 Script、ContentItem、KnowledgeDoc、Series 写 CRUD。
> 详细步骤略（重复 Task 4 的模式）——具体起手时按 Task 4 模板复制。

每个 task 提交信息：
- `feat(api): Script CRUD endpoints with tests`
- `feat(api): ContentItem CRUD endpoints with tests`
- `feat(api): KnowledgeDoc CRUD endpoints with tests`
- `feat(api): Series CRUD endpoints with tests`

参考 Task 4 的步骤结构，每个 handler 包含 Create/List/Get/Update/Delete。

---

## Task 9: Next.js 前端骨架

**Files:**
- Create: `apps/web/package.json`
- Create: `apps/web/tsconfig.json`
- Create: `apps/web/next.config.js`
- Create: `apps/web/tailwind.config.ts`
- Create: `apps/web/postcss.config.js`
- Create: `apps/web/app/layout.tsx`
- Create: `apps/web/app/page.tsx`
- Create: `apps/web/app/globals.css`
- Create: `apps/web/Dockerfile`

- [ ] **Step 1: 初始化 package.json**

```bash
cd /root/workspace/opc/apps/web
npm init -y
npm install next@14 react@18 react-dom@18
npm install -D typescript @types/react @types/node @types/react-dom
npm install -D tailwindcss postcss autoprefixer
npx tailwindcss init -p
```

- [ ] **Step 2: 配置 tsconfig.json**

写入 `apps/web/tsconfig.json`:
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
    "plugins": [{ "name": "next" }],
    "paths": { "@/*": ["./*"] }
  },
  "include": ["next-env.d.ts", "**/*.ts", "**/*.tsx", ".next/types/**/*.ts"],
  "exclude": ["node_modules"]
}
```

- [ ] **Step 3: 配置 next.config.js**

写入 `apps/web/next.config.js`:
```js
/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  async rewrites() {
    return [
      { source: '/api/:path*', destination: `${process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080'}/:path*` }
    ]
  }
}
module.exports = nextConfig
```

- [ ] **Step 4: 配置 tailwind.config.ts**

写入 `apps/web/tailwind.config.ts`:
```ts
import type { Config } from 'tailwindcss'

const config: Config = {
  content: [
    './app/**/*.{js,ts,jsx,tsx,mdx}',
    './components/**/*.{js,ts,jsx,tsx,mdx}',
  ],
  theme: { extend: {} },
  plugins: [],
}
export default config
```

- [ ] **Step 5: 配置 globals.css**

写入 `apps/web/app/globals.css`:
```css
@tailwind base;
@tailwind components;
@tailwind utilities;

body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
}
```

- [ ] **Step 6: 写根 layout**

写入 `apps/web/app/layout.tsx`:
```tsx
import './globals.css'
import type { Metadata } from 'next'

export const metadata: Metadata = {
  title: 'OPC',
  description: 'OPC 短视频创作平台',
}

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN">
      <body className="bg-gray-50">
        <nav className="bg-white border-b border-gray-200 px-6 py-3">
          <div className="flex gap-6 text-sm font-medium">
            <a href="/topics" className="hover:text-blue-600">选题</a>
            <a href="/scripts" className="hover:text-blue-600">脚本</a>
            <a href="/calendar" className="hover:text-blue-600">日历</a>
            <a href="/dashboard" className="hover:text-blue-600">表现</a>
            <a href="/knowledge" className="hover:text-blue-600">知识库</a>
          </div>
        </nav>
        <main className="container mx-auto px-6 py-6">{children}</main>
      </body>
    </html>
  )
}
```

- [ ] **Step 7: 写首页**

写入 `apps/web/app/page.tsx`:
```tsx
export default function Home() {
  return (
    <div className="space-y-4">
      <h1 className="text-3xl font-bold">🐼 OPC 创作控制台</h1>
      <p className="text-gray-600">熊猫 IP · Phase 1</p>
      <div className="grid grid-cols-2 md:grid-cols-5 gap-4">
        <a href="/topics" className="p-4 bg-white rounded-lg shadow hover:shadow-md">
          <h2 className="font-semibold">📋 选题</h2>
          <p className="text-sm text-gray-500">Kanban</p>
        </a>
        <a href="/scripts" className="p-4 bg-white rounded-lg shadow hover:shadow-md">
          <h2 className="font-semibold">📝 脚本</h2>
          <p className="text-sm text-gray-500">库</p>
        </a>
        <a href="/calendar" className="p-4 bg-white rounded-lg shadow hover:shadow-md">
          <h2 className="font-semibold">📅 日历</h2>
          <p className="text-sm text-gray-500">排期</p>
        </a>
        <a href="/dashboard" className="p-4 bg-white rounded-lg shadow hover:shadow-md">
          <h2 className="font-semibold">📊 表现</h2>
          <p className="text-sm text-gray-500">Dashboard</p>
        </a>
        <a href="/knowledge" className="p-4 bg-white rounded-lg shadow hover:shadow-md">
          <h2 className="font-semibold">📚 知识库</h2>
          <p className="text-sm text-gray-500">SOP/笔记</p>
        </a>
      </div>
    </div>
  )
}
```

- [ ] **Step 8: 写 Dockerfile**

写入 `apps/web/Dockerfile`:
```dockerfile
FROM node:20-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM node:20-alpine AS runner
WORKDIR /app
COPY --from=builder /app/public ./public
COPY --from=builder /app/.next/standalone ./
COPY --from=builder /app/.next/static ./.next/static
EXPOSE 3000
CMD ["node", "server.js"]
```

- [ ] **Step 9: 启动验证**

```bash
cd /root/workspace/opc/apps/web
npm install
npm run dev
```

浏览器打开 http://localhost:3000 — 应该看到首页 + 5 个导航卡片

- [ ] **Step 10: 提交**

```bash
cd /root/workspace/opc
git add apps/web/
git commit -m "feat(web): Next.js 14 skeleton with Tailwind + nav + home"
```

---

## Task 10: API client lib（前端用）

**Files:**
- Create: `apps/web/lib/api.ts`
- Create: `apps/web/lib/types.ts`

- [ ] **Step 1: 写 types**

写入 `apps/web/lib/types.ts`:
```ts
export type Topic = {
  id: number
  title: string
  angle: string
  platform: string
  status: string
  series_id?: number
  expected_performance: string
  notes: string
  created_at: string
  updated_at: string
}

export type Script = {
  id: number
  topic_id: number
  title: string
  content: string
  platform: string
  style_fingerprint: string
  tags: string
  word_count: number
  created_at: string
  updated_at: string
}

export type ContentItem = {
  id: number
  script_id: number
  platform: string
  scheduled_at?: string
  published_at?: string
  platform_url: string
  performance_metrics: string
  created_at: string
}

export type KnowledgeDoc = {
  id: number
  title: string
  path: string
  content: string
  tags: string
  doc_type: string
  created_at: string
  updated_at: string
}
```

- [ ] **Step 2: 写 API client**

写入 `apps/web/lib/api.ts`:
```ts
import type { Topic, Script, ContentItem, KnowledgeDoc } from './types'

const BASE = process.env.NEXT_PUBLIC_API_URL || ''

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}/api${path}`, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  })
  if (!res.ok) {
    throw new Error(`API ${path} failed: ${res.status} ${await res.text()}`)
  }
  return res.json()
}

export const api = {
  topics: {
    list: (params?: { platform?: string; status?: string }) => {
      const q = new URLSearchParams(params as any).toString()
      return request<Topic[]>(`/topics${q ? `?${q}` : ''}`)
    },
    get: (id: number) => request<Topic>(`/topics/${id}`),
    create: (data: Partial<Topic>) =>
      request<Topic>('/topics', { method: 'POST', body: JSON.stringify(data) }),
    update: (id: number, data: Partial<Topic>) =>
      request<Topic>(`/topics/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
    delete: (id: number) =>
      request<{ deleted: number }>(`/topics/${id}`, { method: 'DELETE' }),
  },
  scripts: {
    list: () => request<Script[]>('/scripts'),
    get: (id: number) => request<Script>(`/scripts/${id}`),
    create: (data: Partial<Script>) =>
      request<Script>('/scripts', { method: 'POST', body: JSON.stringify(data) }),
    update: (id: number, data: Partial<Script>) =>
      request<Script>(`/scripts/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
    delete: (id: number) =>
      request<{ deleted: number }>(`/scripts/${id}`, { method: 'DELETE' }),
  },
  contentItems: {
    list: () => request<ContentItem[]>('/content-items'),
    create: (data: Partial<ContentItem>) =>
      request<ContentItem>('/content-items', { method: 'POST', body: JSON.stringify(data) }),
    update: (id: number, data: Partial<ContentItem>) =>
      request<ContentItem>(`/content-items/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  },
  knowledge: {
    list: (params?: { doc_type?: string }) => {
      const q = new URLSearchParams(params as any).toString()
      return request<KnowledgeDoc[]>(`/knowledge${q ? `?${q}` : ''}`)
    },
    get: (id: number) => request<KnowledgeDoc>(`/knowledge/${id}`),
    create: (data: Partial<KnowledgeDoc>) =>
      request<KnowledgeDoc>('/knowledge', { method: 'POST', body: JSON.stringify(data) }),
    update: (id: number, data: Partial<KnowledgeDoc>) =>
      request<KnowledgeDoc>(`/knowledge/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  },
}
```

- [ ] **Step 3: 提交**

```bash
cd /root/workspace/opc
git add apps/web/lib/
git commit -m "feat(web): API client + shared types"
```

---

## Task 11-15: 5 个页面（选题 / 脚本 / 日历 / Dashboard / 知识库）

> 每个页面跟 Task 4 / Task 9 的 TDD 模式。先写一个最小的列表视图 + 创建表单。后续迭代加拖拽、搜索、富文本等。

**Task 11: 选题看板**（页面 1）
- 创建 `apps/web/app/topics/page.tsx`
- 列表 + 4 列状态（简化为单列表 + 状态过滤）
- 提交: `feat(web): topics page with list + create form`

**Task 12: 脚本库**（页面 2）
- 创建 `apps/web/app/scripts/page.tsx`
- 列表 + 创建表单（标题 + 关联选题 + markdown textarea）
- 提交: `feat(web): scripts page with list + create form`

**Task 13: 内容日历**（页面 3）
- 创建 `apps/web/app/calendar/page.tsx`
- 简化为按日期分组的列表（先不做月视图）
- 提交: `feat(web): calendar page with date-grouped list`

**Task 14: 表现 Dashboard**（页面 4）
- 创建 `apps/web/app/dashboard/page.tsx`
- 简化为 ContentItem 列表 + 总播放/互动/涨粉汇总
- 提交: `feat(web): dashboard page with summary stats`

**Task 15: 知识库**（页面 5）
- 创建 `apps/web/app/knowledge/page.tsx`
- 文档树（先做扁平列表 + 类型过滤）+ 创建表单
- 提交: `feat(web): knowledge page with doc list + create form`

---

## Task 16: MCP server 骨架

**Files:**
- Create: `apps/api/internal/mcp/server.go`
- Modify: `apps/api/cmd/server/main.go`

- [ ] **Step 1: 写 mcp server 测试**

写入 `apps/api/internal/mcp/server_test.go`:
```go
package mcp

import (
	"strings"
	"testing"
)

func TestServerHasCoreTools(t *testing.T) {
	s := NewServer(nil, "")
	tools := s.ListTools()
	want := []string{
		"list_topics", "create_topic", "get_script",
		"create_script", "log_content", "get_performance",
	}
	for _, w := range want {
		found := false
		for _, t := range tools {
			if t == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing tool: %s", w)
		}
	}
}

func TestToolNamesHaveOpcPrefix(t *testing.T) {
	s := NewServer(nil, "")
	for _, t := range s.ListTools() {
		if !strings.HasPrefix(t, "opc_") {
			t.Errorf("tool %s missing opc_ prefix", t)
		}
	}
}
```

- [ ] **Step 2: 跑测试，验证失败**

```bash
cd /root/workspace/opc/apps/api
go test ./internal/mcp/ -v
```

Expected: FAIL

- [ ] **Step 3: 实现 mcp server（最小可用版）**

写入 `apps/api/internal/mcp/server.go`:
```go
package mcp

import (
	"gorm.io/gorm"
)

type Server struct {
	db           *gorm.DB
	anthropicKey string
	tools        []string
}

func NewServer(db *gorm.DB, anthropicKey string) *Server {
	return &Server{
		db:           db,
		anthropicKey: anthropicKey,
		tools: []string{
			"opc_list_topics",
			"opc_create_topic",
			"opc_get_script",
			"opc_create_script",
			"opc_update_script",
			"opc_log_content",
			"opc_get_performance",
			"opc_generate_topics",
			"opc_humanize_script",
			"opc_deconstruct_viral",
		},
	}
}

func (s *Server) ListTools() []string {
	return s.tools
}

// 注：M2 上线版本只暴露工具列表，handler 真正实现在 Task 17+
```

- [ ] **Step 4: 跑测试，验证通过**

```bash
cd /root/workspace/opc/apps/api
go test ./internal/mcp/ -v
```

Expected: PASS

- [ ] **Step 5: 在 main.go 注册 mcp server 启动**

更新 `apps/api/cmd/server/main.go`，在最后加:
```go
	// 启动 MCP server（标准 IO 模式，让 Claude Code CLI 通过 stdio 调用）
	mcpServer := mcp.NewServer(gormDB, cfg.AnthropicAPIKey)
	log.Printf("🔌 MCP tools exposed: %v", mcpServer.ListTools())
	// 实际 stdIO 启动在 Task 17
	_ = mcpServer
```

- [ ] **Step 6: 提交**

```bash
cd /root/workspace/opc
git add apps/api/
git commit -m "feat(api): MCP server skeleton with tool list"
```

---

## Task 17: Claude agent 集成（用于 AI 选题/改写/复盘）

**Files:**
- Create: `apps/api/internal/agents/claude.go`
- Create: `apps/api/internal/agents/claude_test.go`

- [ ] **Step 1: 写测试**

写入 `apps/api/internal/agents/claude_test.go`:
```go
package agents

import (
	"strings"
	"testing"
)

func TestGenerateTopicsPrompt(t *testing.T) {
	a := NewClaude("test-key")
	prompt := a.GenerateTopicsPrompt("美食探店", "抖音", 5)
	if !strings.Contains(prompt, "美食探店") {
		t.Error("prompt should contain seed")
	}
	if !strings.Contains(prompt, "抖音") {
		t.Error("prompt should contain platform")
	}
	if !strings.Contains(prompt, "5") {
		t.Error("prompt should contain count")
	}
}

func TestHumanizeScriptPrompt(t *testing.T) {
	a := NewClaude("test-key")
	prompt := a.HumanizeScriptPrompt("这是 AI 生成的脚本，结构化、对仗工整")
	if !strings.Contains(prompt, "拟人化") || !strings.Contains(prompt, "AI 痕迹") {
		t.Error("humanize prompt missing key concepts")
	}
}
```

- [ ] **Step 2: 跑测试，验证失败**

```bash
cd /root/workspace/opc/apps/api
go test ./internal/agents/ -v
```

Expected: FAIL

- [ ] **Step 3: 实现 claude agent**

写入 `apps/api/internal/agents/claude.go`:
```go
package agents

import (
	"context"

	anthropic "github.com/anthropics/anthropic-sdk-go"
)

type Claude struct {
	client *anthropic.Client
}

func NewClaude(apiKey string) *Claude {
	return &Claude{
		client: anthropic.NewClient(anthropic.WithAPIKey(apiKey)),
	}
}

func (c *Claude) GenerateTopicsPrompt(seed, platform string, count int) string {
	return `你是 OPC 的"熊猫"IP 选题助手。
赛道：治愈 / 御宅 / 哲学 / 国潮。
平台：` + platform + `。
基于以下种子概念：` + seed + `

请生成 ` + itoa(count) + ` 个差异化选题，每个包含：
- 标题（10-20 字）
- 角度（独特视角）
- 预期表现推理（为什么这条有机会爆）
- 钩子方向（前 3 秒如何抓人）

输出 JSON 数组。`
}

func (c *Claude) HumanizeScriptPrompt(script string) string {
	return `请把以下脚本改写得"不像 AI 写的"：
- 去掉工整对仗、完美结构
- 加人手配、停顿、犹豫、转折
- 留白和呼吸感
- 平台不喜欢的"AI 八股"：排比、三段式、过度铺垫

原脚本：
` + script + `

输出改写后的版本。`
}

func (c *Claude) Complete(ctx context.Context, prompt string) (string, error) {
	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.F(anthropic.ModelClaudeSonnet4_5),
		MaxTokens: anthropic.Int(4096),
		Messages: anthropic.F([]anthropic.Message{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		}),
	})
	if err != nil {
		return "", err
	}
	if len(resp.Content) == 0 {
		return "", nil
	}
	return resp.Content[0].Text, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := false
	if n < 0 {
		negative = true
		n = -n
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if negative {
		return "-" + string(digits)
	}
	return string(digits)
}
```

- [ ] **Step 4: 跑测试，验证通过**

```bash
cd /root/workspace/opc/apps/api
go test ./internal/agents/ -v
```

Expected: PASS

- [ ] **Step 5: 提交**

```bash
cd /root/workspace/opc
git add apps/api/
git commit -m "feat(api): Claude agent integration with prompt templates"
```

---

## Task 18: 部署到 VPS（首次上线）

**Files:**
- Create: `scripts/deploy.sh`
- Create: `apps/api/.dockerignore`
- Create: `apps/web/.dockerignore`

- [ ] **Step 1: 写 deploy.sh**

写入 `/root/workspace/opc/scripts/deploy.sh`:
```bash
#!/bin/bash
set -e

VPS_HOST=${VPS_HOST:-"opc-vps"}
REMOTE_DIR=${REMOTE_DIR:-"/opt/opc"}

echo "🚀 Deploying OPC to $VPS_HOST..."

# Build images locally
docker compose build

# Save and copy
docker save opc-api | ssh $VPS_HOST "docker load"
docker save opc-web | ssh $VPS_HOST "docker load"

# Push compose file
scp docker-compose.yml $VPS_HOST:$REMOTE_DIR/

# Remote restart
ssh $VPS_HOST "cd $REMOTE_DIR && docker compose down && docker compose up -d"

echo "✅ Deployed. Check: ssh $VPS_HOST 'docker ps'"
```

```bash
chmod +x /root/workspace/opc/scripts/deploy.sh
```

- [ ] **Step 2: 写 .dockerignore**

写入 `apps/api/.dockerignore`:
```
bin/
tmp/
*.db
*.db-journal
.git/
.idea/
*.md
```

写入 `apps/web/.dockerignore`:
```
node_modules/
.next/
out/
.git/
.idea/
*.md
```

- [ ] **Step 3: 本地 docker compose 验证**

```bash
cd /root/workspace/opc
docker compose up -d
sleep 5
curl http://localhost:8080/health
# Expected: {"status":"ok"}
curl http://localhost:3000
# Expected: HTML
docker compose down
```

- [ ] **Step 4: 提交**

```bash
cd /root/workspace/opc
git add scripts/ apps/*/.dockerignore
git commit -m "feat: deploy script and docker config"
```

---

# M3：爆发期（4 周）— 工具迭代 + 押注爆款

> **内容主线**: 押注爆款，目标首个 100 万+播放
> **工具主线**: 接入 Claude agent 到 web UI（AI 选题、AI 改写、爆款复盘）

## Task 19: 选题页面集成 AI 选题按钮

**Files:**
- Modify: `apps/web/app/topics/page.tsx`
- Create: `apps/api/internal/handlers/agents.go`

- [ ] **Step 1: 后端：写 AI 选题 endpoint**

在 `apps/api/internal/handlers/agents.go`:
```go
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/opc/api/internal/agents"
)

type AgentsHandler struct {
	claude *agents.Claude
}

func NewAgentsHandler(claude *agents.Claude) *AgentsHandler {
	return &AgentsHandler{claude: claude}
}

type GenerateTopicsRequest struct {
	Seed     string `json:"seed" binding:"required"`
	Platform string `json:"platform" binding:"required"`
	Count    int    `json:"count"`
}

func (h *AgentsHandler) GenerateTopics(c *gin.Context) {
	var req GenerateTopicsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Count == 0 {
		req.Count = 5
	}
	prompt := h.claude.GenerateTopicsPrompt(req.Seed, req.Platform, req.Count)
	result, err := h.claude.Complete(c.Request.Context(), prompt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": result})
}
```

- [ ] **Step 2: 注册路由**

更新 `apps/api/cmd/server/main.go`:
```go
	agentsH := handlers.NewAgentsHandler(agents.NewClaude(cfg.AnthropicAPIKey))
	r.POST("/agents/generate-topics", agentsH.GenerateTopics)
```

- [ ] **Step 3: 前端：加 "AI 选题" 按钮**

在 `apps/web/app/topics/page.tsx` 加按钮 + 调 API 的逻辑（完整代码略，按现有 UI 模式加）

- [ ] **Step 4: 端到端验证**

```bash
cd /root/workspace/opc/apps/api
ANTHROPIC_API_KEY=$YOUR_KEY DB_PATH=/tmp/test.db go run ./cmd/server &
sleep 2

curl -X POST http://localhost:8080/agents/generate-topics \
  -H "Content-Type: application/json" \
  -d '{"seed":"美食探店","platform":"抖音","count":3}'
# Expected: 200, 返回 AI 生成的选题

kill %1
```

- [ ] **Step 5: 提交**

```bash
git add apps/
git commit -m "feat: AI generate-topics endpoint + UI button"
```

---

## Task 20-25: M3-M4 持续迭代

> 这部分是 M3-M4 的内容主线 + 工具微迭代。详细内容留白，按需展开。

- **Task 20**: 脚本库接入 AI 改写 + AI 拟人化按钮
- **Task 21**: 表现 dashboard 接入爆款复盘（输入视频链接 → AI 复盘报告）
- **Task 22**: 内容日历支持拖拽改时间
- **Task 23**: 知识库接入 AI 总结 + 交叉引用推荐
- **Task 24**: 选题看板支持拖拽改状态（完整 kanban）
- **Task 25**: 性能优化（DB 索引、查询优化、SQLite WAL 模式）

每个 task 按 M2 的 TDD 模式：写测试 → 实现 → 验证 → 提交。

---

# M5-M6：稳定 + 复盘（8 周）

> **内容主线**: 试 1-2 个副 IP；运营节奏稳定
> **工具主线**: bug 修复、性能优化、准备对外开放

## Task 26-30: M5-M6 任务（outline）

- **Task 26**: 副 IP 模板化（看熊猫 IP 经验能否复用）
- **Task 27**: 知识库全文搜索升级（SQLite FTS5）
- **Task 28**: 数据导入/导出（JSON / CSV）
- **Task 29**: web UI 响应式优化（移动端可用）
- **Task 30**: 部署稳定化（HTTPS、自动备份、监控告警）

---

# 阶段交付物

## M1 末（4 周后）
- [x] Monorepo 骨架（Task 1）
- [x] Go 后端 + DB（Task 2-3）
- [x] 5 实体 CRUD（Task 4-8）
- [x] Next.js 骨架（Task 9）
- [x] API client（Task 10）
- [x] 30 条内容已发，1 万粉账号
- [x] M1 SOP 已写入知识库

## M2 末（8 周后）
- [x] 5 个 web UI 页面（Task 11-15）
- [x] MCP server 暴露 10 个工具（Task 16-17）
- [x] 部署到 VPS（Task 18）
- [x] Claude agent 集成（Task 17 + 19）
- [x] 首个 10 万+播放

## M3 末（12 周后）
- [x] AI 选题 / 改写 / 复盘 UI（Task 19-23）
- [x] 完整 kanban + 日历
- [x] 首个 100 万+播放
- [x] 5 万粉

## M4 末（16 周后）
- [x] SOP 完整沉淀到知识库
- [x] 风格库有 100+ 脚本特征
- [x] 工具成为日常工作流一部分

## M5 末（20 周后）
- [x] 副 IP 验证（1-2 个）
- [x] 知识库 FTS5 搜索
- [x] 数据导入/导出

## M6 末（24 周后 = Phase 1 结束）
- [x] 至少 1 平台 10 万+粉
- [x] 至少 1 个 500 万+爆款
- [x] web UI 5 页面稳定可用
- [x] MCP server 稳定
- [x] **Go/No-Go 决策**：是否进入 Phase 2？

---

# 风险预案

| 风险 | 触发条件 | 应对 |
|------|----------|------|
| 全栈开发不到位 | M1 末还没找到 Go 经验开发者 | 降低到极简模式：1+1，无 web UI |
| 内容跑不出来 | M1 末 < 1 万粉 | 评估是否换 IP/换赛道 |
| 平台政策收紧 | 抖音限流/AI 限流严重 | 加快 B 站/小红书投入 |
| DB 性能瓶颈 | SQLite 撑不住 | M4 末迁 PostgreSQL |

---

**下一步**：
- 用户审阅本计划
- 审阅通过后选择执行方式（subagent-driven 推荐 / inline）
