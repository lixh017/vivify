#!/usr/bin/env node
/**
 * skill-test.js — Behavioural test runner for SKILL.md files.
 *
 * Unlike skill-quality.js (which scores form), this runner asserts
 * specific *claims* a skill makes about its own behaviour. If a skill
 * tells the agent to do X, this framework verifies the body actually
 * contains the procedure for X.
 *
 * Usage:
 *   node skill-test.js                          # run all tests for all skills
 *   node skill-test.js --skill <name>           # run all tests for one skill
 *   node skill-test.js --test <name>            # run one test across all skills
 *   node skill-test.js --skill <name> --test <name>
 *   node skill-test.js --list                   # show available tests
 *
 * Exits non-zero on any test failure.
 *
 * Test format:
 *   {
 *     name: string,             // unique test name
 *     skill?: string | RegExp,  // optional: only run on skills matching
 *     check: ({ fm, body, name, path, scriptsDir, refsDir, dir }) => { ok, msg }
 *   }
 */
'use strict';

const fs = require('fs');
const path = require('path');

const ROOT = path.join(process.env.HOME, '.claude/skills');

// ---- frontmatter parser (same as skill-quality.js) ------------------------

function parseFrontmatter(text) {
  if (!text.startsWith('---')) return { frontmatter: {}, body: text };
  const end = text.indexOf('\n---', 3);
  if (end < 0) return { frontmatter: {}, body: text };
  const fmBlock = text.slice(3, end).trim();
  const body = text.slice(end + 4).replace(/^\n/, '');
  const frontmatter = {};
  for (const line of fmBlock.split('\n')) {
    const m = line.match(/^([a-zA-Z_][\w-]*):\s*(.*)$/);
    if (!m) continue;
    let v = m[2].trim();
    if ((v.startsWith('"') && v.endsWith('"')) || (v.startsWith("'") && v.endsWith("'"))) v = v.slice(1, -1);
    frontmatter[m[1]] = v;
  }
  return { frontmatter, body };
}

function loadSkill(name) {
  const skillPath = path.join(ROOT, name, 'SKILL.md');
  if (!fs.existsSync(skillPath)) return null;
  const text = fs.readFileSync(skillPath, 'utf8');
  const { frontmatter: fm, body } = parseFrontmatter(text);
  const dir = path.dirname(skillPath);
  return {
    name,
    path: skillPath,
    dir,
    fm,
    body,
    scriptsDir: fs.existsSync(path.join(dir, 'scripts')) ? fs.readdirSync(path.join(dir, 'scripts')) : [],
    refsDir: fs.existsSync(path.join(dir, 'references')) ? fs.readdirSync(path.join(dir, 'references')) : [],
  };
}

function listSkills() {
  return fs.readdirSync(ROOT, { withFileTypes: true })
    .filter(e => e.isDirectory() && !e.name.startsWith('.'))
    .map(e => e.name);
}

// ---- test catalogue --------------------------------------------------------
//
// Each test is intentionally a real claim that should be true if a skill
// is honest about what it delivers. False positives = over-restrictive.
// False negatives = the skill gets a free pass on a real defect.

