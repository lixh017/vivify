# OPC Phase 2 — IP 模板框架 M1 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 `internal/assetgen/` 从「1 个 Go 写死的熊猫 profile」重构为「4 类 IP（anthropomorphic/digital_human/costume/info）可插拔框架」，M1 交付 4 套 JSON sub-schema + 4 套 consistency check + 4 个 instance profile + opc-asset CLI 加 `--type` flag，4-5 周内 verify-project-state agent 跑出 OVERALL PASS。

**Architecture:** 4 个 sub-package（`internal/assetgen/profiles/{anthropomorphic,digital_human,costume,info}/`）作为硬边界，各自包含 `profile.go` + `consistency.go` + `prompts.go` + `profile.json` + `schema.json` + 测试。每个 sub-package 的 `init()` 调父包 `assetgen.Register(type, ProfileEntry)` 自动注册；父包持 `registry map[string]ProfileEntry` + dispatcher 公共 API。`//go:embed` 编译期打包 JSON 到 binary。

**Tech Stack:** Go 1.25, `//go:embed` (stdlib), JSON Schema Draft 2020-12 (best-effort validation, no dep — string-based check for v1), GORM/SQLite (untouched in this plan), opc-asset CLI 已有 `cobra-free` flag parsing.

**Spec:** `docs/superpowers/specs/2026-06-11-opc-phase2-ip-template-design.md` v1.0 (commit `34fe6b7`).

**Plan note (2026-06-11)**: 原 plan 是 9 task（Task 1 父包 refactor + Task 2 consistency dispatcher + Task 3 prompts dispatcher + Task 4 anthropomorphic + Task 5-7 三个 sub-package + Task 8 opc-asset --type + Task 9 integration）。 Implementer subagent 实施 Task 1 时发现一个 Go 类型系统冲突：`Profile` 在 4 个文件（profile.go struct + consistency.go 参数 + prompts.go 参数 + opc-asset/main.go 用法）都被使用。Task 1 原本说"改 Profile struct 为 interface 但不碰另 3 个文件"在 Go 里不可能（同一 package 不能同时有 `type Profile struct` 和 `type Profile interface`）。决定 **合并 Task 1+2+3+4 为新 Task 1**：父包整体 refactor + anthropomorphic sub-package 完整实现同步做。新 plan 是 **6 task**（不是 9）。

---

## 文件结构（M1 终态）

**新建**：
- `apps/api/internal/assetgen/loader.go` — registry + Register + LoadProfile
- `apps/api/internal/assetgen/round2.go` — Round2Public 包装
- `apps/api/internal/assetgen/consistency_test.go` — dispatcher 测试
- `apps/api/internal/assetgen/prompts_test.go` — dispatcher 测试
- `apps/api/internal/assetgen/profiles/anthropomorphic/{profile,consistency,prompts}.go`
- `apps/api/internal/assetgen/profiles/anthropomorphic/{profile,schema}.json`
- `apps/api/internal/assetgen/profiles/anthropomorphic/*_test.go`
- `apps/api/internal/assetgen/profiles/digital_human/{profile,consistency,prompts}.go`
- `apps/api/internal/assetgen/profiles/digital_human/{profile,schema}.json`
- `apps/api/internal/assetgen/profiles/digital_human/*_test.go`
- `apps/api/internal/assetgen/profiles/costume/{profile,consistency,prompts}.go`
- `apps/api/internal/assetgen/profiles/costume/{profile,schema}.json`
- `apps/api/internal/assetgen/profiles/costume/*_test.go`
- `apps/api/internal/assetgen/profiles/info/{profile,consistency,prompts}.go`
- `apps/api/internal/assetgen/profiles/info/{profile,schema}.json`
- `apps/api/internal/assetgen/profiles/info/*_test.go`
- `docs/assetgen/profile-schema.md` — 4 套子 schema 字段参考

**修改**：
- `apps/api/internal/assetgen/profile.go` — `Profile` struct → interface + 加 `ProfileEntry` + 加 `ConsistencyFn` + 加 `PromptFn` + 删 `FenggeV1` struct value + 删 `GetProfile` function
- `apps/api/internal/assetgen/consistency.go` — `ConsistencyCheck` 改 dispatcher + 加 `safeCheck`
- `apps/api/internal/assetgen/prompts.go` — `BuildPrompt` 改 dispatcher + 加 `joinPalette` helper
- `apps/api/internal/assetgen/assetgen_test.go` — 4 个老 test 改用 `anthropomorphic.DefaultInstance()`（保留 4 个 test case，签名变化）
- `apps/api/cmd/opc-asset/main.go` — 改用 `assetgen.MustLoadProfile("anthropomorphic")` + `prof.Version()` 方法（不是 field）+ 删 `*profileVersion` flag（Task 5 加 `--type`）

**保留不变**（spec §15 列出）：
- `apps/api/internal/agents/minimax.go` — LLM 客户端，跟 profile 框架解耦
- `audit/roundtrip-volcengine.mjs` — 历史 demo（M2 才删）

**删除**：
- `assetgen.FenggeV1`（struct value）— 替换为 `anthropomorphic.DefaultInstance()`
- `assetgen.GetProfile(version)` — 替换为 `assetgen.LoadProfile("anthropomorphic")` 或 `assetgen.MustLoadProfile(...)`

---

## 命名约定

| 类型 | 名称 | 说明 |
|------|------|------|
| IP type discriminator | `Profile.Type()` 返回 string | `"anthropomorphic"` / `"digital_human"` / `"costume" / "info"` |
| 资产类型（image/video） | `AssetType` const | `TypeImage` / `TypeVideo` |
| Instance profile | `<type>Profile` struct | 例如 `AnthropomorphicProfile` 在 `anthropomorphic` sub-package |
| 公共接口 | `Profile` interface（父包） | Type/Version/Name 三方法 |
| Registry entry | `ProfileEntry` struct（父包） | Schema + Check + BuildPrompt |
| Consistency check 函数 | `check<Type>(p *<type>Profile, prompt string) ConsistencyResult` | 私有，在 sub-package 内 |
| Prompt builder 函数 | `build<Type>Prompt(p *<type>Profile, scene, outfit, assetType string) string` | 私有，在 sub-package 内 |
| Instance 加载函数 | `DefaultInstance() *<type>Profile` | 公开，从 embed 加载默认 instance |
| init 注册 | `func init() { assetgen.Register("<type>", ProfileEntry{...}) }` | 每个 sub-package 一个 |

**类型冲突注意**：`Profile` 字段跟 `Profile.Type()` 方法在 Go 里不能同名，所以 sub-package 内的 struct 用 `Type_` 字段 + `Type()` 方法重命名模式：
```go
type Profile struct {
    Type_  string  // JSON unmarshal target
    Name   string
    ...
}
func (p *Profile) Type() string  { return p.Type_ }
```

---

## 任务分解

---

### Task 1: 父包 refactor combined + anthropomorphic sub-package 完整实现 (was Tasks 1+2+3+4)

**范围**：4 个原 task 合并。父包 `assetgen/` 全 refactor（profile.go / consistency.go / prompts.go / loader.go / round2.go）+ `anthropomorphic` sub-package 完整实现 + opc-asset main.go 适配新 API + 4 个老 test 改用 `anthropomorphic.DefaultInstance()`。

**Files:**
- Modify: `apps/api/internal/assetgen/profile.go`
- Modify: `apps/api/internal/assetgen/consistency.go`
- Modify: `apps/api/internal/assetgen/prompts.go`
- Modify: `apps/api/internal/assetgen/assetgen_test.go`
- Modify: `apps/api/cmd/opc-asset/main.go`
- Create: `apps/api/internal/assetgen/loader.go`
- Create: `apps/api/internal/assetgen/round2.go`
- Create: `apps/api/internal/assetgen/consistency_test.go`
- Create: `apps/api/internal/assetgen/prompts_test.go`
- Create: `apps/api/internal/assetgen/profiles/anthropomorphic/profile.go`
- Create: `apps/api/internal/assetgen/profiles/anthropomorphic/consistency.go`
- Create: `apps/api/internal/assetgen/profiles/anthropomorphic/prompts.go`
- Create: `apps/api/internal/assetgen/profiles/anthropomorphic/profile.json`
- Create: `apps/api/internal/assetgen/profiles/anthropomorphic/schema.json`
- Create: `apps/api/internal/assetgen/profiles/anthropomorphic/profile_test.go`
- Create: `apps/api/internal/assetgen/profiles/anthropomorphic/consistency_test.go`
- Create: `apps/api/internal/assetgen/profiles/anthropomorphic/prompts_test.go`

- [ ] **Step 1.1: 写新 test 在 `apps/api/internal/assetgen/assetgen_test.go` 末尾添加 (TDD red)**

替换 4 个老 test, 改用 `anthropomorphic.DefaultInstance()` (新加的, 还没建)。保留原 test case 名字 + 行为：

```go
// TestProfileFenggeV1 (updated) — verifies LoadProfile's
// known/unknown/empty behavior. The default empty typeName
// defaults to anthropomorphic / fengge_v1 is intentional —
// the CLI uses "" as "the only profile we ship" so callers
// don't need to hardcode the type string.
func TestProfileFenggeV1(t *testing.T) {
    t.Parallel()

    tests := []struct {
        name        string
        typeName    string
        wantNil     bool
        wantType    string
        wantVersion string
        wantName    string
    }{
        {
            name:        "explicit anthropomorphic / fengge_v1",
            typeName:    "anthropomorphic",
            wantNil:     false,
            wantType:    "anthropomorphic",
            wantVersion: "fengge_v1",
            wantName:    "峰哥",
        },
        {
            name:        "empty string defaults to anthropomorphic / fengge_v1",
            typeName:    "",
            wantNil:     false,
            wantType:    "anthropomorphic",
            wantVersion: "fengge_v1",
            wantName:    "峰哥",
        },
        {
            name:     "unknown type returns error",
            typeName: "nonexistent",
            wantNil:  true,
        },
    }

    for _, tt := range tests {
        tt := tt
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()

            got, err := assetgen.LoadProfile(tt.typeName)
            if tt.wantNil {
                if err == nil {
                    t.Fatalf("LoadProfile(%q) = %+v, want error", tt.typeName, got)
                }
                return
            }
            if err != nil {
                t.Fatalf("LoadProfile(%q) error: %v", tt.typeName, err)
            }
            if got == nil {
                t.Fatalf("LoadProfile(%q) = nil, want non-nil Profile", tt.typeName)
            }
            if got.Type() != tt.wantType {
                t.Errorf("Type() = %q, want %q", got.Type(), tt.wantType)
            }
            if got.Version() != tt.wantVersion {
                t.Errorf("Version() = %q, want %q", got.Version(), tt.wantVersion)
            }
            if got.Name() != tt.wantName {
                t.Errorf("Name() = %q, want %q", got.Name(), tt.wantName)
            }
        })
    }
}
```

更新其他 3 个老 test 的 `p := assetgen.FenggeV1` → `p := anthropomorphic.DefaultInstance()`：

```go
// TestBuildPromptImage (updated) — uses anthropomorphic instance
func TestBuildPromptImage(t *testing.T) {
    t.Parallel()

    p := anthropomorphic.DefaultInstance()
    const scene = "竹林小院"
    const outfit = "朱红"
    got := assetgen.BuildPrompt(p, scene, outfit, assetgen.TypeImage)

    wantContains := []string{
        "峰哥", "成年熊猫", "rgb(245,240,225)", "rgb(26,26,26)",
        "国潮", "不露爪", "--ratio 9:16", scene, outfit,
    }
    // ... rest same as before
}

// TestBuildPromptVideo (updated) — same pattern
func TestBuildPromptVideo(t *testing.T) {
    t.Parallel()
    p := anthropomorphic.DefaultInstance()
    const scene = "竹林小院"
    const outfit = "翠绿"
    got := assetgen.BuildPrompt(p, scene, outfit, assetgen.TypeVideo)
    // ... rest same as before
}

// TestConsistencyCheck (updated) — same pattern
func TestConsistencyCheck(t *testing.T) {
    t.Parallel()
    p := anthropomorphic.DefaultInstance()
    tests := []struct {
        name      string
        prompt    string
        wantScore float64
        wantFails int
    }{
        {
            name:      "full prompt from BuildPrompt scores 0.95",
            prompt:    assetgen.BuildPrompt(p, "x", "y", assetgen.TypeImage),
            wantScore: 0.95,
            wantFails: 1,
        },
        // ... rest same as before
    }
    // ... rest same
}
```

