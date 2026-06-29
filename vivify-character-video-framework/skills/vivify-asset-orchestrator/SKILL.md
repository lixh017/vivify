---
name: vivify-asset-orchestrator
description: Use this skill when opc needs to generate real assets — coordinates router → cost-cap → provider → retry → ledger pipeline. Backed by `vivify.asset_orchestrator.generate_asset()`. Reads `vivify-asset-router` for routing, calls `vivify-provider-volcengine` (Ark) or `vivify-provider-minimax` (Hailuo) for execution, writes `~/.claude/agents/vivify-asset-ledger.jsonl`. Triggers on "opc 生成资产", "跑 asset pipeline", "调 router", or any opc asset generation request.
---

# vivify-asset-orchestrator

> Verified against commit cb9a7e8. If you find drift, file an issue.

协调 `vivify-asset-router` + `ProviderAdapter` + `cost_cap` + `retry` + `asset_ledger`。**唯一可以调 provider 的入口** (`vivify.asset_orchestrator.generate_asset`)。其他 skill / CLI 命令都通过 `generate_asset(req, env=...)` 跑。

## 1. 输入 (GenerateRequest, vivify/providers/base.py)

```python
from vivify.providers.base import GenerateRequest

req = GenerateRequest(
    scene_id="panda-st05-sh02-v1",        # 来自 vivify-scene-decomposition
    asset_type="video",                   # AssetType literal: "video" | "image" | "tts" | "bgm"
    prompt="...",                         # 已嵌 fenggeV1 anchor 的完整 prompt
    reference_image="/path/to/first_frame.jpg",  # 可选, video 必填 (minimax 强制)
    duration_sec=5,                       # video only
    size="1024x1792",                     # image only
    outfit="hufu_red",                    # IP anchor 字段 (5 套之一)
    tone="国潮",                          # IP anchor 字段: 国潮/治愈/哲学/御宅
    options={"out_path": "/tmp/out.mp4",  # vendor-specific extras
             "model": "doubao-seedance-1-5-pro-251215",
             "character_ref": "/path/to/face.jpg"},
)
```

## 2. 流程 (vivify.asset_orchestrator.generate_asset)

```python
from vivify.asset_orchestrator import generate_asset, PROFILE_VERSION
from vivify.providers.base import GenerateRequest

result = generate_asset(
    req,
    env={"ARK_API_KEY": "...", "ARK_BASE_URL": "https://ark.cn-beijing.volces.com/api/v3"},
    config_path=None,           # 默认 ~/.claude/config/vivify-providers.yaml
    provider_filter=None,      # "ark" 强制只用 ark
    db_path="opc.db",           # 可选, 用于月度 cap 检查
    monthly_hard=60000,        # ¥, 需要 db_path 才生效
)

# result: GenerateResult
# result.ok=True   → result.local_path / result.public_url / result.cost_yuan / result.model
# result.ok=False  → result.error / result.status_hint (ledger status: provider-failed / budget-exceeded / timeout / network-error / pending: no-provider)
```

内部步骤 (`generate_asset`, 节选):

1. **Router picks provider** — `pick(cfg, req.asset_type, requirements={duration_sec, outfit, tone, options}, provider_filter=...)`
2. **No provider found** → 写 `pending: no-provider` ledger 行 + 返回 `GenerateResult(ok=False, status_hint="pending: no-provider")`
3. **Pre-call cost gate** — `_default_model_for(provider_id, cfg)` 解析 provider_id → model_name, 再 `_model_cost_yuan(model, duration_sec)`。若 `> per_asset_cny` → `budget-exceeded` (不调 provider)
4. **Monthly cap check** (可选) — 若给 `db_path + monthly_hard`, 从 `episodes` 表 SUM `cost_yuan` 当月, `+ estimate > monthly_hard` → `budget-exceeded`
5. **Build adapter** — `_build_adapter(provider_id, env, asset_type)`: `"ark"`→`ArkProvider`, `"minimax"`→`MiniMaxProvider`, 其他 → `stubs.py` 的桩 (`NotImplementedError`)
6. **Generate with retry** — `RetryPolicy(max_retries=3, base_delay_sec=5, backoff_factor=3)` (5s/15s/45s/135s backoff)。每轮:
   - 调 `adapter.generate(req, **kwargs)` — kwargs 仅 orchestrator 知道的几个 (`out_path`, `model`, `character_ref`)
   - `result.ok` → `_emit_success` (写 ledger) + return
   - `result.ok=False` → `should_retry(error, attempt, policy)` 决定 — permanent (quota/401/400/policy) 直接停, transient (5xx/429/network/timeout) 重试; 中间态写 `provider-retry` ledger
   - 用 `classify_error()` 映射到 ledger status (`"timeout"` / `"network-error"` / `"provider-failed"`)
