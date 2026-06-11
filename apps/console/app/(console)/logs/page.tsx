'use client'

// /logs is the MCN operator's full call-log query surface.
// Layout (top-to-bottom):
//   1. TopBar (title only)
//   2. Filter bar: time range, skill, status, search, export
//   3. Table: created_at / skill / provider / status /
//      latency / cost / action
//   4. Pagination: prev / next / total count
//
// The detail dialog is rendered as an overlay on the same page
// rather than a separate route so the operator can open one row,
// read the error, and close it without losing their filter
// state. Detail is loaded lazily from GET /api/logs/:id only on
// open; the list payload already carries the row's basic fields
// so we don't fetch twice for a row that's already in the table.
//
// Scope cuts (intentional, see task #45):
//   - No realtime refresh — the page does a single fetch per
//     filter change. A "刷新" button surfaces a manual retry.
//   - No saved filters — the URL is not currently reflected in
//     the page state, so a deep link cannot restore the exact
//     filter set. A future task can wire the filter into
//     useSearchParams for shareable links.
//   - No bulk export — the export always reflects the active
//     filter, so a "select all" UX is intentionally not built.

import { useEffect, useMemo, useState } from 'react'
import { TopBar } from '@/components/TopBar'
import { useLogs, type LogsFilter } from '@/lib/use-logs'
import { SKILLS } from '@/lib/skills-registry'
import { api, ApiError } from '@opc/shared/api'
import type { CallLog, CallLogStatus } from '@opc/shared/types'

// -------- Constants --------

// Range presets — same vocabulary as /observability, but the
// wire shape is different (logs takes [from, to), observability
// takes a `range=` string). We resolve the preset to an
// [from, to) ISO pair at submit time so the rest of the
// pipeline is a single code path.
const RANGE_OPTIONS: {
  value: '7d' | '30d' | 'today'
  label: string
}[] = [
  { value: '7d', label: '近 7 天' },
  { value: '30d', label: '近 30 天' },
  { value: 'today', label: '今天' },
]

// Status options — labels match the buckets the Go middleware
// stores. The wire value is the int bucket (0..3); the dropdown
// uses '' as the "any status" sentinel.
const STATUS_OPTIONS: { value: '' | CallLogStatus; label: string }[] = [
  { value: '', label: '全部' },
  { value: 0, label: '成功 (2xx)' },
  { value: 1, label: '客户端错误 (4xx)' },
  { value: 2, label: '服务端错误 (5xx)' },
  { value: 3, label: '超时' },
]

// Status tone for the chip + the table. The status field on the
// wire is the int bucket; we use a switch here so a future
// status code lands as a TypeScript error (the union is closed).
const STATUS_TONE: Record<
  CallLogStatus,
  { label: string; tone: string; dot: string }
> = {
  0: {
    label: '成功',
    tone: 'bg-claude-success/15 text-claude-success',
    dot: 'bg-claude-success',
  },
  1: {
    label: '4xx',
    tone: 'bg-claude-warning/15 text-claude-warning',
    dot: 'bg-claude-warning',
  },
  2: {
    label: '5xx',
    tone: 'bg-claude-error/15 text-claude-error',
    dot: 'bg-claude-error',
  },
  3: {
    label: '超时',
    tone: 'bg-claude-surface-card text-claude-muted',
    dot: 'bg-claude-muted-soft',
  },
}

// PAGE_SIZE mirrors the hook's PAGE_SIZE — duplicated here so
// the pagination math in the page (total pages) does not need
// to thread the value out of the hook. Keep these in sync.
const PAGE_SIZE = 50

// -------- Helpers --------

// resolveRange turns a 7d/30d/today preset into an [from, to)
// RFC3339 pair. The "to" is "now" and the "from" is the
// matching lookback. The "today" preset uses local-day bounds
// (00:00 to 24:00) so the operator's "今天" intent is
// calendar-day, not a rolling 24h window.
function resolveRange(
  preset: '7d' | '30d' | 'today',
): { from: string; to: string } {
  const now = new Date()
  if (preset === 'today') {
    const start = new Date(
      now.getFullYear(),
      now.getMonth(),
      now.getDate(),
      0,
      0,
      0,
      0,
    )
    const end = new Date(
      now.getFullYear(),
      now.getMonth(),
      now.getDate() + 1,
      0,
      0,
      0,
      0,
    )
    return { from: start.toISOString(), to: end.toISOString() }
  }
  const days = preset === '30d' ? 30 : 7
  const from = new Date(now.getTime() - days * 24 * 60 * 60 * 1000)
  return { from: from.toISOString(), to: now.toISOString() }
}

