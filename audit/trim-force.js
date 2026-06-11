'use strict';
const fs = require('fs');
const path = require('path');
const SKILLS = '/root/.claude/skills';

// FORCE trim all skills > 200 lines, including ones that already have appendix
// (we'll APPEND to the existing appendix file or create a new section)
const TARGET = 130;

const TARGETS = [];
for (const d of fs.readdirSync(SKILLS)) {
  if (d.startsWith('.')) continue;
  const p = path.join(SKILLS, d, 'SKILL.md');
  if (!fs.existsSync(p)) continue;
  const lines = fs.readFileSync(p, 'utf8').split('\n').length;
  if (lines > 140) TARGETS.push(d);
}
console.log(`Force-trim: ${TARGETS.length} skills > ${TARGET} lines`);

function trim(skillName) {
  const skillPath = path.join(SKILLS, skillName, 'SKILL.md');
  const text = fs.readFileSync(skillPath, 'utf8');
  const lines = text.split('\n');
  const before = lines.length;
  if (before <= TARGET) return;

  // HARD cut at TARGET. If we land mid-section, we add a divider + move whole sections.
  let cut = TARGET;
  // If line TARGET happens to be inside a section (not at a ## boundary),
  // jump back to the previous ## so we cut at a section boundary.
  for (let i = TARGET; i > 0; i--) {
    if (lines[i] && lines[i].startsWith('## ')) { cut = i; break; }
  }

  const moved = lines.slice(cut).join('\n').trim();
  const kept = lines.slice(0, cut);
  const refDir = path.join(SKILLS, skillName, 'references');
  fs.mkdirSync(refDir, { recursive: true });
  const appendixPath = path.join(refDir, 'appendix.md');
  if (moved) {
    fs.appendFileSync(appendixPath, '\n\n---\n\n' + moved + '\n');
  }
  // Only add the "## References" footer if not already present in kept
  const hasFooter = kept.join('\n').match(/^##\s+References\s*$/m);
  const closing = hasFooter ? [] : [
    '',
    '## References',
    '',
    `- [\`references/appendix.md\`](references/appendix.md) — full reference material extracted from this skill`,
    '',
  ];
  fs.writeFileSync(skillPath, kept.join('\n') + closing.join('\n'));
  console.log(`  ${skillName}: ${before} → ${kept.length + closing.length}`);
}

for (const s of TARGETS) trim(s);
