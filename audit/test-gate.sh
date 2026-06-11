#!/usr/bin/env bash
# test-gate.sh — exercise the 4 gates with synthetic tool inputs.
set -u

GATE="node /root/workspace/opc/audit/gate-pretool.js"
PASS=0
FAIL=0

run_test() {
  local name="$1"
  local expected="$2"  # "block" or "allow"
  local stdin="$3"

  local actual
  echo "$stdin" | $GATE > /tmp/gate-out 2>&1
  local code=$?

  if [[ "$expected" == "block" ]]; then
    if [[ $code -ne 0 ]]; then
      echo "  ✓ $name  (blocked, exit=$code)"
      PASS=$((PASS+1))
    else
      echo "  ✗ $name  (EXPECTED block, got allow)"
      cat /tmp/gate-out
      FAIL=$((FAIL+1))
    fi
  else
    if [[ $code -eq 0 ]]; then
      echo "  ✓ $name  (allowed)"
      PASS=$((PASS+1))
    else
      echo "  ✗ $name  (EXPECTED allow, got block exit=$code)"
      cat /tmp/gate-out
      FAIL=$((FAIL+1))
    fi
  fi
}

echo "=== Gate tests ==="

# 1. hardcoded token in settings.json — should BLOCK
mkdir -p /tmp/gate-test
cat > /tmp/gate-test/settings.json <<'JSON'
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "sk-aabbccddeeff00112233445566778899"
  }
}
JSON
run_test "hardcoded token in settings.json" "block" \
  '{"tool_name":"Write","tool_input":{"file_path":"/tmp/gate-test/settings.json"}}'

# 2. env-var ref in settings.json — should ALLOW
cat > /tmp/gate-test/settings.json <<'JSON'
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "${ANTHROPIC_AUTH_TOKEN}"
  },
  "permissions": { "allow": ["Read"] }
}
JSON
run_test "env-var reference in settings.json" "allow" \
  '{"tool_name":"Write","tool_input":{"file_path":"/tmp/gate-test/settings.json"}}'

# 3. SKILL.md with no description — should BLOCK
mkdir -p /root/.claude/skills/_gate-test
cat > /root/.claude/skills/_gate-test/SKILL.md <<'MD'
---
name: _gate-test
---
# Body
content
MD
run_test "SKILL.md with no description" "block" \
  '{"tool_name":"Write","tool_input":{"file_path":"/root/.claude/skills/_gate-test/SKILL.md"}}'

# 4. SKILL.md with TODO — should BLOCK
cat > /root/.claude/skills/_gate-test/SKILL.md <<'MD'
---
name: _gate-test
description: Use this skill to test the gate against TODO placeholders in shipped bodies. It exists only to exercise the no-todo rule.
---
# Body
TODO: fix
MD
run_test "SKILL.md with TODO placeholder" "block" \
  '{"tool_name":"Write","tool_input":{"file_path":"/root/.claude/skills/_gate-test/SKILL.md"}}'

# 5. Clean SKILL.md — should ALLOW
cat > /root/.claude/skills/_gate-test/SKILL.md <<'MD'
---
name: _gate-test
description: Use this skill to test the gate passing a clean SKILL.md file with proper frontmatter. It exists only to exercise the happy path.
---
# Body

```bash
echo hi
```
MD
run_test "clean SKILL.md" "allow" \
  '{"tool_name":"Write","tool_input":{"file_path":"/root/.claude/skills/_gate-test/SKILL.md"}}'

# 6. unprotected file — should ALLOW
run_test "unprotected file path" "allow" \
  '{"tool_name":"Write","tool_input":{"file_path":"/tmp/foo.txt"}}'

# 7. settings.json with .example suffix — should ALLOW (skipped)
cat > /tmp/gate-test/settings.json.example <<'JSON'
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "sk-aabbccddeeff00112233445566778899"
  }
}
JSON
run_test "settings.json.example (skipped)" "allow" \
  '{"tool_name":"Write","tool_input":{"file_path":"/tmp/gate-test/settings.json.example"}}'

# cleanup
rm -rf /root/.claude/skills/_gate-test /tmp/gate-test

echo ""
echo "=== Result: $PASS passed, $FAIL failed ==="
[[ $FAIL -eq 0 ]] && exit 0 || exit 1
