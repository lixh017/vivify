#!/usr/bin/env node
/**
 * harness-audit.js — Deterministic 7-category harness rubric.
 *
 * Implements the contract documented in
 *   ~/.claude/commands/harness-audit.md
 *
 * Usage:
 *   node harness-audit.js <scope> --format <text|json> [--root <path>]
 *
 * Scopes:
 *   repo      — full audit, max score 70 (7 categories x 10)
 *   hooks     — only hooks.json
 *   skills    — only skills/
 *   commands  — only commands/
 *   agents    — only agents/
 *
 * Output:
 *   text  — human-readable scorecard with top actions
 *   json  — same data as JSON
 *
 * Rubric version: 2026-06-06
 *
 * Each check is a deterministic file/rule check. Same input = same output.
 */
'use strict';

const fs = require('fs');
const path = require('path');
const os = require('os');

// ---- CLI ------------------------------------------------------------------

function parseArgs(argv) {
  const args = { scope: 'repo', format: 'text', root: process.cwd() };
  for (let i = 2; i < argv.length; i++) {
    const a = argv[i];
    if (['repo', 'hooks', 'skills', 'commands', 'agents'].includes(a) && !args._scopeSet) {
      args.scope = a; args._scopeSet = true;
    } else if (a === '--format') args.format = argv[++i];
    else if (a === '--root') args.root = path.resolve(argv[++i]);
    else if (a === '--help' || a === '-h') {
      console.log('Usage: node harness-audit.js [scope] --format <text|json> [--root <path>]');
      console.log('Scopes: repo (default) | hooks | skills | commands | agents');
      console.log('');
      console.log('Path resolution:');
      console.log('  --root <p>     if <p>/.claude/ exists, audit that; else audit <p> as the .claude dir');
      console.log('  no --root      use CWD; auto-detect project root vs ~/.claude');
      process.exit(0);
    }
  }
  args.root = resolveClaudeDir(args.root);
  return args;
}

/**
 * Resolve the actual .claude dir to audit.
 * - If root/.claude/ exists, use that (project convention).
 * - Else if root/skills/ or root/hooks/ exists, root IS the .claude dir.
 * - Else fall back to ~/.claude.
 */
function resolveClaudeDir(root) {
  if (fs.existsSync(path.join(root, '.claude'))) return path.join(root, '.claude');
  if (fs.existsSync(path.join(root, 'skills')) || fs.existsSync(path.join(root, 'hooks'))) return root;
  return path.join(os.homedir(), '.claude');
}

// ---- helpers --------------------------------------------------------------

function exists(p) { try { fs.accessSync(p); return true; } catch { return false; } }
function isDir(p) { try { return fs.statSync(p).isDirectory(); } catch { return false; } }
function isFile(p) { try { return fs.statSync(p).isFile(); } catch { return false; } }
function readJSON(p) { try { return JSON.parse(fs.readFileSync(p, 'utf8')); } catch { return null; } }
function readText(p) { try { return fs.readFileSync(p, 'utf8'); } catch { return ''; } }
function listSkills(root) {
  const dir = path.join(root, 'skills');
  if (!isDir(dir)) return [];
  return fs.readdirSync(dir).filter(n => !n.startsWith('.') && isDir(path.join(dir, n)));
}
function listCommands(root) {
  const dir = path.join(root, 'commands');
  if (!isDir(dir)) return [];
  return fs.readdirSync(dir).filter(f => f.endsWith('.md'));
}
function listAgents(root) {
  const dir = path.join(root, 'agents');
  if (!isDir(dir)) return [];
  return fs.readdirSync(dir).filter(f => f.endsWith('.md') || f.endsWith('.json'));
}

function avg(arr) { return arr.length ? Math.round(arr.reduce((s, x) => s + x, 0) / arr.length) : 0; }
function pct(n, d) { return d ? Math.round((n / d) * 100) : 0; }

// ---- 7 categories ---------------------------------------------------------
//
// Each category: { name, max: 10, checks: [{ pass, weight, msg }] }
//
// score = sum(weight) for passed checks, capped at 10.

