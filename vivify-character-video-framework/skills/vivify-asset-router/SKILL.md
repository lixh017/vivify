---
name: vivify-asset-router
description: Use this skill when generating video/image/audio assets for opc — picks the right provider (火山方舟 Ark / 海螺 Hailuo / Kling / Jimeng / Suno / Udio) per scene based on vivify-providers.yaml config, scene type, outfit, and budget. Triggers on scene.asset_generation, "生成资产", "调可灵", "调即梦", "调 Suno", or any 峰哥 IP asset request.
---

# vivify-asset-router

> Verified against commit cb9a7e8. If you find drift, file an issue.

按 scene 特征 + YAML 配置选 provider, 调对应 adapter 跑生成。是 v1 (vivify-asset-generation, hardcoded 模板) 的**真集成**替代。

**YAML 配置位置**: `~/.claude/config/vivify-providers.yaml` (项目级 `./vivify-providers.yaml` 优先 → 全局 fallback)。文件不存在时, 首次调用 `vivify asset router` 会自动生成默认配置。

## 1. 路由决策

```yaml
# ~/.claude/config/vivify-providers.yaml (默认内容, auto-generated)
default_providers:
  video: ark
  image: ark
  tts:   ark        # placeholder — 火山 TTS adapter 未实装, 当前仍走 render_episode.gen_tts (海螺)
  bgm:   suno       # stub — SunoProvider 今日抛 NotImplementedError

fallback_chains:
  video: [ark, minimax, kling, manual-pending]
  image: [ark, jimeng, manual-pending]
  tts:   [ark, manual-pending]
  bgm:   [suno, udio, manual-pending]

cost_caps:
  per_video_cny: 50      # soft cap; orchestrator 警告, 可强制覆盖
  per_asset_cny: 5       # hard cap for any single asset
  monthly_cny: 60000     # hard cap per calendar month (Phase 1 budget)

scene_overrides: {}      # optional, by scene category (e.g. 'video_short_character')

default_models:
  ark:     doubao-seedance-1-0-pro-fast-251015  # 0.21 ¥/s
  minimax: MiniMax-Hailuo-2.3-Fast
  kling:   doubao-seedance-1-0-lite-i2v-250428  # kling 今日未进 MODEL_CATALOG, 走 catalog scan fallback
```

路由 pick 顺序 (`vivify.asset_router.pick`):

1. `scene_overrides[scene_category]` — 若 `requirements["scene_category"]` 在 overrides 字典里
2. `default_providers[asset_type]` — 默认, 但需通过 cap / filter 检查
3. `fallback_chains[asset_type]` — 顺序遍历, 跳过被 exclude 或超 cap 的
4. 全部拒 → 返回 `"manual-pending"` (sentinel, 不是真 provider)

## 2. 路由算法 (Python, vivify.asset_router.pick)

```python
from vivify.asset_router import RouterConfig, load_config, pick

cfg = load_config()                      # ./vivify-providers.yaml > ~/.claude/config/vivify-providers.yaml
provider_id = pick(
    cfg,
    asset_type="video",                  # AssetType literal: video|image|tts|bgm
    requirements={"duration_sec": 5,
                 "outfit": "hufu_red",
                 "tone": "国潮"},
    exclude=[],                          # 可选: 黑名单
    provider_filter=None,                # 可选: "ark" 强制只用 ark
)

# provider_id ∈ {"ark", "minimax", "kling", "jimeng", "suno", "udio", "manual-pending"}
```

`_acceptable(candidate)` 检查三项:

- 不在 `exclude` 里
- 通过 `provider_filter` (filter 是 None 则放行所有, 是 sentinel 也放行)
- 通过 `per_asset_cny` cap — 解析 `provider_id → model_name` (经 `config.default_models` 或 catalog prefix scan), 再查 `MODEL_CATALOG.cost_per_sec_yuan × duration_sec`

任何一项 fail 即跳过该候选, 链上下一个。`manual-pending` 是 sentinel — 永远视为可接受, 走到它就停 (不自动用其他 provider)。

## 3. Provider 决策表 (硬规则)

| Scene 特征 | 选 | 原因 |
|-----------|----|------|
| 视频 ≤ 10s + 角色 (峰哥) | 可灵 3.0 / Hailuo S2V | 角色锁定 (subject-image) |
| 视频 10-30s + 剧情 | Seedance 1.5-pro / 2.0 (ark) | 多镜头原生 + 音画同步 |
| 视频 国潮服饰 outfit_* | ark (Seedance) | 中文 prompt + 视觉一致 |
| 图像 9:16 / 16:9 | ark (Seedream 4.0) | 中文 SOTA |
| BGM 治愈/哲学/国潮 | Suno v5.5 (stub) | 4.5 版新自定义人声 |
| 配音 中文情感 | 海螺 speech-02-hd (render_episode.gen_tts) | 当前唯一通路 |

## 4. 失败回退链

```text
video: ark    → minimax → kling      → manual-pending
image: ark    → jimeng                → manual-pending
tts:   ark    → manual-pending
bgm:   suno   → udio                  → manual-pending
```

`manual-pending` = 写 ledger + 标 `pending: no-provider`, 进入人工队列, **不能自动用其他 provider** (会破坏 IP 一致性)。Stubs (`suno`, `udio`, `jimeng`, `kling`) 当前抛 `NotImplementedError`, orchestrator 视作 `provider-failed`, 继续沿链 walk。

