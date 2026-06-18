# Panda EP#001 — Render (L2 视频成品)

> **58 秒抖音版,Seedream 4.0 (图) + Seedance 1.5-pro (视频) + ffmpeg 拼接**

## 文件

- `panda-ep001-final.mp4` — **完整 58s 视频** (8.6 MB, 720p 9:16, 24fps, AAC audio)
- `shot-01.jpg` ... `shot-09.jpg` — 6 张原图 (shot 1/6/8 是黑屏文字,没图)

## 渲染时间

- 6 张图 (Seedream 4.0): ~30s (并行)
- 7 段视频 (Seedance 1.5-pro i2v): ~6 min (串行,每段 ~50s)
- 拼接 (ffmpeg): ~30s
- **总**: ~7 min (包括模型排队时间)

## 成本 (估算)

- 6 张图 @ Seedream 4.0: ¥0.10-0.30
- 7 段 5-8s 视频 @ Seedance 1.5-pro 720p: ¥7-15
- **单条总**: ~¥10-18

对比 EP#001 STORYBOARD 估算 (¥15-32),落在下限,因为我们用 720p 不是 1080p,且 5-8s 镜头没拉满 12s。

## 流程

1. **L1 (script + storyboard)** — `SCRIPT-douyin.md` + `STORYBOARD.md`
2. **L2a (image)** — `panda IP bible` prompt (含毛色 RGB / outfit ID / 场景白名单 / panda_visual_anchor) → Seedream 4.0
3. **L2b (image → video)** — Seedance 1.5-pro i2v, prompt = 镜头动作
4. **L2c (post)** — ffmpeg 黑屏文字层 + 字幕 + 时长拉伸 (shot 7: 8s → 12s)
5. **L3 (todo)** — 配 Suno 旁白 / 剪映 BGM / 抖音发布

## 与原 storyboard 差异

| Shot | 原计划 | 实际渲染 | 差异 |
|---|---|---|---|
| 1 钩子 | 黑屏文字 3s | 黑屏文字 3s | ✅ |
| 2 背影 | 5s 慢推 | 5s 慢推 | ✅ |
| 3 侧脸 | 6s 静态 | 6s 静态 | ✅ |
| 4 正面 | 6s 慢推 | 6s 慢推 | ✅ |
| 5 抬头 | 8s 焦平面 | 8s 焦平面 | ✅ |
| **6 评论区** | 手机抖音录屏 8s | **黑屏占位 8s** | ❌ 需剪映录屏 |
| 7 拉镜 | 12s 拉镜 | **8s + 末帧定格 4s = 12s** | ⚠ 动作不够"拉" |
| 8 共鸣 | 黑屏文字淡入 7s | 黑屏文字淡入 7s | ✅ |
| 9 收尾 | 背影 3s | 背影 3s + 字幕 | ✅ |

**未做**: 配音 (Suno/海螺 TTS) + BGM (剪映钢琴) + 抖音发布。

## 下一步

1. **配音**: MiniMax 海螺 TTS (我有 key),慢速男声 ~30s 一段
2. **BGM**: 剪映库"深夜钢琴" 60 BPM + 雨声白噪
3. **Shot 6 评论区**: 剪映录屏模板 (无需 API)
4. **抖音发布**: 需抖音开放平台 API key

跑完这 4 步 = 完整 L3 episode,可发抖音。
