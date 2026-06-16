#!/usr/bin/env bash
#
# scripts/verify-phase4-provider.sh — Phase 4 acceptance test:
# verify that a configured user credential results in a
# call_log row stamped with the resolved provider name
# ("anthropic" or "openai"), not the legacy "minimax" or
# the middleware default "internal".
#
# This is the one observable contract Phase 4 ships against
# the audit dashboard — the resolver's job is to land the
# request on the right protocol adapter AND surface the
# decision in the call_log row.
#
# Procedure:
#   1. Boot a throwaway API on :8082 (so we don't collide
#      with anything else the developer has running).
#   2. Register a smoke user + log in.
#   3. Create one anthropic credential + one openai
#      credential, both pointed at an invalid BaseURL so
#      the live call fails fast with a network error —
#      we do NOT want this script to bill anyone.
#   4. POST /api/ai/topics twice. The call will return 5xx
#      (upstream unreachable) but the call_log middleware
#      runs first and the resolver's per-request stamp
#      runs inside resolveTextForRequest — so the row will
#      be there with the right provider.
#   5. Query call_logs and assert provider matches the
#      expected protocol name.
#
# Exit codes:
#   0 — provider stamping verified
#   1 — boot, login, credential, or assertion failed
#
set -uo pipefail

PORT="${PORT:-8082}"
DB_PATH="${DB_PATH:-/tmp/opc-phase4-verify.db}"
BASE_URL="${BASE_URL:-http://127.0.0.1:${PORT}}"
TIMEOUT="${TIMEOUT:-30}"
KEEP_DB="${KEEP_DB:-1}" # default: leave DB for inspection

# 32-byte ASCII key: "phase4-verify-encryption-32bytes" (base64 below).
# The default in api-smoke.sh was 36 bytes (one over AES-256's
# 32-byte requirement), which made Encrypt() return
# "crypto/aes: invalid key size 37" any time the smoke path
# tried to insert a credential. Fixed here so the Phase 4
# verification can round-trip through /api/credentials.
export ENCRYPTION_KEY="${ENCRYPTION_KEY:-cGhhc2U0LXZlcmlmeS1lbmNyeXB0aW9uLTMyYnl0ZXM=}"
export OPC_INSECURE_COOKIES=1
export REGISTRATION_ENABLED=1
# Empty upstream keys so the default MiniMax stays in
# Available()==false and the topic route hits the live
# path (where the resolver is consulted, which is the
# whole point of this test).
export ANTHROPIC_API_KEY=""
export DEEPSEEK_API_KEY=""
export GEMINI_API_KEY=""
export MINIMAX_API_KEY=""

# 5s per-request timeout is fine because the BaseURL is
# invalid (test.not-resolvable.invalid) — the resolver
# returns the provider, the call attempts the URL, the
# DNS lookup fails almost instantly, and the handler
# returns 5xx with the call_log row already stamped.
export PORT DB_PATH

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

log()  { printf '\033[1;34m[verify]\033[0m %s\n' "$*"; }
ok()   { printf '\033[1;32m[verify]\033[0m %s\n' "$*"; }
err()  { printf '\033[1;31m[verify]\033[0m %s\n' "$*" >&2; }
warn() { printf '\033[1;33m[verify]\033[0m %s\n' "$*" >&2; }

SMOKE_EMAIL="phase4-verify@e2e.local"
SMOKE_PASSWORD="phase4-verify-pass-3f7c"

# stop any leftover server
./scripts/stop-server.sh "$PORT" >/dev/null 2>&1 || true

API_PID=""
cleanup() {
    local exit_code=$?
    if [ -n "$API_PID" ] && kill -0 "$API_PID" 2>/dev/null; then
        kill "$API_PID" 2>/dev/null || true
        sleep 1
        kill -0 "$API_PID" 2>/dev/null && kill -9 "$API_PID" 2>/dev/null || true
    fi
    ./scripts/stop-server.sh "$PORT" >/dev/null 2>&1 || true
    if [ "$KEEP_DB" != "1" ] && [ -f "$DB_PATH" ]; then
        rm -f "$DB_PATH" "${DB_PATH}-shm" "${DB_PATH}-wal"
    fi
    exit "$exit_code"
}
trap cleanup EXIT
trap 'exit 1' INT TERM

# 1. boot
if [ ! -x "./apps/bin/opc-api" ]; then
    log "building API…"
    make build >/tmp/opc-phase4-build.log 2>&1 || { err "make build failed"; tail -20 /tmp/opc-phase4-build.log >&2; exit 1; }
fi
log "starting API on :$PORT (db=$DB_PATH)…"
nohup ./apps/bin/opc-api >/tmp/opc-phase4-api.log 2>&1 &
API_PID=$!

# poll /readyz
ready=0
deadline=$((SECONDS + TIMEOUT))
while [ "$SECONDS" -lt "$deadline" ]; do
    code=$(curl -sS -o /dev/null -w '%{http_code}' "$BASE_URL/readyz" 2>/dev/null || true)
    if [ "$code" = "200" ]; then ready=1; break; fi
    if ! kill -0 "$API_PID" 2>/dev/null; then
        err "API died during boot; tail of log:"
        tail -40 /tmp/opc-phase4-api.log >&2
        exit 1
    fi
    sleep 1
done
[ "$ready" = "1" ] || { err "/readyz never came up; tail of log:"; tail -40 /tmp/opc-phase4-api.log >&2; exit 1; }
ok "/readyz up on :$PORT"

