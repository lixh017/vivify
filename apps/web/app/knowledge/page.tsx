'use client'

import { useEffect, useState, FormEvent } from 'react'
import { api } from '@/lib/api'
import type { KnowledgeDoc } from '@/lib/types'

const DOC_TYPES = ['SOP', '人物', '世界观', '历史', '其它']

interface FormState {
  title: string
  path: string
  content: string
  doc_type: string
}

const EMPTY_FORM: FormState = {
  title: '',
  path: '',
  content: '',
  doc_type: DOC_TYPES[0],
}

function pathToBreadcrumb(path: string): string[] {
  if (!path) return []
  return path.split('/').filter((segment) => segment.length > 0)
}

export default function KnowledgePage() {
  const [docs, setDocs] = useState<KnowledgeDoc[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState<FormState>(EMPTY_FORM)
  const [submitting, setSubmitting] = useState(false)

  async function loadDocs() {
    setLoading(true)
    setError(null)
    try {
      const res = await api.knowledge.list()
      setDocs(res.items)
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadDocs()
  }, [])

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setSubmitting(true)
    setError(null)
    try {
      await api.knowledge.create({
        title: form.title,
        path: form.path,
        content: form.content,
        doc_type: form.doc_type,
      })
      setForm(EMPTY_FORM)
      setShowForm(false)
      await loadDocs()
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : '创建失败')
    } finally {
      setSubmitting(false)
    }
  }

  const groups = docs.reduce<Record<string, KnowledgeDoc[]>>((acc, doc) => {
    const key = doc.doc_type || '未分类'
    if (!acc[key]) acc[key] = []
    acc[key].push(doc)
    return acc
  }, {})

  const sortedKeys = Object.keys(groups).sort((a, b) => a.localeCompare(b))

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-3xl font-bold">📚 知识库</h1>
        <button
          onClick={() => setShowForm((v) => !v)}
          className="px-4 py-2 bg-blue-600 text-white rounded-lg shadow hover:bg-blue-700"
        >
          {showForm ? '取消' : '+ 新建文档'}
        </button>
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
              路径
            </label>
            <input
              type="text"
              required
              placeholder="例如: panda/characters/mama"
              value={form.path}
              onChange={(e) => setForm({ ...form, path: e.target.value })}
              className="mt-1 w-full px-3 py-2 border border-gray-300 rounded"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700">
              类型
            </label>
            <select
              value={form.doc_type}
              onChange={(e) => setForm({ ...form, doc_type: e.target.value })}
              className="mt-1 w-full px-3 py-2 border border-gray-300 rounded"
            >
              {DOC_TYPES.map((t) => (
                <option key={t} value={t}>
                  {t}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700">
              内容
            </label>
            <textarea
              required
              value={form.content}
              onChange={(e) => setForm({ ...form, content: e.target.value })}
              className="mt-1 w-full px-3 py-2 border border-gray-300 rounded"
              rows={6}
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
      ) : docs.length === 0 ? (
        <div className="p-4 bg-white rounded-lg shadow text-gray-500 text-center">
          还没有数据
        </div>
      ) : (
        <div className="space-y-4">
          {sortedKeys.map((key) => (
            <section key={key} className="space-y-2">
              <h2 className="text-lg font-semibold text-gray-700">
                {key}
                <span className="ml-2 text-sm text-gray-400">
                  ({groups[key].length})
                </span>
              </h2>
              <div className="space-y-2">
                {groups[key].map((doc) => {
                  const segments = pathToBreadcrumb(doc.path)
                  return (
                    <div
                      key={doc.id}
                      className="p-4 bg-white rounded-lg shadow hover:shadow-md"
                    >
                      <div className="flex items-start justify-between gap-3">
                        <div className="flex-1 min-w-0">
                          <h3 className="font-semibold text-lg">
                            {doc.title}
                          </h3>
                          {segments.length > 0 && (
                            <p className="text-xs text-gray-500 mt-1 break-all">
                              {segments.map((seg, i) => (
                                <span key={i}>
                                  {i > 0 && (
                                    <span className="mx-1 text-gray-400">
                                      /
                                    </span>
                                  )}
                                  {seg}
                                </span>
                              ))}
                            </p>
                          )}
                          {doc.content && (
                            <p className="text-sm text-gray-600 mt-2 line-clamp-3 whitespace-pre-wrap">
                              {doc.content}
                            </p>
                          )}
                        </div>
                      </div>
                    </div>
                  )
                })}
              </div>
            </section>
          ))}
        </div>
      )}
    </div>
  )
}
