'use client'

import { useEffect, useState, FormEvent } from 'react'
import { api } from '@/lib/api'
import type { ContentItem, UpdateContentItemPatch } from '@/lib/types'

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

// EditForm holds the in-flight edit for a single content item. We use
// `null` for scheduled_at when the user wants to clear the date back
// to 待定; a populated string holds the wire-format ISO timestamp.
interface EditForm {
  scheduled_at: string
  published_at: string
  platform: string
}

const EMPTY_EDIT: EditForm = {
  scheduled_at: '',
  published_at: '',
  platform: PLATFORMS[0],
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

// toLocalInput converts an ISO timestamp (RFC3339) to the
// "YYYY-MM-DDTHH:MM" shape <input type="datetime-local"> expects. The
// value is rendered in the browser's local timezone, which matches how
// the user thinks about the date, and we convert back to UTC ISO when
// submitting. Accepts null (cleared) and undefined (absent) and treats
// both as "no value".
function toLocalInput(iso: string | null | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

// nextMondayAtNine returns a Date for the next Monday at 09:00 local
// time. If today is Monday, "next Monday" means the *following*
// Monday — the spec calls for the user to be able to defer work, not
// for "today, since today happens to be Monday" to be silently
// selected.
function nextMondayAtNine(): Date {
  const d = new Date()
  d.setHours(9, 0, 0, 0)
  const day = d.getDay() // 0=Sun, 1=Mon, ...
  const daysUntilMon = ((8 - day) % 7) || 7
  d.setDate(d.getDate() + daysUntilMon)
  return d
}

export default function CalendarPage() {
  const [items, setItems] = useState<ContentItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState<FormState>(EMPTY_FORM)
  const [submitting, setSubmitting] = useState(false)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [editForm, setEditForm] = useState<EditForm>(EMPTY_EDIT)
  const [savingIds, setSavingIds] = useState<Set<number>>(new Set())
  const [editError, setEditError] = useState<string | null>(null)

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

  // startEdit populates the inline edit form from the current item.
  // We prefill published_at as well so the user can adjust both at
  // once, which is the common "I just published it, mark it" flow.
  function startEdit(item: ContentItem) {
    setEditingId(item.id)
    setEditForm({
      scheduled_at: toLocalInput(item.scheduled_at),
      published_at: toLocalInput(item.published_at),
      platform: item.platform || PLATFORMS[0],
    })
    setEditError(null)
  }

  function cancelEdit() {
    setEditingId(null)
    setEditForm(EMPTY_EDIT)
    setEditError(null)
  }

  // setQuickDate applies a preset (today at the current wall-clock
  // time, or next Monday at 09:00) to the scheduled_at field. We don't
  // auto-save — the user still has to hit "保存" — so accidental
  // clicks never mutate backend state.
  function setQuickDate(kind: 'today' | 'nextMonday' | 'clear' | 'markPublished') {
    if (kind === 'clear') {
      setEditForm((prev) => ({ ...prev, scheduled_at: '' }))
      return
    }
    if (kind === 'markPublished') {
      const now = new Date()
      setEditForm((prev) => ({
        ...prev,
        published_at: toLocalInput(now.toISOString()),
      }))
      return
    }
    const target = kind === 'today' ? new Date() : nextMondayAtNine()
    // For "today" we keep the current minute so the user doesn't
    // see the time jump to 09:00 mid-day; for "next Monday" we pin
    // to 09:00 because that is the conventional planning slot.
    if (kind === 'nextMonday') {
      target.setHours(9, 0, 0, 0)
    }
    setEditForm((prev) => ({
      ...prev,
      scheduled_at: toLocalInput(target.toISOString()),
    }))
  }

  // saveEdit persists the inline form. Like moveTopic on the topics
  // page, we optimistically update the in-memory list and roll back
  // on failure. The patch object uses `null` to clear
  // scheduled_at (back to 待定); an empty string for an absent
  // datetime-local input is treated as "clear".
  async function saveEdit(item: ContentItem) {
    if (savingIds.has(item.id)) return
    setSavingIds((prev) => {
      const next = new Set(prev)
      next.add(item.id)
      return next
    })
    setEditError(null)

    // The wire patch uses `null` to clear a nullable timestamp back
    // to 待定; UpdateContentItemPatch models that explicitly so we
    // don't need a `Partial<ContentItem>` cast at the boundary. The Go
    // handler's Update reads `scheduled_at: nil` to mean "clear".
    const patch: UpdateContentItemPatch = {
      platform: editForm.platform,
    }
    if (editForm.scheduled_at) {
      patch.scheduled_at = new Date(editForm.scheduled_at).toISOString()
    } else {
      patch.scheduled_at = null
    }
    if (editForm.published_at) {
      patch.published_at = new Date(editForm.published_at).toISOString()
    } else {
      patch.published_at = null
    }

    const previous = items
    // Round-trip the wire shape directly so the optimistic state mirrors
    // what the server will return — no `null` -> `undefined` coercion.
    setItems((prev) =>
      prev.map((it) =>
        it.id === item.id
          ? {
              ...it,
              platform: editForm.platform,
              scheduled_at: patch.scheduled_at,
              published_at: patch.published_at,
            }
          : it,
      ),
    )

    try {
      const updated = await api.contentItems.update(item.id, patch)
      setItems((prev) => prev.map((it) => (it.id === item.id ? updated : it)))
      setEditingId(null)
      setEditForm(EMPTY_EDIT)
    } catch (err: unknown) {
      setEditError(err instanceof Error ? err.message : '更新失败')
      setItems(previous)
    } finally {
      setSavingIds((prev) => {
        const next = new Set(prev)
        next.delete(item.id)
        return next
      })
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
      <div className="flex items-center justify-between flex-wrap gap-2">
        <h1 className="text-2xl md:text-3xl font-bold">📅 日历</h1>
        <button
          onClick={() => setShowForm((v) => !v)}
          className="px-3 md:px-4 py-1.5 md:py-2 text-xs md:text-sm bg-blue-600 text-white rounded-lg shadow hover:bg-blue-700"
        >
          {showForm ? '取消' : '+ 新建排期'}
        </button>
      </div>

      {showForm && (
        <form
          onSubmit={handleSubmit}
          className="p-3 md:p-4 bg-white rounded-lg shadow space-y-3"
        >
          <div>
            <label
              htmlFor="new-content-platform"
              className="block text-xs md:text-sm font-medium text-gray-700"
            >
              平台
            </label>
            <select
              id="new-content-platform"
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
            <label
              htmlFor="new-content-pending"
              className="flex items-center gap-2 text-xs md:text-sm text-gray-700"
            >
              <input
                id="new-content-pending"
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
            <label
              htmlFor="new-content-scheduled-at"
              className="block text-xs md:text-sm font-medium text-gray-700"
            >
              排期时间
            </label>
            <input
              id="new-content-scheduled-at"
              type="datetime-local"
              disabled={form.is_pending}
              value={form.scheduled_at}
              onChange={(e) =>
                setForm({ ...form, scheduled_at: e.target.value })
              }
              className="mt-1 w-full px-3 py-2 text-sm border border-gray-300 rounded disabled:bg-gray-100"
            />
          </div>
          <button
            type="submit"
            disabled={submitting || (!form.is_pending && !form.scheduled_at)}
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

      {editError && (
        <div className="p-3 bg-red-50 border border-red-200 text-red-700 rounded text-sm">
          {editError}
        </div>
      )}

      {loading ? (
        <div className="text-sm text-gray-600">加载中…</div>
      ) : items.length === 0 ? (
        <div className="p-4 bg-white rounded-lg shadow text-sm text-gray-500 text-center">
          还没有数据
        </div>
      ) : (
        <div className="space-y-4">
          {sortedKeys.map((key) => (
            <section key={key} className="space-y-2">
              <h2 className="text-base md:text-lg font-semibold text-gray-700">
                {formatGroupLabel(key)}
                <span className="ml-2 text-xs md:text-sm text-gray-400">
                  ({groups[key].length})
                </span>
              </h2>
              <div className="space-y-2">
                {groups[key].map((item) => {
                  const isEditing = editingId === item.id
                  const isSaving = savingIds.has(item.id)
                  return (
                    <div
                      key={item.id}
                      data-testid={`content-item-${item.id}`}
                      className="p-3 md:p-4 bg-white rounded-lg shadow hover:shadow-md"
                    >
                      <div className="flex items-start justify-between gap-3 flex-col sm:flex-row">
                        <div className="flex-1 min-w-0">
                          <p className="text-xs md:text-sm text-gray-600">
                            Script #{item.script_id}
                          </p>
                          {item.scheduled_at && (
                            <p className="text-xs text-gray-400 mt-1 break-all">
                              {item.scheduled_at}
                            </p>
                          )}
                          {item.published_at && (
                            <p className="text-xs text-green-600 mt-1 break-all">
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
                        <div className="flex sm:flex-col items-start sm:items-end gap-2">
                          <span className="px-2 py-1 bg-blue-50 text-blue-700 rounded text-xs">
                            {item.platform}
                          </span>
                          {!isEditing && (
                            <button
                              type="button"
                              onClick={() => startEdit(item)}
                              className="px-2 py-1 text-xs bg-gray-100 hover:bg-gray-200 text-gray-700 rounded border border-gray-200"
                            >
                              编辑
                            </button>
                          )}
                        </div>
                      </div>

                      {isEditing && (
                        <div className="mt-3 pt-3 border-t border-gray-100 space-y-3">
                          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                            <div>
                              <label
                                htmlFor={`edit-scheduled-at-${item.id}`}
                                className="block text-xs font-medium text-gray-600"
                              >
                                排期时间
                              </label>
                              <input
                                id={`edit-scheduled-at-${item.id}`}
                                type="datetime-local"
                                value={editForm.scheduled_at}
                                onChange={(e) =>
                                  setEditForm({
                                    ...editForm,
                                    scheduled_at: e.target.value,
                                  })
                                }
                                className="mt-1 w-full px-2 py-1 text-xs md:text-sm border border-gray-300 rounded"
                              />
                            </div>
                            <div>
                              <label
                                htmlFor={`edit-published-at-${item.id}`}
                                className="block text-xs font-medium text-gray-600"
                              >
                                发布时间
                              </label>
                              <input
                                id={`edit-published-at-${item.id}`}
                                type="datetime-local"
                                value={editForm.published_at}
                                onChange={(e) =>
                                  setEditForm({
                                    ...editForm,
                                    published_at: e.target.value,
                                  })
                                }
                                className="mt-1 w-full px-2 py-1 text-xs md:text-sm border border-gray-300 rounded"
                              />
                            </div>
                            <div>
                              <label
                                htmlFor={`edit-platform-${item.id}`}
                                className="block text-xs font-medium text-gray-600"
                              >
                                平台
                              </label>
                              <select
                                id={`edit-platform-${item.id}`}
                                value={editForm.platform}
                                onChange={(e) =>
                                  setEditForm({
                                    ...editForm,
                                    platform: e.target.value,
                                  })
                                }
                                className="mt-1 w-full px-2 py-1 text-xs md:text-sm border border-gray-300 rounded"
                              >
                                {PLATFORMS.map((p) => (
                                  <option key={p} value={p}>
                                    {p}
                                  </option>
                                ))}
                              </select>
                            </div>
                          </div>
                          <div className="flex flex-wrap gap-2">
                            <button
                              type="button"
                              onClick={() => setQuickDate('today')}
                              className="px-2 py-1 text-xs bg-amber-50 text-amber-800 rounded border border-amber-200 hover:bg-amber-100"
                            >
                              设为今天
                            </button>
                            <button
                              type="button"
                              onClick={() => setQuickDate('nextMonday')}
                              className="px-2 py-1 text-xs bg-amber-50 text-amber-800 rounded border border-amber-200 hover:bg-amber-100"
                            >
                              设为下周一 09:00
                            </button>
                            <button
                              type="button"
                              onClick={() => setQuickDate('markPublished')}
                              className="px-2 py-1 text-xs bg-green-50 text-green-800 rounded border border-green-200 hover:bg-green-100"
                            >
                              标记为已发布
                            </button>
                            <button
                              type="button"
                              onClick={() => setQuickDate('clear')}
                              className="px-2 py-1 text-xs bg-gray-50 text-gray-700 rounded border border-gray-200 hover:bg-gray-100"
                            >
                              清空排期
                            </button>
                          </div>
                          <div className="flex gap-2 justify-end flex-wrap">
                            <button
                              type="button"
                              onClick={cancelEdit}
                              disabled={isSaving}
                              className="px-3 py-1 text-sm bg-white text-gray-700 rounded border border-gray-300 hover:bg-gray-50 disabled:opacity-50"
                            >
                              取消
                            </button>
                            <button
                              type="button"
                              onClick={() => saveEdit(item)}
                              disabled={isSaving}
                              className="px-3 py-1 text-sm bg-blue-600 text-white rounded shadow hover:bg-blue-700 disabled:opacity-50"
                            >
                              {isSaving ? '保存中...' : '保存'}
                            </button>
                          </div>
                        </div>
                      )}
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