# 2. register + login
COOKIE_JAR="/tmp/opc-phase4-cookies.txt"
rm -f "$COOKIE_JAR"

call() {
    local method="$1" path="$2" body="${3:-}"
    if [ -n "$body" ]; then
        curl -sS --max-time 30 -X "$method" \
            -H 'Content-Type: application/json' \
            -b "$COOKIE_JAR" -c "$COOKIE_JAR" \
            -d "$body" \
            -w '\n__HTTP__%{http_code}' \
            "$BASE_URL$path"
    else
        curl -sS --max-time 30 -X "$method" \
            -b "$COOKIE_JAR" -c "$COOKIE_JAR" \
            -w '\n__HTTP__%{http_code}' \
            "$BASE_URL$path"
    fi
}

extract_code() { printf '%s' "$1" | sed -n 's/^__HTTP__//p' | tail -1; }
extract_body() { printf '%s' "$1" | sed 's/__HTTP__[0-9]*$//'; }

reg_resp=$(call POST /api/auth/register \
    "$(printf '{"email":"%s","password":"%s","name":"Phase4 Verify"}' "$SMOKE_EMAIL" "$SMOKE_PASSWORD")")
reg_code=$(extract_code "$reg_resp")
if [ "$reg_code" = "201" ] || [ "$reg_code" = "200" ]; then
    ok "registered"
elif [ "$reg_code" = "409" ]; then
    log "register returned 409 (re-run); falling through to login"
else
    err "register failed: HTTP $reg_code body=$(extract_body "$reg_resp" | head -c 200)"
    exit 1
fi

login_resp=$(call POST /api/auth/login \
    "$(printf '{"email":"%s","password":"%s"}' "$SMOKE_EMAIL" "$SMOKE_PASSWORD")")
login_code=$(extract_code "$login_resp")
[ "$login_code" = "200" ] || { err "login failed: HTTP $login_code"; exit 1; }
ok "logged in"

# 3. create credentials
create_cred() {
    # $1=provider  $2=protocol  $3=base_url  $4=model_name  $5=name
    local resp
    resp=$(call POST /api/credentials \
        "$(printf '{"provider":"%s","protocol":"%s","base_url":"%s","model_name":"%s","name":"%s","plaintext_key":"sk-fake-test-key-1234567890","scope":"all"}' \
            "$1" "$2" "$3" "$4" "$5")")
    local code body
    code=$(extract_code "$resp")
    body=$(extract_body "$resp")
    if [ "$code" = "201" ] || [ "$code" = "200" ]; then
        ok "created $1 credential (id=$(echo "$body" | jq -r '.id' 2>/dev/null))"
        return 0
    else
        err "create $1 credential failed: HTTP $code body=$body"
        return 1
    fi
}

create_cred "anthropic" "anthropic" "https://test.not-resolvable.invalid" "claude-test" "phase4-anthropic" || exit 1
create_cred "openai" "openai" "https://test.not-resolvable.invalid" "gpt-test" "phase4-openai" || exit 1

# 4. call /api/ai/topics
# Expected: 5xx (upstream unreachable), but the call_log row
# is stamped with the resolved provider name.
# Pick which credential the resolver should use: send a
# second credential to force the lookup to scan and pick —
# actually, the resolver picks the "all"-scoped credential
# newer-first, so the openai one wins (created last). We
# don't actually care which wins — we care that the row's
# provider column is one of {anthropic, openai}, not
# {minimax, internal, echo}.
topic_resp=$(call POST /api/ai/topics \
    '{"seed":"测试","platform":"抖音","count":3}')
topic_code=$(extract_code "$topic_resp")
topic_body=$(extract_body "$topic_resp")
log "topic call returned HTTP $topic_code (expected 5xx — upstream unreachable by design)"
log "  body: $(echo "$topic_body" | head -c 200)"

# 5. query call_logs
if [ ! -f "$DB_PATH" ]; then
    err "DB not found at $DB_PATH"
    exit 1
fi

log "call_log rows for user (newest first):"
sqlite3 -header -column "$DB_PATH" \
    "SELECT id, user_id, provider, skill, status, latency_ms, created_at
     FROM call_logs
     ORDER BY id DESC
     LIMIT 5"

provider_value=$(sqlite3 "$DB_PATH" \
    "SELECT provider FROM call_logs ORDER BY id DESC LIMIT 1;")

if [ -z "$provider_value" ]; then
    err "no call_log row was written — middleware missing on /api/ai/topics"
    exit 1
fi

case "$provider_value" in
    anthropic|openai)
        ok "✓ call_log.provider = '$provider_value' (resolver-aware)"
        echo
        echo "════════════════════════════════════════════════"
        ok "PHASE 4 ACCEPTANCE: provider stamping verified end-to-end"
        echo "════════════════════════════════════════════════"
        echo
        echo "  call_log row → provider = $provider_value"
        echo "  This is the audit signal the spec requires: when a"
        echo "  user configures an anthropic / openai credential, the"
        echo "  resolver picks the right protocol adapter AND the row"
        echo "  surfaces the decision in the observability dashboard."
        exit 0
        ;;
    minimax|internal|echo)
        err "✗ call_log.provider = '$provider_value' (regression — Phase 4 resolver not consulted)"
        err "  expected 'anthropic' or 'openai'"
        exit 1
        ;;
    *)
        err "✗ call_log.provider = '$provider_value' (unexpected)"
        err "  expected 'anthropic' or 'openai'"
        exit 1
        ;;
esac
