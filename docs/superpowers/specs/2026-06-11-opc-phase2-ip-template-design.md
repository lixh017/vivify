# OPC Phase 2 — IP 模板框架设计（Sub-Spec A）

> **文档版本**：v1.0（初稿，待审）
> **日期**：2026-06-11
> **作者**：OPC 团队（与 Claude 协作）
> **状态**：设计阶段 → 待用户审阅
> **路径**：`docs/superpowers/specs/2026-06-11-opc-phase2-ip-template-design.md`
> **父 spec**：`docs/superpowers/specs/2026-06-03-opc-phase1-ip-design.md` §5.1（Phase 2 轮廓）
> **兄弟 spec**：M1 5 sub-spec 之 **A**（B/C/D/E 待启）

---

## 一、文档目的

把 Phase 1 单 IP（峰哥/成年熊猫）的硬编码 `assetgen` 扩成 4 类 IP 可插拔的模板框架，作为 Phase 2 「把工作流沉淀为工具」的 **平台基底**。M1 交付 4 套子 schema + 4 套 consistency check + 4 个 instance profile。M2 在此之上加多创作者 profile 上传，M3 加 agent 交易。

本文档是 spec 级别（WHAT & WHY），具体任务分解见后续 `writing-plans` skill 输出。

---

## 二、Executive Summary

| 项 | 内容 |
|----|------|
| **目标** | 把 `internal/assetgen/` 从「1 个 Go 写死的熊猫 profile」重构为「4 类 IP 可插拔框架」 |
| **范围（M1）** | 4 个 sub-package + 4 套 JSON sub-schema + 4 套 consistency check + 4 个 instance profile + opc-asset CLI 加 `--type` flag |
| **4 类 IP** | anthropomorphic（拟人动物，panda 续）/ digital_human（数字人）/ costume（古装）/ info（资讯） |
| **Pluggability 层** | Data layer（JSON profile，编译期 `//go:embed` 打包） |
| **Schema 架构** | Discriminated union — `type` 字段路由到 4 套子 schema |
| **Consistency enforce** | Advisory（记 score 不拦生成，threshold M1 不定） |
| **代码结构** | 4 sub-package 硬边界，父包 `assetgen` 持 registry |
| **存储** | `//go:embed profiles/<type>/{profile,schema}.json`，编译期进 binary，运行时只读 |
| **多租户** | M1 无（Phase 2 中段才上） |
| **M1 timeline** | 4-5 周（1 周 1 类型） |
| **依赖** | 现有 `internal/assetgen/{profile,prompts,consistency}.go` 全部重构；`cmd/opc-asset/main.go` 加 `--type` flag；`agent.minimax.go` 不动 |
| **风险等级** | 🟡 中（schema 设计走偏会限制 M2 多创作者扩展） |

---

## 三、背景与现状

### 3.1 Phase 1 IP 模板的边界

Phase 1 跑熊猫 IP 1 个时，IP 模板长这样（`internal/assetgen/profile.go`）：

```go
type Profile struct {
    Version, Name, Species, Body, Eyes string
    Colors  ProfileColors
    Palette []string
}
var FenggeV1 = Profile{...}  // 唯一 profile
func GetProfile(version string) *Profile
```

5 个 hardcoded 字段 + 1 个 `rgb(245,240,225)` 身体色 + 1 个 `rgb(26,26,26)` 眼周色，**够 panda 用，但撑不起 4 类 IP**：

| 现状问题 | 影响 Phase 2 的哪一类 |
|---------|----------------------|
| 只有 1 个 `Profile` struct | 数字人需要 `age/ethnicity/face_shape/voice` 字段；古装需要 `era/costume_layer/prop_kit` 字段；资讯需要 `newsroom_style/font_tone/host_persona` 字段 |
| `ConsistencyCheck` 7 个硬编码 anchor | 数字人检查「表情自然度」「无恐怖谷」；古装检查「时代准确度」「服装不穿越」；资讯检查「无 AI 播报感」 |
| `BuildPrompt` 一段 Chinese 硬编码 | 古装的 prompt 模板要嵌朝代关键词；资讯的 prompt 模板要嵌「演播室/字幕/主播」结构 |
| 单 profile 版本号 | 数字人同一个 host 可有 v1（写实版）v2（卡通风）两个 profile，需要 schema-level multi-version |

### 3.2 已有可复用资产

不要重新造轮子——M1 复用 Phase 1 的：

