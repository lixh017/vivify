'use client'

import { useEffect, useRef, useState, FormEvent } from 'react'
import { api } from '@/lib/api'
import type { KnowledgeDoc, IpTemplate } from '@/lib/types'
import type {
  ExportType,
  ImportType,
  ImportResult,
} from '@/lib/api'
import { useT } from '@/lib/i18n-client'

// Canonical doc type vocabulary mirroring the backend's allowedDocTypes
// set. Wire values; user-facing labels for "Character/World/History/Other"
// are looked up by raw key from the i18n catalog so the labels switch
// with the active locale while the API value stays the same.
const DOC_TYPES = ['SOP', '人物', '世界观', '历史', '其它'] as const

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

// docTypeLabel returns the user-facing label for a raw doc type
// value. Falls back to a localized "Unknown" key for backend values
// we don't recognize so the UI never leaks raw wire strings to the
// end user. The SENTINEL "未分类" value (returned from a missing
// doc_type column) is rendered as the localized "Uncategorized"
// label so headings stay grammatical in both locales.
function docTypeLabel(t: (k: string) => string, raw: string): string {
  if (raw === 'SOP') return t('knowledge.doc_type.SOP')
  // The Chinese keys are valid TS keys (knowledge.doc_type.人物 etc.)
  // but template-string indexing doesn't know that. The explicit
  // lookup table below is a safe alternative.
  switch (raw) {
    case '人物':
      return t('knowledge.doc_type.人物')
    case '世界观':
      return t('knowledge.doc_type.世界观')
    case '历史':
      return t('knowledge.doc_type.历史')
    case '其它':
      return t('knowledge.doc_type.其它')
    case '':
      return t('common.uncategorized')
    default:
      return t('common.unknown')
  }
}

