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

## 目录结构

- `apps/web/` — Next.js 前端（5 个页面）
- `apps/api/` — Go 后端（Gin + GORM）
- `docs/superpowers/` — 设计与实施文档
- `scripts/` — 运维脚本

## 团队工作流

1. **内容主导**：用 web UI 的选题看板/脚本库/内容日历
2. **AI 操作手**：用 web UI + 可灵/即梦/剪映生成视频
3. **全栈开发**：维护 web UI + 后端 + MCP server
