'use client'

// /credentials is the operator's view onto the encrypted
// API-key store. The page is a single screen with three
// states: empty (no rows yet — invite the operator to add
// one), loaded (table), and error (retry CTA). The action
// surface lives in three dialogs (add / rotate / confirm-
// delete) and a top-right primary action button.
//
// We do not paginate yet: the wire envelope is {items, total}
// but the handler returns everything. For an MCN with < 50
// providers this is fine; pagination lands with the rest of
// the table-heavy surfaces.

import { useState } from 'react'
import { TopBar } from '@/components/TopBar'
import { AddCredentialDialog } from '@/components/AddCredentialDialog'
import { RotateDialog } from '@/components/RotateDialog'
import { ConfirmDeleteDialog } from '@/components/ConfirmDeleteDialog'
import { useCredentials } from '@/lib/use-credentials'
import type { Credential, CredentialProvider } from '@opc/shared/types'

// PROVIDER_META is the per-provider visual treatment used by
// the table chip and (later) any filters. We map to the
// accent palette already in tailwind.config.ts so the chip
// stays consistent with the rest of the surface.
//
// "tone" picks a class string that combines a soft
// background and a foreground. We avoid hard-coded hex
// strings in JSX to keep the dark-mode switch (when it
// lands) a one-place change.
const PROVIDER_META: Record<
  CredentialProvider,
  { label: string; tone: string; dot: string }
> = {
  volcengine: {
    label: '火山引擎',
    tone: 'bg-claude-accent-teal/15 text-claude-accent-teal-active',
    dot: 'bg-claude-accent-teal',
  },
  anthropic: {
    label: 'Anthropic',
    tone: 'bg-claude-coral/15 text-claude-coral-active',
    dot: 'bg-claude-coral',
  },
  openai: {
    label: 'OpenAI',
    // Distinct from the coral Anthropic tone so the table stays
    // legible when both providers sit side-by-side. We reuse the
    // violet accent introduced for secondary callouts; it pairs
    // well with the warm coral without competing for attention.
    tone: 'bg-claude-accent-violet/15 text-claude-accent-violet-active',
    dot: 'bg-claude-accent-violet',
  },
  douyin: {
    label: '抖音',
    tone: 'bg-claude-accent-amber/20 text-claude-accent-amber-active',
    dot: 'bg-claude-accent-amber',
  },
}

function formatDate(iso: string | undefined): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (isNaN(d.getTime())) return '—'
  // Operator-facing compact date. We deliberately skip the
  // time component — credentials are reviewed on the scale
  // of days/weeks, not minutes.
  const yyyy = d.getFullYear()
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  return `${yyyy}-${mm}-${dd}`
}

// truncate keeps the base_url column readable when a tenant
// self-hosts on a long internal hostname. The full value
// stays reachable via the cell's title attribute (browser
// tooltip on hover). The ellipsis suffix is unicode so we
// avoid a sentinel byte that a real URL could legally
// contain.
function truncate(value: string, max: number): string {
  if (value.length <= max) return value
  return value.slice(0, max - 1) + '…'
}

function ProviderChip({ provider }: { provider: CredentialProvider }) {
  const meta = PROVIDER_META[provider]
  return (
    <span
      className={
        'inline-flex items-center gap-1.5 px-2 py-0.5 rounded-pill text-xs font-medium ' +
        meta.tone
      }
    >
      <span className={'w-1.5 h-1.5 rounded-full ' + meta.dot} />
      {meta.label}
    </span>
  )
}