// formatDateTime renders an ISO timestamp as YYYY-MM-DD HH:MM:SS
// in the operator's local timezone. Used for the detail dialog
// where second-level precision is wanted.
function formatDateTime(iso: string): string {
  const d = new Date(iso)
  if (isNaN(d.getTime())) return '—'
  const yyyy = d.getFullYear()
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  const hh = String(d.getHours()).padStart(2, '0')
  const mi = String(d.getMinutes()).padStart(2, '0')
  const ss = String(d.getSeconds()).padStart(2, '0')
  return `${yyyy}-${mm}-${dd} ${hh}:${mi}:${ss}`
}

// formatDate renders an ISO timestamp as YYYY-MM-DD in local
// time. Used for the table + the detail dialog's secondary
// timestamps (user_id etc. don't carry a date; created_at is
// the only timestamp on the row).
function formatDate(iso: string): string {
  const d = new Date(iso)
  if (isNaN(d.getTime())) return '—'
  const yyyy = d.getFullYear()
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  return `${yyyy}-${mm}-${dd}`
}

// formatCost renders cost_cents as a ¥x.xx string. The handler
// always emits CNY today; cost_currency is shown alongside as a
// small label so a future multi-currency export isn't a
// surprise.
function formatCost(cents: number, currency: string): string {
  const symbol = currency === 'CNY' ? '¥' : currency === 'USD' ? '$' : ''
  return `${symbol}${(cents / 100).toFixed(2)}`
}

// buildInitialFilter seeds the hook with the default range. We
// resolve to [from, to) so a filter change in the page can
// re-derive the same window without re-running the helper.
function buildInitialFilter(): LogsFilter {
  const { from, to } = resolveRange('7d')
  return {
    from,
    to,
    skill: '',
    status: undefined,
    provider: '',
    search: '',
    page: 1,
  }
}

// -------- Subcomponents --------

// RangeSelector mirrors the /observability segmented control so
// the operator's muscle memory carries over. Active button gets
// the raised white pill; inactive stays flat in muted.
function RangeSelector({
  value,
  onChange,
}: {
  value: '7d' | '30d' | 'today'
  onChange: (next: '7d' | '30d' | 'today') => void
}) {
  return (
    <div className="inline-flex p-0.5 bg-claude-surface-card rounded-md border border-claude-hairline-soft">
      {RANGE_OPTIONS.map((opt) => {
        const active = opt.value === value
        return (
          <button
            key={opt.value}
            type="button"
            onClick={() => onChange(opt.value)}
            className={
              'px-3 py-1 text-xs rounded transition-colors ' +
              (active
                ? 'bg-white text-claude-ink shadow-claude-soft'
                : 'text-claude-muted hover:text-claude-ink')
            }
          >
            {opt.label}
          </button>
        )
      })}
    </div>
  )
}

