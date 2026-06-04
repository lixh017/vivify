'use client'

import { useEffect, useRef, useState, FormEvent } from 'react'
import { api } from '@/lib/api'
import type { KnowledgeDoc, IpTemplate } from '@/lib/types'
import type {
  ExportType,
  ImportType,
  ImportResult,
} from '@/lib/api'

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
  const [importExportNotice, setImportExportNotice] = useState<
    { kind: 'ok' | 'err'; text: string } | null
  >(null)
  const [importing, setImporting] = useState<ImportType | null>(null)
  const importInputRef = useRef<HTMLInputElement | null>(null)
  const pendingImportRef = useRef<ImportType | null>(null)

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

  // deleteDoc removes a knowledge doc via the API. We optimistically
  // drop it from the list and roll back on failure.
  async function deleteDoc(id: number) {
    const previous = docs
    setDocs((prev) => prev.filter((d) => d.id !== id))
    try {
      await api.knowledge.delete(id)
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : '删除失败')
      setDocs(previous)
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

  // Trigger a file download for the chosen export type. We navigate
  // directly to /export?type=… so the server's Content-Disposition
  // header does the right thing; doing this via fetch + Blob would
  // require hand-rolling the same UX (filename, attachment header,
  // URL revocation) for no upside.
  function handleExport(type: ExportType) {
    if (typeof window === 'undefined') return
    window.location.href = api.importExport.exportURL(type)
  }

  function handleImportClick(type: ImportType) {
    pendingImportRef.current = type
    importInputRef.current?.click()
  }

  async function handleImportFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    const type = pendingImportRef.current
    // Always reset the input value so re-selecting the same file
    // still fires onChange.
    e.target.value = ''
    pendingImportRef.current = null
    if (!file || !type) return
    setImporting(type)
    setImportExportNotice(null)
    try {
      const text = await file.text()
      const parsed = JSON.parse(text)
      const items = Array.isArray(parsed?.items) ? parsed.items : null
      if (!items) {
        setImportExportNotice({
          kind: 'err',
          text: '文件格式错误: 找不到 items 数组',
        })
        return
      }
      const result: ImportResult = await api.importExport.import(type, {
        items,
      })
      const errCount = result.errors.length
      if (errCount === 0) {
        setImportExportNotice({
          kind: 'ok',
          text: `导入成功 ${result.imported} 条`,
        })
      } else {
        const first = result.errors[0]
        setImportExportNotice({
          kind: 'err',
          text: `部分失败: 成功 ${result.imported} 条, 失败 ${errCount} 条 (第 ${first.index + 1} 条: ${first.message})`,
        })
      }
      // Reload whatever collection we just touched. The knowledge
      // page only owns docs, but the import could have hit scripts
      // or content items; those are owned by other pages so we keep
      // the reload scoped to docs.
      if (type === 'knowledge') {
        await loadDocs()
      }
    } catch (err: unknown) {
      setImportExportNotice({
        kind: 'err',
        text: err instanceof Error ? err.message : '导入失败',
      })
    } finally {
      setImporting(null)
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-end justify-between flex-wrap gap-3">
        <div>
          <h1 className="font-serif text-3xl text-claude-ink tracking-tight">
            📚 知识库
          </h1>
          <p className="text-claude-muted text-sm mt-1">
            IP 风格、SOP、复盘档案 — 你的第二大脑
          </p>
        </div>
        {tab === 'docs' ? (
          <button
            onClick={() => setShowForm((v) => !v)}
            className="px-3 md:px-4 py-1.5 md:py-2 text-xs md:text-sm bg-claude-coral text-claude-on-primary rounded-md hover:bg-claude-coral-active transition-colors"
          >
            {showForm ? '取消' : '+ 新建文档'}
          </button>
        ) : (
          <button
            onClick={() => setShowIPForm((v) => !v)}
            className="px-3 md:px-4 py-1.5 md:py-2 text-xs md:text-sm bg-claude-success text-claude-on-primary rounded-md hover:opacity-90 transition-opacity"
          >
            {showIPForm ? '取消' : '+ 新建 IP'}
          </button>
        )}
      </div>

      <div className="flex gap-2 border-b border-claude-hairline overflow-x-auto">
        <button
          onClick={() => {
            setTab('docs')
            setSelectedIP(null)
          }}
          className={
            'px-3 md:px-4 py-2 text-xs md:text-sm font-medium border-b-2 -mb-px transition-colors whitespace-nowrap ' +
            (tab === 'docs'
              ? 'border-claude-coral text-claude-coral'
              : 'border-transparent text-claude-muted hover:text-claude-ink')
          }
        >
          文档
        </button>
        <button
          onClick={() => setTab('ip-templates')}
          className={
            'px-3 md:px-4 py-2 text-xs md:text-sm font-medium border-b-2 -mb-px transition-colors whitespace-nowrap ' +
            (tab === 'ip-templates'
              ? 'border-claude-success text-claude-success'
              : 'border-transparent text-claude-muted hover:text-claude-ink')
          }
        >
          IP 模板
        </button>
      </div>

      {error && (
        <div className="p-3 bg-claude-error/10 border border-claude-error text-claude-error rounded text-sm">
          {error}
        </div>
      )}

      {importExportNotice && (
        <div
          className={
            'p-3 rounded border text-sm ' +
            (importExportNotice.kind === 'ok'
              ? 'bg-claude-success/10 border-claude-success/30 text-claude-success'
              : 'bg-claude-surface-card border-claude-error text-claude-error')
          }
        >
          {importExportNotice.text}
        </div>
      )}

      <ImportExportPanel
        importing={importing}
        onExport={handleExport}
        onImport={handleImportClick}
      />
      <input
        ref={importInputRef}
        type="file"
        accept="application/json,.json"
        className="hidden"
        onChange={handleImportFileChange}
      />

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
          onDelete={deleteDoc}
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
  onDelete: (id: number) => void
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
  onDelete,
}: DocsTabProps) {
  return (
    <>
      {showForm && (
        <form
          onSubmit={onSubmit}
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
              className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
            />
          </div>
          <div>
            <label className="block text-xs md:text-sm font-medium text-claude-ink">
              路径
            </label>
            <input
              type="text"
              required
              data-testid="input-test-path"
              placeholder="例如: panda/characters/mama"
              value={form.path}
              onChange={(e) => setForm({ ...form, path: e.target.value })}
              className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
            />
          </div>
          <div>
            <label className="block text-xs md:text-sm font-medium text-claude-ink">
              类型
            </label>
            <select
              data-testid="input-test-doc_type"
              value={form.doc_type}
              onChange={(e) => setForm({ ...form, doc_type: e.target.value })}
              className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink"
            >
              {DOC_TYPES.map((t) => (
                <option key={t} value={t}>
                  {t}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-xs md:text-sm font-medium text-claude-ink">
              内容
            </label>
            <textarea
              required
              data-testid="input-test-content"
              value={form.content}
              onChange={(e) => setForm({ ...form, content: e.target.value })}
              className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
              rows={6}
            />
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

      {loading ? (
        <div className="text-sm text-claude-body">加载中…</div>
      ) : sortedKeys.length === 0 ? (
        <div className="p-4 bg-claude-surface-card rounded-lg border border-claude-hairline text-sm text-claude-muted text-center">
          还没有数据
        </div>
      ) : (
        <div className="space-y-4">
          {sortedKeys.map((key) => (
            <section key={key} className="space-y-2">
              <h2 className="font-serif text-lg text-claude-ink">
                {key}
                <span className="ml-2 text-xs md:text-sm text-claude-muted-soft">
                  ({groups[key].length})
                </span>
              </h2>
              <div className="space-y-2">
                {groups[key].map((doc) => {
                  const segments = pathToBreadcrumb(doc.path)
                  return (
                    <div
                      key={doc.id}
                      data-testid="list-item"
                      data-doc-id={doc.id}
                      className="p-3 md:p-4 bg-claude-surface-card rounded-lg border border-claude-hairline hover:border-claude-coral/50 transition-colors"
                    >
                      <div className="flex items-start justify-between gap-3">
                        <div className="flex-1 min-w-0">
                          <h3 className="font-semibold text-sm md:text-base text-claude-ink">
                            {doc.title}
                          </h3>
                          {segments.length > 0 && (
                            <p className="text-xs text-claude-muted mt-1 break-all">
                              {segments.map((seg, i) => (
                                <span key={i}>
                                  {i > 0 && (
                                    <span className="mx-1 text-claude-muted-soft">
                                      /
                                    </span>
                                  )}
                                  {seg}
                                </span>
                              ))}
                            </p>
                          )}
                          {doc.content && (
                            <p className="text-xs md:text-sm text-claude-body mt-2 line-clamp-3 whitespace-pre-wrap">
                              {doc.content}
                            </p>
                          )}
                        </div>
                        <button
                          type="button"
                          data-testid={`btn-delete-${doc.id}`}
                          onClick={() => onDelete(doc.id)}
                          aria-label="删除"
                          className="shrink-0 px-2 py-1 text-xs bg-claude-canvas hover:bg-claude-surface-soft text-claude-error rounded border border-claude-hairline"
                        >
                          删除
                        </button>
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
          className="p-3 md:p-4 bg-claude-surface-card rounded-lg border border-claude-hairline space-y-3"
        >
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            <div>
              <label className="block text-xs md:text-sm font-medium text-claude-ink">
                IP 类型
              </label>
              <input
                type="text"
                required
                placeholder="例如: 数字人 / 古装 / 言情"
                value={form.type}
                onChange={(e) => setForm({ ...form, type: e.target.value })}
                className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
              />
            </div>
            <div>
              <label className="block text-xs md:text-sm font-medium text-claude-ink">
                名称
              </label>
              <input
                type="text"
                required
                placeholder="例如: 云岚"
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
              />
            </div>
          </div>
          <div>
            <label className="block text-xs md:text-sm font-medium text-claude-ink">
              简介 (可选)
            </label>
            <textarea
              value={form.description}
              onChange={(e) =>
                setForm({ ...form, description: e.target.value })
              }
              className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
              rows={2}
            />
          </div>
          <div className="text-xs text-claude-muted">
            提交后会自动创建 4 份标准文档：风格指南 / 语气调性 / 种子选题 /
            反面清单
          </div>
          <button
            type="submit"
            disabled={submitting}
            className="w-full sm:w-auto px-4 py-2 text-sm bg-claude-success text-claude-on-primary rounded-md hover:opacity-90 disabled:opacity-50 transition-opacity"
          >
            {submitting ? '提交中...' : '创建 IP 模板'}
          </button>
        </form>
      )}

      {selectedIP ? (
        <IpDetail ipType={selectedIP} docs={ipDocs} onBack={() => onSelect(null)} />
      ) : templates.length === 0 ? (
        <div className="p-4 bg-claude-surface-card rounded-lg border border-claude-hairline text-sm text-claude-muted text-center">
          还没有 IP 模板 — 点击右上角「+ 新建 IP」创建第一个
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
          {templates.map((tpl) => (
            <button
              key={tpl.type}
              onClick={() => onSelect(tpl.type)}
              className="p-3 md:p-4 bg-claude-surface-card rounded-lg border border-claude-hairline hover:border-claude-coral/50 text-left transition-colors"
            >
              <div className="flex items-baseline justify-between gap-2">
                <h3 className="font-semibold text-sm md:text-base text-claude-ink">
                  {tpl.name}
                </h3>
                <span className="text-xs text-claude-muted">{tpl.type}</span>
              </div>
              {tpl.description && (
                <p className="text-xs md:text-sm text-claude-body mt-2 line-clamp-3">
                  {tpl.description}
                </p>
              )}
              <p className="text-xs text-claude-muted-soft mt-2">
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
      <div className="flex items-center gap-2 md:gap-3 flex-wrap">
        <button
          onClick={onBack}
          className="px-3 py-1 text-xs md:text-sm bg-claude-surface-soft rounded hover:bg-claude-surface-card text-claude-body border border-claude-hairline"
        >
          ← 返回
        </button>
        <h2 className="text-lg md:text-xl font-semibold break-all text-claude-ink">
          {ipType}
        </h2>
        <span className="text-xs md:text-sm text-claude-muted">
          ({docs.length} 份文档)
        </span>
      </div>
      {docs.length === 0 ? (
        <div className="p-4 bg-claude-surface-card rounded-lg border border-claude-hairline text-sm text-claude-muted text-center">
          这个 IP 还没有关联文档
        </div>
      ) : (
        <div className="space-y-2">
          {docs.map((doc) => {
            const segments = pathToBreadcrumb(doc.path)
            return (
              <div
                key={doc.id}
                className="p-3 md:p-4 bg-claude-surface-card rounded-lg border border-claude-hairline hover:border-claude-coral/50 transition-colors"
              >
                <h3 className="font-semibold text-sm md:text-base text-claude-ink">
                  {doc.title}
                </h3>
                {segments.length > 0 && (
                  <p className="text-xs text-claude-muted mt-1 break-all">
                    {segments.join(' / ')}
                  </p>
                )}
                {doc.content && (
                  <p className="text-xs md:text-sm text-claude-body mt-2 whitespace-pre-wrap">
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

interface ImportExportPanelProps {
  importing: ImportType | null
  onExport: (type: ExportType) => void
  onImport: (type: ImportType) => void
}

const EXPORT_TYPES: { value: ExportType; label: string }[] = [
  { value: 'all', label: '全部' },
  { value: 'topic', label: '选题' },
  { value: 'script', label: '脚本' },
  { value: 'content_item', label: '发布记录' },
  { value: 'knowledge', label: '知识库' },
]

const IMPORT_TYPES: { value: ImportType; label: string }[] = [
  { value: 'topic', label: '选题' },
  { value: 'script', label: '脚本' },
  { value: 'content_item', label: '发布记录' },
  { value: 'knowledge', label: '知识库' },
]

// ImportExportPanel exposes the JSON-based bulk import/export from a
// single place in the knowledge page. The same handlers work for
// every entity; we deliberately keep the panel visible at the top so
// a user doing a backup round-trip (export → download → re-import
// elsewhere) does not have to dig through per-collection pages.
function ImportExportPanel({
  importing,
  onExport,
  onImport,
}: ImportExportPanelProps) {
  return (
    <details className="p-3 md:p-4 bg-claude-surface-soft border border-claude-hairline rounded-lg">
      <summary className="cursor-pointer text-xs md:text-sm font-medium text-claude-ink select-none">
        ⚙️ 数据导入 / 导出 (JSON)
      </summary>
      <div className="mt-3 grid grid-cols-1 md:grid-cols-2 gap-3 md:gap-4">
        <section>
          <h3 className="text-xs md:text-sm font-semibold text-claude-ink mb-2">
            导出
          </h3>
          <div className="flex flex-wrap gap-2">
            {EXPORT_TYPES.map((t) => (
              <button
                key={t.value}
                onClick={() => onExport(t.value)}
                className="px-2.5 md:px-3 py-1 md:py-1.5 text-xs md:text-sm bg-claude-canvas border border-claude-hairline rounded hover:bg-claude-surface-card text-claude-body"
              >
                导出 {t.label}
              </button>
            ))}
          </div>
        </section>
        <section>
          <h3 className="text-xs md:text-sm font-semibold text-claude-ink mb-2">
            导入
          </h3>
          <div className="flex flex-wrap gap-2">
            {IMPORT_TYPES.map((t) => (
              <button
                key={t.value}
                onClick={() => onImport(t.value)}
                disabled={importing !== null}
                className="px-2.5 md:px-3 py-1 md:py-1.5 text-xs md:text-sm bg-claude-canvas border border-claude-hairline rounded hover:bg-claude-surface-card text-claude-body disabled:opacity-50"
              >
                {importing === t.value ? '导入中…' : `导入 ${t.label}`}
              </button>
            ))}
          </div>
          <p className="mt-2 text-xs text-claude-muted">
            导入文件需为导出的 JSON (包含 items 数组)。导入会跳过原 ID 并分配新主键。
          </p>
        </section>
      </div>
    </details>
  )
}
