# Skills Deep-Audit Evidence

> 12 skills 深度采样，按 skill-stocktake 7 维度 + 1 个额外维度 (testability) 评。
> 数据源：实际 `Read` 完整 SKILL.md + `ls` 实际目录 + 实际跑脚本验证。
> 不是凭印象，是凭文件证据。

## 抽样的 12 个 skill

按代表性选：1 个元 skill (skill-creator)、3 个工程化 (tdd/code-review/verify)、1 个质量审计 (frontend-audit)、1 个安全 (security-review)、1 个设计 (frontend-design)、1 个评估 (eval-harness)、1 个自主 (autonomous-loops)、1 个元 (agent-harness-construction)、3 个不存在的 (writing-skills/using-superpowers/code-review/verification-before-completion)、1 个只存在 command 不存在 skill (harness-audit)。

## Per-skill findings

### skill-creator  → Keep (gold standard)

| 维度 | 评分 | 证据 |
|------|------|------|
| Actionability | 9/10 | 6 步流程明确：understand → plan → init → edit → package → iterate |
| Scope fit | 9/10 | description 写得清楚 "when users want to create a new skill" |
| Uniqueness | 10/10 | 元 skill，无替代 |
| Currency | 8/10 | 提及 progressive disclosure 思想正确 |
| Has scripts | ✓ | `init_skill.py` + `package_skill.py` + `quick_validate.py` 实际存在 |
| Describes what NOT to include | ✓ | 显式列出 README/INSTALLATION_GUIDE/QUICK_REFERENCE/CHANGELOG 是反模式 |
| Description carries trigger | ✓ | 把 "when to use" 写进 description，不写进 body |

**唯一短板**：356 行偏长，且没引用 sub-references 文件导致全部塞在 SKILL.md。

### tdd-workflow  → Keep

| 维度 | 评分 | 证据 |
|------|------|------|
| Actionability | 9/10 | 7 步流程：写 journey → 生成 case → RED → 实现 → GREEN → refactor → coverage |
| RED/GREEN 显式 | ✓ | 强制要求 "A test that was only written but not compiled and executed does not count as RED" |
| Git checkpoint | ✓ | 与提交信息规范绑定 (test: add reproducer / fix: / refactor:) |
| Coverage target | ✓ | 80% lines/branches/functions/statements |
| 错误示例 | ✓ | FAIL/PASS 对照的 anti-pattern |
| **No enforcement script** | ✗ | 没有任何脚本能强制 RED 必须真失败 |

**短板**：没有 hook/script 把 "test was actually run" 写进 commit-msg 检查或 pre-commit。

### code-review  → **不存在**

```
$ read /root/.claude/skills/code-review/SKILL.md
File does not exist.
```

但 `/root/.claude/commands/code-review.md` 存在 — 命名空间不一致（skill 命名空间 vs command 命名空间）。用户输 `/code-review` 走 command 不走 skill。

**Verdict: 命名空间冲突需要 audit 解决**。

### verification-before-completion  → **不存在**

```
$ read /root/.claude/skills/verification-before-completion/SKILL.md
File does not exist.
```

`/root/.claude/commands/verify.md` 存在。同上命名空间问题。

### frontend-design  → Keep

145 行，opinionated，有清晰的 anti-pattern 列表 + 视觉决策原则。短到能完整加载。**唯一不足：缺代码示例**（只讲原则）。

### frontend-audit  → Keep

333 行，4 维度 (Visual/UX/Product/Polish) + P0/P1/P2 优先级 + Playwright 抓图代码。**真正能用**。

### security-review  → Keep

496 行，10 大类（secrets / input validation / SQL inj / authn / XSS / CSRF / rate limit / 数据暴露 / 区块链 / 依赖）每类都有 FAIL/PASS 对照 + verification checklist + 部署前 checklist。**完整且实用**。

### eval-harness  → Improve (文档完整但工具是空的)

| 宣称 | 现实 |
|------|------|
| "Formal evaluation framework" | 仅 273 行 markdown |
| 提到 `/eval define feature-name` | `/root/.claude/commands/eval.md` 只是个 shim |
| Storage 写 `.claude/evals/*.md` | 实际不存在 |
| Code-based grader 例子 | 没有真实脚本能跑 |

**真实状态**：eval-harness 是教科书式定义，**没有任何可执行入口**。要让 eval 真的能跑起来，要么补脚本，要么明确说 "本 skill 只提供 prompt 模板"。

### autonomous-loops  → Update (与 continuous-agent-loop 重复)

610 行，6 种 loop pattern（Sequential Pipeline / NanoClaw / Infinite Agentic Loop / Continuous Claude PR Loop / De-Sloppify / Ralphinho RFC-DAG），含 credit 标注原始作者。

**自带 deprecation 标注**（L9-12）：
> Compatibility note (v1.8.0): `autonomous-loops` is retained for one release. The canonical skill name is now `continuous-agent-loop`. New loop guidance should be authored there.

