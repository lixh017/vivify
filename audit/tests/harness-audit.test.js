const { execSync } = require('child_process');
const path = require('path');
const ROOT = path.join(__dirname, '..');
const ENGINE = path.join(ROOT, 'harness-audit.js');

let pass = 0, fail = 0;
function test(name, fn) {
  try { fn(); console.log(`  ✓ ${name}`); pass++; }
  catch (e) { console.log(`  ✗ ${name}: ${e.message}`); fail++; }
}

// 1. exits 0 on ~/.claude (above 80%)
test('runs on ~/.claude (should be >= 80% post-cleanup)', () => {
  const r = execSync(`node ${ENGINE} repo --root /root/.claude --format json`, { encoding: 'utf8' });
  const d = JSON.parse(r);
  if (d.overall_score < 56) throw new Error(`score too low: ${d.overall_score}/70`);
});

// 2. all 7 categories present
test('reports all 7 categories', () => {
  const r = execSync(`node ${ENGINE} repo --root /root/.claude --format json`, { encoding: 'utf8' });
  const d = JSON.parse(r);
  const names = d.categories.map(c => c.name);
  const required = ['Tool Coverage', 'Context Efficiency', 'Quality Gates', 'Memory Persistence', 'Eval Coverage', 'Security Guardrails', 'Cost Efficiency'];
  for (const n of required) if (!names.includes(n)) throw new Error(`missing: ${n}`);
});

// 3. scoped audit (hooks) returns narrower output
test('hooks scope returns smaller score', () => {
  const repoR = JSON.parse(execSync(`node ${ENGINE} repo --root /root/.claude --format json`, { encoding: 'utf8' }));
  const hooksR = JSON.parse(execSync(`node ${ENGINE} hooks --root /root/.claude --format json`, { encoding: 'utf8' }));
  if (hooksR.max_score >= repoR.max_score) throw new Error('hooks scope should have smaller max');
});

console.log(`\n${pass} passed, ${fail} failed`);
process.exit(fail > 0 ? 1 : 0);
