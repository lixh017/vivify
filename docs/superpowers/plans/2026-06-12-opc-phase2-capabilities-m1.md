# OPC Phase 2 — 4 能力工具化 M1 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 Phase 1 4 能力 (选题/脚本/拆解/资产) 的第 1 步 — **选题 (topic)** — 抽成纯 library `internal/capabilities/topic.Generate(ctx, text, input)` + 现有 HTTP handler 重构调 library + opc-asset 加 `topic` subcommand 调同一 library. M1 走 1 capability × 1 surface, 2-3 周内 verify-project-state agent 跑出 OVERALL PASS.

**Architecture:** 3 层 clean architecture: LLM 包装 (`agents/minimax.go:Text`) → 业务层 (`capabilities/topic/`) → 入口层 (`handlers/ai.go:GenerateTopics` HTTP + `cmd/opc-asset` CLI subcommand). 1 个 library 入口, 2 个 caller 复用, 0 业务代码重复. Library 自包含 (含 validate + parseTopics), 跟 Sub-Spec A 风格一致 (硬类型 + 单一 provider).

**Tech Stack:** Go 1.25, stdlib `flag` (CLI), `encoding/json` (parse), `context` (timeout), `os` (file write), `agents.MiniMax.Text` (LLM), `agents.GenerateTopicsPrompt` (prompt template), 现有 `config/cost.go` (cost 换算), 现有 `middleware/call_log.go:StampClaudeCost` (HTTP 端 cost 标记).

**Spec:** `docs/superpowers/specs/2026-06-12-opc-phase2-capabilities-design.md` v1.0 (commit `f00eb1e`).

---

## 文件结构（M1 终态）

**新建**:
- `apps/api/internal/capabilities/topic/topic.go` — Topic + Input + Result + Generate + validate + parseTopics
- `apps/api/internal/capabilities/topic/topic_test.go` — 14 个 unit test (4 validate + 5 parseTopics + 5 Generate)

**修改**:
- `apps/api/internal/handlers/ai.go` — `GenerateTopics` 重构调 `topic.Generate`; `parseTopics` 改名 `parseTopicsLegacy` (给 demo 路径用)
- `apps/api/cmd/opc-asset/main.go` — 加 `runTopic` 函数 + `case "topic":` 路由 + 4 flag
- `apps/api/cmd/opc-asset/main_test.go` — 加 `TestRunTopicMissingFlags` (1 个 case, 不依赖 LLM)
- `scripts/api-smoke.sh` — 加 1 个 HTTP topic case + 1 个 CLI topic case

**保留不变**:
- `internal/agents/prompts.go` — `GenerateTopicsPrompt` 复用
- `internal/agents/minimax.go` — `Text` 复用
- `internal/agents/demo.go` — `DemoOpTopics` 保留 (handler demo 路径仍调)
- `internal/middleware/call_log.go` — `StampClaudeCost` 复用
- `internal/config/cost.go` — `SkillMiniMaxM27` 复用
- `cmd/opc-asset/main.go` (check / generate / ledger) — 不动

---

## 命名约定

| 类型 | 名称 | 说明 |
|------|------|------|
| Capability output struct | `topic.Topic` | 跟 `models.Topic` (DB row) 区分 — DB row 用 `models.Topic`, library output 用 `topic.Topic` |
| Capability input struct | `topic.Input` | Seed + Platform + Count |
| Capability result struct | `topic.Result` | `Topics []Topic` + `Cost CostInfo` |
| Token counts | `topic.CostInfo{InputTokens, OutputTokens}` | caller 决定怎么换算 cost_cents |
| Library entry | `topic.Generate(ctx, text, input) (*Result, error)` | 唯一 公开 函数 |
| Internal validate | `topic.validate(input) error` | 私有, 4 条规则 |
| Internal parse | `topic.parseTopics(raw string) ([]Topic, error)` | 私有, 移到 library |
| Legacy parse (demo 路径) | `handlers.parseTopicsLegacy(raw)` | 改名给 demo 用, 内容不变 |
| CLI flag | `--seed` / `--platform` / `--count` / `--out` | 4 flag |
| CLI output | JSON array `[Topic, ...]` 写到 `--out` 文件 | 跟 generate subcommand 同模式 |
| CLI cost | stderr line "Wrote N topics to FILE (cost: X in / Y out tokens)" | M1 不写 ledger |

---

## 任务分解

---

### Task 1: Library `internal/capabilities/topic/` 完整实现 (含 14 unit tests)

**Files:**
- Create: `apps/api/internal/capabilities/topic/topic.go`
- Create: `apps/api/internal/capabilities/topic/topic_test.go`

- [ ] **Step 1.1: 写 14 个 test (TDD red) — 全部放进新建 `topic_test.go`**

