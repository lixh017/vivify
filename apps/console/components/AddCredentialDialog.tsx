'use client'

// AddCredentialDialog collects the four fields needed to
// create a new Credential: provider, display name, the
// plaintext key (encrypted server-side; never echoed back),
// and an optional scope tag. The form mirrors the field set
// of CreateCredentialInput from @opc/shared/types.
//
// Visual style follows the web demo: fixed backdrop, sticky
// header, scroll-safe body, coral primary action. The dialog
// traps no focus (we let the native form flow handle it) and
// closes on backdrop click or the ✕ button.

import { FormEvent, useEffect, useRef, useState } from 'react'
import { ApiError } from '@opc/shared/api'
import type {
  CredentialProvider,
  CreateCredentialInput,
} from '@opc/shared/types'

const PROVIDERS: { value: CredentialProvider; label: string; hint: string }[] = [
  { value: 'volcengine', label: '火山引擎', hint: '内容平台 / 视频理解' },
  { value: 'anthropic', label: 'Anthropic', hint: 'Claude API key' },
  { value: 'douyin', label: '抖音开放平台', hint: '发布 / 数据接口' },
]

// Common scope tags surfaced as quick-pick chips. The user
// can also type a free-form scope; we just need a string the
// downstream caller can bucket by.
const SCOPE_PRESETS = ['default', 'publish', 'analysis'] as const

export function AddCredentialDialog({
  open,
  onClose,
  onSubmit,
}: {
  open: boolean
  onClose: () => void
  onSubmit: (input: CreateCredentialInput) => Promise<void>
}) {
  const [provider, setProvider] = useState<CredentialProvider>('volcengine')
  const [name, setName] = useState('')
  const [plaintextKey, setPlaintextKey] = useState('')
  const [scope, setScope] = useState<string>('default')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const dialogRef = useRef<HTMLDivElement | null>(null)

  // Reset the form every time the dialog re-opens so a
  // cancelled-then-reopened dialog never carries over a stale
  // half-typed key.
  useEffect(() => {
    if (!open) return
    setProvider('volcengine')
    setName('')
    setPlaintextKey('')
    setScope('default')
    setError(null)
    setSubmitting(false)
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

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setSubmitting(true)
    setError(null)
    try {
      await onSubmit({
        provider,
        name: name.trim(),
        plaintext_key: plaintextKey.trim(),
        scope: scope.trim() || 'default',
      })
      onClose()
    } catch (err: unknown) {
      setError(
        err instanceof ApiError
          ? `${err.status} ${err.body || err.message}`
          : err instanceof Error
          ? err.message
          : '保存失败',
      )
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div
      className="fixed inset-0 bg-black/40 z-50 flex items-center justify-center p-4"
      onClick={onClose}
      data-testid="add-credential-backdrop"
    >
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="add-credential-title"
        onClick={(e) => e.stopPropagation()}
        className="bg-claude-surface-card rounded-lg shadow-xl max-w-lg w-full max-h-[90vh] overflow-y-auto border border-claude-hairline"
      >
        <div className="sticky top-0 bg-claude-surface-card border-b border-claude-hairline px-6 py-4 flex items-center justify-between">
          <div>
            <h2
              id="add-credential-title"
              className="text-base font-medium text-claude-ink"
            >
              添加凭据
            </h2>
            <p className="text-xs text-claude-muted mt-0.5">
              密钥将以 AES-GCM 加密后存储。
            </p>
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
              Provider
            </label>
            <select
              value={provider}
              onChange={(e) =>
                setProvider(e.target.value as CredentialProvider)
              }
              className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink"
            >
              {PROVIDERS.map((p) => (
                <option key={p.value} value={p.value}>
                  {p.label} — {p.hint}
                </option>
              ))}
            </select>
          </div>

          <div>
            <label className="block text-sm font-medium text-claude-ink">
              名称
            </label>
            <input
              type="text"
              required
              maxLength={64}
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="例如：抖音主号 / 火山生产"
              className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-claude-ink">
              API Key
            </label>
            <textarea
              required
              value={plaintextKey}
              onChange={(e) => setPlaintextKey(e.target.value)}
              placeholder="粘贴上游服务的 key"
              rows={3}
              className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink font-mono focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
            />
            <p className="mt-1 text-[11px] text-claude-muted-soft">
              仅用于加密保存，不会回显到页面。
            </p>
          </div>

          <div>
            <label className="block text-sm font-medium text-claude-ink">
              Scope
            </label>
            <div className="mt-1 flex flex-wrap gap-2">
              {SCOPE_PRESETS.map((s) => (
                <button
                  key={s}
                  type="button"
                  onClick={() => setScope(s)}
                  className={
                    'px-2.5 py-1 text-xs rounded-pill border transition-colors ' +
                    (scope === s
                      ? 'bg-claude-coral text-claude-on-primary border-claude-coral'
                      : 'bg-claude-canvas text-claude-body border-claude-hairline hover:border-claude-coral')
                  }
                >
                  {s}
                </button>
              ))}
              <input
                type="text"
                value={scope}
                onChange={(e) => setScope(e.target.value)}
                placeholder="自定义"
                className="px-2.5 py-1 text-xs border border-claude-hairline rounded-pill bg-claude-canvas text-claude-ink"
              />
            </div>
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
              {submitting ? '保存中…' : '保存'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
