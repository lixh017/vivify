'use client'

import { useEffect, useState, FormEvent } from 'react'
import { api } from '@/lib/api'
import type { ContentItem } from '@/lib/types'

const PLATFORMS = ['抖音', '小红书', 'B站', '视频号', 'YouTube']

const PENDING_KEY = 'pending'

interface FormState {
  platform: string
  scheduled_at: string
  is_pending: boolean
}

const EMPTY_FORM: FormState = {
  platform: PLATFORMS[0],
  scheduled_at: '',
  is_pending: false,
}

function groupKey(item: ContentItem): string {
  if (!item.scheduled_at) return PENDING_KEY
  // Take YYYY-MM-DD prefix from ISO timestamp.
  return item.scheduled_at.slice(0, 10)
}

function formatGroupLabel(key: string): string {
  if (key === PENDING_KEY) return '(待定)'
  return key
}

export default function CalendarPage() {
  const [items, setItems] = useState<ContentItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState<FormState>(EMPTY_FORM)
  const [submitting, setSubmitting] = useState(false)

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
      const payload: { platform: string; scheduled_at?: string } = {
        platform: form.platform,
      }
      if (!form.is_pending && form.scheduled_at) {
        payload.scheduled_at = new Date(form.scheduled_at).toISOString()
      }
      await api.contentItems.create(payload)
      setForm(EMPTY_FORM)
      setShowForm(false)
      await loadItems()
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : '创建失败')
    } finally {
      setSubmitting(false)
    }
  }

  const groups = items.reduce<Record<string, ContentItem[]>>((acc, item) => {
    const key = groupKey(item)
    if (!acc[key]) acc[key] = []
    acc[key].push(item)
    return acc
  }, {})

  const sortedKeys = Object.keys(groups).sort((a, b) => {
    if (a === PENDING_KEY) return 1
    if (b === PENDING_KEY) return -1
    return a.localeCompare(b)
  })

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-3xl font-bold">📅 日历</h1>
        <button
          onClick={() => setShowForm((v) => !v)}
          className="px-4 py-2 bg-blue-600 text-white rounded-lg shadow hover:bg-blue-700"
        >
          {showForm ? '取消' : '+ 新建排期'}
        </button>
      </div>

      {showForm && (
        <form
          onSubmit={handleSubmit}
          className="p-4 bg-white rounded-lg shadow space-y-3"
        >
          <div>
            <label className="block text-sm font-medium text-gray-700">
              平台
            </label>
            <select
              value={form.platform}
              onChange={(e) => setForm({ ...form, platform: e.target.value })}
              className="mt-1 w-full px-3 py-2 border border-gray-300 rounded"
            >
              {PLATFORMS.map((p) => (
                <option key={p} value={p}>
                  {p}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="flex items-center gap-2 text-sm text-gray-700">
              <input
                type="checkbox"
                checked={form.is_pending}
                onChange={(e) =>
                  setForm({ ...form, is_pending: e.target.checked })
                }
              />
              时间待定
            </label>
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700">
              排期时间
            </label>
            <input
              type="datetime-local"
              disabled={form.is_pending}
              value={form.scheduled_at}
              onChange={(e) =>
                setForm({ ...form, scheduled_at: e.target.value })
              }
              className="mt-1 w-full px-3 py-2 border border-gray-300 rounded disabled:bg-gray-100"
            />
          </div>
          <button
            type="submit"
            disabled={submitting || (!form.is_pending && !form.scheduled_at)}
            className="px-4 py-2 bg-blue-600 text-white rounded-lg shadow hover:bg-blue-700 disabled:opacity-50"
          >
            {submitting ? '提交中...' : '保存'}
          </button>
        </form>
      )}

      {error && (
        <div className="p-3 bg-red-50 border border-red-200 text-red-700 rounded">
          {error}
        </div>
      )}

      {loading ? (
        <div className="text-gray-600">Loading...</div>
      ) : items.length === 0 ? (
        <div className="p-4 bg-white rounded-lg shadow text-gray-500 text-center">
          还没有数据
        </div>
      ) : (
        <div className="space-y-4">
          {sortedKeys.map((key) => (
            <section key={key} className="space-y-2">
              <h2 className="text-lg font-semibold text-gray-700">
                {formatGroupLabel(key)}
                <span className="ml-2 text-sm text-gray-400">
                  ({groups[key].length})
                </span>
              </h2>
              <div className="space-y-2">
                {groups[key].map((item) => (
                  <div
                    key={item.id}
                    className="p-4 bg-white rounded-lg shadow hover:shadow-md"
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div className="flex-1">
                        <p className="text-sm text-gray-600">
                          Script #{item.script_id}
                        </p>
                        {item.scheduled_at && (
                          <p className="text-xs text-gray-400 mt-1">
                            {item.scheduled_at}
                          </p>
                        )}
                        {item.published_at && (
                          <p className="text-xs text-green-600 mt-1">
                            已发布: {item.published_at}
                          </p>
                        )}
                        {item.platform_url && (
                          <a
                            href={item.platform_url}
                            target="_blank"
                            rel="noreferrer"
                            className="text-xs text-blue-600 hover:underline block mt-1 break-all"
                          >
                            {item.platform_url}
                          </a>
                        )}
                      </div>
                      <span className="px-2 py-1 bg-blue-50 text-blue-700 rounded text-xs">
                        {item.platform}
                      </span>
                    </div>
                  </div>
                ))}
              </div>
            </section>
          ))}
        </div>
      )}
    </div>
  )
}
