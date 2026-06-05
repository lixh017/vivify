'use client'

import { useEffect, useState, FormEvent, Suspense } from 'react'
import { useSearchParams } from 'next/navigation'
import { api, type GeneratedTopic, type PlatformAdapt } from '@/lib/api'
import type { Topic } from '@/lib/types'
import { useT } from '@/lib/i18n-client'

// Canonical platform + status vocabulary, mirroring the backend's
// allowedStatuses / allowedPlatforms sets (see apps/api/internal/models/topic.go).
// These are the API wire values; the user-facing label is rendered via the
// i18n catalog and keyed off the raw value. Switching to English locale
// does not change what the backend stores — only the surrounding chrome.
const PLATFORMS = ['抖音', '小红书', 'B站', '视频号', 'YouTube']
const STATUSES = ['想法', '评估', '待写', '撰写中', '已发布'] as const

// The 4-column kanban view as specified in section 4.9.1 of the IP design.
// Topics with status "撰写中" are bucketed under 待写 so the columns line up
// with the spec's 想法 / 评估 / 待写 / 已发布 layout.
const KANBAN_COLUMNS: { status: string; next: string | null }[] = [
  { status: '想法', next: '评估' },
  { status: '评估', next: '待写' },
  { status: '待写', next: '撰写中' },
  { status: '已发布', next: null },
]

// "撰写中" doesn't have a dedicated column, so we surface it as a thin row
// between 待写 and 已发布. It advances to 已发布 when the user moves it.
const WRITING_STATUS = '撰写中'

// statusPillClass maps a status to a (semantic) pill color so the kanban
// and list views stay legible. The pill uses the warm / cool status tokens
// from the Claude palette rather than ad-hoc colors.
function statusPillClass(status: string): string {
  switch (status) {
    case '想法':
      return 'bg-claude-surface-soft text-claude-muted'
    case '评估':
      return 'bg-claude-accent-teal/15 text-claude-accent-teal'
    case '待写':
      return 'bg-claude-accent-amber/15 text-claude-accent-amber'
    case '撰写中':
      return 'bg-claude-accent-amber/20 text-claude-accent-amber'
    case '已发布':
      return 'bg-claude-success/15 text-claude-success'
    default:
      return 'bg-claude-surface-soft text-claude-muted'
  }
}

interface FormState {
  title: string
  angle: string
  platform: string
  status: string
}

const EMPTY_FORM: FormState = {
  title: '',
  angle: '',
  platform: PLATFORMS[0],
  status: STATUSES[0],
}

type ViewMode = 'kanban' | 'list'

interface KanbanCardProps {
  topic: Topic
  onMove: (id: number, nextStatus: string) => void
  onStatusChange: (id: number, status: string) => void
  moving: boolean
  onDelete?: (id: number) => void
  onAdapt?: (id: number) => void
  adapting?: boolean
}

function KanbanCard({
  topic,
  onMove,
  onStatusChange,
  moving,
  onDelete,
  onAdapt,
  adapting,
}: KanbanCardProps) {
  const t = useT()
  return (
    <div
      data-testid={`topic-card-${topic.id}`}
      className="p-3 bg-claude-surface-card rounded-md border border-claude-hairline space-y-2"
    >
      <div className="font-semibold text-sm text-claude-ink leading-snug">
        {topic.title}
      </div>
      {topic.angle && (
        <p className="text-xs text-claude-body line-clamp-3">{topic.angle}</p>
      )}
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs px-2 py-0.5 bg-claude-canvas text-claude-coral rounded">
          {topic.platform}
        </span>
        <span
          className={`rounded-full px-2 py-0.5 text-xs font-medium ${statusPillClass(topic.status)}`}
        >
          {topic.status}
        </span>
        <select
          aria-label={t('topics.aria.change_status')}
          value={topic.status}
          disabled={moving}
          onChange={(e) => onStatusChange(topic.id, e.target.value)}
          className="text-xs px-1 py-0.5 border border-claude-hairline rounded bg-claude-canvas text-claude-ink"
        >
          {STATUSES.map((s) => (
            <option key={s} value={s}>
              {s}
            </option>
          ))}
        </select>
      </div>
      <div className="flex gap-1 flex-wrap">
        <button
          type="button"
          onClick={() => {
            // The dropdown above handles the generic case; this shortcut moves
            // the card to the next column. The column definition supplies the
            // next status so we don't hardcode ordering in the card itself.
            const col = KANBAN_COLUMNS.find((c) => c.status === topic.status)
            if (col?.next) onMove(topic.id, col.next)
          }}
          disabled={moving}
          className="flex-1 text-xs px-2 py-1 bg-claude-canvas hover:bg-claude-surface-soft text-claude-body rounded border border-claude-hairline disabled:opacity-50"
        >
          →
        </button>
        {onAdapt && (
          <button
            type="button"
            data-testid={`btn-adapt-${topic.id}`}
            onClick={() => onAdapt(topic.id)}
            disabled={moving || adapting}
            className="text-xs px-2 py-1 bg-claude-accent-teal/15 hover:bg-claude-accent-teal/25 text-claude-accent-teal rounded border border-claude-accent-teal/30 disabled:opacity-50"
          >
            {adapting ? t('topics.adapt.loading') : t('topics.adapt.button')}
          </button>
        )}
        {onDelete && (
          <button
            type="button"
            data-testid={`btn-delete-${topic.id}`}
            onClick={() => onDelete(topic.id)}
            disabled={moving}
            aria-label={t('topics.aria.delete')}
            className="text-xs px-2 py-1 bg-claude-canvas hover:bg-claude-surface-soft text-claude-error rounded border border-claude-hairline disabled:opacity-50"
          >
            {t('common.delete')}
          </button>
        )}
      </div>
    </div>
  )
}

