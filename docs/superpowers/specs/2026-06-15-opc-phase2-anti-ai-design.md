# OPC Phase 2 — 反 AI 检测 / 拟人化设计（Sub-Spec E）

> **文档版本**：v1.0（初稿，待审）
> **日期**：2026-06-15
> **作者**：OPC 团队（与 Claude 协作）
> **状态**：设计阶段 → 待用户审阅
> **路径**：`docs/superpowers/specs/2026-06-15-opc-phase2-anti-ai-design.md`
> **父 spec**：`docs/superpowers/specs/2026-06-03-opc-phase1-ip-design.md` §3.2（Phase 2 6-12 月）、§3.3（平台能力）
> **依赖 spec**：
> - `docs/superpowers/specs/2026-06-11-opc-phase2-ip-template-design.md` v1.0（Sub-Spec A done）
> - `docs/superpowers/specs/2026-06-12-opc-phase2-capabilities-design.md` v1.0（Sub-Spec B done）
> - `docs/superpowers/specs/2026-06-15-opc-phase2-external-ui-design.md` v1.0（Sub-Spec C done）
> - `docs/superpowers/specs/2026-06-15-opc-phase2-agent-agnostic-design.md` v1.0（Sub-Spec D done）
> **兄弟 spec**：M1 5 sub-spec 之 **E**（最后 1 个）

---

## 一、文档目的

Phase 1 spec §3.2 写：「70% 创作者应用 AI、近半数遭限流；用户对 AI 内容负反馈率是真人 3.2 倍」。Sub-Spec A M1 已在 IP profile consistency check 加 "无 AI 感" 类锚点（prompt 侧），但**没** 检测 AI 生成的**实际内容**。Sub-Spec E 补这个 gap：加 1 个**启发式 anti-AI 检测层**（不调外部 API），扫描 LLM 生成的 prompt 看是否含 "作为一名 AI 语言模型" 等 cliché 词，返 1 个 "humanized score" 给创作者参考。

E M1 走最小范围：1 个新 `internal/antiai/` package + 现有 `GenerateTopics` handler 加 2 行 + 2 个 E2E smoke case。**不**调 GPTZero / 其他外部 API（成本 + 准确率 trade-off 留 M2 评估）、**不**加 middleware（避免跟 Sub-Spec D 的 `MaybeAgentKey` 复杂化，E 直接在 handler 调 1 个 check 函数）、**不**改 capability 业务（B M1 已有 library 不动）。

---

## 二、Executive Summary

| 项 | 内容 |
|----|------|
| **目标** | 给 LLM 生成的 prompt 加 1 个启发式 anti-AI 检测层，返 0-1 humanized score 走 `X-Humanized-Score` HTTP header。E M1 是 soft signal，不 硬 拦 任何 request。 |
| **范围（M1）** | 1 个新 `internal/antiai/` package (Check function) + 现有 handler 加 2 行 + 2 个 E2E smoke case。 |
| **应用范围** | 仅 topic capability（`POST /api/ai/topics`），跟 Sub-Spec A/B/C/D M1 一致（最小 1 capability）。M2 扩 4 capability 全部。 |
| **检测方式** | 启发式 — 扫描 prompt 文本含 中文/英文 AI cliché 词（"作为一名 AI 语言模型", "as an AI language model" 等 ~15 词），按 词权重 累减 1.0 起始 score, 0.0 下限。**不** 调任何 外部 API. |
| **不做** | GPTZero / 任何 外部 AI detection API、middleware、per-capability 重构、ML model 自训。 |
| **输出** | HTTP response header `X-Humanized-Score: 0.85` (2 decimal). Response body 不动（跟 A/B/C/D M1 风格 一致 — 走 HTTP header 跟 response body 双轨). |
| **Operator UI** | 不 加新 UI (operator 在 console /observability 看 call_log; E M1 返 score 给 client, console 不 显式). |
| **M1 timeline** | 1 周 (3 task: package + 8 test + handler integration + 2 smoke case) |
| **依赖** | A (IP profile framework, done) + B (topic library, done) + D (X-API-Key 仍 走 同 chain) |
| **风险等级** | 🟡 中（启发式 检测 准确率 限; M2 可 加 GPTZero 加 准确率） |

---

## 三、背景与现状

### 3.1 Phase 1 + Phase 2 A/B/C/D 已有

