'use client'

import { useEffect, useState, FormEvent } from 'react'
import { api } from '@/lib/api'
import type { KnowledgeDoc, IpTemplate } from '@/lib/types'

const DOC_TYPES = ['SOP', '人物', '世界观', '历史', '其它']

type Tab = 'docs' | 'ip-templates'

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

interface IPFormState {
  type: string
  name: string
  description: string
}

const EMPTY_IP_FORM: IPFormState = {
  type: '',
  name: '',
  description: '',
}

function pathToBreadcrumb(path: string): string[] {
  if (!path) return []
  return path.split('/').filter((segment) => segment.length > 0)
}

// Count knowledge docs that belong to a given IP type. The backend
// stores IP-template docs under `ip-style-guide/<type>/...`, so we
// match by that prefix. This keeps the IP detail view in sync with the
// underlying CRUD without any extra state on the client.
function docsForIP(docs: KnowledgeDoc[], ipType: string): KnowledgeDoc[] {
  const prefix = `ip-style-guide/${ipType}/`
  return docs.filter((d) => d.path.startsWith(prefix))
}

export default function KnowledgePage() {
  const [tab, setTab] = useState<Tab>('docs')
  const [docs, setDocs] = useState<KnowledgeDoc[]>([])
  const [templates, setTemplates] = useState<IpTemplate[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState<FormState>(EMPTY_FORM)
  const [submitting, setSubmitting] = useState(false)
  const [showIPForm, setShowIPForm] = useState(false)
  const [ipForm, setIPForm] = useState<IPFormState>(EMPTY_IP_FORM)
  const [ipSubmitting, setIPSubmitting] = useState(false)
  const [selectedIP, setSelectedIP] = useState<string | null>(null)

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

  async function loadTemplates() {
    try {
      const res = await api.ipTemplates.list()
      setTemplates(res.templates)
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : '加载 IP 模板失败')
    }
  }

  useEffect(() => {
    void loadDocs()
    void loadTemplates()
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

  async function handleIPSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setIPSubmitting(true)
    setError(null)
    try {
      const payload: { type: string; name: string; description?: string } = {
        type: ipForm.type.trim(),
        name: ipForm.name.trim(),
      }
      if (ipForm.description.trim()) {
        payload.description = ipForm.description.trim()
      }
      await api.ipTemplates.create(payload)
      setIPForm(EMPTY_IP_FORM)
      setShowIPForm(false)
      await Promise.all([loadTemplates(), loadDocs()])
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : '创建 IP 失败')
    } finally {
      setIPSubmitting(false)
    }
  }

  const groups = docs.reduce<Record<string, KnowledgeDoc[]>>((acc, doc) => {
    const key = doc.doc_type || '未分类'
    if (!acc[key]) acc[key] = []
    acc[key].push(doc)
    return acc
  }, {})

  const sortedKeys = Object.keys(groups).sort((a, b) => a.localeCompare(b))
  const ipDocs = selectedIP ? docsForIP(docs, selectedIP) : []

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-3xl font-bold">📚 知识库</h1>
        {tab === 'docs' ? (
          <button
            onClick={() => setShowForm((v) => !v)}
            className="px-4 py-2 bg-blue-600 text-white rounded-lg shadow hover:bg-blue-700"
          >
            {showForm ? '取消' : '+ 新建文档'}
          </button>
        ) : (
          <button
            onClick={() => setShowIPForm((v) => !v)}
            className="px-4 py-2 bg-emerald-600 text-white rounded-lg shadow hover:bg-emerald-700"
          >
            {showIPForm ? '取消' : '+ 新建 IP'}
          </button>
        )}
      </div>

      <div className="flex gap-2 border-b border-gray-200">
        <button
          onClick={() => {
            setTab('docs')
            setSelectedIP(null)
          }}
          className={
            'px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors ' +
            (tab === 'docs'
              ? 'border-blue-600 text-blue-700'
              : 'border-transparent text-gray-500 hover:text-gray-700')
          }
        >
          文档
        </button>
        <button
          onClick={() => setTab('ip-templates')}
          className={
            'px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors ' +
            (tab === 'ip-templates'
              ? 'border-emerald-600 text-emerald-700'
              : 'border-transparent text-gray-500 hover:text-gray-700')
          }
        >
          IP 模板
        </button>
      </div>

      {error && (
        <div className="p-3 bg-red-50 border border-red-200 text-red-700 rounded">
          {error}
        </div>
      )}

      {tab === 'docs' && (
        <DocsTab
          showForm={showForm}
          form={form}
          setForm={setForm}
          submitting={submitting}
          onSubmit={handleSubmit}
          loading={loading}
          groups={groups}
          sortedKeys={sortedKeys}
        />
      )}

      {tab === 'ip-templates' && (
        <IpTemplatesTab
          templates={templates}
          showForm={showIPForm}
          form={ipForm}
          setForm={setIPForm}
          submitting={ipSubmitting}
          onSubmit={handleIPSubmit}
          selectedIP={selectedIP}
          onSelect={setSelectedIP}
          ipDocs={ipDocs}
        />
      )}
    </div>
  )
}

