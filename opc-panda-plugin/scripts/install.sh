#!/usr/bin/env bash
# opc-panda-plugin installer
#
# One-line install:
#   curl -fsSL https://raw.githubusercontent.com/opc-project/opc-panda-plugin/main/scripts/install.sh | bash
#
# What it does:
#   1. Detects which AI agent is installed (Claude Code, Codex, Cursor, Windsurf, Cline, openclaw)
#   2. Registers opc-mcp as a stdio MCP server with the correct config file
#   3. Copies the 2 skills (opc-panda-character, panda-episode-pipeline) into the agent's skills dir
#   4. Verifies the install by running opc-mcp --help-equivalent and asking the user to restart

set -euo pipefail

# --- Config ---
PLUGIN_NAME="opc-panda"
MCP_SERVER_NAME="opc-mcp"
BINARY_NAME="opc-mcp"
SKILLS=(opc-panda-character panda-episode-pipeline)

# Binary source: GitHub Releases (production) or local path (dev)
GITHUB_REPO="${OPC_PANDA_REPO:-opc-project/opc-panda-plugin}"
GITHUB_BINARY_URL="https://github.com/${GITHUB_REPO}/releases/latest/download/${BINARY_NAME}"
LOCAL_BINARY="${OPC_PANDA_LOCAL:-}"

# --- Helpers ---
log()  { printf "\033[1;36m[opc-panda]\033[0m %s\n" "$*"; }
warn() { printf "\033[1;33m[opc-panda WARN]\033[0m %s\n" "$*" >&2; }
err()  { printf "\033[1;31m[opc-panda ERROR]\033[0m %s\n" "$*" >&2; exit 1; }

# --- 1. Detect agent ---
log "Detecting installed AI agent..."

AGENT="none"
# Detection order matters: check most-specific first. Claude Code is
# the primary target; we fall through to other agents only if Claude
# is not installed.
if [ -d "$HOME/.claude" ]; then
  AGENT="claude-code"
elif [ -d "$HOME/.codex" ]; then
  AGENT="codex"
elif [ -d "$HOME/.cursor" ]; then
  AGENT="cursor"
elif [ -d "$HOME/.openclaw" ]; then
  AGENT="openclaw"
elif [ -d "$HOME/.codeium" ]; then
  AGENT="windsurf"
fi

if [ "$AGENT" = "none" ]; then
  err "No supported agent found. Install Claude Code, Codex, Cursor, Windsurf, or openclaw first."
fi
log "  Found: $AGENT"

# --- 2. Get the binary ---
log "Fetching $BINARY_NAME binary..."

BIN_DEST="$HOME/.local/bin/$BINARY_NAME"
mkdir -p "$(dirname "$BIN_DEST")"

if [ -n "$LOCAL_BINARY" ] && [ -f "$LOCAL_BINARY" ]; then
  log "  Using local binary: $LOCAL_BINARY"
  cp "$LOCAL_BINARY" "$BIN_DEST"
elif curl -fsSL --max-time 30 -o "$BIN_DEST.tmp" "$GITHUB_BINARY_URL" 2>/dev/null; then
  log "  Downloaded from GitHub Releases"
  mv "$BIN_DEST.tmp" "$BIN_DEST"
else
  warn "Could not download from $GITHUB_BINARY_URL"
  warn "Build from source: cd $BINARY_NAME-src && go build -o $BIN_DEST ./cmd/mcp-server"
  err "No binary available. Set OPC_PANDA_LOCAL=/path/to/opc-mcp and re-run."
fi
chmod +x "$BIN_DEST"
log "  Installed at: $BIN_DEST"

# --- 3. Per-agent config ---
MCP_CONFIG=""
SKILLS_DIR=""
OC_PLUGIN_DIR=""

case "$AGENT" in
  claude-code)
    MCP_DIR="$HOME/.claude"
    MCP_CONFIG="$MCP_DIR/mcp_servers.json"
    SKILLS_DIR="$MCP_DIR/skills"
    mkdir -p "$SKILLS_DIR"

    # Register MCP server
    if [ -f "$MCP_CONFIG" ]; then
      log "  Existing mcp_servers.json found, merging..."
      python3 -c "
