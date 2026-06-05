'use client'

import { useEffect, useState, FormEvent } from 'react'
import { api } from '@/lib/api'
import type { PostmortemStructured } from '@/lib/api'
import type { ContentItem } from '@/lib/types'
import { useT } from '@/lib/i18n-client'

// Canonical platform vocabulary mirroring the backend's allowedPlatforms
// set. Wire values; the surrounding chrome is rendered via i18n.
const PLATFORMS = ['抖音', '小红书', 'B站', '视频号', 'YouTube']

interface FormState {
  platform: string
  platform_url: string
  performance_metrics: string
}

const EMPTY_FORM: FormState = {
  platform: PLATFORMS[0],
  platform_url: '',
  performance_metrics: '',
}

// PostmortemModal — inline component for the AI postmortem report
// viewer. State machine: null (closed) | {loading} (in flight) |
// {result} (done) | {error} (failed). The modal stays mounted while
// the result is shown so the Close button can dismiss it cleanly.
interface PostmortemModalState {
  loading: boolean
  report: string
  structured: PostmortemStructured | null
  error: string | null
  demo: boolean
}

const INITIAL_MODAL: PostmortemModalState = {
  loading: false,
  report: '',
  structured: null,
  error: null,
  demo: false,
}

export default function DashboardPage() {
  const t = useT()
  const [items, setItems] = useState<ContentItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState<FormState>(EMPTY_FORM)
  const [submitting, setSubmitting] = useState(false)
  const [postmortem, setPostmortem] = useState<{ item: ContentItem; state: PostmortemModalState } | null>(null)

  async function loadItems() {
    setLoading(true)
    setError(null)
    try {
      const res = await api.contentItems.list()
      setItems(res.items)
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t('dashboard.error.load'))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadItems()
  }, [])

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setSubmitting(true)
    setError(null)
    try {
      await api.contentItems.create({
        platform: form.platform,
        platform_url: form.platform_url,
        performance_metrics: form.performance_metrics,
      })
      setForm(EMPTY_FORM)
      setShowForm(false)
      await loadItems()
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t('dashboard.error.create'))
    } finally {
      setSubmitting(false)
    }
  }

  // deleteItem removes a content item via the API. We optimistically
  // drop it from the list and roll back on failure.
  async function deleteItem(id: number) {
    const previous = items
    setItems((prev) => prev.filter((it) => it.id !== id))
    try {
      await api.contentItems.delete(id)
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t('dashboard.error.delete'))
      setItems(previous)
    }
  }

  async function handlePostmortem(item: ContentItem) {
    setPostmortem({ item, state: { ...INITIAL_MODAL, loading: true } })
    try {
      const res = await api.ai.postmortem(item.id)
      setPostmortem({
        item,
        state: {
          loading: false,
          report: res.data.report,
          structured: res.data.structured,
          error: null,
          demo: res.demo,
        },
      })
    } catch (err: unknown) {
      setPostmortem({
        item,
        state: {
          loading: false,
          report: '',
          structured: null,
          error: err instanceof Error ? err.message : t('dashboard.postmortem_failed'),
          demo: false,
        },
      })
    }
  }

  function closePostmortem() {
    setPostmortem(null)
  }

  const totalCount = items.length
  const withUrlCount = items.filter((i) => i.platform_url && i.platform_url.length > 0).length

  return (
    <div className="space-y-4">
      <div className="flex items-end justify-between flex-wrap gap-3">
        <div>
          <h1 className="font-serif text-3xl text-claude-ink tracking-tight">
            📊 {t('dashboard.title')}
          </h1>
          <p className="text-claude-muted text-sm mt-1">
            {t('dashboard.subhead')}
          </p>
        </div>
        <button
          onClick={() => setShowForm((v) => !v)}
          className="px-3 md:px-4 py-1.5 md:py-2 text-xs md:text-sm bg-claude-coral text-claude-on-primary rounded-md hover:bg-claude-coral-active transition-colors"
        >
          {showForm ? t('common.cancel') : t('dashboard.create')}
        </button>
      </div>

      <div className="grid grid-cols-2 gap-3 md:gap-4">
        <div className="p-4 md:p-6 bg-claude-surface-card rounded-lg border border-claude-hairline">
          <p className="text-xs md:text-sm text-claude-muted">
            {t('dashboard.metric.total')}
          </p>
          <p className="font-serif text-3xl mt-2 text-claude-ink">{totalCount}</p>
        </div>
        <div className="p-4 md:p-6 bg-claude-surface-card rounded-lg border border-claude-hairline">
          <p className="text-xs md:text-sm text-claude-muted">
            {t('dashboard.metric.with_url')}
          </p>
          <p className="font-serif text-3xl mt-2 text-claude-ink">{withUrlCount}</p>
        </div>
      </div>

      {showForm && (
        <form
          onSubmit={handleSubmit}
          className="p-3 md:p-4 bg-claude-surface-card rounded-lg border border-claude-hairline space-y-3"
        >
          <div>
            <label className="block text-xs md:text-sm font-medium text-claude-ink">
              {t('dashboard.form.platform_label')}
            </label>
            <select
              data-testid="input-test-platform"
              value={form.platform}
              onChange={(e) => setForm({ ...form, platform: e.target.value })}
              className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
            >
              {PLATFORMS.map((p) => (
                <option key={p} value={p}>
                  {p}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-xs md:text-sm font-medium text-claude-ink">
              {t('dashboard.form.url_label')}
            </label>
            <input
              type="url"
              data-testid="input-test-platform_url"
              value={form.platform_url}
              onChange={(e) =>
                setForm({ ...form, platform_url: e.target.value })
              }
              placeholder="https://..."
              className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
            />
          </div>
          <div>
            <label className="block text-xs md:text-sm font-medium text-claude-ink">
              {t('dashboard.form.metrics_label')}
            </label>
            <textarea
              data-testid="input-test-performance_metrics"
              value={form.performance_metrics}
              onChange={(e) =>
                setForm({ ...form, performance_metrics: e.target.value })
              }
              placeholder={t('dashboard.form.metrics_placeholder')}
              className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
              rows={3}
            />
          </div>
          <button
            type="submit"
            data-testid="btn-submit"
            disabled={submitting}
            className="w-full sm:w-auto px-4 py-2 text-sm bg-claude-coral text-claude-on-primary rounded-md hover:bg-claude-coral-active disabled:opacity-50 transition-colors"
          >
            {submitting
              ? t('dashboard.form.submit_loading')
              : t('dashboard.form.submit_idle')}
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
      ) : items.length === 0 ? (
        <div className="p-4 bg-claude-surface-card rounded-lg border border-claude-hairline text-sm text-claude-muted text-center">
          {t('dashboard.empty')}
        </div>
      ) : (
        <div className="space-y-3">
          {items.map((item) => (
            <div
              key={item.id}
              data-testid="list-item"
              data-content-item-id={item.id}
              className="p-3 md:p-4 bg-claude-surface-card rounded-lg border border-claude-hairline hover:border-claude-coral/50 transition-colors"
            >
              <div className="flex items-start justify-between gap-3 flex-col sm:flex-row">
                <div className="flex-1 min-w-0">
                  <p className="text-xs md:text-sm text-claude-body">
                    {t('dashboard.field.script')}
                    {item.script_id}
                  </p>
                  {item.published_at && (
                    <p className="text-xs text-claude-success mt-1 break-all">
                      {t('calendar.field.published_at')} {item.published_at}
                    </p>
                  )}
                  {item.platform_url ? (
                    <a
                      href={item.platform_url}
                      target="_blank"
                      rel="noreferrer"
                      className="text-xs text-claude-coral hover:text-claude-coral-active hover:underline block mt-1 break-all"
                    >
                      {item.platform_url}
                    </a>
                  ) : (
                    <p className="text-xs text-claude-muted-soft mt-1">
                      {t('dashboard.metric.no_url')}
                    </p>
                  )}
                  {item.performance_metrics && (
                    <p className="text-xs md:text-sm text-claude-ink mt-2 whitespace-pre-wrap">
                      {item.performance_metrics}
                    </p>
                  )}
                </div>
                <div className="flex sm:flex-col items-start sm:items-end gap-2">
                  <span className="px-2 py-1 bg-claude-canvas text-claude-coral rounded text-xs">
                    {item.platform}
                  </span>
                  <button
                    data-testid="btn-ai-postmortem"
                    onClick={() => handlePostmortem(item)}
                    disabled={postmortem?.item.id === item.id && postmortem.state.loading}
                    className="px-2.5 md:px-3 py-1 text-xs bg-claude-accent-amber text-white rounded hover:opacity-90 disabled:opacity-50 transition-opacity"
                  >
                    {postmortem?.item.id === item.id && postmortem.state.loading
                      ? t('dashboard.postmortem_loading')
                      : t('dashboard.postmortem')}
                  </button>
                  <button
                    type="button"
                    data-testid={`btn-delete-${item.id}`}
                    onClick={() => deleteItem(item.id)}
                    aria-label={t('dashboard.aria.delete')}
                    className="px-2 py-1 text-xs bg-claude-canvas hover:bg-claude-surface-soft text-claude-error rounded border border-claude-hairline"
                  >
                    {t('common.delete')}
                  </button>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {postmortem && (
        <PostmortemModal
          item={postmortem.item}
          state={postmortem.state}
          onClose={closePostmortem}
        />
      )}
    </div>
  )
}

// PostmortemModal — displays the AI postmortem report. When the
// structured parse succeeded we show the four buckets as a clear
// summary; the raw report text is always available below for
// transparency. When parsing failed or the call errored, we show
// whatever the API returned.
interface PostmortemModalProps {
  item: ContentItem
  state: PostmortemModalState
  onClose: () => void
}

function PostmortemModal({ item, state, onClose }: PostmortemModalProps) {
  const t = useT()
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
            {t('dashboard.postmortem')} #{item.id}
            <span className="ml-2 text-xs text-claude-muted">({item.platform})</span>
          </h2>
          <button
            onClick={onClose}
            aria-label={t('dashboard.aria.close')}
            className="text-claude-muted-soft hover:text-claude-ink text-2xl leading-none"
          >
            ×
          </button>
        </div>

        <div className="p-3 sm:p-4 space-y-4">
          {state.demo && !state.loading && !state.error && (
            <div
              data-testid="demo-badge-postmortem"
              className="inline-flex items-center gap-1.5 px-2 py-1 text-xs font-medium rounded bg-claude-accent-amber/15 text-claude-accent-amber border border-claude-accent-amber/30"
            >
              {t('dashboard.postmortem_demo')}
            </div>
          )}
          {state.loading && (
            <div className="text-sm text-claude-body">
              {t('dashboard.postmortem_loading_view')}
            </div>
          )}

          {state.error && (
            <div className="p-3 bg-claude-error/10 border border-claude-error text-claude-error rounded text-sm">
              {state.error}
            </div>
          )}

          {!state.loading && !state.error && state.structured && (
            <PostmortemStructuredView structured={state.structured} />
          )}

          {!state.loading && !state.error && state.report && (
            <details className="text-sm">
              <summary className="cursor-pointer text-claude-body hover:text-claude-ink">
                {t('dashboard.postmortem_raw')}
              </summary>
              <pre className="mt-2 p-3 bg-claude-surface-card rounded whitespace-pre-wrap break-words text-xs text-claude-ink">
                {state.report}
              </pre>
            </details>
          )}
        </div>

        <div className="p-3 sm:p-4 border-t border-claude-hairline flex justify-end">
          <button
            onClick={onClose}
            className="w-full sm:w-auto px-4 py-2 text-sm bg-claude-surface-card text-claude-ink rounded hover:bg-claude-surface-soft border border-claude-hairline"
          >
            {t('dashboard.aria.close')}
          </button>
        </div>
      </div>
    </div>
  )
}

// PostmortemStructuredView renders the four parsed buckets. Each
// section is hidden when empty so a partial parse (e.g. Claude
// returned only success_factors) still renders cleanly.
interface PostmortemStructuredViewProps {
  structured: PostmortemStructured
}

function PostmortemStructuredView({ structured }: PostmortemStructuredViewProps) {
  const t = useT()
  return (
    <div className="space-y-3">
      <BucketSection
        title={t('dashboard.bucket.success')}
        items={structured.success_factors}
        accent="bg-claude-success/10 text-claude-success"
      />
      <BucketSection
        title={t('dashboard.bucket.patterns')}
        items={structured.reusable_patterns}
        accent="bg-claude-accent-teal/10 text-claude-accent-teal"
      />
      <BucketSection
        title={t('dashboard.bucket.insights')}
        items={structured.insights}
        accent="bg-claude-accent-amber/10 text-claude-accent-amber"
      />
      <BucketSection
        title={t('dashboard.bucket.suggestions')}
        items={structured.suggestions}
        accent="bg-claude-coral/10 text-claude-coral"
      />
    </div>
  )
}

interface BucketSectionProps {
  title: string
  items: string[]
  accent: string
}

function BucketSection({ title, items, accent }: BucketSectionProps) {
  if (!items || items.length === 0) return null
  return (
    <div>
      <h3 className={`inline-block px-2 py-0.5 rounded text-xs font-semibold ${accent}`}>
        {title}
      </h3>
      <ul className="mt-2 space-y-1 text-sm text-claude-ink list-disc list-inside">
        {items.map((it, i) => (
          <li key={i}>{it}</li>
        ))}
      </ul>
    </div>
  )
}
