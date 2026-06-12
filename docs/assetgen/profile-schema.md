# OPC Assetgen Profile Schema Reference

> 4 套 JSON sub-schema 定义 4 类 IP profile (anthropomorphic /
> digital_human / costume / info). Spec:
> docs/superpowers/specs/2026-06-11-opc-phase2-ip-template-design.md

## 公共字段

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `version` | string | ✓ | 实例级唯一 version 字符串 |
| `name` | string | ✓ | 展示名 |
| `type` | enum | ✓ | discriminator: anthropomorphic / digital_human / costume / info |

## 1. anthropomorphic

适用: 拟人动物 IP (panda, fox, dog, ...).

```json
{
  "version": "fengge_v1",
  "name": "峰哥",
  "type": "anthropomorphic",
  "species": "成年熊猫",
  "body_shape": "头身比 1:1.2 圆胖身材",
  "body_color": "rgb(245,240,225)",
  "eye_color": "rgb(26,26,26)",
  "eye_expression": "半阖带笑意, 不直视镜头",
  "palette": ["朱红#C73E1D", "暖橙#E89B45", "翠绿#3B8C5A", "宝蓝#1F5FA8", "米白#F5F0E1"]
}
```

Consistency anchors (7 个, 1.0 total): name(0.20) + species(0.20) + body_color(0.20) + eye_color(0.20) + theme("国潮")(0.10) + no_claws("不露爪")(0.05) + no_ai_look("不要 AI 生成感")(0.05).

## 2. digital_human

适用: 数字人 IP (虚拟主播 / 虚拟偶像 / 新闻主播).

```json
{
  "version": "lina_v1",
  "name": "莉娜",
  "type": "digital_human",
  "age": 28,
  "gender": "female",
  "ethnicity": "东亚",
  "face_shape": "鹅蛋脸",
  "hair_style": "黑色长发披肩",
  "voice_tone": "温柔知性, 普通话",
  "skin_tone": "rgb(245,228,210)",
  "accent_color": "宝蓝#1F5FA8"
}
```

Anchors (8 个, 1.0 total): name(0.15) + age(0.10) + gender(0.10) + ethnicity(0.10) + skin_tone(0.15) + voice_tone(0.10) + "表情自然"(0.15) + "无恐怖谷"(0.15).

## 3. costume

适用: 古装 IP (朝代角色).

```json
{
  "version": "xianyun_v1",
  "name": "纤云",
  "type": "costume",
  "era": "唐代",
  "role": "侠女",
  "costume_layer": ["襦裙", "披帛", "绣花鞋"],
  "prop_kit": ["长剑", "酒壶", "书卷"],
  "palette": ["朱红#C73E1D", "鹅黄#F5DEB3", "黛绿#2F4F4F"]
}
```

Anchors (7 个, 1.0 total): name(0.15) + era(0.20) + role(0.10) + costume_layer(0.15) + prop_kit(0.10) + "无穿越"(0.15) + palette_color(0.15).

Era enum: 汉代 / 唐代 / 宋代 / 明代 / 清代.

## 4. info

适用: 资讯 IP (演播室新闻节目).

```json
{
  "version": "opc_daily_v1",
  "name": "OPC 每日资讯",
  "type": "info",
  "newsroom_style": "演播室双主播",
  "host_persona": "主播阿橙",
  "font_tone": "黑体加粗, 暖色字幕",
  "accent_color": "宝蓝#1F5FA8",
  "background_color": "rgb(245,240,225)"
}
```

Anchors (6 个, 1.0 total): host_persona(0.20) + newsroom_style(0.20) + font_tone(0.15) + accent_color(0.15) + "无 AI 播报感"(0.20) + background_color(0.10).

## CLI

```bash
opc-asset generate --asset-type image --scene 演播室 --outfit 蓝色西装 --out /tmp/x.jpg
opc-asset check --type costume --prompt "纤云, 唐代, ..."
opc-asset ledger --last 10
```

不加 `--type` 默认 `anthropomorphic` (向后兼容).

注: asset type 仍是 `--asset-type` flag (image / video). 与 IP type --type 区分.

## 加载机制

4 个 sub-package (internal/assetgen/profiles/<type>/) 各自 `init()` 调 `assetgen.Register(type, ProfileEntry)`. JSON profile + schema 通过 `//go:embed` 编译进 binary, 运行时无 IO.

**重要**: 调用方必须在 main.go 里 blank-import 这 4 个 sub-package (例: `_ "github.com/opc/api/internal/assetgen/profiles/anthropomorphic"`) 才能触发 init() 注册. 不 import = registry 空 = LoadProfile 全部返错.

## 扩展 (M2+)

- 加新 IP type = 新 sub-package + profile.json + schema.json + consistency.go + prompts.go
- M2: profile 上传 endpoint (用 sub-package.SchemaJSON() 验证)
- M3: per-version lookup (M1 一个 type 一个 instance)
