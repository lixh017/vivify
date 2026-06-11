/**
 * tests/runner.js — Minimal test runner for the audit scripts.
 * Run with: node tests/runner.js
 */
'use strict';
const fs = require('fs');
const path = require('path');
const { execSync, spawnSync } = require('child_process');

const TEST_FILES = fs.readdirSync(__dirname)
  .filter(f => f.endsWith('.test.js') && f !== 'runner.js');

let pass = 0, fail = 0;
const failures = [];

for (const tf of TEST_FILES) {
  console.log(`\n=== ${tf} ===`);
  const r = spawnSync('node', [path.join(__dirname, tf)], { encoding: 'utf8' });
  process.stdout.write(r.stdout);
  if (r.status === 0) {
    pass++;
  } else {
    fail++;
    failures.push({ file: tf, stderr: r.stderr, code: r.status });
  }
}

console.log(`\n${'='.repeat(40)}`);
console.log(`Tests: ${pass} files passed, ${fail} files failed`);
if (fail > 0) {
  console.log('\nFailures:');
  for (const f of failures) {
    console.log(`  ${f.file}: exit ${f.code}`);
    if (f.stderr) console.log(`    stderr: ${f.stderr.split('\n')[0]}`);
  }
  process.exit(1);
}