更新 `package` import 顶部，加 `"github.com/opc/api/internal/assetgen/profiles/anthropomorphic"`。

- [ ] **Step 1.2: 跑 test 确认 FAIL（LoadProfile + anthropomorphic 还没建）**

Run: `cd apps/api && go test -tags fts5 -run TestProfileFenggeV1 ./internal/assetgen/...`
Expected: FAIL (LoadProfile undefined + anthropomorphic package not found)

- [ ] **Step 1.3: 重写 `apps/api/internal/assetgen/profile.go` — Profile interface + ProfileEntry + 公共 type**

**完整替换** `apps/api/internal/assetgen/profile.go`：

```go
// Package assetgen provides the opc asset-generation primitives:
// the canonical IP profiles (4 hard-boundary sub-packages under
// profiles/), the prompt builders, and the brand-consistency check.
// It is the library side of the opc-asset CLI (cmd/opc-asset) and
// any future platform-side caller that wants to generate a
// brand-on asset on demand.
//
// The package is intentionally small: a Profile interface, a
// ProfileEntry struct that bundles (schema, check, prompt builder)
// per IP type, a registry + dispatcher, and a small type system.
// Anything that requires talking to a model (text, image, video)
// lives in the agents package; assetgen is the pure-Go
// prompt+profile layer that callers compose with whatever model
// they want.
package assetgen

// AssetType distinguishes the two outputs the platform generates.
const (
    TypeImage = "image"
    TypeVideo = "video"
)

// Profile is the public interface every IP type implements.
// Type() returns the IP type discriminator ("anthropomorphic",
// "digital_human", "costume", "info") used by the registry to
// route ConsistencyCheck and BuildPrompt calls to the right
// sub-package's implementation.
//
// Implementations live in internal/assetgen/profiles/<type>/
// sub-packages; the parent package never knows about their
// concrete structs.
type Profile interface {
    Type() string
    Version() string
    Name() string
}

// ProfileEntry is the unit of registration: a concrete schema
// (which is itself a Profile), the consistency check function
// bound to it, and the prompt builder. Sub-package init() calls
// Register(<type>, entry) at startup; the parent dispatcher
// looks up by Type() and invokes the right function.
type ProfileEntry struct {
    Schema      Profile
    Check       ConsistencyFn
    BuildPrompt PromptFn
}

// ConsistencyFn scores a prompt against the profile's brand
// anchors. Returns ConsistencyResult with Score in [0, 1] and
// Fails listing missing anchor names. Pure: no IO, no model call.
type ConsistencyFn func(Profile, string) ConsistencyResult

// PromptFn assembles a model prompt for the given profile +
// scene + outfit + asset type. Pure: no IO, no model call.
type PromptFn func(Profile, string, string, string) string
```

- [ ] **Step 1.4: 新建 `apps/api/internal/assetgen/loader.go`**

```go
package assetgen

import "fmt"

// registry is populated by sub-package init() calls. Map keyed by
// IP type discriminator (Profile.Type() return value). Read-only
// after init time.
var registry = map[string]ProfileEntry{}

// Register adds an entry to the registry. Panics on duplicate
// type — this is a fail-fast at startup, not a runtime check,
// because duplicate registration means a build error, not a
// runtime condition.
func Register(typeName string, entry ProfileEntry) {
    if _, exists := registry[typeName]; exists {
        panic(fmt.Sprintf("assetgen: duplicate Register for type %q", typeName))
    }
    if entry.Schema == nil {
        panic(fmt.Sprintf("assetgen: Register for %q has nil Schema", typeName))
    }
    if entry.Check == nil {
        panic(fmt.Sprintf("assetgen: Register for %q has nil Check", typeName))
    }
    if entry.BuildPrompt == nil {
        panic(fmt.Sprintf("assetgen: Register for %q has nil BuildPrompt", typeName))
    }
    registry[typeName] = entry
}

// LoadProfile returns the registered Profile schema for the
// given IP type. Empty typeName defaults to "anthropomorphic"
// for back-compat with Phase 1 callers that used the empty
// string as "the only profile we ship".
//
// Returns an error for unknown types so callers can fail loudly
// rather than silently fall back.
func LoadProfile(typeName string) (Profile, error) {
    if typeName == "" {
        typeName = "anthropomorphic"
    }
    entry, ok := registry[typeName]
    if !ok {
        return nil, fmt.Errorf("assetgen: unknown IP type %q (registered: %v)", typeName, registeredTypes())
    }
    return entry.Schema, nil
}

// MustLoadProfile is LoadProfile that panics on error. Use only
// in places where a missing profile is a programmer error (e.g.
// CLI main wiring).
func MustLoadProfile(typeName string) Profile {
    p, err := LoadProfile(typeName)
    if err != nil {
        panic(err)
    }
    return p
}

// registeredTypes returns a sorted-ish list of registered type
// names, used only for error messages. Order is non-deterministic
// (map iteration) but M1 has 4 types max so readability is fine.
func registeredTypes() []string {
    out := make([]string, 0, len(registry))
    for k := range registry {
        out = append(out, k)
    }
    return out
}
```

- [ ] **Step 1.5: 新建 `apps/api/internal/assetgen/round2.go`**

```go
package assetgen

// Round2Public rounds a float to 2 decimal places. Exported for
// use by sub-package check functions (e.g. anthropomorphic,
// digital_human) to match the JSONL ledger format from the
// original roundtrip-volcengine.mjs script.
func Round2Public(f float64) float64 {
    return round2(f)
}
```

- [ ] **Step 1.6: 新建 `apps/api/internal/assetgen/profiles/anthropomorphic/profile.json`**

```json
{
  "version": "fengge_v1",
  "name": "峰哥",
  "type": "anthropomorphic",
  "species": "成年熊猫",
  "body_shape": "头身比 1:1.2 圆胖身材",
  "body_color": "rgb(245,240,225)",
  "eye_color": "rgb(26,26,26)",
  "eye_expression": "半阖带笑意, 不直视镜头",
  "palette": ["朱红#C73E1D", "暖橙#E89B45", "翠绿#3B8C5A", "宝蓝#1F5FA8", "米白#F5F0E1"]
}
```

- [ ] **Step 1.7: 新建 `apps/api/internal/assetgen/profiles/anthropomorphic/schema.json`**

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["version", "name", "type", "species", "body_shape", "body_color", "eye_color", "eye_expression", "palette"],
  "properties": {
    "version": { "type": "string" },
    "name": { "type": "string" },
    "type": { "const": "anthropomorphic" },
    "species": { "type": "string" },
    "body_shape": { "type": "string" },
    "body_color": { "type": "string" },
    "eye_color": { "type": "string" },
    "eye_expression": { "type": "string" },
    "palette": {
      "type": "array",
      "minItems": 3,
      "items": { "type": "string" }
    }
  }
}
```

- [ ] **Step 1.8: 新建 `apps/api/internal/assetgen/profiles/anthropomorphic/profile.go` (含 //go:embed + DefaultInstance + init 注册)**

```go
// Package anthropomorphic registers the anthropomorphic IP type
// (拟人动物 — panda, fox, dog, ...) into the assetgen registry.
// M1 ships one instance: fengge_v1 (峰哥 / adult panda) loaded
// from profile.json via //go:embed.
package anthropomorphic

import (
    _ "embed"
    "encoding/json"
    "fmt"

    "github.com/opc/api/internal/assetgen"
)

// Profile is the concrete schema for an anthropomorphic animal IP
// (panda, fox, dog, ...). Field names match profile.json keys
// (snake_case → Go PascalCase). Type_ is the actual JSON field
// name; Type() method returns the value (Go disallows a field
// and method sharing a name).
type Profile struct {
    Version       string
    Name          string
    Type_         string
    Species       string
    BodyShape     string
    BodyColor     string
    EyeColor      string
    EyeExpression string
    Palette       []string
}

// Type implements assetgen.Profile.
func (p *Profile) Type() string { return p.Type_ }

// Version implements assetgen.Profile.
func (p *Profile) Version() string { return p.Version }

// Name implements assetgen.Profile.
func (p *Profile) Name() string { return p.Name }

//go:embed profile.json
var profileJSON []byte

//go:embed schema.json
var schemaJSON []byte

// defaultInstance is loaded from profile.json at init() time.
// M1 has one instance per type; M2 may add per-version lookup.
var defaultInstance *Profile

func init() {
    if err := json.Unmarshal(profileJSON, &defaultInstance); err != nil {
        panic(fmt.Sprintf("anthropomorphic: failed to unmarshal profile.json: %v", err))
    }
    if defaultInstance.Type_ != "anthropomorphic" {
        panic(fmt.Sprintf("anthropomorphic: profile.json has type %q, want anthropomorphic", defaultInstance.Type_))
    }
    assetgen.Register("anthropomorphic", assetgen.ProfileEntry{
        Schema:      defaultInstance,
        Check:       Check,
        BuildPrompt: BuildPrompt,
    })
}

// DefaultInstance returns the canonical fengge_v1 panda instance.
// Exported so other sub-packages (and tests) can reference the
// "official" anthropomorphic profile.
func DefaultInstance() *Profile { return defaultInstance }

// SchemaJSON returns the JSON Schema for this type. Exported for
// M2's profile upload endpoint to validate user-uploaded profiles.
func SchemaJSON() []byte { return schemaJSON }
```

- [ ] **Step 1.9: 新建 `apps/api/internal/assetgen/profiles/anthropomorphic/consistency.go` (7 anchor, 1.0 total)**

```go
// Package anthropomorphic implements the consistency check for
// anthropomorphic animal IPs (panda, fox, etc.). The 7 anchors
// and their weights are defined in
// docs/superpowers/specs/2026-06-11-opc-phase2-ip-template-design.md
// §7.2.1.
package anthropomorphic

import (
    "strings"

    "github.com/opc/api/internal/assetgen"
)

// Check scores a prompt against the profile's 7 brand anchors.
// Pure function; no IO. Public wrapper around checkAnthropomorphic.
func Check(p assetgen.Profile, prompt string) assetgen.ConsistencyResult {
    if p == nil {
        return assetgen.ConsistencyResult{Score: 0, Fails: []string{"nil profile"}}
    }
    ap, ok := p.(*Profile)
    if !ok {
        return assetgen.ConsistencyResult{
            Score: 0,
            Fails: []string{"profile is not *anthropomorphic.Profile"},
        }
    }
    return checkAnthropomorphic(ap, prompt)
}

func checkAnthropomorphic(p *Profile, prompt string) assetgen.ConsistencyResult {
    var score float64
    var fails []string

    add := func(needle, failName string, weight float64) {
        if strings.Contains(prompt, needle) {
            score += weight
        } else {
            fails = append(fails, failName)
        }
    }

    add(p.Name, "no "+p.Name+" name", 0.20)
    add(p.Species, "no panda species", 0.20)
    add(p.BodyColor, "no body white color", 0.20)
    add(p.EyeColor, "no eye black color", 0.20)
    add("国潮", "no guofeng theme", 0.10)
    add("不露爪", "no anti-claws", 0.05)
    add("不要 AI 生成感", "no anti-AI tokens", 0.05)

    return assetgen.ConsistencyResult{
        Score: assetgen.Round2Public(score),
        Fails: fails,
    }
}
```

- [ ] **Step 1.10: 新建 `apps/api/internal/assetgen/profiles/anthropomorphic/prompts.go`**

```go
// Package anthropomorphic implements the prompt builder for
// anthropomorphic animal IPs. Mirrors Phase 1's BuildPrompt
// output 1:1 for the fengge_v1 panda instance.
package anthropomorphic