```go
// Package topic_test contains unit tests for the topic capability
// library (internal/capabilities/topic). Tests use the
// agents.NewMiniMaxWithTextOverride hook to mock the LLM, so
// no real API key is required.
package topic_test

import (
    "context"
    "encoding/json"
    "strings"
    "testing"

    "github.com/opc/api/internal/agents"
    "github.com/opc/api/internal/capabilities/topic"
)

// ====================
// validate tests (4)
// ====================

func TestValidateEmptySeed(t *testing.T) {
    t.Parallel()
    _, err := topic.Generate(context.Background(), nil, topic.Input{
        Seed: "", Platform: "抖音", Count: 5,
    })
    if err == nil {
        t.Fatal("expected error for empty seed")
    }
    if !strings.Contains(err.Error(), "seed") {
        t.Errorf("error %q should mention 'seed'", err.Error())
    }
}

func TestValidateEmptyPlatform(t *testing.T) {
    t.Parallel()
    _, err := topic.Generate(context.Background(), nil, topic.Input{
        Seed: "个人成长", Platform: "", Count: 5,
    })
    if err == nil {
        t.Fatal("expected error for empty platform")
    }
    if !strings.Contains(err.Error(), "platform") {
        t.Errorf("error %q should mention 'platform'", err.Error())
    }
}

func TestValidateCountZero(t *testing.T) {
    t.Parallel()
    _, err := topic.Generate(context.Background(), nil, topic.Input{
        Seed: "个人成长", Platform: "抖音", Count: 0,
    })
    if err == nil {
        t.Fatal("expected error for count <= 0")
    }
    if !strings.Contains(err.Error(), "count") {
        t.Errorf("error %q should mention 'count'", err.Error())
    }
}

func TestValidateCountTooLarge(t *testing.T) {
    t.Parallel()
    _, err := topic.Generate(context.Background(), nil, topic.Input{
        Seed: "个人成长", Platform: "抖音", Count: 21,
    })
    if err == nil {
        t.Fatal("expected error for count > 20")
    }
    if !strings.Contains(err.Error(), "count") {
        t.Errorf("error %q should mention 'count'", err.Error())
    }
}

// ====================
// parseTopics tests (5) — exercised via Generate mock
// ====================

func TestParseTopicsPerfect(t *testing.T) {
    t.Parallel()
    raw := `[{"title":"T1","angle":"A1","expected_performance":"P1","hook":"H1","pattern":"PAT","voice_tags":["a","b"]}]`
    res, err := runWithMockLLM(t, raw, nil)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(res.Topics) != 1 {
        t.Fatalf("len(Topics) = %d, want 1", len(res.Topics))
    }
    if res.Topics[0].Title != "T1" {
        t.Errorf("Title = %q, want T1", res.Topics[0].Title)
    }
}

func TestParseTopicsMarkdownFence(t *testing.T) {
    t.Parallel()
    raw := "```json\n[{\"title\":\"T1\",\"angle\":\"A1\",\"expected_performance\":\"P1\",\"hook\":\"H1\"}]\n```"
    res, err := runWithMockLLM(t, raw, nil)
    if err != nil {
        t.Fatalf("markdown-fence should be stripped, got: %v", err)
    }
    if len(res.Topics) != 1 {
        t.Errorf("len(Topics) = %d, want 1", len(res.Topics))
    }
}

func TestParseTopicsNoArray(t *testing.T) {
    t.Parallel()
    raw := "no JSON array here, just text"
    _, err := runWithMockLLM(t, raw, nil)
    if err == nil {
        t.Fatal("expected error when no JSON array in response")
    }
    if !strings.Contains(err.Error(), "no JSON array") {
        t.Errorf("error %q should mention 'no JSON array'", err.Error())
    }
}

func TestParseTopicsTooLarge(t *testing.T) {
    t.Parallel()
    // 257KB of garbage
    big := strings.Repeat("x", 257*1024)
    _, err := runWithMockLLM(t, big, nil)
    if err == nil {
        t.Fatal("expected error for too-large response")
    }
    if !strings.Contains(err.Error(), "too large") {
        t.Errorf("error %q should mention 'too large'", err.Error())
    }
}

func TestParseTopicsInvalidJSON(t *testing.T) {
    t.Parallel()
    raw := `[{"title":broken json}]`
    _, err := runWithMockLLM(t, raw, nil)
    if err == nil {
        t.Fatal("expected error for invalid JSON")
    }
    if !strings.Contains(err.Error(), "invalid JSON") {
        t.Errorf("error %q should mention 'invalid JSON'", err.Error())
    }
}

// ====================
// Generate tests (5)
// ====================

func TestGeneratePerfect(t *testing.T) {
    t.Parallel()
    raw := `[{"title":"T1","angle":"A","expected_performance":"P","hook":"H"},
            {"title":"T2","angle":"A","expected_performance":"P","hook":"H"},
            {"title":"T3","angle":"A","expected_performance":"P","hook":"H"},
            {"title":"T4","angle":"A","expected_performance":"P","hook":"H"},
            {"title":"T5","angle":"A","expected_performance":"P","hook":"H"}]`
    res, err := runWithMockLLM(t, raw, nil)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(res.Topics) != 5 {
        t.Errorf("len(Topics) = %d, want 5", len(res.Topics))
    }
    if res.Cost.InputTokens != 100 || res.Cost.OutputTokens != 50 {
        t.Errorf("Cost = %+v, want {100, 50}", res.Cost)
    }
}

func TestGenerateTrimsToCount(t *testing.T) {
    t.Parallel()
    // LLM returns 8 topics, library should trim to 5.
    var rawBuilder strings.Builder
    rawBuilder.WriteString("[")
    for i := 0; i < 8; i++ {
        if i > 0 {
            rawBuilder.WriteString(",")
        }
        rawBuilder.WriteString(`{"title":"T` + fmtInt(i) + `","angle":"A","expected_performance":"P","hook":"H"}`)
    }
    rawBuilder.WriteString("]")
    res, err := runWithMockLLM(t, rawBuilder.String(), nil)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(res.Topics) != 5 {
        t.Errorf("len(Topics) = %d, want 5 (library should trim)", len(res.Topics))
    }
}

func TestGenerateLLMError(t *testing.T) {
    t.Parallel()
    _, err := runWithMockLLM(t, "", &llmError{msg: "API rate limit"})
    if err == nil {
        t.Fatal("expected error from LLM call")
    }
    if !strings.Contains(err.Error(), "rate limit") {
        t.Errorf("error %q should mention 'rate limit'", err.Error())
    }
}

func TestGenerateParseError(t *testing.T) {
    t.Parallel()
    _, err := runWithMockLLM(t, "garbage", nil)
    if err == nil {
        t.Fatal("expected parse error")
    }
}

func TestGenerateAcceptsFewerTopicsThanCount(t *testing.T) {
    t.Parallel()
    // LLM returns only 2 topics when asked for 5. Library should
    // not error — return what it got.
    raw := `[{"title":"T1","angle":"A","expected_performance":"P","hook":"H"},
            {"title":"T2","angle":"A","expected_performance":"P","hook":"H"}]`
    res, err := runWithMockLLM(t, raw, nil)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(res.Topics) != 2 {
        t.Errorf("len(Topics) = %d, want 2 (LLM returned fewer, no error)", len(res.Topics))
    }
}

// ====================
// Test helpers
// ====================

// runWithMockLLM runs topic.Generate with a mocked LLM that
// returns raw text (or an error). Returns the result and any error.
func runWithMockLLM(t *testing.T, raw string, returnErr error) (*topic.Result, error) {
    t.Helper()
    text := agents.NewMiniMaxWithTextOverride(
        func(ctx context.Context, prompt string, opts agents.MiniMaxTextOptions) (*agents.MiniMaxTextResult, error) {
            if returnErr != nil {
                return nil, returnErr
            }
            return &agents.MiniMaxTextResult{
                Text:         raw,
                InputTokens:  100,
                OutputTokens: 50,
            }, nil
        },
    )
    return topic.Generate(context.Background(), text, topic.Input{
        Seed: "个人成长", Platform: "抖音", Count: 5,
    })
}

// llmError is a simple error type for TestGenerateLLMError.
type llmError struct{ msg string }

func (e *llmError) Error() string { return e.msg }

// fmtInt formats an int as a string (helper to avoid importing fmt).
func fmtInt(n int) string {
    if n == 0 {
        return "0"
    }
    var b [20]byte
    i := len(b)
    for n > 0 {
        i--
        b[i] = byte('0' + n%10)
        n /= 10
    }
    return string(b[i:])
}
```

- [ ] **Step 1.2: 跑 test 确认 FAIL (package + functions 还没建)**

Run: `cd /root/workspace/opc/apps/api && go test -tags fts5 ./internal/capabilities/topic/...`
Expected: FAIL (package doesn't exist: "no Go files in /root/workspace/opc/apps/api/internal/capabilities/topic")

- [ ] **Step 1.3: 新建 `apps/api/internal/capabilities/topic/topic.go` (完整 library)**

```go
// Package topic implements the topic generation capability.
//
// Generate is the library entry point. It is reused by:
//   - internal/handlers/ai.go:GenerateTopics (HTTP POST /api/ai/topics)
//   - cmd/opc-asset/main.go:runTopic (CLI subcommand)
//
// The library is pure: no HTTP context, no middleware, no IO
// besides the LLM call (which is provided by the caller via
// *agents.MiniMax). The HTTP handler adds demo-mode + call_log
// cost stamping on top; the CLI adds file output + stderr cost
// summary on top.
package topic

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "regexp"
    "strings"

    "github.com/opc/api/internal/agents"
)

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

