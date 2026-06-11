/**
 * trim-long-skills.js — For each target skill, find a "reference-style"
 * section near the bottom (Common Patterns / Examples / Real-World /
 * Reference Implementation / API Reference / etc.) and move it to
 * references/<name>.md, leaving a one-line pointer.
 *
 * Targets the 5 longest skills that haven't been cut yet.
 */
'use strict';

const fs = require('fs');
const path = require('path');

const SKILLS = '/root/.claude/skills';
const TARGETS = [
  'data-scraper-agent',
  'python-patterns',
  'django-patterns',
  'django-tdd',
  'cpp-coding-standards',
];

// Patterns that mark a section as "extracted reference material"
const EXTRACTABLE = /^(#{2,3})\s+(Common\s+(?:Scraping\s+)?Patterns?|Examples?|Reference\s+Implementation|Real[-\s]World\s+Examples?|API\s+Reference|Common\s+Anti[-\s]?Patterns?|Code\s+Examples?|Type\s+Reference|API\s+Surface|Standard\s+Library\s+Reference|Cheat\s+Sheet|Quick\s+Reference\s+Table|Standard\s+Idioms|Module\s+Reference|Configuration\s+Reference|Framework\s+Specific\s+Patterns?|Language[-\s]Specific\s+Notes?|Glossary|Complete\s+API\s+List|Built[-\s]in\s+Reference|All\s+(?:the\s+)?Examples?\b)/i;

function trim(skillName) {
  const skillPath = path.join(SKILLS, skillName, 'SKILL.md');
  if (!fs.existsSync(skillPath)) {
    console.log(`  skip ${skillName}: no SKILL.md`);
    return;
  }
  const text = fs.readFileSync(skillPath, 'utf8');
  const lines = text.split('\n');
  const before = lines.length;

  // Find all extractable section starts
  const candidates = [];
  for (let i = 0; i < lines.length; i++) {
    const m = lines[i].match(EXTRACTABLE);
    if (m) candidates.push({ line: i, level: m[1], title: lines[i] });
  }
  if (candidates.length === 0) {
    console.log(`  skip ${skillName}: no extractable section found`);
    return;
  }

  // Take the LAST extractable section (deepest into the doc = most "extras")
  const target = candidates[candidates.length - 1];
  const start = target.line;
  // Find end: next ## or --- (frontmatter) at the same or shallower level
  let end = lines.length;
  const minLevel = target.level.length;
  for (let i = start + 1; i < lines.length; i++) {
    const h = lines[i].match(/^(#{1,3})\s+/);
    if (h && h[1].length <= minLevel) {
      end = i;
      break;
    }
  }
  const sectionContent = lines.slice(start, end).join('\n');
  // Strip the leading heading line and the trailing --- if any
  const refName = target.title
    .replace(/^#+\s+/, '')
    .replace(/[^a-z0-9]+/gi, '-')
    .toLowerCase()
    .replace(/^-|-$/g, '');
  const refPath = path.join(SKILLS, skillName, 'references', `${refName}.md`);
  fs.mkdirSync(path.dirname(refPath), { recursive: true });
  fs.writeFileSync(refPath, sectionContent.trim() + '\n');

  // Replace in SKILL.md with a 1-line pointer
  const newLines = [
    ...lines.slice(0, start),
    `${target.level} ${target.title.replace(/^#+\s+/, '')}`,
    '',
    `See [\`references/${refName}.md\`](references/${refName}.md) for the full reference content.`,
    '',
    ...lines.slice(end),
  ];
  fs.writeFileSync(skillPath, newLines.join('\n'));
  const after = newLines.length;
  console.log(`  ${skillName}: ${before} → ${after} (cut ${before - after} lines, moved "${target.title.replace(/^#+\s+/, '')}")`);
}

console.log('Trimming long skills...');
for (const s of TARGETS) trim(s);