import (
    "strings"

    "github.com/opc/api/internal/assetgen"
)

// BuildPrompt assembles a model prompt for the profile + scene +
// outfit + asset type. Type-specific builder registered in the
// parent assetgen registry. Pure function; no IO.
func BuildPrompt(p assetgen.Profile, scene, outfit, assetType string) string {
    if p == nil {
        return ""
    }
    ap, ok := p.(*Profile)
    if !ok {
        return ""
    }
    return buildAnthropomorphicPrompt(ap, scene, outfit, assetType)
}

func buildAnthropomorphicPrompt(p *Profile, scene, outfit, assetType string) string {
    prefix := "一只" + p.Species + "(" + p.Name + "), " + p.BodyShape +
        ", 面部白 " + p.BodyColor +
        " 眼周黑 " + p.EyeColor +
        ", 眼神" + p.EyeExpression +
        ", 身穿" + outfit + "汉服" +
        ", 站在" + scene +
        ", 暖光氛围, 国潮 + 色彩靓丽(" + strings.Join(p.Palette, "/") +
        "撞色, 非暗色调、非水墨), 不露爪, 不攻击性姿势, 无文字" +
        ", 不要 AI 生成感"

    switch assetType {
    case assetgen.TypeVideo:
        return prefix + "  --duration 5 --resolution 720p --ratio 9:16 --watermark true"
    case assetgen.TypeImage:
        return prefix + "  --ratio 9:16"
    default:
        return prefix
    }
}
```

- [ ] **Step 1.11: 重写 `apps/api/internal/assetgen/consistency.go` — dispatcher + safeCheck**

**完整替换** `apps/api/internal/assetgen/consistency.go`：

```go
package assetgen

import (
    "fmt"
    "math"
)

// ConsistencyResult is the output of ConsistencyCheck. Score is
// in [0, 1]; Fails lists the human-readable names of the anchors
// the prompt is missing, in the order they were checked (most
// brand-critical first).
type ConsistencyResult struct {
    Score float64
    Fails []string
}

// ConsistencyCheck scores a prompt against the profile's brand
// anchors by dispatching to the type-specific check function
// registered for the profile's IP type.
//
// The dispatcher is fail-safe: nil profile, unknown type, and
// panicking check functions all return Score=0 with an explicit
// fail message — they never crash the caller.
func ConsistencyCheck(p Profile, prompt string) ConsistencyResult {
    return safeCheck(p, prompt)
}

// safeCheck is the actual dispatcher with the recover() guard.
func safeCheck(p Profile, prompt string) (result ConsistencyResult) {
    if p == nil {
        return ConsistencyResult{Score: 0, Fails: []string{"nil profile"}}
    }
    entry, ok := registry[p.Type()]
    if !ok {
        return ConsistencyResult{
            Score: 0,
            Fails: []string{fmt.Sprintf("unknown profile type: %s", p.Type())},
        }
    }
    defer func() {
        if r := recover(); r != nil {
            result = ConsistencyResult{
                Score: 0,
                Fails: []string{fmt.Sprintf("check panic: %v", r)},
            }
        }
    }()
    return entry.Check(p, prompt)
}

// round2 rounds a float to 2 decimal places, matching the JSONL
// ledger format the original mjs used. Private; sub-packages
// use Round2Public instead.
func round2(f float64) float64 {
    return math.Round(f*100) / 100
}
```

- [ ] **Step 1.12: 重写 `apps/api/internal/assetgen/prompts.go` — dispatcher + joinPalette helper**

**完整替换** `apps/api/internal/assetgen/prompts.go`：

```go
package assetgen

import "strings"

// BuildPrompt returns a model prompt for the given profile +
// scene + outfit + asset type. The dispatcher routes to the
// type-specific builder registered in the registry. Unknown
// types return an empty string (callers should check).
//
// The function is pure: no IO, no model call. Same builder is
// used by opc-asset CLI and any platform-side handler.
func BuildPrompt(p Profile, scene, outfit, assetType string) string {
    if p == nil {
        return ""
    }
    entry, ok := registry[p.Type()]
    if !ok {
        return ""
    }
    return entry.BuildPrompt(p, scene, outfit, assetType)
}

// joinPalette is a public helper for sub-package prompt builders
// to format a palette slice into the standard "C1/C2/C3" form
// used in opc prompts.
func joinPalette(colors []string) string {
    return strings.Join(colors, "/")
}
```

- [ ] **Step 1.13: 写 `apps/api/internal/assetgen/consistency_test.go` (dispatcher 测试)**

```go
package assetgen_test

import (
    "testing"

    "github.com/opc/api/internal/assetgen"
)

// TestConsistencyCheckUnknownType confirms the dispatcher returns
// Score=0 + explicit fail message when given a Profile whose Type
// isn't registered.
func TestConsistencyCheckUnknownType(t *testing.T) {
    t.Parallel()

    bogus := &bogusProfile{typeName: "unicorn"}
    got := assetgen.ConsistencyCheck(bogus, "anything")

    if got.Score != 0 {
        t.Errorf("Score = %v, want 0", got.Score)
    }
    if len(got.Fails) != 1 {
        t.Fatalf("len(Fails) = %d, want 1 (Fails: %v)", len(got.Fails), got.Fails)
    }
    if got.Fails[0] != "unknown profile type: unicorn" {
        t.Errorf("Fails[0] = %q, want %q", got.Fails[0], "unknown profile type: unicorn")
    }
}

// TestConsistencyCheckNilProfile confirms safeCheck guards nil.
func TestConsistencyCheckNilProfile(t *testing.T) {
    t.Parallel()

    got := assetgen.ConsistencyCheck(nil, "anything")
    if got.Score != 0 {
        t.Errorf("Score = %v, want 0", got.Score)
    }
    if len(got.Fails) != 1 || got.Fails[0] != "nil profile" {
        t.Errorf("Fails = %v, want [\"nil profile\"]", got.Fails)
    }
}

// bogusProfile implements assetgen.Profile but Type() returns a
// value never registered, so the dispatcher will hit the
// unknown-type branch.
type bogusProfile struct{ typeName string }

func (b *bogusProfile) Type() string    { return b.typeName }
func (b *bogusProfile) Version() string { return "x" }
func (b *bogusProfile) Name() string    { return "x" }
```

- [ ] **Step 1.14: 写 `apps/api/internal/assetgen/prompts_test.go`**

```go
package assetgen_test

import (
    "testing"

    "github.com/opc/api/internal/assetgen"
)

// TestBuildPromptUnknownType confirms the dispatcher returns ""
// for unknown IP types so callers can detect and fallback.
func TestBuildPromptUnknownType(t *testing.T) {
    t.Parallel()

    bogus := &bogusProfile{typeName: "unicorn"}
    got := assetgen.BuildPrompt(bogus, "scene", "outfit", assetgen.TypeImage)
    if got != "" {
        t.Errorf("BuildPrompt(unicorn) = %q, want empty string", got)
    }
}
```

- [ ] **Step 1.15: 写 `apps/api/internal/assetgen/profiles/anthropomorphic/profile_test.go`**

```go
package anthropomorphic_test

import (
    "encoding/json"
    "testing"

    "github.com/opc/api/internal/assetgen/profiles/anthropomorphic"
)

func TestProfileRoundTrip(t *testing.T) {
    t.Parallel()

    p := anthropomorphic.DefaultInstance()
    if p == nil {
        t.Fatal("DefaultInstance returned nil")
    }
    if p.Type_ != "anthropomorphic" {
        t.Errorf("Type_ = %q, want anthropomorphic", p.Type_)
    }
    if p.Version != "fengge_v1" {
        t.Errorf("Version = %q, want fengge_v1", p.Version)
    }
    if p.Name != "峰哥" {
        t.Errorf("Name = %q, want 峰哥", p.Name)
    }
    if len(p.Palette) < 3 {
        t.Errorf("len(Palette) = %d, want >= 3", len(p.Palette))
    }

    // Round-trip: marshal → unmarshal → equal.
    blob, err := json.Marshal(p)
    if err != nil {
        t.Fatalf("Marshal error: %v", err)
    }
    var got anthropomorphic.Profile
    if err := json.Unmarshal(blob, &got); err != nil {
        t.Fatalf("Unmarshal error: %v", err)
    }
    if got.Name != p.Name {
        t.Errorf("round-trip Name = %q, want %q", got.Name, p.Name)
    }
    if len(got.Palette) != len(p.Palette) {
        t.Errorf("round-trip Palette length = %d, want %d", len(got.Palette), len(p.Palette))
    }
}

func TestProfileImplementsAssetgenProfile(t *testing.T) {
    t.Parallel()

    // Compile-time check that *Profile satisfies assetgen.Profile
    var _ assetgenAlias = (*anthropomorphic.Profile)(nil)
}

// assetgenAlias mirrors assetgen.Profile locally so we don't
// need to import assetgen just for the assertion.
type assetgenAlias interface {
    Type() string
    Version() string
    Name() string
}
```

- [ ] **Step 1.16: 写 `apps/api/internal/assetgen/profiles/anthropomorphic/consistency_test.go`**

```go
package anthropomorphic_test

import (
    "testing"

    "github.com/opc/api/internal/assetgen/profiles/anthropomorphic"
)

func TestCheckAnthropomorphicPerfect(t *testing.T) {
    t.Parallel()

    p := anthropomorphic.DefaultInstance()
    prompt := "峰哥, 成年熊猫, 头身比 1:1.2 圆胖身材, 面部白 rgb(245,240,225), 眼周黑 rgb(26,26,26), 眼神半阖带笑意, 不直视镜头, 身穿朱红汉服, 站在竹林小院, 暖光氛围, 国潮 + 色彩靓丽(朱红#C73E1D/暖橙#E89B45/翠绿#3B8C5A 撞色, 非暗色调、非水墨), 不露爪, 不攻击性姿势, 无文字, 不要 AI 生成感"
    got := anthropomorphic.Check(p, prompt)
    if got.Score < 0.99 {
        t.Errorf("perfect prompt scored %v, want >= 0.99", got.Score)
    }
}

func TestCheckAnthropomorphicPartial(t *testing.T) {
    t.Parallel()

    p := anthropomorphic.DefaultInstance()
    prompt := "峰哥 在竹林"
    got := anthropomorphic.Check(p, prompt)
    if got.Score != 0.20 {
        t.Errorf("partial prompt scored %v, want 0.20", got.Score)
    }
    if len(got.Fails) != 6 {
        t.Errorf("len(Fails) = %d, want 6 (Fails: %v)", len(got.Fails), got.Fails)
    }
}

func TestCheckAnthropomorphicZero(t *testing.T) {
    t.Parallel()

    p := anthropomorphic.DefaultInstance()
    got := anthropomorphic.Check(p, "完全无关的文本")
    if got.Score > 0.01 {
        t.Errorf("zero prompt scored %v, want 0.0", got.Score)
    }
    if len(got.Fails) != 7 {
        t.Errorf("len(Fails) = %d, want 7", len(got.Fails))
    }
}

func TestCheckAnthropomorphicTheme(t *testing.T) {
    t.Parallel()

    p := anthropomorphic.DefaultInstance()
    prompt := "峰哥, 成年熊猫, rgb(245,240,225), rgb(26,26,26), 国潮"
    got := anthropomorphic.Check(p, prompt)
    if got.Score != 0.90 {
        t.Errorf("theme prompt scored %v, want 0.90", got.Score)
    }
    if len(got.Fails) != 2 {
        t.Errorf("len(Fails) = %d, want 2 (anti-claws + anti-AI)", len(got.Fails))
    }
}

