#!/usr/bin/env bash
# audit.sh — One command runs all 5 frameworks. The canonical entry point.
#
# Usage:
#   ./audit.sh                 # full audit (everything)
#   ./audit.sh quick           # just gates + evals (skip the heavy quality scoring)
#   ./audit.sh pre-commit      # what to run before any commit
#   ./audit.sh score           # just the harness-audit scorecard
#   ./audit.sh evals           # just the 20-eval baseline
#   ./audit.sh gates           # 7-test gate smoke
#   ./audit.sh tests           # unit tests
#   ./audit.sh tokens          # validate-settings
#
# Exit codes:
#   0 = everything green
#   1 = warnings or non-critical failures
#   2 = critical failure
set -uo pipefail

cd "$(dirname "$0")"
ROOT="$(pwd)"
HOME_DIR="${HOME}"
export PATH="$PATH"

PASS=0
FAIL=0
CRIT=0

run() {
  local name="$1"; shift
  echo
  echo "=== $name ==="
  if "$@"; then
    echo "  ✓ $name"
    PASS=$((PASS+1))
  else
    local rc=$?
    if [[ $rc -eq 2 ]]; then
      echo "  ✗ $name  (CRITICAL, exit $rc)"
      CRIT=$((CRIT+1))
    else
      echo "  ✗ $name  (exit $rc)"
      FAIL=$((FAIL+1))
    fi
  fi
}

score()    { node "$ROOT/harness-audit.js" repo --root "$HOME_DIR/.claude"; }
evals()    { node "$ROOT/run-evals.js" --all; }
gates()    { bash "$ROOT/test-gate.sh"; }
tests()    { node "$ROOT/tests/runner.js"; }
tokens()   { node "$ROOT/validate-settings.js"; }
quality()  { node "$ROOT/skill-quality.js" --root "$HOME_DIR/.claude/skills" --top 5 > /dev/null && echo "  (form scores captured)"; }
behaviour() { node "$ROOT/skill-test.js" > /dev/null 2>&1; }

case "${1:-all}" in
  all)      run "harness-audit" score
           run "eval suite"   evals
           run "gate smoke"   gates
           run "unit tests"   tests
           run "settings"     tokens
           ;;
  quick)    run "eval suite"   evals
           run "gate smoke"   gates
           run "settings"     tokens
           ;;
  pre-commit) run "eval suite" evals
              run "gate smoke" gates
              run "unit tests" tests
              ;;
  score)    score ;;
  evals)    evals ;;
  gates)    gates ;;
  tests)    tests ;;
  tokens)   tokens ;;
  quality)  quality ;;
  behaviour) behaviour ;;
  *)        echo "Usage: $0 {all|quick|pre-commit|score|evals|gates|tests|tokens|quality|behaviour}"
           exit 1 ;;
esac

echo
echo "============================================="
echo "  Summary: $PASS passed, $FAIL failed, $CRIT critical"
echo "============================================="

if [[ $CRIT -gt 0 ]]; then exit 2; fi
if [[ $FAIL -gt 0 ]]; then exit 1; fi
exit 0
