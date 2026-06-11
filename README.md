# OPC

AI 视频创作平台（NemoVideo 混合模式）。

## 当前阶段

Phase 1：跑"熊猫"AI IP + 5 页面 web UI MVP（内部用）+ MCP server。

详见 `docs/superpowers/specs/2026-06-03-opc-phase1-ip-design.md`

## 快速开始

```bash
# 安装依赖
./scripts/dev.sh setup

# 启动开发环境（DB + 后端 + 前端）
./scripts/dev.sh up
```

### 构建选项

后端默认 build 不包含 FTS5（知识库全文搜索）。要启用：

```bash
# 启用 FTS5（带搜索功能）—— 输出到 apps/bin/opc-api
cd apps/api && go build -tags fts5 -o ../bin/opc-api ./cmd/server

# 不启用 FTS5（更小的二进制，知识库只能按标题/类型过滤）
cd apps/api && go build -o ../bin/opc-api ./cmd/server
```

构建产物在 monorepo 布局下统一写到 `apps/bin/`：API 服务在
`apps/bin/opc-api`，asset CLI 在 `apps/bin/opc-asset`（通过
`make asset` 构建）。

测试同理：`go test -tags fts5 ./...`

## 目录结构

- `apps/web/` — Next.js 前端（5 个页面）
- `apps/api/` — Go 后端（Gin + GORM）
- `docs/superpowers/` — 设计与实施文档
- `scripts/` — 运维脚本

## 团队工作流

1. **内容主导**：用 web UI 的选题看板/脚本库/内容日历
2. **AI 操作手**：用 web UI + 可灵/即梦/剪映生成视频
3. **全栈开发**：维护 web UI + 后端 + MCP server
