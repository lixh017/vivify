---
name: vivify-scene-decomposition
description: Decompose an OPC script into a per-shot scene table (镜头表) — contract between脚本 and资产. Use when an vivify-script-generation output exists and per-platform shots must be split with资产 IDs for independent generation.
---

# OPC Scene Decomposition

把 `vivify-script-generation`产出的脚本，拆成镜头表——脚本 →资产 的契约：每镜一个资产 ID，由 `vivify-asset-generation`独立生成。

##何时切

**三触发，任一即切**：①动作切换（坐→站）②情绪转折（温柔→毒舌）③5 秒硬切（同动作 ≤5s）。

**硬上限**：1镜 ≤3s。30s脚本最多10镜。表情微变/眼神/口型 →不切，用运镜代替。

##镜头类型（5 种）

```text
LS远景：全幅、交代环境
MS 中景：腰部以上、主对话
MCU 近景：胸口以上、强调表情
CU 特写：脸/手/道具
ES 空镜：纯环境/物（转场/留白）
```

##平台时长规范

|平台 | 时长 |节奏 |
|------|------|------|
|抖音 |15-60s |黄金3s钩子、首尾密集 |
| B站 |30-180s | 中视频、留白多、深度展开 |
| 小红书 |15-90s |视觉精致、口播干净 |

##资产 ID命名

```text
<ip>-<role>-<scene>-<shot>-v<n>
```

- `ip` ：IP标识（如 `panda`）
- `role` ：角色短码（如 `xb` = 小白）
- `scene` ：`st01` `st02`…
- `shot` ：`sh01` `sh02`…
- `v<n>` ：从 v1 起，重做 +1

**示例**：`panda-xb-st05-sh02-v1` =熊猫/小白/第5场/第2镜/v1。

**稳定性**：同 ID画面必须一致（角色锚定/画风/服饰）；改画面 =升版本号。

##镜头表 Schema（JSON）

```json
{
 "$schema": "opc/scene-table/v1",
 "scriptId": "string",
 "platform": "douyin | bilibili | xiaohongshu",
 "totalDuration":30,
 "scenes": [{
 "shotNo":1, "duration":3, "shotType": "MS",
 "visualDesc": "string —构图/光/动作",
 "roleAction": "string —姿态指令",
 "dialogue": "string —台词（可空）",
 "transition": "cut | fade | dissolve",
 "assetId": "panda-xb-st01-sh01-v1"
 }]
}
```

- `shotType`：必为5 种之一
- `visualDesc`：`vivify-asset-generation` 的 prompt 输入
- `transition`：默认 `cut`，情绪转折用 `fade`，转场用 `dissolve`

##跨平台 re-cut

同一脚本 →3 个独立 scene table（不可共享，节奏差异大）：

```text
原脚本（30s通用）
 →抖音版（15-30s）：砍中段、加钩子、镜 ≤2s
 → B站版（60-180s）：补 CU/ES细节镜、加留白
 → 小红书版（15-60s）：留视觉精致镜、删长独白
```

**工作流**：①选 platform②调 `totalDuration` + `scenes.length`③抖音合 MS / B站插 CU-ES④ 所有 `assetId`重新生成（避免跨平台复用）⑤3份独立 JSON存盘。

##完整示例（30s抖音版）

```json
{
 "$schema": "opc/scene-table/v1",
 "scriptId": "sc-2026-06-08-001",
 "platform": "douyin", "totalDuration":30,
 "scenes": [
 {"shotNo":1,"duration":3,"shotType":"MCU",
 "visualDesc":"竹林小院、晨光左侧入，熊猫持折扇正坐，眼神温和",
 "roleAction":"双手持扇、轻摇一下停住、抬头看镜头",
 "dialogue":"你说呢，最近是不是越来越忙？","transition":"cut",
 "assetId":"panda-xb-st01-sh01-v1"},
 {"shotNo":2,"duration":4,"shotType":"CU",
 "visualDesc":"特写眼睛，眼角微动，光斑在瞳孔里",
 "roleAction":"眨眼一次、嘴角轻扬",
 "dialogue":"忙到最后，好像也没忙出什么。","transition":"fade",
 "assetId":"panda-xb-st01-sh02-v1"},
 {"shotNo":3,"duration":5,"shotType":"LS",
 "visualDesc":"空镜：竹叶随微风摇，茶盏蒸汽缓缓升起",
 "roleAction":"无（纯环境）","dialogue":"（留白）","transition":"dissolve",
 "assetId":"panda-xb-st01-sh03-v1"},
 {"shotNo":4,"duration":6,"shotType":"MS",
 "visualDesc":"熊猫起身、背手站、望向远处山",
 "roleAction":"缓缓站起、背手、转身面向山",
 "dialogue":"但你看那山，它从来不忙。","transition":"cut",
 "assetId":"panda-xb-st01-sh04-v1"},
 {"shotNo":5,"duration":5,"shotType":"MCU",
 "visualDesc":"回身、扇尖指向镜头、光晕在扇面",
 "roleAction":"转身、扇尖指镜头、点头",
 "dialogue":"慢慢来，比较快。","transition":"fade",
 "assetId":"panda-xb-st01-sh05-v1"},
 {"shotNo":6,"duration":7,"shotType":"ES",
 "visualDesc":"空镜：远山云雾、字幕浮现在底部",
 "roleAction":"无",
 "dialogue":"（互动字幕：评论区告诉我，你今天在忙什么？）",
 "transition":"fade","assetId":"panda-xb-st01-sh06-v1"}
 ]
}
```

##验证清单

- 总时长 ≤平台上限
- 每镜 ≤3s
- 所有 `assetId`唯一 +符合命名规范
-5 秒硬切满足
-钩子（前3s）必须有冲击力（CU 或反常识台词）
-结尾：抖音留互动 / B站留白

## Cross-references

- **上游**：`vivify-script-generation`（脚本输入）
- **下游**：`vivify-asset-generation`（消费 `assetId` + `visualDesc` + `roleAction`）
- **同级**：`vivify-platform-adaptation`（基于本产物做平台差异化后期）