// maxAIResultBytes is the upper bound on raw LLM response size.
// A single 20-topic batch is well under 256KB in practice; this
// guard prevents a runaway model from blowing up memory.
const maxAIResultBytes = 256 * 1024

// maxCount hard-caps how many topics one call may request. Keeps
// token usage bounded; a "give me 500" request would burn budget.
const maxCount = 20

// Generate is the library entry point. It validates input,
// builds the prompt, calls the LLM, parses the JSON array
// response, and returns topics + token counts.
func Generate(ctx context.Context, text *agents.MiniMax, input Input) (*Result, error) {
    if err := validate(input); err != nil {
        return nil, err
    }
    if text == nil {
        return nil, errors.New("topic: text provider is nil")
    }
    prompt := agents.GenerateTopicsPrompt(input.Seed, input.Platform, input.Count)
    res, err := text.Text(ctx, prompt, agents.MiniMaxTextOptions{Model: "MiniMax-M2.7-highspeed"})
    if err != nil {
        return nil, fmt.Errorf("topic: LLM call failed: %w", err)
    }
    topics, err := parseTopics(res.Text)
    if err != nil {
        return nil, fmt.Errorf("topic: parse failed: %w", err)
    }
    if len(topics) > input.Count {
        topics = topics[:input.Count]
    }
    return &Result{
        Topics: topics,
        Cost:   CostInfo{InputTokens: res.InputTokens, OutputTokens: res.OutputTokens},
    }, nil
}