| 现状 | 状态 | 跟 E 关系 |
|------|------|----------|
| Phase 1 老 平台 (G6 closed 2026-06-09) | ✅ | call_log middleware 记 cost, E 加 1 列 score (M2 优先) |
| Sub-Spec A IP profile consistency check (done) | ✅ 5-7 anchor per type | A 检查 **prompt 侧**; E 检查 **prompt 侧 + (M2) output 侧** — 不 重复, 互补 |
| Sub-Spec B topic library + handler (done) | ✅ | E 在 handler 调 1 行 antiai.Check, 不 改 B 业务 |
| Sub-Spec C 创作者 external web UI (done) | ✅ | C 调 topic 时 拿 X-Humanized-Score, 可选 显示 "拟人化分数" badge (M2) |
| Sub-Spec D agent X-API-Key (done) | ✅ | D 调 topic 也 走 E (X-Humanized-Score 头 跟 X-API-Key 头 互 不 影响) |

**关键观察**：E M1 是 1 个 极小增量 — 1 个新包 + 1 行 handler 改. 跟 D 互 不 冲突 (D 加 auth 表面, E 加 output 检查). 跟 A 互补 (A 在 prompt 构造 时 含 反 AI 锚点, E 在 LLM 调 用 时 验证 prompt 真 不含 AI 词 — 跟 A 反向 验证).

### 3.2 A M1 已有的反 AI 锚点 (对比)

| IP Type | A 已有 锚点 | Weight | E 启发式 check 同 词? |
|---------|------------|--------|---------------------|
| anthropomorphic | "不要 AI 生成感" | 0.05 | ❌ (E 检测 实际 AI 词, A 是 prompt 内嵌) |
| digital_human | "表情自然" | 0.15 | ❌ (类似) |
| digital_human | "无恐怖谷" | 0.15 | ❌ |
| info | "无 AI 播报感" | 0.20 | ❌ |

A 的锚点 是 "避免 AI 感" 的 prompt 指令. E 的检查 是 "AI 词 真不真在 prompt" 的反验证. 两者**不重复**, **互补** (A 防御 prompt 构造, E 验证 prompt 实际).

### 3.3 不在 M1 范围（推到 M2/M3）

- ❌ GPTZero / Originality.ai / 任何 外部 AI detection API
- ❌ Per-response score (M1 check prompt 侧, M2 改 check output 侧 — 需重写 library return value)
- ❌ Console UI 显示 score (M2 加 console `/observability` 列 X-Humanized-Score)
- ❌ Per-capability threshold gate (M1 soft signal, M2 可加 "score < 0.5 → 拒" 硬 拦)
- ❌ 启发式 check 学 习 (no ML model, no training pipeline)
- ❌ 任何 新 capability 端点

---

## 四、设计目标（M1 验收）

| # | 验收 | 验证命令 |
|---|------|---------|
| 1 | `internal/antiai/antiai.go` Check function 存在, 返 `Result{Score, Fails}` struct | `go doc -all ./internal/antiai` |
| 2 | 8-10 个 unit test 覆盖 perfect / partial / zero / dedup / 案例 不敏感 / 中英 混 / 多词累计 | `go test -tags fts5 ./internal/antiai/...` PASS |
| 3 | `coverage: 80%` on `internal/antiai/` | `go test -tags fts5 -cover ./internal/antiai/...` |
| 4 | `GenerateTopics` handler 加 2 行: `aiScore := antiai.Check(prompt).Score` + `c.Header("X-Humanized-Score", ...)` | smoke 验 头 存在 |
| 5 | 老 `scripts/api-smoke.sh` 0 回归 (12 case topic HTTP 仍 PASS, 不 受 header 加 1 行 影响) | `go test ./...` 全绿 |
| 6 | 2 个新 E2E smoke case: topic 调 头含 `X-Humanized-Score`; 含 AI 词 模拟 LLM 返 score 低 | `./scripts/api-smoke.sh` 22/22 PASS |
| 7 | verify-project-state agent OVERALL: PASS | `~/.claude/audit/verify-reports/verify-*.md` PASS |
| 8 | PROJECT-STATE.md + GAP-LOG.md 更新 | G17 CLOSED 记录在案 |

**Out of scope（M1 不做）**:
- ❌ GPTZero / 外部 API
- ❌ middleware
- ❌ per-capability threshold gate
- ❌ console UI 显示 score
- ❌ output 侧 check (M1 仅 check prompt; M2 改 res.Text)

---

## 五、架构

### 5.1 包结构（M1 终态）

