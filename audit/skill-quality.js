#!/usr/bin/env node
/**
 * skill-quality.js — Deterministic skill quality scorer.
 *
 * Replaces the missing scripts/harness-audit.js claimed by the
 * `/harness-audit skills` command. Scores every SKILL.md under a root
 * against a fixed rubric of 8 categories (0-10 each, normalised).
 *
 * Usage:
 *   node skill-quality.js [--root <path>] [--format text|json] [--top N]
 *
 * Output: writes JSON to stdout (or formatted text).
 * Exits 0 always — this is a reporter, not a gate.
 *
 * Rubric version: 2026-06-06-audit
 */
'use strict';

const fs = require('fs');
const path = require('path');

// ---- CLI ------------------------------------------------------------------

function parseArgs(argv) {
  const args = { root: path.join(process.env.HOME, '.claude/skills'), format: 'text', top: null };
  for (let i = 2; i < argv.length; i++) {
    const a = argv[i];
    if (a === '--root') args.root = argv[++i];
    else if (a === '--format') args.format = argv[++i];
    else if (a === '--top') args.top = parseInt(argv[++i], 10);
    else if (a === '--help' || a === '-h') {
      console.log('Usage: node skill-quality.js [--root <path>] [--format text|json] [--top N]');
      process.exit(0);
    }
  }
  return args;
}

// ---- YAML frontmatter parser (minimal, line-based) ------------------------

function parseFrontmatter(text) {
  if (!text.startsWith('---')) return { frontmatter: {}, body: text, ok: false };
  const end = text.indexOf('\n---', 3);
  if (end < 0) return { frontmatter: {}, body: text, ok: false };
  const fmBlock = text.slice(3, end).trim();
  const body = text.slice(end + 4).replace(/^\n/, '');
  const frontmatter = {};
  for (const line of fmBlock.split('\n')) {
    const m = line.match(/^([a-zA-Z_][\w-]*):\s*(.*)$/);
    if (!m) continue;
    let v = m[2].trim();
    // strip surrounding quotes
    if ((v.startsWith('"') && v.endsWith('"')) || (v.startsWith("'") && v.endsWith("'"))) {
      v = v.slice(1, -1);
    }
    frontmatter[m[1]] = v;
  }
  return { frontmatter, body, ok: true };
}

// ---- Scoring --------------------------------------------------------------

/**
 * Each category is a function (ctx) -> { score: 0-10, findings: [string] }.
 * ctx = { fm, body, name, path, scriptsDir, refsDir, hasCode, hasLinkToOtherSkill, lineCount, wordCount }
 */