// validate enforces the 4 input rules documented in spec §11.
// TrimSpace is applied so that whitespace-only seed/platform
// fail the empty check (avoids "    " slipping through).
func validate(input Input) error {
    if strings.TrimSpace(input.Seed) == "" {
        return errors.New("seed is required")
    }
    if strings.TrimSpace(input.Platform) == "" {
        return errors.New("platform is required")
    }
    if input.Count <= 0 {
        return errors.New("count must be > 0")
    }
    if input.Count > maxCount {
        return fmt.Errorf("count must be <= %d", maxCount)
    }
    return nil
}

// jsonArrayRE matches a top-level JSON array, allowing optional
// leading/trailing markdown fences (```json ... ```) and
// surrounding prose. The first non-greedy match is used; the
// caller validates that it parses.
var jsonArrayRE = regexp.MustCompile(`(?s)\[.*?\]`)

// parseTopics extracts []Topic from raw LLM output. Lenient about
// markdown fences because LLM often wraps JSON in ```json ... ```
// blocks despite explicit instructions.
func parseTopics(raw string) ([]Topic, error) {
    if len(raw) > maxAIResultBytes {
        return nil, errors.New("response too large")
    }
    cleaned := strings.TrimSpace(raw)
    // Strip a single ```json ... ``` wrapper if present.
    if strings.HasPrefix(cleaned, "```") {
        if i := strings.Index(cleaned, "\n"); i >= 0 {
            cleaned = cleaned[i+1:]
        }
        if strings.HasSuffix(cleaned, "```") {
            cleaned = cleaned[:len(cleaned)-3]
        }
        cleaned = strings.TrimSpace(cleaned)
    }
    match := jsonArrayRE.FindString(cleaned)
    if match == "" {
        return nil, errors.New("no JSON array found in response")
    }
    var topics []Topic
    if err := json.Unmarshal([]byte(match), &topics); err != nil {
        return nil, fmt.Errorf("invalid JSON: %w", err)
    }
    return topics, nil
}
```

- [ ] **Step 1.4: 跑 test 确认 PASS**

Run: `cd /root/workspace/opc/apps/api && go test -tags fts5 -v -count=1 ./internal/capabilities/topic/...`
Expected: 14 tests PASS

- [ ] **Step 1.5: 跑全 API tests 确认没破**

Run: `cd /root/workspace/opc/apps/api && go test -tags fts5 -count=1 ./...`
Expected: All packages PASS (only changes: new `capabilities/topic` package added; nothing else touched)

- [ ] **Step 1.6: 检查 coverage ≥ 80%**

Run: `cd /root/workspace/opc/apps/api && go test -tags fts5 -cover ./internal/capabilities/topic/...`
Expected: `coverage: 80.0% of statements` or higher. If below, add more fixtures to cover remaining branches.

- [ ] **Step 1.7: Commit**

```bash
cd /root/workspace/opc
git add apps/api/internal/capabilities/
git commit -m "feat(capabilities): topic library + 14 unit tests (Task 1)

New internal/capabilities/topic/ package with pure library:
- Topic struct (6 JSON fields, snake_case wire shape)
- Input struct (Seed + Platform + Count)
- Result struct (Topics + CostInfo)
- Generate(ctx, text, input) (*Result, error) entry point
- validate + parseTopics helpers (private)

Library is reused by:
- internal/handlers/ai.go:GenerateTopics (refactored in Task 2)
- cmd/opc-asset/main.go:runTopic (new in Task 3)

14 unit tests with agents.NewMiniMaxWithTextOverride mock LLM:
- 4 validate cases (empty seed / empty platform / count<=0 / count>20)
- 5 parseTopics cases (perfect / markdown fence / no array / too large / invalid JSON)
- 5 Generate cases (perfect / trims to count / LLM error / parse error / accepts fewer)

