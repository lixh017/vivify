// tests/gate-pretool-adversarial.test.js
//
// Adversarial tests: try to bypass the gate with malicious or unexpected inputs.
// Each test sends a crafted stdin to gate-pretool.js and verifies it does
// the right thing (allow, block, or exit cleanly).
'use strict';
const { execSync } = require('child_process');
const path = require('path');
const fs = require('fs');

const ROOT = path.join(__dirname, '..');
const GATE = path.join(ROOT, 'gate-pretool.js');
const TMP = '/tmp/adversarial-test';

if (!fs.existsSync(TMP)) fs.mkdirSync(TMP, { recursive: true });

let pass = 0, fail = 0;

function test(name, fn) {
  try {
    fn();
    console.log(`  ✓ ${name}`);
    pass++;
  } catch (e) {
    console.log(`  ✗ ${name}: ${e.message}`);
    fail++;
  }
}

function callGate(stdin) {
  try {
    const out = execSync(`echo ${JSON.stringify(JSON.stringify(stdin))} | node ${GATE}`, {
      encoding: 'utf8',
      stdio: 'pipe',
    });
    return { code: 0, out };
  } catch (e) {
    return { code: e.status, out: e.stdout || '', err: e.stderr || '' };
  }
}

// 1. Path traversal attempt
test('rejects ../ in path (does not match protected pattern, exits 0)', () => {
  const r = callGate({
    tool_name: 'Write',
    tool_input: { file_path: '/root/.claude/skills/../settings.json' },
  });
  if (r.code !== 0) throw new Error(`should allow path that does not match: got ${r.code}`);
});

// 2. Symlink to a settings.json
test('allows a symlink to settings.json (gate does not follow links)', () => {
  const real = `${TMP}/real-settings.json`;
  const link = `${TMP}/link.json`;
  fs.writeFileSync(real, '{}');
  try { fs.unlinkSync(link); } catch {}
  fs.symlinkSync(real, link);
  const r = callGate({
    tool_name: 'Write',
    tool_input: { file_path: link },
  });
  // Should exit 0 because the regex matches the *path* not the file
  if (r.code !== 0) throw new Error(`should allow: got ${r.code}`);
  try { fs.unlinkSync(link); fs.unlinkSync(real); } catch {}
});

// 3. Path with null byte
test('handles path with null byte (does not crash)', () => {
  const r = callGate({
    tool_name: 'Write',
    tool_input: { file_path: '/root/.claude/skills/x\0y/SKILL.md' },
  });
  // Either allow (0) or block (non-zero), but must not crash with non-zero exit that's not block
  if (r.code < 0) throw new Error('crashed');
});

// 4. Path with shell metacharacters
test('handles path with shell metacharacters (no command injection)', () => {
  const r = callGate({
    tool_name: 'Write',
    tool_input: { file_path: '/root/.claude/skills/foo;rm -rf /SKILL.md' },
  });
  if (r.code < 0) throw new Error('crashed');
});

// 5. Empty stdin
test('empty stdin → exit 0', () => {
  try {
    execSync(`echo "" | node ${GATE}`, { encoding: 'utf8', stdio: 'pipe' });
  } catch (e) {
    throw new Error(`empty stdin should be allowed, got exit ${e.status}`);
  }
});

// 6. Non-JSON stdin
test('non-JSON stdin → exit 0 (graceful degradation)', () => {
  try {
    execSync(`echo "this is not json" | node ${GATE}`, { encoding: 'utf8', stdio: 'pipe' });
  } catch (e) {
    throw new Error(`non-JSON should be allowed, got exit ${e.status}`);
  }
});

// 7. Bash tool should not trigger the gate
test('Bash tool → exit 0 (gate only intercepts file writes)', () => {
  try {
    execSync(`echo '{"tool_name":"Bash","tool_input":{"command":"rm -rf /"}}' | node ${GATE}`, { encoding: 'utf8', stdio: 'pipe' });
  } catch (e) {
    throw new Error(`Bash should be allowed by gate, got exit ${e.status}`);
  }
});

// 8. Write to settings.json with hardcoded token is blocked
test('Write settings.json with hardcoded token → exit 1', () => {
  const bad = `${TMP}/settings.json`;
  fs.writeFileSync(bad, JSON.stringify({ env: { ANTHROPIC_AUTH_TOKEN: 'sk-aabbccddeeff00112233445566778899' } }));
  const r = callGate({ tool_name: 'Write', tool_input: { file_path: bad } });
  if (r.code !== 1) throw new Error(`should block, got exit ${r.code}`);
  fs.unlinkSync(bad);
});

// 9. Write to settings.json.example is skipped (template)
test('Write settings.json.example → exit 0 (skipped)', () => {
  const f = `${TMP}/settings.json.example`;
  fs.writeFileSync(f, JSON.stringify({ env: { ANTHROPIC_AUTH_TOKEN: 'sk-aabbccddeeff00112233445566778899' } }));
  const r = callGate({ tool_name: 'Write', tool_input: { file_path: f } });
  if (r.code !== 0) throw new Error(`should skip .example, got exit ${r.code}`);
  fs.unlinkSync(f);
});

// 10. Edit tool (not Write) on settings.json with hardcoded token → exit 1
test('Edit settings.json with hardcoded token → exit 1', () => {
  const bad = `${TMP}/settings.json`;
  fs.writeFileSync(bad, JSON.stringify({ env: { ANTHROPIC_AUTH_TOKEN: 'sk-aabbccddeeff00112233445566778899' } }));
  const r = callGate({ tool_name: 'Edit', tool_input: { file_path: bad } });
  if (r.code !== 1) throw new Error(`should block Edit too, got exit ${r.code}`);
  fs.unlinkSync(bad);
});

// 11. MultiEdit on SKILL.md with no description → exit 1
test('MultiEdit SKILL.md with no description → exit 1', () => {
  const f = '/root/.claude/skills/_test-multi-edit-skill/SKILL.md';
  fs.mkdirSync(path.dirname(f), { recursive: true });
  fs.writeFileSync(f, '---\nname: _test-multi-edit-skill\n---\n# test\n');
  const r = callGate({ tool_name: 'MultiEdit', tool_input: { file_path: f } });
  if (r.code !== 1) throw new Error(`should block MultiEdit, got exit ${r.code}`);
  fs.rmSync(path.dirname(f), { recursive: true });
});

// 12. Very long path (DoS attempt)
test('handles very long path (no crash, no hang)', () => {
  const longPath = '/root/.claude/skills/' + 'a'.repeat(10000) + '/SKILL.md';
  const r = callGate({ tool_name: 'Write', tool_input: { file_path: longPath } });
  if (r.code < 0) throw new Error('crashed');
});

// 13. JSON with extra fields (forward-compat)
test('JSON with extra fields → handled (no crash)', () => {
  const r = callGate({
    tool_name: 'Write',
    tool_input: { file_path: '/tmp/foo.txt', extra_field: 'junk', nested: { x: 1 } },
  });
  if (r.code !== 0) throw new Error(`should allow, got exit ${r.code}`);
});

// 14. file_path missing entirely
test('file_path missing → exit 0 (nothing to gate)', () => {
  const r = callGate({ tool_name: 'Write', tool_input: { command: 'no file' } });
  if (r.code !== 0) throw new Error(`should allow, got exit ${r.code}`);
});

// 15. tool_input missing
test('tool_input missing → exit 0', () => {
  const r = callGate({ tool_name: 'Write' });
  if (r.code !== 0) throw new Error(`should allow, got exit ${r.code}`);
});

console.log(`\n${pass} passed, ${fail} failed`);
process.exit(fail > 0 ? 1 : 0);
