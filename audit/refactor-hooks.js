#!/usr/bin/env node
/**
 * refactor-hooks.js — Replace 26 inline `node -e "..."` blocks in
 * hooks.json with calls to ~/.claude/scripts/hooks/dispatcher.js.
 *
 * Idempotent: detects already-refactored commands and skips them.
 *
 * Usage: node refactor-hooks.js [--dry-run]
 */
'use strict';

const fs = require('fs');
const path = require('path');

const HOOKS = '/root/.claude/hooks/hooks.json';
const DISPATCHER = '/root/.claude/scripts/hooks/dispatcher.js';
const DRY = process.argv.includes('--dry-run');

function extractLastScriptArg(cmd) {
  // Match `node scripts/hooks/<name>.js` at the end (before any closing quote)
  // Allow the script path to be quoted or unquoted
  const m = cmd.match(/node\s+([^\s"']+\.js)/g);
  if (!m) return null;
  // The last one is the actual hook name; earlier "node" was inside -e
  const last = m[m.length - 1].replace(/^node\s+/, '');
  return last;
}

function extractStopArgs(cmd) {
  // The Stop hooks do spawnSync(execPath, [script, '<id>', '<rel>', '<flags>'], ...)
  // We want the args AFTER script.
  // Look for `spawnSync(process.execPath,[script,'<id>','<rel>','<flags>']`
  // and capture the args.
  const m = cmd.match(/spawnSync\([^,]+,\s*\[([^\]]+)\]/);
  if (!m) return null;
  // split on commas at top level, handling quotes
  const argStr = m[1];
  const args = [];
  let cur = '', inQ = false, q = '';
  for (let i = 0; i < argStr.length; i++) {
    const c = argStr[i];
    if (inQ) {
      if (c === q) { inQ = false; }
      else cur += c;
    } else if (c === "'" || c === '"') {
      inQ = true; q = c;
    } else if (c === ',') {
      args.push(cur.trim()); cur = '';
    } else {
      cur += c;
    }
  }
  if (cur.trim()) args.push(cur.trim());
  // First arg is `script` (variable, not literal), skip it
  return args.slice(1);
}

function isAlreadyRefactored(cmd) {
  return cmd.includes('hooks/dispatcher.js');
}

function refactorEntry(entry) {
  let changed = 0, skipped = 0;
  for (const h of entry.hooks || []) {
    const cmd = h.command || '';
    if (isAlreadyRefactored(cmd)) { skipped++; continue; }
    if (!cmd.includes('node -e')) { skipped++; continue; }

    const newCmd = tryRefactor(cmd);
    if (newCmd) {
      h.command = newCmd;
      changed++;
    } else {
      skipped++;
    }
  }
  return { changed, skipped };
}

function tryRefactor(cmd) {
  // Try Stop pattern first (it has spawnSync with args)
  const stopArgs = extractStopArgs(cmd);
  if (stopArgs && stopArgs.length >= 3) {
    return `node ${DISPATCHER} run-with-flags.js ${stopArgs.map(a => `'${a}'`).join(' ')}`;
  }

  // Shell hook pattern: ends with `shell <rel-script> <id> <rel> <flags>`
  // (the inline `node -e` is a plugin-root resolver; actual invocation is a shell script)
  if (cmd.includes('" shell ')) {
    const m = cmd.match(/"\s+shell\s+([^\s"']+\.sh)\s+(\S+)\s+([^\s"']+)\s+([^\s"']+)/);
    if (m) {
      const [, script, id, rel, flags] = m;
      return `bash /root/.claude/scripts/hooks/dispatcher.sh '${id}' '${rel}' '${flags}'`;
    }
  }

  // Simple case: last `node scripts/hooks/<name>.js`
  const lastScript = extractLastScriptArg(cmd);
  if (lastScript) {
    return `node ${DISPATCHER} ${path.basename(lastScript)}`;
  }

  return null;
}

function main() {
  const data = JSON.parse(fs.readFileSync(HOOKS, 'utf8'));
  let totalChanged = 0, totalSkipped = 0, totalAlready = 0;

  for (const [event, entries] of Object.entries(data.hooks || {})) {
    for (const entry of entries) {
      const { changed, skipped } = refactorEntry(entry);
      totalChanged += changed;
      totalSkipped += skipped;
    }
  }

  if (DRY) {
    console.log(`Would change: ${totalChanged}`);
    console.log(`Skipped (already refactored or not inline): ${totalSkipped}`);
    return;
  }

  fs.writeFileSync(HOOKS, JSON.stringify(data, null, 2));
  console.log(`Refactored: ${totalChanged} inline commands replaced with dispatcher calls`);
  console.log(`Skipped:    ${totalSkipped}`);

  // Verify
  const verify = JSON.parse(fs.readFileSync(HOOKS, 'utf8'));
  let remaining = 0;
  for (const [, entries] of Object.entries(verify.hooks || {})) {
    for (const e of entries) {
      for (const h of e.hooks || []) {
        if ((h.command || '').includes('node -e')) remaining++;
      }
    }
  }
  console.log(`Remaining inline 'node -e' commands: ${remaining}`);
}

if (require.main === module) main();
