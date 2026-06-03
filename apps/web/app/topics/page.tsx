'use client'

import { useEffect, useState, FormEvent } from 'react'
import { api } from '@/lib/api'
import type { Topic } from '@/lib/types'

const PLATFORMS = ['抖音', '小红书', 'B站', '视频号', 'YouTube']
const STATUSES = ['idea', 'writing', 'review', 'approved', 'archived']

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

export default function TopicsPage() {
  const [topics, setTopics] = useState<Topic[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState<FormState>(EMPTY_FORM)
  const [submitting, setSubmitting] = useState(false)
  const [filterPlatform, setFilterPlatform] = useState<string>('')
  const [filterStatus, setFilterStatus] = useState<string>('')

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

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-3xl font-bold">📋 选题</h1>
        <button
          onClick={() => setShowForm((v) => !v)}
          className="px-4 py-2 bg-blue-600 text-white rounded-lg shadow hover:bg-blue-700"
        >
          {showForm ? '取消' : '+ 新建选题'}
        </button>
      </div>

      <div className="flex gap-3 items-center">
        <label className="text-sm text-gray-600">
          平台:
          <select
            value={filterPlatform}
            onChange={(e) => setFilterPlatform(e.target.value)}
            className="ml-2 px-2 py-1 border border-gray-300 rounded"
          >
            <option value="">全部</option>
            {PLATFORMS.map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
          </select>
        </label>
        <label className="text-sm text-gray-600">
          状态:
          <select
            value={filterStatus}
            onChange={(e) => setFilterStatus(e.target.value)}
            className="ml-2 px-2 py-1 border border-gray-300 rounded"
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

      {showForm && (
        <form
          onSubmit={handleSubmit}
          className="p-4 bg-white rounded-lg shadow space-y-3"
        >
          <div>
            <label className="block text-sm font-medium text-gray-700">
              标题
            </label>
            <input
              type="text"
              required
              value={form.title}
              onChange={(e) => setForm({ ...form, title: e.target.value })}
              className="mt-1 w-full px-3 py-2 border border-gray-300 rounded"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700">
              角度
            </label>
            <textarea
              required
              value={form.angle}
              onChange={(e) => setForm({ ...form, angle: e.target.value })}
              className="mt-1 w-full px-3 py-2 border border-gray-300 rounded"
              rows={3}
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
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
                状态
              </label>
              <select
                value={form.status}
                onChange={(e) => setForm({ ...form, status: e.target.value })}
                className="mt-1 w-full px-3 py-2 border border-gray-300 rounded"
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
      ) : topics.length === 0 ? (
        <div className="p-4 bg-white rounded-lg shadow text-gray-500 text-center">
          还没有数据
        </div>
      ) : (
        <div className="space-y-3">
          {topics.map((t) => (
            <div
              key={t.id}
              className="p-4 bg-white rounded-lg shadow hover:shadow-md"
            >
              <div className="flex items-start justify-between">
                <div className="flex-1">
                  <h2 className="font-semibold text-lg">{t.title}</h2>
                  <p className="text-sm text-gray-600 mt-1">{t.angle}</p>
                </div>
                <div className="flex gap-2 text-xs">
                  <span className="px-2 py-1 bg-blue-50 text-blue-700 rounded">
                    {t.platform}
                  </span>
                  <span className="px-2 py-1 bg-gray-100 text-gray-700 rounded">
                    {t.status}
                  </span>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
