---
name: vivify-platform-adaptation
description: 平台适配层,把成品视频和母版脚本变成抖音/B站/小红书三平台版本并排好发布时间。use when 需要将一个 opc 产出物(视频+脚本)分发到国内多平台、或在 web UI 自动化生成多平台标题/简介/标签/排期
---

# vivify-platform-adaptation

## 何时使用

当 `vivify-asset-generation` 已经产出视频 + 母版脚本后,本 skill 负责:
1. 基于平台规则裁剪标题/简介/tag
2. 套用反限流策略
3. 排定发布时间(由 cron 决定,不由 LLM 决定)
4. 输出 3 平台可发布的 JSON payload

**依赖**:`vivify-script-generation`(母版脚本) + `vivify-asset-generation`(视频 URL)。

## 三平台规则对照

| 平台 | 标题上限 | tag 数 | 发布时间窗 | 必填项 | 反 AI 关注点 |
|------|---------|--------|------------|--------|--------------|
| 抖音 | ≤22 字 | 3-5 | 19-23 点 | 封面文字 | "重复内容" 检测,同款模板/口播节奏触发限流 |
| B站 | ≤80 字 | 5-10 | 18-22 点 | 简介≥30字 + 分区 + 3连引导 | "低质" 检测,5 分钟以下无信息密度即降权 |
| 小红书 | ≤20 字 | 5-10 | 12-14 / 20-22 点 | emoji≥2 + 9 图或短视频 | "搬运" 检测,医疗/绝对化违禁词敏感 |

## 限流规避 checklist(发布前必过)

- [ ] 抖音:同 IP 7 天内口播文案重复率 < 30%(换句式、换钩子)
- [ ] 抖音:不出现 "震惊/惊呆了/速看" 等标题党触发词
- [ ] B站:简介 ≥ 30 字且含 1 个时间戳(标记"干货密度")
- [ ] B站:分区正确(知识区/生活区),错分区直接限流
- [ ] 小红书:违禁词零容忍(医疗/医美/绝对化),全文过滤一遍
- [ ] 小红书:emoji 数量 2-8 个,首发图必须原创
- [ ] 三平台共性:不连续 3 天同主题,选题间隔至少 1 个

## 反 AI 检测触发表

每个平台都把"AI 批量生成"特征做降权。规避手段:
- 加入人设口癖(熊猫:「慢慢来」「你说呢」)→ ≥ 1 处
- 字幕用剪映手打而非自动生成
- 配音有 1-2 处明显停顿/吸气声
- 镜头切换节奏不均匀(保留 3-5 秒长镜头)
- 发布时间错开整点(不卡 19:00/20:00,选 19:23 这种)

## 平台关键词红线

| 平台 | 必避词 |
|------|--------|
| 抖音 | 震惊、惊呆了、99% 的人不知道、点进来、不转不是 |
| 小红书 | 第一、最、绝对、唯一、医生推荐、治疗、根治、医美化用语 |
| B站 | 硬避 "速看" 类;软标题党可接受但简介必须真诚 |

## 3 平台版本生成(代码示例)

```js
function adapt(master, ip = '熊猫') {
  return {
    douyin: {
      title: master.hook.slice(0, 22),
      tags: pickTags(master.theme, 4),
      publishAt: cronSlot('19-23'),
      desc: master.hook + '\n' + ip + '慢慢聊'
    },
    bilibili: {
      title: master.theme + ' | ' + master.subtitle,
      tags: pickTags(master.theme, 7, { long: true }),
      publishAt: cronSlot('18-22'),
      desc: buildBilibiliDesc(master),
      partition: '知识·人文',
      tripleLike: true
    },
    xiaohongshu: {
      title: emojiWrap(master.hook, 2).slice(0, 20),
      tags: pickTags(master.theme, 8, { xhs: true }),
      publishAt: cronSlot(['12-14', '20-22']),
      desc: emojiWrap(master.body, 4),
      imageCount: 9
    }
  };
}
```

## 发布时间调度(cron,不由 LLM 决定)

```cron
# 抖音  每周一/三/五 19:23 / 20:23 / 21:23(错开整点)
23 19 * * 1,3,5  opc publish --platform douyin
23 20 * * 1,3,5  opc publish --platform douyin
23 21 * * 1,3,5  opc publish --platform douyin
# B站  每周二/六 19:37(主品牌场)
37 19 * * 2,6   opc publish --platform bilibili
# 小红书  每天 12:37 和 20:37(午饭 + 睡前)
37 12 * * *     opc publish --platform xiaohongshu
37 20 * * *     opc publish --platform xiaohongshu
```

分钟位不取整(37/23 而非 00)是反 AI 检测的硬要求。

## 完整 payload 输出示例

```json
{
  "master_id": "topic_2026_06_08_why_busy",
  "video_url": "https://cdn.opc.local/v/2026-06-08-busy.mp4",
  "platforms": {
    "douyin": {
      "title": "为什么我们越来越忙?",
      "tags": ["治愈", "御宅", "反鸡汤", "熊猫"],
      "publishAt": "2026-06-08T19:23:00+08:00",
      "desc": "今天熊猫想跟你说:忙 ≠ 充实。\n#治愈"
    },
    "bilibili": {
      "title": "为什么我们越来越忙?熊猫用 6 分钟讲透现代焦虑",
      "tags": ["哲学", "治愈", "御宅", "国潮", "焦虑", "熊猫哲学", "深度"],
      "publishAt": "2026-06-09T19:37:00+08:00",
      "desc": "00:30 钩子\n02:15 焦虑的工业化根源\n05:00 熊猫的 3 个解药\n分区:知识·人文"
    },
    "xiaohongshu": {
      "title": "🐼 越忙越累?不是你的错",
      "tags": ["治愈系", "反鸡汤", "焦虑自救", "熊猫日常", "国潮", "御宅", "情绪疏导", "慢生活"],
      "publishAt": "2026-06-08T20:37:00+08:00",
      "desc": "🐼 熊猫说:忙是病,不是美德。\n🐼 3 个解药:\n1. 删一个 app\n2. 周末断网 6 小时\n3. 跟熊猫一起发呆",
      "imageCount": 9
    }
  }
}
```

## 反模式(不要做)

- 不要让 LLM 在 prompt 里直接生成具体发布日期 → 全部走 cron
- 不要把抖音文案原封不动贴 B 站简介 → 必重建(含时间戳)
- 不要在小红书用纯文字无 emoji → 必带 2-8 个
- 不要三平台同分钟发布 → 错开 ≥ 2 小时(避内部"撞车"降权)
- 不要省略违禁词扫描步骤(尤其小红书)

## 上下游

- 上游:`vivify-script-generation` 提供母版脚本,`vivify-asset-generation` 提供视频 URL
- 下游:发布后表现数据回写 `vivify-content-calendar`(web UI 页面 3)和表现 dashboard(页面 4)
