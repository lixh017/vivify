'use client'

import { useEffect, useState, FormEvent } from 'react'
import { api, type PublishChecklist, type PublishCheckStatus } from '@opc/shared/api'
import type { ContentItem, UpdateContentItemPatch } from '@opc/shared/types'
import { useT } from '@opc/shared/i18n-client'

// Canonical platform vocabulary mirroring the backend's allowedPlatforms
// set. Wire values; the surrounding chrome is rendered via i18n.
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
// to pending; a populated string holds the wire-format ISO timestamp.
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

// statusIcon maps a PublishCheckStatus to a leading emoji glyph.
// Centralising this keeps the modal's row rendering tidy and gives
// us one place to swap for SVG icons later if the design grows up.
function statusIcon(status: PublishCheckStatus): string {
  if (status === 'pass') return '✅'
  if (status === 'warn') return '⚠️'
  return '❌'
}

function statusClass(status: PublishCheckStatus): string {
  if (status === 'pass') {
    return 'bg-claude-success/10 text-claude-success border-claude-success/30'
  }
  if (status === 'warn') {
    return 'bg-claude-accent-amber/10 text-claude-accent-amber border-claude-accent-amber/30'
  }
  return 'bg-claude-error/10 text-claude-error border-claude-error/30'
}

export default function CalendarPage() {
  const t = useT()
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

  // Per-item publish-checklist cache. We persist the latest result
  // so the green "ready" badge sticks on the card after the modal
  // closes — that mirrors how a content planner thinks about their
  // queue ("which items are publish-ready?").
  const [checklistCache, setChecklistCache] = useState<
    Record<number, PublishChecklist>
  >({})
  const [checklistLoadingIds, setChecklistLoadingIds] = useState<Set<number>>(
    new Set(),
  )
  const [checklistModalId, setChecklistModalId] = useState<number | null>(null)
  const [checklistError, setChecklistError] = useState<string | null>(null)

  async function loadItems() {
    setLoading(true)
    setError(null)
    try {
      const res = await api.contentItems.list()
      setItems(res.items)
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t('calendar.error.load'))
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
      setError(err instanceof Error ? err.message : t('calendar.error.create'))
    } finally {
      setSubmitting(false)
    }
  }

  // deleteItem removes a content item via the API. We optimistically
  // drop it from the list and roll back on failure.
  async function deleteItem(id: number) {
    const previous = items
    setItems((prev) => prev.filter((it) => it.id !== id))
    try {
      await api.contentItems.delete(id)
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t('calendar.error.delete'))
      setItems(previous)
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
  // auto-save — the user still has to hit "Save" — so accidental
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
  // scheduled_at; an empty string for an absent
  // datetime-local input is treated as "clear".
  async function saveEdit(item: ContentItem) {
    if (savingIds.has(item.id)) return
    setSavingIds((prev) => {
      const next = new Set(prev)
      next.add(item.id)
      return next
    })
    setEditError(null)

    // The wire patch uses `null` to clear a nullable timestamp;
    // UpdateContentItemPatch models that explicitly so we don't
    // need a `Partial<ContentItem>` cast at the boundary.
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
      setEditError(err instanceof Error ? err.message : t('calendar.error.update'))
      setItems(previous)
    } finally {
      setSavingIds((prev) => {
        const next = new Set(prev)
        next.delete(item.id)
        return next
      })
    }
  }

  // runChecklist hits /ai/publish-checklist for a single content
  // item, stores the result in cache, and opens the modal. We open
  // the modal even on cache hit so re-clicking the button feels
  // immediate — the user wants to see the breakdown again, not
  // wait for another round trip.
  async function runChecklist(item: ContentItem) {
    if (checklistLoadingIds.has(item.id)) return
    setChecklistLoadingIds((prev) => {
      const next = new Set(prev)
      next.add(item.id)
      return next
    })
    setChecklistError(null)
    try {
      const data = await api.ai.publishChecklist({ content_item_id: item.id })
      setChecklistCache((prev) => ({ ...prev, [item.id]: data }))
      setChecklistModalId(item.id)
    } catch (err: unknown) {
      setChecklistError(
        err instanceof Error
          ? err.message
          : t('calendar.checklist.error_failed'),
      )
    } finally {
      setChecklistLoadingIds((prev) => {
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

  const modalItem =
    checklistModalId !== null
      ? items.find((it) => it.id === checklistModalId) || null
      : null
  const modalData =
    checklistModalId !== null ? checklistCache[checklistModalId] : undefined

  return (
    <div className="space-y-4">
      <div className="flex items-end justify-between flex-wrap gap-3">
        <div>
          <h1 className="font-serif text-3xl text-claude-ink tracking-tight">
            📅 {t('calendar.title')}
          </h1>
          <p className="text-claude-muted text-sm mt-1">
            {t('calendar.subhead')}
          </p>
        </div>
        <button
          onClick={() => setShowForm((v) => !v)}
          className="px-3 md:px-4 py-1.5 md:py-2 text-xs md:text-sm bg-claude-coral text-claude-on-primary rounded-md hover:bg-claude-coral-active transition-colors"
        >
          {showForm ? t('calendar.cancel') : t('calendar.create')}
        </button>
      </div>

      {showForm && (
        <form
          onSubmit={handleSubmit}
          className="p-3 md:p-4 bg-claude-surface-card rounded-lg border border-claude-hairline space-y-3"
        >
          <div>
            <label
              htmlFor="new-content-platform"
              className="block text-xs md:text-sm font-medium text-claude-ink"
            >
              {t('calendar.field.platform')}
            </label>
            <select
              id="new-content-platform"
              value={form.platform}
              onChange={(e) => setForm({ ...form, platform: e.target.value })}
              className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
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
              className="flex items-center gap-2 text-xs md:text-sm text-claude-ink"
            >
              <input
                id="new-content-pending"
                type="checkbox"
                checked={form.is_pending}
                onChange={(e) =>
                  setForm({ ...form, is_pending: e.target.checked })
                }
              />
              {t('calendar.pending_checkbox')}
            </label>
          </div>
          <div>
            <label
              htmlFor="new-content-scheduled-at"
              className="block text-xs md:text-sm font-medium text-claude-ink"
            >
              {t('calendar.field.scheduled')}
            </label>
            <input
              id="new-content-scheduled-at"
              type="datetime-local"
              disabled={form.is_pending}
              value={form.scheduled_at}
              onChange={(e) =>
                setForm({ ...form, scheduled_at: e.target.value })
              }
              className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink disabled:bg-claude-surface-soft focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
            />
          </div>
          <button
            type="submit"
            disabled={submitting || (!form.is_pending && !form.scheduled_at)}
            className="w-full sm:w-auto px-4 py-2 text-sm bg-claude-coral text-claude-on-primary rounded-md hover:bg-claude-coral-active disabled:opacity-50 transition-colors"
          >
            {submitting
              ? t('calendar.submit_loading')
              : t('calendar.submit_idle')}
          </button>
        </form>
      )}

      {error && (
        <div className="p-3 bg-claude-error/10 border border-claude-error text-claude-error rounded text-sm">
          {error}
        </div>
      )}

      {editError && (
        <div className="p-3 bg-claude-error/10 border border-claude-error text-claude-error rounded text-sm">
          {editError}
        </div>
      )}

      {checklistError && (
        <div className="p-3 bg-claude-error/10 border border-claude-error text-claude-error rounded text-sm">
          {checklistError}
        </div>
      )}

      {loading ? (
        <div className="text-sm text-claude-body">{t('common.loading')}</div>
      ) : items.length === 0 ? (
        <div className="p-4 bg-claude-surface-card rounded-lg border border-claude-hairline text-sm text-claude-muted text-center">
          {t('calendar.empty')}
        </div>
      ) : (
        <div className="space-y-4">
          {sortedKeys.map((key) => (
            <section key={key} className="space-y-2">
              <h2 className="font-serif text-lg text-claude-ink">
                {key === PENDING_KEY ? t('calendar.pending_label') : key}
                <span className="ml-2 text-xs md:text-sm text-claude-muted-soft">
                  ({groups[key].length})
                </span>
              </h2>
              <div className="space-y-2">
                {groups[key].map((item) => {
                  const isEditing = editingId === item.id
                  const isSaving = savingIds.has(item.id)
                  const cached = checklistCache[item.id]
                  const isChecking = checklistLoadingIds.has(item.id)
                  return (
                    <div
                      key={item.id}
                      data-testid="list-item"
                      data-content-item-id={item.id}
                      data-content-item-test={`content-item-${item.id}`}
                      className="p-3 md:p-4 bg-claude-surface-card rounded-lg border border-claude-hairline hover:border-claude-coral/50 transition-colors"
                    >
                      <div className="flex items-start justify-between gap-3 flex-col sm:flex-row">
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2 flex-wrap">
                            <p className="text-xs md:text-sm text-claude-body">
                              {t('calendar.field.script')}
                              {item.script_id}
                            </p>
                            {cached && cached.ready && (
                              <span
                                data-testid={`ready-badge-${item.id}`}
                                className="inline-flex items-center gap-1 px-2 py-0.5 text-xs font-medium rounded-full bg-claude-success/15 text-claude-success border border-claude-success/30"
                              >
                                {t('calendar.checklist.ready_badge')}
                              </span>
                            )}
                            {cached && !cached.ready && (
                              <span
                                data-testid={`not-ready-badge-${item.id}`}
                                className="inline-flex items-center gap-1 px-2 py-0.5 text-xs font-medium rounded-full bg-claude-error/10 text-claude-error border border-claude-error/30"
                              >
                                {t('calendar.checklist.not_ready_badge')}
                              </span>
                            )}
                          </div>
                          {item.scheduled_at && (
                            <p className="text-xs text-claude-muted-soft mt-1 break-all">
                              {item.scheduled_at}
                            </p>
                          )}
                          {item.published_at && (
                            <p className="text-xs text-claude-success mt-1 break-all">
                              {t('calendar.field.published_at')}{' '}
                              {item.published_at}
                            </p>
                          )}
                          {item.platform_url && (
                            <a
                              href={item.platform_url}
                              target="_blank"
                              rel="noreferrer"
                              className="text-xs text-claude-coral hover:text-claude-coral-active hover:underline block mt-1 break-all"
                            >
                              {item.platform_url}
                            </a>
                          )}
                        </div>
                        <div className="flex sm:flex-col items-start sm:items-end gap-2">
                          <span className="px-2 py-1 bg-claude-canvas text-claude-coral rounded text-xs">
                            {item.platform}
                          </span>
                          {!isEditing && (
                            <button
                              type="button"
                              data-testid={`btn-checklist-${item.id}`}
                              onClick={() => runChecklist(item)}
                              disabled={isChecking}
                              className="px-2 py-1 text-xs bg-claude-accent-teal/15 hover:bg-claude-accent-teal/25 text-claude-accent-teal rounded border border-claude-accent-teal/30 disabled:opacity-50"
                            >
                              {isChecking
                                ? t('calendar.checklist.loading')
                                : t('calendar.checklist.button')}
                            </button>
                          )}
                          {!isEditing && (
                            <button
                              type="button"
                              onClick={() => startEdit(item)}
                              aria-label={t('calendar.aria.edit')}
                              className="px-2 py-1 text-xs bg-claude-canvas hover:bg-claude-surface-soft text-claude-body rounded border border-claude-hairline"
                            >
                              {t('common.edit')}
                            </button>
                          )}
                          {!isEditing && (
                            <button
                              type="button"
                              data-testid={`btn-delete-${item.id}`}
                              onClick={() => deleteItem(item.id)}
                              aria-label={t('calendar.aria.delete')}
                              className="px-2 py-1 text-xs bg-claude-canvas hover:bg-claude-surface-soft text-claude-error rounded border border-claude-hairline"
                            >
                              {t('common.delete')}
                            </button>
                          )}
                        </div>
                      </div>

                      {isEditing && (
                        <div className="mt-3 pt-3 border-t border-claude-hairline-soft space-y-3">
                          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                            <div>
                              <label
                                htmlFor={`edit-scheduled-at-${item.id}`}
                                className="block text-xs font-medium text-claude-body"
                              >
                                {t('calendar.field.scheduled')}
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
                                className="mt-1 w-full px-2 py-1 text-xs md:text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
                              />
                            </div>
                            <div>
                              <label
                                htmlFor={`edit-published-at-${item.id}`}
                                className="block text-xs font-medium text-claude-body"
                              >
                                {t('calendar.field.published')}
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
                                className="mt-1 w-full px-2 py-1 text-xs md:text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
                              />
                            </div>
                            <div>
                              <label
                                htmlFor={`edit-platform-${item.id}`}
                                className="block text-xs font-medium text-claude-body"
                              >
                                {t('calendar.field.platform')}
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
                                className="mt-1 w-full px-2 py-1 text-xs md:text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink"
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
                              className="px-2 py-1 text-xs bg-claude-surface-soft text-claude-ink rounded border border-claude-hairline hover:bg-claude-surface-card"
                            >
                              {t('calendar.quick.today')}
                            </button>
                            <button
                              type="button"
                              onClick={() => setQuickDate('nextMonday')}
                              className="px-2 py-1 text-xs bg-claude-surface-soft text-claude-ink rounded border border-claude-hairline hover:bg-claude-surface-card"
                            >
                              {t('calendar.quick.next_monday')}
                            </button>
                            <button
                              type="button"
                              onClick={() => setQuickDate('markPublished')}
                              className="px-2 py-1 text-xs bg-claude-success/10 text-claude-success rounded border border-claude-hairline hover:bg-claude-success/20"
                            >
                              {t('calendar.quick.mark_published')}
                            </button>
                            <button
                              type="button"
                              onClick={() => setQuickDate('clear')}
                              className="px-2 py-1 text-xs bg-claude-canvas text-claude-body rounded border border-claude-hairline hover:bg-claude-surface-soft"
                            >
                              {t('calendar.quick.clear')}
                            </button>
                          </div>
                          <div className="flex gap-2 justify-end flex-wrap">
                            <button
                              type="button"
                              onClick={cancelEdit}
                              disabled={isSaving}
                              className="px-3 py-1 text-sm bg-claude-canvas text-claude-body rounded border border-claude-hairline hover:bg-claude-surface-soft disabled:opacity-50"
                            >
                              {t('calendar.cancel')}
                            </button>
                            <button
                              type="button"
                              onClick={() => saveEdit(item)}
                              disabled={isSaving}
                              className="px-3 py-1 text-sm bg-claude-coral text-claude-on-primary rounded hover:bg-claude-coral-active disabled:opacity-50 transition-colors"
                            >
                              {isSaving
                                ? t('calendar.save_loading')
                                : t('calendar.save_idle')}
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

      {modalItem && modalData && (
        <ChecklistModal
          item={modalItem}
          data={modalData}
          onClose={() => setChecklistModalId(null)}
        />
      )}
    </div>
  )
}

interface ChecklistModalProps {
  item: ContentItem
  data: PublishChecklist
  onClose: () => void
}

// ChecklistModal renders the pass/warn/fail breakdown plus the
// overall score. Click-outside on the backdrop closes; the Escape
// key is intentionally NOT bound here because the close button
// already covers the keyboard case and we want to avoid swallowing
// global key events.
function ChecklistModal({ item, data, onClose }: ChecklistModalProps) {
  const t = useT()
  const overallBand =
    data.overall_score >= 80
      ? 'bg-claude-success/15 text-claude-success border-claude-success/30'
      : data.overall_score >= 60
        ? 'bg-claude-accent-amber/15 text-claude-accent-amber border-claude-accent-amber/30'
        : 'bg-claude-error/10 text-claude-error border-claude-error/30'
  return (
    <div
      data-testid="checklist-modal-backdrop"
      className="fixed inset-0 bg-claude-ink/40 flex items-center justify-center p-4 z-50"
      onClick={onClose}
    >
      <div
        data-testid={`checklist-modal-${item.id}`}
        className="bg-claude-surface-card border border-claude-hairline rounded-lg shadow-claude-soft max-w-lg w-full max-h-[90vh] overflow-y-auto"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
      >
        <div className="p-4 border-b border-claude-hairline-soft flex items-center justify-between gap-3 flex-wrap">
          <h2 className="font-serif text-lg text-claude-ink">
            {t('calendar.checklist.title')}
          </h2>
          <div className="flex items-center gap-2">
            {data.ready ? (
              <span className="inline-flex items-center gap-1 px-2 py-0.5 text-xs font-medium rounded-full bg-claude-success/15 text-claude-success border border-claude-success/30">
                {t('calendar.checklist.ready_badge')}
              </span>
            ) : (
              <span className="inline-flex items-center gap-1 px-2 py-0.5 text-xs font-medium rounded-full bg-claude-error/10 text-claude-error border border-claude-error/30">
                {t('calendar.checklist.not_ready_badge')}
              </span>
            )}
            <button
              type="button"
              onClick={onClose}
              className="text-xs text-claude-muted hover:text-claude-ink px-2 py-1 rounded hover:bg-claude-surface-soft"
            >
              {t('calendar.checklist.close')}
            </button>
          </div>
        </div>

        <div className="p-4 space-y-3">
          <div
            className={`p-3 rounded border ${overallBand} flex items-center justify-between`}
          >
            <span className="text-sm font-medium">
              {t('calendar.checklist.overall_score')}
            </span>
            <span className="text-2xl font-semibold">{data.overall_score}</span>
          </div>
          {data.checks.length === 0 ? (
            <div className="text-sm text-claude-muted italic">
              {t('calendar.checklist.empty')}
            </div>
          ) : (
            <ul className="space-y-2">
              {data.checks.map((check, i) => (
                <li
                  key={`${check.name}-${i}`}
                  className={`p-2.5 rounded border ${statusClass(check.status)} flex gap-2 items-start`}
                >
                  <span aria-hidden className="text-base leading-none mt-0.5">
                    {statusIcon(check.status)}
                  </span>
                  <div className="flex-1 min-w-0">
                    <div className="text-xs md:text-sm font-medium text-claude-ink">
                      {check.name}
                    </div>
                    <div className="text-xs text-claude-body mt-0.5">
                      {check.message}
                    </div>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>
    </div>
  )
}
