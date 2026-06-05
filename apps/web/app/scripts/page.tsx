'use client'

import { useEffect, useState, FormEvent } from 'react'
import { api, type QualityScore } from '@/lib/api'
import type { Script } from '@/lib/types'
import { useT } from '@/lib/i18n-client'

// Canonical platform vocabulary mirroring the backend's allowedPlatforms
// set. These are the API wire values; the user-facing label is rendered
// via the i18n catalog. Switching locale does not change what the
// backend stores.
const PLATFORMS = ['抖音', '小红书', 'B站', '视频号', 'YouTube']

interface FormState {
  title: string
  content: string
  platform: string
}

const EMPTY_FORM: FormState = {
  title: '',
  content: '',
  platform: PLATFORMS[0],
}

// QualityBand maps a 0-100 score to a colour band used by the badge
// chip. The thresholds match the spec: 80+ green, 60-79 amber, else
// red. Keeping this pure (no Tailwind strings inline) means the
// badge logic stays test-friendly and the colour decision is
// centralised.
type QualityBand = 'green' | 'amber' | 'red'

function bandForScore(score: number): QualityBand {
  if (score >= 80) return 'green'
  if (score >= 60) return 'amber'
  return 'red'
}

function bandIcon(band: QualityBand): string {
  if (band === 'green') return '✨'
  if (band === 'amber') return '⚠️'
  return '❌'
}

function bandClass(band: QualityBand): string {
  if (band === 'green') {
    return 'bg-claude-success/15 text-claude-success border-claude-success/30'
  }
  if (band === 'amber') {
    return 'bg-claude-accent-amber/15 text-claude-accent-amber border-claude-accent-amber/30'
  }
  return 'bg-claude-error/10 text-claude-error border-claude-error/30'
}

