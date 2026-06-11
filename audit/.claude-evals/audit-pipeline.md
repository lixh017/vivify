## EVAL: audit-pipeline

Smoke eval for the 5 audit frameworks. Re-runnable any time.
Pass = all 4 code-based checks succeed.

### Capability Evals

- [ ] harness-audit.js runs and reports 7 categories
- [ ] run-evals.js runs and reports ≥ 18 of 20 evals passing
- [ ] skill-creator has init_skill.py in scripts/
- [ ] skill-stocktake has a name field in frontmatter
- [ ] security-review contains no real-looking tokens

### Regression Evals

- [ ] No settings.json has a hardcoded real API token
- [ ] hooks.json has no inline `node -e` longer than 200 chars
- [ ] At least 5 of the 6 settings variants use env-var references
- [ ] The eval-harness skill has at least one .js file in scripts/

### Checks

- file:/root/workspace/opc/audit/harness-audit.js:exists
- file:/root/workspace/opc/audit/run-evals.js:exists
- file:/root/.claude/skills/skill-creator/scripts/init_skill.py:exists
- file:/root/.claude/skills/skill-creator/scripts/package_skill.py:exists
- file:/root/.claude/skills/skill-creator/scripts/quick_validate.py:exists
- file:/root/.claude/skills/eval-harness/scripts/grader.js:exists
- file:/root/.claude/skills/skill-stocktake/SKILL.md:contains:name: skill-stocktake
- file:/root/.claude/skills/security-review/references/appendix.md:contains:hardcoded secrets
- file:/root/.claude/settings.json:contains:${ANTHROPIC_AUTH_TOKEN}
- file:/root/.claude/settings.json.deepseek:contains:${ANTHROPIC_AUTH_TOKEN}
- file:/root/.claude/settings.json.jd:contains:${ANTHROPIC_AUTH_TOKEN}
- file:/root/.claude/settings.json.minimax:contains:${ANTHROPIC_AUTH_TOKEN}
- file:/root/.claude/settings.json.xiaomi:contains:${ANTHROPIC_AUTH_TOKEN}
