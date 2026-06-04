'use client'

import { useEffect, useState, FormEvent } from 'react'
import { api, type GeneratedTopic } from '@/lib/api'
import type { Topic } from '@/lib/types'

const PLATFORMS = ['抖音', '小红书', 'B站', '视频号', 'YouTube']

// Canonical status vocabulary, mirroring the backend's allowedStatuses set
// (apps/api/internal/models/topic.go). The previous English placeholders
// ("idea", "writing", ...) would fail backend validation, so we use the
// Chinese values end-to-end.
const STATUSES = ['想法', '评估', '待写', '撰写中', '已发布'] as const

// The 4-column kanban view as specified in section 4.9.1 of the IP design.
// Topics with status "撰写中" are bucketed under 待写 so the columns line up
// with the spec's 想法 / 评估 / 待写 / 已发布 layout.
const KANBAN_COLUMNS: { label: string; status: string; next: string | null }[] = [
  { label: '想法', status: '想法', next: '评估' },
  { label: '评估', status: '评估', next: '待写' },
  { label: '待写', status: '待写', next: '撰写中' },
  { label: '已发布', status: '已发布', next: null },
]

// "撰写中" doesn't have a dedicated column, so we surface it as a thin row
// between 待写 and 已发布. It advances to 已发布 when the user moves it.
const WRITING_STATUS = '撰写中'

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
}

function KanbanCard({ topic, onMove, onStatusChange, moving, onDelete }: KanbanCardProps) {
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
        <select
          aria-label="更改状态"
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
      <div className="flex gap-1">
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
        {onDelete && (
          <button
            type="button"
            data-testid={`btn-delete-${topic.id}`}
            onClick={() => onDelete(topic.id)}
            disabled={moving}
            aria-label="删除"
            className="text-xs px-2 py-1 bg-claude-canvas hover:bg-claude-surface-soft text-claude-error rounded border border-claude-hairline disabled:opacity-50"
          >
            删除
          </button>
        )}
      </div>
    </div>
  )
}