✓ 这条很重要：作者意识到重复，主动标记迁移。但**没标记 "retire"**，所以两边都会留在 skill 列表里 — 选错的风险由用户承担。

### agent-harness-construction  → Improve (过薄)

73 行。提了 action space / observation / error recovery / context budget / anti-pattern，**没代码、没具体例子、没脚本**。

如果用户想要的是 "如何设计 agent tool set"，光读这 73 行不够用，需要搭配 harness-audit 的 7 维度 rubric 才能落地。

### writing-skills  → **不存在**

```
$ read /root/.claude/skills/writing-skills/SKILL.md
File does not exist.
```

只有 superpowers 命名空间下的 `superpowers:writing-skills`（在 system reminder 中提及）。**用户视角下，输入 /writing-skills 期望的是哪个？**

### superpowers:using-superpowers  → 已加载但只以 reminder 形式

没有对应 SKILL.md，content 通过 system-reminder 注入。**不在本地 skills 目录**。

### harness-audit  → **CRITICAL: 引用了不存在的脚本**

`/root/.claude/commands/harness-audit.md` 第 28-30 行：

```bash
node scripts/harness-audit.js <scope> --format <text|json>
```

跑了：
```bash
$ find /root/.claude -name "harness-audit.js"
(no result)
```

**确定文件不存在**。命令文档还自称 "Do not invent additional dimensions or ad-hoc points. Rubric version: 2026-03-30." — 但 rubric 引擎从未实际写出来。

## 跨 skill 发现

### A. `settings.json` 硬编码 auth token — **CRITICAL**

```json
"ANTHROPIC_AUTH_TOKEN": "sk-a1b3ccfed987484e81bf36a021e0b10c"
```

**security-review 自己** 在第 27-29 行写着：
```typescript
const apiKey = "sk-proj-xxxxx"  // Hardcoded secret  // WRONG
```

而 ECC 自己在 main settings 里就这么干。讽刺。

### B. use_7d / use_30d 全是 0

`skill-stocktake/scripts/scan.sh` 输出的所有 skill 都有 `use_7d: 0, use_30d: 0`。说明 **use tracking 没工作**，或者记录字段没被 harness 写。214 个 skill 没有任何一个被使用统计的反馈 — **空数据 = 没法做 evidence-based retire 决策**。

### C. hooks.json 47KB，inline node -e 250 字符

`hooks.json` 的 PreToolUse matcher=Bash 里有一行：

```bash
node -e "const p=require('path');const r=(()=>{var e=process.env.CLAUDE_PLUGIN_ROOT;...if(f.existsSync(p.join(c,q)))return c}}...return d})();..."
```

约 250 字符 inline JS，写在 command 字符串里 — **不可读、不可测、不可改**。这是 LLM 时代之前 copy-paste 时代的写法。

### D. 多模型 settings 变体无明确治理

```
/root/.claude/settings.json            (main, 用 deepseek-v4-pro)
/root/.claude/settings.json.ark
/.claude/settings.json.deepseek
/.claude/settings.json.jd
/.claude/settings.json.minimax
/.claude/settings.json.xiaomi
```

哪个是 source of truth？切到 minimax 模型要 cp 哪个文件？没有 README 说明。

### E. Skill 命名空间 vs Command 命名空间错位

- `/skills/<name>/SKILL.md` : 用于自动 trigger
- `/commands/<name>.md` : 用于 slash command

但：
- `code-review` skill 不存在，command 存在
- `verification-before-completion` skill 不存在，command 存在
- `harness-audit` 只有 command，没有 skill
- `writing-skills` 只有 superpowers 注入，没有本地

**用户/agent 视角下，行为不可预测**：输 `/code-review` 走 command，但 description 文本里如果有 `code-review` 关键词，可能会 trigger 别的 skill（如果存在的话）。

## Verdict 汇总

| Skill | Verdict | 主要理由 |
|-------|---------|---------|
| skill-creator | Keep | 黄金标准，scripts 齐全 |
| tdd-workflow | Keep | RED/GREEN 强制正确，但缺 enforcement script |
| frontend-design | Keep | 短、清晰、opinionated |
| frontend-audit | Keep | 4 维度方法论完整 |
| security-review | Keep | 10 大类 + checklist 完整 |
| eval-harness | Improve | 文档完整但 0 脚本，承诺未兑现 |
| autonomous-loops | Update | 已被 continuous-agent-loop 取代，需合并 |
| agent-harness-construction | Improve | 73 行过薄，缺例子 |
| code-review | Discover | skill 不存在，command 存在 |
| verification-before-completion | Discover | skill 不存在，command 存在 |
| writing-skills | Discover | 只有 superpowers 注入，无本地 |
| harness-audit | **CRITICAL: Update** | 引用了不存在的 JS 引擎 |
