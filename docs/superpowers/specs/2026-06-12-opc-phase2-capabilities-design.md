# OPC Phase 2 — 4 能力工具化设计（Sub-Spec B）

> **文档版本**：v1.0（初稿，待审）
> **日期**：2026-06-12
> **作者**：OPC 团队（与 Claude 协作）
> **状态**：设计阶段 → 待用户审阅
> **路径**：`docs/superpowers/specs/2026-06-12-opc-phase2-capabilities-design.md`
> **父 spec**：`docs/superpowers/specs/2026-06-03-opc-phase1-ip-design.md` §5.1（Phase 2 轮廓）
> **依赖 spec**：`docs/superpowers/specs/2026-06-11-opc-phase2-ip-template-design.md` v1.0（Sub-Spec A, 4 IP profile 框架, M1 已完成）
> **兄弟 spec**：M1 5 sub-spec 之 **B**（C/D/E 待启）

---

## 一、文档目的

把 Phase 1 已经 working 的 4 能力（选题 / 脚本 / 拆解 / 资产）从「埋在 handlers/ 和 agents/ 里、需要 HTTP 入口才能用」的状态，沉淀为「library 函数 + CLI + HTTP + MCP」可复用 tool 体系。M1 严格按"最小"边界：1 capability (topic) × 1 surface (CLI subcommand in opc-asset) + 顺便 refactor HTTP handler 共用 library。剩余 3 capability + 剩余 3 surface 推到后续 M-batches。

---

## 二、Executive Summary

| 项 | 内容 |
|----|------|
| **目标** | 抽出 `internal/capabilities/topic.Generate(ctx, text, input) → (*Result, error)` library；opc-asset 加 `topic` subcommand；现有 HTTP handler `POST /api/ai/topics` 重构调同一 library |
| **范围（M1）** | 1 capability (topic) × 1 surface (CLI) + 1 handler refactor；library + 5 fixtures + integration test |
| **不在 M1** | script / deconstruct / asset capability library；HTTP/MCP/library surface for topic；多 capability 并行 batch；profile-aware topic（topic 跟 IP type 联动） |
| **Pluggability** | Library 是 Go 强类型；CLI subcommand 是 stdlib flag；跟 A 风格一致 |
| **Code 位置** | `internal/capabilities/topic/` 新 package；`cmd/opc-asset/main.go` 加 subcommand；`internal/handlers/ai.go` 重构（call `topic.Generate`） |
| **Demo 模式** | Library 不支持；HTTP handler 仍保留 `?demo=true` 走 canned pool；CLI 不支持 demo（实打实调 LLM） |
| **Cost 追踪** | Library 返 `CostInfo{InputTokens, OutputTokens}`；HTTP handler 走 `StampClaudeCost` middleware；CLI 写 stderr + (M2 考虑) ledger 写一行 |
| **M1 timeline** | 2-3 周（比 A 小：library 1 个 + CLI 1 个 + handler refactor + 测试） |
| **风险等级** | 🟢 低（library 边界清晰，handler 已有 working code 可对照） |

---

## 三、背景与现状

### 3.1 Phase 1 4 能力现状（代码已 working）

| Capability | 现有入口 | 现有实现位置 |
|------------|---------|-------------|
| 选题 (topic) | `POST /api/ai/topics` (G1 closed 2026-06-09, E2E verify) | `handlers/ai.go:149-235` (handler) + `agents/prompts.go:GenerateTopicsPrompt` (prompt) + `agents/minimax.go:Text` (LLM) + `agents/demo.go:DemoOpTopics` (demo) |
| 脚本 (script) | `POST /api/ai/humanize` 等 | `handlers/ai.go:HumanizeScript` + `agents/prompts.go:HumanizePrompt` |
| 拆解 (deconstruct) | `POST /api/ai/deconstruct` | `handlers/deconstruct.go` + `agents/deconstruct.go` + `agents/prompts.go:DeconstructPrompt` |
| 资产 (asset) | `opc-asset check` / `opc-asset generate` | `cmd/opc-asset/main.go` + `internal/assetgen/profiles/<type>/` (Sub-Spec A) |

**关键观察**：4 能力**都已在工作**，但都"绑死"在某个入口上：
- 选题/脚本/拆解：只能在 HTTP POST 调用
- 资产：只能在 opc-asset CLI 调用

这意味着：
- Claude Code 通过 MCP 想用 topic 生成 → 走完整 HTTP 往返（高 latency，gzip JSON 解析）
- 批处理 SOP 想离线跑 50 个 topic → 需要 curl + jq + bash
- 自研 agent 想直接调 LLM 业务 → 必须复制 handler 内部 30 行 prompt-building + 20 行 parseTopics 代码

**B 的工作** = 把"业务逻辑"从入口层抽出来，让 library / CLI / HTTP / MCP 4 种入口都能复用。

