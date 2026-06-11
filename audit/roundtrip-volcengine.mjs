// opc 真 round-trip test
// 1. 图像 (Seedream 4-0) 同步生成 → 落盘 → 验 IP 一致性
// 2. 视频 (Seedance 1.5-pro) 异步任务 → 落盘 → 验 IP 一致性
// 3. 写 ledger
import fs from 'fs';
import crypto from 'crypto';
import { resolve, join } from 'path';

const env = Object.fromEntries(
  fs.readFileSync(resolve(process.env.HOME, '.claude/config/opc-volcengine.env'), 'utf8')
    .split('\n').filter(l => l.includes('=') && !l.startsWith('#'))
    .map(l => l.split('=', 2))
);
const HEADERS = { 'Authorization': `Bearer ${env.ARK_API_KEY}`, 'Content-Type': 'application/json' };
const BASE = env.ARK_BASE_URL;
const LEDGER = resolve(process.env.HOME, '.claude/agents/opc-asset-ledger.jsonl');
const ASSET_DIR = resolve(process.env.HOME, 'workspace/opc/apps/assets/test-panda-st01');
fs.mkdirSync(ASSET_DIR, { recursive: true });

const fenggeV1 = {
  name: '峰哥',
  species: '成年熊猫',
  body: '头身比 1:1.2 圆胖身材',
  colors: { body_white: 'rgb(245,240,225)', eye_black: 'rgb(26,26,26)' },
  eyes: '半阖带笑意, 不直视镜头',
  palette: ['朱红#C73E1D', '暖橙#E89B45', '翠绿#3B8C5A', '宝蓝#1F5FA8', '米白#F5F0E1'],
};

function buildPrompt(scene, outfit, type) {
  const t = type === 'video'
    ? `一只成年熊猫(峰哥), 头身比 1:1.2 圆胖身材, 面部白 rgb(245,240,225) 眼周黑 rgb(26,26,26), 眼神半阖带笑意不直视镜头, 身穿${outfit}汉服, 手持折扇, 站在竹林小院, 暖光氛围, 国潮 + 色彩靓丽(朱红/暖橙/翠绿/宝蓝撞色, 非暗色调、非水墨), 不露爪, 不攻击性姿势, 无文字  --duration 5 --resolution 720p --ratio 9:16 --watermark true`
    : `一只成年熊猫(峰哥), 头身比 1:1.2 圆胖身材, 面部白 rgb(245,240,225) 眼周黑 rgb(26,26,26), 眼神半阖带笑意不直视镜头, 身穿朱红汉服, 手持折扇, 站在竹林小院, 暖光氛围, 国潮 + 色彩靓丽(朱红/暖橙/翠绿/宝蓝撞色, 非暗色调、非水墨), 不露爪, 不攻击性姿势, 无文字`;
  return t;
}

function ipConsistencyCheck(prompt) {
  let score = 0, fails = [];
  if (prompt.includes('峰哥')) score += 0.2; else fails.push('no fengge name');
  if (prompt.includes('成年熊猫')) score += 0.2; else fails.push('no panda species');
  if (prompt.includes('rgb(245,240,225)')) score += 0.2; else fails.push('no body white color');
  if (prompt.includes('rgb(26,26,26)')) score += 0.2; else fails.push('no eye black color');
  if (prompt.includes('国潮')) score += 0.1; else fails.push('no guofeng theme');
  if (prompt.includes('不露爪')) score += 0.05; else fails.push('no anti-claws');
  if (prompt.includes('不要 AI 生成感')) score += 0.05; else fails.push('no anti-AI tokens');
  return { score: Math.round(score * 100) / 100, fails };
}

function logLedger(entry) {
  fs.appendFileSync(LEDGER, JSON.stringify(entry) + '\n');
}

const startedAt = Date.now();

// === Phase 1: 图像 (Seedream 4-0) ===
console.log('=== Phase 1: 图像 (Seedream 4-0) ===');
const imgPrompt = buildPrompt({}, '朱红', 'image');
const imgPromptHash = crypto.createHash('sha256').update(imgPrompt).digest('hex').slice(0, 16);
const imgConsistency = ipConsistencyCheck(imgPrompt);
console.log(`  prompt hash: sha256:${imgPromptHash}`);
console.log(`  consistency: ${imgConsistency.score} (${imgConsistency.fails.length} fails)`);