**新建**:
```
apps/api/internal/antiai/
├── antiai.go                              (Check function + 词表 + Result struct)
└── antiai_test.go                         (8-10 个 fixture)
```

**修改**:
```
apps/api/internal/handlers/ai.go            (GenerateTopics 加 2 行)
scripts/api-smoke.sh                        (append 2 case)
```

**复用不动**:
- `internal/capabilities/topic/` (B M1 done, library 不改)
- `internal/assetgen/` (A M1 done, 词表 不动)
- `internal/middleware/auth.go` (D M1 done, RequireEitherAuth 不动)
- `internal/handlers/creator.go` (C M1 done, admin endpoint 不动)
- `internal/handlers/agent.go` (D M1 done, 4 admin endpoint 不动)
- `cmd/server/main.go` (D M1 done, 路由 不动)

### 5.2 关键设计决策

| 决策 | 选项 | 选定 | 理由 |
|------|------|------|------|
| M1 范围 | 启发式 / GPTZero / 拟人化 LLM call | **启发式** | 用户最小化; 0 外部依赖; 1 周 走 |
| 应用 capability | 仅 topic / 4 capability / 0 (扩 A) | **仅 topic** | 跟 A/B/C/D M1 一致; 1 capability 暴露面 |
| Check 位置 | middleware / handler / library 内 | **handler 顶部 1 行** | 跟 D 风格 一致 — 业务逻辑 改 handler 不加 middleware |
| Check 时刻 | prompt 构造 后 / LLM 调 用 前 / LLM 返 response 后 | **prompt 构造 后, LLM 调 用 前** | 跟 A consistency check 互补 (A 在 prompt 内嵌 反 AI 锚点, E 验证 prompt 真 不含 AI 词) |
| Output 表面 | HTTP header / response body 字段 / 新 endpoint | **HTTP header `X-Humanized-Score`** | 跟 老 wire contract 不 破 (response body 不 变); 头 信息 给 client 透明 显示 |
| 词 表 大小 | 5 词 / 15 词 / 50 词 | **15 词 (中英 混)** | M1 起步; 1 词 weight 0.05-0.40, 5 词 命中 即 score 0.0; M2 扩 50 词 |
| Score 格式 | 0-1 / 0-100 / 5 档 评级 | **0-1 (2 decimal, 跟 A consistency check 风格 一致)** | 跟 `ConsistencyResult.Score` 同 格式 |
| Score 阈 值 | 软 信号 / 硬 拦 (score < 0.5 拒) | **软 信号 (M1)** | 启发式 准确率 限, 硬 拦 误伤 高; M2 加 GPTZero 提 准确率 后 再 考 虑 硬 拦 |
| 词 权 重 调 | 跑 100 实 测 校准 / 拍脑袋 0.05-0.40 | **拍脑袋 0.05-0.40 (跟 现有 Sub-Spec A anchor weight 风格 一致)** | M1 启发式 不 要 求 准; M2 拿 实 测 调 |

### 5.3 Heuristic Check Algorithm

**Score 计算**:
```
score = 1.0
for each phrase in aiPhrases:
    if phrase not in dedup_seen:
        if phrase in lower(text):
            score -= weight
            fails.append(phrase)
            dedup_seen.add(phrase)
score = max(0, score)  // clamp
return round2(score)
```

**关键不变量**:
- Score ∈ [0, 1]
- 1 词 命中 仅 扣 1 次 (dedup)
- Case-insensitive (strings.ToLower)
- 词 表 真不真 在 text 里 (strings.Contains, 不 用 regex — 简单 + 够 用)

**词 表 15 词** (拍脑袋, M2 校准):

| 词 | Lang | Weight | 类 |
|----|------|--------|-----|
| "作为一名 ai 语言模型" | zh | 0.40 | hard |
| "作为一个 ai" | zh | 0.30 | hard |
| "作为一个大语言模型" | zh | 0.30 | hard |
| "本文由 ai 生成" | zh | 0.40 | hard |
| "this response was generated by ai" | en | 0.40 | hard |
| "as an ai language model" | en | 0.30 | hard |
| "i am an ai" | en | 0.30 | hard |
| "i'm an ai" | en | 0.30 | hard |
| "as a large language model" | en | 0.30 | hard |
| "作为一个 ai 助手" | zh | 0.20 | medium |
| "以上内容仅供参考" | zh | 0.15 | medium |
| "请注意" | zh | 0.05 | light |
| "希望对您有帮助" | zh | 0.10 | medium |
| "根据您提供" | zh | 0.05 | light |
| "综上所述" | zh | 0.05 | light |