### 3.2 为什么不直接复用 Sub-Spec A 的 sub-package 模式？

A 的 IP profile 用了 4 个 sub-package 硬边界（anthropomorphic / digital_human / costume / info），每个 sub-package 自己实现 `Check` + `BuildPrompt`。这个模式**适合 IP profile**（5-7 个 anchor 一致性检查，profile 多了才需要硬边界）。

但 B 的 4 capability 业务逻辑**差异极大**：
- Topic 返 `[]Topic` (6 字段)
- Script 返 string (humanize 后的人话)
- Deconstruct 返 `DeconstructedVideo` (viral formula + scene/hook 拆解)
- Asset 返 image/video 文件 (走 `*agents.MiniMax`，不是 Text)

**强行 4 sub-package 等于 YAGNI**。M1 一个 capability 一个 package（`capabilities/topic/`），M2 再加 `capabilities/script/` `capabilities/deconstruct/` `capabilities/asset/` 各 1 个。

### 3.3 已有可复用资产

- `internal/agents/prompts.go:GenerateTopicsPrompt(seed, platform, count)` — prompt 构造，**直接复用**
- `internal/agents/minimax.go:Text(ctx, prompt, MiniMaxTextOptions) (*MiniMaxTextResult, error)` — LLM 调用，**直接复用**
- `internal/agents/demo.go:DemoOpTopics` — demo canned pool，**保留在 HTTP handler 层**（不挪到 library）
- `internal/handlers/ai.go:parseTopics` — JSON 解析，**移到 library**（library 应该自包含）
- `internal/config/cost.go:SkillMiniMaxM27` + `config.CostForTokenSkill` — cost 换算，**复用**
- `internal/middleware/call_log.go:StampClaudeCost` — HTTP 端 cost 标记，**复用**（CLI 不走）
- Sub-Spec A 的 `internal/assetgen/` 完整 IP profile framework（topic 暂不 profile-aware，M2 再说）

---

## 四、设计目标（M1 验收）

| # | 验收 | 验证命令 |
|---|------|---------|
| 1 | `internal/capabilities/topic.Generate(ctx, text, input)` 函数存在且签名匹配 | `go doc -all ./internal/capabilities/topic` |
| 2 | `topic.Generate` 输入校验（seed/platform 必填, count 1-20） | `go test -tags fts5 -run TestValidate ./internal/capabilities/topic/...` PASS |
| 3 | `topic.Generate` 调 LLM + parseTopics 正确（5 个 fixture 测试） | `go test -tags fts5 ./internal/capabilities/topic/...` 全绿 |
| 4 | `topic` package coverage ≥ 80% | `go test -tags fts5 -cover ./internal/capabilities/topic/...` ≥ 80% |
| 5 | `opc-asset topic` subcommand 存在且 4 flag 正常（--seed/--platform/--count/--out） | `opc-asset topic --help` 列出 4 flag |
| 6 | `opc-asset topic --seed X --platform Y --count 5 --out /tmp/x.json` 写出有效 JSON 数组 | 跑一次, `cat /tmp/x.json \| jq length` 返 5 |
| 7 | `opc-asset topic` 缺必填 flag 返非 0 退出 | `opc-asset topic` (无 flag) 退 1 + Usage 打印 |
| 8 | `POST /api/ai/topics` 行为不变 (跟 Sub-Spec A G1 closed 2026-06-09 一致) | `scripts/api-smoke.sh` 5 handler cases 全 PASS |
| 9 | 4 个现有 handler (topic / humanize / postmortem / deconstruct) smoke 不回归 | `make -C apps/api asset && ./scripts/api-smoke.sh` 10/10 PASS |
| 10 | `cmd/opc-asset` 5 subcommand 全可用 (check / generate / ledger / topic + 未来 asset 自带) | `opc-asset --help` 列 4+ subcommands |

**Out of scope（M1 不做）**：
- ❌ Script / Deconstruct / Asset capability library (推到 M2/M3)
- ❌ HTTP surface for topic 之外的 capability (script / deconstruct 在 HTTP 已有，asset 走 opc-asset；这些不动)
- ❌ MCP tool 暴露 topic (Phase 1 MCP server 已有 10 tool，没加 topic 是 M2 范围)
- ❌ Topic profile-aware (topic 跟 IP type 联动，generateTopicsPrompt 调 `assetgen.LoadProfile` 取 IP 字段) — M2
- ❌ Topic 批处理 / 并行 / streaming
- ❌ CLI cost ledger 写入 (M2 决定; M1 只 stderr 输出 cost summary)
- ❌ Demo 模式在 CLI (M1 CLI 走真实 LLM; demo 留在 HTTP handler)

---

## 五、架构

### 5.1 包结构（M1 终态）