- `internal/assetgen/profile.go` —— 公共 `Profile` interface 留父包
- `internal/assetgen/prompts.go` —— 公共 prefix builder（颜色 + 调性）抽出来，type-specific tail 移到 sub-package
- `internal/assetgen/consistency.go` —— `ConsistencyResult{Score, Fails}` 公共 type
- `cmd/opc-asset/main.go` —— `check`/`generate`/`ledger` 3 个 subcommand + godotenv + ledger 格式（向后兼容）
- `internal/agents/minimax.go` —— 不动（profile 框架跟 LLM 客户端解耦）
- `apps/api/internal/assetgen/assetgen_test.go` —— 已有的 4 个 unit test 保留 + 扩

---

## 四、设计目标（M1 验收）

| # | 验收 | 验证命令 |
|---|------|---------|
| 1 | 4 个 sub-package（`profiles/{anthropomorphic,digital_human,costume,info}`）init() 自动注册到 `assetgen.registry` | `cd apps/api && go test -tags fts5 -run TestRegistry ./internal/assetgen/...` PASS |
| 2 | 4 个 instance profile JSON 通过 `//go:embed` 编译进 binary，启动期反序列化无错 | `go build -tags fts5 ./...` exit 0 + `opc-asset check --type digital_human --prompt "..."` 返回 score |
| 3 | 4 套 consistency check 各 5-10 个 fixture 测试（perfect/partial/zero/edge） | `go test -tags fts5 ./internal/assetgen/profiles/...` 100% PASS |
| 4 | `GetProfile("fengge_v1")` 向后兼容（Phase 1 行为不变） | `cd apps/api && go test -tags fts5 -run TestBackwardCompat ./internal/assetgen/...` PASS |
| 5 | opc-asset CLI 加 `--type` flag，4 种 type 都能跑 | `opc-asset check --type info --prompt "..."` 返 0+ score |
| 6 | Consistency check 是 advisory（不拦生成） | 跑 0 分 prompt，`opc-asset generate` 仍成功（仅 score 入 ledger） |
| 7 | assetgen package line coverage ≥ 80% | `go test -tags fts5 -cover ./internal/assetgen/...` 报告 ≥ 80% |
| 8 | 文档：`docs/assetgen/profile-schema.md` 列 4 套子 schema 字段 | 文件存在 + ≥ 100 行 |

**Out of scope（M1 不做）**：
- ❌ Web UI profile editor
- ❌ 多创作者 + 权限
- ❌ profile 版本演进机制（v1→v2 迁移工具）
- ❌ JSON Schema 强校验（best-effort validate，M2 才强制）
- ❌ 任何 new IP type（仅在 4 个声明的范围内）
- ❌ profile 热重载（重启生效即可）

---

## 五、架构

### 5.1 包结构

```
internal/assetgen/                              (refactored parent)
├── profile.go                                  (公共: Profile interface, AssetType const, ProfileEntry struct)
├── consistency.go                              (公共: ConsistencyResult, ConsistencyFn type, ConsistencyCheck dispatcher)
├── loader.go                                   (NEW: registry map + Register + LoadProfile)
├── prompts.go                                  (公共: BuildPrompt dispatcher + 公共 prefix helper)
├── *_test.go                                   (公共 dispatcher + round-trip 测试)
└── profiles/                                   (NEW: 4 sub-packages, 硬边界)
    ├── anthropomorphic/
    │   ├── profile.go                          (AnthropomorphicProfile struct + Instance() + init() 注册)
    │   ├── consistency.go                      (checkAnthropomorphic)
    │   ├── prompts.go                          (buildAnthropomorphicPrompt)
    │   ├── profile.json                        (fengge_v1 instance, 从现有 FenggeV1 移植)
    │   ├── schema.json                         (JSON Schema 验证文件, best-effort)
    │   └── *_test.go
    ├── digital_human/                          (同上 6 文件)
    ├── costume/                                (同上 6 文件)
    └── info/                                   (同上 6 文件)
```

### 5.2 关键设计决策