export default function ScriptsPage() {
  const t = useT()
  const [scripts, setScripts] = useState<Script[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState<FormState>(EMPTY_FORM)
  const [submitting, setSubmitting] = useState(false)
  const [filterPlatform, setFilterPlatform] = useState<string>('')
  const [humanizing, setHumanizing] = useState(false)
  const [toast, setToast] = useState<string | null>(null)

  // Per-script quality cache. Keyed by script ID so the badge sticks
  // around after the panel closes — the user can score several
  // scripts and see all the badges at a glance. We deliberately do
  // NOT persist this server-side: scoring is on-demand, not a
  // stored field on Script (the cron job for batch scoring is out
  // of scope for Phase 1).
  const [scoreCache, setScoreCache] = useState<Record<number, QualityScore>>({})
  const [scoringIds, setScoringIds] = useState<Set<number>>(new Set())
  // Open quality panel state — we let the user expand exactly one
  // panel at a time to keep the page from turning into a wall of
  // suggestions.
  const [openPanelId, setOpenPanelId] = useState<number | null>(null)
  const [applyingId, setApplyingId] = useState<number | null>(null)
  const [demoIds, setDemoIds] = useState<Set<number>>(new Set())

  async function loadScripts() {
    setLoading(true)
    setError(null)
    try {
      const params: { platform?: string } = {}
      if (filterPlatform) params.platform = filterPlatform
      const res = await api.scripts.list(params)
      setScripts(res.items)
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t('scripts.error.load'))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadScripts()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filterPlatform])

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setSubmitting(true)
    setError(null)
    try {
      await api.scripts.create({
        title: form.title,
        content: form.content,
        platform: form.platform,
      })
      setForm(EMPTY_FORM)
      setShowForm(false)
      await loadScripts()
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t('scripts.error.create'))
    } finally {
      setSubmitting(false)
    }
  }

  // deleteScript removes a script via the API. We optimistically drop
  // it from the list and roll back on failure. The scriptId disabling
  // pattern mirrors the topics page so a fast double-click can't
  // double-fire a delete.
  async function deleteScript(id: number) {
    const previous = scripts
    setScripts((prev) => prev.filter((s) => s.id !== id))
    try {
      await api.scripts.delete(id)
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t('scripts.error.delete'))
      setScripts(previous)
    }
  }

  // handleHumanize asks Claude to rewrite the current script content
  // in a more human, less "AI-tinted" voice. We confirm first because
  // the action overwrites the textarea. Toast surfaces server failures
  // (e.g. 503 when no API key is configured).
  async function handleHumanize() {
    const content = form.content.trim()
    if (!content) {
      setToast(t('scripts.humanize_required'))
      return
    }
    if (!window.confirm(t('scripts.humanize_confirm'))) {
      return
    }
    setHumanizing(true)
    setToast(null)
    try {
      const res = await api.ai.humanize({ script: content })
      setForm((prev) => ({ ...prev, content: res.data.humanized }))
      if (res.demo) {
        setToast(t('scripts.humanize_toast_demo'))
      }
    } catch (err: unknown) {
      setToast(err instanceof Error ? err.message : t('scripts.humanize_failed'))
    } finally {
      setHumanizing(false)
    }
  }

  // scoreScript fires /ai/score for a single script and stores the
  // result in scoreCache. If the panel is closed we open it so the
  // user immediately sees the breakdown; if it was already open for
  // the same script we keep it open and just refresh the contents.
  async function scoreScript(script: Script) {
    if (scoringIds.has(script.id)) return
    setScoringIds((prev) => {
      const next = new Set(prev)
      next.add(script.id)
      return next
    })
    setError(null)
    try {
      const res = await api.ai.score({
        title: script.title,
        script: script.content,
        platform: script.platform,
      })
      setScoreCache((prev) => ({ ...prev, [script.id]: res.data }))
      setDemoIds((prev) => {
        const next = new Set(prev)
        if (res.demo) {
          next.add(script.id)
        } else {
          next.delete(script.id)
        }
        return next
      })
      setOpenPanelId(script.id)
    } catch (err: unknown) {
      setError(
        err instanceof Error ? err.message : t('scripts.quality.error_failed'),
      )
    } finally {
      setScoringIds((prev) => {
        const next = new Set(prev)
        next.delete(script.id)
        return next
      })
    }
  }

  // applySuggestions re-runs /ai/humanize on the current script
  // content prefixed with the AI suggestions as a soft brief. We
  // re-use humanize rather than build a new endpoint because the
  // existing humanize path already wraps Claude with the same
  // safety net (demo fallback when no key is configured).
  async function applySuggestions(script: Script) {
    const cached = scoreCache[script.id]
    if (!cached) return
    if (applyingId === script.id) return
    if (!window.confirm(t('scripts.quality.apply_confirm'))) return

    setApplyingId(script.id)
    setError(null)
    try {
      const suggestionLines = cached.suggestions
        .map((s) => {
          // Phase-1 wire shape: each suggestion is a "before/after"
          // pair (problem → rewrite). When rewrite is empty, fall
          // back to the problem so the humanize prompt still gets
          // actionable copy.
          const fix = s.rewrite ? `${s.problem} → ${s.rewrite}` : s.problem
          return `- (${s.category}/${s.severity}) ${fix}`
        })
        .join('\n')
      const brief = suggestionLines
        ? `请按以下建议改写脚本:\n${suggestionLines}\n\n原始脚本:\n${script.content}`
        : script.content

      const res = await api.ai.humanize({ script: brief })
      const next = res.data.humanized
      const previous = scripts
      setScripts((prev) =>
        prev.map((s) => (s.id === script.id ? { ...s, content: next } : s)),
      )
      try {
        const updated = await api.scripts.update(script.id, { content: next })
        setScripts((prev) =>
          prev.map((s) => (s.id === script.id ? updated : s)),
        )
        if (res.demo) {
          setToast(t('scripts.humanize_toast_demo'))
        }
      } catch (err: unknown) {
        setScripts(previous)
        setError(err instanceof Error ? err.message : t('scripts.error.create'))
      }
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t('scripts.humanize_failed'))
    } finally {
      setApplyingId(null)
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-end justify-between flex-wrap gap-3">
        <div>
          <h1 className="font-serif text-3xl text-claude-ink tracking-tight">
            📝 {t('scripts.title')}
          </h1>
          <p className="text-claude-muted text-sm mt-1">
            {t('scripts.subhead')}
          </p>
        </div>
        <button
          onClick={() => setShowForm((v) => !v)}
          className="px-3 md:px-4 py-1.5 md:py-2 text-xs md:text-sm bg-claude-coral text-claude-on-primary rounded-md hover:bg-claude-coral-active transition-colors"
        >
          {showForm ? t('common.cancel') : t('scripts.create')}
        </button>
      </div>

      <div className="flex gap-3 items-center">
        <label className="text-xs md:text-sm text-claude-body flex items-center gap-2">
          {t('scripts.filter.platform')}:
          <select
            value={filterPlatform}
            onChange={(e) => setFilterPlatform(e.target.value)}
            className="px-2 py-1 text-xs md:text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink"
          >
            <option value="">{t('scripts.filter.all')}</option>
            {PLATFORMS.map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
          </select>
        </label>
      </div>

      {showForm && (
        <form
          onSubmit={handleSubmit}
          className="p-3 md:p-4 bg-claude-surface-card rounded-lg border border-claude-hairline space-y-3"
        >
          <div>
            <label className="block text-xs md:text-sm font-medium text-claude-ink">
              {t('scripts.form.title_label')}
            </label>
            <input
              type="text"
              required
              data-testid="input-test-title"
              value={form.title}
              onChange={(e) => setForm({ ...form, title: e.target.value })}
              className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
            />
          </div>
          <div>
            <div className="flex items-center justify-between flex-wrap gap-2">
              <label className="block text-xs md:text-sm font-medium text-claude-ink">
                {t('scripts.form.content_label')}
              </label>
              <button
                type="button"
                data-testid="btn-ai-humanize"
                onClick={handleHumanize}
                disabled={humanizing || !form.content.trim()}
                className="px-2.5 md:px-3 py-1 text-xs md:text-sm bg-claude-accent-amber text-white rounded hover:opacity-90 disabled:opacity-50 transition-opacity"
                title={t('scripts.humanize_tooltip')}
              >
                {humanizing
                  ? t('scripts.humanize_loading')
                  : t('scripts.humanize_idle')}
              </button>
            </div>
            <textarea
              required
              data-testid="input-test-content"
              value={form.content}
              onChange={(e) => setForm({ ...form, content: e.target.value })}
              className="mt-1 w-full px-3 py-2 border border-claude-hairline rounded bg-claude-canvas text-claude-ink font-mono text-xs md:text-sm focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
              rows={10}
            />
          </div>
          <div>
            <label className="block text-xs md:text-sm font-medium text-claude-ink">
              {t('scripts.form.platform_label')}
            </label>
            <select
              data-testid="input-test-platform"
              value={form.platform}
              onChange={(e) => setForm({ ...form, platform: e.target.value })}
              className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink"
            >
              {PLATFORMS.map((p) => (
                <option key={p} value={p}>
                  {p}
                </option>
              ))}
            </select>
          </div>
          <button
            type="submit"
            data-testid="btn-submit"
            disabled={submitting}
            className="w-full sm:w-auto px-4 py-2 text-sm bg-claude-coral text-claude-on-primary rounded-md hover:bg-claude-coral-active disabled:opacity-50 transition-colors"
          >
            {submitting
              ? t('scripts.form.submit_loading')
              : t('scripts.form.submit_idle')}
          </button>
        </form>
      )}

      {error && (
        <div className="p-3 bg-claude-error/10 border border-claude-error text-claude-error rounded text-sm">
          {error}
        </div>
      )}

      {toast && (
        <div className="p-3 bg-claude-surface-soft border border-claude-warning text-claude-warning rounded text-sm">
          {toast}
        </div>
      )}

      {loading ? (
        <div className="text-sm text-claude-body">{t('common.loading')}</div>
      ) : scripts.length === 0 ? (
        <div className="p-4 bg-claude-surface-card rounded-lg border border-claude-hairline text-sm text-claude-muted text-center">
          {t('scripts.empty')}
        </div>
      ) : (
        <div className="space-y-3">
          {scripts.map((s) => {
            const cached = scoreCache[s.id]
            const isScoring = scoringIds.has(s.id)
            const isApplying = applyingId === s.id
            const panelOpen = openPanelId === s.id && !!cached
            const isDemo = demoIds.has(s.id)
            return (
              <div
                key={s.id}
                data-testid="list-item"
                data-script-id={s.id}
                className="p-3 md:p-4 bg-claude-surface-card rounded-lg border border-claude-hairline hover:border-claude-coral/50 transition-colors"
              >
                <div className="flex items-start justify-between gap-3 flex-col sm:flex-row">
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2 flex-wrap">
                      <h2 className="font-semibold text-sm md:text-base text-claude-ink">
                        {s.title}
                      </h2>
                      {cached && (
                        <button
                          type="button"
                          data-testid={`quality-badge-${s.id}`}
                          aria-label={t('scripts.quality.badge_label')}
                          onClick={() =>
                            setOpenPanelId((prev) =>
                              prev === s.id ? null : s.id,
                            )
                          }
                          className={`inline-flex items-center gap-1 px-2 py-0.5 text-xs font-medium rounded-full border ${bandClass(
                            bandForScore(cached.overall_score),
                          )} hover:opacity-90 transition-opacity`}
                        >
                          <span aria-hidden>
                            {bandIcon(bandForScore(cached.overall_score))}
                          </span>
                          {cached.overall_score}
                        </button>
                      )}
                    </div>
                    <p className="text-xs md:text-sm text-claude-body mt-1 line-clamp-3 whitespace-pre-wrap font-mono">
                      {s.content}
                    </p>
                  </div>
                  <div className="flex gap-2 text-xs flex-wrap">
                    <span className="px-2 py-1 bg-claude-canvas text-claude-coral rounded">
                      {s.platform}
                    </span>
                    {s.word_count > 0 && (
                      <span className="px-2 py-1 bg-claude-canvas text-claude-body rounded">
                        {s.word_count} {t('scripts.empty_word')}
                      </span>
                    )}
                    <button
                      type="button"
                      data-testid={`btn-score-${s.id}`}
                      onClick={() => scoreScript(s)}
                      disabled={isScoring}
                      className="px-2 py-1 bg-claude-accent-teal/15 hover:bg-claude-accent-teal/25 text-claude-accent-teal rounded border border-claude-accent-teal/30 disabled:opacity-50"
                    >
                      {isScoring
                        ? t('scripts.quality.scoring')
                        : t('scripts.quality.score_button')}
                    </button>
                    <button
                      type="button"
                      data-testid={`btn-delete-${s.id}`}
                      onClick={() => deleteScript(s.id)}
                      aria-label={t('scripts.aria.delete')}
                      className="px-2 py-1 bg-claude-canvas hover:bg-claude-surface-soft text-claude-error rounded border border-claude-hairline"
                    >
                      {t('common.delete')}
                    </button>
                  </div>
                </div>

                {panelOpen && cached && (
                  <div
                    data-testid={`quality-panel-${s.id}`}
                    className="mt-3 pt-3 border-t border-claude-hairline-soft space-y-3"
                  >
                    <div className="flex items-center justify-between flex-wrap gap-2">
                      <h3 className="font-semibold text-sm text-claude-ink">
                        {t('scripts.quality.panel_title')}
                      </h3>
                      <button
                        type="button"
                        onClick={() => setOpenPanelId(null)}
                        className="text-xs text-claude-muted hover:text-claude-ink"
                      >
                        {t('scripts.quality.close')}
                      </button>
                    </div>
                    {isDemo && (
                      <div className="inline-flex items-center gap-1.5 px-2 py-1 text-xs font-medium rounded bg-claude-accent-amber/15 text-claude-accent-amber border border-claude-accent-amber/30">
                        {t('scripts.quality.demo_badge')}
                      </div>
                    )}
                    <div className="grid grid-cols-2 md:grid-cols-4 gap-2">
                      <ScoreCell
                        label={t('scripts.quality.overall')}
                        value={cached.overall_score}
                      />
                      <ScoreCell
                        label={t('scripts.quality.hook')}
                        value={cached.hook_strength}
                      />
                      <ScoreCell
                        label={t('scripts.quality.structure')}
                        value={cached.structure}
                      />
                      <ScoreCell
                        label={t('scripts.quality.platform_fit')}
                        value={cached.platform_fit}
                      />
                    </div>
                    {cached.rewritten_hook && (
                      <div className="p-3 bg-claude-canvas border border-claude-hairline-soft rounded text-xs md:text-sm">
                        <div className="font-medium text-claude-ink mb-1">
                          {t('scripts.quality.rewritten_hook')}
                        </div>
                        <div className="text-claude-body whitespace-pre-wrap">
                          {cached.rewritten_hook}
                        </div>
                      </div>
                    )}
                    <div>
                      <div className="font-medium text-xs text-claude-ink mb-2">
                        {t('scripts.quality.suggestions')}
                      </div>
                      {cached.suggestions.length === 0 ? (
                        <div className="text-xs text-claude-muted italic">
                          {t('scripts.quality.empty_suggestions')}
                        </div>
                      ) : (
                        <ul className="space-y-1.5">
                          {cached.suggestions.map((sg, i) => (
                            <li
                              key={`${sg.category}-${i}`}
                              className="flex gap-2 text-xs md:text-sm text-claude-body"
                            >
                              <SeverityChip severity={sg.severity} />
                              <span className="flex-1">
                                <span className="font-medium text-claude-ink">
                                  {sg.category}
                                </span>
                                <span className="text-claude-muted"> · </span>
                                {sg.problem}
                                {sg.rewrite && (
                                  <>
                                    <span className="text-claude-muted">
                                      {' '}
                                      →{' '}
                                    </span>
                                    <span className="text-claude-ink">
                                      {sg.rewrite}
                                    </span>
                                  </>
                                )}
                              </span>
                            </li>
                          ))}
                        </ul>
                      )}
                    </div>
                    <div className="flex justify-end">
                      <button
                        type="button"
                        data-testid={`btn-apply-${s.id}`}
                        onClick={() => applySuggestions(s)}
                        disabled={isApplying}
                        className="px-3 py-1.5 text-xs md:text-sm bg-claude-coral text-claude-on-primary rounded hover:bg-claude-coral-active disabled:opacity-50 transition-colors"
                      >
                        {isApplying
                          ? t('scripts.quality.apply_loading')
                          : t('scripts.quality.apply_suggestions')}
                      </button>
                    </div>
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}

interface ScoreCellProps {
  label: string
  value: number
}

function ScoreCell({ label, value }: ScoreCellProps) {
  const band = bandForScore(value)
  return (
    <div
      className={`p-2 rounded border ${bandClass(band)} flex flex-col items-center`}
    >
      <div className="text-[10px] uppercase tracking-wide opacity-80">
        {label}
      </div>
      <div className="text-lg font-semibold leading-tight">{value}</div>
    </div>
  )
}

interface SeverityChipProps {
  severity: 'low' | 'medium' | 'high'
}

function SeverityChip({ severity }: SeverityChipProps) {
  const t = useT()
  const cls =
    severity === 'high'
      ? 'bg-claude-error/15 text-claude-error border-claude-error/30'
      : severity === 'medium'
        ? 'bg-claude-accent-amber/15 text-claude-accent-amber border-claude-accent-amber/30'
        : 'bg-claude-surface-soft text-claude-muted border-claude-hairline'
  const label =
    severity === 'high'
      ? t('scripts.quality.severity.high')
      : severity === 'medium'
        ? t('scripts.quality.severity.medium')
        : t('scripts.quality.severity.low')
  return (
    <span
      className={`shrink-0 inline-flex items-center justify-center px-1.5 py-0.5 text-[10px] font-medium rounded border ${cls}`}
    >
      {label}
    </span>
  )
}