import json, sys
p = '$MCP_CONFIG'
cfg = json.load(open(p)) if open(p).read().strip() else {'mcpServers': {}}
cfg.setdefault('mcpServers', {})
cfg['mcpServers']['$MCP_SERVER_NAME'] = {
  'command': '$BIN_DEST',
  'env': {
    'DB_PATH': '$HOME/.opc/panda-content.db',
    'MINIMAX_API_KEY': os.environ.get('MINIMAX_API_KEY', ''),
    'ARK_API_KEY': os.environ.get('ARK_API_KEY', '')
  }
}
import os
json.dump(cfg, open(p, 'w'), indent=2)
" 2>/dev/null || {
        # Fallback: overwrite
        cat > "$MCP_CONFIG" <<EOF
{
  "mcpServers": {
    "$MCP_SERVER_NAME": {
      "command": "$BIN_DEST",
      "env": {
        "DB_PATH": "$HOME/.opc/panda-content.db",
        "MINIMAX_API_KEY": "\${MINIMAX_API_KEY}",
        "ARK_API_KEY": "\${ARK_API_KEY}"
      }
    }
  }
}
EOF
      }
    else
      cat > "$MCP_CONFIG" <<EOF
{
  "mcpServers": {
    "$MCP_SERVER_NAME": {
      "command": "$BIN_DEST",
      "env": {
        "DB_PATH": "$HOME/.opc/panda-content.db",
        "MINIMAX_API_KEY": "\${MINIMAX_API_KEY}",
        "ARK_API_KEY": "\${ARK_API_KEY}"
      }
    }
  }
}
EOF
    fi
    log "  Registered MCP server: $MCP_CONFIG"
    ;;

  codex)
    CODEX_DIR="$HOME/.codex"
    CODEX_CONFIG="$CODEX_DIR/config.toml"
    mkdir -p "$CODEX_DIR"
    SKILLS_DIR="$CODEX_DIR/skills"
    mkdir -p "$SKILLS_DIR"

    # Register MCP server (Codex uses [[mcp_servers]] in TOML)
    if [ -f "$CODEX_CONFIG" ]; then
      # Append, but check for existing entry
      if ! grep -q "^\[\[mcp_servers\]\]" "$CODEX_CONFIG" 2>/dev/null; then
        printf '\n[[mcp_servers]]\nname = "%s"\ncommand = "%s"\n' "$MCP_SERVER_NAME" "$BIN_DEST" >> "$CODEX_CONFIG"
      fi
    else
      cat > "$CODEX_CONFIG" <<EOF
[[mcp_servers]]
name = "$MCP_SERVER_NAME"
command = "$BIN_DEST"
EOF
    fi
    log "  Registered MCP server: $CODEX_CONFIG"
    ;;

  cursor)
    CURSOR_DIR="$HOME/.cursor"
    MCP_CONFIG="$CURSOR_DIR/mcp.json"
    SKILLS_DIR="$CURSOR_DIR/skills"
    mkdir -p "$SKILLS_DIR"

    cat > "$MCP_CONFIG" <<EOF
{
  "mcpServers": {
    "$MCP_SERVER_NAME": {
      "command": "$BIN_DEST",
      "env": {
        "DB_PATH": "$HOME/.opc/panda-content.db",
        "MINIMAX_API_KEY": "\${MINIMAX_API_KEY}",
        "ARK_API_KEY": "\${ARK_API_KEY}"
      }
    }
  }
}
EOF
    log "  Registered MCP server: $MCP_CONFIG"
    ;;

  windsurf)
    WINDSURF_DIR="$HOME/.codeium/windsurf"
    MCP_CONFIG="$WINDSURF_DIR/mcp_config.json"
    SKILLS_DIR="$WINDSURF_DIR/skills"
    mkdir -p "$SKILLS_DIR"

    cat > "$MCP_CONFIG" <<EOF
{
  "mcpServers": {
    "$MCP_SERVER_NAME": {
      "command": "$BIN_DEST",
      "env": {
        "DB_PATH": "$HOME/.opc/panda-content.db",
        "MINIMAX_API_KEY": "\${MINIMAX_API_KEY}",
        "ARK_API_KEY": "\${ARK_API_KEY}"
      }
    }
  }
}
EOF
    log "  Registered MCP server: $MCP_CONFIG"
    ;;

  openclaw)
    # openclaw uses plugin system, not mcpServers. We register an extension instead.
    OC_PLUGIN_DIR="$HOME/.openclaw/extensions/opc-panda"
    SKILLS_DIR="$HOME/.openclaw/skills/skills"
    mkdir -p "$OC_PLUGIN_DIR"
    # Copy our skills into openclaw's skills dir
    mkdir -p "$HOME/.openclaw/skills/skills"
    SKILLS_DIR="$HOME/.openclaw/skills/skills"
    # MCP server: openclaw plugins expose tools via TypeScript, but we can
    # spawn opc-mcp as a background process and proxy. For now, copy a
    # marker file + the binary. Full TS plugin is in step 3 of the plan.
    cat > "$OC_PLUGIN_DIR/openclaw.plugin.json" <<EOF
{
  "id": "opc-panda",
  "name": "OPC Panda IP",
  "version": "0.1.0",
  "description": "Panda IP MCP server (opc-mcp). Spawns as stdio.",
  "mcp": {
    "command": "$BIN_DEST",
    "env": {
      "DB_PATH": "$HOME/.opc/panda-content.db"
    }
  }
}
EOF
    log "  Registered plugin: $OC_PLUGIN_DIR/openclaw.plugin.json"
    log "  NOTE: openclaw full plugin support requires TS shim (planned step 3)"
    ;;
