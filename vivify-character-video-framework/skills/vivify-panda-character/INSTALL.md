# Install: Panda IP + Claude Code + vivify-mcp

> **Audience**: 人类 (content lead / AI 操作手),**不是 Claude**。Claude 读
> `SKILL.md` (那是 IP bible), 这文件是给"第一次用这个 skill"的人读的
> 一次性 setup 指南。
>
> **5 分钟做完**,然后你就有了一个"知道 panda 是什么 + 能调 OPC 真实工具"的
> Claude Code 窗口。

## 这是什么

OPC (Open Personal Creator) 的 panda IP 内容生产链:

```
你 (content lead)  →  Claude Code (AI 客户端)  →  vivify-mcp binary  →  SQLite
                         (装了 panda skill)         (JSON-RPC stdio)   (本地)
```

- **你** 用 Claude Code (跟普通开发 / 写作用一样的工具)
- **Claude Code** 装了 `vivify-panda-character` skill → 知道 panda 是什么 (峰哥 / 4 voice / 5 套服饰 / 场景白黑名单)
- **Claude Code** 启动 `vivify-mcp` 子进程 → JSON-RPC over stdio
- **vivify-mcp** 读 SQLite → 10 个 `opc_*` tool 可调 (create_topic / generate_topics / humanize_script / log_content 等)

Claude 知道 panda (skill) + 能调工具 (MCP) = **panda 内容生产助手**。

## 5 分钟 setup

### 1. Build vivify-mcp binary (1 次, 30s)

```bash
cd /root/workspace/opc
make mcp
# Output: apps/bin/vivify-mcp (12MB, stdio JSON-RPC, no HTTP server)
```

### 2. Wire Claude Code (1 次, 30s)

`~/.claude/mcp_servers.json` (文件可能不存在,创建):

```json
{
  "vivify-mcp": {
    "command": "/root/workspace/opc/apps/bin/vivify-mcp",
    "env": {
      "DB_PATH": "/tmp/vivify-content.db",
      "MINIMAX_API_KEY": "sk-..."
    }
  }
}
```

- `DB_PATH`: SQLite 文件位置,任何可写路径。多人协作时换 shared 路径
- `MINIMAX_API_KEY`: 上游 LLM key (空 = demo mode,跑 canned 响应)
- `ENCRYPTION_KEY` (可选,生产用): 32-byte base64,见 `apps/api/.env.example`

### 3. Restart Claude Code (1 次, 10s)

完全退出再开。Claude Code 启动时会读 `mcp_servers.json` 并 spawn 每个 MCP server。

### 4. Verify (1 次, 30s)

重启后,在 Claude Code 里输入:

```
列出 vivify-mcp 的 10 个 tool
```

或更直接:

```
调 opc_list_topics
```

如果看到 10 个 tool 名 (opc_list_topics / opc_create_topic / opc_get_script / opc_create_script / opc_update_script / opc_log_content / opc_get_performance / opc_generate_topics / opc_humanize_script / opc_deconstruct_viral) — 装好了。

如果 "tool not found" — 检查:

- `apps/bin/vivify-mcp` 文件存在 + 可执行 (`ls -la apps/bin/vivify-mcp`)
- 手动跑一次: `DB_PATH=/tmp/test.db apps/bin/vivify-mcp` 应该不报错 (它在等 stdin)
- `~/.claude/mcp_servers.json` 路径对, JSON 合法 (no trailing comma)

## 5. 开始用 (5 min, 1 个 episode 跑通)

最简单的 panda 内容工作流:

```
[你] 给我 5 个 panda 凌晨独处风格的抖音选题
[Claude] (调 opc_generate_topics, 返 5 候选)
[你] 第 3 个最好, 存下来
[Claude] (调 opc_create_topic, 返 topic id)
[你] 基于这个 topic 写个 60s 脚本, 注意 panda 4 调性里的"御宅" + 留白结尾
[Claude] (用 skill 里的 IP bible + 御宅调性 + opc_create_script, 返 script id)
[你] 调 opc_humanize_script 把 AI 味去掉
[Claude] (调 opc_humanize_script, 返改后脚本; 再调 opc_update_script 存)
[你] 我准备发了, 用 opc_log_content 记下来
[Claude] (调 opc_log_content, 需要你给 platform + URL)
[你] 24h 后用 opc_get_performance 拉数据
[Claude] (调 opc_get_performance, 返 views / likes / comments)
```

**0 行代码, 0 切工具, 全在 Claude Code 一个窗口**。

## 常见问题

### Q: Claude 调错 tool 了 (e.g. 把 opc_create_topic 跟 opc_generate_topics 搞混)

A: 这是 skill description 没读全。提示 Claude "读 vivify-panda-character skill 的 IP bible + 看每个 tool 的 description"。description 现在写得很清楚 (e.g. "Does NOT generate — use opc_generate_topics for that")。

### Q: vivify-mcp 启动失败

A: 跑一次 `apps/bin/vivify-mcp` 看 stderr 错误。常见原因:
- `DB_PATH` 目录不可写 → 改路径
- `MINIMAX_API_KEY` 无效 + 不是 demo mode → 留空
- `ENCRYPTION_KEY` 长度不对 (生产用) → 看 `apps/api/.env.example`

### Q: 我在多人 team 怎么共享 DB?

A: 换 shared path (e.g. SMB / NFS / GCS Fuse) + 共享 `ENCRYPTION_KEY`。SQLite 支持 file lock, 多人并发写会有 busy error — 现阶段 1 个 content lead 用够了, 多人协作是 Phase 2 后期的事。

### Q: 我不想装 skill 直接用 tool 呢?

A: 装 skill 是给 Claude 知识, **不装 skill 也能调 tool**。区别: 装 skill → Claude 知道 panda 是什么 + 调对 tool; 不装 skill → Claude 调得对 tool 但生成的内容不知道 panda voice。

## 跟 OPC API (HTTP) 的关系

- **vivify-mcp**: 给 AI agents (Claude Code / Cursor) 用的 **stdio JSON-RPC** 协议
- **vivify-api** (HTTP, port 8080): 给 web UI / 第三方脚本用的 **HTTP REST** 协议

两个 surface 共享同一个 SQLite。两个 client 不冲突, 可以同时用:
- 内容主导在 Claude Code 用 vivify-mcp 写内容
- 团队其他人用浏览器 / curl 调 vivify-api 看 dashboards (admin 视角)

详见 `docs/console-reserve-audit.md` + `docs/superpowers/plans/2026-06-16-opc-phase2-anti-ai-m1.md` (后者跟 MCP 无关, 是另一个 workstream)。

## 跟 SKILL.md 的边界

- **SKILL.md** (这个 skill 的核心): Claude 读。IP bible / 调性 / 视觉 / 道具 / 场景 规则。
- **INSTALL.md** (本文件): 人类读。怎么装 vivify-mcp + 配 Claude Code + 5 分钟 onboarding。

Claude **不会** 读 INSTALL.md (它在 Claude 的视野外)。 人类 **必须** 读 INSTALL.md 一次。
