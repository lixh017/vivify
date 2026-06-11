# EVALS — Skills & Harness Eval Baseline

> This is the **real** eval baseline the rest of ECC should be using.
> The existing `eval-harness` skill is documentation-only; this file is
> the executable contract: each eval has a grader, a fixture, and a
> pass/fail rule that can be machine-checked.

## How to run

```bash
# Run the code-based evals
node run-evals.js --suite code
# Run the schema/structure evals
node run-evals.js --suite structure
# Run everything
node run-evals.js --all
# Just one eval
node run-evals.js --eval E04
```

Exits non-zero on any failure. Append `--json` for machine-readable output.

## Eval table

| ID | Skill / Surface | Type | What it verifies | Grader |
|----|-----------------|------|------------------|--------|
| E01 | tdd-workflow | code | Body enforces "test was actually RUN", not just "test was written" | regex |
| E02 | tdd-workflow | code | 80% coverage threshold is mentioned as a target, not a goal | regex |
| E03 | tdd-workflow | structure | Body has explicit RED / GREEN / REFACTOR sections | heading |
| E04 | security-review | code | Body never contains a real-looking token (`sk-XXX{20,}`) | regex |
| E05 | security-review | code | Body does NOT recommend `eval()` or unescaped `innerHTML` | regex |
| E06 | security-review | code | All "FAIL:" examples have a matching "PASS:" / "CORRECT:" counterpart | parser |
| E07 | frontend-design | code | Description names a specific trigger phrase, not just "frontend" | regex |
| E08 | frontend-design | code | Anti-patterns section exists with ≥3 bullets | parser |
| E09 | skill-creator | structure | `scripts/init_skill.py` and `scripts/package_skill.py` exist | file |
| E10 | skill-creator | code | Description contains "Use when..." or equivalent trigger | regex |
| E11 | all skills | code | Frontmatter has name + description, description length ≥30 | parser |
| E12 | all skills | structure | No `TODO|FIXME|TBD` in shipped body | regex |
| E13 | harness-audit (cmd) | code | The `node scripts/harness-audit.js` referenced in the command actually exists | file |
| E14 | eval-harness | code | Has at least one runnable grader in `scripts/` (or declares itself prompt-template-only) | file/regex |
| E15 | autonomous-loops | code | Does NOT contain `curl ... | bash` install pattern (violates its own claim) | regex |
| E16 | settings.json | code | No hardcoded real tokens matching sk-/sk-cp-/tp-/pk-/UUID patterns | regex |
| E17 | settings.json | code | `permissions.allow` does not include `Bash` + `Write` + `Edit` simultaneously | parser |
| E18 | hooks.json | structure | No `node -e` inline command longer than 200 chars (forces refactor to real files) | length |
| E19 | all skills | code | `use_7d` / `use_30d` data is populated for installed skills (not all zero) | file |
| E20 | skill-stocktake | code | Self-evaluation: does skill-stocktake pass its own frontmatter check? | parser |

## Eval definitions (executable)

Each eval below is the source-of-truth. The grader code in `run-evals.js` matches these definitions.

### E01 — tdd-workflow enforces test execution

```yaml
id: E01
skill: tdd-workflow
type: regex
assert: "A test that was only written but not compiled and executed does not count"
  # or any phrasing that explicitly forbids "test was written but not run"
fail_message: "tdd-workflow must explicitly require tests be RUN, not just written"
```

### E04 — security-review contains no real-looking token

```yaml
id: E04
skill: security-review
type: regex
pattern: '\bsk-[a-zA-Z0-9_-]{20,}\b'
expected: 0 matches
fail_message: "Found a real-looking API token in security-review body. Use 'sk-proj-xxxxx' or 'sk-...' as placeholder."
```

### E13 — harness-audit command references an existing script

```yaml
id: E13
target: ~/.claude/commands/harness-audit.md
type: file
assert: the path captured from `node <SCRIPT>` in the command body exists on disk
fail_message: "/harness-audit references scripts/harness-audit.js which does not exist. Either build the engine or fix the doc."
```

### E14 — eval-harness has a runnable grader

```yaml
id: E14
skill: eval-harness
type: combined
checks:
  - has scripts/ directory with at least one executable grader
  - OR the body declares itself as prompt-template-only (e.g. "this skill is reference material only")
fail_message: "eval-harness promises a 'formal evaluation framework' but ships no runnable grader."
```

### E16 — settings.json has no hardcoded real tokens

```yaml
id: E16
target: ~/.claude/settings.json
type: regex
patterns:
  - 'sk-[a-zA-Z0-9_-]{20,}'
  - 'sk-cp-[a-zA-Z0-9_-]{20,}'
  - 'tp-[a-zA-Z0-9]{20,}'
  - 'pk-[a-zA-Z0-9-]{20,}'
  - '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}'
expected: 0 matches in any env value
fail_message: "Real-looking API token hardcoded in settings.json. Use \${ANTHROPIC_AUTH_TOKEN} and set the env var in your shell profile."
```

### E17 — permissions.allow is not maximally permissive

```yaml
id: E17
target: ~/.claude/settings.json
type: parser
assert: NOT (Bash IN allow AND Write IN allow AND Edit IN allow)
fail_message: "permissions.allow contains Bash + Write + Edit — high blast radius for prompt injection."
recommended_fix: 'Move Write/Edit to "ask" so the user is prompted.'
```

### E18 — hooks.json commands are not inline node -e

```yaml
id: E18
target: ~/.claude/hooks/hooks.json
type: structural
assert: no command field longer than 200 chars OR starts with "node -e \""
fail_message: "Inline node -e commands of >200 chars are unmaintainable. Extract to scripts/hooks/<name>.js."
```

### E19 — use_7d / use_30d tracking actually works

```yaml
id: E19
target: scan output from skill-stocktake/scripts/scan.sh
type: data
assert: not all 214 skills have use_7d=0 AND use_30d=0
fail_message: "Use tracking is not wired up. Cannot make evidence-based retire decisions."
```

### E20 — skill-stocktake passes its own frontmatter check

```yaml
id: E20
skill: skill-stocktake
type: parser
assert: frontmatter.name == 'skill-stocktake' AND frontmatter.description.length >= 30
fail_message: "The skill that audits skills fails its own frontmatter check."
```

## Eval cadence

| Trigger | Suite to run |
|---------|--------------|
| Before any new skill ships | E11, E12, E20 |
| Before any new settings.json is committed | E16, E17 |
| Before any hooks.json is modified | E18 |
| Monthly skill-stocktake | All E01–E20 |
| After any change to tdd-workflow | E01, E02, E03 |
| After any change to security-review | E04, E05, E06 |
| After any change to frontend-design | E07, E08 |

## Pass criteria

- **Code evals (E01–E20)**: pass rate ≥ 18/20 = 90% for a release
- **E16 must always pass** (no hardcoded secrets — non-negotiable)
- **E04 must always pass** (security-review itself must be clean)
- **E13 must always pass** OR `/harness-audit` command must be removed/fixed

## Anti-patterns (these are the eval smells to avoid)

- Evals that only check that text exists, not that it works (e.g. "does the body mention pass@k" — too easy to game)
- Evals that require human grading for every run (no automation, no value)
- Evals with no `fail_message` (operators can't act on the result)
- Evals that overlap so much with structure that they always pass together (uncorrelated metrics are useless)
