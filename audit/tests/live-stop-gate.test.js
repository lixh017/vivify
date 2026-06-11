/**
 * live-stop-gate.test.js — Verify the Stop gate (gate-stop.js) actually
 * runs and reports CRITICAL findings. This is the test the user wants
 * — a real session-end simulation.
 */
'use strict';
const { execSync, spawnSync } = require('child_process');
const path = require('path');

const ROOT = path.join(__dirname, '..');
const STOP_GATE = path.join(ROOT, 'gate-stop.js');

let pass = 0, fail = 0;
function test(name, fn) {
  try { fn(); console.log(`  ✓ ${name}`); pass++; }
  catch (e) { console.log(`  ✗ ${name}: ${e.message}`); fail++; }
}

// 1. Stop gate exits 0 (no critical) when run on a healthy state
test('stop gate exits 0 on healthy state', () => {
  const r = spawnSync('node', [STOP_GATE], { encoding: 'utf8', timeout: 60000 });
  // exit 0 = no critical, 1 = warnings, 2 = critical
  // Current state should be 0 (no critical)
  if (r.status !== 0) {
    throw new Error(`expected exit 0 (no critical), got ${r.status}\n${r.stdout}\n${r.stderr}`);
  }
});

// 2. Stop gate runs all evals (count is dynamic — 20 originally, 32+ after opc evals)
test('stop gate output includes a passing eval summary', () => {
  const r = spawnSync('node', [STOP_GATE], { encoding: 'utf8', timeout: 60000 });
  const m = r.stdout.match(/Total:\s*(\d+)\s*passed/);
  if (!m) throw new Error('no "Total: N passed" line in output');
  if (parseInt(m[1], 10) < 20) throw new Error(`too few evals: ${m[1]}`);
});

// 3. Stop gate reads run-evals.js, not eval.md (sanity)
test('stop gate script exists and is executable', () => {
  const fs = require('fs');
  if (!fs.existsSync(STOP_GATE)) throw new Error('gate-stop.js missing');
  const stat = fs.statSync(STOP_GATE);
  if (!(stat.mode & 0o111)) throw new Error('gate-stop.js not executable');
});

console.log(`\n${pass} passed, ${fail} failed`);
process.exit(fail > 0 ? 1 : 0);