| 决策 | 选项 | 选定 | 理由 |
|------|------|------|------|
| Pluggability 层 | JSON / Go struct / YAML DSL | **JSON + go:embed** | 创作者低门槛贡献（未来 M2）；编译期打包减少运行时配置错 |
| Schema 架构 | Discriminated union / Generic common / Base+extension / Hybrid | **Discriminated union (4 套子 schema)** | 每类 IP 表达充分，静态验证强，未来加新 type = 新 sub-package |
| M1 范围 | 4 schema/check + N instance | **4 schema + 4 check + 4 instance** | 用户最小化决定，避免 YAGNI 污染 |
| Consistency enforce | Advisory / Gate / Per-type gate / Two-tier | **Advisory** | M1 阶段 threshold 未校准；先收集 score 数据，M2 再决定 gate 阈值 |
| 代码结构 | 4 sub-packages / 1 package+subdirs+registry / 1 all-in-one | **4 sub-packages（硬边界）** | 用户选择；强隔离 + 编译期错误检查 |
| 存储 | embed / file / DB | **//go:embed** | M1 单租户，无需 DB；编译期打包零运行时 IO |
| 多租户 | Yes / No | **M1 无** | Phase 1 spec §5.1 明确 M2 才上 |

### 5.3 Profile interface

```go
// 父包 assetgen/profile.go
type Profile interface {
    Type() string                                       // "anthropomorphic" / "digital_human" / "costume" / "info"
    Version() string                                    // "fengge_v1" / "lina_v1" / ...
    Name() string                                       // 展示名: "峰哥" / "莉娜" / ...
}

type ProfileEntry struct {
    Schema      Profile                                 // type-specific struct 实现 Profile interface
    Check       ConsistencyFn                           // (profile, prompt) ConsistencyResult
    BuildPrompt PromptFn                                // (profile, scene, outfit, assetType) string
}

type ConsistencyFn func(Profile, string) ConsistencyResult
type PromptFn func(Profile, string, string, string) string  // profile, scene, outfit, assetType

var registry = map[string]ProfileEntry{}               // init() 填充

func Register(typeName string, entry ProfileEntry) {
    if _, exists := registry[typeName]; exists {
        panic("assetgen: duplicate Register for type " + typeName)
    }
    registry[typeName] = entry
}
```

### 5.4 4 类 IP sub-schema 概览

每个 sub-package 自带 1 套 JSON Schema 描述 instance profile 的字段。下表是 4 类的概念性差异，具体字段在每个 sub-package 的 `schema.json` 里。

| Type | 关键字段 | 一致性 check 重点 | Prompt tail 特点 |
|------|---------|------------------|-----------------|
| `anthropomorphic` | species, body_shape, body_color, eye_color, eye_expression, palette, name | 名字/物种/身体色/眼周色/调性（5 anchor × 0.20）+ 2 抗 AI（× 0.10） | "汉服+场景"前缀 + 暖光国潮 tail |
| `digital_human` | age, gender, ethnicity, face_shape, hair_style, voice_tone, name | 名字/年龄段/瞳色/肤色调/「表情自然」/「无恐怖谷」/「主播感」 | 演播室灯光 + 写实质感 + 镜头特写 |
| `costume` | era, role, costume_layer, prop_kit, palette, name | 名字/朝代/服饰层级/「无穿越」/「色调统一」/「服化道一致」 | 朝代关键词 + 服饰描述 + 道具罗列 |
| `info` | newsroom_style, host_persona, font_tone, accent_color, name | 名字/演播室类型/主播人设/字幕风格/「无 AI 播报感」/品牌色 | 演播室 + 字幕条 + 主播姿态 |

---

## 六、组件

### 6.1 父包 `internal/assetgen/`

**职责**：公共 type + registry + dispatcher + 向后兼容 API。

| 文件 | 内容 |
|------|------|
| `profile.go` | `Profile` interface, `AssetType` const, `ProfileEntry` struct |
| `consistency.go` | `ConsistencyResult{Score, Fails}`, `ConsistencyFn` type, `ConsistencyCheck(p, prompt)` dispatcher |
| `loader.go` | `registry` map, `Register()`, `LoadProfile(type, version)` |
| `prompts.go` | `BuildPrompt(p, scene, outfit, assetType)` dispatcher, 公共 prefix helper（颜色拼装 + palette join） |
| `*_test.go` | dispatcher 测试、未知 type 处理、重复注册 panic、向后兼容 |

**向后兼容**：
```go
// Phase 1 行为保留
func GetProfile(version string) *Profile {
    if version == "fengge_v1" || version == "" {
        p, _ := LoadProfile("anthropomorphic", "fengge_v1")
        return &p
    }
    return nil
}
```

### 6.2 4 sub-packages（每个同构）

**`internal/assetgen/profiles/<type>/`**

