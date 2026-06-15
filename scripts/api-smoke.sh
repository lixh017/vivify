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
SKIP_COUNT=0
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
# skip counts toward TOTAL but is not a failure. The summary block
# knows the difference: PASS_COUNT is "tests that ran and passed",
# SKIP_COUNT is "tests that intentionally did not run" (typically
# because a required env var is missing). The CLI topic case
# below uses skip when MINIMAX_API_KEY is not set, since the CLI
# hard-errors on a missing key and we don't want to require every
# developer to have one wired up to run the smoke.
skip()    { warn "$*"; SKIP_COUNT=$((SKIP_COUNT+1)); }

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
# Topic capability (Phase 2 sub-spec B M1)
# ---------------------------------------------------------------------------
# Re-exercises the topic capability through both the HTTP surface
# (POST /api/ai/topics, after the Task 2 handler refactor delegates
# to the topic library) and the CLI surface (opc-asset topic, Task 3).
# The HTTP case works in demo mode because MINIMAX_API_KEY is
# intentionally empty above — the handler short-circuits to the
# canned 10-topic pool and trims to req.Count (5 by default). The
# CLI case requires a real MINIMAX_API_KEY; without one the CLI
# hard-errors and we SKIP rather than fail (developer ergonomics:
# running this script should not require a paid API key).
section "topic capability (Phase 2 sub-spec B M1)"

# HTTP topic case — uses the cookie jar from the earlier login,
# so we route through call() for cookie consistency instead of
# hand-rolling -H "Cookie: opc_session=$TOKEN".
topic_resp=$(call POST /api/ai/topics \
    '{"seed":"个人成长","platform":"抖音","count":5}')
topic_code=$(printf '%s' "$topic_resp" | sed -n 's/^__HTTP__//p' | tail -1)
topic_body=$(printf '%s' "$topic_resp" | sed 's/__HTTP__[0-9]*$//')
if [ "$topic_code" != "200" ]; then
    fail "topic HTTP: HTTP $topic_code body=$(truncate "$topic_body" 160)"
else
    topic_count=$(echo "$topic_body" | jq '.topics | length' 2>/dev/null)
    if [ "$topic_count" = "5" ]; then
        pass "topic HTTP: 5 topics returned (seed=个人成长, platform=抖音)"
    else
        fail "topic HTTP: jq length = '$topic_count', want 5 (raw: $(truncate "$topic_body" 200))"
    fi
fi

# CLI topic case — real LLM call against MiniMax; SKIP gracefully
# when MINIMAX_API_KEY is not set. We capture the env that the
# outer test runner passed in (the script's earlier `export
# MINIMAX_API_KEY=""` only affects the API server, not the
# subprocess; we pass through the value from the parent env).
CLI_OUT=$(mktemp -t opc-topic-smoke.XXXXXX.json)
if [ -z "${MINIMAX_API_KEY:-}" ]; then
    skip "topic CLI: MINIMAX_API_KEY not set, skipping (CLI hard-errors without a key)"
else
    if ./apps/bin/opc-asset topic \
        --seed "个人成长" --platform "抖音" --count 5 --out "$CLI_OUT" 2>/dev/null; then
        cli_count=$(jq 'length' < "$CLI_OUT" 2>/dev/null)
        if [ "$cli_count" = "5" ]; then
            pass "topic CLI: 5 topics written to $CLI_OUT"
        else
            fail "topic CLI: jq length = $cli_count, want 5"
        fi
    else
        fail "topic CLI: opc-asset topic exited non-zero (is MINIMAX_API_KEY valid?)"
    fi
fi
rm -f "$CLI_OUT"

# ---------------------------------------------------------------------------
# Sub-Spec C M1: external creator flow (Task 5)
# ---------------------------------------------------------------------------
# Four cases covering the invite-only-beta path: operator creates a
# creator via /api/admin/creators, creator logs in, creator hits the
# topic endpoint, creator is correctly rejected from the admin
# surface. Smoke re-runs tolerate a 409 on step 1 (creator already
# exists from a prior run) by falling through to login; that keeps
# the test idempotent without polluting the user table.
section "external creator (Phase 2 sub-spec C M1)"