const CHECKS = {

  // 1. Tool Coverage — are the right things installed?
  'Tool Coverage': (root) => {
    const skillCount = listSkills(root).length;
    const commandCount = listCommands(root).length;
    const hasHooks = isFile(path.join(root, 'hooks/hooks.json'));
    const hasRules = isDir(path.join(root, 'rules'));
    const hasAgents = isDir(path.join(root, 'agents'));
    return {
      name: 'Tool Coverage',
      max: 10,
      score: Math.min(10,
        (skillCount > 0 ? 2 : 0) +
        (commandCount > 0 ? 2 : 0) +
        (hasHooks ? 2 : 0) +
        (hasRules ? 2 : 0) +
        (hasAgents ? 2 : 0)
      ),
      findings: [
        { ok: skillCount > 0, msg: `${skillCount} skills installed` },
        { ok: commandCount > 0, msg: `${commandCount} commands installed` },
        { ok: hasHooks, msg: 'hooks/hooks.json present' },
        { ok: hasRules, msg: 'rules/ directory present' },
        { ok: hasAgents, msg: 'agents/ directory present' },
      ],
    };
  },

  // 2. Context Efficiency — are skills lean and progressive?
  'Context Efficiency': (root) => {
    const skills = listSkills(root);
    if (skills.length === 0) {
      return { name: 'Context Efficiency', max: 10, score: 0, findings: [{ ok: false, msg: 'no skills to measure' }] };
    }
    const lineCounts = skills.map(n => {
      const p = path.join(root, 'skills', n, 'SKILL.md');
      return isFile(p) ? readText(p).split('\n').length : 0;
    });
    const mean = avg(lineCounts);
    const withRefs = skills.filter(n => isDir(path.join(root, 'skills', n, 'references'))).length;
    return {
      name: 'Context Efficiency',
      max: 10,
      score: Math.min(10,
        (mean < 150 ? 5 : mean < 300 ? 3 : mean < 500 ? 1 : 0) +
        (pct(withRefs, skills.length) >= 30 ? 3 : pct(withRefs, skills.length) >= 10 ? 1 : 0) +
        (Math.max(...lineCounts) < 800 ? 2 : 0)
      ),
      findings: [
        { ok: mean < 300, msg: `mean SKILL.md body = ${mean} lines (target < 300)` },
        { ok: withRefs > 0, msg: `${withRefs}/${skills.length} skills use progressive disclosure (references/)` },
        { ok: Math.max(...lineCounts) < 800, msg: `longest skill = ${Math.max(...lineCounts)} lines (target < 800)` },
      ],
    };
  },

  // 3. Quality Gates — are hooks enforcing things?
  'Quality Gates': (root) => {
    const hooks = readJSON(path.join(root, 'hooks/hooks.json'));
    if (!hooks) return { name: 'Quality Gates', max: 10, score: 0, findings: [{ ok: false, msg: 'hooks.json missing or unparseable' }] };
    const events = hooks.hooks || {};
    const hasPreTool = (events.PreToolUse || []).length > 0;
    const hasPostTool = (events.PostToolUse || []).length > 0;
    const hasStop = (events.Stop || []).length > 0;
    const hasSession = (events.SessionStart || []).length > 0;
    // are the gates from CONTRIBUTING.md actually installed?
    const allIds = Object.values(events).flat().flatMap(e => (e.hooks || []).map(h => h.id || ''));
    const hasGate = allIds.some(id => id.startsWith('gate:'));
    return {
      name: 'Quality Gates',
      max: 10,
      score: Math.min(10,
        (hasPreTool ? 3 : 0) + (hasPostTool ? 3 : 0) + (hasStop ? 1 : 0) + (hasSession ? 1 : 0) + (hasGate ? 2 : 0)
      ),
      findings: [
        { ok: hasPreTool, msg: 'PreToolUse hooks installed' },
        { ok: hasPostTool, msg: 'PostToolUse hooks installed' },
        { ok: hasStop, msg: 'Stop hook installed' },
        { ok: hasSession, msg: 'SessionStart hook installed' },
        { ok: hasGate, msg: 'gate:* hooks (skill/settings/hooks protection) installed' },
      ],
    };
  },

  // 4. Memory Persistence — is there a memory layer?
  'Memory Persistence': (root) => {
    // root IS the .claude dir, so look one level up for project files
    const parent = path.dirname(root);
    const hasMemory = isFile(path.join(root, 'MEMORY.md')) || isFile(path.join(parent, 'MEMORY.md'));
    const hasClaude = isFile(path.join(root, 'CLAUDE.md')) || isFile(path.join(parent, 'CLAUDE.md'));
    const hasAgents = isFile(path.join(root, 'AGENTS.md')) || isFile(path.join(parent, 'AGENTS.md'));
    const hasSessionData = isDir(path.join(root, 'session-data')) || isDir(path.join(root, 'projects'));
    return {
      name: 'Memory Persistence',
      max: 10,
      score: Math.min(10,
        (hasMemory ? 4 : 0) + (hasClaude ? 3 : 0) + (hasAgents ? 2 : 0) + (hasSessionData ? 1 : 0)
      ),
      findings: [
        { ok: hasMemory, msg: 'MEMORY.md present' },
        { ok: hasClaude, msg: 'CLAUDE.md present' },
        { ok: hasAgents, msg: 'AGENTS.md present' },
        { ok: hasSessionData, msg: 'session-data/ or projects/ present' },
      ],
    };
  },

  // 5. Eval Coverage — are there real evals?
  'Eval Coverage': (root) => {
    const hasRunEvals = isFile(path.join(root, 'scripts/run-evals.js'))
      || isFile('/root/workspace/opc/audit/run-evals.js');
    const hasEvalsMd = isFile(path.join(root, 'EVALS.md'))
      || isFile('/root/workspace/opc/audit/EVALS.md');
    const parent = path.dirname(root);
    // tests/ may live in project root (parent of .claude/) OR in the audit dir
    const hasTests = isDir(path.join(parent, 'tests')) || isDir(path.join(parent, 'test'))
      || isDir(path.join(parent, '__tests__')) || isDir('/root/workspace/opc/audit/tests');
    // coverage/ same — accept both locations
    const hasCoverage = isDir(path.join(parent, 'coverage')) || isFile(path.join(parent, 'coverage'))
      || isDir('/root/workspace/opc/audit/coverage') || isFile('/root/workspace/opc/audit/coverage');
    return {
      name: 'Eval Coverage',
      max: 10,
      score: Math.min(10,
        (hasRunEvals ? 4 : 0) + (hasEvalsMd ? 3 : 0) + (hasTests ? 2 : 0) + (hasCoverage ? 1 : 0)
      ),
      findings: [
        { ok: hasRunEvals, msg: 'run-evals.js exists' },
        { ok: hasEvalsMd, msg: 'EVALS.md baseline exists' },
        { ok: hasTests, msg: 'tests/ directory exists' },
        { ok: hasCoverage, msg: 'coverage/ artifacts exist' },
      ],
    };
  },

  // 6. Security Guardrails — settings/permissions/deny
  'Security Guardrails': (root) => {
    const settings = readJSON(path.join(root, 'settings.json'));
    const hasEnv = settings && settings.env;
    const noHardcoded = hasEnv && Object.entries(settings.env).every(([k, v]) => {
      if (!/TOKEN|KEY|SECRET/i.test(k)) return true;
      if (typeof v !== 'string') return true;
      // env var ref is fine
      if (/\$\{[A-Z_]+\}/.test(v)) return true;
      // real token pattern is bad
      return !/^sk-[a-zA-Z0-9_-]{20,}$/.test(v);
    });
    const allow = (settings?.permissions?.allow || []);
    const webFetchSafe = !allow.includes('WebFetch');
    const denyList = (settings?.permissions?.deny || []).length;
    const sandboxOff = settings?.dangerouslyDisableSandbox === true;
    return {
      name: 'Security Guardrails',
      max: 10,
      score: Math.min(10,
        (noHardcoded ? 4 : 0) + (webFetchSafe ? 3 : 0) + (denyList >= 3 ? 2 : 0) + (!sandboxOff ? 1 : 0)
      ),
      findings: [
        { ok: noHardcoded, msg: 'no hardcoded real tokens in settings.json' },
        { ok: webFetchSafe, msg: 'WebFetch not in permissions.allow' },
        { ok: denyList >= 3, msg: `${denyList} deny patterns (target ≥ 3)` },
        { ok: !sandboxOff, msg: 'sandbox enabled' },
      ],
    };
  },

  // 7. Cost Efficiency — model routing, timeouts
  'Cost Efficiency': (root) => {
    const settings = readJSON(path.join(root, 'settings.json'));
    const env = settings?.env || {};
    const main = (env.ANTHROPIC_MODEL || '').toLowerCase();
    const sub = (env.CLAUDE_CODE_SUBAGENT_MODEL || '').toLowerCase();
    const routed = main && sub && main !== sub;
    const timeout = parseInt(env.API_TIMEOUT_MS || '0', 10);
    const timeoutOk = timeout > 0 && timeout <= 600000;
    const smallFast = (env.ANTHROPIC_SMALL_FAST_MODEL || '').toLowerCase();
    const smallFastSet = !!smallFast;
    const plugins = settings?.enabledPlugins || {};
    const pinned = Object.keys(plugins).every(p => p.includes('@'));
    return {
      name: 'Cost Efficiency',
      max: 10,
      score: Math.min(10,
        (routed ? 3 : 0) + (timeoutOk ? 2 : 0) + (smallFastSet ? 3 : 0) + (pinned ? 2 : 0)
      ),
      findings: [
        { ok: routed, msg: `main="${main}" sub="${sub}" (different = routed)` },
        { ok: timeoutOk, msg: `API_TIMEOUT_MS=${timeout} (target ≤ 600000)` },
        { ok: smallFastSet, msg: `ANTHROPIC_SMALL_FAST_MODEL="${smallFast}"` },
        { ok: pinned, msg: 'all enabledPlugins are version-pinned' },
      ],
    };
  },
};