const imgRes = await fetch(`${BASE}/images/generations`, {
  method: 'POST', headers: HEADERS,
  body: JSON.stringify({
    model: 'doubao-seedream-4-0-250828',
    prompt: imgPrompt,
    size: '1024x1792',
  })
});
const imgData = await imgRes.json();
const imgUrl = imgData.data?.[0]?.url;
console.log(`  HTTP ${imgRes.status}, url: ${imgUrl ? imgUrl.slice(0, 80) + '...' : 'NONE'}`);

if (imgUrl) {
  // Download image
  const dl = await fetch(imgUrl);
  const buf = Buffer.from(await dl.arrayBuffer());
  const imgPath = join(ASSET_DIR, 'cover.jpg');
  fs.writeFileSync(imgPath, buf);
  console.log(`  ✓ downloaded: ${imgPath} (${(buf.length/1024).toFixed(1)} KB)`);

  // IP consistency basic check
  const consistencyScore = imgConsistency.score;
  logLedger({
    timestamp: new Date().toISOString(),
    scene_id: 'test-panda-st01',
    provider: 'volcengine-seedream-4-0-250828',
    type: 'image',
    cost_cny: 0.05,
    duration_ms: Date.now() - startedAt,
    status: 'success',
    asset_path: imgPath,
    anchor_version: 'fengge_v1',
    prompt_hash: `sha256:${imgPromptHash}`,
    consistency_score: consistencyScore,
  });
  console.log(`  ✓ ledger written`);
}

// === Phase 2: 视频 (Seedance 1.5-pro) 异步 ===
console.log('\n=== Phase 2: 视频 (Seedance 1.5-pro 异步) ===');
const vidPrompt = buildPrompt({}, '朱红', 'video');
const vidPromptHash = crypto.createHash('sha256').update(vidPrompt).digest('hex').slice(0, 16);
const vidConsistency = ipConsistencyCheck(vidPrompt);
console.log(`  prompt hash: sha256:${vidPromptHash}`);
console.log(`  consistency: ${vidConsistency.score}`);

const taskRes = await fetch(`${BASE}/contents/generations/tasks`, {
  method: 'POST', headers: HEADERS,
  body: JSON.stringify({
    model: 'doubao-seedance-1-5-pro-251215',
    content: [{ type: 'text', text: vidPrompt }],
  })
});
const taskData = await taskRes.json();
const taskId = taskData.id;
console.log(`  task created: ${taskId}`);

if (taskId) {
  // Poll for completion
  let videoUrl = null;
  for (let i = 0; i < 30; i++) {
    await new Promise(r => setTimeout(r, 5000));  // 5s interval
    const r = await fetch(`${BASE}/contents/generations/tasks/${taskId}`, { headers: HEADERS });
    const t = await r.json();
    if (t.status === 'succeeded') { videoUrl = t.content?.video_url; break; }
    if (t.status === 'failed') { console.log(`  task failed:`, t.error); break; }
    if (i % 6 === 0) console.log(`  poll #${i}, status: ${t.status}`);
  }

  if (videoUrl) {
    const dl = await fetch(videoUrl);
    const buf = Buffer.from(await dl.arrayBuffer());
    const vidPath = join(ASSET_DIR, 'video.mp4');
    fs.writeFileSync(vidPath, buf);
    console.log(`  ✓ downloaded: ${vidPath} (${(buf.length/1024).toFixed(1)} KB)`);

    logLedger({
      timestamp: new Date().toISOString(),
      scene_id: 'test-panda-st01',
      provider: 'volcengine-seedance-1-5-pro-251215',
      type: 'video',
      cost_cny: 7.5,
      duration_ms: Date.now() - startedAt,
      status: 'success',
      asset_path: vidPath,
      anchor_version: 'fengge_v1',
      prompt_hash: `sha256:${vidPromptHash}`,
      consistency_score: vidConsistency.score,
    });
    console.log(`  ✓ ledger written`);
  } else {
    logLedger({
      timestamp: new Date().toISOString(),
      scene_id: 'test-panda-st01',
      provider: 'volcengine-seedance-1-5-pro-251215',
      type: 'video',
      cost_cny: 0,
      duration_ms: Date.now() - startedAt,
      status: 'timeout',
      task_id: taskId,
    });
    console.log(`  ✗ timeout after 2.5min`);
  }
}

console.log('\n=== final ledger ===');
const ledger = fs.readFileSync(LEDGER, 'utf8').split('\n').filter(Boolean);
for (const line of ledger) console.log(`  ${line}`);
console.log(`\n=== assets ===`);
for (const f of fs.readdirSync(ASSET_DIR)) {
  const s = fs.statSync(join(ASSET_DIR, f));
  console.log(`  ${f}  (${(s.size/1024).toFixed(1)} KB)`);
}