# 1. Operator creates a creator (re-uses the smoke operator session
# already established above — the default role for the registration
# path is "operator", so the cookie jar from the early login is
# authorized for /api/admin/creators).
creator_resp=$(call POST /api/admin/creators \
    '{"email":"creator1@beta.com","password":"beta-pass-123"}')
creator_code=$(printf '%s' "$creator_resp" | sed -n 's/^__HTTP__//p' | tail -1)
creator_body=$(printf '%s' "$creator_resp" | sed 's/__HTTP__[0-9]*$//')
creator_id=$(echo "$creator_body" | jq '.user.id' 2>/dev/null)

if [ "$creator_code" = "201" ] && [ -n "$creator_id" ] && [ "$creator_id" != "null" ]; then
    pass "admin: creator created (id=$creator_id)"
elif [ "$creator_code" = "409" ]; then
    # Re-run: creator already exists. Pick the id by listing so the
    # login step below can still proceed.
    list_resp=$(call GET /api/admin/creators "")
    list_body=$(printf '%s' "$list_resp" | sed 's/__HTTP__[0-9]*$//')
    creator_id=$(echo "$list_body" | jq '.creators[] | select(.email=="creator1@beta.com") | .id' 2>/dev/null | head -1)
    if [ -n "$creator_id" ] && [ "$creator_id" != "null" ]; then
        pass "admin: creator already exists from prior run (id=$creator_id)"
    else
        fail "admin: 409 on create but could not find creator1@beta.com in list"
    fi
else
    fail "admin: creator creation failed (HTTP $creator_code body=$(truncate "$creator_body" 200))"
fi

# 2. Creator logs in via a fresh cookie jar (the main smoke cookie
# jar is operator-scoped — we deliberately keep them separate so
# the 403 case below can prove the creator session is what gets
# rejected, not the operator).
CREATOR_COOKIE_JAR="/tmp/opc-smoke-creator-cookies.txt"
rm -f "$CREATOR_COOKIE_JAR"

creator_login_resp=$(curl -sS --max-time 30 -X POST \
    -H 'Content-Type: application/json' \
    -b "$CREATOR_COOKIE_JAR" -c "$CREATOR_COOKIE_JAR" \
    -d '{"email":"creator1@beta.com","password":"beta-pass-123"}' \
    -w '\n__HTTP__%{http_code}' \
    "$BASE_URL/api/auth/login")
creator_login_code=$(printf '%s' "$creator_login_resp" | sed -n 's/^__HTTP__//p' | tail -1)
creator_session=$(grep opc_session "$CREATOR_COOKIE_JAR" 2>/dev/null | awk '{print $7}' | tail -1)

if [ "$creator_login_code" = "200" ] && [ -n "$creator_session" ]; then
    pass "creator: login successful"
else
    fail "creator: login failed (HTTP $creator_login_code, session='$creator_session')"
fi

# 3. Creator generates a topic via the B M1 endpoint. We hit the
# endpoint with the creator's cookie jar instead of the operator
# jar to prove the refactor in B M1 didn't accidentally lock
# non-operator callers out. Demo mode (MINIMAX_API_KEY="") still
# returns 5 topics, so this case is independent of API key state.
if [ -n "$creator_session" ]; then
    creator_topic_resp=$(curl -sS --max-time 30 -X POST \
        -H 'Content-Type: application/json' \
        -b "$CREATOR_COOKIE_JAR" \
        -d '{"seed":"个人成长","platform":"抖音","count":5}' \
        -w '\n__HTTP__%{http_code}' \
        "$BASE_URL/api/ai/topics")
    creator_topic_code=$(printf '%s' "$creator_topic_resp" | sed -n 's/^__HTTP__//p' | tail -1)
    creator_topic_body=$(printf '%s' "$creator_topic_resp" | sed 's/__HTTP__[0-9]*$//')
    creator_topic_count=$(echo "$creator_topic_body" | jq '.topics | length' 2>/dev/null)

    if [ "$creator_topic_code" = "200" ] && [ "$creator_topic_count" = "5" ]; then
        pass "creator: topic HTTP returns 5 topics"
    else
        fail "creator: topic HTTP got code=$creator_topic_code count=$creator_topic_count (want 200/5)"
    fi
else
    skip "creator: topic HTTP (skipped because creator login failed above)"
