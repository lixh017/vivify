# opc Completion Pass Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Verify the rest of the opc project (apps/web, apps/console, apps/shared builds + API smoke test) is actually working, document the gaps that remain, and confirm everything via an independent verify-project-state agent.

**Architecture:** Sequential verification with explicit pass/fail per task. Each task produces a record (PASS/FAIL/STILL_UNVERIFIED) that updates PROJECT-STATE.md. The verify-project-state agent is dispatched ONCE at the end (Task 7) to confirm all the records are accurate. No "I think it works" — every claim has a command that was actually run.

**Tech Stack:** Go 1.25, Next.js 14.2.5, TypeScript 5.5, pnpm 9.0, Gin, GORM, SQLite (default), and the opc-internal agent layer (`*agents.MiniMax`).

**Hard rules (from PROJECT-STATE.md):**
1. **Read-before-act**: cat `~/.claude/PROJECT-STATE.md` before starting
2. **Verify-before-done**: independent `verify-project-state` agent must return PASS before declaring done (one final dispatch at the end of this plan)
3. **Gap-strict**: any new gap discovered → append to `~/.claude/GAP-LOG.md` immediately
4. **Scope-cry**: don't expand scope; user authorized B2-B5 only, nothing else

---

## File Structure

**Files created by this plan:**
- `~/.claude/DEFINITION-OF-DONE.md` — what "done" means for opc
- `~/.claude/GAP-LOG.md` — append-only gap ledger (initialized with G1-G6)

**Files modified by this plan:**
- `~/.claude/PROJECT-STATE.md` — update C1/C2/C3/C4/C5/C6 based on test results; close or open G-rows accordingly

**Files tested by this plan (read-only):**
- `apps/web/` (build)
- `apps/console/` (build)
- `apps/shared/` (build)
- `apps/api/cmd/server` (run + curl one endpoint)

**File to save plan to:** `docs/superpowers/plans/2026-06-08-opc-completion-pass.md` (this file)

---

## Task 1: Write DEFINITION-OF-DONE.md

**Files:**
- Create: `/root/.claude/DEFINITION-OF-DONE.md`

- [ ] **Step 1.1: Write the file with the full content below**

Use the Write tool. The file content is:

