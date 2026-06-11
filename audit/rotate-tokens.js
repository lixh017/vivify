#!/usr/bin/env node
/**
 * rotate-tokens.js — Replace hardcoded ANTHROPIC_AUTH_TOKEN in
 * settings.json* with env-var references. Validates each new write
 * with validate-settings.js; if validation fails, the write is
 * rolled back.
 *
 * Why this is the right tool: the gate only blocks new BAD writes
 * (it doesn't rewrite existing files). So the existing 5 hardcoded
 * tokens stay bad until something rewrites them. This script does
 * the rewrite, with the gate running on each new write.
 *
 * Usage:
 *   node rotate-tokens.js            # rotate all settings*.json
 *   node rotate-tokens.js <file>...  # rotate specific files
 *
 * Idempotent: re-running is a no-op if already env-referenced.
 */
'use strict';

const fs = require('fs');
const path = require('path');
const os = require('os');
const { spawnSync } = require('child_process');

const HOME = os.homedir();
const VALIDATOR = path.join(HOME, 'workspace/opc/audit/validate-settings.js');

const ENV_VAR_FOR = {
  ANTHROPIC_AUTH_TOKEN: '${ANTHROPIC_AUTH_TOKEN}',
  ANTHROPIC_BASE_URL: '${ANTHROPIC_BASE_URL:-https://api.anthropic.com}',
  ANTHROPIC_MODEL: '${ANTHROPIC_MODEL:-claude-opus-4-6}',
  ANTHROPIC_SMALL_FAST_MODEL: '${ANTHROPIC_SMALL_FAST_MODEL:-claude-haiku-4-5}',
  ANTHROPIC_DEFAULT_SONNET_MODEL: '${ANTHROPIC_DEFAULT_SONNET_MODEL:-claude-sonnet-4-6}',
  ANTHROPIC_DEFAULT_OPUS_MODEL: '${ANTHROPIC_DEFAULT_OPUS_MODEL:-claude-opus-4-6}',
  ANTHROPIC_DEFAULT_HAIKU_MODEL: '${ANTHROPIC_DEFAULT_HAIKU_MODEL:-claude-haiku-4-5}',
  CLAUDE_CODE_SUBAGENT_MODEL: '${CLAUDE_CODE_SUBAGENT_MODEL:-claude-haiku-4-5}',
  API_TIMEOUT_MS: '600000',
  ANTHROPIC_MAX_OUTPUT_TOKENS: '${ANTHROPIC_MAX_OUTPUT_TOKENS:-16384}',
  CLAUDE_CODE_MAX_OUTPUT_TOKENS: '${CLAUDE_CODE_MAX_OUTPUT_TOKENS:-16000}',
};

function looksLikeRealToken(k, v) {
  if (typeof v !== 'string') return false;
  if (!/TOKEN|KEY|SECRET/i.test(k)) return false;
  return /^(sk-[a-zA-Z0-9_-]{20,}|sk-cp-[a-zA-Z0-9_-]{20,}|tp-[a-zA-Z0-9]{20,}|pk-[a-zA-Z0-9-]{20,}|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$/i.test(v);
}

function looksLikeEnvRef(v) {
  return typeof v === 'string' && /\$\{[A-Z_]+\}/.test(v);
}

function processFile(file) {
  const text = fs.readFileSync(file, 'utf8');
  let data;
  try { data = JSON.parse(text); } catch (e) {
    return { ok: false, msg: `JSON parse: ${e.message}` };
  }

  let changes = 0;
  if (data.env && typeof data.env === 'object') {
    for (const k of Object.keys(data.env)) {
      const v = data.env[k];
      if (looksLikeRealToken(k, v) && !looksLikeEnvRef(v)) {
        const replacement = ENV_VAR_FOR[k] || `\${${k}}`;
        data.env[k] = replacement;
        changes++;
      } else if (k === 'API_TIMEOUT_MS' && parseInt(v, 10) > 600000) {
        data.env[k] = '600000';
        changes++;
      }
    }
  }

  if (changes === 0) {
    return { ok: true, msg: 'no changes needed', changes: 0 };
  }

  // Backup
  const bak = file + '.bak-' + new Date().toISOString().slice(0, 10).replace(/-/g, '');
  fs.writeFileSync(bak, text);

  // Write
  fs.writeFileSync(file, JSON.stringify(data, null, 2));

  // Validate
  const r = spawnSync('node', [VALIDATOR, file], { encoding: 'utf8' });
  if (r.status === 0 || (r.status === 1 && !r.stdout.match(/\[CRITICAL\]/))) {
    return { ok: true, msg: `rotated ${changes} entries; validator clean`, changes };
  }
  // rollback
  fs.writeFileSync(file, text);
  fs.unlinkSync(bak);
  return { ok: false, msg: `validation failed, rolled back: ${r.stdout.split('\n')[0]}` };
}

function listSettingsFiles() {
  const dir = path.join(HOME, '.claude');
  return fs.readdirSync(dir)
    .filter(f => f === 'settings.json' || /^settings\.json\.[a-zA-Z]+$/.test(f))
    .filter(f => !/\.(example|bak)/.test(f))
    .map(f => path.join(dir, f));
}

function main() {
  const targets = process.argv.slice(2);
  const files = targets.length ? targets : listSettingsFiles();
  let okCount = 0, failCount = 0;
  for (const f of files) {
    const r = processFile(f);
    if (r.ok) {
      okCount++;
      console.log(`✓ ${f.replace(HOME, '~')}: ${r.msg}`);
    } else {
      failCount++;
      console.log(`✗ ${f.replace(HOME, '~')}: ${r.msg}`);
    }
  }
  console.log(`\nTotal: ${okCount} rotated, ${failCount} failed.`);
  if (failCount > 0) process.exit(1);
}

if (require.main === module) main();
module.exports = { processFile, listSettingsFiles };