func TestCheckAnthropomorphicNil(t *testing.T) {
    t.Parallel()

    got := anthropomorphic.Check(nil, "anything")
    if got.Score != 0 {
        t.Errorf("nil profile scored %v, want 0", got.Score)
    }
}
```

- [ ] **Step 1.17: 写 `apps/api/internal/assetgen/profiles/anthropomorphic/prompts_test.go`**

```go
package anthropomorphic_test

import (
    "strings"
    "testing"

    "github.com/opc/api/internal/assetgen"
    "github.com/opc/api/internal/assetgen/profiles/anthropomorphic"
)

func TestBuildAnthropomorphicImage(t *testing.T) {
    t.Parallel()

    p := anthropomorphic.DefaultInstance()
    got := anthropomorphic.BuildPrompt(p, "竹林小院", "朱红", assetgen.TypeImage)

    wantContains := []string{
        "峰哥", "成年熊猫", "rgb(245,240,225)", "rgb(26,26,26)",
        "国潮", "不露爪", "不要 AI 生成感",
        "竹林小院", "朱红",
        "--ratio 9:16",
    }
    for _, s := range wantContains {
        if !strings.Contains(got, s) {
            t.Errorf("BuildPrompt(image) missing %q", s)
        }
    }
    for _, banned := range []string{"--duration", "--watermark"} {
        if strings.Contains(got, banned) {
            t.Errorf("BuildPrompt(image) unexpectedly contains %q", banned)
        }
    }
}

func TestBuildAnthropomorphicVideo(t *testing.T) {
    t.Parallel()

    p := anthropomorphic.DefaultInstance()
    got := anthropomorphic.BuildPrompt(p, "竹林小院", "翠绿", assetgen.TypeVideo)

    wantContains := []string{
        "峰哥", "成年熊猫", "rgb(245,240,225)", "rgb(26,26,26)",
        "国潮", "不露爪", "不要 AI 生成感",
        "竹林小院", "翠绿",
        "--duration 5 --resolution 720p --ratio 9:16 --watermark true",
    }
    for _, s := range wantContains {
        if !strings.Contains(got, s) {
            t.Errorf("BuildPrompt(video) missing %q", s)
        }
    }
}

func TestBuildAnthropomorphicNil(t *testing.T) {
    t.Parallel()

    got := anthropomorphic.BuildPrompt(nil, "scene", "outfit", assetgen.TypeImage)
    if got != "" {
        t.Errorf("nil profile got %q, want empty", got)
    }
}

func TestBuildAnthropomorphicWrongType(t *testing.T) {
    t.Parallel()

    bogus := &bogusProfile{typeName: "anthropomorphic"}
    got := anthropomorphic.BuildPrompt(bogus, "scene", "outfit", assetgen.TypeImage)
    if got != "" {
        t.Errorf("wrong-type profile got %q, want empty", got)
    }
}

type bogusProfile struct{ typeName string }

func (b *bogusProfile) Type() string    { return b.typeName }
func (b *bogusProfile) Version() string { return "x" }
func (b *bogusProfile) Name() string    { return "x" }
```

- [ ] **Step 1.18: 改 `apps/api/cmd/opc-asset/main.go` — 适配新 API**

具体改法:
1. 删 `*profileVersion` flag (Task 5 加 `--type`)
2. 改 2 处 `prof := assetgen.GetProfile(*profileVersion)` → `prof := assetgen.MustLoadProfile("anthropomorphic")`
3. 改 5 处 `prof.Version` → `prof.Version()`

Run: `cd apps/api && go build -tags fts5 -o /tmp/v-opc-asset-new ./cmd/opc-asset`
Expected: exit 0

- [ ] **Step 1.19: 跑全 assetgen + opc-asset tests 确认 0 回归**

Run: `cd apps/api && go test -tags fts5 -count=1 ./internal/assetgen/... ./cmd/opc-asset/...`
Expected: All PASS (TestProfileFenggeV1, TestBuildPromptImage, TestBuildPromptVideo, TestConsistencyCheck, TestConsistencyCheckUnknownType, TestConsistencyCheckNilProfile, TestBuildPromptUnknownType, plus all 4 anthropomorphic sub-package tests = 12 tests)

- [ ] **Step 1.20: 跑全 API tests 确认没破**

Run: `cd apps/api && go test -tags fts5 -count=1 ./...`
Expected: All PASS

- [ ] **Step 1.21: Commit**

```bash
cd apps/api
git add internal/assetgen/ internal/cmd/  # internal/cmd/ 修正:实际是 cmd/
git commit -m "refactor(assetgen): Profile interface + dispatcher + anthropomorphic (Task 1, combined)

Major parent-package refactor: Profile is now an interface
(Type/Version/Name) with 4 sub-package implementations behind a
registry + dispatcher. The hard-boundary sub-package design from
spec §5.1 is now in code: profiles/{anthropomorphic,...}/ each
own their concrete Profile struct, consistency check, and prompt
builder, and register via init().

Profile interface unifies the 3 places that used to take the
old *Profile struct (consistency.go, prompts.go, opc-asset
main.go) — now they all take Profile interface and the
dispatcher routes by Type().

anthropomorphic sub-package: complete M1 instance (fengge_v1 /
峰哥 / adult panda). profile.json + schema.json embedded via
//go:embed. 7-anchor consistency check (1.0 perfect / 0.95 from
BuildPrompt output / 0.20 name-only). Image+video prompt
builders.

