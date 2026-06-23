# ID drift prevention — keep character identity stable across shots

> Status: ✅ validated — fengge EP003–EP006 face consistency > 90%
> Root cause + fix for the #1 issue with character-driven video gen.

---

## The core problem

Seedream 4.0 + Seedance 2.0 生成的 panda, **每 5 个 shot 平均有 2–3 个 face drift**:
- 眼睛从"半阖" 漂成"全睁"
- 头身比从 1:1.2 漂成 1:2 (看起来像 3D 玩具熊)
- 配色从 朱红 + 米白 漂成 朱红 + 灰白 (cream 变灰)
- 折扇 / 茶盏 / 山竹杖 等 props 漂成 普通物件

我们用 3 个 fix 解决了这个问题, 翻车率从 50% → < 10%.

---

## Fix 1: 大头照 (close-up face shot) 作为独立 reference

**核心 rule**: full-body canonical **不能** 用来训练 face. 模型在 full-body 图像里学到的 face embedding 弱, 因为脸只占画面的 15%.

**我们的解法**:

```text
characters/fengge/canonical/
├── panda-canonical-zh-red.jpg           # full-body, 朱红汉服 (default)
├── panda-canonical-blue-changsan.jpg    # full-body, 宝蓝长衫
├── panda-canonical-warm-orange.jpg      # full-body, 暖橙短褂
└── panda-face-closeup.jpg               # NEW: 大头照, 占画面 70%+
```

`panda-face-closeup.jpg` 内容:
- 脸部占画面 70%+
- 半阖眼 + 微微笑意 (facial expression register)
- 纯色背景 (cream #F5F0E1)
- 不戴任何 props
- 4K, 锐利

**Pipeline 用法** (`render_episode.py`):

```python
# 1. 选 closest canonical based on outfit
outfit_canonical = choose_canonical_by_outfit(shot.outfit)

# 2. 拼 reference_images: outfit canonical + face closeup
reference_images = [outfit_canonical, FACE_CLOSEUP]

# 3. 喂给 Seedream
seedream_call(prompt=..., reference_images=reference_images, ...)
```

效果: face consistency 从 ~60% → ~92%.

---

## Fix 2: Multi-modal reference (1-2 character + 1 scene + 1 video + 1 audio)

Seedance 2.0 支持 4 modalities input: **text + image + video + audio**.

最佳实践 (per 火山方舟 official guide):

```text
reference_image[0] = character canonical (full-body)        # silhouette + 配色
reference_image[1] = face closeup                          # face embedding
reference_image[2] = scene reference (可选)                 # 场景氛围
reference_video[0]  = 一段 3 秒的 canonical motion (可选)  # 走路 / 转身的方式
reference_audio[0]  = 角色 voice sample (可选)             # 音色锁定
```

**为什么 audio reference 重要**: 短剧 panda 有 4 种 tone (治愈/御宅/哲学/国潮), 每个 tone voice_id 不同. audio reference 让 Seedance 知道"这个角色说话应该用这个调子", 避免 lip-sync 漂到错误音色.

**注意**: 4 个 reference 已经够, 再多模型会 confuse (见下).

---

## Fix 3: NEVER multi-view character reference

**致命错误**: 给模型同时传 正面 + 侧面 + 背面 3 个角度的 character reference, 以为这样模型能"理解 3D 形状".

**实际发生的事**: 模型把 3 个角度当成 3 个不同的角色, 在 shot 里随机切换视角时 face 就 drift 了.

**Rule**:

```text
# WRONG
reference_image = [front_view.jpg, side_view.jpg, back_view.jpg]

# RIGHT
reference_image = [front_view_full_body.jpg, front_view_face_closeup.jpg]
```

如果一定要 3D 旋转感, **用同一个角度的不同 frame**, 不要用不同角度.

---

## Fix 4 (待实施): Style anchor 锁住 "signature expression"

`character.yaml.expression` 是 "半阖眼 + 微微笑意, 不直视镜头". 这个表情容易漂 — 模型经常渲染成"睁大眼睛看镜头"(太 anime).

**草案**: 在 style_anchor 末尾加一行:

```text
半阖眼 + 微微笑意, 不直视镜头, NEVER full-open eyes NEVER direct eye contact with camera
```

(未验证, 🟡 hypothesis. EP007 准备试.)

---

## Anti-patterns in character reference

| ❌ WRONG | ✅ RIGHT |
|---|---|
| 1 张 full-body 撑所有 | 3 张 full-body (3 outfits) + 1 张 face closeup |
| 多角度 character reference | 单角度多 frame |
| Reference 里出现现代道具 (手机, 西装) | Reference 干净, 只有 IP 圣经里的 props |
| Reference 模糊 / 低分辨率 | Reference 4K, 锐利 |
| Reference 背景复杂 | Reference 纯色背景, 让模型聚焦 silhouette |
| Face closeup 带 props (戴帽子 / 戴墨镜) | Face closeup 不戴任何 props |
| Reference 是 3D 渲染 | Reference 是 2D gouache (跟 shot 风格一致) |

---

## Validation pipeline

1. 跑 5-shot 测试 episode
2. 抽取所有 shot 的 face crop (用 `ffmpeg` + `dlib` 或 `mediapipe`)
3. 计算 face embedding similarity (用 `face_recognition` 库, threshold = 0.6)
4. < 90% similarity 就回去修 reference

`tests/test_face_consistency.sh` (待补).

---

## How to add a new canonical face closeup

1. 用 Seedream 4.0 生成 4K 大头照, prompt:
   ```text
   Close-up portrait of 峰哥 (成年熊猫, 朱红汉服 collar visible),
   half-closed eyes, gentle smile, NOT looking at camera,
   cream #F5F0E1 solid background, 4K, sharp focus on face,
   2D gouache, children's book illustration style
   ```
2. Lint pass via `validators/canonical_image_check.py`
3. 保存到 `characters/fengge/canonical/panda-face-closeup.jpg`
4. 跑 5-shot test, face similarity > 0.9 才算 ✅