Coverage >= 80%. go vet / go build clean."
```

---

### Task 2: HTTP handler `GenerateTopics` 重构调 library

**Files:**
- Modify: `apps/api/internal/handlers/ai.go`
- (No test changes — existing 4 老 handler smoke (api-smoke.sh) covers regression)

- [ ] **Step 2.1: 读现有 handler 的 `GenerateTopics` + `parseTopics` 全文 (要 refactor 需 看清)**

Run: `cd /root/workspace/opc && sed -n '149,260p' apps/api/internal/handlers/ai.go`
Note the structure: bind → validate → demo check → real path (Text call + parseTopics + JSON response) + cost stamp

- [ ] **Step 2.2: 加 import `github.com/opc/api/internal/capabilities/topic` 到 `ai.go`**

In `apps/api/internal/handlers/ai.go` import block, add:
```go
"github.com/opc/api/internal/capabilities/topic"
```

- [ ] **Step 2.3: 改 `parseTopics` → `parseTopicsLegacy` (给 demo 路径用, 避免跟 library 内同函数冲突)**

Find: `func parseTopics(raw string) ([]generatedTopic, error) {`
Replace with: `func parseTopicsLegacy(raw string) ([]generatedTopic, error) {`

- [ ] **Step 2.4: 重构 `GenerateTopics` handler 调 `topic.Generate`**

**完整替换** `apps/api/internal/handlers/ai.go:149-235` (the GenerateTopics function body, from `func (h *AIHandler) GenerateTopics` through the closing `}`):

```go
func (h *AIHandler) GenerateTopics(c *gin.Context) {
    var req generateTopicsRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        h.logger.Warn("invalid ai body", "err", err.Error(), "request_id", c.GetString("request_id"))
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
        return
    }
    req.Seed = strings.TrimSpace(req.Seed)
    req.Platform = strings.TrimSpace(req.Platform)
    if req.Seed == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "seed is required"})
        return
    }
    if req.Platform == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "platform is required"})
        return
    }
    if req.Count <= 0 {
        req.Count = 5
    }
    if req.Count > 20 {
        // Hard cap to keep token usage bounded. The prompt does not
        // impose a limit and a runaway "give me 500" would burn budget.
        req.Count = 20
    }

    if h.shouldUseDemo(c) {
        // Demo path stays in the handler — it's an HTTP UX
        // concern (banner + canned pool), not a library concern.
        raw, _ := agents.DemoResponse(agents.DemoOpTopics, len(req.Seed))
        topics, err := parseTopicsLegacy(raw)
        if err != nil {
            h.logger.Error("ai demo parse failed", "err", err.Error(), "request_id", c.GetString("request_id"))
            c.JSON(http.StatusBadGateway, gin.H{
                "error": "AI demo response could not be parsed: " + err.Error(),
            })
            return
        }
        // Demo pool may have grown beyond req.Count (it is a marketing
        // pool, not a per-request fixture). Mirror the real path and
        // return only the first req.Count topics so the wire contract
        // is consistent regardless of which mode the server is in.
        if len(topics) > req.Count {
            topics = topics[:req.Count]
        }
        markDemoResponse(c)
        c.JSON(http.StatusOK, generateTopicsResponse{Topics: topics})
        return
    }

    // Real path: delegate to the capability library.
    ctx, cancel := context.WithTimeout(c.Request.Context(), aiTimeout)
    defer cancel()

    res, err := topic.Generate(ctx, h.text, topic.Input{
        Seed:     req.Seed,
        Platform: req.Platform,
        Count:    req.Count,
    })
    if err != nil {
        h.logger.Error("topic generate failed", "err", err.Error(), "request_id", c.GetString("request_id"))
        c.JSON(http.StatusServiceUnavailable, gin.H{
            "error": "AI service failed: " + err.Error(),
        })
        return
    }
    // Stamp cost onto the call_log row that the middleware created.
    // MiniMax M2.7-highspeed is the highspeed variant; cost
    // attribution uses the per-1k-token skill row keyed on
    // SkillMiniMaxM27.
    StampClaudeCost(c, config.SkillMiniMaxM27, res.Cost.InputTokens, res.Cost.OutputTokens)
    c.JSON(http.StatusOK, generateTopicsResponse{Topics: res.Topics})
}
```

- [ ] **Step 2.5: 跑 `go build` 确认编译通过**

Run: `cd /root/workspace/opc/apps/api && go build -tags fts5 ./...`
Expected: exit 0

- [ ] **Step 2.6: 跑全 API tests 确认 0 回归 (现有 handler 测试应继续 PASS)**

Run: `cd /root/workspace/opc/apps/api && go test -tags fts5 -count=1 ./...`
Expected: All PASS

- [ ] **Step 2.7: Commit**

```bash
cd /root/workspace/opc
git add apps/api/internal/handlers/ai.go
git commit -m "refactor(handlers): GenerateTopics delegates to topic library (Task 2)

The HTTP handler now calls topic.Generate for the real (non-demo)
path. Demo path stays in the handler (HTTP UX concern) and uses
the renamed parseTopicsLegacy to avoid name collision with the
library's parseTopics.

Net result: 1 business entry point (topic.Generate), 0 duplicate
prompt-building / LLM-calling / JSON-parsing code. HTTP wire
contract unchanged (G1-closed 2026-06-09 behavior preserved).