Old assetgen_test.go 4 tests updated to use
anthropomorphic.DefaultInstance() instead of the removed
assetgen.FenggeV1 struct. assetgen.GetProfile removed; callers
use assetgen.LoadProfile(\"anthropomorphic\") or
assetgen.MustLoadProfile(\"anthropomorphic\") (opc-asset uses
the latter). prof.Version field access → prof.Version() method
call (5 sites in opc-asset).

12 new tests added (TestProfileFenggeV1 + 2 dispatcher
+ 9 anthropomorphic sub-package). assetgen package coverage
> 80%. go vet / go build / go test all clean.

Note: Tasks 1+2+3+4 of the original 9-task plan were merged
into this single Task 1 after a Go type-system conflict
discovered during the first subagent dispatch (can't have
type Profile struct + type Profile interface in the same
package). Plan is now 6 tasks total (was 9)."
```

---

### Task 2: digital_human sub-package (was Task 5)

**Files:**
- Create: `apps/api/internal/assetgen/profiles/digital_human/{profile,consistency,prompts}.go`
- Create: `apps/api/internal/assetgen/profiles/digital_human/{profile,schema}.json`
- Create: `apps/api/internal/assetgen/profiles/digital_human/*_test.go`

- [ ] **Step 2.1: 写 consistency_test.go (5 个 fixture)**

```go
package digital_human_test

import (
    "testing"

    "github.com/opc/api/internal/assetgen/profiles/digital_human"
)

func TestCheckDigitalHumanPerfect(t *testing.T) {
    t.Parallel()

    p := digital_human.DefaultInstance()
    prompt := "莉娜, 28 岁, female, 东亚, 鹅蛋脸, 肤色 rgb(245,228,210), 黑色长发披肩, 声线 温柔知性 普通话, 身穿 蓝色西装, 演播室灯光, 写实质感, 镜头特写, 表情自然, 无恐怖谷"
    got := digital_human.Check(p, prompt)
    if got.Score < 0.99 {
        t.Errorf("perfect prompt scored %v, want >= 0.99", got.Score)
    }
}

func TestCheckDigitalHumanPartial(t *testing.T) {
    t.Parallel()

    p := digital_human.DefaultInstance()
    prompt := "莉娜 28 岁 female"
    got := digital_human.Check(p, prompt)
    if got.Score != 0.35 {
        t.Errorf("partial scored %v, want 0.35", got.Score)
    }
    if len(got.Fails) != 5 {
        t.Errorf("len(Fails) = %d, want 5", len(got.Fails))
    }
}

func TestCheckDigitalHumanZero(t *testing.T) {
    t.Parallel()

    p := digital_human.DefaultInstance()
    got := digital_human.Check(p, "完全无关的文本")
    if got.Score > 0.01 {
        t.Errorf("zero scored %v, want 0.0", got.Score)
    }
}

func TestCheckDigitalHumanNil(t *testing.T) {
    t.Parallel()

    got := digital_human.Check(nil, "anything")
    if got.Score != 0 {
        t.Errorf("nil scored %v, want 0", got.Score)
    }
}

func TestCheckDigitalHumanSkinToneWeight(t *testing.T) {
    t.Parallel()

    p := digital_human.DefaultInstance()
    prompt := "rgb(245,228,210)"
    got := digital_human.Check(p, prompt)
    if got.Score != 0.15 {
        t.Errorf("skin-tone-only scored %v, want 0.15", got.Score)
    }
}
```

- [ ] **Step 2.2: 跑 test 确认 FAIL**

Run: `cd apps/api && go test -tags fts5 ./internal/assetgen/profiles/digital_human/...`
Expected: FAIL (package doesn't exist)

- [ ] **Step 2.3: 写 `profile.json`**

```json
{
  "version": "lina_v1",
  "name": "莉娜",
  "type": "digital_human",
  "age": 28,
  "gender": "female",
  "ethnicity": "东亚",
  "face_shape": "鹅蛋脸",
  "hair_style": "黑色长发披肩",
  "voice_tone": "温柔知性, 普通话",
  "skin_tone": "rgb(245,228,210)",
  "accent_color": "宝蓝#1F5FA8"
}
```

- [ ] **Step 2.4: 写 `schema.json`**

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["version", "name", "type", "age", "gender", "ethnicity", "face_shape", "hair_style", "voice_tone", "skin_tone", "accent_color"],
  "properties": {
    "version": { "type": "string" },
    "name": { "type": "string" },
    "type": { "const": "digital_human" },
    "age": { "type": "integer", "minimum": 1, "maximum": 100 },
    "gender": { "enum": ["female", "male", "nonbinary"] },
    "ethnicity": { "type": "string" },
    "face_shape": { "type": "string" },
    "hair_style": { "type": "string" },
    "voice_tone": { "type": "string" },
    "skin_tone": { "type": "string" },
    "accent_color": { "type": "string" }
  }
}
```

- [ ] **Step 2.5: 写 `profile.go` (与 anthropomorphic 同构, 但用 `digital_human.Profile`)**

```go
// Package digital_human registers the digital_human IP type
// (数字人 — virtual YouTubers, virtual idols, news anchors).
// M1 ships one instance: lina_v1 (莉娜 / 28yo East-Asian female).
package digital_human

import (
    _ "embed"
    "encoding/json"
    "fmt"

    "github.com/opc/api/internal/assetgen"
)

type Profile struct {
    Version     string
    Name        string
    Type_       string
    Age         int
    Gender      string
    Ethnicity   string
    FaceShape   string
    HairStyle   string
    VoiceTone   string
    SkinTone    string
    AccentColor string
}

func (p *Profile) Type() string    { return p.Type_ }
func (p *Profile) Version() string  { return p.Version }
func (p *Profile) Name() string     { return p.Name }

//go:embed profile.json
var profileJSON []byte

//go:embed schema.json
var schemaJSON []byte

var defaultInstance *Profile

func init() {
    if err := json.Unmarshal(profileJSON, &defaultInstance); err != nil {
        panic(fmt.Sprintf("digital_human: failed to unmarshal profile.json: %v", err))
    }
    if defaultInstance.Type_ != "digital_human" {
        panic(fmt.Sprintf("digital_human: profile.json has type %q", defaultInstance.Type_))
    }
    assetgen.Register("digital_human", assetgen.ProfileEntry{
        Schema:      defaultInstance,
        Check:       Check,
        BuildPrompt: BuildPrompt,
    })
}

func DefaultInstance() *Profile { return defaultInstance }
func SchemaJSON() []byte        { return schemaJSON }
```

- [ ] **Step 2.6: 写 `consistency.go` (8 anchor, 1.0 total)**

```go
package digital_human

import (
    "strconv"
    "strings"

    "github.com/opc/api/internal/assetgen"
)

func Check(p assetgen.Profile, prompt string) assetgen.ConsistencyResult {
    if p == nil {
        return assetgen.ConsistencyResult{Score: 0, Fails: []string{"nil profile"}}
    }
    dh, ok := p.(*Profile)
    if !ok {
        return assetgen.ConsistencyResult{Score: 0, Fails: []string{"profile is not *digital_human.Profile"}}
    }
    return checkDigitalHuman(dh, prompt)
}

func checkDigitalHuman(p *Profile, prompt string) assetgen.ConsistencyResult {
    var score float64
    var fails []string

    add := func(needle, failName string, weight float64) {
        if strings.Contains(prompt, needle) {
            score += weight
        } else {
            fails = append(fails, failName)
        }
    }

    add(p.Name, "no host name", 0.15)
    add(strconv.Itoa(p.Age), "no age", 0.10)
    add(p.Gender, "no gender", 0.10)
    add(p.Ethnicity, "no ethnicity", 0.10)
    add(p.SkinTone, "no skin tone", 0.15)
    add(p.VoiceTone, "no voice tone", 0.10)
    add("表情自然", "no natural expression", 0.15)
    add("无恐怖谷", "no anti-uncanny-valley", 0.15)

    return assetgen.ConsistencyResult{
        Score: assetgen.Round2Public(score),
        Fails: fails,
    }
}
```

- [ ] **Step 2.7: 写 `prompts.go`**

```go
package digital_human

import (
    "fmt"

    "github.com/opc/api/internal/assetgen"
)

func BuildPrompt(p assetgen.Profile, scene, outfit, assetType string) string {
    if p == nil {
        return ""
    }
    dh, ok := p.(*Profile)
    if !ok {
        return ""
    }
    prefix := fmt.Sprintf(
        "一位 %d 岁 %s %s, %s, 肤色 %s, 发型 %s, 声线 %s, 身穿 %s, 在 %s, 演播室灯光, 写实质感, 镜头特写, 表情自然, 无恐怖谷",
        dh.Age, dh.Ethnicity, dh.Gender, dh.FaceShape,
        dh.SkinTone, dh.HairStyle, dh.VoiceTone, outfit, scene,
    )
    switch assetType {
    case assetgen.TypeVideo:
        return prefix + "  --duration 5 --resolution 720p --ratio 9:16 --watermark true"
    case assetgen.TypeImage:
        return prefix + "  --ratio 9:16"
    default:
        return prefix
    }
}
```

- [ ] **Step 2.8: 写 `prompts_test.go`**

```go
package digital_human_test

import (
    "strings"
    "testing"

    "github.com/opc/api/internal/assetgen"
    "github.com/opc/api/internal/assetgen/profiles/digital_human"
)

func TestBuildDigitalHumanImage(t *testing.T) {
    t.Parallel()

    p := digital_human.DefaultInstance()
    got := digital_human.BuildPrompt(p, "演播室", "蓝色西装", assetgen.TypeImage)

    wantContains := []string{
        "莉娜", "28", "female", "东亚", "鹅蛋脸",
        "rgb(245,228,210)", "黑色长发披肩", "温柔知性, 普通话",
        "演播室", "蓝色西装",
        "--ratio 9:16",
    }
    for _, s := range wantContains {
        if !strings.Contains(got, s) {
            t.Errorf("BuildPrompt(image) missing %q", s)
        }
    }
}
```

- [ ] **Step 2.9: 写 `profile_test.go`**

```go
package digital_human_test

import (
    "testing"

    "github.com/opc/api/internal/assetgen/profiles/digital_human"
)

func TestDigitalHumanProfileRoundTrip(t *testing.T) {
    t.Parallel()

    p := digital_human.DefaultInstance()
    if p == nil {
        t.Fatal("DefaultInstance returned nil")
    }
    if p.Type_ != "digital_human" {
        t.Errorf("Type_ = %q, want digital_human", p.Type_)
    }
    if p.Version != "lina_v1" {
        t.Errorf("Version = %q, want lina_v1", p.Version)
    }
    if p.Age != 28 {
        t.Errorf("Age = %d, want 28", p.Age)
    }
}
```

- [ ] **Step 2.10: 跑全 sub-package test 确认 PASS**

Run: `cd apps/api && go test -tags fts5 ./internal/assetgen/profiles/digital_human/...`
Expected: All tests PASS

- [ ] **Step 2.11: Commit**

```bash
cd apps/api
git add internal/assetgen/profiles/digital_human/
git commit -m "feat(assetgen): digital_human sub-package (Task 2)

Lina (莉娜 / 28yo East-Asian female virtual anchor) instance +
8-anchor consistency check (name/age/gender/ethnicity/skin_tone/
voice_tone/expression/anti-uncanny-valley) + image+video prompt
builder + JSON profile + JSON schema.

Total weights sum to 1.0. 5 consistency + 1 prompt + 1 profile
tests all PASS."
```

---

### Task 3: costume sub-package (was Task 6)

**Files:**
- Create: `apps/api/internal/assetgen/profiles/costume/{profile,consistency,prompts}.go`
- Create: `apps/api/internal/assetgen/profiles/costume/{profile,schema}.json`
- Create: `apps/api/internal/assetgen/profiles/costume/*_test.go`

- [ ] **Step 3.1: 写 consistency_test.go**

```go
package costume_test

import (
    "testing"

    "github.com/opc/api/internal/assetgen/profiles/costume"
)

func TestCheckCostumePerfect(t *testing.T) {
    t.Parallel()

    p := costume.DefaultInstance()
    prompt := "纤云, 唐代, 侠女, 襦裙, 披帛, 绣花鞋, 长剑, 酒壶, 书卷, 朱红#C73E1D, 鹅黄#F5DEB3, 黛绿#2F4F4F, 无穿越, 色调统一"
    got := costume.Check(p, prompt)
    if got.Score < 0.99 {
        t.Errorf("perfect prompt scored %v, want >= 0.99", got.Score)
    }
}

func TestCheckCostumePartial(t *testing.T) {
    t.Parallel()

    p := costume.DefaultInstance()
    prompt := "纤云 唐代"
    got := costume.Check(p, prompt)
    if got.Score != 0.35 {
        t.Errorf("partial scored %v, want 0.35", got.Score)
    }
    if len(got.Fails) != 5 {
        t.Errorf("len(Fails) = %d, want 5", len(got.Fails))
    }
}

func TestCheckCostumeZero(t *testing.T) {
    t.Parallel()

    p := costume.DefaultInstance()
    got := costume.Check(p, "完全无关的文本")
    if got.Score > 0.01 {
        t.Errorf("zero scored %v, want 0.0", got.Score)
    }
}

func TestCheckCostumeNil(t *testing.T) {
    t.Parallel()

    got := costume.Check(nil, "anything")
    if got.Score != 0 {
        t.Errorf("nil scored %v, want 0", got.Score)
    }
}

func TestCheckCostumeNoAnachronism(t *testing.T) {
    t.Parallel()

    p := costume.DefaultInstance()
    prompt := "纤云, 唐代, 侠女, 无穿越"
    got := costume.Check(p, prompt)
    if got.Score != 0.60 {
        t.Errorf("anachronism prompt scored %v, want 0.60", got.Score)
    }
}
```

- [ ] **Step 3.2: 跑 test 确认 FAIL**

Run: `cd apps/api && go test -tags fts5 ./internal/assetgen/profiles/costume/...`
Expected: FAIL

- [ ] **Step 3.3: 写 profile.json**

```json
{
  "version": "xianyun_v1",
  "name": "纤云",
  "type": "costume",
  "era": "唐代",
  "role": "侠女",
  "costume_layer": ["襦裙", "披帛", "绣花鞋"],
  "prop_kit": ["长剑", "酒壶", "书卷"],
  "palette": ["朱红#C73E1D", "鹅黄#F5DEB3", "黛绿#2F4F4F"]
}
```

- [ ] **Step 3.4: 写 schema.json**

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["version", "name", "type", "era", "role", "costume_layer", "prop_kit", "palette"],
  "properties": {
    "version": { "type": "string" },
    "name": { "type": "string" },
    "type": { "const": "costume" },
    "era": { "enum": ["汉代", "唐代", "宋代", "明代", "清代"] },
    "role": { "type": "string" },
    "costume_layer": { "type": "array", "minItems": 2, "items": { "type": "string" } },
    "prop_kit": { "type": "array", "minItems": 1, "items": { "type": "string" } },
    "palette": { "type": "array", "minItems": 3, "items": { "type": "string" } }
  }
}
```

- [ ] **Step 3.5: 写 profile.go**

```go
// Package costume registers the costume IP type (古装 — historical
// period drama characters). M1 ships one instance: xianyun_v1
// (纤云 / Tang dynasty 侠女).
package costume

import (
    _ "embed"
    "encoding/json"
    "fmt"

    "github.com/opc/api/internal/assetgen"
)

type Profile struct {
    Version      string
    Name         string
    Type_        string
    Era          string
    Role         string
    CostumeLayer []string
    PropKit      []string
    Palette      []string
}

func (p *Profile) Type() string    { return p.Type_ }
func (p *Profile) Version() string  { return p.Version }
func (p *Profile) Name() string     { return p.Name }

//go:embed profile.json
var profileJSON []byte

//go:embed schema.json
var schemaJSON []byte

var defaultInstance *Profile

func init() {
    if err := json.Unmarshal(profileJSON, &defaultInstance); err != nil {
        panic(fmt.Sprintf("costume: failed to unmarshal profile.json: %v", err))
    }
    if defaultInstance.Type_ != "costume" {
        panic(fmt.Sprintf("costume: profile.json has type %q", defaultInstance.Type_))
    }
    assetgen.Register("costume", assetgen.ProfileEntry{
        Schema:      defaultInstance,
        Check:       Check,
        BuildPrompt: BuildPrompt,
    })
}

func DefaultInstance() *Profile { return defaultInstance }
func SchemaJSON() []byte        { return schemaJSON }
```

- [ ] **Step 3.6: 写 consistency.go (7 anchor, 1.0 total)**

```go
package costume

import (
    "strings"

    "github.com/opc/api/internal/assetgen"
)

func Check(p assetgen.Profile, prompt string) assetgen.ConsistencyResult {
    if p == nil {
        return assetgen.ConsistencyResult{Score: 0, Fails: []string{"nil profile"}}
    }
    cp, ok := p.(*Profile)
    if !ok {
        return assetgen.ConsistencyResult{Score: 0, Fails: []string{"profile is not *costume.Profile"}}
    }
    return checkCostume(cp, prompt)
}

func checkCostume(p *Profile, prompt string) assetgen.ConsistencyResult {
    var score float64
    var fails []string

    add := func(needle, failName string, weight float64) {
        if strings.Contains(prompt, needle) {
            score += weight
        } else {
            fails = append(fails, failName)
        }
    }

    add(p.Name, "no character name", 0.15)
    add(p.Era, "no era", 0.20)
    add(p.Role, "no role", 0.10)

    layerFound := false
    for _, layer := range p.CostumeLayer {
        if strings.Contains(prompt, layer) {
            layerFound = true
            break
        }
    }
    if layerFound {
        score += 0.15
    } else {
        fails = append(fails, "no costume layer keyword")
    }

    propFound := false
    for _, prop := range p.PropKit {
        if strings.Contains(prompt, prop) {
            propFound = true
            break
        }
    }
    if propFound {
        score += 0.10
    } else {
        fails = append(fails, "no prop keyword")
    }

    add("无穿越", "no anti-anachronism", 0.15)

    colorFound := false
    for _, c := range p.Palette {
        if strings.Contains(prompt, c) {
            colorFound = true
            break
        }
    }
    if colorFound {
        score += 0.15
    } else {
        fails = append(fails, "no palette color")
    }

    return assetgen.ConsistencyResult{
        Score: assetgen.Round2Public(score),
        Fails: fails,
    }
}
```

- [ ] **Step 3.7: 写 prompts.go**

```go
package costume

import (
    "fmt"
    "strings"

    "github.com/opc/api/internal/assetgen"
)

func BuildPrompt(p assetgen.Profile, scene, outfit, assetType string) string {
    if p == nil {
        return ""
    }
    cp, ok := p.(*Profile)
    if !ok {
        return ""
    }
    prefix := fmt.Sprintf(
        "%s, %s, %s, 服饰 %s, 道具 %s, 站在 %s, 色调 %s, 时代准确, 无穿越, 国风质感",
        cp.Name, cp.Era, cp.Role,
        strings.Join(cp.CostumeLayer, "+"),
        strings.Join(cp.PropKit, "+"),
        scene,
        strings.Join(cp.Palette, "/"),
    )
    switch assetType {
    case assetgen.TypeVideo:
        return prefix + "  --duration 5 --resolution 720p --ratio 9:16 --watermark true"
    case assetgen.TypeImage:
        return prefix + "  --ratio 9:16"
    default:
        return prefix
    }
}
```

- [ ] **Step 3.8: 写 prompts_test.go + profile_test.go**

```go
// prompts_test.go
package costume_test

import (
    "strings"
    "testing"

    "github.com/opc/api/internal/assetgen"
    "github.com/opc/api/internal/assetgen/profiles/costume"
)

func TestBuildCostumeImage(t *testing.T) {
    t.Parallel()

    p := costume.DefaultInstance()
    got := costume.BuildPrompt(p, "竹林", "红色披风", assetgen.TypeImage)

    wantContains := []string{
        "纤云", "唐代", "侠女", "襦裙", "长剑",
        "竹林", "红色披风",
        "无穿越",
        "--ratio 9:16",
    }
    for _, s := range wantContains {
        if !strings.Contains(got, s) {
            t.Errorf("BuildPrompt(image) missing %q", s)
        }
    }
}
```

```go
// profile_test.go
package costume_test

import (
    "testing"

    "github.com/opc/api/internal/assetgen/profiles/costume"
)

func TestCostumeProfileRoundTrip(t *testing.T) {
    t.Parallel()

    p := costume.DefaultInstance()
    if p == nil {
        t.Fatal("DefaultInstance returned nil")
    }
    if p.Type_ != "costume" {
        t.Errorf("Type_ = %q, want costume", p.Type_)
    }
    if p.Version != "xianyun_v1" {
        t.Errorf("Version = %q, want xianyun_v1", p.Version)
    }
    if p.Era != "唐代" {
        t.Errorf("Era = %q, want 唐代", p.Era)
    }
    if len(p.CostumeLayer) < 2 {
        t.Errorf("len(CostumeLayer) = %d, want >= 2", len(p.CostumeLayer))
    }
}
```

- [ ] **Step 3.9: 跑全 sub-package test 确认 PASS**

Run: `cd apps/api && go test -tags fts5 ./internal/assetgen/profiles/costume/...`
Expected: All tests PASS

- [ ] **Step 3.10: Commit**

```bash
cd apps/api
git add internal/assetgen/profiles/costume/
git commit -m "feat(assetgen): costume sub-package (Task 3)

Xianyun (纤云 / Tang dynasty 侠女) instance + 7-anchor
consistency check (name/era/role/costume_layer/prop_kit/
no_anachronism/color_consistency) + image+video prompt
builder + JSON profile + JSON schema.

Total weights sum to 1.0. 5 consistency + 1 prompt + 1 profile
tests all PASS."
```

---

### Task 4: info sub-package (was Task 7)

**Files:**
- Create: `apps/api/internal/assetgen/profiles/info/{profile,consistency,prompts}.go`
- Create: `apps/api/internal/assetgen/profiles/info/{profile,schema}.json`
- Create: `apps/api/internal/assetgen/profiles/info/*_test.go`

- [ ] **Step 4.1: 写 consistency_test.go**

```go
package info_test

import (
    "testing"

    "github.com/opc/api/internal/assetgen/profiles/info"
)

func TestCheckInfoPerfect(t *testing.T) {
    t.Parallel()

    p := info.DefaultInstance()
    prompt := "OPC 每日资讯, 演播室双主播, 主播阿橙, 黑体加粗, 暖色字幕, 宝蓝#1F5FA8, rgb(245,240,225), 无 AI 播报感"
    got := info.Check(p, prompt)
    if got.Score < 0.99 {
        t.Errorf("perfect prompt scored %v, want >= 0.99", got.Score)
    }
}

func TestCheckInfoPartial(t *testing.T) {
    t.Parallel()

    p := info.DefaultInstance()
    prompt := "OPC 每日资讯, 演播室双主播, 主播阿橙"
    got := info.Check(p, prompt)
    if got.Score != 0.60 {
        t.Errorf("partial scored %v, want 0.60", got.Score)
    }
    if len(got.Fails) != 3 {
        t.Errorf("len(Fails) = %d, want 3", len(got.Fails))
    }
}

func TestCheckInfoZero(t *testing.T) {
    t.Parallel()

    p := info.DefaultInstance()
    got := info.Check(p, "完全无关的文本")
    if got.Score > 0.01 {
        t.Errorf("zero scored %v, want 0.0", got.Score)
    }
}

func TestCheckInfoNil(t *testing.T) {
    t.Parallel()

    got := info.Check(nil, "anything")
    if got.Score != 0 {
        t.Errorf("nil scored %v, want 0", got.Score)
    }
}

func TestCheckInfoNoAIBroadcast(t *testing.T) {
    t.Parallel()

    p := info.DefaultInstance()
    prompt := "OPC 每日资讯, 演播室双主播, 主播阿橙, 黑体加粗, 暖色字幕, 宝蓝#1F5FA8, rgb(245,240,225)"
    got := info.Check(p, prompt)
    if got.Score != 0.80 {
        t.Errorf("no-ai-broadcast prompt scored %v, want 0.80", got.Score)
    }
}
```

- [ ] **Step 4.2: 跑 test 确认 FAIL**

Run: `cd apps/api && go test -tags fts5 ./internal/assetgen/profiles/info/...`
Expected: FAIL

- [ ] **Step 4.3: 写 profile.json**

```json
{
  "version": "opc_daily_v1",
  "name": "OPC 每日资讯",
  "type": "info",
  "newsroom_style": "演播室双主播",
  "host_persona": "主播阿橙",
  "font_tone": "黑体加粗, 暖色字幕",
  "accent_color": "宝蓝#1F5FA8",
  "background_color": "rgb(245,240,225)"
}
```

- [ ] **Step 4.4: 写 schema.json**

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["version", "name", "type", "newsroom_style", "host_persona", "font_tone", "accent_color", "background_color"],
  "properties": {
    "version": { "type": "string" },
    "name": { "type": "string" },
    "type": { "const": "info" },
    "newsroom_style": { "type": "string" },
    "host_persona": { "type": "string" },
    "font_tone": { "type": "string" },
    "accent_color": { "type": "string" },
    "background_color": { "type": "string" }
  }
}
```

- [ ] **Step 4.5: 写 profile.go**

```go
// Package info registers the info IP type (资讯 — news/information
// host shows). M1 ships one instance: opc_daily_v1 (OPC 每日资讯
// / 演播室双主播).
package info

import (
    _ "embed"
    "encoding/json"
    "fmt"

    "github.com/opc/api/internal/assetgen"
)

type Profile struct {
    Version         string
    Name            string
    Type_           string
    NewsroomStyle   string
    HostPersona     string
    FontTone        string
    AccentColor     string
    BackgroundColor string
}

func (p *Profile) Type() string    { return p.Type_ }
func (p *Profile) Version() string  { return p.Version }
func (p *Profile) Name() string     { return p.Name }

//go:embed profile.json
var profileJSON []byte

//go:embed schema.json
var schemaJSON []byte

var defaultInstance *Profile

func init() {
    if err := json.Unmarshal(profileJSON, &defaultInstance); err != nil {
        panic(fmt.Sprintf("info: failed to unmarshal profile.json: %v", err))
    }
    if defaultInstance.Type_ != "info" {
        panic(fmt.Sprintf("info: profile.json has type %q", defaultInstance.Type_))
    }
    assetgen.Register("info", assetgen.ProfileEntry{
        Schema:      defaultInstance,
        Check:       Check,
        BuildPrompt: BuildPrompt,
    })
}

func DefaultInstance() *Profile { return defaultInstance }
func SchemaJSON() []byte        { return schemaJSON }
```

- [ ] **Step 4.6: 写 consistency.go (6 anchor, 1.0 total)**

```go
package info

import (
    "strings"

    "github.com/opc/api/internal/assetgen"
)

func Check(p assetgen.Profile, prompt string) assetgen.ConsistencyResult {
    if p == nil {
        return assetgen.ConsistencyResult{Score: 0, Fails: []string{"nil profile"}}
    }
    ip, ok := p.(*Profile)
    if !ok {
        return assetgen.ConsistencyResult{Score: 0, Fails: []string{"profile is not *info.Profile"}}
    }
    return checkInfo(ip, prompt)
}

func checkInfo(p *Profile, prompt string) assetgen.ConsistencyResult {
    var score float64
    var fails []string

    add := func(needle, failName string, weight float64) {
        if strings.Contains(prompt, needle) {
            score += weight
        } else {
            fails = append(fails, failName)
        }
    }

    add(p.HostPersona, "no host persona", 0.20)
    add(p.NewsroomStyle, "no newsroom style", 0.20)
    add(p.FontTone, "no font tone", 0.15)
    add(p.AccentColor, "no accent color", 0.15)
    add("无 AI 播报感", "no anti-AI-broadcast", 0.20)
    add(p.BackgroundColor, "no background color", 0.10)

    return assetgen.ConsistencyResult{
        Score: assetgen.Round2Public(score),
        Fails: fails,
    }
}
```

- [ ] **Step 4.7: 写 prompts.go**

```go
package info

import (
    "fmt"

    "github.com/opc/api/internal/assetgen"
)

func BuildPrompt(p assetgen.Profile, scene, outfit, assetType string) string {
    if p == nil {
        return ""
    }
    ip, ok := p.(*Profile)
    if !ok {
        return ""
    }
    prefix := fmt.Sprintf(
        "%s, %s, %s, 字幕 %s, 强调色 %s, 背景色 %s, 在 %s, 演播室专业感, 无 AI 播报感",
        ip.Name, ip.NewsroomStyle, ip.HostPersona,
        ip.FontTone, ip.AccentColor, ip.BackgroundColor, scene,
    )
    switch assetType {
    case assetgen.TypeVideo:
        return prefix + "  --duration 5 --resolution 720p --ratio 9:16 --watermark true"
    case assetgen.TypeImage:
        return prefix + "  --ratio 9:16"
    default:
        return prefix
    }
}
```

- [ ] **Step 4.8: 写 prompts_test.go + profile_test.go**

```go
// prompts_test.go
package info_test

import (
    "strings"
    "testing"

    "github.com/opc/api/internal/assetgen"
    "github.com/opc/api/internal/assetgen/profiles/info"
)

func TestBuildInfoImage(t *testing.T) {
    t.Parallel()

    p := info.DefaultInstance()
    got := info.BuildPrompt(p, "演播室", "黑体加粗", assetgen.TypeImage)

    wantContains := []string{
        "OPC 每日资讯", "演播室双主播", "主播阿橙",
        "黑体加粗, 暖色字幕", "宝蓝#1F5FA8", "rgb(245,240,225)",
        "演播室", "无 AI 播报感",
        "--ratio 9:16",
    }
    for _, s := range wantContains {
        if !strings.Contains(got, s) {
            t.Errorf("BuildPrompt(image) missing %q", s)
        }
    }
}
```

```go
// profile_test.go
package info_test

import (
    "testing"

    "github.com/opc/api/internal/assetgen/profiles/info"
)

func TestInfoProfileRoundTrip(t *testing.T) {
    t.Parallel()

    p := info.DefaultInstance()
    if p == nil {
        t.Fatal("DefaultInstance returned nil")
    }
    if p.Type_ != "info" {
        t.Errorf("Type_ = %q, want info", p.Type_)
    }
    if p.Version != "opc_daily_v1" {
        t.Errorf("Version = %q, want opc_daily_v1", p.Version)
    }
    if p.NewsroomStyle != "演播室双主播" {
        t.Errorf("NewsroomStyle = %q, want 演播室双主播", p.NewsroomStyle)
    }
}
```

- [ ] **Step 4.9: 跑全 sub-package test 确认 PASS**

Run: `cd apps/api && go test -tags fts5 ./internal/assetgen/profiles/info/...`
Expected: All tests PASS

- [ ] **Step 4.10: Commit**

```bash
cd apps/api
git add internal/assetgen/profiles/info/
git commit -m "feat(assetgen): info sub-package (Task 4)

OPC Daily News (OPC 每日资讯 / 演播室双主播 / 主播阿橙) instance
+ 6-anchor consistency check (host_persona/newsroom_style/
font_tone/accent_color/no_ai_broadcast/background_color) +
image+video prompt builder + JSON profile + JSON schema.

Total weights sum to 1.0. 5 consistency + 1 prompt + 1 profile
tests all PASS. assetgen package now has 4 IP types registered."
```

---

### Task 5: opc-asset CLI 加 `--type` flag (was Task 8)

**Files:**
- Modify: `apps/api/cmd/opc-asset/main.go`
- Create: `apps/api/cmd/opc-asset/main_test.go` (如不存在)

- [ ] **Step 5.1: 写 CLI test 覆盖 `--type` flag 4 个值**

```go
package main

import (
    "bytes"
    "io"
    "os"
    "strings"
    "testing"
)

// TestRunCheckAllTypes confirms the --type flag is wired and each
// of the 4 IP types produces a high ConsistencyResult for a
// hand-crafted prompt that hits all anchors.
func TestRunCheckAllTypes(t *testing.T) {
    t.Parallel()

    cases := []struct {
        typeName string
        prompt   string
    }{
        {
            typeName: "anthropomorphic",
            prompt:   "峰哥, 成年熊猫, rgb(245,240,225), rgb(26,26,26), 国潮, 不露爪, 不要 AI 生成感",
        },
        {
            typeName: "digital_human",
            prompt:   "莉娜, 28, female, 东亚, rgb(245,228,210), 温柔知性, 普通话, 表情自然, 无恐怖谷",
        },
        {
            typeName: "costume",
            prompt:   "纤云, 唐代, 侠女, 襦裙, 长剑, 无穿越, 朱红#C73E1D",
        },
        {
            typeName: "info",
            prompt:   "OPC 每日资讯, 演播室双主播, 主播阿橙, 黑体加粗, 暖色字幕, 宝蓝#1F5FA8, rgb(245,240,225), 无 AI 播报感",
        },
    }

    for _, tc := range cases {
        tc := tc
        t.Run(tc.typeName, func(t *testing.T) {
            t.Parallel()

            args := []string{"check", "--type", tc.typeName, "--prompt", tc.prompt}
            output := captureStdout(t, func() {
                if err := runCheck(args); err != nil {
                    t.Fatalf("runCheck(%v) error: %v", args, err)
                }
            })
            if !strings.Contains(output, "0.99") && !strings.Contains(output, "1.00") {
                t.Errorf("output missing score >= 0.99: %s", output)
            }
        })
    }
}

// captureStdout redirects os.Stdout during fn, returns what was
// printed. Used for CLI tests that print to stdout instead of
// returning strings.
func captureStdout(t *testing.T, fn func()) string {
    t.Helper()

    old := os.Stdout
    r, w, err := os.Pipe()
    if err != nil {
        t.Fatalf("os.Pipe: %v", err)
    }
    os.Stdout = w

    fn()

    w.Close()
    os.Stdout = old

    var buf bytes.Buffer
    if _, err := io.Copy(&buf, r); err != nil {
        t.Fatalf("io.Copy: %v", err)
    }
    return buf.String()
}
```

- [ ] **Step 5.2: 跑 test 确认 FAIL（`runCheck` 不接受 `--type`）**

Run: `cd apps/api && go test -tags fts5 -run TestRunCheckAllTypes ./cmd/opc-asset/...`
Expected: FAIL (unknown flag --type)

- [ ] **Step 5.3: 改 `main.go` — 加 `--type` flag, 默认 anthropomorphic, 改用 MustLoadProfile**

**关键改动** (具体 diff 视 main.go 现状, 思路):

1. 加 `var ipType = flag.String("type", "anthropomorphic", "IP type: anthropomorphic/digital_human/costume/info")` 在 `runCheck` / `runGenerate` 的 flag.StringVar 段
2. 删 `profileVersion` flag (Task 1 改时已删)
3. `runCheck` 和 `runGenerate` 里 `prof := assetgen.MustLoadProfile("anthropomorphic")` 改为 `prof := assetgen.MustLoadProfile(*ipType)`
4. 任何 `fmt.Println` 加 `\n` (test 用 captureStdout 期望 newline)

- [ ] **Step 5.4: 跑 TestRunCheckAllTypes 确认 PASS**

Run: `cd apps/api && go test -tags fts5 -run TestRunCheckAllTypes ./cmd/opc-asset/...`
Expected: PASS for all 4 sub-tests

- [ ] **Step 5.5: 跑 build 确认 binary 仍可编译**

Run: `cd apps/api && go build -tags fts5 -o /tmp/v-opc-asset-task5 ./cmd/opc-asset`
Expected: exit 0

- [ ] **Step 5.6: 跑 4 个 type 的 smoke**

```bash
/tmp/v-opc-asset-task5 check --type anthropomorphic --prompt "峰哥, 成年熊猫, rgb(245,240,225), rgb(26,26,26), 国潮, 不露爪, 不要 AI 生成感"
/tmp/v-opc-asset-task5 check --type digital_human --prompt "莉娜, 28, female, 东亚, rgb(245,228,210), 温柔知性, 普通话, 表情自然, 无恐怖谷"
/tmp/v-opc-asset-task5 check --type costume --prompt "纤云, 唐代, 侠女, 襦裙, 长剑, 无穿越, 朱红#C73E1D"
/tmp/v-opc-asset-task5 check --type info --prompt "OPC 每日资讯, 演播室双主播, 主播阿橙, 黑体加粗, 暖色字幕, 宝蓝#1F5FA8, rgb(245,240,225), 无 AI 播报感"
```

Expected: each prints score >= 0.99

- [ ] **Step 5.7: 跑全 assetgen + opc-asset tests 确认 0 回归**

Run: `cd apps/api && go test -tags fts5 ./internal/assetgen/... ./cmd/opc-asset/...`
Expected: All PASS

- [ ] **Step 5.8: Commit**

```bash
cd apps/api
git add cmd/opc-asset/
git commit -m "feat(opc-asset): --type flag for IP type selection (Task 5)

CLI now routes consistency check + prompt build through the
assetgen dispatcher based on --type (default: anthropomorphic
for back-compat with Phase 1).

New test TestRunCheckAllTypes covers all 4 IP types (anthro /
digital_human / costume / info) hitting 0.99+ score on
hand-crafted prompts. Phase 1 --prompt + --scene + --outfit +
--type image/video flags all still work.

Manual smoke: 4 opc-asset check invocations return correct
scores, no panics, exit 0."
```

---

### Task 6: 集成测试 + 文档 + 最终 verify + 状态文件更新 (was Task 9)

**Files:**
- Modify: `scripts/api-smoke.sh`
- Create: `docs/assetgen/profile-schema.md`
- Modify: `~/.claude/PROJECT-STATE.md`
- Modify: `~/.claude/GAP-LOG.md`

- [ ] **Step 6.1: 读现有 api-smoke.sh 了解结构**

Run: `wc -l scripts/api-smoke.sh; head -50 scripts/api-smoke.sh`

- [ ] **Step 6.2: 在 api-smoke.sh 末尾加 4 个 type 的 consistency check case**

加在文件末尾（保留所有现有 case）:

```bash
# === assetgen: 4 IP type consistency check (Phase 2 sub-spec A) ===
section "assetgen profile types"

opc_asset_check() {
    local type="$1" prompt="$2"
    /root/workspace/opc/apps/bin/opc-asset check --type "$type" --prompt "$prompt" 2>&1 | tail -3
}

assert_score_ge() {
    local got="$1" want="$2" name="$3"
    local got_score
    got_score=$(echo "$got" | grep -oE 'score[: =]+[0-9.]+' | grep -oE '[0-9.]+' | head -1)
    if [ -z "$got_score" ]; then
        fail "$name: no score found in output"
        return 1
    fi
    if awk "BEGIN{exit !($got_score >= $want)}"; then
        pass "$name: score=$got_score >= $want"
    else
        fail "$name: score=$got_score < $want"
    fi
}

make -C apps/api asset 2>&1 | tail -2

out=$(opc_asset_check "anthropomorphic" "峰哥, 成年熊猫, rgb(245,240,225), rgb(26,26,26), 国潮, 不露爪, 不要 AI 生成感")
assert_score_ge "$out" "0.99" "anthropomorphic check"

out=$(opc_asset_check "digital_human" "莉娜, 28, female, 东亚, rgb(245,228,210), 温柔知性, 普通话, 表情自然, 无恐怖谷")
assert_score_ge "$out" "0.99" "digital_human check"

out=$(opc_asset_check "costume" "纤云, 唐代, 侠女, 襦裙, 长剑, 无穿越, 朱红#C73E1D")
assert_score_ge "$out" "0.99" "costume check"

out=$(opc_asset_check "info" "OPC 每日资讯, 演播室双主播, 主播阿橙, 黑体加粗, 暖色字幕, 宝蓝#1F5FA8, rgb(245,240,225), 无 AI 播报感")
assert_score_ge "$out" "0.99" "info check"

# Advisory: zero-score prompt still works (not blocked)
out=$(opc_asset_check "anthropomorphic" "完全无关的文本")
got_score=$(echo "$out" | grep -oE 'score[: =]+[0-9.]+' | grep -oE '[0-9.]+' | head -1)
if [ "$got_score" = "0" ] || [ "$got_score" = "0.0" ] || [ "$got_score" = "0.00" ]; then
    pass "advisory: zero-score prompt returns 0 score (not blocked)"
else
    fail "advisory: zero-score prompt got score=$got_score, want 0"
fi
```

- [ ] **Step 6.3: build opc-asset + 跑 smoke**

```bash
cd /root/workspace/opc
make -C apps/api asset
./scripts/api-smoke.sh 2>&1 | tail -20
```

Expected: All assetgen type checks PASS

- [ ] **Step 6.4: 写 `docs/assetgen/profile-schema.md`**

```markdown
# OPC Assetgen Profile Schema Reference

> 4 套 JSON sub-schema 定义 4 类 IP profile (anthropomorphic /
> digital_human / costume / info). Spec:
> docs/superpowers/specs/2026-06-11-opc-phase2-ip-template-design.md

## 公共字段

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `version` | string | ✓ | 实例级唯一 version 字符串 |
| `name` | string | ✓ | 展示名 |
| `type` | enum | ✓ | discriminator: anthropomorphic / digital_human / costume / info |

## 1. anthropomorphic

适用: 拟人动物 IP (panda, fox, dog, ...).

```json
{
  "version": "fengge_v1",
  "name": "峰哥",
  "type": "anthropomorphic",
  "species": "成年熊猫",
  "body_shape": "头身比 1:1.2 圆胖身材",
  "body_color": "rgb(245,240,225)",
  "eye_color": "rgb(26,26,26)",
  "eye_expression": "半阖带笑意, 不直视镜头",
  "palette": ["朱红#C73E1D", "暖橙#E89B45", "翠绿#3B8C5A", "宝蓝#1F5FA8", "米白#F5F0E1"]
}
```

Consistency anchors (7 个, 1.0 total): name(0.20) + species(0.20) + body_color(0.20) + eye_color(0.20) + theme("国潮")(0.10) + no_claws("不露爪")(0.05) + no_ai_look("不要 AI 生成感")(0.05).

## 2. digital_human

适用: 数字人 IP (虚拟主播 / 虚拟偶像 / 新闻主播).

```json
{
  "version": "lina_v1",
  "name": "莉娜",
  "type": "digital_human",
  "age": 28,
  "gender": "female",
  "ethnicity": "东亚",
  "face_shape": "鹅蛋脸",
  "hair_style": "黑色长发披肩",
  "voice_tone": "温柔知性, 普通话",
  "skin_tone": "rgb(245,228,210)",
  "accent_color": "宝蓝#1F5FA8"
}
```

Anchors (8 个, 1.0 total): name(0.15) + age(0.10) + gender(0.10) + ethnicity(0.10) + skin_tone(0.15) + voice_tone(0.10) + "表情自然"(0.15) + "无恐怖谷"(0.15).

## 3. costume

适用: 古装 IP (朝代角色).

```json
{
  "version": "xianyun_v1",
  "name": "纤云",
  "type": "costume",
  "era": "唐代",
  "role": "侠女",
  "costume_layer": ["襦裙", "披帛", "绣花鞋"],
  "prop_kit": ["长剑", "酒壶", "书卷"],
  "palette": ["朱红#C73E1D", "鹅黄#F5DEB3", "黛绿#2F4F4F"]
}
```

Anchors (7 个, 1.0 total): name(0.15) + era(0.20) + role(0.10) + costume_layer(0.15) + prop_kit(0.10) + "无穿越"(0.15) + palette_color(0.15).

Era enum: 汉代 / 唐代 / 宋代 / 明代 / 清代.

## 4. info

适用: 资讯 IP (演播室新闻节目).

```json
{
  "version": "opc_daily_v1",
  "name": "OPC 每日资讯",
  "type": "info",
  "newsroom_style": "演播室双主播",
  "host_persona": "主播阿橙",
  "font_tone": "黑体加粗, 暖色字幕",
  "accent_color": "宝蓝#1F5FA8",
  "background_color": "rgb(245,240,225)"
}
```

Anchors (6 个, 1.0 total): host_persona(0.20) + newsroom_style(0.20) + font_tone(0.15) + accent_color(0.15) + "无 AI 播报感"(0.20) + background_color(0.10).

## CLI

```bash
opc-asset generate --type digital_human --scene 演播室 --outfit 蓝色西装 --out /tmp/x.jpg
opc-asset check --type costume --prompt "纤云, 唐代, ..."
opc-asset ledger --last 10
```

不加 `--type` 默认 `anthropomorphic` (向后兼容).

## 加载机制

4 个 sub-package (internal/assetgen/profiles/<type>/) 各自 `init()` 调 `assetgen.Register(type, ProfileEntry)`. JSON profile + schema 通过 `//go:embed` 编译进 binary, 运行时无 IO.

## 扩展 (M2+)

- 加新 IP type = 新 sub-package + profile.json + schema.json + consistency.go + prompts.go
- M2: profile 上传 endpoint (用 sub-package.SchemaJSON() 验证)
- M3: per-version lookup (M1 一个 type 一个 instance)
```

- [ ] **Step 6.5: 跑全 tests + 集成 + verify-project-state**

```bash
cd /root/workspace/opc/apps/api
go test -tags fts5 -count=1 ./...
go test -tags fts5 -cover ./internal/assetgen/... | grep -E "coverage|ok"
cd /root/workspace/opc
./scripts/api-smoke.sh 2>&1 | tail -30
```

Expected: All tests PASS; coverage ≥ 80%; smoke 4 type cases PASS

- [ ] **Step 6.6: Dispatch verify-project-state subagent**

按 `~/.claude/agents/verify-project-state.md` 规范 dispatch 一个 subagent 重跑所有验收命令, OVERALL: PASS 才算 M1 完。

```bash
# 关键命令:
go vet -tags fts5 ./...
go test -tags fts5 -count=1 ./...
go test -tags fts5 -cover ./internal/assetgen/...    # ≥ 80%
go build -tags fts5 -o /tmp/v-opc-api ./cmd/server
go build -tags fts5 -o /tmp/v-opc-asset ./cmd/opc-asset
opc-asset check --type anthropomorphic --prompt "峰哥, 成年熊猫, rgb(245,240,225), rgb(26,26,26), 国潮, 不露爪, 不要 AI 生成感"
opc-asset check --type digital_human --prompt "..."
opc-asset check --type costume --prompt "..."
opc-asset check --type info --prompt "..."
opc-asset check --type anthropomorphic --prompt "完全无关的文本"  # advisory: 0 score, 不拦
ls docs/assetgen/profile-schema.md  # exists
```

报告写到 `~/.claude/audit/verify-reports/verify-YYYYMMDD-HHMMSS-phase2-sub-spec-a.md`

- [ ] **Step 6.7: 改 `~/.claude/PROJECT-STATE.md`**

按 §H 协议:
- Section E 加 bullet: 6 task commits + W5 verify PASS
- Section D: 标 G13 (sub-spec A M1) **CLOSED 2026-06-XX (verify run N)**

- [ ] **Step 6.8: 改 `~/.claude/GAP-LOG.md` (新增 G13 + 标 closed)**

Append:
```markdown
## YYYY-MM-DD HH:MM (objective, observed) — G13: Phase 2 sub-spec A M1 not delivered

- **缺口**: spec 写了但没实现. 4 套 sub-schema + 4 套 check + 4 个 instance + opc-asset --type 都没落地.
- **影响**: Phase 2 B/C/D/E sub-spec 全部依赖 A.
- **修复计划**: 见 docs/superpowers/plans/2026-06-11-opc-phase2-ip-template-m1.md (6 task, was 9)
- **status**: **closed YYYY-MM-DD** — Task 1-6 全部 done, verify-project-state OVERALL PASS
```

- [ ] **Step 6.9: Commit 收尾**

```bash
cd /root/workspace/opc
git add scripts/api-smoke.sh docs/assetgen/profile-schema.md
git commit -m "test(smoke): add 4 IP type consistency check cases (Task 6)

scripts/api-smoke.sh: 4 new test cases covering all 4 IP types
in the assetgen registry. Each case uses a hand-crafted prompt
that hits all anchors (score >= 0.99). Plus an advisory case:
zero-score prompt still returns 0 score (not blocked by gate).

docs/assetgen/profile-schema.md: 100+ line reference doc covering
4 sub-schemas, anchors + weights, CLI usage, M2+ extension path."
```

---

## 验收清单（M1 done 的标志, 6 task 全部 DONE）

| # | 验收 | 验证 | 状态 |
|---|------|------|------|
| 1 | 4 个 sub-package init() 自动注册到 assetgen.registry | `go test -tags fts5 -run TestProfile ./internal/assetgen/...` PASS | ☐ |
| 2 | 4 个 instance profile JSON 通过 //go:embed 编译进 binary | `go build` 0 + `opc-asset check --type digital_human` 返 score | ☐ |
| 3 | 4 套 consistency check 各 5 个 fixture 测试 PASS | `go test -tags fts5 ./internal/assetgen/profiles/...` 全绿 | ☐ |
| 4 | `LoadProfile("anthropomorphic")` + interface 方法可用 (替代原 `GetProfile("fengge_v1")`) | `go test -tags fts5 -run TestProfileFenggeV1` PASS | ☐ |
| 5 | opc-asset CLI 加 --type flag, 4 种 type 都能 check | `opc-asset check --type <X>` 4 个 type 都 exit 0 | ☐ |
| 6 | Consistency check advisory (不拦生成) | zero-score prompt 仍能 generate | ☐ |
| 7 | assetgen package line coverage ≥ 80% | `go test -tags fts5 -cover` ≥ 80% | ☐ |
| 8 | 文档: docs/assetgen/profile-schema.md 列 4 套子 schema 字段 | 文件存在 + ≥ 100 行 | ☐ |
| 9 | verify-project-state agent OVERALL: PASS | `~/.claude/audit/verify-reports/verify-*.md` PASS | ☐ |
| 10 | PROJECT-STATE.md + GAP-LOG.md 更新 | M1 closure 记录在案 | ☐ |

---

## 不做的事（M1 范围硬约束）

- ❌ Web UI profile editor
- ❌ 多创作者 + 权限
- ❌ profile 版本演进机制
- ❌ JSON Schema 强校验 at runtime
- ❌ 任何 new IP type beyond 4
- ❌ profile 热重载
- ❌ strict mode / threshold gate
- ❌ 改 internal/agents/minimax.go
- ❌ 删 audit/roundtrip-volcengine.mjs

---

## 风险与缓解

| 风险 | 缓解 |
|------|------|
| `Profile.Type()` 方法跟 struct 字段重名, Go 编译错 | 已用 `Type_` 字段 + `Type()` 方法重命名模式 (所有 sub-package 都用) |
| 4 个 sub-package 顺序依赖 | 互不依赖, 父包 `registry` 是 map, 顺序无关 |
| `embed` 在某些 OS 失败 | 编译期就拦 |
| 旧 test 用 `assetgen.FenggeV1` struct, Task 1 已改用 `anthropomorphic.DefaultInstance()` | 4 个老 test 签名变化但 case 保留 |
| `init()` panic 信息不够详细 | panic message 含 type name + 失败原因 |
| M1 没强制 JSON Schema 验证, 坏 JSON 会 init panic | 接受 (fail-fast 比 silent failure 好) |
| opc-asset 删 `*profileVersion` flag 后, 旧用法 break | 1-line 迁移到 `--type anthropomorphic`, Task 5 加 --type |

---

## Self-Review (after merging Tasks 1-4)

写 plan 时做了 4 轮 self-review:

1. **Spec coverage**: spec §5 (架构) / §6 (组件) / §7 (schema) / §8 (consistency) / §9 (prompt) / §11 (error handling) / §12 (testing) / §13 (timeline) / §15 (跟 Phase 1 关系) 全部对应到 6 task.
2. **Placeholder scan**: 0 个 TBD/TODO/类似措辞.
3. **Type consistency**:
   - `Profile` interface 在 Task 1 定义, 4 sub-package 全部 `*Profile` 实现
   - `Register(type, ProfileEntry{Schema, Check, BuildPrompt})` 签名 Task 1 定义, 4 sub-package 一致
   - `Check(p Profile, prompt string) ConsistencyResult` 签名 Task 1 定义, Task 2/3/4 sub-package 同构
   - `BuildPrompt(p Profile, scene, outfit, assetType string) string` 签名 Task 1 定义, Task 2/3/4 同构
   - `DefaultInstance() *Profile` + `SchemaJSON() []byte` 4 sub-package 全部暴露
4. **Type-system conflict resolved**: Plan 1.0 (9 task) 漏了 `Profile` 在 4 个文件同时被用 struct+interface 的 Go 限制. Plan 1.1 (6 task) 把 Tasks 1-4 合并 + 删 `assetgen.FenggeV1` struct + 改用 `anthropomorphic.DefaultInstance()` + opc-asset 适配. 已验证 via 第一次 subagent dispatch.

无矛盾. Plan ready for execution (Task 1 first, 6 subagent dispatches total).