| 文件 | 模板（以 anthropomorphic 为例） |
|------|------------------------------|
| `profile.go` | `AnthropomorphicProfile` struct + `Type()/Version()/Name()` + `Instance() *AnthropomorphicProfile` + `init() { Register("anthropomorphic", ProfileEntry{...}) }` |
| `consistency.go` | `checkAnthropomorphic(p Profile, prompt string) ConsistencyResult` —— 5-7 个 anchor + weight |
| `prompts.go` | `buildAnthropomorphicPrompt(p Profile, scene, outfit, assetType string) string` —— type-specific 模板 |
| `profile.json` | `{"version":"fengge_v1","name":"峰哥",...}` —— 默认 instance |
| `schema.json` | JSON Schema 验证文件 |
| `*_test.go` | round-trip test + consistency fixtures + prompt builder test |

**4 个 sub-package 互不 import**（编译期强边界保证）。需要共享代码走父包 `assetgen` 的公共 helper。

### 6.3 opc-asset CLI 改动

| flag | 默认 | 说明 |
|------|------|------|
| `--type` | `anthropomorphic` | 选 IP type，影响 consistency check + prompt builder |
| `--scene` | 必填（generate）/ 可选（check） | 场景描述 |
| `--outfit` | 必填（generate）/ 可选（check） | 服饰描述 |
| `--prompt` | 互斥 | 跑 consistency check 的 prompt |

**保留向后兼容**：`opc-asset generate --scene X --outfit Y`（无 `--type`）默认走 anthropomorphic，行为跟 Phase 1 一致。

**新行为**：`opc-asset check --type digital_human --prompt "..."` 走数字人 check。

---

## 七、Schema 设计

### 7.1 公共字段（4 个 sub-schema 都有的）

```json
{
  "version": "fengge_v1",          // 必填, 字符串, 实例级唯一
  "name": "峰哥",                  // 必填, 字符串, 展示名
  "type": "anthropomorphic"       // 必填, enum: 4 个值
}
```

`type` 字段是 discriminator，loader 启动期反序列化时按它路由到对应 sub-schema 验证。

### 7.2 4 套子 schema 字段设计

#### 7.2.1 `anthropomorphic`（panda 续）

```json
{
  "version": "fengge_v1",
  "name": "峰哥",
  "type": "anthropomorphic",
  "species": "成年熊猫",            // 必填
  "body_shape": "头身比 1:1.2 圆胖身材",  // 必填
  "body_color": "rgb(245,240,225)",  // 必填
  "eye_color": "rgb(26,26,26)",       // 必填
  "eye_expression": "半阖带笑意, 不直视镜头",  // 必填
  "palette": ["朱红#C73E1D", "暖橙#E89B45", "翠绿#3B8C5A", "宝蓝#1F5FA8", "米白#F5F0E1"]  // 必填, ≥3
}
```

**Consistency check anchors**（7 个，总权重 1.0）：
| Anchor | Weight |
|--------|--------|
| name ("峰哥") | 0.20 |
| species ("成年熊猫") | 0.20 |
| body_color ("rgb(245,240,225)") | 0.20 |
| eye_color ("rgb(26,26,26)") | 0.20 |
| theme ("国潮") | 0.10 |
| no_claws ("不露爪") | 0.05 |
| no_ai_look ("不要 AI 生成感") | 0.05 |

#### 7.2.2 `digital_human`

```json
{
  "version": "lina_v1",
  "name": "莉娜",
  "type": "digital_human",
  "age": 28,                       // 必填, 整数, 1-100
  "gender": "female",              // 必填, enum: female/male/nonbinary
  "ethnicity": "东亚",              // 必填
  "face_shape": "鹅蛋脸",          // 必填
  "hair_style": "黑色长发披肩",     // 必填
  "voice_tone": "温柔知性, 普通话",  // 必填
  "skin_tone": "rgb(245,228,210)", // 必填
  "accent_color": "宝蓝#1F5FA8"     // 必填, 1 个
}
```

**Consistency check anchors**：
| Anchor | Weight |
|--------|--------|
| name | 0.15 |
| age_range (年龄段 in 描述) | 0.10 |
| gender (性别特征) | 0.10 |
| ethnicity (ethnicity 关键词) | 0.10 |
| skin_tone (rgb 字符串) | 0.15 |
| voice_tone (音色调性关键词) | 0.10 |
| natural_expression ("表情自然") | 0.15 |
| no_uncanny_valley ("无恐怖谷") | 0.15 |