(Total weight if all match: 3.65 — saturates well above 1.0; dedup keeps it bounded.)

### 5.4 Handler 集成 (改 2 行)

**`internal/handlers/ai.go:GenerateTopics`** (B M1 写的):
```go
// 现状 (B M1):
res, err := topic.Generate(ctx, h.text, topic.Input{...})
// ... error handling ...
StampClaudeCost(c, config.SkillMiniMaxM27, res.Cost.InputTokens, res.Cost.OutputTokens)
c.JSON(http.StatusOK, generateTopicsResponse{Topics: res.Topics})

// D M1 后 (加 X-API-Key + actor_type):
// 同样 handler 顶部 加 1 行 c.Get("agent_id") == nil && c.GetUint("user_id") == 0 → 401

// E M1 后 (加 启发式 check):
// 不动 业务 逻辑, 仅 在 response 前 加 2 行:

// M1 Sub-Spec E: anti-AI heuristic check
// 检查 LLM 调 用 的 prompt (跟 A consistency check 互补 — A 在 prompt 构造 时
// 含反 AI 锚点, E 验证 prompt 真不真含 AI 词). M2 改 check res.Text (response 侧).
aiScore := antiai.Check(prompt).Score
c.Header("X-Humanized-Score", fmt.Sprintf("%.2f", aiScore))
c.JSON(http.StatusOK, generateTopicsResponse{Topics: res.Topics})
```

(2 行 加 1 个 import. 不动 现有 业务逻辑.)

### 5.5 Auth 兼容 (跟 D 集成)

**E M1 不影响 D M1 的 X-API-Key 表面**:
- Human 调 (session cookie) → 走 RequireEitherAuth 通过 → E 加 X-Humanized-Score 头
- Agent 调 (X-API-Key) → 走 RequireEitherAuth 通过 → E 加 X-Humanized-Score 头
- 任 一 失败 → RequireEitherAuth 401, E 不 调 (handler 不 跑 到)
- 双 头 共存 → 跟 D 一样, 头 都 设, 互不 干扰

**E 不 加新 middleware** — 跟 D 一样, 业务逻辑 改 handler 顶部 1 行, 不 扩 chain. 避免 middleware 越 加 越 多 (Sub-Spec A 已有 3 + B 0 + C 2 + D 1 = 6 middleware; M1 6 → 7 加 1 个 太重).

---

## 六、组件

### 6.1 `internal/antiai/antiai.go` (new)

```go
// Package antiai implements heuristic anti-AI detection for
// generated text. M1 E: scan text for common AI-generated
// phrases (in Chinese + English) and return a "humanized score"
// in [0, 1]. Lower score = more likely AI-generated.
//
// This is a HEURISTIC check — not a sophisticated ML model. It
// catches obvious AI tells (clichés, disclaimers, "as an AI"
// phrasing) but does NOT detect all AI content. Operators should
// use this as a soft signal, not a hard block. M2 may add
// external GPTZero-style API integration for higher accuracy.
package antiai

import (
    "math"
    "strings"
)

// Result is the output of Check. Score is in [0, 1]; Fails
// lists the specific phrases that triggered the score.
type Result struct {
    Score float64  // 1.0 = no AI tells, 0.0 = saturated with AI tells
    Fails []string // specific phrases that lowered the score
}

// aiPhrases is the M1 list of common AI-generated phrases
// (Chinese + English). Each phrase has an associated weight.
// Phrases are kept lowercase for case-insensitive matching.
var aiPhrases = []struct {
    Phrase string
    Weight float64
}{
    // Hard tells (high weight, 0.30-0.40)
    {"作为一名 ai 语言模型", 0.40},
    {"作为一个 ai", 0.30},
    {"作为一个大语言模型", 0.30},
    {"本文由 ai 生成", 0.40},
    {"this response was generated by ai", 0.40},
    {"as an ai language model", 0.30},
    {"i am an ai", 0.30},
    {"i'm an ai", 0.30},
    {"as a large language model", 0.30},
    // Medium tells (0.10-0.20)
    {"作为一个 ai 助手", 0.20},
    {"以上内容仅供参考", 0.15},
    {"希望对您有帮助", 0.10},
    // Light tells (0.05)
    {"请注意", 0.05},
    {"根据您提供", 0.05},
    {"综上所述", 0.05},
}

// Check scans text for AI tells and returns a humanized score.
// The score starts at 1.0 and decreases for each matched phrase.
// Final score is clamped to [0, 1]. Same phrase matched
// multiple times only counts once (dedup).
func Check(text string) Result {
    lower := strings.ToLower(text)
    var score float64 = 1.0
    var fails []string
    seen := make(map[string]bool)
    for _, p := range aiPhrases {
        if seen[p.Phrase] {
            continue
        }
        if strings.Contains(lower, p.Phrase) {
            score -= p.Weight
            fails = append(fails, p.Phrase)
            seen[p.Phrase] = true
        }
    }
    if score < 0 {
        score = 0
    }
    return Result{Score: round2(score), Fails: fails}
}

// round2 rounds to 2 decimal places, matching the
// ConsistencyResult.Score style from Sub-Spec A.
func round2(f float64) float64 {
    return math.Round(f*100) / 100
}
```

