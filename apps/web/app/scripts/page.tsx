'use client'

import { useEffect, useState, FormEvent } from 'react'
import { api } from '@/lib/api'
import type { Script } from '@/lib/types'

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

export default function ScriptsPage() {
  const [scripts, setScripts] = useState<Script[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState<FormState>(EMPTY_FORM)
  const [submitting, setSubmitting] = useState(false)
  const [filterPlatform, setFilterPlatform] = useState<string>('')
  const [humanizing, setHumanizing] = useState(false)
  const [toast, setToast] = useState<string | null>(null)

  async function loadScripts() {
    setLoading(true)
    setError(null)
    try {
      const params: { platform?: string } = {}
      if (filterPlatform) params.platform = filterPlatform
      const res = await api.scripts.list(params)
      setScripts(res.items)
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : '加载失败')
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
      setError(err instanceof Error ? err.message : '创建失败')
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
      setError(err instanceof Error ? err.message : '删除失败')
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
      setToast('请先输入脚本内容')
      return
    }
    if (!window.confirm('会改写当前内容,继续?')) {
      return
    }
    setHumanizing(true)
    setToast(null)
    try {
      const res = await api.ai.humanize({ script: content })
      setForm((prev) => ({ ...prev, content: res.humanized }))
    } catch (err: unknown) {
      setToast(err instanceof Error ? err.message : 'AI 拟人化失败')
    } finally {
      setHumanizing(false)
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between flex-wrap gap-2">
        <h1 className="text-2xl md:text-3xl font-bold">📝 脚本</h1>
        <button
          onClick={() => setShowForm((v) => !v)}
          className="px-3 md:px-4 py-1.5 md:py-2 text-xs md:text-sm bg-blue-600 text-white rounded-lg shadow hover:bg-blue-700"
        >
          {showForm ? '取消' : '+ 新建脚本'}
        </button>
      </div>

      <div className="flex gap-3 items-center">
        <label className="text-xs md:text-sm text-gray-600 flex items-center gap-2">
          平台:
          <select
            value={filterPlatform}
            onChange={(e) => setFilterPlatform(e.target.value)}
            className="px-2 py-1 text-xs md:text-sm border border-gray-300 rounded"
          >
            <option value="">全部</option>
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
          className="p-3 md:p-4 bg-white rounded-lg shadow space-y-3"
        >
          <div>
            <label className="block text-xs md:text-sm font-medium text-gray-700">
              标题
            </label>
            <input
              type="text"
              required
              data-testid="input-test-title"
              value={form.title}
              onChange={(e) => setForm({ ...form, title: e.target.value })}
              className="mt-1 w-full px-3 py-2 text-sm border border-gray-300 rounded"
            />
          </div>
          <div>
            <div className="flex items-center justify-between flex-wrap gap-2">
              <label className="block text-xs md:text-sm font-medium text-gray-700">
                内容 (Markdown)
              </label>
              <button
                type="button"
                data-testid="btn-ai-humanize"
                onClick={handleHumanize}
                disabled={humanizing || !form.content.trim()}
                className="px-2.5 md:px-3 py-1 text-xs md:text-sm bg-purple-600 text-white rounded shadow hover:bg-purple-700 disabled:opacity-50"
                title="调用 Claude 把当前内容改写得不像 AI 写的"
              >
                {humanizing ? '拟人化中...' : 'AI 拟人化'}
              </button>
            </div>
            <textarea
              required
              data-testid="input-test-content"
              value={form.content}
              onChange={(e) => setForm({ ...form, content: e.target.value })}
              className="mt-1 w-full px-3 py-2 border border-gray-300 rounded font-mono text-xs md:text-sm"
              rows={10}
            />
          </div>
          <div>
            <label className="block text-xs md:text-sm font-medium text-gray-700">
              平台
            </label>
            <select
              data-testid="input-test-platform"
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
          <button
            type="submit"
            data-testid="btn-submit"
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

      {toast && (
        <div className="p-3 bg-yellow-50 border border-yellow-200 text-yellow-800 rounded text-sm">
          {toast}
        </div>
      )}

      {loading ? (
        <div className="text-sm text-gray-600">加载中…</div>
      ) : scripts.length === 0 ? (
        <div className="p-4 bg-white rounded-lg shadow text-sm text-gray-500 text-center">
          还没有数据
        </div>
      ) : (
        <div className="space-y-3">
          {scripts.map((s) => (
            <div
              key={s.id}
              data-testid="list-item"
              data-script-id={s.id}
              className="p-3 md:p-4 bg-white rounded-lg shadow hover:shadow-md"
            >
              <div className="flex items-start justify-between gap-3 flex-col sm:flex-row">
                <div className="flex-1 min-w-0">
                  <h2 className="font-semibold text-sm md:text-base">
                    {s.title}
                  </h2>
                  <p className="text-xs md:text-sm text-gray-600 mt-1 line-clamp-3 whitespace-pre-wrap font-mono">
                    {s.content}
                  </p>
                </div>
                <div className="flex gap-2 text-xs flex-wrap">
                  <span className="px-2 py-1 bg-blue-50 text-blue-700 rounded">
                    {s.platform}
                  </span>
                  {s.word_count > 0 && (
                    <span className="px-2 py-1 bg-gray-100 text-gray-700 rounded">
                      {s.word_count} 字
                    </span>
                  )}
                  <button
                    type="button"
                    data-testid={`btn-delete-${s.id}`}
                    onClick={() => deleteScript(s.id)}
                    aria-label="删除"
                    className="px-2 py-1 bg-red-50 hover:bg-red-100 text-red-700 rounded border border-red-200"
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