function TableRow({
  credential,
  onRotate,
  onDelete,
}: {
  credential: Credential
  onRotate: (c: Credential) => void
  onDelete: (c: Credential) => void
}) {
  return (
    <tr className="border-t border-claude-hairline-soft hover:bg-claude-canvas/60">
      <td className="px-4 py-3 text-sm text-claude-ink font-medium">
        {credential.name}
      </td>
      <td className="px-4 py-3">
        <ProviderChip provider={credential.provider} />
      </td>
      <td className="px-4 py-3 text-sm text-claude-body">
        {credential.protocol ? (
          <span className="font-mono text-xs px-1.5 py-0.5 rounded bg-claude-surface-soft text-claude-body">
            {credential.protocol}
          </span>
        ) : (
          <span className="text-claude-muted-soft">—</span>
        )}
      </td>
      <td className="px-4 py-3 text-sm text-claude-body max-w-[14rem]">
        {credential.base_url ? (
          <span
            className="font-mono text-xs block truncate"
            title={credential.base_url}
          >
            {truncate(credential.base_url, 32)}
          </span>
        ) : (
          <span className="text-claude-muted-soft">—</span>
        )}
      </td>
      <td className="px-4 py-3 text-sm text-claude-body">
        {credential.model_name ? (
          <span className="font-mono text-xs px-1.5 py-0.5 rounded bg-claude-surface-soft text-claude-body">
            {credential.model_name}
          </span>
        ) : (
          <span className="text-claude-muted-soft">—</span>
        )}
      </td>
      <td className="px-4 py-3 text-sm text-claude-body">
        <span className="font-mono text-xs px-1.5 py-0.5 rounded bg-claude-surface-soft text-claude-body">
          {credential.scope}
        </span>
      </td>
      <td className="px-4 py-3 text-sm text-claude-muted whitespace-nowrap">
        {formatDate(credential.created_at)}
      </td>
      <td className="px-4 py-3 text-sm text-claude-muted whitespace-nowrap">
        {credential.last_used_at
          ? formatDate(credential.last_used_at)
          : <span className="text-claude-muted-soft">从未使用</span>}
      </td>
      <td className="px-4 py-3 text-right">
        <div className="inline-flex items-center gap-1">
          <button
            type="button"
            onClick={() => onRotate(credential)}
            className="px-2 py-1 text-xs text-claude-body hover:text-claude-ink hover:bg-claude-surface-soft rounded transition-colors"
          >
            轮换
          </button>
          <span className="text-claude-hairline">·</span>
          <button
            type="button"
            onClick={() => onDelete(credential)}
            className="px-2 py-1 text-xs text-claude-error hover:bg-claude-error/10 rounded transition-colors"
          >
            删除
          </button>
        </div>
      </td>
    </tr>
  )
}

function EmptyState({ onAdd }: { onAdd: () => void }) {
  return (
    <div className="bg-claude-canvas border border-claude-hairline border-dashed rounded-lg px-6 py-12 text-center">
      <div className="mx-auto w-10 h-10 rounded-full bg-claude-surface-card flex items-center justify-center text-claude-coral text-lg font-medium">
        🔑
      </div>
      <h3 className="mt-4 text-base font-medium text-claude-ink">
        还没有接入任何上游 API
      </h3>
      <p className="mt-1 text-sm text-claude-muted max-w-md mx-auto">
        添加火山引擎 / Anthropic / 抖音开放平台的 key 后，pipeline 才能调用对应
        服务。key 加密保存，不会在页面回显。
      </p>
      <button
        type="button"
        onClick={onAdd}
        className="mt-5 inline-flex items-center px-4 py-2 text-sm bg-claude-coral text-claude-on-primary rounded-md hover:bg-claude-coral-active transition-colors"
      >
        添加第一个凭据
      </button>
    </div>
  )
}

function ErrorBanner({
  message,
  onRetry,
}: {
  message: string
  onRetry: () => void
}) {
  return (
    <div className="bg-claude-canvas border border-claude-error/30 rounded-lg px-4 py-3 flex items-start gap-3">
      <div className="text-claude-error mt-0.5">!</div>
      <div className="flex-1 min-w-0">
        <p className="text-sm text-claude-ink">加载凭据失败</p>
        <p className="text-xs text-claude-muted mt-0.5 break-all">{message}</p>
      </div>
      <button
        type="button"
        onClick={onRetry}
        className="shrink-0 px-3 py-1 text-xs text-claude-coral hover:bg-claude-coral/10 rounded transition-colors"
      >
        重试
      </button>
    </div>
  )
}

