#!/usr/bin/env bash
# opc-panda-plugin uninstaller
#
# Removes the MCP server entry, skills, and binary. Safe to re-run install.sh after.

set -euo pipefail

log()  { printf "\033[1;36m[opc-panda]\033[0m %s\n" "$*"; }
warn() { printf "\033[1;33m[opc-panda WARN]\033[0m %s\n" "$*" >&2; }

# --- Detect agent ---
# Detection order matters: Claude Code is primary; only fall through
# to other agents if Claude is not installed.
AGENT="none"
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
  warn "No supported agent found, nothing to do"
  exit 0
fi
log "Detected: $AGENT"

# --- Remove MCP config entry ---
case "$AGENT" in
  claude-code|cursor|windsurf)
    # JSON config files
    for f in "$HOME/.claude/mcp_servers.json" \
             "$HOME/.cursor/mcp.json" \
             "$HOME/.codeium/windsurf/mcp_config.json"; do
      if [ -f "$f" ]; then
        log "  Cleaning $f..."
        python3 -c "
import json, sys
p = '$f'
try:
    cfg = json.load(open(p))
except Exception:
    print('  (skip — not valid JSON)')
    sys.exit(0)
if 'mcpServers' in cfg and 'opc-mcp' in cfg['mcpServers']:
    del cfg['mcpServers']['opc-mcp']
    if not cfg['mcpServers']:
        del cfg['mcpServers']
    json.dump(cfg, open(p, 'w'), indent=2)
    print('  ✓ removed opc-mcp entry')
else:
    print('  (no opc-mcp entry)')
" || true
      fi
    done
    ;;

  codex)
    CODEX_CONFIG="$HOME/.codex/config.toml"
    if [ -f "$CODEX_CONFIG" ]; then
      log "  Cleaning $CODEX_CONFIG..."
      # Remove the [[mcp_servers]] block (opc-mcp is the only one we add)
      # This is a multi-line block: [[mcp_servers]]\nname = "opc-mcp"\ncommand = "..."
      python3 -c "
import re
p = '$CODEX_CONFIG'
text = open(p).read()
# Remove [[mcp_servers]] blocks that contain 'opc-mcp'
new_text = re.sub(
    r'^\[\[mcp_servers\]\][^\[]*?name = \"opc-mcp\"[^\[]*?(?=\n\[|\Z)',
    '',
    text,
    flags=re.MULTILINE | re.DOTALL
)
if new_text != text:
    open(p, 'w').write(new_text)
    print('  ✓ removed opc-mcp [[mcp_servers]] block')
else:
    print('  (no opc-mcp block)')
" || true
    fi
    ;;

  openclaw)
    OC_PLUGIN_DIR="$HOME/.openclaw/extensions/opc-panda"
    if [ -d "$OC_PLUGIN_DIR" ]; then
      log "  Removing $OC_PLUGIN_DIR..."
      rm -rf "$OC_PLUGIN_DIR"
      log "  ✓ plugin removed"
    fi
    ;;
esac

# --- Remove skills ---
log "Removing skills..."
for skill in opc-panda-character panda-episode-pipeline; do
  for d in "$HOME/.claude/skills/$skill" \
           "$HOME/.codex/skills/$skill" \
           "$HOME/.cursor/skills/$skill" \
           "$HOME/.openclaw/skills/skills/$skill" \
           "$HOME/.codeium/windsurf/skills/$skill"; do
    if [ -d "$d" ]; then
      rm -rf "$d"
      log "  ✓ $d"
    fi
  done
done

# --- Remove binary (only if no other consumer) ---
BIN="$HOME/.local/bin/opc-mcp"
if [ -f "$BIN" ]; then
  # Don't remove the binary if it's the bundled one from a plugin checkout.
  # Heuristic: only remove if the binary's parent dir is .local/bin (typical install path)
  if [ -x "$BIN" ] && file "$BIN" 2>/dev/null | grep -q "ELF"; then
    # Check if anyone else references it (other MCP configs)
    if ! grep -rq "opc-mcp" "$HOME/.claude" "$HOME/.codex" "$HOME/.cursor" "$HOME/.openclaw" "$HOME/.codeium" 2>/dev/null; then
      log "Removing $BIN (no remaining references)..."
      rm -f "$BIN"
      log "  ✓ binary removed"
    else
      log "  (binary kept — other configs still reference it)"
    fi
  fi
fi

log "✅ Uninstall complete. Restart $AGENT to clear MCP cache."