#### 7.2.3 `costume`

```json
{
  "version": "xianyun_v1",
  "name": "纤云",
  "type": "costume",
  "era": "唐代",                    // 必填, enum: 唐代/宋代/明代/汉代/清代
  "role": "侠女",                  // 必填
  "costume_layer": ["襦裙", "披帛", "绣花鞋"],  // 必填, ≥2 层
  "prop_kit": ["长剑", "酒壶", "书卷"],          // 必填, ≥1
  "palette": ["朱红#C73E1D", "鹅黄#F5DEB3", "黛绿#2F4F4F"]  // 必填, ≥3
}
```

**Consistency check anchors**：
| Anchor | Weight |
|--------|--------|
| name | 0.15 |
| era (朝代关键词) | 0.20 |
| role (角色描述) | 0.10 |
| costume_layer (服饰层级关键词) | 0.15 |
| prop_kit (道具关键词) | 0.10 |
| no_anachronism ("无穿越") | 0.15 |
| color_consistency (palette 颜色在 prompt 中) | 0.15 |

#### 7.2.4 `info`

```json
{
  "version": "opc_daily_v1",
  "name": "OPC 每日资讯",
  "type": "info",
  "newsroom_style": "演播室双主播",   // 必填
  "host_persona": "主播阿橙",         // 必填
  "font_tone": "黑体加粗, 暖色字幕",   // 必填
  "accent_color": "宝蓝#1F5FA8",       // 必填
  "background_color": "rgb(245,240,225)"  // 必填
}
```

**Consistency check anchors**：
| Anchor | Weight |
|--------|--------|
| host_persona | 0.20 |
| newsroom_style (演播室类型) | 0.20 |
| font_tone (字幕风格) | 0.15 |
| accent_color (强调色) | 0.15 |
| no_ai_broadcast ("无 AI 播报感") | 0.20 |
| background_color (rgb 字符串) | 0.10 |

### 7.3 JSON Schema 验证