```
apps/api/internal/
├── handlers/
│   ├── ai.go                            (refactored, 调 topic.Generate)
│   └── ...
├── agents/                              (不动)
│   ├── prompts.go                       (GenerateTopicsPrompt 复用)
│   ├── minimax.go                       (Text 复用)
│   ├── demo.go                          (DemoOpTopics 保留在 handler 调)
│   └── ...
├── capabilities/                        (NEW 目录)
│   └── topic/                           (M1 only, 1 capability)
│       ├── topic.go                     (Topic + Input + Result + Generate + validate + parseTopics)
│       └── topic_test.go                (validate + parseTopics + Generate 5 fixtures)
└── cmd/opc-asset/
    ├── main.go                          (refactored, 加 topic subcommand)
    └── main_test.go                     (加 TestRunTopic)
```

**3 层 clean architecture**：
1. **LLM 层** (`agents/minimax.go`) — HTTP 调 MiniMax, 返 token + text
2. **业务层** (`capabilities/topic/`) — prompt 构造 + JSON 解析 + 输入校验
3. **入口层** (`handlers/` + `cmd/opc-asset/`) — HTTP / CLI 入口, 不含业务逻辑

### 5.2 Library API

```go
package topic

// Topic is the canonical shape of one generated topic.
// Field names match the existing HTTP wire shape (snake_case
// JSON tags) so library + HTTP API stay in sync.
type Topic struct {
    Title               string   `json:"title"`
    Angle               string   `json:"angle"`
    ExpectedPerformance string   `json:"expected_performance"`
    Hook                string   `json:"hook"`
    Pattern             string   `json:"pattern,omitempty"`
    VoiceTags           []string `json:"voice_tags,omitempty"`
}

// Input captures the variables the operator can tweak.
type Input struct {
    Seed     string
    Platform string
    Count    int
}

// Result is what Generate returns: parsed topics + cost data
// (so callers can stamp it to call_log or write to a CLI ledger).
type Result struct {
    Topics []Topic
    Cost   CostInfo
}

// CostInfo carries the per-call token counts. Callers compute
// the actual cost in CNY via config.CostForTokenSkill (HTTP
// handler uses call_log middleware; CLI prints to stderr in M1).
type CostInfo struct {
    InputTokens  int
    OutputTokens int
}

// Generate is the library entry point. It builds the prompt
// from the input, calls the LLM, parses the JSON array response,
// and returns topics + token counts. Pure: no HTTP, no middleware,
// no IO besides the LLM call.
func Generate(ctx context.Context, text *agents.MiniMax, input Input) (*Result, error)
```

### 5.3 关键设计决策

