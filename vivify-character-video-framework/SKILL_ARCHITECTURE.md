# Skill Architecture — 点睛 / Vivify Platform

> **What goes where: skills orchestrate CLI, CLI executes. Don't blur layers.**

## The principle

The framework has **two execution surfaces**:

1. **`vivify` CLI** — the engine. Runs on disk. Touches DB, calls APIs, computes cost, handles retries. Built and verified.
2. **Skills** — the agent's playbook. Read by Claude Code / openclaw / codex. Tell the agent WHEN to run which CLI command and HOW to interpret results.

**Skills never duplicate CLI work.** A skill that re-implements an API
call is wrong. A skill that adds a CLI command is also wrong — that
work belongs in the CLI.

## The 4 layers

```
┌─────────────────────────────────────────────────────────────┐
│ L4  INTENT — "user wants X"                                  │
│     → vivify-character-video  (orchestrator, 30 lines)          │
│     → Asks 5 questions, decides which L3 skill to dispatch   │
├─────────────────────────────────────────────────────────────┤
│ L3  SCENARIO — "user wants X, here's the recipe"            │
│     → vivify-panda-new-ip, vivify-panda-episode-build,             │
│       vivify-panda-episode-publish                             │
│     → Each scenario: input contract + sequence of vivify     │
│       CLI commands + how to read results + recovery          │
├─────────────────────────────────────────────────────────────┤
│ L2  — (reserved for future capability abstractions)          │
│     → When the agent often thinks "make me an image"         │
│       without caring which provider: add an L2 skill.        │
│     → Today: not needed, L3 directly invokes CLI.            │
├─────────────────────────────────────────────────────────────┤
│ L1  ENGINE — `vivify` CLI                                    │
│     → 9 subcommands: character / lesson / asset / episode /  │
│       cost / memory / publish / workflow / db                │
│     → Owns: DB writes, API calls, cost tracking, retry      │
│       classification, provider routing, schema migration     │
├─────────────────────────────────────────────────────────────┤
│ L0  EXTERNAL — Ark / MiniMax / Suno / 抖音 APIs              │
└─────────────────────────────────────────────────────────────┘
```

## Decision rules — where does a new skill go?

Ask: **"Is this skill running on top of the CLI, or replacing it?"**

- **Running on top** (calls vivify, doesn't reimplement) → it's L3 or L4
- **Replacing it** (calls APIs directly, ignores vivify) → it's wrong

Then: **"Is it the user's entry point, or a scenario inside it?"**

- **Entry point** (triggered by "make me a video") → L4
- **Scenario** (triggered by something more specific) → L3

Then: **"Does it produce one or many outcomes?"**

- **One scenario, one CLI sequence** → L3 skill
- **Multiple scenarios, dispatch logic** → L4 skill (orchestrator)

## Skills that already exist (legacy, do not duplicate)

| Skill | Status | Layer |
|---|---|---|
| `vivify-panda-character` | Reference (read-only). IP bible. | L3 (read-only) |
| `vivify-script-generation` | Could be folded into `vivify-panda-episode-build`. Keep as long as scripts are still useful standalone. | L3 |
| `vivify-scene-decomposition` | Same — fold into episode-build if needed. | L3 |
| `vivify-asset-orchestrator` | **Repurpose** to call `vivify asset` instead of duplicating. | L3 |
| `vivify-asset-router` | Could call `vivify cost estimate` + provider picker. | L3 |
| `vivify-provider-volcengine` | Direct curl provider skill. Keep — agents can fall back to it when L1 CLI doesn't suffice. | L0 transport (legacy, kept) |
| `vivify-platform-adaptation` | Pure reference (format rules per platform). | L3 reference |
| `vivify-cost-cap` | Policy reference. Cited by episode-build, render, etc. | L3 |
| `panda-episode-pipeline` | **Old** end-to-end skill. **Deprecate** in favor of `vivify-panda-episode-build` + `vivify episode render`. | (deprecated) |
| `mmx-video-gen` | L0 transport for MiniMax. Cited by episode-build. | L0 transport |

## Skills we just added (this session)

| Skill | Layer | Purpose |
|---|---|---|
| `vivify-character-video` | **L4** | Top-level entry. Asks 5 questions, dispatches. |
| `vivify-panda-new-ip` | L3 | Create new IP + canonical images via `vivify character add`. |
| `vivify-panda-episode-build` | L3 | Generate storyboard/script + `vivify episode add` + `vivify episode render`. |
| `vivify-panda-episode-publish` | L3 | `vivify publish publish` + analytics. |

## Naming convention

- All skills start with `vivify-` (matches platform brand: 点睛 / Vivify)
- L4 orchestrators: `vivify-character-<verb>` (e.g., `vivify-character-video`)
- L3 domain: `vivify-<domain>-<scenario>` (e.g., `vivify-panda-episode-build`)
- L2 capability: `vivify-gen-<thing>` (e.g., `vivify-gen-image`) — **reserved for future**
- L1 transport / direct API: `vivify-provider-<vendor>-<capability>` (e.g., `vivify-provider-volcengine`)
- L0 transport (HTTP helpers): no naming convention yet, just descriptive

## Anti-patterns (don't do this)

| Anti-pattern | Why wrong |
|---|---|
| Skill that calls `curl` directly to Ark | Bypasses cost cap, retry, DB tracking. |
| Skill that re-implements `vivify episode render` | Duplicate logic, will drift. |
| Skill with >100 lines SKILL.md | Not progressive disclosure. Move detail to `references/`. |
| L4 skill that has implementation details | L4 is decision tree only. Push detail down. |
| L3 skill that doesn't invoke CLI | Should orchestrate, not implement. |

## When to add a new skill

Add a new skill when:
- A user intent pattern repeats 3+ times → wrap as L4 sub-router
- A multi-step scenario (recipe) repeats → wrap as L3 skill
- A provider capability is needed outside CLI → wrap as L0/L1 transport skill

Don't add a skill for:
- One-off scripts → just run them
- Things the CLI already covers → use the CLI
- Documentation → put in `references/` of existing skill

## CLI surface (canonical reference)

```
vivify
├── character   IP registry
├── lesson      knowledge base
├── asset       asset library
├── episode     render lifecycle
├── cost        cost tracking + caps
├── memory      cross-IP knowledge (migrated from md files)
├── publish     publish + analytics (stub for real 抖音)
├── workflow    parallel + smart retry
└── db          migrations + inspect
```

Every skill should use these 9 surfaces. If a skill needs something
not in this list, add a new vivify subcommand first, then reference
it from the skill.
