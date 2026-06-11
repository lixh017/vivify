#!/usr/bin/env node
/**
 * gate-pretool.js — Dispatcher for PreToolUse Write/Edit on protected files.
 *
 * Reads Claude Code's PreToolUse stdin JSON, inspects the file_path, and
 * routes to the right gate:
 *   - settings.json (and variants)  -> validate-settings.js
 *   - skills/<name>/SKILL.md        -> skill-test.js --skill <name>
 *   - hooks/hooks.json              -> run-evals.js --eval E18
 *
 * Exit 0 = allow. Non-zero = block.
 */
'use strict';

const fs = require('fs');
const path = require('path');
const { spawnSync } = require('child_process');

const AUDIT_DIR = '/root/workspace/opc/audit';

function runGate(scriptName, args) {
  const r = spawnSync('node', [path.join(AUDIT_DIR, scriptName), ...args], {
    encoding: 'utf8',
    timeout: 30000,
  });
  if (r.stdout) process.stderr.write(r.stdout);
  if (r.stderr) process.stderr.write(r.stderr);
  process.exit(r.status === null ? 1 : r.status);
}

let input = '';
process.stdin.setEncoding('utf8');
process.stdin.on('data', d => { input += d; });
process.stdin.on('end', () => {
  if (!input.trim()) process.exit(0);

  let payload;
  try { payload = JSON.parse(input); } catch { process.exit(0); }

  const toolName = payload?.tool_name || '';
  const ti = payload?.tool_input || {};
  const filePath = ti.file_path || ti.path || '';

  // Only intercept write-shaped tools
  if (!['Write', 'Edit', 'MultiEdit', 'NotebookEdit'].includes(toolName)) {
    process.exit(0);
  }
  if (!filePath) process.exit(0);

  // 1. settings.json*  (skip .example / .bak)
  if (/\/settings\.json(\.[a-zA-Z0-9_-]+)?$/.test(filePath)) {
    if (/\.example$/.test(filePath)) process.exit(0);
    if (/\.bak$/.test(filePath)) process.exit(0);
    return runGate('validate-settings.js', [filePath]);
  }

  // 2. SKILL.md under a skill directory
  const skillMatch = filePath.match(/\.claude\/skills\/([^/]+)\/SKILL\.md$/);
  if (skillMatch) {
    const skillName = skillMatch[1];
    return runGate('skill-test.js', ['--skill', skillName]);
  }

  // 3. hooks/hooks.json
  if (/\/hooks\/hooks\.json$/.test(filePath)) {
    return runGate('run-evals.js', ['--eval', 'E18']);
  }

  // not a protected path
  process.exit(0);
});
