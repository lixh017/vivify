# opc-panda-plugin

OPC panda IP (峰哥 / Fengge) content production for Claude Code.

This plugin bundles:
- **14 MCP tools** via opc-mcp (4 panda_* + 10 opc_*) — stored in SQLite
- **2 skills** (opc-panda-character bible, panda-episode-pipeline runner)
- **1 command** (`/panda-render`) — one-shot episode render

## One-line install

```bash
# From a GitHub repo (production)
curl -fsSL https://raw.githubusercontent.com/opc-project/opc-panda-plugin/main/scripts/install.sh | bash

# From a local checkout (dev)
OPC_PANDA_LOCAL=/path/to/opc-mcp bash /path/to/opc-panda-plugin/scripts/install.sh
```

The installer:
1. Detects your agent (Claude Code, Codex, Cursor, Windsurf, or openclaw)
2. Downloads `opc-mcp` binary from GitHub Releases (or uses `OPC_PANDA_LOCAL` path)
3. Writes the correct MCP config for your agent
4. Copies both skills into the agent's skills directory
5. Asks you to restart the agent

## What you get

After install + restart:

| Tool | What it does |
|---|---|
| `panda_topic` | Generate 5 candidate topics for a panda episode (voice + hook) |
| `panda_script` | Write a 60s script from a topic, baking in IP voice rules |
| `panda_voice` | Read-only audit of a script (voice mix, AI tells, IP compliance) |
| `panda_storyboard` | Split a script into 6+ shot prompts (Kling/即梦 ready) |
| `opc_create_topic` / `opc_list_topics` | Persist + browse topics |
| `opc_create_script` / `opc_get_script` / `opc_update_script` | Script CRUD |
| `opc_log_content` / `opc_get_performance` | Track published content + metrics |
| `opc_generate_topics` / `opc_humanize_script` / `opc_deconstruct_viral` | Generic MCN tools |

Plus 2 skills:
- **opc-panda-character**: the panda IP bible (4 voice tones, 5 outfits, scene whitelist)
- **panda-episode-pipeline**: end-to-end L2+L3 video render

Plus 1 command:
- **`/panda-render <storyboard.md> <script.md>`**: render a finished MP4 in one shot

## Workflow

```
[You] 给我 5 个 panda 凌晨独处风格的抖音选题
[Claude] 调 panda_topic  → 5 candidates (~3 min)
[You] 选 #3
[Claude] 调 opc_create_topic + panda_script  → 60s 脚本
[Claude] 调 panda_voice  → voice report + suggestions
[You] /panda-render STORYBOARD.md SCRIPT.md
[Claude] 调 skill  → 30-50 min later, 1 个 L3 MP4 出来
[你] 上抖音 web UI
```

## Requirements

- ARK_API_KEY (火山引擎 Ark — https://console.volcengine.com/ark/)
- MINIMAX_API_KEY (海螺 MiniMax — https://api.minimaxi.com)
- ffmpeg (free, apt-get install ffmpeg on Debian/Ubuntu)

## File structure

```
opc-panda-plugin/
├── .claude-plugin/
│   └── plugin.json            # Claude Code plugin manifest
├── mcp-servers/
│   └── opc-mcp/
│       └── bin/opc-mcp         # 12 MB Go binary (stdio JSON-RPC MCP server)
├── skills/
│   ├── opc-panda-character/    # SKILL.md (read by Claude)
│   │   └── INSTALL.md
│   └── panda-episode-pipeline/ # SKILL.md + render_episode.py
│       ├── INSTALL.md
│       ├── render_episode.py
│       └── examples/
├── commands/
│   └── panda-render.md         # /panda-render slash command
├── scripts/
│   └── install.sh              # auto-detect agent + install
├── README.md
└── LICENSE
```

## Manual install (alternative to install.sh)

```bash
# 1. Build or download opc-mcp
go build -o ~/.local/bin/opc-mcp ./cmd/mcp-server
# (or download from GitHub Releases)

# 2. Add to Claude Code
mkdir -p ~/.claude
cat >> ~/.claude/mcp_servers.json <<EOF
{
  "mcpServers": {
    "opc-mcp": {
      "command": "~/.local/bin/opc-mcp",
      "env": {
        "DB_PATH": "~/.opc/panda-content.db",
        "MINIMAX_API_KEY": "\${MINIMAX_API_KEY}",
        "ARK_API_KEY": "\${ARK_API_KEY}"
      }
    }
  }
}
EOF

# 3. Copy skills
cp -r skills/* ~/.claude/skills/

# 4. Restart Claude Code
```

## License

MIT
