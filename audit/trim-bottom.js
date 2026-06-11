'use strict';
const fs = require('fs');
const path = require('path');
const SKILLS = '/root/.claude/skills';

const TARGET_LINES = 150;  // aim for each skill to be ≤ 220 lines

// Find skills still > TARGET_LINES that don't have an appendix already
const TARGETS = [];
for (const d of fs.readdirSync(SKILLS)) {
  if (d.startsWith('.')) continue;
  const p = path.join(SKILLS, d, 'SKILL.md');
  if (!fs.existsSync(p)) continue;
  const refsDir = path.join(SKILLS, d, 'references');
  if (fs.existsSync(path.join(refsDir, 'appendix.md'))) continue;
  const lines = fs.readFileSync(p, 'utf8').split('\n').length;
  if (lines > TARGET_LINES) TARGETS.push(d);
}
console.log(`Targets: ${TARGETS.length} skills > ${TARGET_LINES} lines`);

function trim(skillName) {
  const skillPath = path.join(SKILLS, skillName, 'SKILL.md');
  const text = fs.readFileSync(skillPath, 'utf8');
  const lines = text.split('\n');
  const before = lines.length;
  if (before <= TARGET_LINES) {
    console.log(`  skip ${skillName}: only ${before} lines`);
    return;
  }

  // Cut at first `## ` header that brings us under TARGET_LINES
  let cut = TARGET_LINES;
  for (let i = cut; i < lines.length; i++) {
    if (lines[i].startsWith('## ')) { cut = i; break; }
  }

  const moved = lines.slice(cut).join('\n');
  const kept = lines.slice(0, cut);
  const refDir = path.join(SKILLS, skillName, 'references');
  fs.mkdirSync(refDir, { recursive: true });
  fs.writeFileSync(path.join(refDir, 'appendix.md'), moved.trim() + '\n');

  const closing = [
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