No test changes: existing api-smoke.sh covers the HTTP path
end-to-end. Task 4 will add a smoke case for this endpoint to
the existing handler smoke block."
```

---

### Task 3: opc-asset CLI 加 `topic` subcommand

**Files:**
- Modify: `apps/api/cmd/opc-asset/main.go`
- Modify: `apps/api/cmd/opc-asset/main_test.go` (加 `TestRunTopicMissingFlags`)

- [ ] **Step 3.1: 写 1 个 CLI test (TDD red)**

在 `apps/api/cmd/opc-asset/main_test.go` 末尾加:

```go
// TestRunTopicMissingFlags confirms runTopic returns an error
// when --seed/--platform/--out are missing. Does not depend
// on the LLM (no API key required).
func TestRunTopicMissingFlags(t *testing.T) {
    t.Parallel()

    cases := []struct {
        name string
        args []string
    }{
        {"no flags", []string{"topic"}},
        {"only seed", []string{"topic", "--seed", "x"}},
        {"seed and platform, no out", []string{"topic", "--seed", "x", "--platform", "抖音"}},
        {"seed and out, no platform", []string{"topic", "--seed", "x", "--out", "/tmp/x.json"}},
    }
    for _, tc := range cases {
        tc := tc
        t.Run(tc.name, func(t *testing.T) {
            t.Parallel()
            err := runTopic(tc.args, slog.Default())
            if err == nil {
                t.Fatalf("runTopic(%v) error = nil, want error", tc.args)
            }
        })
    }
}
```

- [ ] **Step 3.2: 跑 test 确认 FAIL (`runTopic` 不存在)**

Run: `cd /root/workspace/opc/apps/api && go test -tags fts5 -run TestRunTopicMissingFlags ./cmd/opc-asset/...`
Expected: FAIL (`runTopic` undefined)

- [ ] **Step 3.3: 在 `cmd/opc-asset/main.go` 加 `runTopic` 函数 + subcommand 路由**

在 `runCheck` 函数定义**之后** (但要在 `case "check":` 路由**之前**, 这样 all helpers cluster together) 加:

```go
// runTopic handles `opc-asset topic --seed X --platform Y --count N --out /tmp/x.json`.
// Calls the topic capability library, marshals the result to
// indented JSON, and writes to --out. Cost summary goes to stderr
// (M1 does not write to a ledger; that's a M2 decision).
func runTopic(args []string, logger *slog.Logger) error {
    fs := flag.NewFlagSet("topic", flag.ExitOnError)
    seed := fs.String("seed", "", "Topic seed phrase (required)")
    platform := fs.String("platform", "", "Target platform: 抖音/哔哩哔哩/小红书 (required)")
    count := fs.Int("count", 5, "Number of topics to generate (1-20)")
    out := fs.String("out", "", "Output JSON file path (required)")
    if err := fs.Parse(args); err != nil {
        return err
    }
    if *seed == "" || *platform == "" || *out == "" {
        fs.Usage()
        return errors.New("--seed, --platform, --out are required")
    }
    if *count <= 0 || *count > 20 {
        return fmt.Errorf("--count must be 1-20, got %d", *count)
    }

    // Reuse the same env var as the API server. We construct a
    // fresh minimax client per CLI invocation (cheap; ~ms to init).
    text := agents.NewMiniMax()
    if !text.Available() {
        return errors.New("minimax: ANTHROPIC_API_KEY env var not set")
    }

    ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
    defer cancel()

    res, err := topic.Generate(ctx, text, topic.Input{
        Seed:     *seed,
        Platform: *platform,
        Count:    *count,
    })
    if err != nil {
        return fmt.Errorf("generate failed: %w", err)
    }

    blob, err := json.MarshalIndent(res.Topics, "", "  ")
    if err != nil {
        return fmt.Errorf("marshal: %w", err)
    }
    if err := os.WriteFile(*out, blob, 0644); err != nil {
        return fmt.Errorf("write %s: %w", *out, err)
    }

    fmt.Fprintf(os.Stderr, "Wrote %d topics to %s (cost: %d in / %d out tokens)\n",
        len(res.Topics), *out, res.Cost.InputTokens, res.Cost.OutputTokens)
    return nil
}
```

在 `main()` switch 段 (`case "check":` 旁边) 加:
```go
case "topic":
    err = runTopic(os.Args[2:], logger)
```

在 import 块加:
```go
"context"      // may already be imported
"encoding/json"
"os"
"time"        // may already be imported
"errors"
"fmt"

"github.com/opc/api/internal/capabilities/topic"
```

(只加 缺的那 几个, 已有 跳过)

- [ ] **Step 3.4: 跑 TestRunTopicMissingFlags 确认 PASS**

Run: `cd /root/workspace/opc/apps/api && go test -tags fts5 -run TestRunTopicMissingFlags ./cmd/opc-asset/...`
Expected: PASS for all 4 subtests

- [ ] **Step 3.5: 跑全 assetgen + opc-asset tests 确认 0 回归**

Run: `cd /root/workspace/opc/apps/api && go test -tags fts5 ./internal/assetgen/... ./internal/capabilities/... ./cmd/opc-asset/...`
Expected: All PASS

- [ ] **Step 3.6: 跑 build 确认 binary 仍可编译**

Run: `cd /root/workspace/opc/apps/api && go build -tags fts5 -o /tmp/v-task3-asset ./cmd/opc-asset`
Expected: exit 0, binary at /tmp/v-task3-asset

- [ ] **Step 3.7: 跑 1 个 smoke: missing flag (确认 binary 拒绝)**

Run: `/tmp/v-task3-asset topic 2>&1 | head -3; echo "exit=$?"`
Expected: usage text + "exit=2" (stdlib flag.ExitOnError exits with 2 on Usage error)

- [ ] **Step 3.8: Commit**

```bash
cd /root/workspace/opc
git add cmd/opc-asset/
git commit -m "feat(opc-asset): topic subcommand (Task 3)

\`opc-asset topic --seed X --platform Y --count N --out /tmp/x.json\`
generates N topics via the topic capability library and writes
them as a JSON array to --out. Cost summary goes to stderr
(M1: no ledger write — that's M2).

New test TestRunTopicMissingFlags covers the 4 missing-flag
scenarios (no flags / only seed / seed+platform no out /
seed+out no platform). All 4 subtests PASS.

Manual smoke: \`/tmp/v-task3-asset topic\` (no flags) → usage text
+ exit 2 (stdlib flag.ExitOnError default). Requires
ANTHROPIC_API_KEY env var for actual generation."
```

---

### Task 4: 集成测试 + 文档 + 最终 verify + 状态文件更新

**Files:**
- Modify: `scripts/api-smoke.sh`
- Modify: `~/.claude/PROJECT-STATE.md`
- Modify: `~/.claude/GAP-LOG.md`

