const { execSync } = require('child_process');
const path = require('path');
const fs = require('fs');
const ROOT = path.join(__dirname, '..');
const GATE = path.join(ROOT, 'gate-pretool.js');

let pass = 0, fail = 0;
function test(name, fn) {
  try { fn(); console.log(`  ✓ ${name}`); pass++; }
  catch (e) { console.log(`  ✗ ${name}: ${e.message}`); fail++; }
}

function callGate(stdin) {
  const r = execSync(`echo '${stdin}' | node ${GATE}`, { encoding: 'utf8' });
  return r;
}

// 1. unprotected file → exit 0
test('unprotected file → exit 0', () => {
  callGate('{"tool_name":"Write","tool_input":{"file_path":"/tmp/foo.txt"}}');
});

// 2. SKILL.md path → triggers skill-test
test('SKILL.md path triggers gate', () => {
  const f = '/root/.claude/skills/_test-tmp-skill/SKILL.md';
  fs.mkdirSync(path.dirname(f), { recursive: true });
  fs.writeFileSync(f, '---\nname: _test-tmp-skill\ndescription: Use this skill to test the gate against missing description. Just testing the gate.\n---\n# Test\n```bash\necho hi```');
  try {
    callGate(`{"tool_name":"Write","tool_input":{"file_path":"${f}"}}`);
  } finally {
    fs.rmSync(path.dirname(f), { recursive: true });
  }
});

// 3. settings.json with hardcoded token → exit 1
test('settings.json with hardcoded token → exit 1', () => {
  const f = '/tmp/gate-test/settings.json';
  fs.mkdirSync('/tmp/gate-test', { recursive: true });
  fs.writeFileSync(f, JSON.stringify({ env: { ANTHROPIC_AUTH_TOKEN: 'sk-aabbccddeeff00112233445566778899' } }));
  try {
    callGate(`{"tool_name":"Write","tool_input":{"file_path":"${f}"}}`);
    throw new Error('should have exited 1');
  } catch (e) {
    if (e.status !== 1) throw new Error('exit code should be 1');
  } finally {
    fs.rmSync('/tmp/gate-test', { recursive: true });
  }
});

console.log(`\n${pass} passed, ${fail} failed`);
process.exit(fail > 0 ? 1 : 0);
