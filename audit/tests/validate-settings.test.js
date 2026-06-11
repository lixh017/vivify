const { execSync } = require('child_process');
const path = require('path');
const fs = require('fs');
const ROOT = path.join(__dirname, '..');
const VALIDATOR = path.join(ROOT, 'validate-settings.js');

let pass = 0, fail = 0;
function test(name, fn) {
  try { fn(); console.log(`  ✓ ${name}`); pass++; }
  catch (e) { console.log(`  ✗ ${name}: ${e.message}`); fail++; }
}

const tmp = '/tmp/validate-settings-test';
fs.mkdirSync(tmp, { recursive: true });

// 1. CRITICAL: hardcoded token
test('flags hardcoded real token', () => {
  const f = `${tmp}/bad.json`;
  fs.writeFileSync(f, JSON.stringify({ env: { ANTHROPIC_AUTH_TOKEN: 'sk-aabbccddeeff00112233445566778899' } }));
  try {
    execSync(`node ${VALIDATOR} ${f}`, { encoding: 'utf8', stdio: 'pipe' });
    throw new Error('should have exited non-zero');
  } catch (e) {
    if (!e.stdout || !e.stdout.includes('CRITICAL')) throw new Error('no CRITICAL in output');
  }
});

// 2. env-var ref is clean
test('env-var reference passes', () => {
  const f = `${tmp}/good.json`;
  fs.writeFileSync(f, JSON.stringify({ env: { ANTHROPIC_AUTH_TOKEN: '${ANTHROPIC_AUTH_TOKEN}' } }));
  const r = execSync(`node ${VALIDATOR} ${f}`, { encoding: 'utf8' });
  if (r.includes('CRITICAL')) throw new Error('should not flag env ref');
});

// 3. JSON parse error
test('flags JSON parse error', () => {
  const f = `${tmp}/broken.json`;
  fs.writeFileSync(f, '{not valid json');
  try {
    execSync(`node ${VALIDATOR} ${f}`, { encoding: 'utf8', stdio: 'pipe' });
    throw new Error('should have exited non-zero');
  } catch (e) {
    if (!e.stdout || !e.stdout.includes('parse error')) throw new Error('no parse error in output');
  }
});

// cleanup
for (const f of fs.readdirSync(tmp)) fs.unlinkSync(path.join(tmp, f));
fs.rmdirSync(tmp);

console.log(`\n${pass} passed, ${fail} failed`);
process.exit(fail > 0 ? 1 : 0);