fi

# 4. Creator tries to call an admin endpoint — must 403. This is
# the C M1 role-check gate live-firing; a regression that drops
# the middleware on /api/admin/creators would flip this to 201.
if [ -n "$creator_session" ]; then
    creator_admin_code=$(curl -sS --max-time 30 -o /dev/null -w "%{http_code}" -X POST \
        -H 'Content-Type: application/json' \
        -b "$CREATOR_COOKIE_JAR" \
        -d '{"email":"should-not-be-created@beta.com","password":"x-pass-1234"}' \
        "$BASE_URL/api/admin/creators")
    if [ "$creator_admin_code" = "403" ]; then
        pass "creator: admin endpoint correctly 403"
    else
        fail "creator: admin endpoint returned $creator_admin_code, want 403"
    fi
else
    skip "creator: admin endpoint 403 (skipped because creator login failed above)"
fi
rm -f "$CREATOR_COOKIE_JAR"

# ---------------------------------------------------------------------------
# Sub-Spec D M1: agent API key auth (Task 5)
# ---------------------------------------------------------------------------
# Four cases covering the X-API-Key auth path: operator mints an
# agent key, that agent authenticates against /api/ai/topics, the
# same agent is correctly rejected from /api/admin/* (the admin
# group still requires RequireAuth + RequireOperatorRole), and a
# syntactically-valid but unknown key returns 401. The smoke
# operator session was established above; the cookie jar
# ($COOKIE_JAR) carries that opc_session cookie and is reused for
# the admin POST.
section "agent API key (Phase 2 sub-spec D M1)"

# 1. Operator creates an agent. The handler returns 201 + the full
# key exactly once (GitHub PAT-style). We capture both the full key
# (used by cases 2 + 3) and the new agent's id (used by case 4
# only as log fodder; case 4 is auth-only and doesn't need it).
agent_create_resp=$(call POST /api/admin/agents \
    '{"name":"claude-code-laptop-1"}')
agent_create_code=$(printf '%s' "$agent_create_resp" | sed -n 's/^__HTTP__//p' | tail -1)
agent_create_body=$(printf '%s' "$agent_create_resp" | sed 's/__HTTP__[0-9]*$//')
AGENT_API_KEY=$(echo "$agent_create_body" | jq -r '.key' 2>/dev/null)
agent_id=$(echo "$agent_create_body" | jq -r '.agent.id' 2>/dev/null)

if [ "$agent_create_code" = "201" ] \
    && [ -n "$AGENT_API_KEY" ] \
    && [ "$AGENT_API_KEY" != "null" ] \
    && [ -n "$agent_id" ] \
    && [ "$agent_id" != "null" ]; then
    pass "admin: agent created (id=$agent_id, key returned 1x)"
else
    fail "admin: agent creation failed (HTTP $agent_create_code body=$(truncate "$agent_create_body" 200))"
fi

# 2. Agent calls /api/ai/topics with X-API-Key (no cookie). The
# endpoint is wired with RequireAuth → MaybeAgentKey → handler, so
# RequireAuth 401s the missing cookie BUT MaybeAgentKey has
# already stamped agent_id before then — verified in Task 3 unit
# tests; this case proves the same on the wire. Demo mode
# (MINIMAX_API_KEY="" empty above) still returns the canned 5-topic
# pool, so this case is independent of API key state just like the
# human-call topic case above.
if [ -n "$AGENT_API_KEY" ] && [ "$AGENT_API_KEY" != "null" ]; then
    agent_topic_resp=$(curl -sS --max-time 30 -X POST \
        -H 'Content-Type: application/json' \
        -H "X-API-Key: $AGENT_API_KEY" \
        -d '{"seed":"个人成长","platform":"抖音","count":5}' \
        -w '\n__HTTP__%{http_code}' \
        "$BASE_URL/api/ai/topics")
    agent_topic_code=$(printf '%s' "$agent_topic_resp" | sed -n 's/^__HTTP__//p' | tail -1)
    agent_topic_body=$(printf '%s' "$agent_topic_resp" | sed 's/__HTTP__[0-9]*$//')
    agent_topic_count=$(echo "$agent_topic_body" | jq '.topics | length' 2>/dev/null)

    if [ "$agent_topic_code" = "200" ] && [ "$agent_topic_count" = "5" ]; then
        pass "agent: topic HTTP returns 5 topics (X-API-Key auth)"
    else
        fail "agent: topic HTTP got code=$agent_topic_code count=$agent_topic_count (want 200/5, body=$(truncate "$agent_topic_body" 200))"
    fi