### 6.2 Handler 集成 (改 2 行)

`internal/handlers/ai.go:GenerateTopics` 加:

```go
// M1 Sub-Spec E: anti-AI heuristic check
aiScore := antiai.Check(prompt).Score
c.Header("X-Humanized-Score", fmt.Sprintf("%.2f", aiScore))
```

(2 行 + 1 个 import `"github.com/opc/api/internal/antiai"`. 不动 现有 业务逻辑.)

### 6.3 HTTP Wire 兼容 (response body 不 变)

**`POST /api/ai/topics`** Response:
```http
HTTP/1.1 200 OK
Content-Type: application/json
X-Humanized-Score: 0.85
Set-Cookie: opc_session=...; (human) or X-API-Key header (agent)

{"topics":[{"title":"...","angle":"...","expected_performance":"...","hook":"..."}]}
```

(Response body 跟 现有 完全 一致, 仅 加 1 个 X-Humanized-Score 头. 老 client 不 读 头 不 受 影响.)

### 6.4 Test 覆盖矩阵

| 层 | 覆盖 | 数量 |
|----|------|------|
| Unit: antiai.Check | 0 AI 词 返 1.0 / 1 词 降 分 / 多 词 累计 / dedup / 案例 不敏感 / 中英 混 / 词 表 全 命中 返 0.0 / 词 短 sub-string 假阳性 / 词 在 句 头 跟 句 尾 同样 命中 | 8-10 |
| Integration: handler | X-Humanized-Score 头 存在 / score 是 2 decimal / 跟 response body 不 冲突 / 老 handler 行为 不 变 | 3-4 (跟 现有 TestGenerateTopics* 共 跑) |
| E2E smoke | 2 case: topic 调 头含 X-Humanized-Score; 含 AI 词 模拟 LLM 返 score 低 | 2 |

**Coverage 目标**:
- NEW `internal/antiai/` ≥ 80% line coverage
- 父包 `internal/handlers` ≥ 80% (现有 baseline 满足)

---

## 七、Schema 设计

### 7.1 Result Schema

```go
type Result struct {
    Score float64  // [0, 1], 2 decimal, 跟 ConsistencyResult.Score 同 格式
    Fails []string // 命中 的 AI 词 列表 (dedup, case-preserved)
}
```

### 7.2 HTTP Wire Schema

**`POST /api/ai/topics`** (E M1 后):
- Response header (NEW): `X-Humanized-Score: <float>` (例 "0.85", "0.70", "1.00")
- Response body: 跟 现有 一致, **不** 加 `ai_score` 字段
- 老 client 读 body 行为 不 变; 想 看 score 的 client 读 header

### 7.3 词 表 Schema (代码 静态, 不入 DB)

词 表 在 `internal/antiai/antiai.go` 的 `var aiPhrases` (compiled into binary). **不** 入 DB (M1 拍脑袋 0.05-0.40, M2 校准 后 仍 是 静态; 词 表 频 繁 改 需 code 改 + release, DB 不 灵活).

---

## 八、Consistency Check 设计

**E 不 加新的一致性检查**。Sub-Spec A 已有 IP profile consistency check (7-9 anchor per type, 跟 1-2 软 相关 拟人化 锚点). E 是 **post-prompt-construction 的反 AI 词 检测**, 跟 A 互补, 不 重复.

**E 跟 A 的 关系**:
- A: prompt 构造 时 嵌入 反 AI 锚点 (例 "不要 AI 生成感")
- E: prompt 构造 后 检查 真 不含 AI 词 (例 验证 "不要 AI 生成感" 真 在 prompt 里, 而 "作为一个 AI" 不 在)
- A 跟 E 一起: 防御 prompt 构造 + 验证 prompt 实际, **不** 重复

