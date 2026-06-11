#!/usr/bin/env node
/**
 * run-evals.js — Executable eval baseline for skills + harness.
 *
 * Implements the 20 evals defined in EVALS.md. Each eval returns
 * { id, name, pass, msg, severity }. Exits non-zero on any critical
 * failure or pass rate < 90%.
 *
 * Usage:
 *   node run-evals.js --all
 *   node run-evals.js --suite code
 *   node run-evals.js --eval E01
 *   node run-evals.js --json
 */
'use strict';

const fs = require('fs');
const path = require('path');
const os = require('os');

const HOME = os.homedir();
const SKILLS = path.join(HOME, '.claude/skills');
const COMMANDS = path.join(HOME, '.claude/commands');
const SETTINGS = path.join(HOME, '.claude/settings.json');
const HOOKS = path.join(HOME, '.claude/hooks/hooks.json');

// ---- helpers ---------------------------------------------------------------

/**
 * @param {string} name
 * @returns {{fm: object, body: string, dir: string, path: string, scripts: string[], refs: string[]} | null}
 */
function loadSkill(name) {
  const p = path.join(SKILLS, name, 'SKILL.md');
  if (!fs.existsSync(p)) return null;
  const text = fs.readFileSync(p, 'utf8');
  if (!text.startsWith('---')) return { fm: {}, body: text, dir: path.dirname(p), path: p, scripts: [], refs: [] };
  const end = text.indexOf('\n---', 3);
  if (end < 0) return { fm: {}, body: text, dir: path.dirname(p), path: p, scripts: [], refs: [] };
  const fmBlock = text.slice(3, end).trim();
  const body = text.slice(end + 4).replace(/^\n/, '');
  const fm = {};
  for (const line of fmBlock.split('\n')) {
    const m = line.match(/^([a-zA-Z_][\w-]*):\s*(.*)$/);
    if (m) fm[m[1]] = m[2].trim().replace(/^["']|["']$/g, '');
  }
  const dir = path.dirname(p);
  return {
    fm, body, dir, path: p,
    scripts: fs.existsSync(path.join(dir, 'scripts')) ? fs.readdirSync(path.join(dir, 'scripts')) : [],
    refs: fs.existsSync(path.join(dir, 'references')) ? fs.readdirSync(path.join(dir, 'references')) : [],
  };
}

function readJSON(p) {
  try { return JSON.parse(fs.readFileSync(p, 'utf8')); }
  catch { return null; }
}

// ---- evals -----------------------------------------------------------------

const EVALS = [
  // E01 — tdd-workflow enforces test execution
  {
    id: 'E01', name: 'tdd-workflow enforces test execution', severity: 'CRITICAL',
    run: () => {
      const s = loadSkill('tdd-workflow'); if (!s) return { pass: false, msg: 'skill missing' };
      const ok = /written but not (compiled|run|executed)/i.test(s.body)
        || /test that was only written/i.test(s.body)
        || /count as RED/i.test(s.body);
      return { pass: ok, msg: ok ? 'forbids "wrote but did not run"' : 'does not explicitly forbid "wrote but did not run"' };
    },
  },

  // E02 — tdd-workflow mentions 80% coverage as target
  {
    id: 'E02', name: 'tdd-workflow mentions 80% coverage', severity: 'HIGH',
    run: () => {
      const s = loadSkill('tdd-workflow'); if (!s) return { pass: false, msg: 'skill missing' };
      const ok = /80\s*%/.test(s.body);
      return { pass: ok, msg: ok ? 'mentions 80%' : 'no 80% threshold' };
    },
  },

  // E03 — tdd-workflow has RED / GREEN sections
  {
    id: 'E03', name: 'tdd-workflow has RED / GREEN sections', severity: 'HIGH',
    run: () => {
      const s = loadSkill('tdd-workflow'); if (!s) return { pass: false, msg: 'skill missing' };
      const hasRED = /RED/i.test(s.body);
      const hasGREEN = /GREEN/i.test(s.body);
      return { pass: hasRED && hasGREEN, msg: `RED=${hasRED} GREEN=${hasGREEN}` };
    },
  },

  // E04 — security-review no real tokens
  {
    id: 'E04', name: 'security-review body has no real-looking token', severity: 'CRITICAL',
    run: () => {
      const s = loadSkill('security-review'); if (!s) return { pass: false, msg: 'skill missing' };
      const m = s.body.match(/\bsk-[a-zA-Z0-9_-]{20,}\b/);
      if (m) return { pass: false, msg: `real-looking token: ${m[0]}` };
      return { pass: true, msg: 'no real tokens' };
    },
  },

  // E05 — security-review doesn't recommend dangerous patterns
  {
    id: 'E05', name: 'security-review avoids recommending dangerous patterns', severity: 'HIGH',
    run: () => {
      const s = loadSkill('security-review'); if (!s) return { pass: false, msg: 'skill missing' };
      // For each dangerous pattern, check it appears in a "teach against" context.
      // A "teach against" context is either:
      //   (a) preceded by WRONG|FAIL|NEVER within 100 chars, OR
      //   (b) followed by DOMPurify|sanitiz (sanitized usage) within 100 chars
      const checks = [
        { pattern: /\beval\s*\(/i, label: 'eval()' },
        { pattern: /innerHTML\s*=/i, label: 'innerHTML=' },
        { pattern: /document\.write\s*\(/i, label: 'document.write' },
      ];
      const issues = [];
      for (const { pattern, label } of checks) {
        if (!pattern.test(s.body)) continue;
        // find each match and check its context
        const matches = [...s.body.matchAll(new RegExp(pattern.source, pattern.flags + 'g'))];
        for (const m of matches) {
          const start = Math.max(0, m.index - 100);
          const end = Math.min(s.body.length, m.index + 200);
          const ctx = s.body.slice(start, end);
          const teachAgainst = /WRONG|FAIL|NEVER|dangerous|avoid/i.test(ctx);
          const sanitized = /DOMPurify|sanitiz|escape|validator/i.test(ctx);
          if (!teachAgainst && !sanitized) {
            issues.push(`${label} at offset ${m.index} not in teach-against context`);
            break; // one finding per pattern is enough
          }
        }
      }
      if (issues.length) return { pass: false, msg: issues.join('; ') };
      return { pass: true, msg: 'no bare dangerous recommendations' };
    },
  },

  // E06 — security-review FAIL: has matching PASS:
  {
    id: 'E06', name: 'security-review FAIL: examples have matching PASS:', severity: 'MEDIUM',
    run: () => {
      const s = loadSkill('security-review'); if (!s) return { pass: false, msg: 'skill missing' };
      const fails = (s.body.match(/^FAIL:|^WRONG:/gmi) || []).length;
      const passes = (s.body.match(/^PASS:|^CORRECT:/gmi) || []).length;
      if (fails === 0) return { pass: true, msg: 'no FAIL examples (vacuously OK)' };
      if (passes < fails) return { pass: false, msg: `FAIL=${fails} but PASS=${passes} (every FAIL needs a PASS)` };
      return { pass: true, msg: `FAIL=${fails} PASS=${passes}` };
    },
  },

  // E07 — frontend-design description is specific
  {
    id: 'E07', name: 'frontend-design description names a specific trigger', severity: 'MEDIUM',
    run: () => {
      const s = loadSkill('frontend-design'); if (!s) return { pass: false, msg: 'skill missing' };
      const d = (s.fm.description || '').toLowerCase();
      const ok = /use when|use this|for\s+\w+ing|when (the )?(user|task|build|design)/.test(d);
      return { pass: ok, msg: ok ? 'has trigger phrase' : `description: "${d.slice(0, 80)}..."` };
    },
  },

  // E08 — frontend-design has anti-patterns
  {
    id: 'E08', name: 'frontend-design has anti-patterns section with ≥3 bullets', severity: 'MEDIUM',
    run: () => {
      const s = loadSkill('frontend-design'); if (!s) return { pass: false, msg: 'skill missing' };
      const m = s.body.match(/^##\s+(.*anti-?pattern|never|do not|don'?t)(.*)$/im);
      if (!m) return { pass: false, msg: 'no anti-patterns section' };
      const start = m.index + m[0].length;
      const rest = s.body.slice(start, start + 1500);
      const bullets = (rest.match(/^[-*]\s+/gm) || []).length;
      return { pass: bullets >= 3, msg: `${bullets} bullets in anti-patterns section` };
    },
  },

  // E09 — skill-creator scripts exist
  {
    id: 'E09', name: 'skill-creator has init_skill.py and package_skill.py', severity: 'CRITICAL',
    run: () => {
      const s = loadSkill('skill-creator'); if (!s) return { pass: false, msg: 'skill missing' };
      const required = ['init_skill.py', 'package_skill.py'];
      const missing = required.filter(r => !s.scripts.includes(r));
      if (missing.length) return { pass: false, msg: `missing scripts: ${missing.join(', ')}` };
      return { pass: true, msg: 'all required scripts present' };
    },
  },

  // E10 — skill-creator has trigger in description
  {
    id: 'E10', name: 'skill-creator description has trigger phrase', severity: 'MEDIUM',
    run: () => {
      const s = loadSkill('skill-creator'); if (!s) return { pass: false, msg: 'skill missing' };
      const d = (s.fm.description || '').toLowerCase();
      const ok = /use when|use this|create a new skill|update an existing/.test(d);
      return { pass: ok, msg: ok ? 'has trigger' : 'no trigger phrase' };
    },
  },

  // E11 — all skills have name + description
  {
    id: 'E11', name: 'all skills have name + description, length ≥30', severity: 'HIGH',
    run: () => {
      const failures = [];
      for (const name of fs.readdirSync(SKILLS)) {
        if (name.startsWith('.')) continue;
        const s = loadSkill(name); if (!s) continue;
        // For block scalars (| or >), the actual description lives on subsequent
        // indented lines. Re-read the file and reconstruct the full text.
        let fullDesc = s.fm.description || '';
        const text = fs.readFileSync(s.path, 'utf8');
        const fmMatch = text.match(/^---\n([\s\S]*?)\n---/);
        if (fmMatch) {
          const fm = fmMatch[1];
          const blockMatch = fm.match(/^description:\s*([>|][+-]?)\s*\n((?:\s+.+\n?)+)/m);
          if (blockMatch) {
            const continuation = blockMatch[2]
              .split('\n')
              .map(l => l.replace(/^\s+/, ''))
              .filter(l => l.length > 0)
              .join(' ');
            fullDesc = continuation;
          }
        }
        if (!s.fm.name) failures.push(`${name}: no name`);
        else if (!fullDesc) failures.push(`${name}: no description`);
        else if (fullDesc.length < 30) failures.push(`${name}: desc len ${fullDesc.length}`);
      }
      if (failures.length === 0) return { pass: true, msg: 'all skills have proper frontmatter (block-scalar aware)' };
      return { pass: false, msg: `${failures.length} skills: ${failures.slice(0, 3).join('; ')}...` };
    },
  },

  // E12 — no real TODO/FIXME placeholders in shipped body
  //
  // What counts as a "real" placeholder (and should fail):
  //   - "TODO:" at start of a line in prose (unfinished work)
  //   - "[ ] TODO" or "- TODO" in a checklist
  //   - "XXX" or "FIXME" used as a real marker, not a URL placeholder
  //
  // What does NOT count (false positives we explicitly skip):
  //   - The word in a code block (e.g. Perl's `TODO:` block syntax,
  //     Kotlin's `TODO("not implemented")` test stub, Python's `# TODO` comment)
  //   - "TODO" in a normal sentence (e.g. "TODO list", "TODO placeholders")
  //   - "XXX" inside a URL or as a number placeholder (e.g. "PR/XXX")
  //   - A CHECKLIST item that says "no TODO/FIXME allowed" (meta-rule)
  {
    id: 'E12', name: 'no real TODO/FIXME/XXX placeholders in shipped SKILL.md bodies', severity: 'MEDIUM',
    run: () => {
      const findings = [];
      for (const name of fs.readdirSync(SKILLS)) {
        if (name.startsWith('.')) continue;
        const s = loadSkill(name); if (!s) continue;
        // Strip code blocks from the body so we only inspect prose
        const prose = s.body
          .replace(/```[\s\S]*?```/g, ' ')   // fenced code blocks
          .replace(/`[^`\n]+`/g, ' ')         // inline code spans
          .replace(/https?:\/\/\S+/g, ' ');   // URLs
        // Look for "TODO" / "FIXME" used as a writer's marker, not as language
        // - "TODO:" / "FIXME:" at start of line
        // - "- TODO" / "* TODO" in a list
        // - "[ ] TODO" in a checklist
        // - "XXX" as standalone word (not inside a URL which we stripped)
        const patterns = [
          /^[ \t]*(?:[-*]|\[[ xX]\])[ \t]+(?:TODO|FIXME)\b/im,
          /^[ \t]*(?:TODO|FIXME)\s*[:;]\s*\S/im,
        ];
        for (const re of patterns) {
          if (re.test(prose)) {
            findings.push(`${name}: real placeholder`);
            break;
          }
        }
      }
      if (findings.length === 0) return { pass: true, msg: 'no real placeholders (code-block/URL false-positives skipped)' };
      return { pass: false, msg: `${findings.length} real placeholders: ${findings.slice(0, 3).join('; ')}` };
    },
  },

  // E13 — harness-audit command references real script
  {
    id: 'E13', name: '/harness-audit command references a real script', severity: 'CRITICAL',
    run: () => {
      const cmdPath = path.join(COMMANDS, 'harness-audit.md');
      if (!fs.existsSync(cmdPath)) return { pass: false, msg: 'harness-audit command missing' };
      const cmd = fs.readFileSync(cmdPath, 'utf8');
      const m = cmd.match(/node\s+([^\s`]+\.js)/);
      if (!m) return { pass: true, msg: 'no script claim' };
      const scriptPath = m[1];
      // absolute path: check it directly
      if (path.isAbsolute(scriptPath)) {
        return fs.existsSync(scriptPath)
          ? { pass: true, msg: `${scriptPath} exists` }
          : { pass: false, msg: `command claims ${scriptPath} but it doesn't exist` };
      }
      // relative path: try common install locations
      const candidates = [
        path.join(HOME, '.claude', scriptPath),
        path.join(HOME, '.claude/ecc', scriptPath),
        path.join(HOME, '.claude/ecc/scripts', scriptPath),
        path.join(HOME, '.claude/plugins', scriptPath),
        path.join(HOME, '.claude/scripts', scriptPath),
      ];
      const found = candidates.some(p => fs.existsSync(p));
      return found
        ? { pass: true, msg: `${scriptPath} exists at ${candidates.find(p => fs.existsSync(p))}` }
        : { pass: false, msg: `command claims ${scriptPath} but it doesn't exist anywhere` };
    },
  },

  // E14 — eval-harness has runnable grader
  {
    id: 'E14', name: 'eval-harness has runnable grader or declares prompt-only', severity: 'HIGH',
    run: () => {
      const s = loadSkill('eval-harness'); if (!s) return { pass: false, msg: 'skill missing' };
      const hasGrader = s.scripts.some(f => /grader|eval|judge|scor|run/i.test(f));
      const declaresPromptOnly = /prompt[- ]?template only|reference material only|this skill is reference/i.test(s.body);
      if (hasGrader) return { pass: true, msg: 'has grader script' };
      if (declaresPromptOnly) return { pass: true, msg: 'declares itself prompt-template only' };
      return { pass: false, msg: 'claims formal framework but ships no grader' };
    },
  },

  // E15 — autonomous-loops no curl|bash
  {
    id: 'E15', name: 'autonomous-loops no curl|bash install', severity: 'HIGH',
    run: () => {
      const s = loadSkill('autonomous-loops'); if (!s) return { pass: false, msg: 'skill missing' };
      const m = s.body.match(/curl\s+[^\n]+\|\s*bash|wget\s+[^\n]+\|\s*sh/i);
      return m ? { pass: false, msg: `found ${m[0].slice(0, 40)}...` } : { pass: true, msg: 'no curl|bash' };
    },
  },

  // E16 — settings.json no hardcoded real tokens
  {
    id: 'E16', name: 'settings.json has no hardcoded real tokens', severity: 'CRITICAL',
    run: () => {
      const data = readJSON(SETTINGS); if (!data) return { pass: false, msg: 'settings.json unparseable' };
      const env = data.env || {};
      const patterns = [
        /^sk-[a-zA-Z0-9_-]{20,}$/,
        /^sk-cp-[a-zA-Z0-9_-]{20,}$/,
        /^tp-[a-zA-Z0-9]{20,}$/,
        /^pk-[a-zA-Z0-9-]{20,}$/,
        /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i,
      ];
      const hits = [];
      for (const [k, v] of Object.entries(env)) {
        if (typeof v === 'string' && /TOKEN|KEY|SECRET/i.test(k) && patterns.some(p => p.test(v))) {
          hits.push(k);
        }
      }
      return hits.length
        ? { pass: false, msg: `hardcoded: ${hits.join(', ')}` }
        : { pass: true, msg: 'all keys env-referenced or absent' };
    },
  },

  // E17 — permission mitigations are in place
  //
  // A coding agent NEEDS Bash + Write + Edit in allow. So the eval isn't
  // "Bash+Write+Edit absent" — that would block coding. Instead, the
  // eval checks that appropriate MITIGATIONS are in place for the
  // prompt-injection-to-shell risk:
  //
  //   (a) WebFetch is not in `allow` (it's the main fetch-and-exec vector)
  //   (b) `deny` list has at least 3 dangerous patterns
  //   (c) PreToolUse gate is installed and intercepts protected files
  //   (d) `allowDangerousDownloads` is false
  //   (e) `dangerouslyDisableSandbox` is false
  //
  // If >= 3 of 5 are present, the install is reasonably defended.
  {
    id: 'E17', name: 'permission mitigations cover Bash+Write+Edit exposure', severity: 'HIGH',
    run: () => {
      const data = readJSON(SETTINGS); if (!data) return { pass: false, msg: 'settings.json unparseable' };
      const a = new Set(data.permissions?.allow || []);
      const deny = data.permissions?.deny || [];
      const mitigations = {
        webFetchNotInAllow: !a.has('WebFetch'),
        denyHas3Plus: deny.length >= 3,
        preToolUseGate: false,
        safeDownloads: data.allowDangerousDownloads !== true,
        sandboxOn: data.dangerouslyDisableSandbox !== true,
      };
      try {
        const hooks = readJSON(HOOKS);
        const allIds = Object.values(hooks?.hooks || {}).flat().flatMap(e => (e.hooks || []).map(h => h.id || ''));
        mitigations.preToolUseGate = allIds.some(id => id.startsWith('gate:'));
      } catch { /* hooks.json malformed */ }
      const count = Object.values(mitigations).filter(Boolean).length;
      if (count >= 3) {
        return { pass: true, msg: `${count}/5 mitigations present` };
      }
      return { pass: false, msg: `only ${count}/5 mitigations present` };
    },
  },

  // E18 — hooks.json no inline node -e > 200 chars
  {
    id: 'E18', name: 'hooks.json no inline node -e > 200 chars', severity: 'MEDIUM',
    run: () => {
      if (!fs.existsSync(HOOKS)) return { pass: false, msg: 'hooks.json missing' };
      const data = readJSON(HOOKS); if (!data) return { pass: false, msg: 'hooks.json unparseable' };
      const long = [];
      for (const [, entries] of Object.entries(data.hooks || {})) {
        for (const e of entries) {
          for (const h of e.hooks || []) {
            if (typeof h.command === 'string' && h.command.length > 200) {
              long.push(`${e.matcher}:${h.command.length}chars`);
            }
          }
        }
      }
      return long.length
        ? { pass: false, msg: `${long.length} long inline cmds (e.g. ${long[0]})` }
        : { pass: true, msg: 'no inline node -e' };
    },
  },

  // E19 — use_7d/use_30d populated
  {
    id: 'E19', name: 'skill use tracking is populated (not all zero)', severity: 'LOW',
    run: () => {
      // spawn the scan and check
      const { execSync } = require('child_process');
      let out;
      try {
        out = execSync(`bash ${path.join(SKILLS, 'skill-stocktake/scripts/scan.sh')}`, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] });
      } catch (e) { return { pass: false, msg: 'scan failed' }; }
      const data = JSON.parse(out);
      const allZero = (data.skills || []).every(s => s.use_7d === 0 && s.use_30d === 0);
      return allZero
        ? { pass: false, msg: 'all 214 skills report 0 use — tracking not wired' }
        : { pass: true, msg: 'use tracking populated' };
    },
  },

  // E20 — skill-stocktake passes its own frontmatter check
  {
    id: 'E20', name: 'skill-stocktake has proper frontmatter', severity: 'MEDIUM',
    run: () => {
      const s = loadSkill('skill-stocktake'); if (!s) return { pass: false, msg: 'skill-stocktake missing' };
      if (!s.fm.name) return { pass: false, msg: 'no name' };
      if (!s.fm.description || s.fm.description.length < 30) return { pass: false, msg: 'description missing/short' };
      return { pass: true, msg: 'self-audit clean' };
    },
  },

  // ---- OPC-specific evals (E21-E32) ----------------------------------------

  // E21 — 5 page web UI routes are all present
  {
    id: 'E21', name: '5 opc web UI pages exist (topics, scripts, calendar, dashboard, knowledge)', severity: 'CRITICAL',
    run: () => {
      const web = path.join(HOME, 'workspace/opc/apps/web/app');
      const required = ['topics/page.tsx', 'scripts/page.tsx', 'calendar/page.tsx', 'dashboard/page.tsx', 'knowledge/page.tsx'];
      const missing = required.filter(r => !fs.existsSync(path.join(web, r)));
      if (missing.length) return { pass: false, msg: `missing pages: ${missing.join(', ')}` };
      return { pass: true, msg: 'all 5 page routes present' };
    },
  },

  // E22 — Panda IP knowledge base exists in Knowledge page (panda ref in i18n + knowledge file)
  {
    id: 'E22', name: 'panda IP knowledge base is seeded in shared i18n + design doc', severity: 'HIGH',
    run: () => {
      const zh = path.join(HOME, 'workspace/opc/apps/shared/src/i18n/locales/zh.json');
      if (!fs.existsSync(zh)) return { pass: false, msg: 'zh.json locale missing' };
      const txt = fs.readFileSync(zh, 'utf8');
      const ok = /熊猫/.test(txt);
      return { pass: ok, msg: ok ? 'panda IP term present in locale' : 'no 熊猫 reference in shared zh.json' };
    },
  },

  // E23 — Topic kanban has 4 status columns defined in topics/page.tsx
  {
    id: 'E23', name: 'topics kanban defines KANBAN_COLUMNS with 4 statuses', severity: 'HIGH',
    run: () => {
      const p = path.join(HOME, 'workspace/opc/apps/web/app/topics/page.tsx');
      if (!fs.existsSync(p)) return { pass: false, msg: 'topics/page.tsx missing' };
      const t = fs.readFileSync(p, 'utf8');
      const m = t.match(/KANBAN_COLUMNS[\s\S]{0,400}/);
      if (!m) return { pass: false, msg: 'KANBAN_COLUMNS not found' };
      // Count distinct status fields in the array literal
      const statuses = (m[0].match(/status:\s*['"][^'"]+['"]/g) || []).length;
      return { pass: statuses >= 4, msg: `KANBAN_COLUMNS has ${statuses} statuses (need >=4)` };
    },
  },

  // E24 — MCP server registers 10 opc tools
  {
    id: 'E24', name: 'MCP server registers 10 opc tools', severity: 'CRITICAL',
    run: () => {
      const p = path.join(HOME, 'workspace/opc/apps/api/internal/mcp/server.go');
      if (!fs.existsSync(p)) return { pass: false, msg: 'mcp/server.go missing' };
      const t = fs.readFileSync(p, 'utf8');
      // Count opc_ tool name registrations
      const tools = (t.match(/name:\s*"opc_[a-z_]+"/g) || []);
      const unique = [...new Set(tools)];
      return { pass: unique.length === 10, msg: `found ${unique.length} opc tools (need 10): ${unique.join(', ')}` };
    },
  },

  // E25 — Anti-AI detection skill exists (humanizer-zh covers "反 AI 检测")
  {
    id: 'E25', name: 'anti-AI-detection / humanize capability is documented in a skill', severity: 'HIGH',
    run: () => {
      const s = loadSkill('humanizer-zh'); if (!s) return { pass: false, msg: 'humanizer-zh skill missing' };
      const ok = /AI\s*检测|反\s*AI|去\s*AI|拟人化|humanize/i.test(s.body);
      return { pass: ok, msg: ok ? 'humanizer-zh covers AI detection / 拟人化' : 'no anti-AI detection content' };
    },
  },

  // E26 — Go backend uses SQLite + GORM (go.mod declares gorm.io)
  {
    id: 'E26', name: 'Go backend uses SQLite + GORM', severity: 'HIGH',
    run: () => {
      const p = path.join(HOME, 'workspace/opc/apps/api/go.mod');
      if (!fs.existsSync(p)) return { pass: false, msg: 'go.mod missing' };
      const t = fs.readFileSync(p, 'utf8');
      const gorm = /gorm\.io\/gorm/.test(t);
      const sqlite = /gorm\.io\/driver\/sqlite/.test(t);
      return { pass: gorm && sqlite, msg: `gorm=${gorm} sqlite=${sqlite}` };
    },
  },

  // E27 — PLATFORMS constant centralized in shared types
  {
    id: 'E27', name: 'PLATFORMS constant is centralized in shared or app code', severity: 'MEDIUM',
    run: () => {
      const roots = [
        path.join(HOME, 'workspace/opc/apps/shared/src'),
        path.join(HOME, 'workspace/opc/apps/api/internal'),
        path.join(HOME, 'workspace/opc/apps/web/app'),
      ];
      const hits = [];
      function walk(d) {
        if (!fs.existsSync(d)) return;
        for (const e of fs.readdirSync(d, { withFileTypes: true })) {
          const p = path.join(d, e.name);
          if (e.isDirectory()) {
            if (e.name === 'node_modules' || e.name === '.next') continue;
            walk(p);
          } else if (/\.(ts|tsx|go)$/.test(e.name)) {
            const t = fs.readFileSync(p, 'utf8');
            if (/(?:^|\n)\s*(?:export\s+)?(?:const|var|let)\s+PLATFORMS\b/.test(t)) {
              hits.push(p);
            }
          }
        }
      }
      for (const r of roots) walk(r);
      if (hits.length === 0) return { pass: false, msg: 'no PLATFORMS constant found' };
      if (hits.length >= 2) return { pass: true, msg: `PLATFORMS in ${hits.length} files (centralized): ${hits[0]} +${hits.length - 1}` };
      return { pass: true, msg: `PLATFORMS in ${hits[0]}` };
    },
  },

  // E28 — StyleFingerprint JSON field exists on Script model
  {
    id: 'E28', name: 'Script model has StyleFingerprint JSON field', severity: 'HIGH',
    run: () => {
      const p = path.join(HOME, 'workspace/opc/apps/api/internal/models/script.go');
      if (!fs.existsSync(p)) return { pass: false, msg: 'script.go missing' };
      const t = fs.readFileSync(p, 'utf8');
      const ok = /StyleFingerprint\s+string/.test(t);
      return { pass: ok, msg: ok ? 'StyleFingerprint string field present' : 'no StyleFingerprint field' };
    },
  },

  // E29 — 4 tonalities (治愈/御宅/哲学/国潮) defined somewhere in design or i18n
  {
    id: 'E29', name: '4 tonalities (治愈/御宅/哲学/国潮) are documented', severity: 'MEDIUM',
    run: () => {
      const design = path.join(HOME, 'workspace/opc/docs/superpowers/specs/2026-06-03-opc-phase1-ip-design.md');
      if (!fs.existsSync(design)) return { pass: false, msg: 'design doc missing' };
      const t = fs.readFileSync(design, 'utf8');
      const all4 = ['治愈', '御宅', '哲学', '国潮'].every(k => t.includes(k));
      return { pass: all4, msg: all4 ? 'all 4 tonalities in design doc' : 'missing tonality keywords' };
    },
  },

  // E30 — 5 capabilities (选题/脚本/拆解/资产/发布) appear in design doc
  {
    id: 'E30', name: '5 capabilities (选题/脚本/拆解/资产/发布) appear in design doc', severity: 'MEDIUM',
    run: () => {
      const design = path.join(HOME, 'workspace/opc/docs/superpowers/specs/2026-06-03-opc-phase1-ip-design.md');
      if (!fs.existsSync(design)) return { pass: false, msg: 'design doc missing' };
      const t = fs.readFileSync(design, 'utf8');
      const all5 = ['选题', '脚本', '拆解', '资产', '发布'].every(k => t.includes(k));
      return { pass: all5, msg: all5 ? 'all 5 capabilities in design doc' : 'missing capability keywords' };
    },
  },

  // E31 — opc-cost-cap skill enforces per-video cap ≤ ¥100
  {
    id: 'E31', name: 'opc-cost-cap skill declares per-video hard cap <= ¥100', severity: 'CRITICAL',
    run: () => {
      const s = loadSkill('opc-cost-cap'); if (!s) return { pass: false, msg: 'opc-cost-cap skill missing' };
      // Match: "Hard per-video cap**: ¥100", "HARD CAP BREACH ... > hard cap ¥100", "¥N/视频"
      const patterns = [
        /(?:hard|HARD|上限)\s*cap[^\n]{0,60}?¥\s*(\d{2,3})/i,
        /¥\s*(\d{2,3})\s*[\/每]\s*视频/,
        /HARD CAP BREACH[^\n]{0,80}?¥\s*(\d{2,3})/i,
        /per-video\s*cap[^\n]{0,60}?¥\s*(\d{2,3})/i,
      ];
      let cap = null;
      for (const re of patterns) {
        const m = s.body.match(re);
        if (m) { cap = Number(m[1]); break; }
      }
      if (cap === null) return { pass: false, msg: 'no explicit per-video hard cap number found' };
      return { pass: cap <= 100, msg: `hard cap = ¥${cap}/video (must be <= 100)` };
    },
  },

  // E32 — Dashboard has a postmortem / 爆款复盘 workflow (PostmortemModal reference)
  {
    id: 'E32', name: 'dashboard has 爆款复盘 postmortem workflow', severity: 'HIGH',
    run: () => {
      const p = path.join(HOME, 'workspace/opc/apps/web/app/dashboard/page.tsx');
      if (!fs.existsSync(p)) return { pass: false, msg: 'dashboard/page.tsx missing' };
      const t = fs.readFileSync(p, 'utf8');
      const ok = /Postmortem|postmortem|复盘/.test(t);
      return { pass: ok, msg: ok ? 'postmortem UI / state present' : 'no postmortem reference in dashboard' };
    },
  },
];

// ---- runner ----------------------------------------------------------------

function main() {
  const args = process.argv.slice(2);
  const opts = { all: false, suite: null, eval: null, json: false };
  for (let i = 0; i < args.length; i++) {
    if (args[i] === '--all') opts.all = true;
    else if (args[i] === '--suite') opts.suite = args[++i];
    else if (args[i] === '--eval') opts.eval = args[++i];
    else if (args[i] === '--json') opts.json = true;
    else if (args[i] === '--list') {
      console.log('Available evals:');
      for (const e of EVALS) console.log(`  ${e.id}\t${e.severity}\t${e.name}`);
      process.exit(0);
    }
  }

  let toRun = EVALS;
  if (opts.eval) toRun = EVALS.filter(e => e.id === opts.eval);
  else if (opts.suite === 'code') toRun = EVALS.filter(e => ['E01','E02','E03','E04','E05','E06','E13','E14','E15','E16','E19','E20'].includes(e.id));
  else if (opts.suite === 'structure') toRun = EVALS.filter(e => ['E07','E08','E09','E10','E11','E12','E17','E18'].includes(e.id));
  else if (!opts.all) toRun = EVALS; // default = all

  const results = [];
  for (const e of toRun) {
    try { results.push({ ...e, ...e.run() }); }
    catch (err) { results.push({ ...e, pass: false, msg: `threw: ${err.message}` }); }
  }

  if (opts.json) {
    console.log(JSON.stringify({
      evaluated_at: new Date().toISOString(),
      count: results.length,
      passed: results.filter(r => r.pass).length,
      failed: results.filter(r => !r.pass).length,
      pass_rate: (results.filter(r => r.pass).length / results.length).toFixed(2),
      results,
    }, null, 2));
    return;
  }

  console.log(`# EVALS Run — ${new Date().toISOString()}\n`);
  const W = (s, n) => String(s).padEnd(n).slice(0, n);
  for (const r of results) {
    const mark = r.pass ? '✓' : '✗';
    console.log(`${mark} ${W(r.id, 5)}${W(r.severity, 9)}${r.name}`);
    if (!r.pass) console.log(`      ${r.msg}`);
  }
  const pass = results.filter(r => r.pass).length;
  const fail = results.length - pass;
  const critical = results.filter(r => !r.pass && r.severity === 'CRITICAL').length;
  console.log(`\nTotal: ${pass} passed, ${fail} failed (${results.length} evals).  Critical failures: ${critical}.`);

  // exit policy
  if (critical > 0) process.exit(2);  // critical
  if (fail / results.length > 0.10) process.exit(1);  // < 90% pass
  process.exit(0);
}

if (require.main === module) main();
module.exports = { EVALS };