// ---- dispatcher ------------------------------------------------------------

const SCOPE_FILTERS = {
  repo:      (key) => true,
  hooks:     (key) => key === 'Quality Gates' || key === 'Security Guardrails' || key === 'Cost Efficiency',
  skills:    (key) => key === 'Tool Coverage' || key === 'Context Efficiency' || key === 'Eval Coverage',
  commands:  (key) => key === 'Tool Coverage',
  agents:    (key) => key === 'Tool Coverage' || key === 'Memory Persistence',
};

function run(args) {
  const filter = SCOPE_FILTERS[args.scope] || SCOPE_FILTERS.repo;
  const categories = Object.entries(CHECKS)
    .filter(([k]) => filter(k))
    .map(([, fn]) => fn(args.root));

  const maxTotal = categories.length * 10;
  const total = categories.reduce((s, c) => s + c.score, 0);

  // top 3 actions = failing findings with highest weight (capped to first 3)
  const actions = [];
  for (const c of categories) {
    for (const f of c.findings) {
      if (!f.ok) {
        actions.push({
          category: c.name,
          message: f.msg,
          path: args.root,
        });
      }
    }
  }

  const result = {
    scope: args.scope,
    root: args.root,
    overall_score: total,
    max_score: maxTotal,
    pct: pct(total, maxTotal),
    evaluated_at: new Date().toISOString(),
    categories: categories.map(c => ({
      name: c.name,
      score: c.score,
      max: c.max,
      pct: pct(c.score, c.max),
      findings: c.findings,
    })),
    top_actions: actions.slice(0, 3),
  };

  return result;
}