export default function TopicsPage() {
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
      setError(err instanceof Error ? err.message : '加载失败')
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
      setForm(EMPTY_FORM)
      setShowForm(false)
      await loadTopics()
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : '创建失败')
    } finally {
      setSubmitting(false)
    }
  }

  // handleGenerateTopics asks Claude for N differentiated topic
  // ideas and stores the result for rendering. aiModalOpen is left
  // open so the user can tweak the seed and try again.
  async function handleGenerateTopics() {
    if (!aiSeed.trim()) {
      setAiError('请输入种子概念')
      return
    }
    setAiLoading(true)
    setAiError(null)
    setAiResult(null)
    try {
      const res = await api.ai.generateTopics({
        seed: aiSeed.trim(),
        platform: filterPlatform || PLATFORMS[0],
        count: aiCount,
      })
      setAiResult(res.topics)
    } catch (err: unknown) {
      setAiError(err instanceof Error ? err.message : 'AI 生成失败')
    } finally {
      setAiLoading(false)
    }
  }

  // applyGeneratedTopic pre-fills the create form with a generated
  // topic and closes the AI panel. Keeps the user in flow: click an
  // idea -> tweak the angle -> save.
  function applyGeneratedTopic(t: GeneratedTopic) {
    setForm({
      title: t.title,
      angle: `${t.angle}\n\n预期表现：${t.expected_performance}\n钩子：${t.hook}`,
      platform: filterPlatform || PLATFORMS[0],
      status: STATUSES[0],
    })
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
      prev.map((t) => (t.id === id ? { ...t, status: nextStatus } : t)),
    )
    try {
      await api.topics.update(id, { status: nextStatus })
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : '状态更新失败')
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
    setTopics((prev) => prev.filter((t) => t.id !== id))
    try {
      await api.topics.delete(id)
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : '删除失败')
      setTopics(previous)
    } finally {
      setMovingIds((prev) => {
        const next = new Set(prev)
        next.delete(id)
        return next
      })
    }
  }

  // Group topics by kanban column. 撰写中 lives in its own bucket so the
  // user can still see and advance in-progress topics; the spec's 4 main
  // columns remain the focus.
  const grouped = KANBAN_COLUMNS.map((col) => ({
    ...col,
    topics: topics.filter((t) => t.status === col.status),
  }))
  const writingInProgress = topics.filter((t) => t.status === WRITING_STATUS)

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between flex-wrap gap-2">
        <h1 className="text-2xl md:text-3xl font-bold text-claude-ink">📋 选题</h1>
        <div className="flex gap-2 flex-wrap">
          <div
            role="tablist"
            aria-label="视图切换"
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
              看板
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
              列表
            </button>
          </div>
          <button
            onClick={() => {
              setAiModalOpen((v) => !v)
              setAiError(null)
            }}
            className="px-3 md:px-4 py-1.5 md:py-2 text-xs md:text-sm bg-claude-ink text-claude-on-dark rounded-md hover:bg-claude-surface-dark-elevated disabled:opacity-50 transition-colors"
            disabled={aiLoading}
          >
            {aiLoading ? '生成中...' : 'AI 生成'}
          </button>
          <button
            onClick={() => setShowForm((v) => !v)}
            className="px-3 md:px-4 py-1.5 md:py-2 text-xs md:text-sm bg-claude-coral text-claude-on-primary rounded-md hover:bg-claude-coral-active transition-colors"
          >
            {showForm ? '取消' : '+ 新建'}
          </button>
        </div>
      </div>

      <div className="flex gap-3 items-center flex-wrap">
        <label className="text-xs md:text-sm text-claude-body flex items-center gap-2">
          平台:
          <select
            value={filterPlatform}
            onChange={(e) => setFilterPlatform(e.target.value)}
            className="px-2 py-1 text-xs md:text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink"
          >
            <option value="">全部</option>
            {PLATFORMS.map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
          </select>
        </label>
        <label className="text-xs md:text-sm text-claude-body flex items-center gap-2">
          状态:
          <select
            value={filterStatus}
            onChange={(e) => setFilterStatus(e.target.value)}
            className="px-2 py-1 text-xs md:text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink"
          >
            <option value="">全部</option>
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
            AI 选题生成
          </h2>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
            <div className="md:col-span-2">
              <label className="block text-xs md:text-sm font-medium text-claude-ink">
                种子概念
              </label>
              <input
                type="text"
                data-testid="input-test-ai-seed"
                value={aiSeed}
                onChange={(e) => setAiSeed(e.target.value)}
                placeholder="例如：禅意解压、深夜emo、宅文化..."
                className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:outline-none focus:ring-2 focus:ring-claude-coral"
              />
            </div>
            <div>
              <label className="block text-xs md:text-sm font-medium text-claude-ink">
                数量
              </label>
              <input
                type="number"
                data-testid="input-test-ai-count"
                min={1}
                max={20}
                value={aiCount}
                onChange={(e) => setAiCount(parseInt(e.target.value, 10) || 1)}
                className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:outline-none focus:ring-2 focus:ring-claude-coral"
              />
            </div>
          </div>
          <div className="text-xs md:text-sm text-claude-body">
            平台：<span className="font-medium text-claude-ink">{filterPlatform || PLATFORMS[0]}</span>
            {filterPlatform ? '' : '（默认；可用上方筛选器切换）'}
          </div>
          <div className="flex gap-2 flex-wrap">
            <button
              type="button"
              data-testid="btn-ai-topics"
              onClick={handleGenerateTopics}
              disabled={aiLoading}
              className="px-3 md:px-4 py-1.5 md:py-2 text-xs md:text-sm bg-claude-ink text-claude-on-dark rounded hover:bg-claude-surface-dark-elevated disabled:opacity-50 transition-colors"
            >
              {aiLoading ? '生成中...' : '生成'}
            </button>
            <button
              onClick={() => {
                setAiModalOpen(false)
                setAiError(null)
                setAiResult(null)
              }}
              className="px-3 md:px-4 py-1.5 md:py-2 text-xs md:text-sm bg-claude-canvas text-claude-body rounded border border-claude-hairline hover:bg-claude-surface-soft"
            >
              关闭
            </button>
          </div>
          {aiError && (
            <div className="p-3 bg-claude-surface-card border border-claude-error text-claude-error rounded text-xs md:text-sm">
              {aiError}
            </div>
          )}
          {aiResult && aiResult.length > 0 && (
            <div className="space-y-2">
              <div className="text-xs md:text-sm text-claude-body">
                点击任意选题可填入下方创建表单：
              </div>
              {aiResult.map((t, i) => (
                <button
                  key={`${t.title}-${i}`}
                  onClick={() => applyGeneratedTopic(t)}
                  className="w-full text-left p-3 bg-claude-canvas border border-claude-hairline rounded hover:border-claude-coral hover:shadow-claude-soft transition-colors"
                >
                  <div className="font-semibold text-sm md:text-base text-claude-ink">
                    {t.title}
                  </div>
                  <div className="text-xs md:text-sm text-claude-body mt-1">
                    {t.angle}
                  </div>
                  <div className="text-xs text-claude-muted mt-2">
                    <span className="font-medium text-claude-ink">预期：</span>
                    {t.expected_performance}
                  </div>
                  <div className="text-xs text-claude-muted">
                    <span className="font-medium text-claude-ink">钩子：</span>
                    {t.hook}
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
              标题
            </label>
            <input
              type="text"
              required
              data-testid="input-test-title"
              value={form.title}
              onChange={(e) => setForm({ ...form, title: e.target.value })}
              className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:outline-none focus:ring-2 focus:ring-claude-coral"
            />
          </div>
          <div>
            <label className="block text-xs md:text-sm font-medium text-claude-ink">
              角度
            </label>
            <textarea
              required
              data-testid="input-test-angle"
              value={form.angle}
              onChange={(e) => setForm({ ...form, angle: e.target.value })}
              className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:outline-none focus:ring-2 focus:ring-claude-coral"
              rows={3}
            />
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label className="block text-xs md:text-sm font-medium text-claude-ink">
                平台
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
            <div>
              <label className="block text-xs md:text-sm font-medium text-claude-ink">
                状态
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
            {submitting ? '提交中...' : '保存'}
          </button>
        </form>
      )}

      {error && (
        <div className="p-3 bg-claude-surface-card border border-claude-error text-claude-error rounded text-sm">
          {error}
        </div>
      )}

      {loading ? (
        <div className="text-sm text-claude-body">加载中…</div>
      ) : topics.length === 0 ? (
        <div className="p-4 bg-claude-surface-card rounded-lg border border-claude-hairline text-sm text-claude-muted text-center">
          还没有数据
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
                data-testid={`kanban-column-${col.label}`}
                className="bg-claude-surface-soft rounded-lg p-3 min-h-[200px] space-y-2 border border-claude-hairline-soft"
              >
                <div className="flex items-center justify-between">
                  <h2 className="font-semibold text-claude-ink">{col.label}</h2>
                  <span className="text-xs text-claude-muted">
                    {col.topics.length}
                  </span>
                </div>
                {col.topics.length === 0 ? (
                  <div className="text-xs text-claude-muted-soft italic py-2">
                    暂无
                  </div>
                ) : (
                  col.topics.map((t) => (
                    <KanbanCard
                      key={t.id}
                      topic={t}
                      onMove={moveTopic}
                      onStatusChange={moveTopic}
                      moving={movingIds.has(t.id)}
                      onDelete={deleteTopic}
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
                <h2 className="font-semibold text-claude-accent-amber">撰写中</h2>
                <span className="text-xs text-claude-accent-amber">
                  {writingInProgress.length}
                </span>
              </div>
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-3">
                {writingInProgress.map((t) => (
                  <KanbanCard
                    key={t.id}
                    topic={t}
                    onMove={moveTopic}
                    onStatusChange={moveTopic}
                    moving={movingIds.has(t.id)}
                    onDelete={deleteTopic}
                  />
                ))}
              </div>
            </div>
          )}
        </div>
      ) : (
        <div className="space-y-3">
          {topics.map((t) => (
            <div
              key={t.id}
              data-testid="list-item"
              data-topic-id={t.id}
              className="p-3 md:p-4 bg-claude-surface-card rounded-lg border border-claude-hairline hover:border-claude-coral/50 transition-colors"
            >
              <div className="flex items-start justify-between gap-3 flex-col sm:flex-row">
                <div className="flex-1 min-w-0">
                  <h2 className="font-semibold text-sm md:text-base text-claude-ink">
                    {t.title}
                  </h2>
                  <p className="text-xs md:text-sm text-claude-body mt-1">
                    {t.angle}
                  </p>
                </div>
                <div className="flex gap-2 text-xs items-center flex-wrap">
                  <span className="px-2 py-1 bg-claude-canvas text-claude-coral rounded">
                    {t.platform}
                  </span>
                  <select
                    aria-label="更改状态"
                    value={t.status}
                    disabled={movingIds.has(t.id)}
                    onChange={(e) => moveTopic(t.id, e.target.value)}
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
                    data-testid={`btn-delete-${t.id}`}
                    onClick={() => deleteTopic(t.id)}
                    disabled={movingIds.has(t.id)}
                    aria-label="删除"
                    className="px-2 py-1 text-xs bg-claude-canvas hover:bg-claude-surface-soft text-claude-error rounded border border-claude-hairline disabled:opacity-50"
                  >
                    删除
                  </button>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
