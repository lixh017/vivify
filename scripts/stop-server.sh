#!/usr/bin/env bash
# stop-server.sh — kill the opc API server actual listener binary
# (not the `go run` wrapper). Used by smoke tests and contributors
# who need to free up :PORT.
#
# Usage: ./scripts/stop-server.sh [PORT]
#        PORT defaults to 8080.

set -euo pipefail

PORT="${1:-8080}"

# Trailing space matters: ":8080 " matches :8080 only, not :80800 or
# :80801 etc. (G8 was filed because a previous version used a
# multi-port regex and accidentally killed an unrelated process on
# :8081.)
LISTENER=$(ss -ltnp 2>/dev/null | grep ":${PORT} " | grep -oE 'pid=[0-9]+' | head -1 | cut -d= -f2 || true)

if [ -z "$LISTENER" ]; then
    echo "no listener on :${PORT}"
    exit 0
fi

kill "$LISTENER" 2>/dev/null
echo "killed :${PORT} pid=${LISTENER}"
