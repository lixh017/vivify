#!/usr/bin/env bash
# Build + upload opc-mcp binary to GitHub Releases
#
# Prerequisites:
#   - gh CLI installed and authenticated (gh auth login)
#   - this repo pushed to github.com/<owner>/<repo>
#   - opc-mcp Go source at apps/api/cmd/mcp-server/main.go (in the opc main repo)
#
# Usage:
#   ./scripts/release.sh v0.1.0
#   ./scripts/release.sh v0.1.0 /path/to/opc-mcp  # use pre-built binary
#
# What it does:
#   1. Builds opc-mcp from the opc main repo (or accepts a pre-built binary)
#   2. Uploads to GitHub Releases as opc-mcp (the binary the install script downloads)
#   3. Tags the release

set -euo pipefail

VERSION="${1:-}"
BINARY_PATH="${2:-}"

if [ -z "$VERSION" ]; then
  echo "Usage: $0 <version> [/path/to/pre-built-opc-mcp]"
  echo "  e.g. $0 v0.1.0"
  exit 1
fi

# --- 1. Get the binary ---
if [ -n "$BINARY_PATH" ] && [ -f "$BINARY_PATH" ]; then
  echo "Using pre-built binary: $BINARY_PATH"
  TMP_BIN="$BINARY_PATH"
else
  echo "Building opc-mcp from source..."
  # Try to find the opc main repo
  OPC_REPO="${OPC_REPO:-/root/workspace/opc}"
  if [ ! -d "$OPC_REPO/apps/api" ]; then
    echo "ERROR: opc main repo not found at $OPC_REPO"
    echo "  Set OPC_REPO=/path/to/opc or pass a pre-built binary as 2nd arg"
    exit 1
  fi

  cd "$OPC_REPO/apps/api"
  TMP_BIN="$(mktemp /tmp/opc-mcp-XXXXXX)"
  go build -tags fts5 -trimpath -ldflags="-s -w" -o "$TMP_BIN" ./cmd/mcp-server
  echo "Built: $TMP_BIN ($(du -h "$TMP_BIN" | cut -f1))"
fi

# --- 2. Identify the repo ---
REPO=$(gh repo view --json nameWithOwner -q .nameWithOwner 2>/dev/null) || {
  echo "ERROR: gh CLI not authenticated or not in a git repo"
  echo "  Run: gh auth login"
  exit 1
}
echo "Repo: $REPO"

# --- 3. Create release + upload binary ---
echo "Creating release $VERSION..."

# Use gh release create with the binary as an asset
gh release create "$VERSION" \
  --title "opc-panda-plugin $VERSION" \
  --notes "opc-mcp binary for opc-panda-plugin. See README.md for install instructions." \
  "$TMP_BIN#opc-mcp" \
  --repo "$REPO"

echo ""
echo "✅ Release $VERSION created!"
echo ""
echo "Install URL:"
echo "  https://github.com/$REPO/releases/latest/download/opc-mcp"
echo ""
echo "One-line install:"
echo "  curl -fsSL https://raw.githubusercontent.com/${REPO#*/}/main/scripts/install.sh | bash"
