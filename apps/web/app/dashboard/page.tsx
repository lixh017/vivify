'use client'

import { useEffect, useState, FormEvent } from 'react'
import { api } from '@/lib/api'
import type { PostmortemStructured } from '@/lib/api'
import type { ContentItem } from '@/lib/types'

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

// PostmortemModal — inline component for the AI 复盘 report viewer.
// State machine: null (closed) | {loading} (in flight) | {result} (done)
// | {error} (failed). The modal stays mounted while the result is
// shown so the Close button can dismiss it cleanly.
interface PostmortemModalState {
  loading: boolean
  report: string
  structured: PostmortemStructured | null
  error: string | null
}

const INITIAL_MODAL: PostmortemModalState = {
  loading: false,
  report: '',
  structured: null,
  error: null,
}

export default function DashboardPage() {
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
      setError(err instanceof Error ? err.message : '加载失败')
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
      setError(err instanceof Error ? err.message : '创建失败')
    } finally {
      setSubmitting(false)
    }
  }

  async function handlePostmortem(item: ContentItem) {
    setPostmortem({ item, state: { ...INITIAL_MODAL, loading: true } })
    try {
      const res = await api.ai.postmortem(item.id)
      setPostmortem({
        item,
        state: { loading: false, report: res.report, structured: res.structured, error: null },
      })
    } catch (err: unknown) {
      setPostmortem({
        item,
        state: {
          loading: false,
          report: '',
          structured: null,
          error: err instanceof Error ? err.message : 'AI 复盘失败',
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
      <div className="flex items-center justify-between flex-wrap gap-2">
        <h1 className="text-2xl md:text-3xl font-bold">📊 表现</h1>
        <button
          onClick={() => setShowForm((v) => !v)}
          className="px-3 md:px-4 py-1.5 md:py-2 text-xs md:text-sm bg-blue-600 text-white rounded-lg shadow hover:bg-blue-700"
        >
          {showForm ? '取消' : '+ 新建记录'}
        </button>
      </div>

      <div className="grid grid-cols-2 gap-3 md:gap-4">
        <div className="p-3 md:p-4 bg-white rounded-lg shadow">
          <p className="text-xs md:text-sm text-gray-500">总记录数</p>
          <p className="text-xl md:text-2xl font-bold mt-1">{totalCount}</p>
        </div>
        <div className="p-3 md:p-4 bg-white rounded-lg shadow">
          <p className="text-xs md:text-sm text-gray-500">含平台链接</p>
          <p className="text-xl md:text-2xl font-bold mt-1">{withUrlCount}</p>
        </div>
      </div>

      {showForm && (
        <form
          onSubmit={handleSubmit}
          className="p-3 md:p-4 bg-white rounded-lg shadow space-y-3"
        >
          <div>
            <label className="block text-xs md:text-sm font-medium text-gray-700">
              平台
            </label>
            <select
              value={form.platform}
              onChange={(e) => setForm({ ...form, platform: e.target.value })}
              className="mt-1 w-full px-3 py-2 text-sm border border-gray-300 rounded"
            >
              {PLATFORMS.map((p) => (
                <option key={p} value={p}>
                  {p}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-xs md:text-sm font-medium text-gray-700">
              平台链接
            </label>
            <input
              type="url"
              value={form.platform_url}
              onChange={(e) =>
                setForm({ ...form, platform_url: e.target.value })
              }
              placeholder="https://..."
              className="mt-1 w-full px-3 py-2 text-sm border border-gray-300 rounded"
            />
          </div>
          <div>
            <label className="block text-xs md:text-sm font-medium text-gray-700">
              表现数据
            </label>
            <textarea
              value={form.performance_metrics}
              onChange={(e) =>
                setForm({ ...form, performance_metrics: e.target.value })
              }
              placeholder="例如: 播放 12k, 点赞 800, 评论 45"
              className="mt-1 w-full px-3 py-2 text-sm border border-gray-300 rounded"
              rows={3}
            />
          </div>
          <button
            type="submit"
            disabled={submitting}
            className="w-full sm:w-auto px-4 py-2 text-sm bg-blue-600 text-white rounded-lg shadow hover:bg-blue-700 disabled:opacity-50"
          >
            {submitting ? '提交中...' : '保存'}
          </button>
        </form>
      )}

      {error && (
        <div className="p-3 bg-red-50 border border-red-200 text-red-700 rounded text-sm">
          {error}
        </div>
      )}

      {loading ? (
        <div className="text-sm text-gray-600">加载中…</div>
      ) : items.length === 0 ? (
        <div className="p-4 bg-white rounded-lg shadow text-sm text-gray-500 text-center">
          还没有数据
        </div>
      ) : (
        <div className="space-y-3">
          {items.map((item) => (
            <div
              key={item.id}
              className="p-3 md:p-4 bg-white rounded-lg shadow hover:shadow-md"
            >
              <div className="flex items-start justify-between gap-3 flex-col sm:flex-row">
                <div className="flex-1 min-w-0">
                  <p className="text-xs md:text-sm text-gray-600">
                    Script #{item.script_id}
                  </p>
                  {item.published_at && (
                    <p className="text-xs text-green-600 mt-1 break-all">
                      已发布: {item.published_at}
                    </p>
                  )}
                  {item.platform_url ? (
                    <a
                      href={item.platform_url}
                      target="_blank"
                      rel="noreferrer"
                      className="text-xs text-blue-600 hover:underline block mt-1 break-all"
                    >
                      {item.platform_url}
                    </a>
                  ) : (
                    <p className="text-xs text-gray-400 mt-1">无链接</p>
                  )}
                  {item.performance_metrics && (
                    <p className="text-xs md:text-sm text-gray-700 mt-2 whitespace-pre-wrap">
                      {item.performance_metrics}
                    </p>
                  )}
                </div>
                <div className="flex sm:flex-col items-start sm:items-end gap-2">
                  <span className="px-2 py-1 bg-blue-50 text-blue-700 rounded text-xs">
                    {item.platform}
                  </span>
                  <button
                    onClick={() => handlePostmortem(item)}
                    disabled={postmortem?.item.id === item.id && postmortem.state.loading}
                    className="px-2.5 md:px-3 py-1 text-xs bg-purple-600 text-white rounded shadow hover:bg-purple-700 disabled:opacity-50"
                  >
                    {postmortem?.item.id === item.id && postmortem.state.loading
                      ? '复盘中...'
                      : 'AI 复盘'}
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
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-2 sm:p-4"
      onClick={onClose}
    >
      <div
        className="bg-white rounded-lg shadow-xl max-w-2xl w-full max-h-[90vh] sm:max-h-[85vh] overflow-y-auto"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="p-3 sm:p-4 border-b flex items-center justify-between gap-2">
          <h2 className="text-base sm:text-lg font-semibold">
            AI 复盘 #{item.id}
            <span className="ml-2 text-xs text-gray-500">({item.platform})</span>
          </h2>
          <button
            onClick={onClose}
            aria-label="关闭"
            className="text-gray-400 hover:text-gray-600 text-2xl leading-none"
          >
            ×
          </button>
        </div>

        <div className="p-3 sm:p-4 space-y-4">
          {state.loading && (
            <div className="text-sm text-gray-600">复盘中，请稍候...</div>
          )}

          {state.error && (
            <div className="p-3 bg-red-50 border border-red-200 text-red-700 rounded text-sm">
              {state.error}
            </div>
          )}

          {!state.loading && !state.error && state.structured && (
            <PostmortemStructuredView structured={state.structured} />
          )}

          {!state.loading && !state.error && state.report && (
            <details className="text-sm">
              <summary className="cursor-pointer text-gray-600 hover:text-gray-800">
                查看原始报告
              </summary>
              <pre className="mt-2 p-3 bg-gray-50 rounded whitespace-pre-wrap break-words text-xs">
                {state.report}
              </pre>
            </details>
          )}
        </div>

        <div className="p-3 sm:p-4 border-t flex justify-end">
          <button
            onClick={onClose}
            className="w-full sm:w-auto px-4 py-2 text-sm bg-gray-200 text-gray-800 rounded hover:bg-gray-300"
          >
            关闭
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
  return (
    <div className="space-y-3">
      <BucketSection title="成功要素" items={structured.success_factors} accent="bg-emerald-50 text-emerald-800" />
      <BucketSection title="可复用模式" items={structured.reusable_patterns} accent="bg-blue-50 text-blue-800" />
      <BucketSection title="数据洞察" items={structured.insights} accent="bg-amber-50 text-amber-800" />
      <BucketSection title="改进建议" items={structured.suggestions} accent="bg-purple-50 text-purple-800" />
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
      <ul className="mt-2 space-y-1 text-sm text-gray-800 list-disc list-inside">
        {items.map((it, i) => (
          <li key={i}>{it}</li>
        ))}
      </ul>
    </div>
  )
}