export default function KnowledgePage() {
  const t = useT()
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
      setError(err instanceof Error ? err.message : t('knowledge.error.load'))
    } finally {
      setLoading(false)
    }
  }

  async function loadTemplates() {
    try {
      const res = await api.ipTemplates.list()
      setTemplates(res.templates)
    } catch (err: unknown) {
      setError(
        err instanceof Error ? err.message : t('knowledge.error.load_templates'),
      )
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
      setError(err instanceof Error ? err.message : t('knowledge.error.create'))
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
      setError(err instanceof Error ? err.message : t('knowledge.error.delete'))
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
      setError(err instanceof Error ? err.message : t('knowledge.error.create_ip'))
    } finally {
      setIPSubmitting(false)
    }
  }

  // The section key is the localized "Uncategorized" string so the
  // heading and the React key stay aligned across locale switches
  // (the previous implementation used a hardcoded Chinese "未分类"
  // for both, which broke the i18n parity and would render a
  // Chinese heading under an English locale).
  const uncategorizedKey = t('common.uncategorized')
  const groups = docs.reduce<Record<string, KnowledgeDoc[]>>((acc, doc) => {
    const key = doc.doc_type || uncategorizedKey
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
          text: t('knowledge.import_export.notice.invalid'),
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
          text: t('knowledge.import_export.notice.import_ok', {
            count: result.imported,
          }),
        })
      } else {
        const first = result.errors[0]
        setImportExportNotice({
          kind: 'err',
          text: t('knowledge.import_export.notice.partial', {
            ok: result.imported,
            err: errCount,
            index: first.index + 1,
            msg: first.message,
          }),
        })
      }
      if (type === 'knowledge') {
        await loadDocs()
      }
    } catch (err: unknown) {
      setImportExportNotice({
        kind: 'err',
        text: err instanceof Error ? err.message : t('knowledge.import_export.notice.failed'),
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
            📚 {t('knowledge.title')}
          </h1>
          <p className="text-claude-muted text-sm mt-1">
            {t('knowledge.subhead')}
          </p>
        </div>
        {tab === 'docs' ? (
          <button
            onClick={() => setShowForm((v) => !v)}
            className="px-3 md:px-4 py-1.5 md:py-2 text-xs md:text-sm bg-claude-coral text-claude-on-primary rounded-md hover:bg-claude-coral-active transition-colors"
          >
            {showForm ? t('common.cancel') : t('knowledge.create_doc')}
          </button>
        ) : (
          <button
            onClick={() => setShowIPForm((v) => !v)}
            className="px-3 md:px-4 py-1.5 md:py-2 text-xs md:text-sm bg-claude-success text-claude-on-primary rounded-md hover:opacity-90 transition-opacity"
          >
            {showIPForm ? t('common.cancel') : t('knowledge.create_ip')}
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
          {t('knowledge.tab.docs')}
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
          {t('knowledge.tab.ip_templates')}
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
  const t = useT()
  return (
    <>
      {showForm && (
        <form
          onSubmit={onSubmit}
          className="p-3 md:p-4 bg-claude-surface-card rounded-lg border border-claude-hairline space-y-3"
        >
          <div>
            <label className="block text-xs md:text-sm font-medium text-claude-ink">
              {t('knowledge.form.title')}
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
              {t('knowledge.form.path')}
            </label>
            <input
              type="text"
              required
              data-testid="input-test-path"
              placeholder={t('knowledge.form.path_placeholder')}
              value={form.path}
              onChange={(e) => setForm({ ...form, path: e.target.value })}
              className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
            />
          </div>
          <div>
            <label className="block text-xs md:text-sm font-medium text-claude-ink">
              {t('knowledge.form.doc_type')}
            </label>
            <select
              data-testid="input-test-doc_type"
              value={form.doc_type}
              onChange={(e) => setForm({ ...form, doc_type: e.target.value })}
              className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink"
            >
              {DOC_TYPES.map((dt) => (
                <option key={dt} value={dt}>
                  {docTypeLabel(t, dt)}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-xs md:text-sm font-medium text-claude-ink">
              {t('knowledge.form.content')}
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
            {submitting
              ? t('knowledge.form.submit_loading')
              : t('knowledge.form.submit_idle')}
          </button>
        </form>
      )}

      {loading ? (
        <div className="text-sm text-claude-body">{t('common.loading')}</div>
      ) : sortedKeys.length === 0 ? (
        <div className="p-4 bg-claude-surface-card rounded-lg border border-claude-hairline text-sm text-claude-muted text-center">
          {t('knowledge.empty.docs')}
        </div>
      ) : (
        <div className="space-y-4">
          {sortedKeys.map((key) => (
            <section key={key} className="space-y-2">
              <h2 className="font-serif text-lg text-claude-ink">
                {docTypeLabel(t, key)}
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
                          aria-label={t('knowledge.aria.delete')}
                          className="shrink-0 px-2 py-1 text-xs bg-claude-canvas hover:bg-claude-surface-soft text-claude-error rounded border border-claude-hairline"
                        >
                          {t('common.delete')}
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
  const t = useT()
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
                {t('knowledge.form.ip_type')}
              </label>
              <input
                type="text"
                required
                placeholder={t('knowledge.form.ip_type_placeholder')}
                value={form.type}
                onChange={(e) => setForm({ ...form, type: e.target.value })}
                className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
              />
            </div>
            <div>
              <label className="block text-xs md:text-sm font-medium text-claude-ink">
                {t('knowledge.form.ip_name')}
              </label>
              <input
                type="text"
                required
                placeholder={t('knowledge.form.ip_name_placeholder')}
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
              />
            </div>
          </div>
          <div>
            <label className="block text-xs md:text-sm font-medium text-claude-ink">
              {t('knowledge.form.ip_description')}
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
            {t('knowledge.form.ip_hint')}
          </div>
          <button
            type="submit"
            disabled={submitting}
            className="w-full sm:w-auto px-4 py-2 text-sm bg-claude-success text-claude-on-primary rounded-md hover:opacity-90 disabled:opacity-50 transition-opacity"
          >
            {submitting
              ? t('knowledge.form.ip_submit_loading')
              : t('knowledge.form.ip_submit_idle')}
          </button>
        </form>
      )}

      {selectedIP ? (
        <IpDetail ipType={selectedIP} docs={ipDocs} onBack={() => onSelect(null)} />
      ) : templates.length === 0 ? (
        <div className="p-4 bg-claude-surface-card rounded-lg border border-claude-hairline text-sm text-claude-muted text-center">
          {t('knowledge.empty.ip_templates')}
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
                {tpl.doc_count} {t('knowledge.ip.doc_count')}
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
  const t = useT()
  return (
    <div className="space-y-3">
      <div className="flex items-center gap-2 md:gap-3 flex-wrap">
        <button
          onClick={onBack}
          className="px-3 py-1 text-xs md:text-sm bg-claude-surface-soft rounded hover:bg-claude-surface-card text-claude-body border border-claude-hairline"
        >
          {t('knowledge.ip.back')}
        </button>
        <h2 className="text-lg md:text-xl font-semibold break-all text-claude-ink">
          {ipType}
        </h2>
        <span className="text-xs md:text-sm text-claude-muted">
          ({docs.length} {t('knowledge.ip.doc_count')})
        </span>
      </div>
      {docs.length === 0 ? (
        <div className="p-4 bg-claude-surface-card rounded-lg border border-claude-hairline text-sm text-claude-muted text-center">
          {t('knowledge.empty.ip_detail')}
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

const EXPORT_TYPES: { value: ExportType; labelKey: string }[] = [
  { value: 'all', labelKey: 'knowledge.import_export.types.all' },
  { value: 'topic', labelKey: 'knowledge.import_export.types.topic' },
  { value: 'script', labelKey: 'knowledge.import_export.types.script' },
  { value: 'content_item', labelKey: 'knowledge.import_export.types.content_item' },
  { value: 'knowledge', labelKey: 'knowledge.import_export.types.knowledge' },
]

const IMPORT_TYPES: { value: ImportType; labelKey: string }[] = [
  { value: 'topic', labelKey: 'knowledge.import_export.types.topic' },
  { value: 'script', labelKey: 'knowledge.import_export.types.script' },
  { value: 'content_item', labelKey: 'knowledge.import_export.types.content_item' },
  { value: 'knowledge', labelKey: 'knowledge.import_export.types.knowledge' },
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
  const t = useT()
  return (
    <details className="p-3 md:p-4 bg-claude-surface-soft border border-claude-hairline rounded-lg">
      <summary className="cursor-pointer text-xs md:text-sm font-medium text-claude-ink select-none">
        {t('knowledge.import_export.title')}
      </summary>
      <div className="mt-3 grid grid-cols-1 md:grid-cols-2 gap-3 md:gap-4">
        <section>
          <h3 className="text-xs md:text-sm font-semibold text-claude-ink mb-2">
            {t('knowledge.import_export.export')}
          </h3>
          <div className="flex flex-wrap gap-2">
            {EXPORT_TYPES.map((entry) => (
              <button
                key={entry.value}
                onClick={() => onExport(entry.value)}
                className="px-2.5 md:px-3 py-1 md:py-1.5 text-xs md:text-sm bg-claude-canvas border border-claude-hairline rounded hover:bg-claude-surface-card text-claude-body"
              >
                {t('knowledge.import_export.export_label')} {t(entry.labelKey)}
              </button>
            ))}
          </div>
        </section>
        <section>
          <h3 className="text-xs md:text-sm font-semibold text-claude-ink mb-2">
            {t('knowledge.import_export.import')}
          </h3>
          <div className="flex flex-wrap gap-2">
            {IMPORT_TYPES.map((entry) => (
              <button
                key={entry.value}
                onClick={() => onImport(entry.value)}
                disabled={importing !== null}
                className="px-2.5 md:px-3 py-1 md:py-1.5 text-xs md:text-sm bg-claude-canvas border border-claude-hairline rounded hover:bg-bg-claude-surface-card text-claude-body disabled:opacity-50"
              >
                {importing === entry.value
                  ? t('knowledge.import_export.import_loading')
                  : `${t('knowledge.import_export.import_label')} ${t(entry.labelKey)}`}
              </button>
            ))}
          </div>
          <p className="mt-2 text-xs text-claude-muted">
            {t('knowledge.import_export.hint')}
          </p>
        </section>
      </div>
    </details>
  )
}
