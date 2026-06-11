const { execSync } = require('child_process');
const path = require('path');
const ROOT = path.join(__dirname, '..');

let pass = 0, fail = 0;
function test(name, fn) {
  try { fn(); console.log(`  ✓ ${name}`); pass++; }
  catch (e) { console.log(`  ✗ ${name}: ${e.message}`); fail++; }
}

// 1. exits 0 on valid path
test('runs without crashing on ~/.claude/skills', () => {
  const r = execSync(`node ${path.join(ROOT, 'skill-quality.js')} --root ~/.claude/skills --top 1`, { encoding: 'utf8' });
  if (!r.includes('Skill Quality Report')) throw new Error('no report header');
});

// 2. exits 0 with no skills (empty dir)
test('handles empty skills dir', () => {
  const tmp = '/tmp/empty-skills-test';
  require('fs').mkdirSync(tmp, { recursive: true });
  try {
    const r = execSync(`node ${path.join(ROOT, 'skill-quality.js')} --root ${tmp}`, { encoding: 'utf8' });
    if (!r.includes('Evaluated: 0')) throw new Error('did not report 0 count');
  } finally {
    require('fs').rmdirSync(tmp);
  }
});

// 3. handles nonexistent dir
test('handles nonexistent dir gracefully', () => {
  const r = execSync(`node ${path.join(ROOT, 'skill-quality.js')} --root /nonexistent-xyz`, { encoding: 'utf8' });
  if (!r.includes('Evaluated: 0')) throw new Error('should report 0 for missing dir');
});

console.log(`\n${pass} passed, ${fail} failed`);
process.exit(fail > 0 ? 1 : 0);
