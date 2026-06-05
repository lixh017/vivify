'use client'

import { useState } from 'react'
import { api } from '@/lib/api'
import type {
  DeconstructResult,
  DeconstructArcPoint,
  ViralFormulaResult,
} from '@/lib/api'
import type { KnowledgeDoc } from '@/lib/types'
import { useT } from '@/lib/i18n-client'

// DeconstructPanel is the modal/dialog launched from the dashboard's
// "AI 拆解爆款" button. It runs the two-stage OpusClip-style flow:
//   1. POST /ai/deconstruct     — break transcript into structured
//      hook / structure / CTA / emotional arc / patterns
//   2. (optional) POST /ai/viral-formula — extract a named formula
//   3. (optional) POST /knowledge — persist the result for later
//      search
//
// The state machine is intentionally flat: a single `phase` union
// (closed | loading | result | error) keeps the modal easy to
// reason about, and a separate `formulaState` tracks the secondary
// viral-formula call so it can run after the main analysis without
// invalidating the deconstruct result.

// Platform values for the dropdown. The wire value uses the
// canonical backend names (Chinese for 抖音/哔哩哔哩/小红书; the
// empty string falls through to the prompt's "(未提供)" default).
const PLATFORMS: { value: string; labelKey: string }[] = [
  { value: '', labelKey: 'dashboard.deconstruct.platform.any' },
  { value: '抖音', labelKey: 'dashboard.deconstruct.platform.douyin' },
  { value: '哔哩哔哩', labelKey: 'dashboard.deconstruct.platform.bilibili' },
  { value: '小红书', labelKey: 'dashboard.deconstruct.platform.xiaohongshu' },
]

// Map of emotion (Chinese label) to a hex color used for the
// emotional-arc swatches. We pick a small palette of Claude-friendly
// emotions; unknown emotions fall back to muted gray so the chart
// never breaks.
const EMOTION_COLORS: Record<string, string> = {
  紧张: '#e85a4f',
  激动: '#e85a4f',
  愤怒: '#e85a4f',
  感动: '#f2a65a',
  惊喜: '#f2a65a',
  好奇: '#8da9c4',
  平静: '#8da9c4',
  舒缓: '#7d9b76',
  快乐: '#f2a65a',
  悲伤: '#6c7a89',
  恐惧: '#6c7a89',
}

const FALLBACK_EMOTION_COLOR = '#9ca3af'

// Map an overall_score (0-100) to a Tailwind color class. We use
// the existing Claude theme tokens so the big number reads as
// intentional in both light and dark modes.
function scoreColor(score: number): string {
  if (score >= 80) return 'text-claude-success'
  if (score >= 60) return 'text-claude-accent-teal'
  if (score >= 40) return 'text-claude-accent-amber'
  return 'text-claude-error'
}

