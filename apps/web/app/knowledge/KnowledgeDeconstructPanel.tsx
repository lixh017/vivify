'use client'

import { useState } from 'react'
import { api } from '@/lib/api'
import type { DeconstructResult } from '@/lib/api'
import type { KnowledgeDoc } from '@/lib/types'
import { useT } from '@/lib/i18n-client'

// KnowledgeDeconstructPanel is the "保存拆解" entry point on the
// knowledge page. It is intentionally a focused variant of the
// dashboard's deconstruct flow: paste a transcript, run the AI
// deconstruct, and on success auto-create a Knowledge doc with the
// full analysis as Markdown. The user never has to leave the
// knowledge page to triage insights they want to keep.
//
// We share the data conversion logic with the dashboard by
// inlining it here — the two callers (dashboard's full panel vs.
// knowledge page's "save-and-go" flow) want different UX
// (dashboard shows the full result inline; this panel just shows
// a success notice + a link back), so a shared component would
// over-couple them.

// truncateForTitle caps a transcript snippet at a UI-friendly length
// for the Knowledge doc title. Mirrors the helper in the dashboard
// panel so doc titles stay consistent between the two entry points.
function truncateForTitle(s: string, max: number): string {
  const t = s.trim().replace(/\s+/g, ' ')
  if (t.length <= max) return t
  return t.slice(0, max) + '...'
}

// deconstructToMarkdown flattens a DeconstructResult into a
// human-readable Markdown body suitable for storing as a Knowledge
// doc. Matches the dashboard panel's output so the two entry
// points produce interchangeable docs.
function deconstructToMarkdown(
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
  lines.push(
    `- 是否存在: ${result.cta.present ? '是' : '否'}`,
  )
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

const PLATFORMS: { value: string; labelKey: string }[] = [
  { value: '', labelKey: 'dashboard.deconstruct.platform.any' },
  { value: '抖音', labelKey: 'dashboard.deconstruct.platform.douyin' },
  { value: '哔哩哔哩', labelKey: 'dashboard.deconstruct.platform.bilibili' },
  { value: '小红书', labelKey: 'dashboard.deconstruct.platform.xiaohongshu' },
]

interface KnowledgeDeconstructPanelProps {
  onClose: () => void
  onSaved: (doc: KnowledgeDoc) => void
}

export function KnowledgeDeconstructPanel({
  onClose,
  onSaved,
}: KnowledgeDeconstructPanelProps) {
  const t = useT()
  const [transcript, setTranscript] = useState('')
  const [platform, setPlatform] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [savedDoc, setSavedDoc] = useState<KnowledgeDoc | null>(null)
  const [demo, setDemo] = useState(false)

  async function handleSubmit() {
    const trimmed = transcript.trim()
    if (!trimmed) {
      setError(t('dashboard.deconstruct.error_required'))
      return
    }
    setError(null)
    setSavedDoc(null)
    setSubmitting(true)
    try {
      const metadata: { platform?: string } = {}
      if (platform) {
        metadata.platform = platform
      }
      const decon = await api.ai.deconstruct({
        transcript: trimmed,
        metadata,
      })
      setDemo(decon.demo)
      const titleText = truncateForTitle(trimmed, 30)
      const title = `爆款拆解: ${titleText}`
      const body = deconstructToMarkdown(
        { transcript: trimmed, platform },
        decon.data,
      )
      const doc = await api.knowledge.create({
        title,
        path: 'ai-deconstruct/auto',
        doc_type: '其它',
        content: body,
      })
      setSavedDoc(doc)
      onSaved(doc)
    } catch (err: unknown) {
      setError(
        err instanceof Error
          ? err.message
          : t('dashboard.deconstruct.action.save_failed'),
      )
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-2 sm:p-4"
      onClick={onClose}
    >
      <div
        className="bg-claude-canvas rounded-lg border border-claude-hairline shadow-claude-soft max-w-2xl w-full max-h-[90vh] sm:max-h-[85vh] overflow-y-auto"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="p-3 sm:p-4 border-b border-claude-hairline flex items-center justify-between gap-2">
          <h2 className="text-base sm:text-lg font-semibold text-claude-ink">
            {t('dashboard.deconstruct.title')}
          </h2>
          <button
            onClick={onClose}
            aria-label={t('dashboard.deconstruct.close')}
            className="text-claude-muted-soft hover:text-claude-ink text-2xl leading-none"
          >
            ×
          </button>
        </div>

        <div className="p-3 sm:p-4 space-y-4">
          <p className="text-xs md:text-sm text-claude-muted">
            {t('dashboard.deconstruct.subtitle')}
          </p>

          <div>
            <label className="block text-xs md:text-sm font-medium text-claude-ink">
              {t('dashboard.deconstruct.transcript_label')}
            </label>
            <textarea
              data-testid="knowledge-deconstruct-transcript"
              value={transcript}
              onChange={(e) => setTranscript(e.target.value)}
              placeholder={t('dashboard.deconstruct.transcript_placeholder')}
              rows={5}
              className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
            />
          </div>

          <div>
            <label className="block text-xs md:text-sm font-medium text-claude-ink">
              {t('dashboard.deconstruct.platform_label')}
            </label>
            <select
              data-testid="knowledge-deconstruct-platform"
              value={platform}
              onChange={(e) => setPlatform(e.target.value)}
              className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
            >
              {PLATFORMS.map((p) => (
                <option key={p.value} value={p.value}>
                  {t(p.labelKey)}
                </option>
              ))}
            </select>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <button
              type="button"
              data-testid="btn-knowledge-deconstruct-save"
              onClick={handleSubmit}
              disabled={submitting}
              className="px-3 md:px-4 py-1.5 md:py-2 text-xs md:text-sm bg-claude-success text-claude-on-primary rounded hover:opacity-90 disabled:opacity-50 transition-opacity"
            >
              {submitting
                ? t('dashboard.deconstruct.action.save_loading')
                : t('dashboard.deconstruct.action.save')}
            </button>
          </div>

          {error && (
            <div className="p-3 bg-claude-error/10 border border-claude-error text-claude-error rounded text-sm">
              {error}
            </div>
          )}

          {savedDoc && (
            <div
              data-testid="knowledge-deconstruct-saved"
              className="p-3 bg-claude-success/10 border border-claude-success/30 text-claude-success rounded text-xs md:text-sm"
            >
              {t('dashboard.deconstruct.action.save_success')}{' '}
              <a
                href="/knowledge"
                className="underline hover:opacity-80"
              >
                /knowledge
              </a>
            </div>
          )}

          {demo && (
            <div className="inline-flex items-center gap-1.5 px-2 py-1 text-xs font-medium rounded bg-claude-accent-amber/15 text-claude-accent-amber border border-claude-accent-amber/30">
              {t('dashboard.deconstruct.demo_badge')}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