---

## 九、Prompt 构建

N/A — E 不 加 capability, 复用 B 的 `topic.GenerateTopicsPrompt` 跟 A 的 IP profile. E 仅 **检查** prompt 不 改 prompt.

---

## 十、数据流

### 10.1 启动期

无 启动期 依赖. `internal/antiai/` 是 纯 library (无 init, 无 registry). 现有 服务 不 重启.

### 10.2 运行时: Topic 调 时 E 走 E

```
$ curl -X POST /api/ai/topics \
    -H "X-API-Key: opc_agent_..." \
    -d '{"seed":"个人成长","platform":"抖音","count":5}'
  ↓
1. POST /api/ai/topics
   ↓ RequireEitherAuth (D M1) 通过
2. AIHandler.GenerateTopics (B M1 + E M1)
   ↓ bind + validate
   ↓ 1 行 check: c.GetUint("user_id") > 0 || c.Get("agent_id") != nil → 通过
   ↓ 构造 prompt = agents.GenerateTopicsPrompt(...)
   ↓ **NEW (E M1)**: aiScore := antiai.Check(prompt).Score
   ↓ topic.Generate(ctx, h.text, input) → 5 topics
   ↓ StampClaudeCost (D M1 加 actor_type)
3. **NEW**: c.Header("X-Humanized-Score", "1.00")
4. c.JSON(200, {topics: [5 items]})
5. Response header: X-Humanized-Score: 1.00
6. Response body: 不 变 (跟 B M1 + D M1 一致)
```

### 10.3 运行时: 模拟 LLM 返 含 AI 词 (smoke test 路径)

```
$ curl -X POST /api/ai/topics ... (跟 上 一样, 但 Mock LLM 返 prompt 含 "作为一个 AI")
  ↓
1. ...
2. aiScore := antiai.Check(prompt).Score
   prompt 含 "作为一个 AI" (weight 0.30) → score = 1.0 - 0.30 = 0.70
3. c.Header("X-Humanized-Score", "0.70")
4. Response: 头 0.70, body 跟 现有 一致
```

(老 client 不 读 头, 0 影响. 想 显示 拟人化 score badge 的 client (apps/external M2) 读 头 渲染.)

---

## 十一、错误处理

| 错误 | 检测 | 处理 |
|------|------|------|
| LLM 返 多重 AI 词 | antiai.Check 多重 dedup | score 0.0 (saturated) + Fails list 完整 |
| 词 表 缺 某些 AI tells | (M1 heuristic 不 全) | 接受 (M1 是 soft signal; M2 加 GPTZero 加 准确率) |
| response header 注入 失败 | (Go stdlib 不会 失败) | (no-op) |
| 词 案例 | strings.ToLower 全部 | 案例 不敏感 |
| 同一 phrase 多次 出现 | dedup map | 仅 扣 1 次 (避免 单 phrase 重复 扣 多次) |
| E 不 跟 D 互动 | RequireEitherAuth 401 优先 | 401 → E 不 调 (handler 不 跑) |
| E 不 跟 call_log 互动 | (M1 不 加 1 列; M2 加) | (M1 接受; M2 加 actor_humanized_score 列) |

---

## 十二、测试策略

### 12.1 Unit Tests (`internal/antiai/antiai_test.go`, 8-10 case)

```go
func TestAntiAICheckEmpty(t *testing.T) {
    // "" → 1.0
}

func TestAntiAICheckZeroAI(t *testing.T) {
    // "个人成长博主" → 1.0
}

func TestAntiAICheckOneHardTell(t *testing.T) {
    // "作为一个 AI 语言模型, ..." → 1.0 - 0.40 = 0.60
}

func TestAntiAICheckMultipleTells(t *testing.T) {
    // "作为一名 AI 助手, 以上内容仅供参考, 希望对您有帮助" → 0.55 (1.0 - 0.40 - 0.15 - 0.10 + dedup)
}

func TestAntiAICheckDedupSamePhrase(t *testing.T) {
    // "AI AI AI AI AI" → 0.70 (dedup, 1.0 - 0.30 = 0.70)
}

func TestAntiAICheckCaseInsensitive(t *testing.T) {
    // "I AM AN AI" → 0.70 (跟 "i am an ai" 一样)
}

func TestAntiAICheckChineseEnglishMix(t *testing.T) {
    // "作为 AI, I am an AI" → 1.0 - 0.30 - 0.30 = 0.40
}

func TestAntiAICheckSaturateToZero(t *testing.T) {
    // All 15 词 命中 → score clamped to 0
}

func TestAntiAICheckFailsListContent(t *testing.T) {
    // 命中 2 词 → Fails list 含 2 项 (case-preserved)
}

func TestAntiAICheckRound2Formatting(t *testing.T) {
    // 0.5555 → 0.56
}
```

