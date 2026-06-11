'use client'

import { useEffect, useState, FormEvent } from 'react'
import { api } from '@opc/shared/api'
import type { ContentItem } from '@opc/shared/types'
import { useT } from '@opc/shared/i18n-client'
import { DeconstructPanel } from './DeconstructPanel'
import { CreateRecordForm, PLATFORMS, FormState, EMPTY_FORM } from './_components/CreateRecordForm'
import {
  PostmortemModal,
  PostmortemModalState,
  INITIAL_MODAL,
} from './_components/PostmortemModal'
import {
  TopicsModal,
  TopicsModalState,
  INITIAL_TOPICS_MODAL,
} from './_components/TopicsModal'

export default function DashboardPage() {
  const t = useT()
  const [items, setItems] = useState<ContentItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState<FormState>(EMPTY_FORM)
  const [submitting, setSubmitting] = useState(false)
  const [postmortem, setPostmortem] = useState<{ item: ContentItem; state: PostmortemModalState } | null>(null)
  const [showDeconstruct, setShowDeconstruct] = useState(false)
  const [topicsModal, setTopicsModal] = useState<TopicsModalState | null>(null)

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

  // handleGenerateTopics asks the AI service for N topic ideas and shows
  // them in the topics modal. The optional `seed` argument lets the
  // modal's input field override the default — when omitted (the
  // initial button click) the panda-IP default is used. The API
  // rejects an empty seed with 400, so we guard against it here.
  const DEFAULT_TOPIC_SEED = '个人成长博主'
  const [seedInput, setSeedInput] = useState(DEFAULT_TOPIC_SEED)

  async function handleGenerateTopics(seedOverride?: string) {
    const seed = (seedOverride ?? seedInput).trim()
    if (!seed) {
      setTopicsModal({
        ...INITIAL_TOPICS_MODAL,
        error: t('dashboard.generate_topics.error_failed'),
      })
      return
    }
    setTopicsModal({ ...INITIAL_TOPICS_MODAL, loading: true })
    try {
      const res = await api.ai.generateTopics({
        seed,
        platform: PLATFORMS[0],
        count: 5,
      })
      setTopicsModal({
        loading: false,
        topics: res.data.topics,
        error: null,
        demo: res.demo,
      })
    } catch (err: unknown) {
      setTopicsModal({
        loading: false,
        topics: [],
        error:
          err instanceof Error
            ? err.message
            : t('dashboard.generate_topics.error_failed'),
        demo: false,
      })
    }
  }

  function closeTopicsModal() {
    setTopicsModal(null)
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
        <button
          type="button"
          data-testid="btn-deconstruct-open"
          onClick={() => setShowDeconstruct(true)}
          className="px-3 md:px-4 py-1.5 md:py-2 text-xs md:text-sm bg-claude-accent-teal text-white rounded-md hover:opacity-90 transition-opacity"
        >
          {t('dashboard.deconstruct.button')}
        </button>
        <button
          type="button"
          data-testid="btn-generate-topics"
          onClick={() => handleGenerateTopics()}
          disabled={topicsModal?.loading}
          className="px-3 md:px-4 py-1.5 md:py-2 text-xs md:text-sm bg-claude-accent-coral text-white rounded-md hover:opacity-90 disabled:opacity-50 transition-opacity"
        >
          {topicsModal?.loading
            ? t('dashboard.postmortem_loading')
            : t('dashboard.generate_topics.button')}
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
        <CreateRecordForm
          form={form}
          submitting={submitting}
          onChange={setForm}
          onSubmit={handleSubmit}
        />
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

      {showDeconstruct && (
        <DeconstructPanel onClose={() => setShowDeconstruct(false)} />
      )}

      {topicsModal && (
        <TopicsModal
          state={topicsModal}
          seed={seedInput}
          onSeedChange={setSeedInput}
          onRegenerate={() => handleGenerateTopics(seedInput)}
          onClose={closeTopicsModal}
        />
      )}
    </div>
  )
}