const CATEGORIES = [
  // 1. Frontmatter integrity: required fields, no junk
  function frontmatter(ctx) {
    const findings = [];
    let s = 0;
    if (ctx.fm.name) s += 4; else findings.push('missing frontmatter name');
    if (ctx.fm.description) s += 4; else findings.push('missing frontmatter description');
    if (Object.keys(ctx.fm).length <= 3) s += 2; else findings.push('frontmatter has unexpected extra fields (should be name+description only per skill-creator)');
    return { score: s, findings };
  },

  // 2. Description quality: must carry "when to use" trigger
  function description(ctx) {
    const d = (ctx.fm.description || '').toLowerCase();
    const findings = [];
    let s = 0;
    if (d.length >= 40) s += 3; else findings.push(`description too short (${d.length} chars, target 40+)`);
    if (d.includes('use when') || d.includes('use this') || d.includes('use the')) s += 3;
    else findings.push('description missing "Use when..." trigger phrase');
    if (d.includes('use when') && d.split('use when').length > 2) s += 2; // multi-condition trigger
    // penalise lazy descriptions
    if (d.length < 20) s -= 2;
    // bonus for mentioning concrete domain/tech keywords
    const kw = /\b(api|test|deploy|review|design|security|frontend|backend|refactor|debug|optimi[sz]e|architect)\b/;
    if (kw.test(d)) s += 2;
    s = Math.max(0, Math.min(10, s));
    return { score: s, findings };
  },

  // 3. Body structure: has triggers, sections, anti-patterns
  function bodyStructure(ctx) {
    const b = ctx.body;
    const findings = [];
    let s = 0;
    if (b.match(/^#{1,3}\s/m)) s += 2; // has headings
    if (/^##\s+(when to|when to use|usage|how to)/im.test(b)) s += 2;
    else findings.push('no "When to use" or "Usage" section in body');
    if (/^##\s+(anti-?pattern|do not|never|don'?t)/im.test(b)) s += 1;
    if (/^##\s+(best practice|guideline|principle)/im.test(b)) s += 1;
    if (/^##\s+(example|usage example)/im.test(b)) s += 1;
    if (/```[\s\S]+?```/m.test(b)) s += 2; // has code blocks
    else findings.push('no code blocks found');
    if (/\[.+\]\(.+\.md\)/m.test(b)) s += 1; // links to other .md
    s = Math.max(0, Math.min(10, s));
    return { score: s, findings };
  },

  // 4. Concision: penalise extreme lengths
  function concision(ctx) {
    const n = ctx.lineCount;
    const findings = [];
    let s = 10;
    if (n < 30) { s = 2; findings.push(`body too thin (${n} lines, target 50+)`); }
    else if (n < 50) { s = 5; findings.push(`body thin (${n} lines)`); }
    else if (n > 800) { s = 4; findings.push(`body too long (${n} lines, consider progressive disclosure; skill-creator target <500)`); }
    else if (n > 500) { s = 7; findings.push(`body long (${n} lines, consider references/ split)`); }
    return { score: s, findings };
  },

  // 5. Actionability: does it actually tell the agent what to DO?
  function actionability(ctx) {
    const b = ctx.body;
    const findings = [];
    let s = 0;
    if (/step\s*\d/im.test(b) || /^##\s+\d+\./m.test(b)) s += 3;
    else findings.push('no numbered steps');
    if (/```(bash|sh|shell)/im.test(b)) s += 2; // has runnable commands
    if (/```(typescript|javascript|ts|js)/im.test(b)) s += 2;
    if (/FAIL:|WRONG:|CORRECT:|PASS:/i.test(b)) s += 2; // contrastive examples
    if (ctx.scriptsDir && ctx.scriptsDir.length > 0) s += 1;
    if (/checkpoint|verify|coverage|test/i.test(b)) s += 1; // mentions verification
    s = Math.min(10, s);
    return { score: s, findings };
  },

  // 6. Bundled resources: scripts/ references/ assets/
  function bundled(ctx) {
    const findings = [];
    let s = 0;
    if (ctx.scriptsDir && ctx.scriptsDir.length > 0) s += 5;
    else findings.push('no scripts/ directory');
    if (ctx.refsDir && ctx.refsDir.length > 0) s += 3;
    else findings.push('no references/ directory (consider for >500 line skills)');
    if (ctx.hasInitScript) s += 1;
    if (ctx.hasValidateScript) s += 1;
    return { score: s, findings };
  },

  // 7. Cross-references: properly names other skills, doesn't orphan itself
  function crossRefs(ctx) {
    const b = ctx.body;
    const findings = [];
    let s = 5; // baseline
    // penalty for absolute paths that won't exist
    if (/node\s+scripts\//.test(b)) {
      findings.push('references a node script via path — verify the script exists in this skill or in the parent repo');
      s -= 2;
    }
    // penalty for self-deprecation without target
    if (/deprecated|superseded|no longer/i.test(b) && !/use\s+[`']?[a-z-]+[`']?/i.test(b)) {
      findings.push('mentions deprecation without naming the replacement skill');
      s -= 1;
    }
    s = Math.max(0, Math.min(10, s));
    return { score: s, findings };
  },

  // 8. Testability: could an evaluator script verify this skill's claims?
  function testability(ctx) {
    const b = ctx.body;
    const findings = [];
    let s = 0;
    if (/pass@k|pass@1|pass@3/i.test(b)) s += 2;
    if (/checkpoint commit|RED.*GREEN/i.test(b)) s += 2;
    if (ctx.hasValidateScript) s += 3; // has scripts/quick_validate.py or similar
    if (/eval[- ]driven|TDD|verify before completion/i.test(b)) s += 2;
    if (/anti-?pattern/i.test(b)) s += 1;
    return { score: Math.min(10, s), findings };
  },
];

// ---- Per-skill evaluation -------------------------------------------------

function evaluateSkill(skillPath) {
  const text = fs.readFileSync(skillPath, 'utf8');
  const { frontmatter: fm, body, ok } = parseFrontmatter(text);
  const dir = path.dirname(skillPath);
  const scriptsDir = fs.existsSync(path.join(dir, 'scripts'))
    ? fs.readdirSync(path.join(dir, 'scripts'))
    : [];
  const refsDir = fs.existsSync(path.join(dir, 'references'))
    ? fs.readdirSync(path.join(dir, 'references'))
    : [];

  const lineCount = body.split('\n').length;
  const wordCount = body.split(/\s+/).length;

  const ctx = {
    name: fm.name || path.basename(dir),
    path: skillPath,
    fm,
    body,
    scriptsDir,
    refsDir,
    hasCode: /```/.test(body),
    hasLinkToOtherSkill: /\[[^\]]+\]\(([a-z-]+)(?:\.md)?\)/i.test(body),
    lineCount,
    wordCount,
    hasInitScript: scriptsDir.some(f => /init|setup|create/i.test(f)),
    hasValidateScript: scriptsDir.some(f => /validate|verify|check|test|lint/i.test(f)),
  };

  const breakdown = {};
  let total = 0;
  for (const cat of CATEGORIES) {
    const r = cat(ctx);
    breakdown[cat.name] = r;
    total += r.score;
  }

  const verdict = pickVerdict(breakdown, ctx);
  return {
    name: ctx.name,
    path: skillPath,
    mtime: fs.statSync(skillPath).mtime.toISOString(),
    lineCount: ctx.lineCount,
    scripts: scriptsDir.length,
    refs: refsDir.length,
    total_score: total,
    max_score: CATEGORIES.length * 10,
    pct: Math.round((total / (CATEGORIES.length * 10)) * 100),
    breakdown,
    verdict,
  };
}

function pickVerdict(b, ctx) {
  // Retire is destructive; only flag when evidence is strong.
  // Strong evidence = thin body AND no scripts AND no refs AND low description score.
  if (!ctx.fm.name || !ctx.fm.description) {
    return { v: 'Broken', why: 'missing required frontmatter (name or description)' };
  }
  const thinAndEmpty =
    ctx.lineCount < 50 &&
    ctx.scriptsDir.length === 0 &&
    ctx.refsDir.length === 0 &&
    (b.description?.score ?? 10) < 5;
  if (thinAndEmpty) {
    return { v: 'Retire', why: `thin (${ctx.lineCount} lines), no scripts/refs, weak description` };
  }
  if (b.crossRefs?.findings?.some(f => f.includes('node script'))) {
    return { v: 'Update', why: 'references a node script — verify it exists or remove the claim' };
  }
  if (ctx.lineCount < 30) {
    return { v: 'Improve', why: `body too thin (${ctx.lineCount} lines)` };
  }
  if (b.testability?.score === 0 && (b.actionability?.score ?? 0) >= 6) {
    return { v: 'Improve', why: 'good actionable guidance but no way to verify outputs' };
  }
  if ((b.bundled?.score ?? 0) < 3 && ctx.lineCount > 300) {
    return { v: 'Improve', why: 'long body with no scripts/ — consider progressive disclosure' };
  }
  return { v: 'Keep', why: 'meets baseline' };
}

// ---- File walking ---------------------------------------------------------

function findSkills(root) {
  const out = [];
  if (!fs.existsSync(root)) return out;
  for (const entry of fs.readdirSync(root, { withFileTypes: true })) {
    if (!entry.isDirectory()) continue;
    if (entry.name.startsWith('.')) continue;
    const skillFile = path.join(root, entry.name, 'SKILL.md');
    if (fs.existsSync(skillFile)) out.push(skillFile);
  }
  return out;
}

// ---- Main -----------------------------------------------------------------

function main() {
  const args = parseArgs(process.argv);
  const skills = findSkills(args.root).map(evaluateSkill);

  // Sort by pct desc
  skills.sort((a, b) => b.pct - a.pct);

  if (args.format === 'json') {
    console.log(JSON.stringify({
      evaluated_at: new Date().toISOString(),
      root: args.root,
      count: skills.length,
      results: skills,
    }, null, 2));
    return;
  }

  // Text format
  const W = (s, n) => String(s).padEnd(n).slice(0, n);
  const verdictCounts = skills.reduce((acc, s) => {
    acc[s.verdict.v] = (acc[s.verdict.v] || 0) + 1;
    return acc;
  }, {});

  console.log(`# Skill Quality Report`);
  console.log(`Root: ${args.root}`);
  console.log(`Evaluated: ${skills.length} skills  |  ${new Date().toISOString()}`);
  console.log('');
  console.log(`## Verdict distribution`);
  for (const [k, v] of Object.entries(verdictCounts).sort((a, b) => b[1] - a[1])) {
    console.log(`  ${W(k, 12)} ${v}`);
  }
  console.log('');
  console.log(`## Bottom 10 (need work)`);
  console.log(W('Skill', 40) + W('Score', 8) + W('Lines', 8) + W('Scr', 5) + 'Verdict');
  console.log('-'.repeat(80));
  for (const s of skills.slice(-10).reverse()) {
    console.log(W(s.name, 40) + W(s.pct + '%', 8) + W(s.lineCount, 8) + W(s.scripts, 5) + s.verdict.v + ' — ' + s.verdict.why);
  }
  console.log('');
  console.log(`## Top ${args.top || 10}`);
  for (const s of (args.top ? skills.slice(0, args.top) : skills.slice(0, 10))) {
    console.log(W(s.name, 40) + W(s.pct + '%', 8) + W(s.lineCount, 8) + W(s.scripts, 5) + s.verdict.v);
  }
  console.log('');
  console.log(`## Critical findings`);
  for (const s of skills) {
    const all = Object.entries(s.breakdown).flatMap(([k, v]) => v.findings.map(f => `[${k}] ${f}`));
    if (all.length === 0) continue;
    if (s.verdict.v === 'Keep' && s.pct >= 70) continue;
    console.log(`- ${s.name} (${s.pct}%):`);
    for (const f of all.slice(0, 5)) console.log(`    ${f}`);
  }
}

if (require.main === module) main();
module.exports = { evaluateSkill, findSkills, CATEGORIES };
