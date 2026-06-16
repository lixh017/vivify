'use client'

// RotateDialog is the rotated-key entry point. It collects a
// single new plaintext key and forwards it to the parent's
// onSubmit. The dialog is narrow because the only editable
// surface is the textarea. We still pre-fill the title with
// the credential's name so an operator rotating several keys
// in a row can confirm they're hitting the right row.

import { FormEvent, useEffect, useState } from 'react'
import { ApiError } from '@opc/shared/api'
import type { Credential } from '@opc/shared/types'

// truncate keeps long base_url values from blowing up the
// dialog header row. Matches the helper used in
// app/(console)/credentials/page.tsx — the full value stays
// reachable via the span's title attribute (browser tooltip
// on hover). The ellipsis suffix is unicode so we avoid a
// sentinel byte that a real URL could legally contain.
function truncate(value: string, max: number): string {
  if (value.length <= max) return value
  return value.slice(0, max - 1) + '…'
}

export function RotateDialog({
  credential,
  open,
  onClose,
  onSubmit,
}: {
  credential: Credential | null
  open: boolean
  onClose: () => void
  onSubmit: (id: number, plaintextKey: string) => Promise<void>
}) {
  const [plaintextKey, setPlaintextKey] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!open) return
    setPlaintextKey('')
    setError(null)
    setSubmitting(false)
  }, [open, credential?.id])

  useEffect(() => {
    if (!open) return
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, onClose])

  if (!open || !credential) return null

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    if (!credential) return
    setSubmitting(true)
    setError(null)
    try {
      await onSubmit(credential.id, plaintextKey.trim())
      onClose()
    } catch (err: unknown) {
      setError(
        err instanceof ApiError
          ? `${err.status} ${err.body || err.message}`
          : err instanceof Error
          ? err.message
          : '轮换失败',
      )
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div
      className="fixed inset-0 bg-black/40 z-50 flex items-center justify-center p-4"
      onClick={onClose}
      data-testid="rotate-credential-backdrop"
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="rotate-credential-title"
        onClick={(e) => e.stopPropagation()}
        className="bg-claude-surface-card rounded-lg shadow-xl max-w-md w-full border border-claude-hairline"
      >
        <div className="border-b border-claude-hairline px-6 py-4 flex items-center justify-between">
          <div>
            <h2
              id="rotate-credential-title"
              className="text-base font-medium text-claude-ink"
            >
              轮换密钥
            </h2>
            <p className="text-xs text-claude-muted mt-0.5">
              {credential.name} · {credential.provider}
            </p>
            {/* Surface the protocol/model/base_url as read-only
                context so the operator can confirm they're rotating
                the right row. Rotate is intentionally narrow —
                the only editable field is the new key. */}
            {credential.protocol && (
              <div className="mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[11px] text-claude-muted-soft">
                <span className="font-mono">{credential.protocol}</span>
                {credential.model_name && (
                  <>
                    <span className="text-claude-hairline">·</span>
                    <span className="font-mono">{credential.model_name}</span>
                  </>
                )}
                {credential.base_url && (
                  <>
                    <span className="text-claude-hairline">·</span>
                    <span
                      className="font-mono truncate max-w-[16rem]"
                      title={credential.base_url}
                    >
                      {truncate(credential.base_url, 32)}
                    </span>
                  </>
                )}
              </div>
            )}
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="关闭"
            className="text-claude-muted hover:text-claude-ink rounded-sm p-1"
          >
            ✕
          </button>
        </div>

        <form onSubmit={handleSubmit} className="px-6 py-5 space-y-4">
          <div>
            <label className="block text-sm font-medium text-claude-ink">
              新 API Key
            </label>
            <textarea
              required
              autoFocus
              value={plaintextKey}
              onChange={(e) => setPlaintextKey(e.target.value)}
              placeholder="粘贴新生成的 key"
              rows={4}
              className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink font-mono focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
            />
            <p className="mt-1 text-[11px] text-claude-muted-soft">
              旧 key 立即失效；无法回滚。
            </p>
          </div>

          {error && (
            <div className="text-sm text-claude-error bg-claude-canvas border border-claude-error/30 rounded-md px-3 py-2">
              {error}
            </div>
          )}

          <div className="flex items-center justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={onClose}
              className="px-3 py-1.5 text-sm text-claude-body hover:text-claude-ink"
            >
              取消
            </button>
            <button
              type="submit"
              disabled={submitting}
              className="px-4 py-1.5 text-sm bg-claude-coral text-claude-on-primary rounded-md hover:bg-claude-coral-active disabled:opacity-50 transition-colors"
            >
              {submitting ? '轮换中…' : '确认轮换'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