// StatusChip renders the bucket as a small pill. We use the
// shared `STATUS_TONE` map so the detail dialog and the table
// stay visually identical.
function StatusChip({ status }: { status: CallLogStatus }) {
  const meta = STATUS_TONE[status]
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

// SkeletonRow is a single animated placeholder line. Five of
// these render the loading state — narrow column, no header
// noise, so the operator can tell at a glance that the table
// is fetching.
function SkeletonRow({ widths }: { widths: string[] }) {
  return (
    <tr className="border-t border-claude-hairline-soft">
      {widths.map((w, i) => (
        <td key={i} className="px-4 py-3">
          <div
            className="h-3 bg-claude-surface-card rounded animate-pulse"
            style={{ width: w }}
          />
        </td>
      ))}
    </tr>
  )
}

// ErrorBanner is the standard "something went wrong, retry"
// surface used across the console.
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
        <p className="text-sm text-claude-ink">加载调用日志失败</p>
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

// LogRow is one row in the table. We intentionally keep the
// "详情" action as a button (not a row-wide <a>) so the row is
// still selectable for future bulk operations.
function LogRow({
  log,
  onOpen,
}: {
  log: CallLog
  onOpen: (id: number) => void
}) {
  return (
    <tr className="border-t border-claude-hairline-soft hover:bg-claude-canvas/60">
      <td className="px-4 py-3 text-sm text-claude-body whitespace-nowrap font-mono tabular-nums">
        {formatDate(log.created_at)}
      </td>
      <td className="px-4 py-3 text-sm text-claude-ink font-medium">
        <span className="font-mono text-xs px-1.5 py-0.5 rounded bg-claude-surface-soft text-claude-body">
          {log.skill}
        </span>
      </td>
      <td className="px-4 py-3 text-sm text-claude-body">
        <span className="font-mono text-xs px-1.5 py-0.5 rounded bg-claude-surface-soft text-claude-body">
          {log.provider}
        </span>
      </td>
      <td className="px-4 py-3">
        <StatusChip status={log.status} />
      </td>
      <td className="px-4 py-3 text-sm text-claude-body whitespace-nowrap font-mono tabular-nums text-right">
        {log.latency_ms}ms
      </td>
      <td className="px-4 py-3 text-sm text-claude-ink whitespace-nowrap font-mono tabular-nums text-right">
        {formatCost(log.cost_cents, log.cost_currency)}
      </td>
      <td className="px-4 py-3 text-right">
        <button
          type="button"
          onClick={() => onOpen(log.id)}
          className="px-2 py-1 text-xs text-claude-coral hover:bg-claude-coral/10 rounded transition-colors"
        >
          详情
        </button>
      </td>
    </tr>
  )
}

// Pagination renders prev/next + a "page N of M · total T"
// hint. We hide the controls entirely when total ≤ page size
// so a single-page result doesn't get a "page 1 of 1" footer.
function Pagination({
  page,
  total,
  onPageChange,
}: {
  page: number
  total: number
  onPageChange: (next: number) => void
}) {
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  if (total <= PAGE_SIZE) {
    return (
      <p className="text-xs text-claude-muted-soft">
        共 {total.toLocaleString('zh-CN')} 条记录
      </p>
    )
  }
  const isFirst = page <= 1
  const isLast = page >= totalPages
  return (
    <div className="flex items-center justify-between">
      <p className="text-xs text-claude-muted-soft">
        共 {total.toLocaleString('zh-CN')} 条记录 · 第 {page} / {totalPages} 页
      </p>
      <div className="inline-flex items-center gap-2">
        <button
          type="button"
          onClick={() => onPageChange(page - 1)}
          disabled={isFirst}
          className="px-3 py-1 text-xs text-claude-body hover:text-claude-ink hover:bg-claude-surface-soft rounded transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
        >
          上一页
        </button>
        <button
          type="button"
          onClick={() => onPageChange(page + 1)}
          disabled={isLast}
          className="px-3 py-1 text-xs text-claude-body hover:text-claude-ink hover:bg-claude-surface-soft rounded transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
        >
          下一页
        </button>
      </div>
    </div>
  )
}

// CallLogDetailDialog renders the full CallLog row in a modal.
// We use a state machine (loading / ok / not_found / error) so
// the dialog stays self-describing across network states. ESC
// closes; backdrop click closes. The dialog is keyed by logId
// in the parent so a row switch re-fetches on open.
function CallLogDetailDialog({
  logId,
  onClose,
}: {
  logId: number | null
  onClose: () => void
}) {
  type State =
    | { kind: 'idle' }
    | { kind: 'loading' }
    | { kind: 'ok'; log: CallLog }
    | { kind: 'not_found' }
    | { kind: 'error'; message: string }
  const [state, setState] = useState<State>({ kind: 'idle' })

  useEffect(() => {
    if (logId === null) {
      setState({ kind: 'idle' })
      return
    }
    let cancelled = false
    setState({ kind: 'loading' })
    ;(async () => {
      try {
        const log = await api.logs.get(logId)
        if (cancelled) return
        setState({ kind: 'ok', log })
      } catch (err: unknown) {
        if (cancelled) return
        if (err instanceof ApiError && err.status === 404) {
          setState({ kind: 'not_found' })
        } else {
          setState({
            kind: 'error',
            message:
              err instanceof Error ? err.message : '加载详情失败',
          })
        }
      }
    })()
    return () => {
      cancelled = true
    }
  }, [logId])

  useEffect(() => {
    if (logId === null) return
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [logId, onClose])

  if (logId === null) return null

  return (
    <div
      className="fixed inset-0 bg-black/40 z-50 flex items-center justify-center p-4"
      onClick={onClose}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="log-detail-title"
        onClick={(e) => e.stopPropagation()}
        className="bg-white rounded-lg shadow-xl max-w-2xl w-full border border-claude-hairline max-h-[85vh] overflow-y-auto"
      >
        <div className="px-6 py-5 space-y-5">
          <div className="flex items-center justify-between">
            <h2
              id="log-detail-title"
              className="text-base font-medium text-claude-ink"
            >
              调用日志 #{logId}
            </h2>
            <button
              type="button"
              onClick={onClose}
              className="text-claude-muted hover:text-claude-ink text-sm"
              aria-label="关闭"
            >
              ✕
            </button>
          </div>

          {state.kind === 'loading' && (
            <div className="space-y-3">
              <div className="h-4 w-1/3 bg-claude-surface-card rounded animate-pulse" />
              <div className="h-3 w-2/3 bg-claude-surface-card rounded animate-pulse" />
              <div className="h-3 w-1/2 bg-claude-surface-card rounded animate-pulse" />
            </div>
          )}

          {state.kind === 'not_found' && (
            <div className="bg-claude-canvas border border-claude-hairline rounded-md px-4 py-3 text-sm text-claude-body">
              这条日志不存在或已被删除。
            </div>
          )}

          {state.kind === 'error' && (
            <div className="bg-claude-canvas border border-claude-error/30 rounded-md px-4 py-3 text-sm text-claude-body">
              加载失败: {state.message}
            </div>
          )}

          {state.kind === 'ok' && <DetailBody log={state.log} />}
        </div>
      </div>
    </div>
  )
}

// DetailBody is the readable surface once the row is loaded.
// The three "横排" fields (input_tokens / output_tokens /
// cost_cents) live in a 3-column grid. error_message, when
// present, gets the error-tone surface so an operator can spot
// a failed call in the table-of-contents at a glance.
function DetailBody({ log }: { log: CallLog }) {
  return (
    <div className="space-y-5">
      {/* Status + skill + provider */}
      <div className="flex items-center gap-3 flex-wrap">
        <StatusChip status={log.status} />
        <span className="font-mono text-xs px-1.5 py-0.5 rounded bg-claude-surface-soft text-claude-body">
          {log.skill}
        </span>
        <span className="text-claude-muted-soft">·</span>
        <span className="font-mono text-xs px-1.5 py-0.5 rounded bg-claude-surface-soft text-claude-body">
          {log.provider}
        </span>
      </div>

      {/* 横排 3 字段: tokens + cost */}
      <div className="grid grid-cols-3 gap-3">
        <LabeledValue label="输入 tokens" value={log.input_tokens.toLocaleString('zh-CN')} />
        <LabeledValue label="输出 tokens" value={log.output_tokens.toLocaleString('zh-CN')} />
        <LabeledValue
          label="成本"
          value={formatCost(log.cost_cents, log.cost_currency)}
        />
      </div>

      {/* Meta: id / user / credential / latency / created_at */}
      <div className="grid grid-cols-2 gap-x-6 gap-y-2 text-sm">
        <MetaRow label="日志 ID" value={`#${log.id}`} />
        <MetaRow label="用户 ID" value={`#${log.user_id}`} />
        <MetaRow
          label="凭据 ID"
          value={
            log.credential_id === null ? '— (无凭据)' : `#${log.credential_id}`
          }
        />
        <MetaRow label="延迟" value={`${log.latency_ms}ms`} />
        <MetaRow label="货币" value={log.cost_currency} />
        <MetaRow
          label="创建时间"
          value={formatDateTime(log.created_at)}
        />
      </div>

      {/* Error message — highlighted when present */}
      {log.error_message ? (
        <div>
          <div className="text-[11px] uppercase tracking-eyebrow text-claude-muted mb-1.5">
            错误信息
          </div>
          <div className="bg-claude-error/10 border border-claude-error/30 rounded-md px-3 py-2.5 text-sm font-mono text-claude-error break-words whitespace-pre-wrap">
            {log.error_message}
          </div>
        </div>
      ) : (
        <div>
          <div className="text-[11px] uppercase tracking-eyebrow text-claude-muted mb-1.5">
            错误信息
          </div>
          <div className="text-sm text-claude-muted-soft">无</div>
        </div>
      )}
    </div>
  )
}

// LabeledValue is a single key/value pair rendered in a card-
// like surface. Used for the 3-field "横排" strip where we
// want the label small + uppercase and the value large + mono.
function LabeledValue({
  label,
  value,
}: {
  label: string
  value: string
}) {
  return (
    <div className="bg-claude-surface-soft rounded-md px-3 py-2.5">
      <div className="text-[11px] uppercase tracking-eyebrow text-claude-muted">
        {label}
      </div>
      <div className="mt-1 font-mono text-base text-claude-ink tabular-nums">
        {value}
      </div>
    </div>
  )
}

// MetaRow is a flat key/value pair for the secondary field
// grid. Label on top in muted-eyebrow style, value below in
// body text — keeps the rhythm with LabeledValue while
// staying lower-prominence.
function MetaRow({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="text-[11px] uppercase tracking-eyebrow text-claude-muted-soft">
        {label}
      </div>
      <div className="mt-0.5 text-sm text-claude-body font-mono tabular-nums">
        {value}
      </div>
    </div>
  )
}

// -------- Page --------

export default function LogsPage() {
  // The active range preset is its own state — the page owns
  // the preset and the hook owns the resolved [from, to). When
  // the preset changes, the page re-derives the window and
  // pushes both into the hook.
  const [range, setRange] = useState<'7d' | '30d' | 'today'>('7d')
  const [filterSeed, setFilterSeed] = useState<LogsFilter>(() =>
    buildInitialFilter(),
  )
  const [skillFilter, setSkillFilter] = useState<string>('')
  const [statusFilter, setStatusFilter] = useState<'' | CallLogStatus>('')
  const [openLogId, setOpenLogId] = useState<number | null>(null)

  const { items, total, loading, error, filter, setFilter, setPage, setSearchInput, refresh } =
    useLogs(filterSeed)

  // Range change → re-derive the [from, to) pair and reset
  // skill/status/search/page. We rebuild the seed so the hook
  // sees a referentially new filter object and re-fetches.
  function handleRangeChange(next: '7d' | '30d' | 'today') {
    setRange(next)
    const { from, to } = resolveRange(next)
    setFilterSeed({
      from,
      to,
      skill: '',
      status: undefined,
      provider: '',
      search: '',
      page: 1,
    })
    setSkillFilter('')
    setStatusFilter('')
    setSearchInput('')
  }

  // Skill / status / provider changes push a partial into the
  // hook. We do not rebuild the seed (the hook owns the page
  // counter), just hand it a partial.
  function handleSkillChange(next: string) {
    setSkillFilter(next)
    setFilter({ skill: next || undefined })
  }

  function handleStatusChange(next: '' | CallLogStatus) {
    setStatusFilter(next)
    setFilter({ status: next === '' ? undefined : next })
  }

  // Export builds a URL with the same filter as the active
  // hook state, then navigates the browser. The server's
  // Content-Disposition header makes the browser save the
  // file directly; we don't fetch+blob it.
  function handleExport() {
    const url = api.logs.exportURL({
      from: filter.from,
      to: filter.to,
      skill: filter.skill,
      status: filter.status,
      provider: filter.provider,
      search: filter.search,
    })
    if (typeof window !== 'undefined') {
      window.location.href = url
    }
  }

  // Skill options for the dropdown. We sort by id to match the
  // registry's natural order; the operator gets a stable list
  // even if the registry re-orders internally.
  const skillOptions = useMemo(
    () => SKILLS.map((s) => ({ value: s.id, label: s.name })),
    [],
  )

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  return (
    <>
      <TopBar title="调用日志" />
      <div className="p-6 space-y-4">
        {/* Filter bar */}
        <div className="bg-white border border-claude-hairline rounded-lg shadow-claude-soft p-4 space-y-3">
          <div className="flex items-center justify-between flex-wrap gap-3">
            <p className="text-sm text-claude-muted">
              全量调用日志查询 + CSV 导出 (给 MCN 内部审计)。
            </p>
            <RangeSelector value={range} onChange={handleRangeChange} />
          </div>
          <div className="flex items-center flex-wrap gap-2">
            <FilterSelect
              value={skillFilter}
              onChange={handleSkillChange}
              options={[{ value: '', label: '全部 skill' }, ...skillOptions]}
              ariaLabel="按 skill 过滤"
            />
            <FilterSelect
              value={statusFilter === '' ? '' : String(statusFilter)}
              onChange={(v) =>
                handleStatusChange(
                  v === '' ? '' : (Number(v) as CallLogStatus),
                )
              }
              options={STATUS_OPTIONS.map((o) => ({
                value: o.value === '' ? '' : String(o.value),
                label: o.label,
              }))}
              ariaLabel="按 status 过滤"
            />
            <input
              type="search"
              placeholder="搜索 error_message…"
              defaultValue=""
              onChange={(e) => setSearchInput(e.target.value)}
              className="flex-1 min-w-[180px] max-w-xs px-3 py-1.5 text-sm bg-white border border-claude-hairline rounded-md text-claude-ink placeholder:text-claude-muted-soft focus:outline-none focus:ring-2 focus:ring-claude-coral/30 focus:border-claude-coral transition-colors"
              aria-label="搜索 error_message"
            />
            <button
              type="button"
              onClick={() => void refresh()}
              className="px-3 py-1.5 text-sm text-claude-body hover:text-claude-ink hover:bg-claude-surface-soft rounded-md transition-colors"
            >
              刷新
            </button>
            <button
              type="button"
              onClick={handleExport}
              disabled={loading && items.length === 0}
              className="ml-auto px-3 py-1.5 text-sm bg-claude-coral text-claude-on-primary rounded-md hover:bg-claude-coral-active transition-colors disabled:opacity-50"
            >
              导出 CSV
            </button>
          </div>
        </div>

        {/* States */}
        {error && !loading && (
          <ErrorBanner message={error} onRetry={() => void refresh()} />
        )}

        {/* Table */}
        <div className="bg-white border border-claude-hairline rounded-lg shadow-claude-soft overflow-hidden">
          <table className="w-full text-left">
            <thead className="bg-claude-surface-card text-xs uppercase tracking-eyebrow text-claude-muted">
              <tr>
                <th className="px-4 py-2.5 font-medium">时间</th>
                <th className="px-4 py-2.5 font-medium">Skill</th>
                <th className="px-4 py-2.5 font-medium">Provider</th>
                <th className="px-4 py-2.5 font-medium">状态</th>
                <th className="px-4 py-2.5 font-medium text-right">延迟</th>
                <th className="px-4 py-2.5 font-medium text-right">成本</th>
                <th className="px-4 py-2.5 font-medium text-right">操作</th>
              </tr>
            </thead>
            <tbody>
              {loading && items.length === 0 ? (
                <>
                  <SkeletonRow
                    widths={['40%', '40%', '40%', '30%', '20%', '30%', '20%']}
                  />
                  <SkeletonRow
                    widths={['40%', '50%', '30%', '30%', '25%', '30%', '20%']}
                  />
                  <SkeletonRow
                    widths={['40%', '35%', '45%', '30%', '20%', '30%', '20%']}
                  />
                  <SkeletonRow
                    widths={['40%', '40%', '40%', '30%', '25%', '30%', '20%']}
                  />
                  <SkeletonRow
                    widths={['40%', '50%', '30%', '30%', '20%', '30%', '20%']}
                  />
                </>
              ) : items.length === 0 ? (
                <tr>
                  <td
                    colSpan={7}
                    className="px-4 py-12 text-center text-sm text-claude-muted"
                  >
                    当前过滤条件下没有日志记录
                  </td>
                </tr>
              ) : (
                items.map((log) => (
                  <LogRow key={log.id} log={log} onOpen={setOpenLogId} />
                ))
              )}
            </tbody>
          </table>
        </div>

        {/* Pagination */}
        {!loading && (
          <Pagination
            page={filter.page}
            total={total}
            onPageChange={setPage}
          />
        )}

        {/* Footer hint — total + page count. Shown only when we
            have data, so the empty state's "no records" copy
            stays the page's single source of truth. */}
        {!loading && items.length > 0 && totalPages > 1 && (
          <p className="text-xs text-claude-muted-soft">
            按当前过滤条件查询 · 每页 {PAGE_SIZE} 条 · 最多显示前 {Math.min(
              PAGE_SIZE * 5,
              total,
            ).toLocaleString('zh-CN')} 条 (导出 CSV 获取全量)
          </p>
        )}
      </div>

      <CallLogDetailDialog
        logId={openLogId}
        onClose={() => setOpenLogId(null)}
      />
    </>
  )
}

// FilterSelect is a styled <select> that matches the rest of
// the filter bar. Pulled out so the skill + status dropdowns
// can share a single source of truth for the height/padding.
function FilterSelect({
  value,
  onChange,
  options,
  ariaLabel,
}: {
  value: string
  onChange: (next: string) => void
  options: { value: string; label: string }[]
  ariaLabel: string
}) {
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      aria-label={ariaLabel}
      className="px-3 py-1.5 text-sm bg-white border border-claude-hairline rounded-md text-claude-ink focus:outline-none focus:ring-2 focus:ring-claude-coral/30 focus:border-claude-coral transition-colors"
    >
      {options.map((opt) => (
        <option key={opt.value || 'all'} value={opt.value}>
          {opt.label}
        </option>
      ))}
    </select>
  )
}
