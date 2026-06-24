# Error Recovery — What to Tell the User

The vivify CLI surfaces raw errors. **Your job is to translate them**
into action items the user understands.

## 403 / permission_denied

**Raw CLI output:**
```
[pipeline] FATAL: video gen curl failed (HTTP 403):
```

**Tell the user (in Chinese):**
> 视频生成被拒绝了。原因：你当前的 ARK 账号没开通 Seedance 权限。
> 解决：
> 1. 打开 https://console.volcengine.com/ark 开通 Seedance 1.5-pro
> 2. 或换 provider：跑 `--video-provider minimax --video-model MiniMax-Hailuo-2.3`
> 3. 或提供 MiniMax 视频 API key

## quota_exceeded (Token Plan 满)

**Raw CLI output:**
```
{"error":{"code":1,"message":"已达到 Token Plan 用量上限..."}}
```

**Tell the user:**
> MiniMax Token Plan 余额用完。两条路：
> 1. 充值 https://platform.minimaxi.com/ → Token Plan → 升级套餐
> 2. 等下月 1 号自动恢复
> 现在先做不了。

## timeout (MiniMax 单个视频 >5 分钟)

**Raw CLI output:**
```
{"error":{"code":5,"message":"Request timed out"}}
```

**Tell the user:**
> 单个视频生成超时。可能是 MiniMax 服务器临时慢。
> 默认会重试 3 次（5s/15s/45s 退避）。如果还失败：
> - 等几分钟后重跑：`vivify episode render fengge EP005 --retry-only`
> - 或换 provider：`--video-provider ark`

## ARK_API_KEY not set

**Raw CLI output:**
```
[pipeline] FATAL: ARK_API_KEY not set (火山引擎 Ark key — see INSTALL.md)
```

**Tell the user:**
> 没有 ARK_API_KEY 环境变量。请：
> 1. 打开 ~/.bashrc 加：`export ARK_API_KEY="..."`
> 2. 跑 `source ~/.bashrc` 重新加载
> 3. 重跑命令

## Cost cap exceeded

**Raw CLI output:**
```
[cost-cap] ❌ BLOCK — per_video_hard exceeded: ...
```

**Tell the user:**
> 这集预估 ¥X 超过单集硬上限 ¥100。两条路：
> 1. 减时长：`--target-dur 30`（单集从 58s 降到 30s）
> 2. 强制跑：`--force`（不推荐，会刷钱）
> 3. 换便宜模型：`--quality-tier draft`

## ffmpeg not found

**Raw CLI output:**
```
[pipeline] FATAL: ffmpeg not found at /root/.openclaw/.../ffmpeg
```

**Tell the user:**
> ffmpeg 没装或路径不对。两条路：
> 1. `apt install ffmpeg`
> 2. `export FFMPEG=/path/to/ffmpeg`

## storyboard parse failed

**Raw CLI output:**
```
no shots parsed from STORYBOARD.md
```

**Tell the user:**
> 故事板格式不对。应该长这样：
> ```
> ## 镜头 1 — 黑屏白字 (钩子) [0:00-0:03]
> 可灵 prompt: ...
> ```
> 检查格式是不是 `## 镜头 N — 标题 [m1:s1-m2:s2]`，每个镜头都要有 `可灵 prompt:` 行。
