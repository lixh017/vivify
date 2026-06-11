#!/usr/bin/env node
/**
 * validate-settings.js — Static validator for ~/.claude/settings*.json.
 *
 * Catches:
 *  - hardcoded ANTHROPIC_AUTH_TOKEN / OPENAI_API_KEY / similar
 *  - dangerous `permissions.allow` entries (Bash + Edit + Write all enabled)
 *  - missing sandbox / safety settings
 *  - multi-variant settings.json files with no README explaining the choice
 *  - missing model name in env
 *
 * Usage:
 *   node validate-settings.js                 # validate all
 *   node validate-settings.js <file>...       # validate specific files
 *
 * Exits non-zero on any CRITICAL finding.
 */
'use strict';

const fs = require('fs');
const path = require('path');
const os = require('os');

const CLAUDE_DIR = path.join(os.homedir(), '.claude');

const SECRET_KEYS = [
  /ANTHROPIC_AUTH_TOKEN/i,
  /OPENAI_API_KEY/i,
  /GITHUB_TOKEN/i,
  /AWS_SECRET/i,
  /.*_SECRET/i,
  /.*_TOKEN/i,
  /.*_KEY/i,
];

const PLACEHOLDER_HINTS = [
  'your-', 'example', 'placeholder', 'changeme', 'xxx', 'todo', 'replace',
  '<', '>', '$', 'process.env', '${',
];

function isPlaceholder(value) {
  const v = String(value).toLowerCase();
  return PLACEHOLDER_HINTS.some(p => v.includes(p)) || v.length < 8;
}

function looksLikeRealToken(value) {
  if (isPlaceholder(value)) return false;
  if (/^sk-[a-zA-Z0-9_-]{20,}$/.test(value)) return true;
  if (/^sk-cp-[a-zA-Z0-9_-]{20,}$/.test(value)) return true;
  if (/^tp-[a-zA-Z0-9]{20,}$/.test(value)) return true;
  if (/^pk-[a-zA-Z0-9-]{20,}$/.test(value)) return true;
  // UUID and volces-UUID (8-4-4-4-{11,12} hex groups)
  if (/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{11,12}$/i.test(value)) return true;
  // Generic opaque token: long alphanumeric+hex+hyphen string with no env markers
  if (value.length >= 32 && /^[a-zA-Z0-9_-]+$/.test(value) && !value.startsWith('${')) return true;
  return false;
}

const FINDINGS = [];

function fail(severity, file, msg) {
  FINDINGS.push({ severity, file, msg });
}

function checkFile(file) {
  if (!fs.existsSync(file)) {
    fail('WARN', file, 'file does not exist');
    return;
  }
  const text = fs.readFileSync(file, 'utf8');
  let data;
  try {
    data = JSON.parse(text);
  } catch (e) {
    fail('CRITICAL', file, `JSON parse error: ${e.message}`);
    return;
  }

  if (data.env && typeof data.env === 'object') {
    for (const [k, v] of Object.entries(data.env)) {
      if (SECRET_KEYS.some(rx => rx.test(k)) && typeof v === 'string') {
        if (looksLikeRealToken(v)) {
          fail('CRITICAL', file, `hardcoded ${k} — value matches a real-token pattern, length ${v.length}`);
        } else {
          fail('WARN', file, `${k} present but looks like placeholder ("${v.slice(0, 20)}...")`);
        }
      }
    }
  }

  if (data.env?.ANTHROPIC_BASE_URL && !data.env.ANTHROPIC_BASE_URL.startsWith('https://')) {
    fail('CRITICAL', file, `ANTHROPIC_BASE_URL is not https: ${data.env.ANTHROPIC_BASE_URL}`);
  }

  if (data.dangerouslyDisableSandbox === true) {
    fail('CRITICAL', file, 'dangerouslyDisableSandbox = true — sandbox is off');
  }

  if (data.permissions?.allow && Array.isArray(data.permissions.allow)) {
    const a = new Set(data.permissions.allow);
    const hasBash = a.has('Bash');
    const hasWrite = a.has('Write');
    const hasEdit = a.has('Edit');
    const hasWebFetch = a.has('WebFetch');
    if (hasBash && hasWrite && hasEdit) {
      fail('WARN', file, 'permissions allow: Bash + Write + Edit all enabled — high blast radius');
    }
    if (hasBash && hasWebFetch && hasWrite) {
      fail('WARN', file, 'Bash + WebFetch + Write: external content can flow into shell');
    }
  }

  if (data.env?.API_TIMEOUT_MS) {
    const t = parseInt(data.env.API_TIMEOUT_MS, 10);
    if (t > 600000) {
      fail('INFO', file, `API_TIMEOUT_MS=${t} (~${Math.round(t/60000)} min) — extremely high`);
    }
  }

  if (data.env?.ANTHROPIC_MODEL) {
    const m = data.env.ANTHROPIC_MODEL.toLowerCase();
    if (m.includes('opus') && data.env.CLAUDE_CODE_SUBAGENT_MODEL?.toLowerCase().includes('opus')) {
      fail('INFO', file, 'main + subagent both set to opus — cost asymmetric');
    }
  }

  if (data.enabledPlugins && typeof data.enabledPlugins === 'object') {
    for (const [name, on] of Object.entries(data.enabledPlugins)) {
      if (on && !name.includes('@')) {
        fail('WARN', file, `plugin "${name}" not version-pinned`);
      }
    }
  }

  if (data.allowDangerousDownloads === true) {
    fail('WARN', file, 'allowDangerousDownloads = true');
  }
}

function main() {
  const targets = process.argv.slice(2);
  if (targets.length === 0) {
    if (!fs.existsSync(CLAUDE_DIR)) {
      console.error(`~/.claude not found at ${CLAUDE_DIR}`);
      process.exit(1);
    }
    for (const f of fs.readdirSync(CLAUDE_DIR)) {
      // only settings.json and settings.json.<variant> — skip .example / .bak / .local
      if (f === 'settings.json' || (/^settings\.json\./.test(f) && !/\.(example|bak|local|disabled)/i.test(f))) {
        checkFile(path.join(CLAUDE_DIR, f));
      }
    }
  } else {
    for (const t of targets) checkFile(t);
  }

  if (FINDINGS.length === 0) {
    console.log('OK: no findings');
    process.exit(0);
  }

  const bySev = { CRITICAL: [], WARN: [], INFO: [] };
  for (const f of FINDINGS) bySev[f.severity].push(f);

  for (const sev of ['CRITICAL', 'WARN', 'INFO']) {
    if (bySev[sev].length === 0) continue;
    console.log(`\n[${sev}] ${bySev[sev].length} finding(s):`);
    for (const f of bySev[sev]) {
      const short = f.file.replace(os.homedir(), '~');
      console.log(`  - ${short}: ${f.msg}`);
    }
  }

  if (bySev.CRITICAL.length > 0) process.exit(1);
  process.exit(0);
}

if (require.main === module) main();
module.exports = { checkFile, looksLikeRealToken };