```markdown
# opc — Definition of Done (READ BEFORE DECLARING DONE)

> ⚠️ **Hard rule**: do NOT declare "done" unless every ✅ criterion in the
> relevant phase has a command output that proves it. "I checked the code
> and it looks right" is NOT verification.

## Phase 1 — MVP "panda IP" web UI + 5 pages + MCP

The user-stated definition (verbatim from the 2026-06-08 design discussion):

1. **apps/api Go server** 写真, 能 `go run` 起来, asset 端点返回真文件
2. **apps/web Next.js UI** 改, 有 "生成" 按钮, 按钮调用 server, 能在
   dashboard 看历史
3. **MCP server** 启动, Claude Code 能调 `generate_video` 工具并拿到
   真文件
4. **CI hook** (`.github/workflows/`) 写真, 推 code 触发 regen

## Per-criterion verification commands

| # | Criterion | Verification command (must pass) | NOT a substitute |
|---|-----------|-----------------------------------|------------------|
| 1 | API `go run` 起来 | `cd apps/api && go run ./cmd/server &` then `curl /readyz` returns 200 | reading main.go |
| 1 | asset 端点返回真文件 | `curl /ai/cover?demo=true` returns a non-empty image body or URL | mock file on disk |
| 2 | Web 有"生成"按钮 | `grep -rn "type=\"submit\"\|onClick.*generate" apps/web/app/` finds ≥1 button in a page | "looks like there's a button" |
| 2 | 按钮调 server | the button's handler has a `fetch(` or `axios` call to an `/ai/*` endpoint | stub function |
| 2 | dashboard 看历史 | `curl /api/content-items` (or equivalent) returns a JSON array with ≥1 record | empty array |
| 3 | MCP server 启动 | `go run ./cmd/server` shows "MCP listening" or stdio handshake | code looks right |
| 3 | Claude Code 能调 tool | `echo '{}' \| opc-mcp-client` returns a tool list including `generate_video` | "the tool is registered in code" |
| 4 | CI hook 写真 | `cat .github/workflows/*.yml` exists and has a `jobs:` block | empty file |
| 4 | 推 code 触发 regen | push to a test branch and observe workflow run | "I described the workflow" |

## Anti-patterns — these do NOT count as "done"

(Each of these is something the author has previously mistaken for done. They
are NOT verification.)

- ❌ Hand-rolled one-off `mjs` script that runs once. **Demo, not platform.**
- ❌ Skill written with `frontmatter 6/6`. **Skill quality ≠ platform quality.**
- ❌ Gateway test 7/7. **Tests the harness, not the user-facing surface.**
- ❌ Eval suite 20/20. **Tests LLM behavior in isolation, not the integrated flow.**
- ❌ Audit at 100%. **Tests the meta-system, not the product.**
- ❌ A "looks right" visual review of a page. **The page may render but the
  button may not POST.**
- ❌ A passing `pnpm build` with no UI smoke test. **The bundle may build but
  the button may 500 at click time.**
- ❌ A "Done!" line in a commit message. **Commits don't verify.**

## What "verified" means here

- A command was run in this session
- Its output is quoted in the verification report
- The expected and actual outputs match
- For HTTP endpoints: a `curl` returned the expected status and body shape
- For builds: the build process exited 0 with no errors

## Self-check before declaring "done"

Before saying "done" on any task, ask:

1. Is there a `✅ Verified ✅` row in PROJECT-STATE.md for this task?
2. Does that row have a real `Evidence command` that a fresh agent could
   re-run and reproduce?
3. Was the re-run output identical to the original?
4. If the answer to any of these is "no" or "I don't know" → NOT done.

> "verified" is a status, not a feeling.
```

- [ ] **Step 1.2: Read it back to confirm structure**

Run:
```bash
head -10 /root/.claude/DEFINITION-OF-DONE.md
```
Expected: shows the title + first warning line.

- [ ] **Step 1.3: Count the 4 Phase 1 criteria are present**

Run:
```bash
grep -c "^[0-9]\. \*\*apps/\|^[0-9]\. \*\*MCP server\|^[0-9]\. \*\*CI hook" /root/.claude/DEFINITION-OF-DONE.md
```
Expected: `4` (one match per criterion heading).

- [ ] **Step 1.4: Commit the change**

Run:
```bash
cd /root/workspace/opc && git add /root/.claude/DEFINITION-OF-DONE.md 2>/dev/null; echo "note: file is at /root/.claude/, not in repo; no git commit needed"
```
Expected: the echo line. (This file lives at user-level Claude config, not inside the git repo.)

---

## Task 2: Write GAP-LOG.md with G1-G6 from PROJECT-STATE.md

**Files:**
- Create: `/root/.claude/GAP-LOG.md`

- [ ] **Step 2.1: Write the file with the G1-G6 entries from PROJECT-STATE.md section D, plus 2 closed gaps**

Use the Write tool. The file content is:

```markdown
# opc — Gap Log (APPEND-ONLY, do not delete or edit past entries)

> **Rule**: every gap the user flags (or that an independent verifier
> catches) gets appended here. Each entry has 4 fields: 缺口 (deficit),
> 影响 (impact), 修复计划 (fix plan), status. Status is `open` or
> `closed` — never "in progress" or "wip" (use `open` and put a date in
> the fix plan). Closed gaps stay in the log for audit; do not remove.

---

## 2026-06-08 14:30 (user) — G1: opc 工程平台 0 触碰

- **缺口**: apps/api/、apps/web/、apps/console/ 都没改过 — 无真 UI
  按钮能调起 /ai/* 端点
- **影响**: opc 现在不能让用户点 UI 生成视频；前端和后端虽然在，
  但没有 wire
- **修复计划**: 在 apps/web/app/dashboard/ 写真按钮 → fetch /ai/topics
  → 在 history 区显示响应。P0。
- **status**: open

## 2026-06-08 14:00 (user) — G2: v2 asset layer 是 demo 不是 platform

- **缺口**: `audit/roundtrip-volcengine.mjs` 是 one-off 脚本，不在
  任何调用链上
- **影响**: 重启后能力消失；不是 platform integration
- **修复计划**: 包成 opc-asset CLI，注册到 main.go 的初始化路径，
  让 handler 能直接调
- **status**: open

## 2026-06-08 14:45 (user) — G3: self-assessment 不可信

- **缺口**: Claude 多次自己说 "完成" 后被用户发现漏
- **影响**: 浪费时间 + 用户信任流失
- **修复计划**: PROJECT-STATE.md + verify-project-state agent + 4 hard
  rules（read-before-act / verify-before-done / gap-strict / scope-cry）
- **status**: open (meta — by design, never fully closed)

## 2026-06-08 (objective, observed) — G4: API 当前 0 编译

- **缺口**: go vet 报 2 个 syntax error（deconstruct.go:52 + pipeline.go:53）
  + 5 个 type error（handlers 用 TextProvider 接口，调 .Text() 但接口
  没这方法）
- **影响**: 0 endpoints / 0 MCP tools / 0 tests 可跑
- **修复计划**: 修 2 个 closure + 5 个 handler 字段类型从 TextProvider 改
  *agents.MiniMax
- **status**: **closed 2026-06-08 16:57** — go vet / go build / go test /
  go test -race 全绿，独立 verify agent 确认
  (report: `verify-20260608-165749.md`)

## 2026-06-08 (objective, observed) — G5: apps/web 0 编译 (推断)

- **缺口**: 10 个 apps/web/lib/ 文件被删（api.ts / i18n.ts / types.ts
  / locales/{en,zh}.json 等），但 app/ 页面里没有 `@/lib/...` 的直接
  引用——可能是相对路径或 build-time 引用
- **影响**: 推断 pnpm build 会失败；apps/web 不能跑
- **修复计划**: 跑 pnpm build 看实际错误，再决定是补 lib 还是改 import
- **status**: open → 由本计划 Task 3 关闭

## 2026-06-05 (audit, docs/console-reserve-audit.md) — G6: call_log.cost 全部为 0

- **缺口**: middleware/call_log.go 注释明示 "CostCents is left at 0
  by the middleware. The handler is responsible for stamping the cost"，
  但实际没有任何 handler 覆写 cost
- **影响**: billing dashboard 上线时，所有历史数据全是 0
- **修复计划**: 在 StampClaudeCost 之类的辅助函数里把 cost 写回 call_log；
  或者修 middleware 让它根据 provider 查价
- **status**: open

---

## How to add a new entry

Append below the last `---` separator. Format:

```markdown
## YYYY-MM-DD HH:MM (user|agent|objective) — GX: <short title>

- **缺口**: <what's missing>
- **影响**: <consequence>
- **修复计划**: <concrete steps>
- **status**: open | closed YYYY-MM-DD
```

Never edit a past entry. If a fact changes, add a new entry that
references the old one by ID.
```

- [ ] **Step 2.2: Count entries**

Run:
```bash
grep -c "^## 2026-" /root/.claude/GAP-LOG.md
```
Expected: `6` (G1-G6).

- [ ] **Step 2.3: Verify G4 shows as closed**

Run:
```bash
grep "G4" /root/.claude/GAP-LOG.md | grep closed
```
Expected: one line with "**closed 2026-06-08 16:57**".

---

## Task 3: Verify apps/web builds

**Files modified:** none
**Files read:**
- `/root/workspace/opc/apps/web/package.json` (for context)
- `/root/workspace/opc/apps/web/tsconfig.json` (for context — already inspected; uses `@opc/shared` path alias)

- [ ] **Step 3.1: Sanity check pnpm install state**

Run:
```bash
ls /root/workspace/opc/apps/web/node_modules >/dev/null 2>&1 && echo "INSTALLED" || echo "MISSING"
```
Expected: `INSTALLED` (verified by previous observation that `node_modules` exists in the directory).

- [ ] **Step 3.2: Run pnpm build, capture tail of output**

Run:
```bash
cd /root/workspace/opc/apps/web && pnpm build 2>&1 | tail -50
```
Expected: either
- A "Compiled successfully" / "Route (app)" summary with no errors (PASS)
- A specific error block identifying the missing import or type error (FAIL)

- [ ] **Step 3.3: Capture exit code explicitly**

Run:
```bash
cd /root/workspace/opc/apps/web && pnpm build > /tmp/web-build.log 2>&1; echo "exit=$?"
tail -30 /tmp/web-build.log
```
Expected: `exit=0` for PASS, `exit=1` (or non-zero) for FAIL.

- [ ] **Step 3.4: Update PROJECT-STATE.md based on outcome**

If PASS (exit 0 and no errors):
- Change C1 from "❓" to "✅" with:
  - Date: 2026-06-08
  - Item: "apps/web (Next.js 14.2.5) builds clean"
  - Evidence: `cd apps/web && pnpm build`
  - Output: paste the "Compiled successfully" / route summary line

If FAIL:
- Leave C1 as ❓ (the build is broken, so we cannot claim verification)
- Add a new G7 entry to GAP-LOG.md with the actual error
- Update PROJECT-STATE.md G5 from "推断" to "确认" with the error quote
- Stop and report to the user — do NOT try to fix the web build in this plan (scope-cry)

---

## Task 4: Verify apps/console builds

**Files modified:** none
**Files read:** none new (tsconfig already inspected)

- [ ] **Step 4.1: Sanity check**

Run:
```bash
ls /root/workspace/opc/apps/console/node_modules >/dev/null 2>&1 && echo "INSTALLED" || echo "MISSING"
```
Expected: `INSTALLED`.

- [ ] **Step 4.2: Run pnpm build**

Run:
```bash
cd /root/workspace/opc/apps/console && pnpm build 2>&1 | tail -50
```

- [ ] **Step 4.3: Capture exit code**

Run:
```bash
cd /root/workspace/opc/apps/console && pnpm build > /tmp/console-build.log 2>&1; echo "exit=$?"
tail -30 /tmp/console-build.log
```

- [ ] **Step 4.4: Update PROJECT-STATE.md based on outcome**

If PASS: C2 → ✅ with the summary line.
If FAIL: C2 stays ❓; new G-row in GAP-LOG.md; stop and report to user.

---

## Task 5: Verify apps/shared builds

**Files modified:** none

- [ ] **Step 5.1: Sanity check**

Run:
```bash
ls /root/workspace/opc/apps/shared/node_modules >/dev/null 2>&1 && echo "INSTALLED" || echo "MISSING"
```

- [ ] **Step 5.2: Run typecheck or build**

This is a TS library package, not a Next.js app. Use `tsc --noEmit` (or
the project's `build` script — check `apps/shared/package.json`'s
`scripts.build` first).

Run:
```bash
cat /root/workspace/opc/apps/shared/package.json | grep -A 5 '"scripts"'
```
Then run the build script. If only `tsc` is available, run:
```bash
cd /root/workspace/opc/apps/shared && npx tsc --noEmit 2>&1 | tail -30
```

- [ ] **Step 5.3: Capture exit code**

```bash
cd /root/workspace/opc/apps/shared && npx tsc --noEmit > /tmp/shared-tsc.log 2>&1; echo "exit=$?"
tail -20 /tmp/shared-tsc.log
```

- [ ] **Step 5.4: Update PROJECT-STATE.md**

If exit 0: C3 → ✅.
If exit non-zero: C3 stays ❓; new G-row in GAP-LOG.md.

---

## Task 6: API server smoke test

**Files modified:** none (server only runs in background for the test)
**Files read:** none new

- [ ] **Step 6.1: Check if a server is already running on :8080**

Run:
```bash
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/readyz 2>&1
```
Expected: either `200` (something is already running — use it) or
`000` (nothing — start one).

- [ ] **Step 6.2: If nothing is running, start the API server in the background**

Run:
```bash
cd /root/workspace/opc/apps/api && go run ./cmd/server > /tmp/api-server.log 2>&1 &
SERVER_PID=$!
echo "started pid=$SERVER_PID"
```
Expected: PID printed. The `&` backgrounds it.

- [ ] **Step 6.3: Wait for /readyz to be 200 (poll up to 30s)**

Run:
```bash
for i in $(seq 1 30); do
  code=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/readyz 2>/dev/null)
  if [ "$code" = "200" ]; then
    echo "ready after ${i}s"
    break
  fi
  sleep 1
done
curl -s http://localhost:8080/readyz
echo ""
```
Expected: `ready after Ns` followed by the /readyz body (likely `OK` or JSON).

- [ ] **Step 6.4: Curl a demo endpoint**

The `?demo=true` flag short-circuits to canned data — safest first call
since no API key is configured in this env.

Run:
```bash
curl -s "http://localhost:8080/ai/topics?demo=true&count=2" | head -c 800
echo ""
```
Expected: a JSON response with `"topics": [...]` array (or similar
shape — quote the actual response in the report).

- [ ] **Step 6.5: Kill the server**

Run:
```bash
kill $SERVER_PID 2>/dev/null
# fallback in case the PID was lost:
pkill -f "go-build.*server" 2>/dev/null
pkill -f "cmd/server" 2>/dev/null
sleep 1
curl -s -o /dev/null -w "after-kill code=%{http_code}\n" http://localhost:8080/readyz 2>/dev/null
```
Expected: `after-kill code=000` (server is down).

- [ ] **Step 6.6: Update PROJECT-STATE.md**

If 6.3 + 6.4 both succeed:
- C4 (new handlers work) → ✅ or PARTIAL ✅
- C5 (MCP tools) → still ❓ (MCP uses stdio, not HTTP — out of scope for this smoke test)
- C6 (mock assets reachable) → ✅ with curl output

If either 6.3 or 6.4 fails:
- All three stay ❓
- New G-row in GAP-LOG.md with the failure

---

## Task 7: Final verify-project-state dispatch (the rule-2 gate)

**Files modified:** `~/.claude/audit/verify-reports/verify-YYYYMMDD-HHMMSS.md`
(new report)

- [ ] **Step 7.1: Dispatch the verify agent with full-scope prompt**

Use the Agent tool with `subagent_type: general-purpose` and a prompt that:

1. Reads `~/.claude/PROJECT-STATE.md`
2. Re-runs every ✅ row in section A (sanity)
3. Re-confirms B1 is FIXED (go vet exit 0)
4. Verifies the new claims from this plan:
   - DEFINITION-OF-DONE.md exists with 4 Phase 1 criteria
   - GAP-LOG.md exists with 6 entries
   - C1 / C2 / C3 reflect the actual build results
   - C4 / C6 reflect the smoke test result
5. Writes the report to `~/.claude/audit/verify-reports/verify-<timestamp>.md`
6. Returns overall PASS / FAIL / INCONCLUSIVE

Template prompt:

> You are an independent verifier subagent (third run). The previous
> report is at `~/.claude/audit/verify-reports/verify-20260608-165749.md`
> (OVERALL: PASS for the G4 fix). The author has now completed the
> "opc completion pass" plan at
> `docs/superpowers/plans/2026-06-08-opc-completion-pass.md` covering
> Tasks 1-7. Verify:
> 1. `~/.claude/DEFINITION-OF-DONE.md` exists and has 4 Phase 1 criteria.
> 2. `~/.claude/GAP-LOG.md` exists and has 6 gap entries (G1-G6).
> 3. `~/.claude/PROJECT-STATE.md` section C rows C1/C2/C3/C4/C6 reflect
>    the actual build/smoke test outcomes from this plan.
> 4. Section D gaps have been closed or remain open consistent with the
>    plan's outcomes.
> 5. Re-run go vet + go build + go test on apps/api (should still pass).
> 6. For each newly-claimed ✅, the evidence command is real and
>    reproducible.
>
> Write your report to
> `~/.claude/audit/verify-reports/verify-<timestamp>.md` and return
> overall PASS / FAIL / INCONCLUSIVE with one-sentence summary.

- [ ] **Step 7.2: Read the agent's summary and decide**

- If PASS: state "opc completion pass: VERIFIED" in the response. The
  build is green, the docs are in place, the gaps are tracked.
- If FAIL: list the failures and ask the user for direction. Do not
  declare done.
- If INCONCLUSIVE: list what could not be verified and ask the user
  whether to expand scope.

---

## Self-Review (run after writing the plan, before execution)

**1. Spec coverage** — every gap from PROJECT-STATE.md sections C and D
is addressed by a task:
- C1 (apps/web builds) → Task 3 ✓
- C2 (apps/console builds) → Task 4 ✓
- C3 (apps/shared builds) → Task 5 ✓
- C4 (new handlers work) → Task 6 (smoke test for 1 endpoint) ✓
- C5 (MCP tools) → NOT covered by this plan (MCP uses stdio, not HTTP).
  Documented as still ❓ in Task 7 step.
- C6 (mock assets reachable) → Task 6 (curl one asset-like endpoint) ✓
- D G5 (apps/web build) → Task 3 will close or confirm ✓
- D G1, G2, G3, G6 → NOT in this plan's scope (user authorized B2-B5
  only, not the larger G1/G2 work)

**2. Placeholder scan** — search the plan for: TBD, TODO, "implement
later", "fill in details", "appropriate error handling", "similar to
Task N", "write tests for the above". None found. Each step has its
actual command and expected output.

**3. Type consistency** — file paths used in later tasks match the
paths defined in the file structure section. ✓

**Scope reminder**: this plan is B2-B5 only. G1 (real UI button) and
G2 (platform asset CLI) are explicitly out of scope and are documented
in GAP-LOG.md as still open. The user must authorize a follow-up plan
for those.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-06-08-opc-completion-pass.md`. Two execution options:

**1. Subagent-Driven (recommended for this plan)** — I dispatch a fresh subagent per task, review between tasks. Best for plans with verification gates between tasks.

**2. Inline Execution** — Execute tasks in this session with me doing each step, batch execution with checkpoints. Best when the user wants to watch and intervene.

Given the user's "逐个测试" directive, **Inline Execution with explicit "next?" prompts between tasks** is probably what they want. But I'll ask before starting.
