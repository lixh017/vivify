'use client'

import { useEffect, useState, FormEvent } from 'react'
import { api } from '@/lib/api'
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

export default function DashboardPage() {
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

  const totalCount = items.length
  const withUrlCount = items.filter((i) => i.platform_url && i.platform_url.length > 0).length

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-3xl font-bold">📊 表现</h1>
        <button
          onClick={() => setShowForm((v) => !v)}
          className="px-4 py-2 bg-blue-600 text-white rounded-lg shadow hover:bg-blue-700"
        >
          {showForm ? '取消' : '+ 新建记录'}
        </button>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div className="p-4 bg-white rounded-lg shadow">
          <p className="text-sm text-gray-500">总记录数</p>
          <p className="text-2xl font-bold mt-1">{totalCount}</p>
        </div>
        <div className="p-4 bg-white rounded-lg shadow">
          <p className="text-sm text-gray-500">含平台链接</p>
          <p className="text-2xl font-bold mt-1">{withUrlCount}</p>
        </div>
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
            <label className="block text-sm font-medium text-gray-700">
              平台链接
            </label>
            <input
              type="url"
              value={form.platform_url}
              onChange={(e) =>
                setForm({ ...form, platform_url: e.target.value })
              }
              placeholder="https://..."
              className="mt-1 w-full px-3 py-2 border border-gray-300 rounded"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700">
              表现数据
            </label>
            <textarea
              value={form.performance_metrics}
              onChange={(e) =>
                setForm({ ...form, performance_metrics: e.target.value })
              }
              placeholder="例如: 播放 12k, 点赞 800, 评论 45"
              className="mt-1 w-full px-3 py-2 border border-gray-300 rounded"
              rows={3}
            />
          </div>
          <button
            type="submit"
            disabled={submitting}
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
        <div className="space-y-3">
          {items.map((item) => (
            <div
              key={item.id}
              className="p-4 bg-white rounded-lg shadow hover:shadow-md"
            >
              <div className="flex items-start justify-between gap-3">
                <div className="flex-1">
                  <p className="text-sm text-gray-600">
                    Script #{item.script_id}
                  </p>
                  {item.published_at && (
                    <p className="text-xs text-green-600 mt-1">
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
                    <p className="text-sm text-gray-700 mt-2 whitespace-pre-wrap">
                      {item.performance_metrics}
                    </p>
                  )}
                </div>
                <span className="px-2 py-1 bg-blue-50 text-blue-700 rounded text-xs">
                  {item.platform}
                </span>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