### 12.2 E2E Smoke (`scripts/api-smoke.sh` 加 2 case)

```bash
# === Sub-Spec E M1: anti-AI heuristic check ===
section "anti-AI heuristic"

# 1. Topic 调 返 X-Humanized-Score 头 (≥ 0.0, ≤ 1.0)
out=$(curl -s -X POST http://localhost:8081/api/ai/topics \
    -H "Content-Type: application/json" \
    -H "Cookie: opc_session=$SESSION_TOKEN" \
    -d '{"seed":"个人成长","platform":"抖音","count":5}')
score=$(curl -s -X POST .../api/ai/topics ... -D - | grep -i 'X-Humanized-Score' | awk '{print $2}' | tr -d '\r')
if awk "BEGIN{exit !($score >= 0.0 && $score <= 1.0)}"; then
    pass "anti-AI: X-Humanized-Score header present, in [0,1]"
else
    fail "anti-AI: X-Humanized-Score not present or out of range (got '$score')"
fi

# 2. Response body 仍 含 5 topics (老 smoke 不 破)
topic_count=$(echo "$out" | jq '.topics | length' 2>/dev/null)
if [ "$topic_count" = "5" ]; then
    pass "anti-AI: response body unchanged (5 topics)"
else
    fail "anti-AI: response body changed, got $topic_count topics"
fi
```

(M1 不 模拟 含 AI 词 的 LLM response — 需 mock LLM 调 用, 跟 B M1 + D M1 一致, 留 smoke 验 头 存在 跟 body 不 变. Mock 路径 在 现有 ai_smoke.sh 走 "demo mode" 路径, M2 加 词 模拟.)

---

## 十三、实施计划（M1 timeline）

| 周 | 任务 | 交付物 |
|----|------|--------|
| W1 | Task 1: `internal/antiai/antiai.go` + 8-10 unit test | 8-10 test PASS, coverage ≥ 80% |
| W1 | Task 2: `handlers/ai.go:GenerateTopics` 加 2 行 + 老 11 个 handler test 0 回归 | X-Humanized-Score 头 包含; 老 4 个 TestGenerateTopics* test 仍 PASS |
| W1 | Task 3: scripts/api-smoke.sh 加 2 case + verify + state files | 20+2=22 case PASS, G17 CLOSED |

**W1 末 verify agent 必跑**, OVERALL PASS 才算 M1 完.

---

## 十四、风险与不做的事

### 14.1 风险

| 风险 | 缓解 |
|------|------|
| 启发式 检测 准确率 限 (M1 拍 脑袋 词 表) | M1 是 soft signal (仅 头 显示, 不 硬 拦); M2 加 GPTZero 提 准确率 |
| 词 表 真 含 漏 AI tells (新 词 不 命中) | 接受 (M1 起步); M2 跑 100 实 测 校准 + 加 GPTZero 兜底 |
| false negative (真 AI 生成 但 启发式 未 抓到) | 启发式 永远 有限; M2 加 外部 API |
| false positive (真人 文本 误 中 AI 词) | 词 选 偏向 "硬 tell" (例 "作为一个 AI"), 减少 误伤 |
| 词 表 维护 成本 (新 AI 词 频出) | M1 词 表 静态; M2 可 加 DB-backed 词 表 + 频 繁 调 优 |
| 老 client 不 读 X-Humanized-Score 头, 0 影响 | 头 是 additive, 不 破 老 client |
| call_log middleware 不 记 score (operator 不 能 后 查) | 接受 (M2 加 actor_humanized_score 列) |
| RequireEitherAuth 401 优先, E 不 调 | 接受 (M2 加 调 1 次 antiai.Check 即便 401 也 可 — 但 M1 不 加 复杂性) |
| 同 phrase dedup 但 含 大小写 不 一致 (例 "I am an ai" vs "I AM AN AI") | strings.ToLower 全 转, dedup 跟 ToLower 后 一致, 0 问题 |
| LLM 返 prompt 跟 aiPhrases 真 重 叠 (例 真 谈 "作为一个 AI 助手" 是 真人 写 的 帖) | 接受 (M1 软 信号, M2 加 上下文 awareness 减 false positive) |