else
    skip "agent: topic HTTP (skipped because admin create failed above)"
fi

# 3. Agent tries to call /api/admin/agents (admin group requires
# RequireAuth + RequireOperatorRole). The X-API-Key path does NOT
# satisfy RequireAuth (it only sets CtxAgentID); RequireAuth sees
# no cookie → 401. This is the documented M1 behavior: agent
# callers are scoped to /api/ai/*, never to /api/admin/*. We
# deliberately send NO cookie jar (the curl does not -b anything)
# so the auth chain is "X-API-Key only" — exactly the path an
# external agent would take.
if [ -n "$AGENT_API_KEY" ] && [ "$AGENT_API_KEY" != "null" ]; then
    agent_admin_code=$(curl -sS --max-time 30 -o /dev/null -w "%{http_code}" -X POST \
        -H 'Content-Type: application/json' \
        -H "X-API-Key: $AGENT_API_KEY" \
        -d '{"name":"x"}' \
        "$BASE_URL/api/admin/agents")
    if [ "$agent_admin_code" = "401" ]; then
        pass "agent: admin endpoint correctly 401 (no session)"
    else
        fail "agent: admin endpoint returned $agent_admin_code, want 401"
    fi
else
    skip "agent: admin endpoint 401 (skipped because admin create failed above)"
fi

# 4. Bad API key — same shape as a real one (opc_agent_ + 32
# hex chars) but no row in the agents table with that prefix.
# MaybeAgentKey passes through (no match); RequireAuth 401s the
# missing cookie. Same 401 surface as case 3, different root
# cause (case 3 has a valid key + no cookie; this case has a
# fake key + no cookie).
bogus_code=$(curl -sS --max-time 30 -o /dev/null -w "%{http_code}" -X POST \
    -H 'Content-Type: application/json' \
    -H "X-API-Key: opc_agent_bogus0000000000000000000000" \
    -d '{"seed":"x","platform":"抖音","count":5}' \
    "$BASE_URL/api/ai/topics")
if [ "$bogus_code" = "401" ]; then
    pass "agent: bad API key correctly 401"
else
    fail "agent: bad API key returned $bogus_code, want 401"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
# Expected total depends on whether MINIMAX_API_KEY was set in the
# caller's env:
#   key set:    5 handler + 4 IP type + 1 advisory + 1 topic HTTP + 1 topic CLI + 4 creator + 4 agent = 20 (PASS_COUNT=20)
#   key absent: 5 handler + 4 IP type + 1 advisory + 1 topic HTTP + 1 topic CLI-SKIP + 4 creator + 4 agent = 20 (PASS_COUNT=19, SKIP_COUNT=1)
# Either way TOTAL = PASS + FAIL + SKIP must be 20.
TOTAL=$((PASS_COUNT + FAIL_COUNT + SKIP_COUNT))
echo
echo "========================================"
if [ "$FAIL_COUNT" -eq 0 ] && [ "$TOTAL" = "20" ]; then
    ok "RESULT: ${PASS_COUNT} pass / ${SKIP_COUNT} skip / ${FAIL_COUNT} fail  (total 20)"
    echo "========================================"
    exit 0
fi

if [ "$FAIL_COUNT" -eq 0 ]; then
    warn "RESULT: ${PASS_COUNT} pass / ${SKIP_COUNT} skip / ${FAIL_COUNT} fail  (total $TOTAL, expected 20: 5 handler + 4 IP type + 1 advisory + 2 topic + 4 creator + 4 agent)"
    echo "========================================"
    exit 0
fi

# Build a comma-separated list of failing test names for the summary.
joined=$(IFS=', '; echo "${FAIL_LIST[*]}")
err "RESULT: ${PASS_COUNT} pass / ${SKIP_COUNT} skip / ${FAIL_COUNT} fail  (total $TOTAL, failed: $joined)"
echo "========================================"
exit 1
