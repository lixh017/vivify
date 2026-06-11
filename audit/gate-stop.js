#!/usr/bin/env node
/**
 * gate-stop.js — Session-end eval gate.
 *
 * Runs run-evals.js (CRITICAL only) on session end. If any CRITICAL
 * eval is failing, exit non-zero so Claude Code reports it to the
 * user.
 *
 * Why CRITICAL-only: low/medium evals are guidance; CRITICAL = block
 * the session. Don't flood the user with noise.
 */
'use strict';

const { spawnSync } = require('child_process');
const path = require('path');

const AUDIT_DIR = '/root/workspace/opc/audit';

const r = spawnSync('node', [path.join(AUDIT_DIR, 'run-evals.js'), '--all'], {
  encoding: 'utf8',
  timeout: 60000,
});
if (r.stdout) process.stdout.write(r.stdout);
if (r.stderr) process.stderr.write(r.stderr);
// Mirror run-evals.js exit code: 2 = critical, 1 = <90% pass, 0 = clean
process.exit(r.status === null ? 1 : r.status);
