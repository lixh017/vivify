# Panda EP#001 — Render (L2 + L3 视频成品)

> **58 秒抖音版**,完整 L2 渲染 + L3 后期(配音+BGM+评论区+字幕+合成)

## 文件

| 文件 | 大小 | 用途 |
|---|---|---|
| `panda-ep001-L2.mp4` | 8.6 MB | L2 渲染版(无配音BGM字幕) |
| `panda-ep001-L3.mp4` | 9.3 MB | **完整 L3 版**(配音+BGM+评论区) |
| `shot-01..09.jpg` | ~500KB ea | 6 张原图 |

## L2 (image → video) — 7 分钟

- 6 张图 @ Seedream 4.0:¥0.10-0.30
- 7 段 i2v @ Seedance 1.5-pro 720p:¥7-15
- ffmpeg 黑屏文字卡 + 拼成 58s
- **总**:**~¥10-18 / 7 分钟**

## L3 (post-production) — 全自动,5 分钟

不需要剪映/抖音 API,**全部用 ffmpeg + MiniMax 海螺 TTS**:

1. **TTS 配音** (海螺 speech-02-hd,慢速男声 0.82x):
   - 7 段旁白对应 7 段镜头
   - `male-qn-jingying` voice_id (沉稳男声)
   - 注意: 海螺返回 **hex** 不是 base64(`bytes.fromhex()`,不是 `base64.b64decode()`)
2. **BGM 合成** (纯 ffmpeg 合成):
   - 110Hz 持续 drone (pedal tone)
   - 60 BPM 五声音阶 arpeggio (A3/C4/D4/E4/G4)
   - 棕噪音 + 2kHz lowpass 模拟雨声
   - 三者混音 + 头尾各 3s fade
3. **评论区叠加** (ffmpeg drawtext):
   - 8s 抖音手机屏样式
   - 5 条评论 + 顶部视频信息
4. **最终合成**:
   - 9 段视频 concat
   - BGM + 7 段配音(adelay 按时间轴放置)
   - 压制 AAC + mp4 faststart

## 关键 ffmpeg 技巧 (这版 ffmpeg 是 2018 年的,语法有坑)

- 旧版 amix **不支持 `normalize=0`**(直接默认即可,只是音量小)
- `drawtext` 多条文本要用逗号串联,不能分行
- `tpad=stop_mode=clone:stop_duration=N` 末帧定格 N 秒
- `adelay=X|X` 立体声两通道都得给

## 业界做法 (参考 video-editing skill)

```
Screen Studio / raw footage
  → Claude / Codex
  → FFmpeg
  → Remotion
  → ElevenLabs / fal.ai
  → Descript or CapCut
```

我们这版跳过 Remotion/Descript/CapCut(都不必要):
- Remotion 是 React 程序化视频,适合 template-driven 大批量
- Descript 是 transcript-based 编辑,适合播客 / 教程
- 剪映/CapCut 是手工 UI 工具,无 API

**ffmpeg + 海螺 TTS 已经覆盖了"中长视频从脚本到成片"全流程**。

## 下一步 (L3.5)

1. **抖音发布**: 抖音开放平台 API key(还没给)
2. **配字幕**: 用 ffmpeg drawtext 把 7 段旁白当字幕烧进视频(目前只 3 段文字卡,7 段都没字幕)
3. **可灵 1.6 fallback**: 火山 Ark 没 1.6 模型,Seedance 1.5-pro 已经是最佳 video 选择

## 怎么复用

把 `run_video_pipeline.py` + `tts_hex.py` + `final_mix.sh` 拼成 1 个 skill:
**`panda-episode-pipeline`** — 输入 L1 script + storyboard,输出 L3 完整视频

`SKILL.md` 大约 80 行,关键步骤 6-8 个 ffmpeg 命令,内容主导 1 键跑完整 episode。

## 成本汇总

| 项 | 成本 |
|---|---|
| L1 (script + storyboard,人工) | 0 |
| L2 (Seedream + Seedance) | ¥10-18 |
| L3 (海螺 TTS + ffmpeg) | ¥0.1-0.3 (海螺按字符计) |
| **单条总** | **~¥11-19** |
| **6 个月 500 万 views / 180 天** | **¥2000-3400** |

对比外包(¥800-3000/条),panda 自做 1 条 = 外包 1/100。
