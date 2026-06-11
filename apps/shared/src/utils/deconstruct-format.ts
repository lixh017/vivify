// Shared formatting helpers for the AI deconstruct flow. Lives in
// its own module so the dashboard modal and the knowledge page
// modal cannot drift (Phase 2 QA found byte-identical
// `truncateForTitle` + `deconstructToMarkdown` implementations in
// both files; consolidating here makes a future change to the
// Markdown shape a single edit).

import type { DeconstructResult } from '../api/client'

// truncateForTitle caps a transcript snippet at a UI-friendly length
// for the Knowledge doc title. We pull from the start of the string
// (after trim) to mirror what a user would type if they were
// hand-naming the doc.
export function truncateForTitle(s: string, max: number): string {
  const t = s.trim().replace(/\s+/g, ' ')
  if (t.length <= max) return t
  return t.slice(0, max) + '...'
}

// deconstructToMarkdown flattens a DeconstructResult into a
// human-readable Markdown body suitable for storing as a Knowledge
// doc. The first 30 characters of the saved title derive from the
// input transcript; the body keeps the full structured shape so a
// future search over the knowledge base can still match hook types,
// patterns, etc.
export function deconstructToMarkdown(
  input: { transcript: string; platform: string },
  result: DeconstructResult,
): string {
  const lines: string[] = []
  lines.push('# 爆款拆解记录')
  lines.push('')
  if (input.platform) {
    lines.push(`**平台**: ${input.platform}`)
  }
  lines.push(`**整体评分**: ${result.overall_score}`)
  lines.push('')
  lines.push('## 钩子分析')
  lines.push(`- 类型: ${result.hook.type}`)
  lines.push(`- 强度: ${result.hook.strength}`)
  lines.push(`- 原文: ${result.hook.text}`)
  lines.push(`- 分析: ${result.hook.analysis}`)
  lines.push('')
  lines.push('## 结构分析')
  lines.push(`- 范式: ${result.structure.pattern}`)
  lines.push(`- 节奏: ${result.structure.pacing}`)
  lines.push(`- 信息密度: ${result.structure.density}`)
  lines.push('- 节拍:')
  for (const beat of result.structure.beats) {
    lines.push(`  - ${beat.time_pct}% · ${beat.role} · ${beat.description}`)
  }
  lines.push('')
  lines.push('## CTA 检测')
  lines.push(`- 是否存在: ${result.cta.present ? '是' : '否'}`)
  if (result.cta.present) {
    lines.push(`- 类型: ${result.cta.type}`)
    lines.push(`- 位置: ${result.cta.placement}`)
  }
  lines.push('')
  lines.push('## 情绪曲线')
  for (const p of result.emotional_arc) {
    lines.push(`- ${p.time_pct}% · ${p.emotion} · ${p.intensity}`)
  }
  lines.push('')
  lines.push('## 可复用模式')
  for (const pat of result.reusable_patterns) {
    lines.push(`- ${pat}`)
  }
  lines.push('')
  lines.push('## 平台适配建议')
  lines.push(`- 抖音: ${result.platform_fit_notes.抖音}`)
  lines.push(`- 哔哩哔哩: ${result.platform_fit_notes.哔哩哔哩}`)
  lines.push(`- 小红书: ${result.platform_fit_notes.小红书}`)
  lines.push('')
  lines.push('## 原始文案')
  lines.push('```')
  lines.push(input.transcript.trim())
  lines.push('```')
  return lines.join('\n')
}
