'use client'

// ConfirmDeleteDialog is a generic second-chance confirmation
// used for credential deletion. It deliberately doesn't take
// a "danger" lock (no typing the name back) because the
// irreversible action is a single row scoped to the operator,
// but it does require a deliberate button click and renders
// the destructive button in the warning tone.

import { useEffect, useState } from 'react'

export function ConfirmDeleteDialog({
  open,
  title,
  description,
  confirmLabel = '删除',
  cancelLabel = '取消',
  onClose,
  onConfirm,
}: {
  open: boolean
  title: string
  description: string
  confirmLabel?: string
  cancelLabel?: string
  onClose: () => void
  onConfirm: () => Promise<void> | void
}) {
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!open) return
    setBusy(false)
  }, [open])

  useEffect(() => {
    if (!open) return
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, onClose])

  if (!open) return null

  async function handleConfirm() {
    setBusy(true)
    try {
      await onConfirm()
    } finally {
      setBusy(false)
    }
  }

  return (
    <div
      className="fixed inset-0 bg-black/40 z-50 flex items-center justify-center p-4"
      onClick={onClose}
      data-testid="confirm-delete-backdrop"
    >
      <div
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="confirm-delete-title"
        onClick={(e) => e.stopPropagation()}
        className="bg-claude-surface-card rounded-lg shadow-xl max-w-sm w-full border border-claude-hairline"
      >
        <div className="px-6 py-5 space-y-3">
          <h2
            id="confirm-delete-title"
            className="text-base font-medium text-claude-ink"
          >
            {title}
          </h2>
          <p className="text-sm text-claude-body">{description}</p>
          <div className="flex items-center justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={onClose}
              disabled={busy}
              className="px-3 py-1.5 text-sm text-claude-body hover:text-claude-ink"
            >
              {cancelLabel}
            </button>
            <button
              type="button"
              onClick={handleConfirm}
              disabled={busy}
              className="px-4 py-1.5 text-sm bg-claude-error text-claude-on-primary rounded-md hover:opacity-90 disabled:opacity-50 transition-opacity"
            >
              {busy ? '处理中…' : confirmLabel}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
