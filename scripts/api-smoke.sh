#!/usr/bin/env bash
#
# scripts/api-smoke.sh — boot a fresh API on :8081 against a throwaway
# SQLite DB, then run the 5-handler E2E regression the C4 batch verify
# used as its one-shot baseline.
#
# This is a regression guard, not a feature test: each call must return
# HTTP 200 and the response body must contain the fields the frontend
# consumes. We deliberately do NOT assert on field VALUES (those drift
# with the LLM); the contract is "200 + non-empty fields".
#
# Usage:
#   ./scripts/api-smoke.sh
#
# Environment overrides:
#   PORT       API port (default: 8081)
#   DB_PATH    SQLite path (default: /tmp/opc-smoke.db; removed on exit)
#   BASE_URL   Override the API base (default: http://127.0.0.1:$PORT)
#   TIMEOUT    /readyz wait budget in seconds (default: 30)
#   ENCRYPTION_KEY   Required. 32+ bytes after base64 decode.
#   SKIP_BUILD=1     Skip `make build` and reuse ./bin/opc-api
#   KEEP_DB=1        Leave the test DB on disk for inspection
#
# Exit codes:
#   0  — all 5 handler tests passed
#   1  — build failed, /readyz never came up, or a test failed
#
set -uo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
PORT="${PORT:-8081}"
DB_PATH="${DB_PATH:-/tmp/opc-smoke.db}"
BASE_URL="${BASE_URL:-http://127.0.0.1:${PORT}}"
TIMEOUT="${TIMEOUT:-30}"

# ENCRYPTION_KEY must decode to >= 32 bytes (see auth/crypto.go). A
# fixed 32-byte base64 value keeps the script self-contained; this is
# throwaway test data, not a real session, and the smoke user is
# registered fresh on each run. The literal is a base64 encoding of
# the 32-byte ASCII string "opc-smoke-test-encryption-key-32bytes".
export ENCRYPTION_KEY="${ENCRYPTION_KEY:-b3BjLXNtb2tlLXRlc3QtZW5jcnlwdGlvbi1rZXktMzJieXRlcw==}"
# OPC_INSECURE_COOKIES=1 lets the session cookie travel over plain
# HTTP. Required for any localhost /readyz + auth round-trip.
export OPC_INSECURE_COOKIES=1
# Tell the server which port + DB to use.
export PORT DB_PATH
# Open the registration gate so the first run can create the smoke
# user. See apps/api/internal/handlers/auth.go (registerEnabledEnv).
# Re-runs tolerate a 409 here and fall through to login, so leaving
# this on is safe for repeat invocations.
export REGISTRATION_ENABLED=1
# Empty API keys → handler demo paths. We do NOT want a real Claude
# bill from the smoke run; the contract under test is the wire shape,
# not the model output.
export ANTHROPIC_API_KEY=""
export DEEPSEEK_API_KEY=""
export GEMINI_API_KEY=""
export MINIMAX_API_KEY=""

# Fixed test identity so re-runs hit the 409-then-login path, not
# a fresh registration. Email is RFC-valid (G8 regression: a stray
# space in the email previously caused a 500).
SMOKE_EMAIL="api-smoke@e2e.local"
SMOKE_PASSWORD="api-smoke-pass-9b8e"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

# ---------------------------------------------------------------------------
# Logging helpers
# ---------------------------------------------------------------------------
log()  { printf '\033[1;34m[smoke]\033[0m %s\n' "$*"; }
ok()   { printf '\033[1;32m[smoke]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[smoke]\033[0m %s\n' "$*" >&2; }
err()  { printf '\033[1;31m[smoke]\033[0m %s\n' "$*" >&2; }

# truncate echoes the first $2 bytes of $1, suffixing with "…" if it
# actually trimmed. Keeps session tokens, prompts, and large script
# blobs out of CI logs.
truncate() {
    local body="$1"
    local limit="${2:-200}"
    if [ "${#body}" -le "$limit" ]; then
        printf '%s' "$body"
    else
        printf '%s…(truncated, %d bytes total)' "${body:0:$limit}" "${#body}"
    fi
}