// truncateForTitle caps a transcript snippet at a UI-friendly length
// for the Knowledge doc title. We pull from the start of the string
// (after trim) to mirror what a user would type if they were
// hand-naming the doc.
function truncateForTitle(s: string, max: number): string {
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

// EmotionalArcChart renders the timeline as a small horizontal
// strip. Each point becomes a circle positioned by time_pct and
// colored by emotion. Intensity drives the circle's vertical size
// (capped so the strip stays compact). Rendered in pure SVG so we
// don't pull in a charting library for one small visualization.
interface EmotionalArcChartProps {
  points: DeconstructArcPoint[]
}

function EmotionalArcChart({ points }: EmotionalArcChartProps) {
  const width = 480
  const height = 80
  const padding = 8
  if (points.length === 0) {
    return (
      <div className="text-xs text-claude-muted text-center py-4">
        —
      </div>
    )
  }
  const innerWidth = width - padding * 2
  const innerHeight = height - padding * 2
  const maxRadius = 14
  return (
    <svg
      viewBox={`0 0 ${width} ${height}`}
      className="w-full h-20"
      role="img"
      aria-label="emotional arc"
    >
      <line
        x1={padding}
        y1={height - padding}
        x2={width - padding}
        y2={height - padding}
        stroke="currentColor"
        strokeOpacity="0.15"
        strokeWidth="1"
      />
      {points.map((p, i) => {
        const x = padding + (p.time_pct / 100) * innerWidth
        const intensity = Math.max(0, Math.min(100, p.intensity))
        const r = (intensity / 100) * maxRadius + 2
        const color = EMOTION_COLORS[p.emotion] ?? FALLBACK_EMOTION_COLOR
        return (
          <g key={i}>
            <circle cx={x} cy={height - padding} r={r} fill={color} fillOpacity="0.7">
              <title>{`${p.emotion} · ${p.intensity} · ${p.time_pct}%`}</title>
            </circle>
            <text
              x={x}
              y={padding + 8}
              textAnchor="middle"
              fontSize="9"
              fill="currentColor"
              fillOpacity="0.7"
            >
              {p.emotion}
            </text>
          </g>
        )
      })}
    </svg>
  )
}

interface DeconstructPanelProps {
  onClose: () => void
}

export function DeconstructPanel({ onClose }: DeconstructPanelProps) {
  const t = useT()
  const [transcript, setTranscript] = useState('')
  const [platform, setPlatform] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [result, setResult] = useState<DeconstructResult | null>(null)
  const [demo, setDemo] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const [formula, setFormula] = useState<ViralFormulaResult | null>(null)
  const [formulaLoading, setFormulaLoading] = useState(false)
  const [formulaError, setFormulaError] = useState<string | null>(null)

  const [saving, setSaving] = useState(false)
  const [savedDoc, setSavedDoc] = useState<KnowledgeDoc | null>(null)
  const [saveError, setSaveError] = useState<string | null>(null)

  async function runDeconstruct() {
    const trimmed = transcript.trim()
    if (!trimmed) {
      setError(t('dashboard.deconstruct.error_required'))
      return
    }
    setError(null)
    setSaveError(null)
    setSavedDoc(null)
    setResult(null)
    setFormula(null)
    setFormulaError(null)
    setSubmitting(true)
    try {
      const metadata: {
        platform?: string
      } = {}
      if (platform) {
        metadata.platform = platform
      }
      const res = await api.ai.deconstruct({
        transcript: trimmed,
        metadata,
      })
      setResult(res.data)
      setDemo(res.demo)
    } catch (err: unknown) {
      setError(
        err instanceof Error
          ? err.message
          : t('dashboard.deconstruct.error_failed'),
      )
    } finally {
      setSubmitting(false)
    }
  }

  async function runFormula() {
    const trimmed = transcript.trim()
    if (!trimmed) {
      setFormulaError(t('dashboard.deconstruct.error_required'))
      return
    }
    setFormulaError(null)
    setFormulaLoading(true)
    try {
      const res = await api.ai.viralFormula({ transcript: trimmed })
      setFormula(res.data)
    } catch (err: unknown) {
      setFormulaError(
        err instanceof Error
          ? err.message
          : t('dashboard.deconstruct.formula.error_failed'),
      )
    } finally {
      setFormulaLoading(false)
    }
  }

  async function saveToKnowledge() {
    if (!result) return
    setSaving(true)
    setSaveError(null)
    try {
      const titleText = truncateForTitle(transcript, 30)
      const title = `爆款拆解: ${titleText}`
      const body = deconstructToMarkdown(
        { transcript, platform },
        result,
      )
      const doc = await api.knowledge.create({
        title,
        path: 'ai-deconstruct/auto',
        doc_type: '其它',
        content: body,
      })
      setSavedDoc(doc)
    } catch (err: unknown) {
      setSaveError(
        err instanceof Error
          ? err.message
          : t('dashboard.deconstruct.action.save_failed'),
      )
    } finally {
      setSaving(false)
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-2 sm:p-4"
      onClick={onClose}
    >
      <div
        className="bg-claude-canvas rounded-lg border border-claude-hairline shadow-claude-soft max-w-3xl w-full max-h-[90vh] sm:max-h-[85vh] overflow-y-auto"
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
              data-testid="deconstruct-transcript"
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
              data-testid="deconstruct-platform"
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
              data-testid="btn-deconstruct-submit"
              onClick={runDeconstruct}
              disabled={submitting}
              className="px-3 md:px-4 py-1.5 md:py-2 text-xs md:text-sm bg-claude-coral text-claude-on-primary rounded-md hover:bg-claude-coral-active disabled:opacity-50 transition-colors"
            >
              {submitting
                ? t('dashboard.deconstruct.submit_loading')
                : t('dashboard.deconstruct.submit_idle')}
            </button>
          </div>

          {error && (
            <div className="p-3 bg-claude-error/10 border border-claude-error text-claude-error rounded text-sm">
              {error}
            </div>
          )}

          {result && (
            <DeconstructResultView
              result={result}
              demo={demo}
              formula={formula}
              formulaLoading={formulaLoading}
              formulaError={formulaError}
              onRunFormula={runFormula}
              saving={saving}
              savedDoc={savedDoc}
              saveError={saveError}
              onSave={saveToKnowledge}
            />
          )}

          {!result && !submitting && !error && (
            <div className="p-3 bg-claude-surface-card border border-claude-hairline rounded text-xs md:text-sm text-claude-muted text-center">
              {t('dashboard.deconstruct.empty_state')}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

interface DeconstructResultViewProps {
  result: DeconstructResult
  demo: boolean
  formula: ViralFormulaResult | null
  formulaLoading: boolean
  formulaError: string | null
  onRunFormula: () => void
  saving: boolean
  savedDoc: KnowledgeDoc | null
  saveError: string | null
  onSave: () => void
}

function DeconstructResultView({
  result,
  demo,
  formula,
  formulaLoading,
  formulaError,
  onRunFormula,
  saving,
  savedDoc,
  saveError,
  onSave,
}: DeconstructResultViewProps) {
  const t = useT()
  return (
    <div className="space-y-4">
      {demo && (
        <div
          data-testid="deconstruct-demo-badge"
          className="inline-flex items-center gap-1.5 px-2 py-1 text-xs font-medium rounded bg-claude-accent-amber/15 text-claude-accent-amber border border-claude-accent-amber/30"
        >
          {t('dashboard.deconstruct.demo_badge')}
        </div>
      )}

      <section
        data-testid="deconstruct-section-overall"
        className="p-3 md:p-4 bg-claude-surface-card border border-claude-hairline rounded-lg"
      >
        <h3 className="text-xs font-semibold text-claude-muted">
          {t('dashboard.deconstruct.section.overall')}
        </h3>
        <p
          className={`font-serif text-5xl md:text-6xl mt-1 ${scoreColor(result.overall_score)}`}
        >
          {result.overall_score}
        </p>
      </section>

      <section
        data-testid="deconstruct-section-hook"
        className="p-3 md:p-4 bg-claude-surface-card border border-claude-hairline rounded-lg space-y-2"
      >
        <h3 className="text-sm font-semibold text-claude-ink">
          {t('dashboard.deconstruct.section.hook')}
        </h3>
        <div className="text-xs md:text-sm text-claude-body space-y-1">
          <p>
            <span className="font-semibold">{t('dashboard.deconstruct.hook.type')}:</span>{' '}
            {result.hook.type}
          </p>
          <p>
            <span className="font-semibold">{t('dashboard.deconstruct.hook.strength')}:</span>{' '}
            <span className={scoreColor(result.hook.strength)}>
              {result.hook.strength}
            </span>
          </p>
          <p>
            <span className="font-semibold">{t('dashboard.deconstruct.hook.text')}:</span>{' '}
            {result.hook.text}
          </p>
          <p>
            <span className="font-semibold">{t('dashboard.deconstruct.hook.analysis')}:</span>{' '}
            {result.hook.analysis}
          </p>
        </div>
      </section>

      <section
        data-testid="deconstruct-section-structure"
        className="p-3 md:p-4 bg-claude-surface-card border border-claude-hairline rounded-lg space-y-2"
      >
        <h3 className="text-sm font-semibold text-claude-ink">
          {t('dashboard.deconstruct.section.structure')}
        </h3>
        <div className="text-xs md:text-sm text-claude-body space-y-1">
          <p>
            <span className="font-semibold">{t('dashboard.deconstruct.structure.pattern')}:</span>{' '}
            {result.structure.pattern}
          </p>
          <p>
            <span className="font-semibold">{t('dashboard.deconstruct.structure.pacing')}:</span>{' '}
            {result.structure.pacing}
          </p>
          <p>
            <span className="font-semibold">{t('dashboard.deconstruct.structure.density')}:</span>{' '}
            <span className={scoreColor(result.structure.density)}>
              {result.structure.density}
            </span>
          </p>
        </div>
        {result.structure.beats.length > 0 && (
          <ul className="text-xs md:text-sm text-claude-body list-disc list-inside space-y-0.5">
            {result.structure.beats.map((b, i) => (
              <li key={i}>
                <span className="text-claude-muted">{b.time_pct}%</span> ·{' '}
                <span className="font-medium">{b.role}</span> · {b.description}
              </li>
            ))}
          </ul>
        )}
      </section>

      <section
        data-testid="deconstruct-section-cta"
        className="p-3 md:p-4 bg-claude-surface-card border border-claude-hairline rounded-lg space-y-2"
      >
        <h3 className="text-sm font-semibold text-claude-ink">
          {t('dashboard.deconstruct.section.cta')}
        </h3>
        {result.cta.present ? (
          <div className="text-xs md:text-sm text-claude-body space-y-1">
            <p className="text-claude-success font-medium">
              {t('dashboard.deconstruct.cta.present_yes')}
            </p>
            <p>
              <span className="font-semibold">{t('dashboard.deconstruct.cta.type')}:</span>{' '}
              {result.cta.type}
            </p>
            <p>
              <span className="font-semibold">{t('dashboard.deconstruct.cta.placement')}:</span>{' '}
              {result.cta.placement}
            </p>
          </div>
        ) : (
          <p className="text-xs md:text-sm text-claude-muted">
            {t('dashboard.deconstruct.cta.present_no')}
          </p>
        )}
      </section>

      <section
        data-testid="deconstruct-section-emotion"
        className="p-3 md:p-4 bg-claude-surface-card border border-claude-hairline rounded-lg space-y-2"
      >
        <h3 className="text-sm font-semibold text-claude-ink">
          {t('dashboard.deconstruct.section.emotion')}
        </h3>
        {result.emotional_arc.length === 0 ? (
          <p className="text-xs md:text-sm text-claude-muted">
            {t('dashboard.deconstruct.emotion.empty')}
          </p>
        ) : (
          <>
            <EmotionalArcChart points={result.emotional_arc} />
            <ul className="text-xs text-claude-body grid grid-cols-2 sm:grid-cols-3 gap-1">
              {result.emotional_arc.map((p, i) => (
                <li key={i} className="flex items-center gap-2">
                  <span
                    className="inline-block w-2 h-2 rounded-full shrink-0"
                    style={{
                      backgroundColor:
                        EMOTION_COLORS[p.emotion] ?? FALLBACK_EMOTION_COLOR,
                    }}
                  />
                  <span className="text-claude-muted">{p.time_pct}%</span>
                  <span className="font-medium">{p.emotion}</span>
                  <span className="text-claude-muted-soft">
                    {p.intensity}
                  </span>
                </li>
              ))}
            </ul>
          </>
        )}
      </section>

      <section
        data-testid="deconstruct-section-patterns"
        className="p-3 md:p-4 bg-claude-surface-card border border-claude-hairline rounded-lg space-y-2"
      >
        <h3 className="text-sm font-semibold text-claude-ink">
          {t('dashboard.deconstruct.section.patterns')}
        </h3>
        {result.reusable_patterns.length === 0 ? (
          <p className="text-xs md:text-sm text-claude-muted">
            {t('dashboard.deconstruct.patterns.empty')}
          </p>
        ) : (
          <ul className="text-xs md:text-sm text-claude-body list-disc list-inside space-y-1">
            {result.reusable_patterns.map((p, i) => (
              <li key={i}>{p}</li>
            ))}
          </ul>
        )}
      </section>

      <section
        data-testid="deconstruct-section-platform-fit"
        className="p-3 md:p-4 bg-claude-surface-card border border-claude-hairline rounded-lg space-y-2"
      >
        <h3 className="text-sm font-semibold text-claude-ink">
          {t('dashboard.deconstruct.section.platform_fit')}
        </h3>
        <ul className="text-xs md:text-sm text-claude-body space-y-2">
          <li>
            <span className="font-semibold">
              {t('dashboard.deconstruct.platform.douyin')}:
            </span>{' '}
            {result.platform_fit_notes.抖音}
          </li>
          <li>
            <span className="font-semibold">
              {t('dashboard.deconstruct.platform.bilibili')}:
            </span>{' '}
            {result.platform_fit_notes.哔哩哔哩}
          </li>
          <li>
            <span className="font-semibold">
              {t('dashboard.deconstruct.platform.xiaohongshu')}:
            </span>{' '}
            {result.platform_fit_notes.小红书}
          </li>
        </ul>
      </section>

      <div className="flex flex-wrap items-center gap-2 pt-2 border-t border-claude-hairline">
        <button
          type="button"
          data-testid="btn-deconstruct-formula"
          onClick={onRunFormula}
          disabled={formulaLoading}
          className="px-3 md:px-4 py-1.5 md:py-2 text-xs md:text-sm bg-claude-accent-teal text-white rounded hover:opacity-90 disabled:opacity-50 transition-opacity"
        >
          {formulaLoading
            ? t('dashboard.deconstruct.action.formula_loading')
            : t('dashboard.deconstruct.action.formula')}
        </button>
        <button
          type="button"
          data-testid="btn-deconstruct-save"
          onClick={onSave}
          disabled={saving}
          className="px-3 md:px-4 py-1.5 md:py-2 text-xs md:text-sm bg-claude-success text-claude-on-primary rounded hover:opacity-90 disabled:opacity-50 transition-opacity"
        >
          {saving
            ? t('dashboard.deconstruct.action.save_loading')
            : t('dashboard.deconstruct.action.save')}
        </button>
      </div>

      {saveError && (
        <div className="p-3 bg-claude-error/10 border border-claude-error text-claude-error rounded text-sm">
          {saveError}
        </div>
      )}

      {savedDoc && (
        <div
          data-testid="deconstruct-save-success"
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

      {formulaError && (
        <div className="p-3 bg-claude-error/10 border border-claude-error text-claude-error rounded text-sm">
          {formulaError}
        </div>
      )}

      {formula && <FormulaView formula={formula} />}
    </div>
  )
}

interface FormulaViewProps {
  formula: ViralFormulaResult
}

function FormulaView({ formula }: FormulaViewProps) {
  const t = useT()
  return (
    <section
      data-testid="deconstruct-section-formula"
      className="p-3 md:p-4 bg-claude-accent-teal/5 border border-claude-accent-teal/30 rounded-lg space-y-3"
    >
      <h3 className="text-sm font-semibold text-claude-ink">
        {t('dashboard.deconstruct.formula.title')}
      </h3>
      <p className="font-serif text-lg md:text-xl text-claude-accent-teal">
        {formula.formula_name}
      </p>
      {formula.variables.length > 0 && (
        <div>
          <h4 className="text-xs font-semibold text-claude-muted mb-1">
            {t('dashboard.deconstruct.formula.variables')}
          </h4>
          <ul className="text-xs md:text-sm text-claude-body space-y-1">
            {formula.variables.map((v, i) => (
              <li key={i} className="flex items-baseline gap-2">
                <span className="px-1.5 py-0.5 bg-claude-canvas border border-claude-hairline rounded text-claude-accent-teal text-xs">
                  {v.weight}
                </span>
                <span className="font-medium">{v.name}</span>
                <span className="text-claude-muted-soft">—</span>
                <span className="text-claude-body">{v.example}</span>
              </li>
            ))}
          </ul>
        </div>
      )}
      {formula.steps.length > 0 && (
        <div>
          <h4 className="text-xs font-semibold text-claude-muted mb-1">
            {t('dashboard.deconstruct.formula.steps')}
          </h4>
          <ol className="text-xs md:text-sm text-claude-body list-decimal list-inside space-y-1">
            {formula.steps.map((s, i) => (
              <li key={i}>{s}</li>
            ))}
          </ol>
        </div>
      )}
      {formula.example_application && (
        <div>
          <h4 className="text-xs font-semibold text-claude-muted mb-1">
            {t('dashboard.deconstruct.formula.example')}
          </h4>
          <p className="text-xs md:text-sm text-claude-body whitespace-pre-wrap">
            {formula.example_application}
          </p>
        </div>
      )}
      {formula.variations.length > 0 && (
        <div>
          <h4 className="text-xs font-semibold text-claude-muted mb-1">
            {t('dashboard.deconstruct.formula.variations')}
          </h4>
          <ul className="text-xs md:text-sm text-claude-body list-disc list-inside space-y-1">
            {formula.variations.map((v, i) => (
              <li key={i}>{v}</li>
            ))}
          </ul>
        </div>
      )}
    </section>
  )
}
