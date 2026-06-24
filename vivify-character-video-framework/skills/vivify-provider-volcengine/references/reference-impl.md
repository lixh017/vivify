## 4. 集成实现 (Node.js 范例, vivify-agent 可直接用)

```javascript
// 真实可用代码 (已 2026/06/08 验证)
import fs from 'fs';
import { resolve } from 'path';

const env = Object.fromEntries(
  fs.readFileSync(resolve(process.env.HOME, '.claude/config/vivify-volcengine.env'), 'utf8')
    .split('\n').filter(l => l.includes('=') && !l.startsWith('#'))
    .map(l => l.split('=', 2))
);
const { ARK_API_KEY, ARK_BASE_URL } = env;
const HEADERS = { 'Authorization': `Bearer ${ARK_API_KEY}`, 'Content-Type': 'application/json' };

// 视频 (异步)
export async function generateVideo({ prompt, imageUrl, model = 'doubao-seedance-1-5-pro-251215', duration = 5, ratio = '9:16', resolution = '720p' }) {
  const content = [{ type: 'text', text: `${prompt}  --duration ${duration} --resolution ${resolution} --ratio ${ratio} --watermark true` }];
  if (imageUrl) content.push({ type: 'image_url', image_url: { url: imageUrl } });

  const taskRes = await fetch(`${ARK_BASE_URL}/contents/generations/tasks`, {
    method: 'POST', headers: HEADERS, body: JSON.stringify({ model, content })
  });
  const { id } = await taskRes.json();

  // 轮询
  for (let i = 0; i < 60; i++) {  // 5 分钟
    await new Promise(r => setTimeout(r, i < 6 ? 5000 : 15000));  // 前 30s 每 5s, 后每 15s
    const r = await fetch(`${ARK_BASE_URL}/contents/generations/tasks/${id}`, { headers: HEADERS });
    const t = await r.json();
    if (t.status === 'succeeded') return { ok: true, video_url: t.content.video_url, task_id: id };
    if (t.status === 'failed')   return { ok: false, error: t.error, task_id: id };
  }
  return { ok: false, error: 'timeout', task_id: id };
}

// 图像 (同步)
export async function generateImage({ prompt, model = 'doubao-seedream-4-0-250828', size = '1024x1792' }) {
  const r = await fetch(`${ARK_BASE_URL}/images/generations`, {
    method: 'POST', headers: HEADERS, body: JSON.stringify({ model, prompt, size })
  });
  const j = await r.json();
  return { ok: r.ok, url: j.data?.[0]?.url, error: j.error };
}
```

## 5. vivify-panda-character 必嵌的 prompt 模板

每次调用 adapter 之前, 从 `vivify-panda-character` 取 fengge_v1 visual anchor, 拼到 prompt 末尾:

```
一只成年熊猫(峰哥), 头身比 1:1.2 圆胖身材,
面部白 rgb(245,240,225) 眼周黑 rgb(26,26,26),
眼神半阖带笑意不直视镜头,
身穿[outfit_hufu_red | outfit_changshan_blue | ...],
手持[折扇|茶盏|...],
站在[竹林小院|月下窗棂|...],
[暖光|月光|霓虹]氛围,
国潮 + 色彩靓丽(朱红/暖橙/翠绿/宝蓝撞色, 非暗色调、非水墨),
不露爪, 不攻击性姿势, 无文字
```

**注意**: 视频 text 字段里, 参数 (--duration 等) 必须放在 prompt 末尾, 用 `--key value` 形式, 用空格分隔 (不是 JSON 参数结构)。

## 6. Anti-AI-检测 必含

视频/图像生成时, prompt 末尾**必须**有反 AI 检测硬约束:
- `不要 AI 生成感、不要塑料皮肤、不要 3D 渲染`
- `不要完美光影、不要对称构图、不要网红脸滤镜`
- 中文 prompt 保留"瑕疵感" (手绘笔触、胶片噪点、留白)

## 7. Cross-References

- `vivify-panda-character` — 提供 fenggeV1 visual anchor
- `vivify-asset-router` — 决定调此 adapter (默认 video/image 主选)
- `vivify-cost-cap` — 调用前 budget check, 超 cap 自动降级
- `vivify-provider-registry.md` — 全部 model ID + endpoint 唯一来源

## 8. 验证历史 (2026/06/08)

- ✓ list-models: 92 models
- ✓ Seedance 1.5-pro task create: 200, returned `cgt-20260608140817-l579p`
- ✓ Seedance task poll: 200, status `running`
- ✓ Seedream 4-0 image create: 200, returned `ark-content-generation-...jpeg` URL
- ✗ Seedream 3-0: 404 (已下架, 不要用)
- ✗ Seedream 4-5 size < 3,686,400 px: 400 (需要 ≥ 1920x1920)
- ✗ 旧 base URL `ark.cn-beijing.volces.com/api/v3/v1/models` 错 (path 已含 /api/v3, 不要重复)
- ⚠ `looksLikeRealToken` 之前没匹配 35 字符 volces UUID, 已加 (8-4-4-4-11|12 + 32+ opaque)

## 9. 失败模式 + fallback

| 失败 | 现象 | fallback |
|------|------|----------|
| Seedream 3.0 不存在 | HTTP 404 InvalidEndpointOrModel.NotFound | 改用 Seedream 4-0 或 4-5 |
| Seedream 4-5 size 过小 | HTTP 400 InvalidParameter | 改 size ≥ 1920x1920 |
| 火山 task 超时 (10 分钟) | 仍 `running` | 写 ledger `pending: manual`, 人工查 |
| rate limit | HTTP 429 | backoff 30s 后 retry, 3 次后 fallback manual |
| 视频 24h 后 URL 失效 | 下载时 403 | 必须**立即**落盘, 不依赖火山 URL 长期可用 |