# ---------------------------------------------------------------------------
# Zombie sweep + cleanup
# ---------------------------------------------------------------------------
# Belt-and-braces: stop-server.sh is the same helper `make stop-8081`
# uses, so we share the pid-detection logic with the rest of the
# project (G8 lesson: hand-rolled `ss | awk` previously killed the
# wrong process on a shared box).
./scripts/stop-server.sh "$PORT" >/dev/null 2>&1 || true

API_PID=""
cleanup() {
    local exit_code=$?
    if [ -n "$API_PID" ] && kill -0 "$API_PID" 2>/dev/null; then
        kill "$API_PID" 2>/dev/null || true
        # Give it a moment to release the port; then SIGKILL if it
        # is somehow still hanging on a stuck request.
        sleep 1
        kill -0 "$API_PID" 2>/dev/null && kill -9 "$API_PID" 2>/dev/null || true
    fi
    # Final safety net: in case the binary forked or we lost the
    # pid (e.g. the wait built-in's job control misbehaved), make
    # absolutely sure :PORT is free for the next run.
    ./scripts/stop-server.sh "$PORT" >/dev/null 2>&1 || true
    if [ "${KEEP_DB:-0}" != "1" ] && [ -f "$DB_PATH" ]; then
        rm -f "$DB_PATH" "${DB_PATH}-shm" "${DB_PATH}-wal"
    fi
    if [ "$exit_code" -ne 0 ]; then
        err "exiting with status $exit_code"
    fi
    exit "$exit_code"
}
trap cleanup EXIT
trap 'exit 1' INT TERM

# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------
if [ "${SKIP_BUILD:-0}" = "1" ] && [ -x "./apps/bin/opc-api" ]; then
    log "reusing ./apps/bin/opc-api (SKIP_BUILD=1)"
else
    log "building API (this is the slow part on first run)…"
    if ! make build >/tmp/opc-smoke-build.log 2>&1; then
        err "make build failed; tail of build log:"
        tail -30 /tmp/opc-smoke-build.log >&2
        exit 1
    fi
fi
# The Makefile `build` target writes to apps/bin/opc-api (the `..` in
# `-o ../bin/opc-api` is relative to apps/api). Mirror that path here
# so `make smoke-api` and `./scripts/api-smoke.sh` agree on the
# binary location.
[ -x "./apps/bin/opc-api" ] || { err "./apps/bin/opc-api missing after build"; exit 1; }

# ---------------------------------------------------------------------------
# Boot
# ---------------------------------------------------------------------------
log "starting API on :$PORT (db=$DB_PATH)…"
# nohup + & so the server survives any quirky terminal-job-control
# interaction in CI. stdout/stderr captured for post-mortem.
nohup ./apps/bin/opc-api >/tmp/opc-smoke-api.log 2>&1 &
API_PID=$!
log "API pid=$API_PID"

# ---------------------------------------------------------------------------
# /readyz poll
# ---------------------------------------------------------------------------
ready=0
deadline=$((SECONDS + TIMEOUT))
while [ "$SECONDS" -lt "$deadline" ]; do
    code=$(curl -sS -o /dev/null -w '%{http_code}' "$BASE_URL/readyz" 2>/dev/null || true)
    if [ "$code" = "200" ]; then
        ready=1
        break
    fi
    # If the process already exited we can fail fast instead of
    # burning the full 30s budget.
    if ! kill -0 "$API_PID" 2>/dev/null; then
        err "API process died during boot; log tail:"
        tail -40 /tmp/opc-smoke-api.log >&2
        exit 1
    fi
    sleep 1
done
if [ "$ready" -ne 1 ]; then
    err "/readyz did not return 200 within ${TIMEOUT}s; log tail:"
    tail -40 /tmp/opc-smoke-api.log >&2
    exit 1
