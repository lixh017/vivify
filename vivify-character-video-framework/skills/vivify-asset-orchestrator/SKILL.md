---
name: vivify-asset-orchestrator
description: Use this skill when opc needs to generate real assets — coordinates router → provider → ledger → cost-cap → IP consistency check. Reads `vivify-asset-router` for routing, calls `vivify-provider-volcengine` (or future providers) for execution, writes `~/.claude/agents/vivify-asset-ledger.jsonl`. Triggers on "opc 生成资产", "跑 asset pipeline", "调 router", or any opc asset generation request.
---

# vivify-asset-orchestrator (v2)

协调 vivify-asset-router + provider adapter + cost-cap + ledger。**唯一可以调 provider 的入口**。其他 skill 不直接调 vendor。

## 1. 输入 (来自 caller)

```typescript
{
  scene_id: 'panda-st05-sh02-v1',              // 来自 vivify-scene-decomposition
  type: 'video' | 'image' | 'bgm' | 'tts',
  scene: { /* Scene 来自 vivify-scene-decomposition */ },
  outfit: 'hufu_red' | 'changshan_blue' | ...,  // 5 套服饰之一
  tone: 'guofeng' | 'healing' | 'philosophy' | 'otaku',
  options: { size?, duration?, ratio?, seed? },
}
```

## 2. 流程 (伪代码)

```typescript
import { router } from 'vivify-asset-router';
import { volcengine } from 'vivify-provider-volcengine';
import { costCap } from 'vivify-cost-cap';

async function orchestrateAsset(req) {
  // 1. Router 选 provider
  const provider = router.pick({ type: req.type, tone: req.tone, options: req.options });
  
  // 2. Cost cap 闸门
  const cost = costCap.checkAndReserve({ provider, scene: req.scene });
  if (!cost.ok) return { ok: false, status: 'budget-exceeded', cost: cost.estimated };
  
  // 3. Build prompt (必嵌 fenggeV1 anchor)
  const prompt = buildPrompt(req);  // 调用 vivify-panda-character 的 visual_anchor
  
  // 4. 调 provider adapter
  const result = await provider.generate({ prompt, ...req.options, anchor: fenggeV1 });
  
  // 5. 验 IP 一致性 (粗)
  const consistency = await checkConsistency(result, fenggeV1);
  if (!consistency.pass) {
    logLedger({ ...req, status: 'consistency-failed', score: consistency.score });
    return { ok: false, status: 'consistency-failed' };
  }
  
  // 6. 落盘到 apps/assets/{scene_id}/
  const localPath = await download(result.url, `apps/assets/${req.scene_id}/${provider.name}.${result.ext}`);
  
  // 7. 写 ledger
  logLedger({ ...req, status: 'success', asset_path: localPath, cost_cny: cost.actual });
  
  return { ok: true, asset_path: localPath, provider: provider.name, cost_cny: cost.actual };
}
```

## 3. fenggeV1 anchor 嵌入 (必嵌)

```typescript
const fenggeV1 = {
  name: '峰哥',
  species: '成年熊猫',
  body: '头身比 1:1.2 圆胖身材',
  colors: { body_white: 'rgb(245,240,225)', eye_black: 'rgb(26,26,26)' },
  eyes: '半阖带笑意, 不直视镜头',
  palette: ['朱红#C73E1D', '暖橙#E89B45', '翠绿#3B8C5A', '宝蓝#1F5FA8', '米白#F5F0E1'],
  forbidden: ['露爪', '攻击性', '卖惨', '暗灰', '低饱和', '网红脸', '完美对称', '8K', 'ultra-detailed', 'masterpiece'],
};

function buildPrompt(req) {
  // vivify-panda-character §5 的模板 + 场景 + outfit + 反 AI 检测
  return `
一只成年熊猫(峰哥), 头身比 1:1.2 圆胖身材,
面部白 rgb(245,240,225) 眼周黑 rgb(26,26,26),
眼神半阖带笑意不直视镜头,
身穿${outfitFor(req.outfit)},
${req.scene.assets.map(a => `手持${a}`).join('\n')},
${req.scene.location}${req.scene.time},
${req.scene.atmosphere},
**国潮 + 色彩靓丽**(朱红/暖橙/翠绿/宝蓝撞色, 非暗色调、非水墨),
不露爪, 不攻击性姿势, 无文字
负面: 不要 AI 生成感, 不要塑料皮肤, 不要 3D 渲染, 不要完美光影, 不要对称构图, 不要网红脸滤镜
`.trim();
}
```

## 4. Ledger 格式 (JSONL append-only)

```json
{"timestamp":"2026-06-08T14:30:00Z","scene_id":"panda-st05-sh02-v1","provider":"volcengine-seedance-1-5-pro-251215","type":"video","cost_cny":7.5,"duration_ms":45000,"status":"success","asset_path":"apps/assets/panda-st05-sh02-v1/video.mp4","anchor_version":"fengge_v1","prompt_hash":"sha256:abc...","consistency_score":0.92}
```

写入路径: `~/.claude/agents/vivify-asset-ledger.jsonl`

读 ledger: `tail -f ~/.claude/agents/vivify-asset-ledger.jsonl | jq .`

## 5. 失败模式处理

| 失败 | 处理 | ledger 状态 |
|------|------|-------------|
| router 选不到 provider | mark `pending: manual`, 写 ledger | `pending: no-provider` |
| cost-cap 拒 | abort, 不调 provider | `budget-exceeded` |
| provider 4xx (e.g. 400 size 过小) | router 改 provider 再试 (1 次) | `provider-failed` |
| provider 5xx | retry 3 次 with backoff (5s, 15s, 45s) | `provider-retry` |
| provider timeout (10 分钟) | 标 `pending: manual` | `timeout` |
| 一致性 check fail | 标 `consistency-failed`, 落盘到 `apps/assets/FAILED/` | `consistency-failed` |
| 视频 URL 24h 后失效 | **必须**在 succeeded 时立即落盘, 不依赖火山 URL | — |
| 网络错 (ECONNREFUSED) | mark `pending: network` | `network-error` |

## 6. 反 AI 检测硬约束 (火山 prompt 必含)

```typescript
const ANTI_AI_TOKENS = [
  '不要 AI 生成感',
  '不要塑料皮肤',
  '不要 3D 渲染',
  '不要完美光影',
  '不要对称构图',
  '不要网红脸滤镜',
  '不露爪', '不攻击性姿势',
  '胶片颗粒', '手绘笔触', '留白',
];
// 拼到 prompt 末尾, 用 `.` 分隔
```

**绝对禁止**英文 token: `perfect symmetry`, `8K`, `ultra-detailed`, `masterpiece`, `award-winning` — 已被反 AI 系统标记。

## 7. Cross-References

- `vivify-asset-router` — 选 provider (本 skill 调它)
- `vivify-provider-volcengine` — 调真 API (本 skill 调它)
- `vivify-panda-character` — 提供 fenggeV1 visual_anchor
- `vivify-cost-cap` — budget check
- `vivify-scene-decomposition` — 提供 scene 输入
- `vivify-provider-registry.md` — model ID + endpoint 唯一来源

## 8. Anti-Patterns (orchestrator 层禁)

- ❌ 跳过 fenggeV1 anchor 嵌入
- ❌ 跳过 ledger 写入 (哪怕失败也要写)
- ❌ 视频成功但 24h 后才落盘
- ❌ 偷偷把 outfit 改成 "默认" (破坏 IP 一致性)
- ❌ 不带 anchor_version 写 ledger (审计必挂)
- ❌ 跨 ledger 写没 SHA256 的 prompt_hash (审计无法复现)