| 决策 | 选项 | 选定 | 理由 |
|------|------|------|------|
| M1 scope | 4 cap × 1 surface / 1 cap × 1 surface / 1 cap × 4 surface / 4 × 4 | **1 cap × 1 surface (topic + CLI)** | 用户最小化决定；YAGNI 优先；后续 M2/M3 加 cap/surface |
| CLI 工具 | 扩展 opc-asset / 新 opc-topic / 新 opc-capability / 不动 binary | **扩展 opc-asset (加 topic subcommand)** | 1 个 binary 维护成本低；4 capability 都能 opc-asset {topic,generate,deconstruct,check,ledger}；不增加 Makefile 复杂度 |
| Topic output | 写 JSON 文件 / JSON stdout / 人读文本 / 双输出 | **写 JSON 到 --out 文件** | 跟 generate 模式一致；可复用 (含 title/angle/hook/expected_performance 结构)；machine-friendly |
| Library 位置 | 现有 agents/ 加函数 / 新 capabilities/topic/ / 不抽 copy | **新 internal/capabilities/topic/** | 职责清（agents 是 LLM 包装，capability 是业务）；为 M2 加 script/deconstruct/asset capability 留位 |
| Handler 范围 | 重构调 library / 不动 / 删 handler | **重构调 library** | 1 个业务入口 0 重复；HTTP handler 行为不变（回归测试 已有） |
| CLI cost | 入 call_log / 写 ledger / stderr / 完全 不 追 | **stderr 输出 cost summary** | CLI 是 offline/batch tool，call_log middleware 本为 HTTP 写；M1 最简 |
| Demo 模式 | Library 支持 / Handler 支持 / CLI 支持 / 全不支持 | **Handler 保留 (refactor 不影响); Library/CLI 不支持** | demo 是 HTTP UX 概念；library 是纯业务；CLI 应走真 LLM |
| Library 验证 | Library 内 / Caller / 两层 | **Library 内（defense in depth 通过双层也 OK，spec 选单层）** | Library 是 contract 入口；validate 集中一处 caller 简单 |
| Library 错误 | Wrap err / 返 string / 返 typed error | **fmt.Errorf("topic: %w", err) wrap** | 标准 Go 风格；caller 可用 errors.Is/As |
| parseTopics 位置 | Library / Handler 保留 / 两份 | **Library** | library 自包含；handler 不需要知道 LLM 输出格式 |
| M1 package 命名 | `capabilities/topic` (内部 package 名 `topic`) / `topics` (复数) / `topicgen` | **`capabilities/topic` (内部名 `topic`)** | 单数对单 capability 清晰；`topic.Topic` 跟 `time.Time` 同惯例 |

---

## 六、组件

### 6.1 Library `internal/capabilities/topic/`

**职责**：纯业务逻辑。prompt 构造 + LLM 调用 + JSON 解析 + 输入校验。**不依赖** HTTP context / 中间件 / DB。

**`topic.go`** 导出：
- `Topic` (struct, 6 JSON 字段)
- `Input` (struct, 3 字段: Seed/Platform/Count)
- `Result` (struct: Topics []Topic + Cost CostInfo)
- `CostInfo` (struct: InputTokens + OutputTokens)
- `Generate(ctx, text, input) (*Result, error)`

**私有**：
- `validate(input) error` — 4 条规则（empty seed/platform, count ≤ 0, count > 20）
- `parseTopics(raw string) ([]Topic, error)` — 从 LLM 输出提 JSON 数组 (容忍 markdown fence, 拒绝 > 256KB, 拒绝无数组)

### 6.2 HTTP Handler `internal/handlers/ai.go` (refactored)

**`GenerateTopics` 重构后**：
```go
func (h *AIHandler) GenerateTopics(c *gin.Context) {
    // 1. Bind + validate (HTTP-shape)
    var req generateTopicsRequest
    if err := c.ShouldBindJSON(&req); err != nil { ... 400 ... }
    req.Seed = strings.TrimSpace(req.Seed)
    req.Platform = strings.TrimSpace(req.Platform)
    if req.Seed == "" { ... 400 ... }
    if req.Platform == "" { ... 400 ... }
    if req.Count <= 0 { req.Count = 5 }
    if req.Count > 20 { req.Count = 20 }

    // 2. Demo mode (HTTP-specific, 不挪到 library)
    if h.shouldUseDemo(c) {
        raw, _ := agents.DemoResponse(agents.DemoOpTopics, len(req.Seed))
        topics, err := parseTopicsLegacy(raw)  // 现有 demo 解析路径
        if err != nil { ... 502 ... }
        if len(topics) > req.Count { topics = topics[:req.Count] }
        markDemoResponse(c)
        c.JSON(http.StatusOK, generateTopicsResponse{Topics: topics})
        return
    }

    // 3. Library call (新)
    ctx, cancel := context.WithTimeout(c.Request.Context(), aiTimeout)
    defer cancel()
    res, err := topic.Generate(ctx, h.text, topic.Input{
        Seed: req.Seed, Platform: req.Platform, Count: req.Count,
    })
    if err != nil {
        h.logger.Error("topic generate failed", "err", err.Error(), "request_id", c.GetString("request_id"))
        c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
        return
    }

    // 4. Cost stamp (HTTP-specific, 走 call_log middleware)
    StampClaudeCost(c, config.SkillMiniMaxM27, res.Cost.InputTokens, res.Cost.OutputTokens)

    // 5. Response
    c.JSON(http.StatusOK, generateTopicsResponse{Topics: res.Topics})
}
```

**handler 内部函数保留**：
- `parseTopics`（demo 路径用，库内同名函数是新版）— 改名 `parseTopicsLegacy` 避免冲突
- 其它 helper 不动

### 6.3 CLI Subcommand `cmd/opc-asset/main.go`

**`runTopic(args []string, logger *slog.Logger) error`**：
```go
func runTopic(args []string, logger *slog.Logger) error {
    fs := flag.NewFlagSet("topic", flag.ExitOnError)
    seed := fs.String("seed", "", "Topic seed phrase (required)")
    platform := fs.String("platform", "", "Target platform: 抖音/哔哩哔哩/小红书 (required)")
    count := fs.Int("count", 5, "Number of topics to generate (1-20)")
    out := fs.String("out", "", "Output JSON file path (required)")
    if err := fs.Parse(args); err != nil { return err }
    if *seed == "" || *platform == "" || *out == "" {
        fs.Usage()
        return errors.New("--seed, --platform, --out are required")
    }

    // 复用 API server 的 minimax client (同 env var: ANTHROPIC_API_KEY)
    text := agents.NewMiniMax()
    if !text.Available() {
        return errors.New("minimax: API key not set (ANTHROPIC_API_KEY env var)")
    }

    ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
    defer cancel()

    res, err := topic.Generate(ctx, text, topic.Input{
        Seed: *seed, Platform: *platform, Count: *count,
    })
    if err != nil {
        return fmt.Errorf("generate failed: %w", err)
    }

    blob, err := json.MarshalIndent(res.Topics, "", "  ")
    if err != nil { return err }
    if err := os.WriteFile(*out, blob, 0644); err != nil {
        return fmt.Errorf("write %s: %w", *out, err)
    }

    fmt.Fprintf(os.Stderr, "Wrote %d topics to %s (cost: %d in / %d out tokens)\n",
        len(res.Topics), *out, res.Cost.InputTokens, res.Cost.OutputTokens)
    return nil
}
```

**`main()` switch 新增**：
```go
case "topic":
    err = runTopic(os.Args[2:], logger)
```

### 6.4 Test 覆盖矩阵

| 层 | 覆盖 | 文件 | 数量 |
|----|------|------|------|
| Unit: validate | empty seed / empty platform / count 0 / count 21 | topic_test.go | 4 |
| Unit: parseTopics | 完美 array / markdown fence / no array / too large / invalid JSON | topic_test.go | 5 |
| Unit: Generate (mock LLM) | perfect / partial (3 topics) / trims to count / empty response / wrong JSON | topic_test.go | 5 |
| Integration: CLI | TestRunTopic (4 subtest: perfect/missing-flag/empty-result/invalid-platform) | main_test.go | 4 |
| Regression: HTTP handler | 现有 `scripts/api-smoke.sh` 5 handler cases | 无需新 test | 0 |

**Coverage 目标**: `capabilities/topic` package ≥ 80% line coverage.

---

## 七、Schema 设计

### 7.1 Library Input Schema

```go
type Input struct {
    Seed     string  // required, non-empty after TrimSpace
    Platform string  // required, non-empty after TrimSpace, free string (handler validates against allowed list)
    Count    int     // required, 1-20 inclusive
}
```

**注**：Library 不强制 platform 必须是 `抖音/哔哩哔哩/小红书` 之一（那是 model 层约束）。Library 只校验非空。HTTP handler 走 `models.AllowedPlatforms` 校验，CLI 暂不强制（用户可能想试新平台）。

### 7.2 Library Output Schema

```go
type Topic struct {
    Title               string   `json:"title"`
    Angle               string   `json:"angle"`
    ExpectedPerformance string   `json:"expected_performance"`
    Hook                string   `json:"hook"`
    Pattern             string   `json:"pattern,omitempty"`
    VoiceTags           []string `json:"voice_tags,omitempty"`
}
```

**JSON wire format** (CLI `--out` 写出):
```json
[
  {
    "title": "...",
    "angle": "...",
    "expected_performance": "...",
    "hook": "...",
    "pattern": "...",          // optional
    "voice_tags": ["..."]      // optional
  }
]
```

跟现有 HTTP API 的 `generateTopicsResponse{Topics: []generatedTopic}` 区别：
- Library output: `[Topic, Topic, ...]` (raw array)
- HTTP output: `{"topics": [Topic, Topic, ...]}` (envelope)

CLI 写 raw array（更紧凑，jq-friendly）。HTTP 保留 envelope（向后兼容）。

### 7.3 CLI CLI Help Text

```
$ opc-asset topic --help
Usage of topic:
  -count int
        Number of topics to generate (1-20) (default 5)
  -out string
        Output JSON file path (required)
  -platform string
        Target platform: 抖音/哔哩哔哩/小红书 (required)
  -seed string
        Topic seed phrase (required)
```

---

## 八、Consistency Check 设计

N/A — B 单元是 capability library，无 consistency check。Consistency check 属于 Sub-Spec A（IP profile）。

---

## 九、Prompt 构建

**Library 不构造 prompt** — 调 `agents.GenerateTopicsPrompt(seed, platform, count)`。Prompt 模板所有权在 `agents/prompts.go`（已有版本），library 复用。

**M1 不动 prompt 模板**。如果未来要 profile-aware topic（topic 跟 IP type 联动），prompt 模板需要注入 IP 字段。这是 M2 范围。

---

## 十、数据流

### 10.1 启动期

```
无启动期依赖。`internal/capabilities/topic` 是纯 library,
不需要 init() 或 registry。`agents.MiniMax` 由 caller 提供
(handler 注入 `*AIHandler.text`, CLI 调 `NewMiniMax()` 从 env 读).
```

### 10.2 运行时（CLI topic subcommand）

```
$ opc-asset topic --seed "个人成长" --platform 抖音 --count 5 --out topics.json
  ↓
1. flag 解析: seed, platform, count, out
2. validate (3 required fields)
   ↓ fail
   返 error + fs.Usage() + exit 2
   ↓ pass
3. agents.NewMiniMax() — 读 env ANTHROPIC_API_KEY
   ↓ fail (无 key)
   返 "minimax: API key not set" + exit 1
   ↓ pass
4. topic.Generate(ctx, text, input)
   4.1 validate seed/platform 非空 + count 1-20
       ↓ fail 返 error
   4.2 agents.GenerateTopicsPrompt(seed, platform, count) → prompt
   4.3 text.Text(ctx, prompt, MiniMaxTextOptions{Model: "MiniMax-M2.7-highspeed"})
       ↓ fail (network/key/rate limit) 返 wrapped error
   4.4 parseTopics(result.Text)
       ↓ fail (no JSON / invalid JSON / > 256KB) 返 error
   4.5 trim to count
   4.6 return {Topics, Cost}
5. json.MarshalIndent(topics) → blob
6. os.WriteFile(*out, blob, 0644)
   ↓ fail (磁盘/权限) 返 wrapped error
7. stderr: "Wrote 5 topics to topics.json (cost: 1234 in / 567 out tokens)\n"
8. exit 0
```

### 10.3 运行时（HTTP handler GenerateTopics, refactored）

```
$ curl -X POST http://localhost:8080/api/ai/topics \
    -H "Content-Type: application/json" \
    -d '{"seed": "个人成长", "platform": "抖音", "count": 5}'
  ↓
1. Gin 路由 → AIHandler.GenerateTopics(c)
2. c.ShouldBindJSON(&req)  →  parse JSON
   ↓ fail 返 400
3. 校验 seed/platform/count (handler-level)
   ↓ fail 返 400
4. demo check (h.shouldUseDemo(c))
   ↓ true 走 demo 路径 (canned pool) → 返 JSON envelope
   ↓ false 继续
5. topic.Generate(ctx, h.text, input)
   5.1-5.6 跟 CLI 一样
   ↓ fail 返 502 (Service Unavailable)
6. StampClaudeCost(c, config.SkillMiniMaxM27, res.Cost.InputTokens, res.Cost.OutputTokens)
7. c.JSON(200, generateTopicsResponse{Topics: res.Topics})
```

---

## 十一、错误处理

| 错误 | 检测 | 处理 |
|------|------|------|
| Missing flag (--seed/--platform/--out) | CLI flag 解析后 | 返 error + fs.Usage() + exit 2 |
| count ≤ 0 | library validate | 返 "count must be > 0" |
| count > 20 | library validate | 返 "count must be <= 20" |
| API key missing | agents.NewMiniMax().Available() | 返 "minimax: API key not set" + exit 1 |
| LLM call failed (network/key/rate limit/timeout) | text.Text() | 返 wrapped error, library 透传 |
| LLM 返非 JSON 数组 | parseTopics | 返 "no JSON array found in response" |
| LLM 返> 256KB | parseTopics 入口 | 返 "response too large" |
| LLM 返 invalid JSON | parseTopics | 返 "invalid JSON: <wrap>" |
| 写文件失败 (磁盘/权限) | os.WriteFile | 返 wrapped error + exit 1 |
| 生成 topics 数 < count | parseTopics | 不报错, library 返实际数 (LLM 偶尔少返) |
| 生成 topics 数 > count | parseTopics 后 trim | 不报错, library trim 到 count |

---

## 十二、测试策略

### 12.1 Unit Tests (topic package)

**12 个 fixture** (8 个库 + 4 个 CLI 不在库测):

```go
package topic_test

import (
    "context"
    "errors"
    "strings"
    "testing"

    "github.com/opc/api/internal/agents"
    "github.com/opc/api/internal/capabilities/topic"
)

// TestValidateEmptySeed
func TestValidateEmptySeed(t *testing.T) {
    _, err := topic.Generate(context.Background(), nil, topic.Input{Seed: "", Platform: "抖音", Count: 5})
    if err == nil || !strings.Contains(err.Error(), "seed") {
        t.Errorf("expected seed error, got %v", err)
    }
}

// TestValidateEmptyPlatform
// TestValidateCountZero (count <= 0)
// TestValidateCountTooLarge (count > 20)
// TestParseTopicsPerfect
// TestParseTopicsMarkdownFence (```json [...] ``` wrapped)
// TestParseTopicsNoArray (raw text has no JSON array)
// TestParseTopicsTooLarge (> 256KB rejected)
// TestParseTopicsInvalidJSON (malformed JSON)
// TestGeneratePerfect (mock LLM, 5 topics returned)
// TestGenerateTrimsToCount (mock LLM returns 8, library trims to 5)
// TestGenerateLLMError (mock LLM returns error, library wraps)
// TestGenerateParseError (mock LLM returns invalid JSON)
```

**Mock LLM** 用 `agents.NewMiniMaxWithTextOverride(fn)` 注入 fake `textOverride` 函数：
```go
fakeText := agents.NewMiniMaxWithTextOverride(func(ctx context.Context, prompt string, opts agents.MiniMaxTextOptions) (*agents.MiniMaxTextResult, error) {
    return &agents.MiniMaxTextResult{
        Text:         `[{"title":"T1","angle":"A1","expected_performance":"P1","hook":"H1"}]`,
        InputTokens:  100,
        OutputTokens: 50,
    }, nil
})
res, err := topic.Generate(ctx, fakeText, topic.Input{...})
```

### 12.2 Integration Tests (opc-asset CLI)

**`cmd/opc-asset/main_test.go`** 加 `TestRunTopic`:

```go
func TestRunTopicMissingFlags(t *testing.T) {
    args := []string{"topic"}  // no --seed/--platform/--out
    err := runTopic(args, slog.Default())
    if err == nil {
        t.Fatal("expected error for missing flags")
    }
}

// TestRunTopicWithMockLLM — requires refactor to allow test injection
//   (probably: add a package-level var for the minimax constructor that
//   tests can override, OR run the test with real LLM via env var)
// M1 simplification: only test the missing-flags path in unit test;
//   full E2E runs via `scripts/api-smoke.sh` (extended with topic case).
```

**Decision**: M1 CLI unit test 限于 missing-flags path (不依赖 LLM mock)。Full E2E 通过 `scripts/api-smoke.sh` 加 topic case 覆盖。

### 12.3 E2E Smoke (`scripts/api-smoke.sh`)

新增 1 case (用真实 server + 真实 LLM, 跟现有 5 handler case 一致):

```bash
# === Sub-Spec B: topic capability (CLI + HTTP smoke) ===
section "topic capability"

# HTTP 端 smoke (跟 G1-closed 2026-06-09 一致, 不回归)
out=$(curl -s -X POST http://localhost:8081/api/ai/topics \
    -H "Content-Type: application/json" \
    -H "Cookie: opc_session=$SESSION_TOKEN" \
    -d '{"seed":"个人成长","platform":"抖音","count":5}')
topic_count=$(echo "$out" | jq '.topics | length')
if [ "$topic_count" = "5" ]; then
    pass "topic HTTP: 5 topics returned"
else
    fail "topic HTTP: got $topic_count, want 5"
fi

# CLI 端 smoke (新, 需 先 build binary)
make -C apps/api asset 2>&1 | tail -1
CLI_OUT=$(mktemp)
if opc-asset topic --seed "个人成长" --platform 抖音 --count 5 --out "$CLI_OUT" 2>/dev/null; then
    cli_count=$(jq length < "$CLI_OUT")
    if [ "$cli_count" = "5" ]; then
        pass "topic CLI: 5 topics written to $CLI_OUT"
    else
        fail "topic CLI: jq length = $cli_count, want 5"
    fi
else
    fail "topic CLI: opc-asset topic exited non-zero (ANTHROPIC_API_KEY missing?)"
fi
rm -f "$CLI_OUT"
```

**注**: CLI smoke 依赖 `ANTHROPIC_API_KEY` env var. 在 CI 跑会用真实 key; 在 dev 跑会 skip 这 case (跟现有 handler smoke 一致).

---

## 十三、实施计划（M1 timeline）

| 周 | 任务 | 交付物 |
|----|------|--------|
| W1 | Library `internal/capabilities/topic/` (topic.go + topic_test.go) | 14 个 unit test PASS, coverage ≥ 80% |
| W1 | HTTP handler `GenerateTopics` 重构调 library | 4 个老 handler smoke 不回归 (跑 `scripts/api-smoke.sh`) |
| W2 | CLI subcommand `topic` (main.go 加 runTopic + subcommand 路由) | 4 flag 解析 + 调用 library + 写文件 + stderr |
| W2 | CLI test (TestRunTopic missing-flags) | 1 个 unit test PASS |
| W2 | `scripts/api-smoke.sh` 加 topic case (HTTP + CLI) | 2 个新 case, 10/10 PASS |
| W3 | verify-project-state agent run 22 → OVERALL PASS | M1 closed |

**W3 末 verify agent 必跑**, OVERALL PASS 才算 M1 完。

---

## 十四、风险与不做的事

### 14.1 风险

| 风险 | 缓解 |
|------|------|
| Library refactor 破 HTTP handler (G1-closed 2026-06-09 行为变化) | 老 smoke 必跑; handler 路径写 wrap error 同 demo 路径不冲突 |
| parseTopics 移到 library 后, demo 路径 (现有 handler 内部) 调老 parseTopics 需重命名 | 改名 `parseTopicsLegacy` 显式; 测试覆盖 demo 路径 |
| CLI 缺 `ANTHROPIC_API_KEY` 跑不出真实 topics, 测不出 end-to-end | smoke 设计成"无 key 跳过" (跟现有 handler smoke 一致); 真实 CI 用 secret 跑 |
| topic.MiniMaxTextOptions 的 Model 字段硬编码 `"MiniMax-M2.7-highspeed"` | 跟现有 handler 一致; M1 不抽 model 切换 (M2 再说) |
| Library 接收 `*agents.MiniMax` 是强类型依赖, 未来切 DeepSeek/Gemini 难 | M1 沿用 A 模式 (硬类型 + 单一 provider); M2 abstract `*agents.TextProvider` interface |
| count > 20 hard cap | 跟现有 handler 一致; 拒绝是不让 token 烧爆 |

### 14.2 不做的事（M1 范围硬约束）

- ❌ Script / Deconstruct / Asset capability library (M2/M3)
- ❌ Topic profile-aware (M2)
- ❌ HTTP surface for script / deconstruct asset (已有不动)
- ❌ MCP tool 加 topic (M2)
- ❌ CLI batch / 并行 / streaming (M2)
- ❌ CLI cost ledger 写入 (M2 决定)
- ❌ Library demo 模式 (M1 仅 HTTP handler 保留 demo)
- ❌ Platform enum 强制 (抖音/哔哩哔哩/小红书) — Library 接受任何非空字符串, handler 走 `models.AllowedPlatforms`
- ❌ 改 `internal/agents/{prompts,minimax}.go` — 复用, 不动
- ❌ 删 `internal/handlers/ai.go:parseTopics` — 改名 `parseTopicsLegacy` 给 demo 路径用
- ❌ Topic cap > 20 (跟 Phase 1 行为一致, 硬 cap)
- ❌ Profile-aware topic (M2)

---

## 十五、与 Sub-Spec A 既有代码的关系

| 已有 | M1 处置 | 理由 |
|------|--------|------|
| `internal/agents/prompts.go:GenerateTopicsPrompt` | 不动 | Library 复用 |
| `internal/agents/minimax.go:Text` | 不动 | Library 复用 |
| `internal/agents/demo.go:DemoOpTopics` | 不动, handler 仍调 | Demo 走 HTTP handler 层 |
| `internal/handlers/ai.go:parseTopics` | 改名 `parseTopicsLegacy`, 内容不变 (给 demo 路径) | Library 自包含 |
| `internal/handlers/ai.go:GenerateTopics` | 重构调 `topic.Generate` | 1 个业务入口 0 重复 |
| `internal/handlers/ai.go:generateTopicsRequest/Response` | 不动 (HTTP wire 形状) | 向后兼容 G1-closed 2026-06-09 |
| `internal/handlers/ai.go:shouldUseDemo` | 不动 | Demo 决策点保留在 handler |
| `internal/middleware/call_log.go:StampClaudeCost` | 不动, handler 继续调 | Cost 标记在 HTTP 中间件层 |
| `internal/config/cost.go:SkillMiniMaxM27` | 不动 | Cost 换算逻辑 |
| `cmd/opc-asset/main.go` (check/generate/ledger) | 不动 | 老 subcommand 保留 |
| `cmd/opc-asset/main.go` (subcommand 路由) | 加 `case "topic":` | 新 subcommand 入口 |
| `internal/assetgen/` (Sub-Spec A 完整 framework) | 不动 | Topic M1 不 profile-aware |

---

## 十六、与 Phase 2 其他 sub-spec 的关系

| Sub-spec | 与本 sub-spec 的关系 |
|----------|---------------------|
| A (IP profile) | 已完成 ✅。M1 topic 暂不 profile-aware; M2 扩为 "topic + IP type" |
| B (4 能力工具化) | 本 sub-spec。M1 = topic + CLI; M2 = script + deconstruct + asset libraries + 3 surface (HTTP/MCP/library) |
| C (对外 web UI) | 依赖 B 完。M1 之后启 C, 复用 topic library |
| D (agent-agnostic 平台) | 跟 B 并行。D 不依赖 B (Phase 1 MCP 已有 10 tool, 跟 topic 无关) |
| E (反 AI 检测) | 依赖 A consistency check。跟 B 弱关联 (拟人化脚本 = 改 prompt 模板) |

**关键路径** (按 Phase 1 spec §16):
```
A ✅ → B (本) → C
       ↓
       D (parallel)
       E (parallel, 需 A)
```

B 完成后, 即可启 C (对外 web UI) 跟 D (agent-agnostic) 并行。

---

## 十七、变更记录

| 版本 | 日期 | 变更 | 作者 |
|------|------|------|------|
| v1.0 | 2026-06-12 | 初稿（基于 brainstorming 4 问澄清：1 capability × 1 surface / opc-asset 扩展 / JSON 写文件 / 新 internal/capabilities/topic/ / Handler 重构 + CLI 不入 call_log） | OPC + Claude |

---

**下一步**：
- 用户审阅本文档，提出修改意见
- 通过后进入 Spec 自审（检查占位符/矛盾/歧义/范围）
- 然后调用 `writing-plans` skill，把 sub-spec B 拆为可执行的 M1 实施计划
- M1 完成后，依次启 C/D/E sub-spec 的 brainstorming → spec → plan 循环
