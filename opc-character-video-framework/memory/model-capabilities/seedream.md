# Seedream 4.0 / 5.0 — model capabilities

> Source: 火山方舟 (Ark) Seedream 官方 documentation, manual testing on fengge EP001–EP006.
> Status: 🟡 hypothesis for unreleased features, ✅ validated for what we use.

---

## What we USE (validated)

| Feature | How we use it | Status |
|---|---|---|
| `reference_image` (subject) | 喂 full-body canonical, 锁 silhouette | ✅ |
| `reference_image` (style) | 喂 2D gouache 样本, 锁画风 | ✅ |
| 中文 prompt | 全中文 prompt, 跟 panda IP 中文属性一致 | ✅ |
| Resolution 2K | 默认 2K, premium tier 4K | ✅ |
| Aspect ratio 9:16 / 16:9 / 1:1 | 抖音 9:16, B站 16:9, 小红书 1:1 或 3:4 | ✅ |

---

## What we COULD use (未实施, 假设)

### 1. Multi-image input (replace / combine / transfer style) 🟡

Seedream 4.0 支持一次传多张 input image, 然后用文本指令:
- "把 image[1] 的 outfit 替换到 image[0] 的角色身上"
- "用 image[2] 的风格重新画 image[0]"
- "把 image[0] 和 image[1] 合并成一个场景"

**潜在应用**:
- 换 outfit 不需要重新画 canonical — 直接 "把 image[1] 的 朱红汉服 替换到 image[0] 的 panda 身上"
- 国潮 → 御宅 风格迁移 — 给一张 panda, 一张 sample 御宅风格, 让模型 transfer

**未实施原因**: 暂时手动 round-robin 3 张 canonical 就够, 不值得加 pipeline complexity.

### 2. Multi-image output (一组图) 🟡

Seedream 4.0 支持在一次 call 里生成 4 张 consistent 的图, trigger 词:
- "一系列"
- "一套"
- "组图"
- "一组"

**例子**:
```text
生成一组 4 张图, 峰哥 (成年熊猫) 在不同时辰的竹林小院:
1. 清晨, 朝阳斜照
2. 正午, 树影斑驳
3. 黄昏, 暖橙晚霞
4. 月夜, 冷蓝月光
```

**潜在应用**:
- 一次生成 4 outfit canonical (不用 4 次 call)
- 一次生成同一场景的 4 个 angle (但违反 NEVER multi-angle, 见 `id-drift-prevention.md`)

**未实施原因**: 担心 4 张图的 ID 一致性不如 4 次独立 call. 待 stress test.

### 3. Image editing (add / delete / replace / modify) 🟡

Seedream 4.0 支持文本指令修改已有图像:
- "在 image 里添加 一只小狗"
- "删除 image 里的 折扇"
- "把 image 里的 朱红 改成 宝蓝"
- "修改 image 里峰哥的表情, 让它更开心"

**潜在应用**:
- 后期微调 (表情微调, 道具微调) — 比重新生成省 80% 成本
- A/B test 同一个场景不同细节

**未实施原因**: 没找到稳定的 API 入口, 待火山方舟 文档更新.

### 4. Visual signals (arrows / wireframes / doodles) 🔴

Seedream 4.0 支持在 prompt 里用 ASCII art / 箭头 / 简笔画 精确控制构图:

```text
[Image: rough doodle of panda sitting in bamboo courtyard]
  ↑
  bamboo on left, panda center, tea table right
```

**潜在应用**: 复杂场景 (5 个角色 + 5 个道具) 时用 doodle 锁布局.

**未实施原因**: 短剧场景通常 1 角色 + 2-3 道具, 不需要这种精度.

---

## Reference image 三种类型

| 类型 | 用途 | 我们怎么用 |
|---|---|---|
| `subject` (主体) | 锁角色 silhouette + 配色 | 喂 canonical full-body |
| `style` (风格) | 锁画风 / 媒介 | 喂 2D gouache 样本 |
| `product` (产品) | 锁具体物件 (电商用) | 不用 |

API 字段名 (火山方舟): `reference_image` 后面跟 `type: "subject" | "style" | "product"`.

---

## Limitations (踩过的坑)

| 限制 | 表现 | 我们的 workaround |
|---|---|---|
| 中文 prompt 偶尔会输出中文水印 | "峰哥" 出现在画面角落像 logo | 末尾加 "不要生成水印 不要生成 Logo" |
| 不擅长生成文字 | "题字: 静" 会渲染成乱码 | ffmpeg 后期压字幕 |
| 不擅长精确的 2 个角色同框 | 2 个角色会 merge / 黏在一起 | 1 shot 1 角色, 复杂场景用 2 个 shot 拼 |
| 不擅长 手指精确动作 | 6+ 根手指 / 扭曲手指常见 | 不要写 "数手指" / "弹钢琴", 改 "握扇" / "拿杯" |
| 3D drift 严重 (见 style-anchors.md) | 50% 漂到 3D toy | Style anchor 6 行 NEVER |

---

## 火山方舟 API call example (供参考)

```python
import base64
from openai import OpenAI  # 火山方舟 OpenAI-compatible

client = OpenAI(
    api_key=os.environ["ARK_API_KEY"],
    base_url="https://ark.cn-beijing.volces.com/api/v3",
)

with open("characters/fengge/canonical/panda-canonical-zh-red.jpg", "rb") as f:
    canonical_b64 = base64.b64encode(f.read()).decode()

response = client.images.generate(
    model="doubao-seedream-4-0-250828",
    prompt="峰哥 (成年熊猫, 朱红汉服) 站在竹林小院, 折扇半展, ...",
    size="1024x1792",  # 9:16
    reference_image=[
        {
            "image": canonical_b64,
            "type": "subject"
        }
    ]
)
print(response.data[0].url)
```

---

## Roadmap (下一步试)

- [ ] Multi-image output 试一次, 验证 4 张图 ID 一致性
- [ ] Image editing 试微调表情
- [ ] Visual signals 试复杂 2 角色场景