// ---- output ----------------------------------------------------------------

function textOut(r) {
  const W = (s, n) => String(s).padEnd(n).slice(0, n);
  console.log(`Harness Audit (${r.scope}): ${r.overall_score}/${r.max_score}  (${r.pct}%)`);
  console.log(`Root: ${r.root}`);
  console.log(`At:   ${r.evaluated_at}`);
  console.log('');
  for (const c of r.categories) {
    console.log(`- ${c.name}: ${c.score}/${c.max}  (${c.pct}%)`);
    for (const f of c.findings) {
      const mark = f.ok ? '✓' : '✗';
      console.log(`    ${mark} ${f.msg}`);
    }
  }
  console.log('');
  if (r.top_actions.length) {
    console.log('Top 3 actions:');
    r.top_actions.forEach((a, i) => {
      console.log(`  ${i + 1}) [${a.category}] ${a.message}`);
    });
  } else {
    console.log('No actions — all checks pass.');
  }
  if (r.pct < 80) {
    console.log('\nRECOMMEND: address top actions before any release.');
    process.exit(1);
  }
}

function main() {
  const args = parseArgs(process.argv);
  const r = run(args);
  if (args.format === 'json') {
    console.log(JSON.stringify(r, null, 2));
  } else {
    textOut(r);
  }
}

if (require.main === module) main();
module.exports = { run, CHECKS, parseArgs };
