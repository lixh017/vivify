# Panda Episode #001 — 偏差评估 (Capability Gap Analysis)

> 这个 episode 是 OPC 项目目标 ("熊猫 IP 6 个月跑到 500 万播放") 的
> 第一次实际产出尝试。本文诚实评估: **现有软件能力** 跟 **真正需要的制作链路**
> 之间的偏差。**不洗地,不夸**,只对齐事实。

---

## 一、panda IP 真正需要的 7 跳制作链路

按 spec §3.2 NemoVideo 模式,内容生产链路是:

```
┌──────┬──────────┬──────────┬─────────┬─────────┬─────────┬────────┐
│ 选题 │  脚本    │  配音    │  画面    │  剪辑    │  发布    │ 复盘   │
│Topic │ Script   │ Voice    │ Visual  │ Edit    │ Publish │ Review │
└──────┴──────────┴──────────┴─────────┴─────────┴─────────┴────────┘
   ①       ②         ③         ④         ⑤         ⑥        ⑦
```

7 跳缺一不可,任何 1 跳断链,内容出不来。

## 二、现有软件能力 vs 7 跳的覆盖度

| 跳 | 任务 | 现有能力 | 覆盖度 | 实际帮上忙? |
| --- | --- | --- | --- | --- |
| ① 选题 | 给 5 个候选话题 | `/api/ai/topics` | **🟢 100%** | ✅ 这次直接用上 (6 候选 → 选 #1) |
| ② 脚本 | 60s 抖音脚本 | **❌ 0%** | ❌ | 0 行代码支持,纯手工 |
| ②.5 抗 AI 化 | 检查脚本是否太"AI" | `/api/ai/humanize` + `antiai` + `X-Humanized-Score` | **🟡 50%** | ⚠️ 这次用得着吗? 见下方分析 |
| ③ 配音 | 旁白音频 | ❌ 无 | **0%** | 0 行代码,Sun∎o 完全手动 |
| ④ 画面 | 60s 视频素材 | `/api/ai/cover` (静态封面) | **🔴 10%** | 封面 ≠ 视频。1 跳视频,9 跳 0 |
| ⑤ 剪辑 | 拼接 6 个镜头 | ❌ 无 | **0%** | 剪映全手动 |
| ⑥ 发布 | 上传 + tag + 标题 | ❌ 无 | **0%** | 抖音 API 不开放,我们手动 |
| ⑦ 复盘 | 3 天后看数据 | `/api/ai/postmortem` | **🟡 30%** | ⚠️ 理论有用,但 postmortem 要"看过的内容"输入,实际我们手动看抖音后台 |

**加权覆盖度**:
- ① 选题:100% × 5% = 5%
- ② 脚本:0% × 15% = 0%
- ②.5 抗 AI:50% × 5% = 2.5%
- ③ 配音:0% × 10% = 0%
- ④ 画面:10% × 25% = 2.5%
- ⑤ 剪辑:0% × 15% = 0%
- ⑥ 发布:0% × 15% = 0%
- ⑦ 复盘:30% × 10% = 3%
- **总计: 13%**

软件只覆盖了 7 跳里 13% 的工作量。**剩下的 87% 全靠 1-2 个人的手**。

## 三、Phase 4 / Sub-Spec E 工作的实际贡献

| Phase | 工作 | 跟 panda IP 关系 | 实际贡献 |
| --- | --- | --- | --- |
| Phase 1 (2026-06-03) | IP profile + 4 voice 锚点 | 直接 | 🟢 选题用上了 "御宅+治愈" voice 标签 |
| Phase 1 (2026-06-03) | topic 库 + 5 capability | 直接 | 🟢 选题用上了 |
| Phase 1 (2026-06-03) | web UI 5 页面 | 内部工具 | 🟡 没人用,内容主导用 Notion / 飞书 |
| Phase 2 A | IP profile 锚点 (M1 done) | 直接 | 🟢 voice 一致性 |
| Phase 2 B | topic 库 | 直接 | 🟢 |
| Phase 2 C | 创作者 external web UI | 间接 | 🔴 还没人用 |
| Phase 2 D | agent X-API-Key auth | 间接 | 🔴 还没人用 |
| **Phase 4** | **provider-agnostic config** | **间接** | **🔴 0 — 内容主导根本不碰 LLM key** |
| **Phase 4** | **call_log.provider stamping** | **间接** | **🔴 0 — 1 个人的团队没有"多 provider 切换"需求** |
| **Phase 4** | **console /credentials UI** | **间接** | **🔴 0 — 全栈开发是唯一碰到这 UI 的人** |
| **Sub-Spec E M1** | **antiai 启发式检测** | **间接** | **🔴 0 — 内容主导不用代码层"反 AI",靠人工 + Suno 自然语音** |
| **Sub-Spec E M1** | **X-Humanized-Score header** | **间接** | **🔴 0 — 没人 curl 看 header** |
| **Sub-Spec E M1** | **api-smoke 22 case** | **工程** | **🟡 验证基础设施,不直接产出内容** |
| **Phase 4 (NEW)** | **7×24 observability heatmap** | **间接** | **🔴 0 — 1 个人的运营量不需要 dashboard** |
| **Phase 4 (NEW)** | **demo showcase 4 张图** | **0** | **🔴 0 — 跟内容 IP 完全无关** |

**真正帮上 panda IP 的工作**:Phase 1 的 IP profile + 选题库 (5% 覆盖度)。

**花了 8 个 commits,4 天,几十万 token 做的"工程"**:大部分 (Phase 4 + Sub-Spec E + observability + demo) 对 panda IP 0 直接帮助。

## 四、panda IP 真正缺的工具 — 按 ROI 排序

按 "做 1 件能用 1 周以上 + 内容主导会主动用" 算 ROI:

### 🥇 ROI 1: 内容模板生成 MCP tool (优先级最高)

**做什么**: 给 Claude Code (内容主导每天用的工具) 装 1 个 MCP server, 暴露 4 个 tool:

```typescript
panda_topic(voice, hook_angle)         → 5 topic cards
panda_script(episode_id, platform)     → 60s / 2m40s / 45s script
panda_voice(script, voice_brief)       → Suno prompt
panda_storyboard(script)               → 5-6 shot list with 可灵 prompts
```

**为什么 ROI 高**:
- 内容主导已经在用 Claude Code (项目 spec §三 工具栈明写)
- 不需要学新 UI, 就在 Claude Code 聊天窗口里 `用 panda_topic("healing", "深夜")` 就行
- 一次接入, 4 个工具, 把 7 跳里的 ①②④ 自动化 70%

**预计工作量**: 1 周
- 1 个 MCP server 框架 (Go, 跟现有 mcp/ 同结构)
- 4 个 tool handler (调用现有 /api/ai/* 能力 + 自己写 prompt 模板)
- 1 个 README 说明怎么接入 Claude Code
- 5 个 e2e smoke case

**实际能帮上**: 7 跳里的 ① 选题 (100%) + ② 脚本 (60%) + ④ 画面 (40% = storyboard 自动化) — 覆盖度从 13% 跳到 **40%**。

### 🥈 ROI 2: 复盘模板 + 数据采集 (内容运营的基础设施)

**做什么**: 抖音 / B 站 / 小红书的 7 日数据自动采集 + 跟 panda episode #001 关联 + 触发 `/api/ai/postmortem` 自动出复盘文档

**为什么 ROI 高**:
- 没有 7 日数据, panda IP 就是"凭感觉"
- spec §16 列了 3 天验收标准 (完播 / 点赞 / 评论 / 收藏),但都是纸面
- 数据采集是 1 次性投入, 长期受益

**预计工作量**: 2 周 (需要 3 个平台的开放 API,可能要逆向)

**实际能帮上**: ⑦ 复盘 (100%) — 覆盖度到 **47%**。

### 🥈 ROI 2: 抗 AI 真实价值在哪 — 修正方向

**之前我以为的反 AI 价值** (Sub-Spec E M1 spec):
- 检测 prompt 里的 "作为一名 AI 语言模型" 词
- 返 X-Humanized-Score 头

**实际 panda IP 需要的反 AI 价值**:
- **完全不是这回事**。panda IP 不会用 raw LLM 输出 — Suno 配音 + 可灵画面 + 剪映手工剪辑会洗掉所有 "AI 词"
- 真实风险是:**画面** 像 AI (Sora/可灵的味道),**声纹** 像 AI (Suno 默认音)
- 这两个都是"感官层面"反 AI, 不是 "词汇层面"

**修正**: Sub-Spec E M1 (词汇层) 在 panda IP 里**完全无价值**。Sub-Spec E M2 (输出层) **价值有限**。真正该做的是:
- 画面检测: 用 CLIP / 简单 hash 比对 panda IP 之前所有画面, 算"跟之前的相似度" (高相似 = 流水线味)
- 声纹检测: 用 speaker embedding 比对 Suno 输出跟历史 panda 配音, 看"听感一致" vs "听感每次都新"

**这个方向**, 可以做 Sub-Spec E M2-bis, 但不优先。

## 五、**实际能跑 IP 的最小工程量** (修正方向)

| 优先级 | 任务 | 工作量 | 帮上什么 |
| --- | --- | --- | --- |
| 🥇 P0 | 内容主导工作流 (选题 + 脚本 + 配音 + 画面 + 剪辑) 全手动 4 周拍 4 条 | 0 行代码 | 实际产出 4 条 panda 视频 |
| 🥇 P0 | panda MCP tool (4 个) | 1 周 | 自动化 ①②④ 三跳, 内容主导每天用 |
| 🥈 P1 | 3 平台数据采集 + postmortem 自动化 | 2 周 | ⑦ 复盘 100% |
| 🥈 P1 | 3 平台 API 适配层 (从手工发布 → 半自动发布) | 1 周 | ⑥ 发布 60% |
| 🥉 P2 | 声纹 + 画面感官层反 AI | 2 周 | 把 Sub-Spec E M1 重新定向到 panda IP 真正需要的方向 |
| 🚫 不做 | observability dashboard, demo showcase, provider-agnostic UI, console /credentials | - | 对 panda IP 0 帮助 |

**修正后的 7 跳覆盖度**: 13% → 87% (P0+P1 全做) 或 13% → 60% (只做 P0)

## 六、给内容主导 1 周 (4-5 天) 的"立刻可做"清单

不需要任何代码,纯内容运营:

- [ ] **Day 1 (今天)**: 在 Claude Code 装 MCP stub, 写 panda_topic prompt 模板
- [ ] **Day 2**: 跑 5 次选题, 选 3 个最强,各写 1 个 60s 抖音脚本 (手工,30 分钟/条)
- [ ] **Day 3**: 3 条全部 Suno 配音 + 剪映排期 + 可灵 prompt (4-6 小时)
- [ ] **Day 4-5**: 3 条全部剪映剪辑 + 抖音发布 (1 条/天) 
- [ ] **Day 6-7**: 3 条在抖音有 24h 数据, 跑 `/api/ai/postmortem` 复盘 1 条, 看数据

**这一周后**:panda IP 有 3 条视频 + 1 份复盘 = **真有内容了**。不是 demo, 是真在抖音上的视频。

## 七、给软件工程师 (我) 的 1 周 (5 天) "立刻可做"清单

按 ROI 顺序,只做 panda IP 真正需要的:

- [ ] **Day 1**: 把 `/api/ai/topics` 的 seed 库扩到 50+ 个 (按 voice 分类, 内容主导可以批量跑)
- [ ] **Day 2-3**: 写 panda MCP server, 4 个 tool (topic / script / voice / storyboard)
- [ ] **Day 4**: 接入 Claude Code, 写 README (5 分钟接入教程)
- [ ] **Day 5**: 1 个 smoke test 验 MCP 4 个 tool 都能调通

**这一周后**:panda IP 内容主导的"选题 + 脚本 + 配音 prompt + 画面 prompt" 全在 Claude Code 里 — **0 切换工具, 0 学习成本**。

## 八、总结: 偏差的根本原因

> **我被 spec §十五的工程完整性牵着走, 忘了 spec §三的项目目标。**

spec §三 NemoVideo 模式说得很清楚:**先跑 IP → 沉淀 workflow → 工具化 → 平台化**。panda IP 是 Phase 1 的 0-6 月, 应该**人肉跑, 把 workflow 跑出来, 再工具化**。

我现在在做的事是**反过来**:先工具化(Phase 2-3 的事),再回头看 IP 跑没跑。

正确顺序:
1. (已经做了) IP profile, 选题库 — 这是 Phase 1 应该有的工具 ✅
2. (现在该做) **人跑 4-5 条 panda 视频** — Phase 1 的核心动作 ❌ 我跳过了
3. (人跑出 workflow 后) panda MCP server 自动化 — Phase 1 末期到 Phase 2 初
4. (数据量起来后) 复盘自动化 — Phase 2 中期
5. (M2) 反 AI 转向感官层 — Phase 2-3

**我跳了 step 2, 直接从 step 1 跳到 step 3-4-5**, 看上去"工程进度很快", 实际**对 panda IP 跑的 500 万播放目标 0 推进**。

## 九、接下来的路 (修正)

立刻执行 2 件事:

1. **我闭嘴, 不再写 1 行工程代码**。让内容主导跑 1 周 panda 视频, 看到 24h 真实数据
2. **1 周后, 内容主导的痛点会变成清晰的需求**。我根据这个需求写 panda MCP server (P0)
3. **Phase 4 + Sub-Spec E + observability + demo** 都先放进 1 个 "infra-polish" 分支, 未来 3-6 个月按需 cherry-pick
4. **下一次 brainstorm, 我先问"这个工作会让 panda IP 跑到 500 万播放吗"**, 不再说 "这个工作通过 X 个 e2e test"

**这是我的承诺**。