7. **Ledger write** — `_emit_success` / `_emit_failure` 写 `LedgerEntry` (`timestamp`, `scene_id`, `provider`, `type`, `cost_cny`, `duration_ms`, `status`, `asset_path`, `profile_version="fengge_v1"`, `prompt_hash=fnv1a_64(req.prompt)`, `outfit`, `error`)

Retry policy (`vivify/retry.py`):

- `RetryPolicy(max_retries=3, base_delay_sec=5, backoff_factor=3)` → 5s / 15s / 45s / 135s backoff
- `FailureClass.PERMANENT` (quota_exceeded / 401 / 400 / content_policy) 不重试
- `FailureClass.TRANSIENT` (5xx / 429 / timeout / network) 重试
- `FailureClass.UNKNOWN` 重试一次

## 3. fenggeV1 anchor 嵌入 (必嵌)

Orchestrator **不在调用时现拼 prompt** — caller (`vivify episode render` 或 test) 负责在 `GenerateRequest.prompt` 里把 anchor 拼好。Orchestrator 责任是:

- 给 ledger 行打 `profile_version: "fengge_v1"` (`asset_orchestrator.PROFILE_VERSION`, 模块常量)
- 把 `req.outfit` / `req.tone` 转发给 adapter (ark adapter 不用, minimax adapter 可能用)

Anchor 内容见 `vivify-panda-character` SKILL § (峰哥 visual_anchor: 头身比 1:1.2 圆胖, 面部白 rgb(245,240,225) 眼周黑 rgb(26,26,26), 半阖带笑意, 5 套服饰, 朱红/暖橙/翠绿/宝蓝/米白 palette, 禁: 露爪/攻击性/卖惨/暗灰/低饱和/网红脸/完美对称/8K/ultra-detailed/masterpiece)。

## 4. Ledger 格式 (JSONL append-only)

`vivify/asset_ledger.py` `LedgerEntry` 字段 (实际写入的):

```jsonl
{"timestamp":"2026-06-24T10:00:00.123Z","scene_id":"panda-st05-sh02-v1","provider":"ark","type":"video","cost_cny":1.05,"duration_ms":45000,"status":"success","asset_path":"/tmp/out/panda-st05-sh02-v1.mp4","profile_version":"fengge_v1","prompt_hash":"fnv64:0123456789abcdef","outfit":"hufu_red"}
```

字段:

| 字段 | 类型 | 来源 |
|------|------|------|
| `timestamp` | ISO-8601 `.sssZ` UTC | `_now_iso()` |
| `scene_id` | str | `req.scene_id` |
| `provider` | str | provider id (`"ark"`) — **不是** vendor model name |
| `type` | str | `req.asset_type` (`"video"` / `"image"` / `"tts"` / `"bgm"`) |
| `cost_cny` | float | `result.cost_yuan` (实际值, 非估值) |
| `duration_ms` | int | `result.duration_ms` |
| `status` | str | 见 §5 失败模式表 |
| `asset_path` | str \| null | `result.local_path` 或 `result.public_url` |
| `profile_version` | str | `PROFILE_VERSION` 常量 = `"fengge_v1"` |
| `prompt_hash` | str | `fnv1a_64(req.prompt)` → `"fnv64:" + 16 hex chars` |
| `outfit` | str \| null | `req.outfit` (orchestrator 透传) |
| `error` | str \| null | 失败时填 |

`LedgerEntry` dataclass 也有 `consistency_score` 字段, 但 orchestrator **不写** — L2 spec §6 的 IP consistency check 当前没实装, audit 不会校验。

写入路径: `~/.claude/agents/vivify-asset-ledger.jsonl` (`$VIVIFY_LEDGER_PATH` 可覆盖)。Legacy `~/.claude/agents/opc-asset-ledger.jsonl` 首次调用 `ensure_ledger()` 时自动 rename。

读 ledger: `tail -f ~/.claude/agents/vivify-asset-ledger.jsonl | jq .`

## 5. 失败模式处理