- [ ] **Step 4.1: 读现有 api-smoke.sh 找合适位置加 topic case**

Run: `cd /root/workspace/opc && wc -l scripts/api-smoke.sh; tail -50 scripts/api-smoke.sh`
Note the existing 10 test cases (5 handler + 4 IP type + 1 advisory). Add 2 more (1 HTTP topic + 1 CLI topic).

- [ ] **Step 4.2: 在 api-smoke.sh 末尾加 2 个 topic case**

Append:

```bash
# === Sub-Spec B M1: topic capability (HTTP + CLI) ===
section "topic capability"

# HTTP topic case (uses existing session from earlier register/login)
out=$(curl -s -X POST http://localhost:8081/api/ai/topics \
    -H "Content-Type: application/json" \
    -H "Cookie: opc_session=$SESSION_TOKEN" \
    -d '{"seed":"个人成长","platform":"抖音","count":5}')
topic_count=$(echo "$out" | jq '.topics | length' 2>/dev/null)
if [ "$topic_count" = "5" ]; then
    pass "topic HTTP: 5 topics returned"
else
    fail "topic HTTP: got '$topic_count' topics, want 5 (raw: ${out:0:200})"
fi

# CLI topic case (requires real ANTHROPIC_API_KEY in env; skip gracefully if missing)
CLI_OUT=$(mktemp)
if [ -z "$ANTHROPIC_API_KEY" ]; then
    skip "topic CLI: ANTHROPIC_API_KEY not set, skipping"
else
    if /root/workspace/opc/apps/bin/opc-asset topic \
        --seed "个人成长" --platform "抖音" --count 5 --out "$CLI_OUT" 2>/dev/null; then
        cli_count=$(jq length < "$CLI_OUT" 2>/dev/null)
        if [ "$cli_count" = "5" ]; then
            pass "topic CLI: 5 topics written to $CLI_OUT"
        else
            fail "topic CLI: jq length = $cli_count, want 5"
        fi
    else
        fail "topic CLI: opc-asset topic exited non-zero"
    fi
fi
rm -f "$CLI_OUT"
```

- [ ] **Step 4.3: 跑 smoke 验证**

```bash
cd /root/workspace/opc
make -C apps/api asset 2>&1 | tail -2
./scripts/api-smoke.sh 2>&1 | tail -25
```

Expected: All 12 cases PASS (10 existing + 2 new). Topic HTTP: 5 topics. Topic CLI: 5 topics (if ANTHROPIC_API_KEY set) or SKIP.

- [ ] **Step 4.4: 改 `~/.claude/PROJECT-STATE.md`**

```bash
# 1. Update Last refreshed line
# 2. Section E append M1 closure entry (6 commit summary)
# 3. Section D: mark G14 (sub-spec B M1) CLOSED
```

具体 edit (类似 A 的 M1 closure 处理):
- Header `Last refreshed: 2026-MM-DD ~HH:MM, after verify agent run 22 (Phase 2 sub-spec B M1 closure) returned OVERALL PASS.`
- Section E append 1 个 bullet (commit 链 + coverage + 4 plan deviations 文档)
- Section D add `G14-closed` row

- [ ] **Step 4.5: 改 `~/.claude/GAP-LOG.md`**

Append G14 closure entry:
```markdown
## YYYY-MM-DD HH:MM (objective, observed) — G14: Phase 2 sub-spec B M1 not delivered

- **缺口**: spec + plan 写了但没实现. topic library + opc-asset topic subcommand + handler refactor 都没落地.
- **影响**: Phase 2 C (对外 web UI) 依赖 B.
- **修复计划**: 见 docs/superpowers/plans/2026-06-12-opc-phase2-capabilities-m1.md (5 task, 2-3 周)
- **status**: **closed YYYY-MM-DD** — Task 1-4 全部 done, 14 unit test + 4 CLI test PASS, parent + topic coverage >= 80%, 4 commit landed, smoke 12/12 (含 topic HTTP + topic CLI case)
```

- [ ] **Step 4.6: Dispatch verify-project-state subagent**

按 `~/.claude/agents/verify-project-state.md` 规范 dispatch. 关键命令:
```bash
go vet -tags fts5 ./...
go test -tags fts5 -race -count=1 ./...
go test -tags fts5 -cover ./internal/capabilities/topic/...  # >= 80%
go build -tags fts5 -o /tmp/v-task4-api ./cmd/server
go build -tags fts5 -o /tmp/v-task4-asset ./cmd/opc-asset
# Topic HTTP smoke: curl POST /api/ai/topics 返 5 topics
# Topic CLI smoke: opc-asset topic --seed X --out /tmp/x.json 写 5 topics
```

报告写 `~/.claude/audit/verify-reports/verify-YYYYMMDD-HHMMSS-phase2-sub-spec-b-m1.md`. OVERALL: PASS 才算 M1 closed.

- [ ] **Step 4.7: Commit 收尾**

```bash
cd /root/workspace/opc
git add scripts/api-smoke.sh
git commit -m "test(smoke): add topic capability HTTP + CLI cases (Task 4)

scripts/api-smoke.sh: 2 new test cases for Sub-Spec B M1:
- Topic HTTP: POST /api/ai/topics returns 5 topics (regression
  for G1-closed 2026-06-09 behavior preserved by handler refactor)
- Topic CLI: opc-asset topic --seed X --out Y writes 5 topics
  (gracefully SKIP if ANTHROPIC_API_KEY env var not set; full
  run requires it)

Project state + gap log updated to reflect M1 closure (G14
CLOSED YYYY-MM-DD)."
```

