# 5 必须问题 — User Intake Template

Ask these **in order, one at a time**. Don't dump all 5 in one message —
that's annoying. Wait for each answer before asking the next.

## Q1 — IP / Character

> 你想做什么角色？(IP 名字或描述)

| 答案分支 | 后续 |
|---|---|
| 已有 IP（如"峰哥"） | 跳过 `vivify-panda-new-ip`，直接进 episode 流程 |
| 描述角色（如"成年熊猫，国潮风，半阖眼"） | 调 `vivify-panda-new-ip` |
| "我不知道，给我推荐" | 推荐 `峰哥`（已完整配置）作为默认 |

## Q2 — Tone

> 想要什么调性？(治愈 / 御宅 / 哲学 / 国潮)

> 用户答不上来时：推荐 `治愈`（用户最接受、最容易过审）。

## Q3 — Platform

> 发哪里？(抖音 / B站 / 小红书)

> 默认：抖音（已验证可发布）。

## Q4 — Story / Topic

> 这集讲什么主题？(一句话)

> 例：「凌晨 3 点失眠」「与苏轼喝茶」「给陌生人的一封信」

## Q5 — Duration

> 想要多长？(15-60 秒)

> 默认：58 秒（已验证标准时长，含 title + end card）。

## After Q1-Q5 answered

You have enough to dispatch:

```
tone         = Q2
platform     = Q3
topic        = Q4
duration     = Q5
ip_name      = Q1 (or "fengge" if default)
```

Invoke the right L3 skill:

```
new IP?     → vivify-panda-new-ip        then vivify-panda-episode-build
existing?   → vivify-panda-episode-build  directly
```

Then `vivify-panda-episode-build` will:
1. Generate storyboard (1-line → STORYBOARD.md via LLM)
2. Generate script (3-段式 via LLM)
3. Run `vivify episode render ... --parallel 4`
4. Report MP4 path + cost