export default function TopicsPage() {
  return (
    <Suspense fallback={<div className="text-sm text-claude-body">Loading…</div>}>
      <TopicsPageInner />
    </Suspense>
  )
}

function TopicsPageInner() {
  const t = useT()
  // demoFromUrl reflects ?demo=true in the address bar. When true we
  // force every /ai/* call through demo mode regardless of whether
  // the server has an API key configured. The flag is read on mount
  // and not reactive — sales demos navigate here directly, they don't
  // toggle the URL mid-session.
  const searchParams = useSearchParams()
  const demoFromUrl =
    (searchParams.get('demo') || '').toLowerCase() === 'true'

  // Platforms are a small fixed set in Phase 1 (mirroring the backend's
  // allowedPlatforms). When the backend grows a /platforms endpoint we
  // can swap this for a fetched list without touching the JSX.
  const [platforms] = useState<string[]>(PLATFORMS)
  const [topics, setTopics] = useState<Topic[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState<FormState>(EMPTY_FORM)
  const [submitting, setSubmitting] = useState(false)
  const [filterPlatform, setFilterPlatform] = useState<string>('')
  const [filterStatus, setFilterStatus] = useState<string>('')
  const [view, setView] = useState<ViewMode>('kanban')
  const [movingIds, setMovingIds] = useState<Set<number>>(new Set())

  // AI topic generation state. aiModalOpen drives a small inline
  // panel for seed/count; aiResult holds the latest batch; aiError
  // surfaces server failures (e.g. 503 when no API key configured).
  const [aiModalOpen, setAiModalOpen] = useState(false)
  const [aiSeed, setAiSeed] = useState('')
  const [aiCount, setAiCount] = useState(5)
  const [aiLoading, setAiLoading] = useState(false)
  const [aiResult, setAiResult] = useState<GeneratedTopic[] | null>(null)
  const [aiError, setAiError] = useState<string | null>(null)
  // aiDemoActive is true when the latest AI response was served
  // from canned data. The badge below the AI form surfaces this so
  // the user understands they're looking at demo output. Source of
  // truth is the X-Demo-Mode response header (read by requestAi in
  // lib/api.ts) — we never derive it from the request flag because
  // the server may serve a real response even when the user opted
  // in (and vice versa, demo mode kicks in when no key is set).
  const [aiDemoActive, setAiDemoActive] = useState(false)

  // Platform-adapt modal state. adaptCache keeps results keyed by
  // topic ID so re-opening the same topic skips the round trip;
  // adaptLoadingId tracks the in-flight request; adaptModalTopic is
  // the topic being shown in the modal (null when the modal is
  // closed). We snapshot the Topic object rather than just the id
  // so the modal renders correctly even if the topic list refreshes
  // mid-modal.
  const [adaptCache, setAdaptCache] = useState<
    Record<number, { data: PlatformAdapt; demo: boolean }>
  >({})
  const [adaptLoadingId, setAdaptLoadingId] = useState<number | null>(null)
  const [adaptModalTopic, setAdaptModalTopic] = useState<Topic | null>(null)
  const [adaptError, setAdaptError] = useState<string | null>(null)

  async function loadTopics() {
    setLoading(true)
    setError(null)
    try {
      const params: { platform?: string; status?: string } = {}
      if (filterPlatform) params.platform = filterPlatform
      if (filterStatus) params.status = filterStatus
      const res = await api.topics.list(params)
      setTopics(res.items)
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t('topics.error.load'))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadTopics()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filterPlatform, filterStatus])

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setSubmitting(true)
    setError(null)
    try {
      await api.topics.create({
        title: form.title,
        angle: form.angle,
        platform: form.platform,
        status: form.status,
      })
      setForm((prev) => ({ ...prev, title: '', angle: '' }))
      setShowForm(false)
      await loadTopics()
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t('topics.error.create'))
    } finally {
      setSubmitting(false)
    }
  }

  // When the user lands via /topics?demo=true (the "Try AI Demo"
  // CTA on the home page), open the AI panel so they immediately
  // see where the demo lives. We don't auto-fire the request —
  // the user still picks a seed — but the panel is visible and the
  // demo badge will show on first response.
  useEffect(() => {
    if (demoFromUrl) {
      setAiModalOpen(true)
      if (!aiSeed) setAiSeed(t('topics.ai_demo_seed'))
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [demoFromUrl])

  // handleGenerateTopics asks Claude for N differentiated topic
  // ideas and stores the result for rendering. aiModalOpen is left
  // open so the user can tweak the seed and try again.
  async function handleGenerateTopics() {
    if (!aiSeed.trim()) {
      setAiError(t('topics.ai_seed_required'))
      return
    }
    setAiLoading(true)
    setAiError(null)
    setAiResult(null)
    setAiDemoActive(false)
    try {
      const res = await api.ai.generateTopics(
        {
          seed: aiSeed.trim(),
          platform: filterPlatform || platforms[0],
          count: aiCount,
        },
        { demo: demoFromUrl },
      )
      setAiResult(res.data.topics)
      setAiDemoActive(res.demo)
    } catch (err: unknown) {
      setAiError(err instanceof Error ? err.message : t('topics.error.ai'))
    } finally {
      setAiLoading(false)
    }
  }

  // applyGeneratedTopic pre-fills the create form with a generated
  // topic and closes the AI panel. Keeps the user in flow: click an
  // idea -> tweak the angle -> save.
  function applyGeneratedTopic(tg: GeneratedTopic) {
    // The label prefixes here are reused from topics.ai_expected and
    // topics.ai_hook so the seed string stays in lock-step with the
    // labels we render in the result-card above (lines 569/575).
    // We trim the trailing colon so the assembled text reads cleanly
    // inside the angle textarea; the visible label still carries the
    // colon.
    const expectedLabel = t('topics.ai_expected').replace(/[:：]\s*$/, '')
    const hookLabel = t('topics.ai_hook').replace(/[:：]\s*$/, '')
    setForm((prev) => ({
      ...prev,
      title: tg.title,
      angle: `${tg.angle}\n\n${expectedLabel}: ${tg.expected_performance}\n${hookLabel}: ${tg.hook}`,
    }))
    setShowForm(true)
    setAiResult(null)
  }

  // moveTopic updates a topic's status with optimistic UI. On failure
  // we reload the list to reconcile. The `moving` Set keeps buttons
  // disabled per-card so the user can't double-fire a transition.
  async function moveTopic(id: number, nextStatus: string) {
    if (movingIds.has(id)) return
    setMovingIds((prev) => {
      const next = new Set(prev)
      next.add(id)
      return next
    })
    const previous = topics
    setTopics((prev) =>
      prev.map((tp) => (tp.id === id ? { ...tp, status: nextStatus } : tp)),
    )
    try {
      await api.topics.update(id, { status: nextStatus })
    } catch (err: unknown) {
      setError(
        err instanceof Error ? err.message : t('topics.error.update_status'),
      )
      setTopics(previous)
    } finally {
      setMovingIds((prev) => {
        const next = new Set(prev)
        next.delete(id)
        return next
      })
    }
  }

  // deleteTopic removes a topic via the API. We optimistically drop it
  // from the list and roll back on failure. The movingIds set is used
  // to disable the card buttons so a fast double-click can't double-fire
  // a delete + a status change.
  async function deleteTopic(id: number) {
    if (movingIds.has(id)) return
    setMovingIds((prev) => {
      const next = new Set(prev)
      next.add(id)
      return next
    })
    const previous = topics
    setTopics((prev) => prev.filter((tp) => tp.id !== id))
    try {
      await api.topics.delete(id)
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t('topics.error.delete'))
      setTopics(previous)
    } finally {
      setMovingIds((prev) => {
        const next = new Set(prev)
        next.delete(id)
        return next
      })
    }
  }

  // adaptTopic runs /ai/platform-adapt for a single topic and opens
  // the modal with the result. Cached results render immediately
  // without re-hitting the API — the user often wants to compare
  // the same topic's adaptation across platforms (the modal already
  // shows all three).
  async function adaptTopic(id: number) {
    const topic = topics.find((tp) => tp.id === id)
    if (!topic) return
    setAdaptError(null)
    setAdaptModalTopic(topic)
    if (adaptCache[id]) return // open with cached data, no extra request
    setAdaptLoadingId(id)
    try {
      const res = await api.ai.platformAdapt({
        title: topic.title,
        angle: topic.angle || '',
        source_platform: topic.platform,
      })
      setAdaptCache((prev) => ({
        ...prev,
        [id]: { data: res.data, demo: res.demo },
      }))
    } catch (err: unknown) {
      setAdaptError(
        err instanceof Error ? err.message : t('topics.adapt.error_failed'),
      )
    } finally {
      setAdaptLoadingId(null)
    }
  }

  // Group topics by kanban column. 撰写中 lives in its own bucket so the
  // user can still see and advance in-progress topics; the spec's 4 main
  // columns remain the focus.
  const grouped = KANBAN_COLUMNS.map((col) => ({
    ...col,
    topics: topics.filter((tp) => tp.status === col.status),
  }))
  const writingInProgress = topics.filter((tp) => tp.status === WRITING_STATUS)

  return (
    <div className="space-y-4">
      <div className="flex items-end justify-between flex-wrap gap-3">
        <div>
          <h1 className="font-serif text-3xl text-claude-ink tracking-tight">
            📋 {t('topics.title')}
          </h1>
          <p className="text-claude-muted text-sm mt-1">
            {t('topics.subhead')}
          </p>
        </div>
        <div className="flex gap-2 flex-wrap">
          <div
            role="tablist"
            aria-label={t('topics.kanban') + '/' + t('topics.list')}
            className="inline-flex rounded-md border border-claude-hairline overflow-hidden"
          >
            <button
              role="tab"
              aria-selected={view === 'kanban'}
              onClick={() => setView('kanban')}
              className={`px-2.5 md:px-3 py-1.5 md:py-2 text-xs md:text-sm transition-colors ${
                view === 'kanban'
                  ? 'bg-claude-ink text-claude-on-dark'
                  : 'bg-claude-canvas text-claude-body hover:bg-claude-surface-soft'
              }`}
            >
              {t('topics.kanban')}
            </button>
            <button
              role="tab"
              aria-selected={view === 'list'}
              onClick={() => setView('list')}
              className={`px-2.5 md:px-3 py-1.5 md:py-2 text-xs md:text-sm border-l border-claude-hairline transition-colors ${
                view === 'list'
                  ? 'bg-claude-ink text-claude-on-dark'
                  : 'bg-claude-canvas text-claude-body hover:bg-claude-surface-soft'
              }`}
            >
              {t('topics.list')}
            </button>
          </div>
          <button
            onClick={() => {
              setAiModalOpen((v) => !v)
              setAiError(null)
            }}
            className="px-3 md:px-4 py-1.5 md:py-2 text-xs md:text-sm bg-claude-accent-amber text-white rounded-md hover:opacity-90 disabled:opacity-50 transition-opacity"
            disabled={aiLoading}
          >
            {aiLoading
              ? t('topics.ai_button_loading')
              : t('topics.ai_button_idle')}
          </button>
          <button
            onClick={() => {
              setShowForm((v) => !v)
              setForm((prev) => ({
                ...prev,
                status: prev.status || STATUSES[0],
              }))
            }}
            className="px-3 md:px-4 py-1.5 md:py-2 text-xs md:text-sm bg-claude-coral text-claude-on-primary rounded-md hover:bg-claude-coral-active transition-colors"
          >
            {showForm ? t('common.cancel') : t('topics.create')}
          </button>
        </div>
      </div>

      <div className="flex gap-3 items-center flex-wrap">
        <label className="text-xs md:text-sm text-claude-body flex items-center gap-2">
          {t('topics.filter.platform')}:
          <select
            value={filterPlatform}
            onChange={(e) => setFilterPlatform(e.target.value)}
            className="px-2 py-1 text-xs md:text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink"
          >
            <option value="">{t('topics.filter.all')}</option>
            {platforms.map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
          </select>
        </label>
        <label className="text-xs md:text-sm text-claude-body flex items-center gap-2">
          {t('topics.filter.status')}:
          <select
            value={filterStatus}
            onChange={(e) => setFilterStatus(e.target.value)}
            className="px-2 py-1 text-xs md:text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink"
          >
            <option value="">{t('topics.filter.all')}</option>
            {STATUSES.map((s) => (
              <option key={s} value={s}>
                {s}
              </option>
            ))}
          </select>
        </label>
      </div>

      {aiModalOpen && (
        <div
          data-testid="ai-panel-topics"
          className="p-3 md:p-4 bg-claude-surface-card border border-claude-hairline rounded-lg space-y-3"
        >
          <h2 className="font-semibold text-claude-ink text-sm md:text-base">
            {t('topics.ai_panel_title')}
          </h2>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
            <div className="md:col-span-2">
              <label className="block text-xs md:text-sm font-medium text-claude-ink">
                {t('topics.ai_seed_label')}
              </label>
              <input
                type="text"
                data-testid="input-test-ai-seed"
                value={aiSeed}
                onChange={(e) => setAiSeed(e.target.value)}
                placeholder={t('topics.ai_seed_placeholder')}
                className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
              />
            </div>
            <div>
              <label className="block text-xs md:text-sm font-medium text-claude-ink">
                {t('topics.ai_count_label')}
              </label>
              <input
                type="number"
                data-testid="input-test-ai-count"
                min={1}
                max={20}
                value={aiCount}
                onChange={(e) => setAiCount(parseInt(e.target.value, 10) || 1)}
                className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
              />
            </div>
          </div>
          <div className="text-xs md:text-sm text-claude-body">
            {t('topics.ai_platform_hint')}:{' '}
            <span className="font-medium text-claude-ink">
              {filterPlatform || platforms[0] || ''}
            </span>
            {filterPlatform ? '' : t('topics.ai_platform_hint_default')}
          </div>
          <div className="flex gap-2 flex-wrap">
            <button
              type="button"
              data-testid="btn-ai-topics"
              onClick={handleGenerateTopics}
              disabled={aiLoading}
              className="px-3 md:px-4 py-1.5 md:py-2 text-xs md:text-sm bg-claude-accent-amber text-white rounded hover:opacity-90 disabled:opacity-50 transition-opacity"
            >
              {aiLoading
                ? t('topics.ai_generate_loading')
                : t('topics.ai_generate')}
            </button>
            <button
              onClick={() => {
                setAiModalOpen(false)
                setAiError(null)
                setAiResult(null)
              }}
              className="px-3 md:px-4 py-1.5 md:py-2 text-xs md:text-sm bg-claude-canvas text-claude-body rounded border border-claude-hairline hover:bg-claude-surface-soft"
            >
              {t('topics.ai_close')}
            </button>
          </div>
          {aiError && (
            <div className="p-3 bg-claude-error/10 border border-claude-error text-claude-error rounded text-xs md:text-sm">
              {aiError}
            </div>
          )}
          {aiDemoActive && !aiError && (
            <div
              data-testid="demo-badge-topics"
              className="inline-flex items-center gap-1.5 px-2 py-1 text-xs md:text-sm font-medium rounded bg-claude-accent-amber/15 text-claude-accent-amber border border-claude-accent-amber/30"
            >
              🎭{' '}
              {demoFromUrl
                ? t('topics.ai_demo_chosen')
                : t('topics.ai_demo_no_key')}
            </div>
          )}
          {aiResult && aiResult.length > 0 && (
            <div className="space-y-2">
              <div className="text-xs md:text-sm text-claude-body">
                {t('topics.ai_apply_hint')}
              </div>
              {aiResult.map((tg, i) => (
                <button
                  key={`${tg.title}-${i}`}
                  onClick={() => applyGeneratedTopic(tg)}
                  className="w-full text-left p-3 bg-claude-canvas border border-claude-hairline rounded hover:border-claude-coral hover:shadow-claude-soft transition-colors"
                >
                  <div className="font-semibold text-sm md:text-base text-claude-ink">
                    {tg.title}
                  </div>
                  <div className="text-xs md:text-sm text-claude-body mt-1">
                    {tg.angle}
                  </div>
                  <div className="text-xs text-claude-muted mt-2">
                    <span className="font-medium text-claude-ink">
                      {t('topics.ai_expected')}
                    </span>
                    {tg.expected_performance}
                  </div>
                  <div className="text-xs text-claude-muted">
                    <span className="font-medium text-claude-ink">
                      {t('topics.ai_hook')}
                    </span>
                    {tg.hook}
                  </div>
                </button>
              ))}
            </div>
          )}
        </div>
      )}

      {showForm && (
        <form
          onSubmit={handleSubmit}
          className="p-3 md:p-4 bg-claude-surface-card rounded-lg border border-claude-hairline space-y-3"
        >
          <div>
            <label className="block text-xs md:text-sm font-medium text-claude-ink">
              {t('topics.form.title_label')}
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
            <label className="block text-xs md:text-sm font-medium text-claude-ink">
              {t('topics.form.angle_label')}
            </label>
            <textarea
              required
              data-testid="input-test-angle"
              value={form.angle}
              onChange={(e) => setForm({ ...form, angle: e.target.value })}
              className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
              rows={3}
            />
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label className="block text-xs md:text-sm font-medium text-claude-ink">
                {t('topics.form.platform_label')}
              </label>
              <select
                data-testid="input-test-platform"
                value={form.platform}
                onChange={(e) => setForm({ ...form, platform: e.target.value })}
                className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink"
              >
                {platforms.map((p) => (
                  <option key={p} value={p}>
                    {p}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label className="block text-xs md:text-sm font-medium text-claude-ink">
                {t('topics.form.status_label')}
              </label>
              <select
                data-testid="input-test-status"
                value={form.status}
                onChange={(e) => setForm({ ...form, status: e.target.value })}
                className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink"
              >
                {STATUSES.map((s) => (
                  <option key={s} value={s}>
                    {s}
                  </option>
                ))}
              </select>
            </div>
          </div>
          <button
            type="submit"
            data-testid="btn-submit"
            disabled={submitting}
            className="w-full sm:w-auto px-4 py-2 text-sm bg-claude-coral text-claude-on-primary rounded-md hover:bg-claude-coral-active disabled:opacity-50 transition-colors"
          >
            {submitting
              ? t('topics.form.submit_loading')
              : t('topics.form.submit_idle')}
          </button>
        </form>
      )}

      {error && (
        <div className="p-3 bg-claude-error/10 border border-claude-error text-claude-error rounded text-sm">
          {error}
        </div>
      )}

      {loading ? (
        <div className="text-sm text-claude-body">{t('common.loading')}</div>
      ) : topics.length === 0 ? (
        <div className="p-4 bg-claude-surface-card rounded-lg border border-claude-hairline text-sm text-claude-muted text-center">
          {t('topics.empty')}
        </div>
      ) : view === 'kanban' ? (
        <div className="space-y-4">
          <div
            data-testid="kanban-board"
            className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-3"
          >
            {grouped.map((col) => (
              <div
                key={col.status}
                data-testid={`kanban-column-${col.status}`}
                className="bg-claude-surface-soft rounded-lg p-3 min-h-[200px] space-y-2 border border-claude-hairline-soft"
              >
                <div className="flex items-center justify-between">
                  <h2 className="font-semibold text-claude-ink">
                    {col.status}
                  </h2>
                  <span className="text-xs text-claude-muted">
                    {col.topics.length}
                  </span>
                </div>
                {col.topics.length === 0 ? (
                  <div className="text-xs text-claude-muted-soft italic py-2">
                    {t('topics.column.empty')}
                  </div>
                ) : (
                  col.topics.map((tp) => (
                    <KanbanCard
                      key={tp.id}
                      topic={tp}
                      onMove={moveTopic}
                      onStatusChange={moveTopic}
                      moving={movingIds.has(tp.id)}
                      onDelete={deleteTopic}
                      onAdapt={adaptTopic}
                      adapting={adaptLoadingId === tp.id}
                    />
                  ))
                )}
              </div>
            ))}
          </div>
          {writingInProgress.length > 0 && (
            <div
              data-testid="kanban-writing"
              className="bg-claude-accent-amber/10 border border-claude-accent-amber/30 rounded-lg p-3 space-y-2"
            >
              <div className="flex items-center justify-between">
                <h2 className="font-semibold text-claude-accent-amber">
                  {t('topics.column.writing')}
                </h2>
                <span className="text-xs text-claude-accent-amber">
                  {writingInProgress.length}
                </span>
              </div>
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-3">
                {writingInProgress.map((tp) => (
                  <KanbanCard
                    key={tp.id}
                    topic={tp}
                    onMove={moveTopic}
                    onStatusChange={moveTopic}
                    moving={movingIds.has(tp.id)}
                    onDelete={deleteTopic}
                    onAdapt={adaptTopic}
                    adapting={adaptLoadingId === tp.id}
                  />
                ))}
              </div>
            </div>
          )}
        </div>
      ) : (
        <div className="space-y-3">
          {topics.map((tp) => (
            <div
              key={tp.id}
              data-testid="list-item"
              data-topic-id={tp.id}
              className="p-3 md:p-4 bg-claude-surface-card rounded-lg border border-claude-hairline hover:border-claude-coral/50 transition-colors"
            >
              <div className="flex items-start justify-between gap-3 flex-col sm:flex-row">
                <div className="flex-1 min-w-0">
                  <h2 className="font-semibold text-sm md:text-base text-claude-ink">
                    {tp.title}
                  </h2>
                  <p className="text-xs md:text-sm text-claude-body mt-1">
                    {tp.angle}
                  </p>
                </div>
                <div className="flex gap-2 text-xs items-center flex-wrap">
                  <span className="px-2 py-1 bg-claude-canvas text-claude-coral rounded">
                    {tp.platform}
                  </span>
                  <span
                    className={`rounded-full px-2 py-0.5 font-medium ${statusPillClass(tp.status)}`}
                  >
                    {tp.status}
                  </span>
                  <select
                    aria-label={t('topics.aria.change_status')}
                    value={tp.status}
                    disabled={movingIds.has(tp.id)}
                    onChange={(e) => moveTopic(tp.id, e.target.value)}
                    className="px-2 py-1 text-xs border border-claude-hairline rounded bg-claude-canvas text-claude-ink"
                  >
                    {STATUSES.map((s) => (
                      <option key={s} value={s}>
                        {s}
                      </option>
                    ))}
                  </select>
                  <button
                    type="button"
                    data-testid={`btn-adapt-${tp.id}`}
                    onClick={() => adaptTopic(tp.id)}
                    disabled={adaptLoadingId === tp.id}
                    className="px-2 py-1 text-xs bg-claude-accent-teal/15 hover:bg-claude-accent-teal/25 text-claude-accent-teal rounded border border-claude-accent-teal/30 disabled:opacity-50"
                  >
                    {adaptLoadingId === tp.id
                      ? t('topics.adapt.loading')
                      : t('topics.adapt.button')}
                  </button>
                  <button
                    type="button"
                    data-testid={`btn-delete-${tp.id}`}
                    onClick={() => deleteTopic(tp.id)}
                    disabled={movingIds.has(tp.id)}
                    aria-label={t('topics.aria.delete')}
                    className="px-2 py-1 text-xs bg-claude-canvas hover:bg-claude-surface-soft text-claude-error rounded border border-claude-hairline disabled:opacity-50"
                  >
                    {t('common.delete')}
                  </button>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {adaptError && (
        <div className="p-3 bg-claude-error/10 border border-claude-error text-claude-error rounded text-sm">
          {adaptError}
        </div>
      )}

      {adaptModalTopic && (
        <PlatformAdaptModal
          topic={adaptModalTopic}
          cached={adaptCache[adaptModalTopic.id]}
          loading={adaptLoadingId === adaptModalTopic.id}
          onClose={() => setAdaptModalTopic(null)}
        />
      )}
    </div>
  )
}

interface PlatformAdaptModalProps {
  topic: Topic
  cached: { data: PlatformAdapt; demo: boolean } | undefined
  loading: boolean
  onClose: () => void
}

// PlatformAdaptModal renders the three platform-specific variants
// in a single scrollable card. We chose a single modal over a
// per-platform expandable list because the user needs to compare
// the variants side-by-side; collapsing them defeats the purpose.
function PlatformAdaptModal({
  topic,
  cached,
  loading,
  onClose,
}: PlatformAdaptModalProps) {
  const t = useT()
  const data = cached?.data
  return (
    <div
      data-testid="adapt-modal-backdrop"
      className="fixed inset-0 bg-claude-ink/40 flex items-center justify-center p-4 z-50"
      onClick={onClose}
    >
      <div
        data-testid={`adapt-modal-${topic.id}`}
        className="bg-claude-surface-card border border-claude-hairline rounded-lg shadow-claude-soft max-w-2xl w-full max-h-[90vh] overflow-y-auto"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
      >
        <div className="p-4 border-b border-claude-hairline-soft flex items-center justify-between gap-3 flex-wrap">
          <div>
            <h2 className="font-serif text-lg text-claude-ink">
              {t('topics.adapt.title')}
            </h2>
            <p className="text-xs text-claude-muted mt-0.5">
              {t('topics.adapt.subtitle')} · {topic.title}
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="text-xs text-claude-muted hover:text-claude-ink px-2 py-1 rounded hover:bg-claude-surface-soft"
          >
            {t('topics.adapt.close')}
          </button>
        </div>

        <div className="p-4 space-y-4">
          {loading && (
            <div className="text-sm text-claude-body">{t('common.loading')}</div>
          )}
          {cached?.demo && (
            <div className="inline-flex items-center gap-1.5 px-2 py-1 text-xs font-medium rounded bg-claude-accent-amber/15 text-claude-accent-amber border border-claude-accent-amber/30">
              {t('topics.adapt.demo_badge')}
            </div>
          )}
          {data && (
            <>
              <AdaptCard
                platform="抖音"
                title={data.adaptations['抖音'].title}
                description={data.adaptations['抖音'].description}
                hashtags={data.adaptations['抖音'].hashtags}
              />
              <AdaptCard
                platform="哔哩哔哩"
                title={data.adaptations['哔哩哔哩'].title}
                description={data.adaptations['哔哩哔哩'].description}
                tags={data.adaptations['哔哩哔哩'].tags}
              />
              <AdaptCard
                platform="小红书"
                title={data.adaptations['小红书'].title}
                body={data.adaptations['小红书'].body}
                tags={data.adaptations['小红书'].tags}
              />
              {data.cross_platform_tips.length > 0 && (
                <div className="p-3 bg-claude-canvas border border-claude-hairline-soft rounded">
                  <div className="text-xs font-medium text-claude-ink mb-2">
                    {t('topics.adapt.tips_label')}
                  </div>
                  <ul className="text-xs md:text-sm text-claude-body list-disc list-inside space-y-1">
                    {data.cross_platform_tips.map((tip, i) => (
                      <li key={`${tip}-${i}`}>{tip}</li>
                    ))}
                  </ul>
                </div>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  )
}

interface AdaptCardProps {
  platform: string
  title: string
  description?: string
  body?: string
  hashtags?: string[]
  tags?: string[]
}

// AdaptCard renders a single platform variant. We accept both
// `description` (抖音/哔哩哔哩) and `body` (小红书) since the
// wire shapes are slightly different per platform — exposing the
// platform-specific fields keeps the modal honest about what the
// API actually returns instead of normalising to a lowest-common
// denominator.
function AdaptCard({
  platform,
  title,
  description,
  body,
  hashtags,
  tags,
}: AdaptCardProps) {
  const t = useT()
  const [copied, setCopied] = useState(false)

  // composeCopyText assembles a clipboard-friendly version of the
  // adaptation. We keep it on a single newline-separated string
  // so it pastes cleanly into the platform's composer.
  function composeCopyText(): string {
    const parts: string[] = [`【${platform}】${title}`]
    if (description) parts.push(description)
    if (body) parts.push(body)
    if (hashtags && hashtags.length > 0) parts.push(hashtags.join(' '))
    if (tags && tags.length > 0) parts.push(tags.join(' '))
    return parts.join('\n\n')
  }

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(composeCopyText())
      setCopied(true)
      // 1.5s is the standard "I saw the feedback" window — long
      // enough to register, short enough that a second copy press
      // feels responsive.
      setTimeout(() => setCopied(false), 1500)
    } catch {
      // Clipboard API can reject in non-secure contexts (HTTP)
      // or when the page is hidden. Swallow silently — the user
      // will notice the missing affirmation and retry. We avoid
      // logging because console.log is disallowed in production
      // and an error toast would be overkill for an optional
      // utility action.
    }
  }

  return (
    <div className="p-3 bg-claude-canvas border border-claude-hairline rounded space-y-2">
      <div className="flex items-center justify-between gap-2 flex-wrap">
        <div className="text-xs font-semibold uppercase tracking-wide text-claude-coral">
          {platform}
        </div>
        <button
          type="button"
          data-testid={`btn-copy-${platform}`}
          onClick={handleCopy}
          className="text-xs px-2 py-0.5 bg-claude-surface-soft text-claude-body rounded border border-claude-hairline hover:bg-claude-surface-card"
        >
          {copied ? t('topics.adapt.copied') : t('topics.adapt.copy')}
        </button>
      </div>
      <div>
        <div className="text-[10px] uppercase tracking-wide text-claude-muted">
          {t('topics.adapt.title_label')}
        </div>
        <div className="text-sm font-medium text-claude-ink mt-0.5">
          {title}
        </div>
      </div>
      {description && (
        <div>
          <div className="text-[10px] uppercase tracking-wide text-claude-muted">
            {t('topics.adapt.description_label')}
          </div>
          <div className="text-xs md:text-sm text-claude-body whitespace-pre-wrap">
            {description}
          </div>
        </div>
      )}
      {body && (
        <div>
          <div className="text-[10px] uppercase tracking-wide text-claude-muted">
            {t('topics.adapt.body_label')}
          </div>
          <div className="text-xs md:text-sm text-claude-body whitespace-pre-wrap">
            {body}
          </div>
        </div>
      )}
      {hashtags && hashtags.length > 0 && (
        <div>
          <div className="text-[10px] uppercase tracking-wide text-claude-muted">
            {t('topics.adapt.hashtags_label')}
          </div>
          <div className="flex flex-wrap gap-1 mt-1">
            {hashtags.map((h, i) => (
              <span
                key={`${h}-${i}`}
                className="text-xs px-1.5 py-0.5 bg-claude-surface-soft text-claude-accent-teal rounded"
              >
                {h}
              </span>
            ))}
          </div>
        </div>
      )}
      {tags && tags.length > 0 && (
        <div>
          <div className="text-[10px] uppercase tracking-wide text-claude-muted">
            {t('topics.adapt.tags_label')}
          </div>
          <div className="flex flex-wrap gap-1 mt-1">
            {tags.map((tg, i) => (
              <span
                key={`${tg}-${i}`}
                className="text-xs px-1.5 py-0.5 bg-claude-surface-soft text-claude-accent-teal rounded"
              >
                {tg}
              </span>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