### 14.2 不做的事（M1 范围硬约束）

- ❌ GPTZero / Originality.ai / 任何 外部 AI detection API
- ❌ middleware (E 改 handler 1 行, 不 扩 chain)
- ❌ per-capability threshold gate (M1 软 信号, M2 加 硬 拦)
- ❌ console UI 显示 score (M2)
- ❌ output 侧 check (M1 仅 check prompt, M2 改 res.Text)
- ❌ per-call score 持久化 (M2 加 call_log 列)
- ❌ DB-backed 词 表 (M1 静态 Go var, M2 加 DB)
- ❌ 任何 新 capability 端点
- ❌ 改 capability 业务 (B M1 + Phase 1 老 code 全部 不动)
- ❌ 改 老 handler endpoint 行为 (除 加 1 行 E check + 1 个 header)

---

## 十五、与 Sub-Spec A / B / C / D 既有代码的关系

| 已有 | E M1 处置 | 理由 |
|------|--------|------|
| `internal/assetgen/` (A M1) | 不动 | E 不 改 IP profile 词 表, 不 改 consistency check, 互补 不 重 复 |
| `internal/capabilities/topic/` (B M1) | 不动 | E 不 改 prompt-building 业务, 不 改 library API, 仅 handler 顶部 加 1 行 check |
| `internal/handlers/ai.go:GenerateTopics` (B M1) | modify 加 2 行 | E 仅 加 antiai.Check + c.Header, 不 改 业务 逻辑 |
| `internal/handlers/ai.go` RequireEitherAuth check (D M1) | 不动 | E 跟 D 互不 冲突, E 在 D 401 后 不 调 |
| `internal/middleware/call_log.go` actor_type (D M1) | 不动 | M1 不 加 1 列 humanized_score, M2 加 |
| `internal/handlers/{creator,agent}.go` (C/D M1) | 不动 | E 不 改 admin endpoint |
| `cmd/server/main.go` 路由 (D M1) | 不动 | E 不 加 middleware, 不 改 路由 chain |
| `apps/console/{creators,api-keys}/` (C/D M1) | 不动 | E 不 加 console UI |
| `apps/external/` (C M1) | 不动 | E 不 加 UI page; M2 apps/external 可 显示 X-Humanized-Score badge |
| `apps/agents/minimax.go` (B M1) | 不动 | E 不 改 LLM client |
| `scripts/api-smoke.sh` (C/D M1) | modify 加 2 case | E 加 2 个 smoke case, 老 0 破 |

---

## 十六、与 Phase 2 其他 sub-spec 的关系

| Sub-spec | 与本 sub-spec 的关系 |
|----------|---------------------|
| A (IP profile) | M1 done ✅。E 检查 prompt 侧, A 检查 prompt 构造 时 含反 AI 锚点 — 互补 |
| B (4 能力工具化) | M1 done ✅。E 在 B topic handler 加 1 行, 不 改 B 业务 |
| C (对外 web UI) | M1 done ✅。E 跟 C 互不 冲突, M2 C 可 显示 score badge |
| D (agent-agnostic) | M1 done ✅。E 跟 D 互不 冲突, agent 调 topic 也 收 X-Humanized-Score 头 |
| E (反 AI 检测) | 本 sub-spec。M1 = 仅 启发式 check on topic |

**关键路径** (按 Phase 1 spec §16):
```
A ✅ → B ✅ → C ✅ ─┐
       ↓          ├─ D ✅ ─┐
       E (本) ←─┘          │
                           │
D + E 互 弱 关联
```

E M1 完成后, Phase 2 全部 5 个 sub-spec 完. **5 sub-spec 闭环**.

---

## 十七、变更记录

| 版本 | 日期 | 变更 | 作者 |
|------|------|------|------|
| v1.0 | 2026-06-15 | 初稿（基于 brainstorming 2 问澄清：仅加反 AI 启发式 check / 仅 topic 1 capability） | OPC + Claude |

---

**下一步**：
- 用户审阅本文档，提出修改意见
- 通过后进入 Spec 自审（检查占位符/矛盾/歧义/范围）
- 然后调用 `writing-plans` skill，把 sub-spec E 拆为可执行的 M1 实施计划（3 task, 1 周）
- M1 完成后，**Phase 2 全部 5 sub-spec 完** — 全平台 foundation 上线