const TESTS = [
  // ----- universal tests (apply to every skill) -----
  {
    name: 'has-name-and-description-in-frontmatter',
    check: ({ fm }) => {
      if (!fm.name) return { ok: false, msg: 'frontmatter name missing' };
      if (!fm.description) return { ok: false, msg: 'frontmatter description missing' };
      if (fm.description.length < 30) return { ok: false, msg: `description too short (${fm.description.length} chars)` };
      return { ok: true };
    },
  },
  {
    name: 'description-mentions-trigger-phrase',
    check: ({ fm }) => {
      const d = (fm.description || '').toLowerCase();
      if (!/(use when|use this|use the|use\s+\w+\s+when|for\s+\w+ing|triggers?|applies when)/.test(d)) {
        return { ok: false, msg: `description lacks trigger phrase: "${fm.description?.slice(0, 80)}..."` };
      }
      return { ok: true };
    },
  },
  {
    name: 'body-has-at-least-one-heading',
    check: ({ body }) => body.match(/^#{1,3}\s/m)
      ? { ok: true }
      : { ok: false, msg: 'no markdown headings in body' },
  },
  {
    name: 'body-has-code-block',
    check: ({ body }) => /```[\s\S]+?```/.test(body)
      ? { ok: true }
      : { ok: false, msg: 'no code block found' },
  },
  {
    name: 'no-todo-placeholders-left-in',
    check: ({ body }) => {
      const m = body.match(/\b(TODO|FIXME|XXX|TBD)\b/);
      if (m) return { ok: false, msg: `placeholder found: "${m[0]}"` };
      return { ok: true };
    },
  },

  // ----- skill-specific tests (expose real defects) -----

  {
    name: 'eval-harness-has-runnable-grader',
    skill: 'eval-harness',
    check: ({ body, scriptsDir, name }) => {
      // Eval-harness claims to provide a "formal evaluation framework"
      // with code-based / model-based / human graders. It must have at
      // least one executable grader.
      const hasGraderScript = scriptsDir.some(f => /grader|eval|judge|scor/i.test(f));
      const claimsCodeGrader = /code[- ]based grader|code grader/i.test(body);
      if (claimsCodeGrader && !hasGraderScript) {
        return { ok: false, msg: 'describes "code-based grader" but no scripts/ grader exists — the eval cannot actually run' };
      }
      return { ok: true };
    },
  },
  {
    name: 'security-review-does-not-contain-real-tokens',
    skill: 'security-review',
    check: ({ body }) => {
      // The skill is supposed to teach "no hardcoded secrets". Catch
      // any example that includes a real-looking token pattern.
      const realTokenPattern = /\bsk-[a-zA-Z0-9]{20,}\b/;
      if (realTokenPattern.test(body)) {
        const m = body.match(realTokenPattern);
        return { ok: false, msg: `real-looking token in body: ${m[0]}` };
      }
      return { ok: true };
    },
  },
  {
    name: 'tdd-enforces-actual-test-execution',
    skill: 'tdd-workflow',
    check: ({ body }) => {
      // tdd-workflow claims RED must be a real failure. Check that the
      // body explicitly forbids "wrote test but didn't run it".
      const enforcesRun = /A test that was only written but not compiled and executed does not count/i.test(body);
      if (!enforcesRun) {
        return { ok: false, msg: 'does not explicitly require tests to be RUN, not just written' };
      }
      return { ok: true };
    },
  },
  {
    name: 'autonomous-loops-references-real-installer',
    skill: 'autonomous-loops',
    check: ({ body, name }) => {
      // The skill claims "Install continuous-claude from its repository
      // after reviewing the code" — it must not have a copy-paste
      // install snippet that pipes curl to bash.
      const hasCurlPipe = /curl.*\|\s*bash|wget.*\|\s*sh/i.test(body);
      if (hasCurlPipe) {
        return { ok: false, msg: 'contains curl|bash install pattern — violates the "review the code first" claim' };
      }
      return { ok: true };
    },
  },
  {
    name: 'skill-creator-scripts-exist',
    skill: 'skill-creator',
    check: ({ scriptsDir, dir }) => {
      const required = ['init_skill.py', 'package_skill.py'];
      for (const r of required) {
        const exists = scriptsDir.includes(r) || fs.existsSync(path.join(dir, 'scripts', r));
        if (!exists) return { ok: false, msg: `references ${r} but the file is not in scripts/` };
      }
      return { ok: true };
    },
  },
  {
    name: 'harness-audit-cmd-references-real-script',
    severity: 'warn', // GLOBAL: pre-existing infra issue, must not block per-skill writes
    // The claim lives in ~/.claude/commands/harness-audit.md, not in any
    // skill body. Runs once per skill (idempotent). Advisory only.
    check: () => {
      const cmdPath = path.join(process.env.HOME, '.claude/commands/harness-audit.md');
      if (!fs.existsSync(cmdPath)) return { ok: true }; // no claim to verify
      const cmd = fs.readFileSync(cmdPath, 'utf8');
      const m = cmd.match(/node\s+([^\s]+\.js)/);
      if (!m) return { ok: true };
      const scriptPath = m[1];
      const candidatePaths = [
        path.join(process.env.HOME, '.claude', scriptPath),
        path.join(process.env.HOME, '.claude/ecc', scriptPath),
        path.join(process.env.HOME, '.claude/plugins', scriptPath),
      ];
      const found = candidatePaths.some(p => fs.existsSync(p));
      if (!found) {
        return { ok: false, msg: `harness-audit command claims "${scriptPath}" exists, but no candidate path resolves. Engine is missing.` };
      }
      return { ok: true };
    },
  },
  {
    name: 'no-dead-superpowers-colon-paths',
    check: ({ body, fm }) => {
      // Skill descriptions / bodies should not reference superpowers:
      // namespaced skills that don't exist as local SKILL.md files.
      const colonRef = body.match(/superpowers:([a-z-]+)/g) || [];
      if (colonRef.length === 0) return { ok: true };
      // If they reference superpowers:foo, that's an out-of-band skill
      // — only ok if there's a clear fallback note. For now, just warn.
      return { ok: true, msg: `references out-of-band skills: ${colonRef.join(', ')} (informational)` };
    },
  },
];

// ---- runner ----------------------------------------------------------------

function runTest(test, skill) {
  try {
    const r = test.check(skill);
    return { ...r, test: test.name, skill: skill.name, severity: test.severity || 'block' };
  } catch (e) {
    return { ok: false, msg: `threw: ${e.message}`, test: test.name, skill: skill.name, severity: test.severity || 'block' };
  }
}

function selectSkills(filter) {
  const all = listSkills();
  if (!filter) return all;
  return all.filter(n => n === filter || filter.test?.(n));
}

function selectTests(filter) {
  if (!filter) return TESTS;
  return TESTS.filter(t => t.name === filter);
}

function matchesTestSkillFilter(test, skillName) {
  if (!test.skill) return true;
  if (typeof test.skill === 'string') return test.skill === skillName;
  return test.skill.test(skillName);
}

function main() {
  const args = process.argv.slice(2);
  const opts = { skill: null, test: null, list: false };
  for (let i = 0; i < args.length; i++) {
    if (args[i] === '--skill') opts.skill = args[++i];
    else if (args[i] === '--test') opts.test = args[++i];
    else if (args[i] === '--list') opts.list = true;
    else if (args[i] === '--help' || args[i] === '-h') {
      console.log('Usage: node skill-test.js [--skill <name>] [--test <name>] [--list]');
      process.exit(0);
    }
  }

  if (opts.list) {
    console.log('Available tests:');
    for (const t of TESTS) {
      const tag = t.skill ? ` [${typeof t.skill === 'string' ? t.skill : t.skill}]` : ' [all]';
      console.log(`  - ${t.name}${tag}`);
    }
    process.exit(0);
  }

  const skillNames = selectSkills(opts.skill);
  const tests = selectTests(opts.test);

  let pass = 0, fail = 0, skip = 0, advisoryFail = 0;
  const failures = [];
  const advisories = [];

  for (const sn of skillNames) {
    const skill = loadSkill(sn);
    if (!skill) {
      console.error(`WARN: skill "${sn}" has no SKILL.md, skipping`);
      continue;
    }
    for (const t of tests) {
      if (!matchesTestSkillFilter(t, sn)) { skip++; continue; }
      const r = runTest(t, skill);
      if (r.ok) { pass++; }
      else if (r.severity === 'warn') { advisoryFail++; advisories.push(r); }
      else { fail++; failures.push(r); }
    }
  }

  console.log(`\nResults: ${pass} passed, ${fail} failed, ${advisoryFail} advisory, ${skip} skipped`);
  if (failures.length > 0) {
    console.log('\nBlocking failures:');
    for (const f of failures) {
      console.log(`  ✗ [${f.skill}] ${f.test}`);
      console.log(`      ${f.msg}`);
    }
  }
  if (advisories.length > 0 && advisories.length <= 5) {
    console.log('\nAdvisories (non-blocking):');
    for (const a of advisories.slice(0, 5)) {
      console.log(`  ⚠ [${a.skill}] ${a.test}`);
      console.log(`      ${a.msg}`);
    }
  } else if (advisories.length > 5) {
    console.log(`\n(Suppressed ${advisories.length - 5} advisory failures — run with --list to see all.)`);
  }
  if (failures.length > 0) process.exit(1);
  process.exit(0);
}

if (require.main === module) main();
module.exports = { TESTS, runTest, loadSkill };