export default function CredentialsPage() {
  const { items, loading, error, refresh, add, rotate, remove } =
    useCredentials()
  const [addOpen, setAddOpen] = useState(false)
  const [rotateTarget, setRotateTarget] = useState<Credential | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Credential | null>(null)

  async function handleAdd(input: Parameters<typeof add>[0]) {
    await add(input)
  }

  async function handleRotate(id: number, key: string) {
    await rotate(id, { plaintext_key: key })
  }

  async function handleDelete() {
    if (!deleteTarget) return
    try {
      await remove(deleteTarget.id)
    } catch {
      // remove() already rolls back optimistic state; we
      // intentionally swallow here because the hook surfaces
      // a fresh error via the next refresh and the page
      // banner is the right place to display it. Leaving the
      // dialog open on failure lets the user retry.
    }
  }

  return (
    <>
      <TopBar title="凭据" />
      <div className="p-6 space-y-4">
        {/* Toolbar */}
        <div className="flex items-center justify-between">
          <p className="text-sm text-claude-muted">
            第三方 API key 加密存储，调用方按需解出。
          </p>
          <button
            type="button"
            onClick={() => setAddOpen(true)}
            className="px-4 py-1.5 text-sm bg-claude-coral text-claude-on-primary rounded-md hover:bg-claude-coral-active transition-colors"
          >
            + 添加凭据
          </button>
        </div>

        {/* States */}
        {error && !loading && (
          <ErrorBanner message={error} onRetry={() => void refresh()} />
        )}

        {loading ? (
          <div className="bg-white border border-claude-hairline rounded-lg shadow-claude-soft px-6 py-12 text-center text-sm text-claude-muted">
            正在加载凭据…
          </div>
        ) : items.length === 0 ? (
          <EmptyState onAdd={() => setAddOpen(true)} />
        ) : (
          <div className="bg-white border border-claude-hairline rounded-lg shadow-claude-soft overflow-hidden">
            <table className="w-full text-left">
              <thead className="bg-claude-surface-card text-xs uppercase tracking-eyebrow text-claude-muted">
                <tr>
                  <th className="px-4 py-2.5 font-medium">名称</th>
                  <th className="px-4 py-2.5 font-medium">Provider</th>
                  <th className="px-4 py-2.5 font-medium">Protocol</th>
                  <th className="px-4 py-2.5 font-medium">Base URL</th>
                  <th className="px-4 py-2.5 font-medium">Model</th>
                  <th className="px-4 py-2.5 font-medium">Scope</th>
                  <th className="px-4 py-2.5 font-medium">创建时间</th>
                  <th className="px-4 py-2.5 font-medium">最后使用</th>
                  <th className="px-4 py-2.5 font-medium text-right">
                    操作
                  </th>
                </tr>
              </thead>
              <tbody>
                {items.map((c) => (
                  <TableRow
                    key={c.id}
                    credential={c}
                    onRotate={setRotateTarget}
                    onDelete={setDeleteTarget}
                  />
                ))}
              </tbody>
            </table>
          </div>
        )}

        {/* Footer hint: only show when there are rows, so the
            empty state stays the page's single source of truth
            on first-run. */}
        {!loading && items.length > 0 && (
          <p className="text-xs text-claude-muted-soft">
            共 {items.length} 条 · key 已加密，scope 用于调用方路由。
          </p>
        )}
      </div>

      <AddCredentialDialog
        open={addOpen}
        onClose={() => setAddOpen(false)}
        onSubmit={handleAdd}
      />
      <RotateDialog
        credential={rotateTarget}
        open={rotateTarget !== null}
        onClose={() => setRotateTarget(null)}
        onSubmit={handleRotate}
      />
      <ConfirmDeleteDialog
        open={deleteTarget !== null}
        title="删除这条凭据？"
        description={
          deleteTarget
            ? `将永久删除 "${deleteTarget.name}"（${deleteTarget.provider}）。后续使用此凭据的调用会立即失败。`
            : ''
        }
        confirmLabel="确认删除"
        onClose={() => setDeleteTarget(null)}
        onConfirm={handleDelete}
      />
    </>
  )
}
