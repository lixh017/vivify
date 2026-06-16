#!/usr/bin/env bash
# scripts/demo.sh — the one-shot demo showcase orchestrator.
#
# Boots the API + console with seeded data, runs the demo
# journey (login → credentials → /api/ai/topics → observability),
# and writes the screenshots into docs/showcase/.
#
# Run from repo root:   ./scripts/demo.sh
#
# After this script finishes, the demo artifacts in
# docs/showcase/ are reproducible: 01-login.png,
# 02-credentials.png, 03-topics.txt, 04-observability.png.
#
# The script tears down the API + console on exit so a
# subsequent api-smoke run starts from a clean port.

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

API_PORT=8084
CONSOLE_PORT=3001
DB_PATH="${DB_PATH:-/tmp/opc-demo.db}"
ENCRYPTION_KEY="${ENCRYPTION_KEY:-cGhhc2U0LXZlcmlmeS1lbmNyeXB0aW9uLTMyYnl0ZXM=}"

log()  { printf '\033[1;34m[demo]\033[0m %s\n' "$*"; }
err()  { printf '\033[1;31m[demo]\033[0m %s\n' "$*" >&2; }
ok()   { printf '\033[1;32m[demo]\033[0m %s\n' "$*"; }

cleanup() {
    local exit_code=$?
    pkill -9 -f "opc-api" 2>/dev/null || true
    pkill -9 -f "next dev"  2>/dev/null || true
    ./scripts/stop-server.sh "$API_PORT"    >/dev/null 2>&1 || true
    ./scripts/stop-server.sh "$CONSOLE_PORT" >/dev/null 2>&1 || true
    if [ "${KEEP_DB:-0}" != "1" ] && [ -f "$DB_PATH" ]; then
        rm -f "$DB_PATH" "${DB_PATH}-shm" "${DB_PATH}-wal"
    fi
    exit "$exit_code"
}
trap cleanup EXIT
trap 'exit 1' INT TERM

# ---- 1. build if needed
if [ ! -x "./apps/bin/opc-api" ]; then
    log "building API (one-time)…"
    make build >/tmp/opc-demo-build.log 2>&1 || { err "make build failed"; tail -20 /tmp/opc-demo-build.log >&2; exit 1; }
fi

# ---- 2. boot API
log "booting API on :$API_PORT (db=$DB_PATH)…"
rm -f "$DB_PATH"*
ENCRYPTION_KEY="$ENCRYPTION_KEY" \
OPC_INSECURE_COOKIES=1 \
REGISTRATION_ENABLED=1 \
MINIMAX_API_KEY="" \
PORT="$API_PORT" \
DB_PATH="$DB_PATH" \
nohup ./apps/bin/opc-api >/tmp/opc-demo-api.log 2>&1 &
disown
deadline=$((SECONDS + 30))
while [ "$SECONDS" -lt "$deadline" ]; do
    code=$(curl -sS -o /dev/null -w '%{http_code}' "http://127.0.0.1:$API_PORT/readyz" 2>/dev/null || true)
    [ "$code" = "200" ] && break
    sleep 1
done
[ "$code" = "200" ] || { err "API never came up"; tail -20 /tmp/opc-demo-api.log >&2; exit 1; }
ok "API up on :$API_PORT"

# ---- 3. register the 3 demo personas (real bcrypt hashes)
log "registering demo personas…"
for u in "panda-admin@e2e.local:panda-admin-pass:Panda Admin" \
         "creator-1@beta.com:creator-1-pass-7e2:Creator One" \
         "creator-2@beta.com:creator-2-pass-9b1:Creator Two"; do
    IFS=":" read email pw name <<< "$u"
    code=$(curl -sS -X POST -H 'Content-Type: application/json' \
        -d "{\"email\":\"$email\",\"password\":\"$pw\",\"name\":\"$name\"}" \
        -o /dev/null -w '%{http_code}' \
        "http://127.0.0.1:$API_PORT/api/auth/register" || true)
    log "  $email → $code"
done

# ---- 4. seed the call_log (1 week of realistic activity)
log "seeding 1 week of call_log activity…"
python3 scripts/demo-seed.py "$DB_PATH" 2>&1 | tail -3

# ---- 5. boot console
log "booting console on :$CONSOLE_PORT…"
cd "$REPO_ROOT/apps/console"
NEXT_PUBLIC_API_URL="http://127.0.0.1:$API_PORT" nohup pnpm dev >/tmp/opc-demo-console.log 2>&1 &
disown
cd "$REPO_ROOT"
deadline=$((SECONDS + 30))
while [ "$SECONDS" -lt "$deadline" ]; do
    code=$(curl -sS -o /dev/null -w '%{http_code}' "http://127.0.0.1:$CONSOLE_PORT/login" 2>/dev/null || true)
    [ "$code" = "200" ] && break
    sleep 1
done
[ "$code" = "200" ] || { err "console never came up"; tail -20 /tmp/opc-demo-console.log >&2; exit 1; }
ok "console up on :$CONSOLE_PORT"

# ---- 6. run the Playwright demo journey
log "running demo journey (4 captures)…"
python3 scripts/demo-journey.py 2>&1 | tail -10

# ---- 7. summary
echo
echo "=========================================="
ok "Demo artifacts written to docs/showcase/:"
ls -la docs/showcase/ | grep -E "^-" | awk '{printf "  %-32s  %8s bytes\n", $9, $5}'
echo
echo "Open docs/showcase/index.md to see the narrative."
echo "=========================================="