每个 sub-package 自带 `schema.json` 用 [JSON Schema Draft 2020-12](https://json-schema.org/) 描述。loader 启动时 best-effort 校验 instance `profile.json`：

- ✅ 校验通过：log info，继续
- ❌ 校验失败：exit 1 + 详细 error（M1 严格模式，阻止启动）

**M1 限制**：校验只在启动时跑一次，运行时（hot-reload）不重跑。M2 接 profile 上传时再扩。

---

## 八、Consistency Check 设计

### 8.1 Dispatcher

```go
// 父包 assetgen/consistency.go
func ConsistencyCheck(p Profile, prompt string) ConsistencyResult {
    entry, ok := registry[p.Type()]
    if !ok {
        return ConsistencyResult{Score: 0, Fails: []string{"unknown profile type: " + p.Type()}}
    }
    return safeCheck(entry.Check, p, prompt)  // recover panic → Score=0
}

func safeCheck(fn ConsistencyFn, p Profile, prompt string) (result ConsistencyResult) {
    defer func() {
        if r := recover(); r != nil {
            result = ConsistencyResult{Score: 0, Fails: []string{fmt.Sprintf("check panic: %v", r)}}
        }
    }()
    return fn(p, prompt)
}
```

### 8.2 Type-specific check 模板

```go
// internal/assetgen/profiles/digital_human/consistency.go
func checkDigitalHuman(p Profile, prompt string) ConsistencyResult {
    dh := p.(*DigitalHumanProfile)  // type assertion 限定在 sub-package
    var score float64
    var fails []string

    check := func(needle, failName string, weight float64) {
        if strings.Contains(prompt, needle) {
            score += weight
        } else {
            fails = append(fails, failName)
        }
    }

    check(dh.Name, "no host name", 0.15)
    check(strconv.Itoa(dh.Age), "no age", 0.10)
    check(dh.Gender, "no gender", 0.10)
    check(dh.Ethnicity, "no ethnicity", 0.10)
    check(dh.SkinTone, "no skin tone", 0.15)
    check(dh.VoiceTone, "no voice tone", 0.10)
    check("表情自然", "no natural expression", 0.15)
    check("无恐怖谷", "no anti-uncanny-valley", 0.15)

    return ConsistencyResult{Score: round2(score), Fails: fails}
}
```

### 8.3 Advisory 行为

ConsistencyCheck **不返回 error**，仅返回 result。调用方（opc-asset / future platform handler）决定怎么处理：

- **opc-asset check 子命令**：打印 score + fails，exit 0（除非调用方传 `--strict`，M1 不实现）
- **opc-asset generate 子命令**：跑 check 仅作 ledger 记录，不阻止生成
- **future platform handler**：M2 决定如何用 score（M1 不预设）

### 8.4 Score 格式

`math.Round(score*100) / 100` —— 保留 2 位小数，跟 Phase 1 `audit/roundtrip-volcengine.mjs` ledger 格式一致。

---

## 九、Prompt 构建

### 9.1 Dispatcher

```go
// 父包 assetgen/prompts.go
func BuildPrompt(p Profile, scene, outfit, assetType string) string {
    entry, ok := registry[p.Type()]
    if !ok {
        return ""  // 未知 type 返回空, caller 决定 fallback
    }
    return entry.BuildPrompt(p, scene, outfit, assetType)
}
```

### 9.2 公共 prefix helper

```go
// 父包 assetgen/prompts.go
func joinPalette(colors []string) string {
    return strings.Join(colors, "/")
}
```

### 9.3 Type-specific 模板（示意，digital_human 为例）

```go
// internal/assetgen/profiles/digital_human/prompts.go
func buildDigitalHumanPrompt(p Profile, scene, outfit, assetType string) string {
    dh := p.(*DigitalHumanProfile)
    prefix := fmt.Sprintf(
        "一位 %d 岁 %s %s, %s, 肤色 %s, 发型 %s, 声线 %s, 身穿 %s, 在 %s, 演播室灯光, 写实质感, 镜头特写",
        dh.Age, dh.Ethnicity, dh.Gender, dh.FaceShape,
        dh.SkinTone, dh.HairStyle, dh.VoiceTone, outfit, scene,
    )
    switch assetType {
    case TypeVideo:
        return prefix + "  --duration 5 --resolution 720p --ratio 9:16 --watermark true"
    case TypeImage:
        return prefix + "  --ratio 9:16"
    default:
        return prefix
    }
}
```

anthropomorphic/costume/info 同构，模板细节在各自 sub-package。

---

## 十、数据流

### 10.1 启动期

```
┌─────────────────────────────────────────────────────┐
│ cmd/opc-asset/main.go 启动                          │
│ 1. godotenv.Load()                                  │
│ 2. 隐式触发 4 sub-package 的 init()                 │
│    → assetgen.Register("anthropomorphic", entry1)   │
│    → assetgen.Register("digital_human", entry2)     │
│    → assetgen.Register("costume", entry3)           │
│    → assetgen.Register("info", entry4)              │
│ 3. //go:embed 加载 4 个 profile.json               │
│ 4. 反序列化为对应 type struct                       │
│ 5. best-effort JSON Schema 验证                    │
│    → 失败: exit 1 + error log                       │
│    → 成功: 继续                                     │
└─────────────────────────────────────────────────────┘
```

### 10.2 运行时（check 子命令）

```
opc-asset check --type digital_human --prompt "..."
  ↓
1. flag 解析, 拿到 type="digital_human" + prompt
2. assetgen.LoadProfile("digital_human", "")  // version 省略取默认
3. assetgen.ConsistencyCheck(profile, prompt)
   2.1 查 registry["digital_human"] → 拿到 entry
   2.2 调 entry.Check(profile, prompt) → ConsistencyResult{Score, Fails}
4. 打印 result
5. exit 0
```

### 10.3 运行时（generate 子命令）

```
opc-asset generate --type costume --scene 竹林 --outfit 红色披风 --out /tmp/x.jpg
  ↓
1. flag 解析
2. assetgen.LoadProfile("costume", "")
3. assetgen.BuildPrompt(profile, "竹林", "红色披风", "image") → "..."
4. agents.MiniMax 生成图片 → /tmp/x.jpg
5. assetgen.ConsistencyCheck(profile, prompt) → score 入 ledger
6. ledger 追加一行 (timestamp, profile_version, cost_cny=0.05, score, status)
```

---

## 十一、错误处理

| 错误情形 | 检测时机 | 处理 |
|---------|---------|------|
| profile.json 缺失 | 编译期 (`go:embed` 失败) | `go build` 失败，编译期就拦 |
| profile.json 损坏 / 不符 schema | 启动期反序列化 | exit 1 + 详细 error（M1 严格模式） |
| 重复注册相同 type | init() 期间 | `Register` panic，启动崩溃，强制 fail-fast |
| `ConsistencyCheck` 收到未知 type | 运行时 | 返回 `Score=0, Fails=["unknown profile type: X"]` + log warn（不 panic） |
| type-specific check 抛 panic | 运行时 | `safeCheck` recover + 返回 Score=0 + 错 fail msg（不 crash 服务） |
| schema.json 缺失 | 启动期 | 跳过校验 + log warning（best-effort 模式） |
| `LoadProfile` 未知 type 或 version | 运行时 | 返回 error，caller 决定 fallback |
| `BuildPrompt` 未知 type | 运行时 | 返回空字符串 + log warn |

---

## 十二、测试策略

### 12.1 测试矩阵

| 层 | 覆盖 | 文件 | 数量目标 |
|----|------|------|---------|
| Unit: round-trip | 4 个 profile.json 都能 unmarshal → marshal → unmarshal 不丢字段 | `profiles/<type>/profile_test.go` | 4 个 sub-package × 3 case = 12 |
| Unit: consistency fixtures | 4 个 check 函数各 5-10 个 fixture（perfect/partial/zero/edge/typo） | `profiles/<type>/consistency_test.go` | 4 × 8 = 32 |
| Unit: prompt builder | 4 个 build 函数各 3-5 个 fixture（type=image/video/empty） | `profiles/<type>/prompts_test.go` | 4 × 4 = 16 |
| Unit: dispatcher | registry lookup, 未知 type 处理, 重复注册 panic, safeCheck recover | `assetgen/{loader,consistency,prompts}_test.go` | ~10 |
| Unit: 向后兼容 | `GetProfile("fengge_v1")` 仍返回相同结果 | `assetgen/profile_test.go` | 1（snapshot test） |
| Integration: opc-asset CLI | `--type digital_human --prompt "..."` 跑出 0+ score | `cmd/opc-asset/main_test.go` | 4 type × 1 = 4 |
| Coverage | assetgen package ≥ 80% line |  |  |

### 12.2 关键 fixture 例子（consistency）

`profiles/digital_human/consistency_test.go`:

```go
func TestCheckDigitalHumanPerfect(t *testing.T) {
    p := digital_human.Instance()  // 默认 lina_v1
    prompt := "莉娜, 28 岁, female, 东亚, 鹅蛋脸, 肤色 rgb(245,228,210), 黑色长发披肩, 温柔知性 普通话, 身穿 蓝色西装, 演播室灯光, 表情自然, 无恐怖谷, 镜头特写"
    got := digital_human.Check(p, prompt)
    if got.Score < 0.95 {
        t.Errorf("perfect prompt scored %v, want >= 0.95", got.Score)
    }
    if len(got.Fails) > 0 {
        t.Errorf("perfect prompt has fails: %v", got.Fails)
    }
}

func TestCheckDigitalHumanZero(t *testing.T) {
    p := digital_human.Instance()
    got := digital_human.Check(p, "完全无关的文本")
    if got.Score > 0.05 {
        t.Errorf("zero prompt scored %v, want <= 0.05", got.Score)
    }
}
```

---

## 十三、实施计划（M1 timeline）

| 周 | 任务 | 交付物 |
|----|------|--------|
| W1 | 父包 `assetgen` 重构（公共 type + registry + dispatcher + 向后兼容） | 现有 `profile.go` / `prompts.go` / `consistency.go` 拆出 dispatcher；`GetProfile("fengge_v1")` 测试通过 |
| W1 | `anthropomorphic` sub-package 完整实现（profile.go + consistency.go + prompts.go + profile.json + schema.json + 测试） | 全套 + 测试全绿 |
| W2 | `digital_human` sub-package 完整实现 | 同上 |
| W3 | `costume` sub-package 完整实现 | 同上 |
| W4 | `info` sub-package 完整实现 | 同上 |
| W4 | opc-asset CLI 加 `--type` flag | 4 type 都能 check + generate |
| W5 | 集成测试 + 文档（`docs/assetgen/profile-schema.md`） + verify-project-state PASS | 4/4 DOD 验证 |

**W5 末 verify-project-state agent 必跑**，OVERALL PASS 才算 M1 完。

---

## 十四、风险与不做的事

### 14.1 风险

| 风险 | 缓解 |
|------|------|
| Schema 设计走偏，M2 多创作者 profile 上传时大改 | 子 schema 留 `extras: object` 自由扩展字段（M1 不强制验证） |
| 4 套 consistency check 写得太相似，重构时漂移 | 抽公共 `checkOne(needle, name, weight, prompt, *score, *fails)` helper 到父包 |
| JSON profile 在某些 OS 上 embed 失败（极罕见） | `go build` 期就拦，不会到运行时 |
| 4 个 sub-package 边界太严，未来想加跨 type helper 困难 | 跨 type helper 全部放父包 `assetgen` |
| `init()` 注册顺序依赖 | 4 sub-package 互不依赖；父包 `registry` 是 map，顺序无关 |
| ledger 格式变化破坏向后兼容 | M1 保留旧 ledger 字段（profile_version 已有，type 字段新增） |

### 14.2 不做的事（避坑清单）

- ❌ **不做 profile 热重载**（重启生效即可，节省复杂度）
- ❌ **不做 Web UI profile editor**（M1 范围最小化）
- ❌ **不做多创作者 + 权限**（Phase 2 中段才上）
- ❌ **不做 profile 版本演进机制**（v1→v2 迁移工具推到 M2）
- ❌ **不做 strict mode / threshold gate**（M1 advisory，先收集 score 数据）
- ❌ **不做 any new IP type**（仅 4 个声明的 anthropomorphic/digital_human/costume/info）
- ❌ **不动 `internal/agents/minimax.go`**（profile 框架跟 LLM 客户端解耦）
- ❌ **不写「创作者风格库」**（Phase 1 spec §4.6 隐藏任务，与本 sub-spec 正交）

---

## 十五、与 Phase 1 既有代码的关系

| 现有 | M1 处置 | 理由 |
|------|--------|------|
| `internal/assetgen/profile.go` (`Profile` struct, `FenggeV1`, `GetProfile`) | 拆：公共 type 留父包，`FenggeV1` 移到 `profiles/anthropomorphic/profile.json` + `AnthropomorphicProfile` struct，`GetProfile` 改写为 delegate | 保持向后兼容 + 走新 dispatcher |
| `internal/assetgen/prompts.go` (`BuildPrompt`) | 拆：dispatcher 留父包，type-specific tail 移到 4 sub-package | 复用公共 prefix |
| `internal/assetgen/consistency.go` (`ConsistencyCheck`) | 拆：dispatcher 留父包，type-specific check 移到 4 sub-package | 复用 `ConsistencyResult` type |
| `internal/assetgen/assetgen_test.go` (4 个 test) | 扩：保留原 4 个 + 加 dispatcher / 向后兼容 / round-trip 测试 | 不丢测试 |
| `cmd/opc-asset/main.go` | 加 `--type` flag，dispatch 时按 type 选 profile | CLI 仍向后兼容（默认 anthropomorphic） |
| `internal/agents/minimax.go` | 不动 | 解耦 |
| `audit/roundtrip-volcengine.mjs` | 不动 | 历史 demo，M2 才删 |

---

## 十六、与 Phase 2 其他 sub-spec 的关系

| Sub-spec | 与本 sub-spec 的关系 |
|----------|---------------------|
| B（4 能力工具化） | 依赖 A：选题/脚本/拆解/资产 4 个 CLI 都要消费 IP profile |
| C（对外 web UI） | 依赖 B + A：web UI 选 IP type → 调 B 的能力 → 走 A 的 profile |
| D（agent-agnostic 平台） | 正交：D 是 agent 调用层，跟 profile 框架解耦 |
| E（反 AI 检测 / 拟人化） | 依赖 A 的 consistency check 基础：拟人化 = 提升 consistency score |

**关键路径**：A → B → C 是 M1 串行依赖。A 完成前 B/C 无法启动；D 可与 A/B/C 并行。

---

## 十七、变更记录

| 版本 | 日期 | 变更 | 作者 |
|------|------|------|------|
| v1.0 | 2026-06-11 | 初稿（基于 brainstorming 4 问澄清：JSON pluggable / 1+3 IP 类型 / Discriminated union / 最小范围 / Advisory / 4 sub-packages 硬边界） | OPC + Claude |

---

**下一步**：
- 用户审阅本文档，提出修改意见
- 通过后进入 Spec 自审（检查占位符/矛盾/歧义/范围）
- 然后调用 `writing-plans` skill，把 sub-spec A 拆为可执行的 M1 实施计划（具体到周/任务）
- M1 完成后，依次启 B/C/D/E sub-spec 的 brainstorming → spec → plan 循环