## 5. 必含 峰哥 visual_anchor (IP 一致性)

每次调用 provider adapter 之前, **必须**把 `vivify-panda-character` 的 visual_anchor 嵌入 prompt。Orchestrator 通过 `request.outfit` + `request.tone` 字段把 anchor 上下文传给 adapter; ledger 行通过 `profile_version: "fengge_v1"` 标记 anchor 版本 (见 `vivify.asset_orchestrator.PROFILE_VERSION`)。每个 adapter 收到 anchor 上下文后, 自己把它翻译成 provider 自己的 prompt 句法 (Seedance / Seedream / mmx 各自不同)。

> ⚠️ 注意: anchor 不是作为 `anchor: {...}` 对象传给 adapter — orchestrator 已经在 `GenerateRequest.prompt` 里把 anchor 拼好, adapter 拿到的是完整 prompt 字符串。`outfit` 和 `tone` 是 `GenerateRequest` 上的独立可选字段, adapter 自行选用。

## 6. ProviderAdapter 接口 (Python ABC, vivify/providers/base.py)

```python
from abc import ABC, abstractmethod
from pathlib import Path

AssetType = Literal["video", "image", "tts", "bgm"]

@dataclass
class GenerateRequest:
    scene_id: str
    asset_type: AssetType
    prompt: str                         # 已嵌 anchor 的完整 prompt
    reference_image: Optional[str] = None   # local path / http(s):// / data: URI
    duration_sec: Optional[int] = None      # video only
    size: Optional[str] = None              # image only, e.g. "1024x1792"
    outfit: Optional[str] = None            # IP anchor 字段
    tone: Optional[str] = None              # IP anchor 字段
    options: dict = field(default_factory=dict)

@dataclass
class GenerateResult:
    ok: bool
    asset_id: Optional[str] = None          # vendor task id
    local_path: Optional[Path] = None       # 下载到本地的文件
    public_url: Optional[str] = None        # vendor URL (video 24h TTL)
    cost_yuan: float = 0.0
    duration_ms: int = 0
    provider: str = ""                      # "ark" | "minimax" | ...
    model: str = ""                         # e.g. "doubao-seedance-1-5-pro-251215"
    error: Optional[str] = None
    status_hint: Optional[str] = None       # orchestrator 设置: budget-exceeded 等

class ProviderAdapter(ABC):
    name: str
    asset_type: AssetType

    @abstractmethod
    def generate(self, req: GenerateRequest) -> GenerateResult: ...

    @abstractmethod
    def cost_estimate(self, req: GenerateRequest) -> float: ...
```

Adapters (`vivify/providers/`): `ArkProvider`, `MiniMaxProvider`, `KlingProvider`, `JimengProvider`, `SunoProvider`, `UdioProvider` — 后四个是 `vivify/providers/stubs.py` 里的 `NotImplementedError` 桩。

## 7. Ledger 写入

每次调用 (成功或失败, 含 retry 中间态) 必写 `~/.claude/agents/vivify-asset-ledger.jsonl`。字段见 `vivify/asset_ledger.py` `LedgerEntry` (实际行 — 不含 `consistency_score` 填充, orchestrator 不算 consistency):

```jsonl
{"timestamp":"2026-06-24T10:00:00.123Z","scene_id":"panda-st05-sh02-v1","provider":"ark","type":"video","cost_cny":1.05,"duration_ms":45000,"status":"success","asset_path":"/tmp/out/panda-st05-sh02-v1.mp4","profile_version":"fengge_v1","prompt_hash":"fnv64:0123456789abcdef","outfit":"hufu_red"}
```

注意:

- `provider` = provider id (`"ark"`), 不是 vendor model name (`"doubao-seedance-..."`) — model 单独存 `LedgerEntry` 不收, 见 `GenerateResult.model` (orchestrator 路径上未写 ledger, 是已知 gap)。
- `profile_version` 默认 `"fengge_v1"`, 由 `vivify.asset_orchestrator.PROFILE_VERSION` 注入。
- `prompt_hash` 用 `fnv1a_64(req.prompt)` → `"fnv64:" + 16 hex chars`。
- `consistency_score` 字段 dataclass 里有定义, 但 orchestrator 不写 — L2 spec §3 的 consistency check 当前没实装。

## 8. Cross-References

- `vivify-panda-character` — 提供 visual_anchor (orchestrator 嵌进 prompt)
- `vivify-scene-decomposition` — 提供 scene_category (进 router requirements)
- `vivify-cost-cap` — 月度 / 单次硬 cap (orchestrator 在 adapter call 前调)
- `vivify-asset-orchestrator` — 唯一直接调 provider 的入口
- `vivify/providers/ark.py` / `minimax.py` / `stubs.py` — adapter 实现

## 9. Anti-Patterns (router 层禁)

- ❌ 跳过 anchor 嵌入 (orchestrator 层负责, 但 router 不要绕过 orchestrator 直调 adapter)
- ❌ Fallback 时改换 outfit / scene (破坏 IP 一致性)
- ❌ 写 ledger 失败时 "吞掉错误" — orchestrator 走 try/except 兜底, router 不直接写 ledger
- ❌ 跑成功 + cost ledger 都对, 但 `profile_version` 字段缺失 — audit 失败