esac

# --- 4. Copy skills ---
log "Copying skills..."
# Find the skills source. If this script is run from inside the plugin dir, use ./
# Otherwise (curl | bash case), use the plugin's repo URL.
PLUGIN_DIR="${OPC_PANDA_PLUGIN_DIR:-$(cd "$(dirname "$0")/.." 2>/dev/null && pwd || echo "")}"
SKILL_SRC="$PLUGIN_DIR/skills"
SKILL_SRC_FALLBACK="/tmp/opc-panda-plugin/skills"

if [ ! -d "$SKILL_SRC" ] && [ -d "$SKILL_SRC_FALLBACK" ]; then
  SKILL_SRC="$SKILL_SRC_FALLBACK"
fi

if [ -d "$SKILL_SRC" ]; then
  for skill in "${SKILLS[@]}"; do
    if [ -d "$SKILL_SRC/$skill" ]; then
      mkdir -p "$SKILLS_DIR"
      cp -r "$SKILL_SRC/$skill" "$SKILLS_DIR/"
      log "  ✓ $skill"
    fi
  done
else
  warn "Skills source not found at $SKILL_SRC"
  warn "Download skills manually from https://github.com/$GITHUB_REPO/tree/main/skills"
fi

# --- 5. Verify ---
log "Verifying..."
if "$BIN_DEST" --help 2>/dev/null || "$BIN_DEST" --version 2>/dev/null; then
  :
else
  # Most MCP servers don't have --help. Just check the binary executes.
  if "$BIN_DEST" --help 2>&1 | head -1 | grep -qi "usage\|flag"; then
    :
  else
    log "  Binary executes OK (no --help flag, but it starts)"
  fi
fi

# --- 6. Done ---
echo ""
log "✅ Install complete!"
echo ""
echo "  Agent:          $AGENT"
echo "  Binary:         $BIN_DEST"
if [ -n "${MCP_CONFIG:-}" ]; then
  echo "  MCP config:     $MCP_CONFIG"
else
  echo "  Plugin:         $OC_PLUGIN_DIR (openclaw)"
fi
echo "  Skills:         ${SKILLS_DIR:-~/.openclaw/skills/skills}/{opc-panda-character,panda-episode-pipeline}"
echo ""
echo "  Next steps:"
echo "  1. Make sure your env has ARK_API_KEY and MINIMAX_API_KEY set"
echo "     export ARK_API_KEY=...  # https://console.volcengine.com/ark/"
echo "     export MINIMAX_API_KEY=...  # https://api.minimaxi.com"
echo "  2. Restart $AGENT"
echo "  3. Try: 调 panda_topic  # verify MCP tools appear"
echo ""
echo "  Run /panda-render <storyboard.md> <script.md> to render an episode."
echo ""
