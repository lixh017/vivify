#!/usr/bin/env bash
#
# scripts/healthcheck.sh — verify the OPC API is responding on /health.
#
# Designed to be run from cron, systemd timers, or any external monitor.
# Exits 0 with the response body on HTTP 200, exits 1 with an alert message
# on any other status, network error, or timeout.
#
# Environment overrides:
#   HEALTH_URL   Full URL to probe (default: http://localhost:8080/health)
#   TIMEOUT      curl timeout in seconds (default: 10)
#
set -euo pipefail

HEALTH_URL="${HEALTH_URL:-http://localhost:8080/health}"
TIMEOUT="${TIMEOUT:-10}"

log()  { printf '[healthcheck %s] %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"; }
die()  { printf '[healthcheck %s] ALERT: %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*" >&2; exit 1; }

# Capture the HTTP status code AND the body in a single request using
# `-w` so we can decide on success/failure based on the actual code.
RESPONSE="$(mktemp)"
trap 'rm -f "$RESPONSE"' EXIT

HTTP_CODE="$(curl --silent --show-error \
                  --max-time "$TIMEOUT" \
                  --output "$RESPONSE" \
                  --write-out '%{http_code}' \
                  "$HEALTH_URL" 2>/dev/null || echo "000")"

if [[ "$HTTP_CODE" != "200" ]]; then
  BODY="$(head -c 200 "$RESPONSE" 2>/dev/null || true)"
  die "Health check FAILED — $HEALTH_URL returned HTTP $HTTP_CODE${BODY:+ — body: $BODY}"
fi

# Lightweight body sanity check: the API's health handler returns
# {"status":"ok"}. Be tolerant of whitespace and pretty-printing.
if command -v grep >/dev/null 2>&1; then
  if ! grep -Eq '"status"[[:space:]]*:[[:space:]]*"ok"' "$RESPONSE"; then
    die "Health check FAILED — response body missing status=ok: $(cat "$RESPONSE")"
  fi
fi

log "OK — $HEALTH_URL returned 200"
cat "$RESPONSE"
printf '\n'
exit 0