---

## 验收清单（M1 done 的标志, 5 task 全部 DONE）

| # | 验收 | 验证 | 状态 |
|---|------|------|------|
| 1 | `topic.Generate(ctx, text, input) (*Result, error)` 函数存在 | `go doc ./internal/capabilities/topic` | ☐ |
| 2 | library 输入校验（4 条规则）+ parseTopics (5 边界) + Generate (5 case) 测试 PASS | `go test -tags fts5 ./internal/capabilities/topic/...` 14 PASS | ☐ |
| 3 | `capabilities/topic` package coverage ≥ 80% | `go test -tags fts5 -cover` ≥ 80% | ☐ |
| 4 | `opc-asset topic` subcommand 4 flag 正常 | `opc-asset topic --help` | ☐ |
| 5 | `opc-asset topic` 缺必填 flag 返非 0 退出 | `opc-asset topic` exit 2 + Usage | ☐ |
| 6 | `POST /api/ai/topics` 行为不变 (G1-closed 2026-06-09) | `scripts/api-smoke.sh` topic HTTP case PASS | ☐ |
| 7 | 4 个老 handler smoke 不回归 | `scripts/api-smoke.sh` 5+1 handler cases PASS | ☐ |
| 8 | verify-project-state agent OVERALL: PASS | `~/.claude/audit/verify-reports/verify-*.md` PASS | ☐ |
| 9 | PROJECT-STATE.md + GAP-LOG.md 更新 | G14 CLOSED 记录在案 | ☐ |

---

## 不做的事（M1 范围硬约束）

- ❌ Script / Deconstruct / Asset capability library (M2/M3)
- ❌ Topic profile-aware (M2)
- ❌ HTTP surface for script / deconstruct asset (已有不动)
- ❌ MCP tool 加 topic (M2)
- ❌ CLI batch / 并行 / streaming (M2)
- ❌ CLI cost ledger 写入 (M2 决定; M1 stderr 输出)
- ❌ Library demo 模式 (M1 仅 HTTP handler 保留 demo)
- ❌ Platform enum 强制 (抖音/哔哩哔哩/小红书) — Library 接受任何非空字符串
- ❌ 改 `internal/agents/{prompts,minimax}.go` — 复用, 不动
- ❌ Topic cap > 20 (跟 Phase 1 行为一致, 硬 cap)

---

## 风险与缓解

| 风险 | 缓解 |
|------|------|
| Library refactor 破 HTTP handler (G1-closed 2026-06-09 行为变化) | 老 smoke 必跑 (Task 4 Step 4.3); handler 路径 wrap error 同 demo 路径不冲突 |
| parseTopics 移到 library 后, demo 路径 (现有 handler 内部) 调老 parseTopics 需重命名 | 改名 `parseTopicsLegacy` 显式; 测试覆盖 demo 路径 |
| CLI 缺 `ANTHROPIC_API_KEY` 跑不出真实 topics, smoke 测不出 end-to-end | smoke 设计成"无 key 跳过" (Step 4.2); 真实 CI 用 secret 跑 |
| `*agents.MiniMax` 是强类型依赖, 未来切 DeepSeek/Gemini 难 | M1 沿用 A 模式; M2 abstract `*agents.TextProvider` interface |
| count > 20 hard cap | 跟现有 handler 一致; 拒绝是不让 token 烧爆 |
| `topic.Topic` vs `models.Topic` 同名 (不同 package) | caller 用 `topic.Topic` / `models.Topic` qualifier; 无 import 冲突 |
| opc-asset main_test.go 缺 `slog` import | 现有文件应已 import; 如果没有, Step 3.3 加 import |

---

## Self-Review

写 plan 时做了 4 轮 self-review:

1. **Spec coverage**: spec §5 (架构) / §6 (组件) / §7 (schema) / §10 (数据流) / §11 (错误处理) / §12 (测试) / §13 (timeline) / §15 (跟 Sub-Spec A 关系) 全部对应到 5 task. 验收清单 9 项覆盖.
2. **Placeholder scan**: 0 个 TBD/TODO/类似措辞.
3. **Type consistency**:
   - `Topic` struct 6 字段在 spec §7.2 定义, Task 1.3 实现完全一致
   - `Input` 3 字段 (Seed/Platform/Count) 一致
   - `Result{Topics, Cost}` + `CostInfo{InputTokens, OutputTokens}` 一致
   - `Generate(ctx, text, input) (*Result, error)` 签名 spec §5.2 → Task 1.3 → Task 3.3 (CLI) 一致
   - `parseTopics` 移到 library 后 handler 改 `parseTopicsLegacy` (Task 2.3), demo 路径用 (Task 2.4)
4. **Ambiguity check**: 
   - 验证错误 (count 21) 在 library 返, CLI 不二次校验 → 显式 (Step 3.3 注释)
   - count = 0 library 返错, CLI 也可返错 (Step 3.3 有 二次校验 as defense)
   - ANTHROPIC_API_KEY missing 时 CLI 返明确错 (Step 3.3) + smoke 跳过 (Step 4.2)

无矛盾. Plan ready for execution.