fi
ok "/readyz up"

# ---------------------------------------------------------------------------
# Auth — register (tolerate 409) then login
# ---------------------------------------------------------------------------
COOKIE_JAR="/tmp/opc-smoke-cookies.txt"
rm -f "$COOKIE_JAR"

# We use --max-time to keep a wedged server from hanging the script
# forever, and --silent --show-error to keep the body available for
# parsing while not duplicating the progress log.
call() {
    # $1=method  $2=path  $3=json-body (may be "")  → echoes "<HTTP_CODE>\n<body>"
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

register_resp=$(call POST /api/auth/register \
    "$(printf '{"email":"%s","password":"%s","name":"%s"}' "$SMOKE_EMAIL" "$SMOKE_PASSWORD" "Smoke Test")")
register_code=$(printf '%s' "$register_resp" | sed -n 's/^__HTTP__//p' | tail -1)
register_body=$(printf '%s' "$register_resp" | sed 's/__HTTP__[0-9]*$//')

if [ "$register_code" = "201" ] || [ "$register_code" = "200" ]; then
    ok "registered smoke user"
elif [ "$register_code" = "409" ] || [ "$register_code" = "400" ]; then
    log "register returned $register_code (re-run); falling back to login"
else
    warn "register returned $register_code: $(truncate "$register_body" 120)"
    # Don't abort yet — login may still succeed if the user actually exists.
fi

login_resp=$(call POST /api/auth/login \
    "$(printf '{"email":"%s","password":"%s"}' "$SMOKE_EMAIL" "$SMOKE_PASSWORD")")
login_code=$(printf '%s' "$login_resp" | sed -n 's/^__HTTP__//p' | tail -1)
login_body=$(printf '%s' "$login_resp" | sed 's/__HTTP__[0-9]*$//')

if [ "$login_code" != "200" ]; then
    err "login failed: HTTP $login_code body=$(truncate "$login_body" 200)"
    exit 1
fi
ok "logged in as $SMOKE_EMAIL"

# ---------------------------------------------------------------------------
# 5-handler E2E
# ---------------------------------------------------------------------------
# Each test prints a single PASS/FAIL line so a CI log stays scannable.
# We accumulate pass/fail counts and the name of the failing handler
# in case the final summary needs to call out a partial.
PASS_COUNT=0
FAIL_COUNT=0
FAIL_LIST=()

run_test() {
    # $1=label  $2=method  $3=path  $4=body  $5=grep-pattern  (jq optional)
    local label="$1" method="$2" path="$3" body="$4" pattern="$5"
    local resp code payload
    resp=$(call "$method" "$path" "$body")
    code=$(printf '%s' "$resp" | sed -n 's/^__HTTP__//p' | tail -1)
    payload=$(printf '%s' "$resp" | sed 's/__HTTP__[0-9]*$//')

    if [ "$code" != "200" ]; then
        printf '  [FAIL] %-20s HTTP %s  body=%s\n' "$label" "$code" "$(truncate "$payload" 160)"
        FAIL_COUNT=$((FAIL_COUNT+1))
        FAIL_LIST+=("$label")
        return 1
    fi
    if ! printf '%s' "$payload" | grep -qE "$pattern"; then
        printf '  [FAIL] %-20s 200 but missing /%s/  body=%s\n' "$label" "$pattern" "$(truncate "$payload" 160)"
        FAIL_COUNT=$((FAIL_COUNT+1))
        FAIL_LIST+=("$label")
        return 1
    fi
    printf '  [PASS] %-20s 200  body=%s\n' "$label" "$(truncate "$payload" 120)"
    PASS_COUNT=$((PASS_COUNT+1))
    return 0
}

log "running 5 handler tests…"

# (a) cover — title-only body. Image is generated via MiniMax image
# path; demo mode still returns a non-empty image_url because the
# handler short-circuits to a placeholder when no API key is set.
run_test "cover" POST /api/ai/cover \
    '{"title":"测试封面"}' \
    '"image_url"'

# (b) humanize — short script. Demo path returns a canned sentence
# of length matching the input, so "humanized" is always present.
run_test "humanize" POST /api/ai/humanize \
    '{"script":"这是一个用于烟雾测试的短脚本。"}' \
    '"humanized"'

# (c) score — needs all three fields. Demo path returns the
# rule-based quality score.
run_test "score" POST /api/ai/score \
    '{"title":"测试封面","script":"这是一个用于烟雾测试的短脚本。","platform":"抖音"}' \
    '"overall_score"'

# (d) postmortem — first create a content item to point at, then
# call postmortem with the new id. The handler returns 404 if the
# item does not exist, so this dance is the contract.
ci_resp=$(call POST /api/content-items \
    '{"script_id":0,"platform":"抖音"}')
ci_code=$(printf '%s' "$ci_resp" | sed -n 's/^__HTTP__//p' | tail -1)
ci_body=$(printf '%s' "$ci_resp" | sed 's/__HTTP__[0-9]*$//')
if [ "$ci_code" != "201" ] && [ "$ci_code" != "200" ]; then
    printf '  [FAIL] %-20s HTTP %s  body=%s\n' "content-items(create)" "$ci_code" "$(truncate "$ci_body" 160)"
    # The content-items call is a setup step for postmortem, not a
    # standalone test in the 5-handler spec. We track its failure as
    # a prerequisite skip rather than inflating FAIL_COUNT, but we
    # still surface it in the per-test log so CI operators can see
    # why postmortem did not run.
    CI_ID=0
    CI_SETUP_FAILED=1
else
    # Parse the id from the JSON response. Tolerant of key order.
    CI_ID=$(printf '%s' "$ci_body" | sed -n 's/.*"id"[[:space:]]*:[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -1)
    CI_ID="${CI_ID:-0}"
    printf '  [ ok ] %-20s %s  id=%s (setup for postmortem)\n' "content-items(create)" "$ci_code" "$CI_ID"
fi

if [ "$CI_ID" -gt 0 ]; then
    run_test "postmortem" POST /api/ai/postmortem \
        "$(printf '{"content_item_id":%s}' "$CI_ID")" \
        '"report"'
else
    # Without a content-item id the postmortem call is meaningless
    # and would 400/404. If the create step itself failed, attribute
    # the skip to the setup failure so the per-test summary is
    # truthful.
    if [ "${CI_SETUP_FAILED:-0}" = "1" ]; then
        printf '  [FAIL] %-20s skipped: content-items create failed\n' "postmortem"
    else
        printf '  [FAIL] %-20s skipped: no content item id\n' "postmortem"
    fi
    FAIL_COUNT=$((FAIL_COUNT+1))
    FAIL_LIST+=("postmortem")
fi

# (e) pipeline — multi-step. We pin to 4 steps so the test is
# deterministic; empty steps would invoke the default which is the
# same set, but explicit is better than implicit here.
run_test "pipeline" POST /api/ai/pipeline \
    '{"seed":"测试封面","platform":"抖音","steps":["topics","script","score","adapt"]}' \
    '"topics"'

# ---------------------------------------------------------------------------
# assetgen: 4 IP type consistency check (Phase 2 sub-spec A)
# ---------------------------------------------------------------------------
# The first 5 cases above exercise the HTTP handler surface; this
# block exercises the brand-on asset pipeline. Each case uses a
# hand-crafted prompt that hits all anchors (score >= 0.99) plus an
# advisory case proving a zero-score prompt is still allowed through
# (not blocked by the gate, just scored). See docs/assetgen/profile-schema.md
# for the anchor breakdown.
section() { log "$*"; }
pass()    { ok "$*"; PASS_COUNT=$((PASS_COUNT+1)); }
fail()    { err "$*"; FAIL_COUNT=$((FAIL_COUNT+1)); FAIL_LIST+=("$*"); }

# opc_asset_check runs the CLI and trims to the JSON tail so the
# trailing score line is what awk/grep operate on. --type is the
# IP-type flag (4 enum); the asset pipeline's --asset-type is a
# different flag and is not relevant to consistency scoring.
opc_asset_check() {
    local type="$1" prompt="$2"
    ./apps/bin/opc-asset check --type "$type" --prompt "$prompt" 2>&1 | tail -8
}

# assert_score_ge parses the "score" field out of the JSON output
# and compares against a floor. We do not need full jq — the JSON
# is hand-formatted by the CLI on exactly one line, so a grep on
# the "score" key is robust. Decimal and integer scores are both
# accepted (1, 1.0, 0.99 all PASS).
assert_score_ge() {
    local out="$1" want="$2" name="$3"
    local got_score
    got_score=$(echo "$out" | grep -oE '"score"[[:space:]]*:[[:space:]]*[0-9.]+' | grep -oE '[0-9.]+' | head -1)
    if [ -z "$got_score" ]; then
        fail "$name: no score found in output"
        return 1
    fi
    if awk "BEGIN{exit !($got_score >= $want)}"; then
        pass "$name: score=$got_score >= $want"
    else
        fail "$name: score=$got_score < $want"
    fi
}

section "assetgen profile types (Phase 2 sub-spec A)"

out=$(opc_asset_check "anthropomorphic" "峰哥, 成年熊猫, rgb(245,240,225), rgb(26,26,26), 国潮, 不露爪, 不要 AI 生成感")
assert_score_ge "$out" "0.99" "anthropomorphic check"

out=$(opc_asset_check "digital_human" "莉娜, 28 岁, female, 东亚, rgb(245,228,210), 温柔知性, 普通话, 表情自然, 无恐怖谷")
assert_score_ge "$out" "0.99" "digital_human check"

out=$(opc_asset_check "costume" "纤云, 唐代, 侠女, 襦裙, 长剑, 无穿越, 朱红#C73E1D")
assert_score_ge "$out" "0.99" "costume check"

out=$(opc_asset_check "info" "OPC 每日资讯, 演播室双主播, 主播阿橙, 黑体加粗, 暖色字幕, 宝蓝#1F5FA8, rgb(245,240,225), 无 AI 播报感")
assert_score_ge "$out" "0.99" "info check"

# Advisory: zero-score prompt still works (not blocked). The gate
# reports a low score but the CLI exits 0 — the intent is to flag
# to the operator, not to refuse. This guards against future
# regressions that would short-circuit the check.
out=$(opc_asset_check "anthropomorphic" "完全无关的文本")
got_score=$(echo "$out" | grep -oE '"score"[[:space:]]*:[[:space:]]*[0-9.]+' | grep -oE '[0-9.]+' | head -1)
if [ "$got_score" = "0" ] || [ "$got_score" = "0.0" ] || [ "$got_score" = "0.00" ]; then
    pass "advisory: zero-score prompt returns 0 score (not blocked)"
else
    fail "advisory: zero-score prompt got score=$got_score, want 0"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
# Expected total: 5 handler tests + 4 IP type checks + 1 advisory = 10.
TOTAL=$((PASS_COUNT + FAIL_COUNT))
echo
echo "========================================"
if [ "$FAIL_COUNT" -eq 0 ] && [ "$TOTAL" = "10" ]; then
    ok "RESULT: ${PASS_COUNT}/${TOTAL} PASS"
    echo "========================================"
    exit 0
fi

if [ "$FAIL_COUNT" -eq 0 ]; then
    warn "RESULT: ${PASS_COUNT}/${TOTAL} PASS (expected 10 tests: 5 handler + 4 IP type + 1 advisory)"
    echo "========================================"
    exit 0
fi

# Build a comma-separated list of failing test names for the summary.
joined=$(IFS=', '; echo "${FAIL_LIST[*]}")
err "RESULT: ${PASS_COUNT}/${TOTAL} PARTIAL  (failed: $joined)"
echo "========================================"
exit 1