interface DocsTabProps {
  showForm: boolean
  form: FormState
  setForm: React.Dispatch<React.SetStateAction<FormState>>
  submitting: boolean
  onSubmit: (e: FormEvent<HTMLFormElement>) => void
  loading: boolean
  groups: Record<string, KnowledgeDoc[]>
  sortedKeys: string[]
}

function DocsTab({
  showForm,
  form,
  setForm,
  submitting,
  onSubmit,
  loading,
  groups,
  sortedKeys,
}: DocsTabProps) {
  return (
    <>
      {showForm && (
        <form
          onSubmit={onSubmit}
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

      {loading ? (
        <div className="text-gray-600">加载中…</div>
      ) : sortedKeys.length === 0 ? (
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
    </>
  )
}

interface IpTemplatesTabProps {
  templates: IpTemplate[]
  showForm: boolean
  form: IPFormState
  setForm: React.Dispatch<React.SetStateAction<IPFormState>>
  submitting: boolean
  onSubmit: (e: FormEvent<HTMLFormElement>) => void
  selectedIP: string | null
  onSelect: (ipType: string | null) => void
  ipDocs: KnowledgeDoc[]
}

function IpTemplatesTab({
  templates,
  showForm,
  form,
  setForm,
  submitting,
  onSubmit,
  selectedIP,
  onSelect,
  ipDocs,
}: IpTemplatesTabProps) {
  return (
    <div className="space-y-4">
      {showForm && (
        <form
          onSubmit={onSubmit}
          className="p-4 bg-white rounded-lg shadow space-y-3"
        >
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            <div>
              <label className="block text-sm font-medium text-gray-700">
                IP 类型
              </label>
              <input
                type="text"
                required
                placeholder="例如: 数字人 / 古装 / 言情"
                value={form.type}
                onChange={(e) => setForm({ ...form, type: e.target.value })}
                className="mt-1 w-full px-3 py-2 border border-gray-300 rounded"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700">
                名称
              </label>
              <input
                type="text"
                required
                placeholder="例如: 云岚"
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                className="mt-1 w-full px-3 py-2 border border-gray-300 rounded"
              />
            </div>
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700">
              简介 (可选)
            </label>
            <textarea
              value={form.description}
              onChange={(e) =>
                setForm({ ...form, description: e.target.value })
              }
              className="mt-1 w-full px-3 py-2 border border-gray-300 rounded"
              rows={2}
            />
          </div>
          <div className="text-xs text-gray-500">
            提交后会自动创建 4 份标准文档：风格指南 / 语气调性 / 种子选题 /
            反面清单
          </div>
          <button
            type="submit"
            disabled={submitting}
            className="px-4 py-2 bg-emerald-600 text-white rounded-lg shadow hover:bg-emerald-700 disabled:opacity-50"
          >
            {submitting ? '提交中...' : '创建 IP 模板'}
          </button>
        </form>
      )}

      {selectedIP ? (
        <IpDetail ipType={selectedIP} docs={ipDocs} onBack={() => onSelect(null)} />
      ) : templates.length === 0 ? (
        <div className="p-4 bg-white rounded-lg shadow text-gray-500 text-center">
          还没有 IP 模板 — 点击右上角「+ 新建 IP」创建第一个
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
          {templates.map((tpl) => (
            <button
              key={tpl.type}
              onClick={() => onSelect(tpl.type)}
              className="p-4 bg-white rounded-lg shadow hover:shadow-md text-left transition-shadow"
            >
              <div className="flex items-baseline justify-between gap-2">
                <h3 className="font-semibold text-lg text-gray-900">
                  {tpl.name}
                </h3>
                <span className="text-xs text-gray-500">{tpl.type}</span>
              </div>
              {tpl.description && (
                <p className="text-sm text-gray-600 mt-2 line-clamp-3">
                  {tpl.description}
                </p>
              )}
              <p className="text-xs text-gray-400 mt-2">
                {tpl.doc_count} 份文档
              </p>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

interface IpDetailProps {
  ipType: string
  docs: KnowledgeDoc[]
  onBack: () => void
}

function IpDetail({ ipType, docs, onBack }: IpDetailProps) {
  return (
    <div className="space-y-3">
      <div className="flex items-center gap-3">
        <button
          onClick={onBack}
          className="px-3 py-1 text-sm bg-gray-100 rounded hover:bg-gray-200"
        >
          ← 返回
        </button>
        <h2 className="text-xl font-semibold">{ipType}</h2>
        <span className="text-sm text-gray-500">({docs.length} 份文档)</span>
      </div>
      {docs.length === 0 ? (
        <div className="p-4 bg-white rounded-lg shadow text-gray-500 text-center">
          这个 IP 还没有关联文档
        </div>
      ) : (
        <div className="space-y-2">
          {docs.map((doc) => {
            const segments = pathToBreadcrumb(doc.path)
            return (
              <div
                key={doc.id}
                className="p-4 bg-white rounded-lg shadow hover:shadow-md"
              >
                <h3 className="font-semibold">{doc.title}</h3>
                {segments.length > 0 && (
                  <p className="text-xs text-gray-500 mt-1 break-all">
                    {segments.join(' / ')}
                  </p>
                )}
                {doc.content && (
                  <p className="text-sm text-gray-600 mt-2 whitespace-pre-wrap">
                    {doc.content}
                  </p>
                )}
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