| 失败 | 处理 | ledger status | 来源 |
|------|------|---------------|------|
| router 选不到 provider | 写 ledger, 不调 vendor | `pending: no-provider` | `pick()` 返回 `"manual-pending"` |
| cost-cap 拒 (per_asset 或 monthly) | abort, 不调 vendor | `budget-exceeded` | `cost_cap.check_cost_caps` (caller 注入) 或 `_model_cost_yuan > per_asset_cny` |
| adapter `NotImplementedError` (stub) | 写 ledger + return | `provider-failed` | `_build_adapter` 抛 → `_emit_failure` |
| provider 4xx (400 bad_request / 403 auth / 401 key / content_policy) | **不重试** (PERMANENT), 写 ledger + return | `provider-failed` | `classify_error` → `PERMANENT` |
| provider 429 (rate_limit) | retry 3 次 (5s/15s/45s backoff), 中间写 `provider-retry` | 最终 `provider-failed` 或 `success` | `RetryPolicy`, `should_retry` |
| provider 5xx (server_error) | retry 3 次, backoff | `provider-retry` (中间) / `provider-failed` (终) | 同上 |
| provider timeout (>10 分钟) | 写 ledger + return | `timeout` | `_classify_to_status` (ark `_poll_task` 600s cap) |
| 网络错 (ECONNREFUSED / curl exit nonzero) | retry transient | `network-error` (终) / `provider-retry` (中间) | `classify_error` → `network` |
| 视频 URL 24h 后失效 | **必须**在 succeeded 时立即 `out_path` 落盘, 不依赖火山 URL | — | `ArkProvider.generate_video` 下载到本地 |

## 6. 反 AI 检测硬约束 (火山 prompt 必含)

`vivify-panda-character` 推荐的 anti-AI tokens (拼到 prompt 末尾, 用 `, ` 分隔):

```python
ANTI_AI_TOKENS = [
    "不要 AI 生成感",
    "不要塑料皮肤",
    "不要 3D 渲染",
    "不要完美光影",
    "不要对称构图",
    "不要网红脸滤镜",
    "不露爪", "不攻击性姿势",
    "胶片颗粒", "手绘笔触", "留白",
]
```

**绝对禁止**英文 token: `perfect symmetry`, `8K`, `ultra-detailed`, `masterpiece`, `award-winning` — 已被反 AI 系统标记。

> Orchestrator 不强制 prompt 内容 — 这是 caller (`vivify episode render`) 的责任。Orchestrator 只把 caller 给的 `req.prompt` 原样转发给 adapter, ledger 行只记 hash 不记全文 (出于成本 — 完整 prompt 不落盘)。

## 7. Cross-References

- `vivify-asset-router` — 选 provider (本 skill 调它: `from vivify.asset_router import pick`)
- `vivify/providers/ark.py` — Ark adapter (Seedance 视频 + Seedream 图像)
- `vivify/providers/minimax.py` — Hailuo adapter (mmx CLI)
- `vivify/providers/stubs.py` — Kling / Jimeng / Suno / Udio 桩 (NotImplementedError)
- `vivify-panda-character` — 提供 fenggeV1 visual_anchor (caller 嵌进 prompt)
- `vivify-cost-cap` — 月度 cap 检查 (`monthly_hard` 参数 + `db_path`)
- `vivify/asset_ledger.py` — JSONL ledger (`LedgerEntry` dataclass + `append()`)
- `vivify/retry.py` — `RetryPolicy` + `classify_error` + `should_retry`

## 8. Anti-Patterns (orchestrator 层禁)

- ❌ 跳过 ledger 写入 (`_emit_success` / `_emit_failure` 包了 try/except, ledger 写失败不挂 call, 但 caller 审计时 ledger 必空 — audit 必挂)
- ❌ 跳过 `profile_version="fengge_v1"` (直接构造 `LedgerEntry` 时漏写 → audit 失败)
- ❌ 不带 `prompt_hash` 写 ledger (`fnv1a_64(req.prompt)` 必填, 否则无法复现)
- ❌ 视频成功但 24h 后才下载 (`ArkProvider.generate_video` 在 polling 后立刻 `curl GET → out_path`, 不依赖 vendor URL 持久)
- ❌ Bypass orchestrator 直调 vendor (其他 skill 必须通过 `generate_asset()`)
- ❌ 把 anchor 当 `anchor: {...}` 对象传给 adapter — orchestrator 已经把它拼进 `req.prompt` 字符串, adapter 不认 anchor 对象