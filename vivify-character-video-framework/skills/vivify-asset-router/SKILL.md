---
name: vivify-asset-router
description: Use this skill when generating video/image/audio assets for opc — picks the right provider (火山引擎 Ark / 可灵 Kling 3.0 / Suno v5.5) per scene based on vivify-providers.yaml config, scene type, outfit, and budget. Triggers on scene.asset_generation, "生成资产", "调可灵", "调即梦", "调 Suno", or any 峰哥 IP asset request.
---

# vivify-asset-router (v2)

按 scene 特征 + YAML 配置选 provider, 调对应 adapter 跑生成。是 v1 (vivify-asset-generation, hardcoded 模板) 的**真集成**替代。

**YAML 配置位置**: `~/.claude/config/vivify-providers.yaml` (项目级优先 → 全局 fallback)。

## 1. 路由决策

```yaml
# 配置示例 (3 vendor, 5 model, 1 key 起)
default_providers:
  video: volcengine-seedance-2.0
  image: volcengine-jimeng-4.5
  bgm:   suno-v5.5

fallback_chains:
  video: [kling-3-std, kling-3-pro, manual-pending]
  image: [volcengine-jimeng-4.5]   # 唯一
  bgm:   [suno-v5.5]              # 唯一

cost_caps:
  per_video_cny: 50
  per_asset_cny: 5
  monthly_cny: 3000

scene_to_provider:
  video_short_character: volcengine-seedance-2.0
  video_long_narrative:  volcengine-seedance-2.0
  image_cover:          volcengine-jimeng-4.5
  image_keyframe:       volcengine-jimeng-4.5
  bgm:                  suno-v5.5
  tts:                  volcengine-tts
```

## 2. 路由算法

```typescript
// 1. 读 vivify-providers.yaml
const cfg = readConfig('vivify-providers.yaml');

// 2. 给定 scene.type (video|image|bgm|tts):
const sceneType = scene.type;
const defaultKey = `default_providers.${sceneType}`;
let provider = cfg[defaultKey] || sceneToProvider(scene);

// 3. 调对应 adapter
const adapter = require(`./vivify-provider-${provider.split('-')[0]}`);
const result = await adapter.generate({ scene, anchor: fenggeV1 });

// 4. 失败 → fallback chain
if (!result.ok && cfg.fallback_chains[sceneType]) {
  for (const fb of cfg.fallback_chains[sceneType]) {
    const fbAdapter = require(`./vivify-provider-${fb.split('-')[0]}`);
    const r = await fbAdapter.generate({ scene, anchor: fenggeV1 });
    if (r.ok) { result = r; provider = fb; break; }
  }
}

// 5. 写 ledger
appendLedger({ scene_id, provider, cost_cny, status: result.ok ? 'success' : 'fallback-failed' });

// 6. 超 cap → 标记 budget-exceeded
if (currentCost > cfg.cost_caps.per_video_cny) {
  result = { ok: false, status: 'budget-exceeded' };
}
```

## 3. Provider 决策表 (硬规则)

| Scene 特征 | 选 | 原因 |
|-----------|----|------|
| 视频 ≤ 10s + 角色 (峰哥) | 可灵 3.0 std | Kling 角色一致最强 |
| 视频 10-30s + 剧情 | Seedance 2.0 | 多镜头原生 + 音画同步 |
| 视频 国潮服饰 outfit_* | Seedance 2.0 | 中文 prompt + 视觉一致 |
| 图像 9:16 封面 | 即梦 4.5 (Jimeng 4.5/Seedream 3.0) | 中文 SOTA |
| 图像 16:9 关键帧 | 即梦 4.5 | 同上 |
| BGM 治愈/哲学/国潮 | Suno v5.5 | 4.5 版新自定义人声 |
| 配音 中文情感 | 火山 TTS | 国内便宜 |

## 4. 失败回退链

```
volcengine → kling-3-std → kling-3-pro → manual-pending
kling-3 → volcengine → manual
jimeng → manual  # 唯一,失败就人工
suno → udio → manual
```

`manual-pending` = 写 ledger + 标 `pending: manual`, 进入人工队列, **不能自动用其他 provider** (会破坏 IP 一致性)。

## 5. 必含 峰哥 visual_anchor (IP 一致性)

每次调用 provider adapter 之前, **必须**把 `vivify-panda-character` 的 visual_anchor 嵌入 prompt:

```typescript
const fenggeV1 = {
  name: '峰哥',
  species: '成年熊猫',
  body: '头身比 1:1.2 圆胖',
  colors: {
    body_white: 'rgb(245,240,225)',
    eye_black: 'rgb(26,26,26)',
  },
  eyes: '半阖带笑意, 不直视镜头',
  outfits: ['hufu_red', 'changshan_blue', 'workwear_orange', 'robe_green', 'jacket_redwhite'],
  palette: ['朱红#C73E1D', '暖橙#E89B45', '翠绿#3B8C5A', '宝蓝#1F5FA8', '米白#F5F0E1'],
  forbidden: ['露爪', '攻击性', '卖惨', '暗灰', '低饱和', '网红脸'],
};
```

每个 provider adapter 收到 `anchor: fenggeV1` 后, 必须把它翻译成 provider 自己的 prompt 句法 (可灵/即梦/Suno 各自不同)。

## 6. Adapter 接口 (所有 provider 必须实现)

```typescript
interface AssetProvider {
  name: string;                          // 'volcengine-seedance-2.0' 等
  type: 'video' | 'image' | 'audio';
  costPerUnit: { cny: number; unit: string };  // e.g. {cny: 0.5, unit: '秒'}

  generate(params: {
    scene: Scene;                        // 来自 vivify-scene-decomposition
    anchor: typeof fenggeV1;             // 峰哥 visual anchor
    options?: { size?, duration?, style?, seed? };
  }): Promise<{
    ok: boolean;
    asset_id?: string;                   // vendor 侧 ID
    local_path?: string;                 // 下载到 apps/assets/{scene_id}/{tool}.{ext}
    cost_cny: number;
    duration_ms: number;
    error?: string;
  }>;
}
```

## 7. Ledger 写入

每次调用 (成功或失败) 必写 `~/.claude/agents/vivify-asset-ledger.jsonl`:

```jsonl
{"timestamp":"2026-06-08T12:00:00Z","scene_id":"panda-st05-sh02-v1","provider":"volcengine-seedance-2.0","tool":"video","cost_cny":2.5,"duration_ms":45000,"status":"success","asset_path":"apps/assets/panda-st05-sh02-v1/video.mp4"}
```

## 8. Cross-References

- `vivify-panda-character` — 提供 fenggeV1 visual anchor
- `vivify-scene-decomposition` — 提供 scene 输入
- `vivify-cost-cap` — 提供月度 cap 检查 (router 内会调)
- `vivify-provider-volcengine` / `vivify-provider-kling` / `vivify-provider-suno` — adapter 实现

## 9. Anti-Patterns (router 层禁)

- ❌ 跳过 anchor 嵌入 (IP 一致性审计必挂)
- ❌ Fallback 时改换 outfit / scene (破坏 IP 一致性)
- ❌ 写 ledger 失败时 "吞掉错误" — 必须让 caller 知道
- ❌ 跑成功 + cost ledger 都对, 但 visual_anchor 字段缺失 — audit 失败
