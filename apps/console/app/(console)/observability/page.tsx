'use client'

// /observability is the operator's view onto the call-observability
// dashboard. The single GET /api/observability/summary endpoint
// powers every region on the page; we do the per-region
// shaping client-side.
//
// Layout (top-to-bottom):
//   1. TopBar (title only)
//   2. Range selector (7d / 30d / today) — drives the
//      useObservabilitySummary hook
//   3. Three "today" cards: calls, success rate, cost
//   4. Trend chart (dual-axis line chart, hand-rolled SVG)
//   5. Two breakdowns side by side: by_skill / by_provider
//
// The 7d / 30d / today buttons look like a segmented control
// and live just under the TopBar. The "today" cards reflect
// the wire shape's `today` field regardless of the active
// range — the page is honest that "today" is always the
// calendar day, not the window average.

import { useState } from 'react'
import { TopBar } from '@/components/TopBar'
import { TrendChart } from '@/components/TrendChart'
import { useObservabilitySummary } from '@/lib/use-observability'
import type {
  ObservabilityByProviderRow,
  ObservabilityBySkillRow,
  ObservabilitySummary,
  ObservabilitySummaryRange,
} from '@opc/shared/api'

// Range tabs — the wire surface the page exposes. The labels
// are Chinese; the values match the server-side constants.
const RANGE_OPTIONS: { value: ObservabilitySummaryRange; label: string }[] = [
  { value: '7d', label: '近 7 天' },
  { value: '30d', label: '近 30 天' },
  { value: 'today', label: '今天' },
]

// "今天" card copy — three labels that map 1:1 to the
// ObservabilityTodaySummary fields. The card is purely a
// presentational wrapper; the value rendering is done in
// SummaryCard below.
const TODAY_CARDS: { key: keyof SummaryCardValue; label: string; hint: string }[] = [
  { key: 'calls', label: '调用次数', hint: '今日 00:00 起' },
  { key: 'success', label: '成功率', hint: '成功 / 全部' },
  { key: 'cost', label: '总成本', hint: '单位: 分 (CNY)' },
]

type SummaryCardValue = {
  calls: string
  success: string
  cost: string
}

function buildSummaryValues(data: ObservabilitySummary | null): SummaryCardValue {
  if (!data) return { calls: '—', success: '—', cost: '—' }
  const t = data.today
  return {
    calls: t.calls.toLocaleString('zh-CN'),
    // success_rate is a 0-1 fraction; render as a percentage
    // with one decimal. 0 calls => 0% (the server already
    // maps the divide-by-zero to 0).
    success: `${(t.success_rate * 100).toFixed(1)}%`,
    // cost is integer cents; show "¥x.xx" so the operator
    // sees the actual RMB rather than "1200 分". Phase 4
    // will introduce multi-currency; today is hard-CNY.
    cost: `¥${(t.cost_cents / 100).toFixed(2)}`,
  }
}

function RangeSelector({
  value,
  onChange,
}: {
  value: ObservabilitySummaryRange
  onChange: (next: ObservabilitySummaryRange) => void
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

function SkeletonBar({ width }: { width: string }) {
  return (
    <div
      className="h-3 bg-claude-surface-card rounded animate-pulse"
      style={{ width }}
    />
  )
}

function SummaryCard({
  label,
  hint,
  value,
  loading,
}: {
  label: string
  hint: string
  value: string
  loading: boolean
}) {
  return (
    <div className="bg-white border border-claude-hairline rounded-lg p-5 shadow-claude-soft">
      <div className="text-xs uppercase tracking-eyebrow text-claude-muted">
        {label}
      </div>
      <div className="mt-2 h-7 flex items-center">
        {loading ? (
          <SkeletonBar width="60%" />
        ) : (
          <div className="font-mono text-2xl text-claude-ink tabular-nums">
            {value}
          </div>
        )}
      </div>
      <div className="mt-1 text-[11px] text-claude-muted-soft">{hint}</div>
    </div>
  )
}

function SkillTable({
  rows,
  loading,
}: {
  rows: ObservabilityBySkillRow[]
  loading: boolean
}) {
  if (loading) {
    return (
      <div className="space-y-2">
        {[0, 1, 2].map((i) => (
          <SkeletonBar key={i} width={`${70 - i * 10}%`} />
        ))}
      </div>
    )
  }
  if (rows.length === 0) {
    return (
      <div className="text-sm text-claude-muted py-6 text-center">
        该时间范围内还没有 skill 调用记录
      </div>
    )
  }
  return (
    <table className="w-full text-left text-sm">
      <thead className="text-xs uppercase tracking-eyebrow text-claude-muted">
        <tr className="border-b border-claude-hairline">
          <th className="py-2 font-medium">Skill</th>
          <th className="py-2 font-medium text-right">调用</th>
          <th className="py-2 font-medium text-right">成功率</th>
          <th className="py-2 font-medium text-right">p50</th>
          <th className="py-2 font-medium text-right">p95</th>
          <th className="py-2 font-medium text-right">成本</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((r) => (
          <tr
            key={r.skill}
            className="border-b border-claude-hairline-soft last:border-0"
          >
            <td className="py-2 text-claude-ink font-medium">
              <span className="font-mono text-xs px-1.5 py-0.5 rounded bg-claude-surface-soft text-claude-body">
                {r.skill}
              </span>
            </td>
            <td className="py-2 text-right font-mono tabular-nums text-claude-body">
              {r.calls.toLocaleString('zh-CN')}
            </td>
            <td className="py-2 text-right font-mono tabular-nums text-claude-body">
              {(r.success_rate * 100).toFixed(1)}%
            </td>
            <td className="py-2 text-right font-mono tabular-nums text-claude-muted">
              {r.p50_ms ? `${r.p50_ms}ms` : '—'}
            </td>
            <td className="py-2 text-right font-mono tabular-nums text-claude-muted">
              {r.p95_ms ? `${r.p95_ms}ms` : '—'}
            </td>
            <td className="py-2 text-right font-mono tabular-nums text-claude-ink">
              ¥{(r.cost_cents / 100).toFixed(2)}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

function ProviderTable({
  rows,
  loading,
}: {
  rows: ObservabilityByProviderRow[]
  loading: boolean
}) {
  if (loading) {
    return (
      <div className="space-y-2">
        {[0, 1, 2].map((i) => (
          <SkeletonBar key={i} width={`${70 - i * 10}%`} />
        ))}
      </div>
    )
  }
  if (rows.length === 0) {
    return (
      <div className="text-sm text-claude-muted py-6 text-center">
        该时间范围内还没有 provider 调用记录
      </div>
    )
  }
  return (
    <table className="w-full text-left text-sm">
      <thead className="text-xs uppercase tracking-eyebrow text-claude-muted">
        <tr className="border-b border-claude-hairline">
          <th className="py-2 font-medium">Provider</th>
          <th className="py-2 font-medium text-right">调用</th>
          <th className="py-2 font-medium text-right">成本</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((r) => (
          <tr
            key={r.provider}
            className="border-b border-claude-hairline-soft last:border-0"
          >
            <td className="py-2 text-claude-ink font-medium">
              <span className="font-mono text-xs px-1.5 py-0.5 rounded bg-claude-surface-soft text-claude-body">
                {r.provider}
              </span>
            </td>
            <td className="py-2 text-right font-mono tabular-nums text-claude-body">
              {r.calls.toLocaleString('zh-CN')}
            </td>
            <td className="py-2 text-right font-mono tabular-nums text-claude-ink">
              ¥{(r.cost_cents / 100).toFixed(2)}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
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
        <p className="text-sm text-claude-ink">加载观测数据失败</p>
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

// Skill sort options — the page defaults to "调用次数倒序" to
// match the server's natural order, and offers 成功率 / 成本
// alternatives so an operator can pivot quickly without a
// dedicated filter UI.
type SkillSortKey = 'calls' | 'success' | 'cost'
const SKILL_SORT_OPTIONS: { value: SkillSortKey; label: string }[] = [
  { value: 'calls', label: '调用次数' },
  { value: 'success', label: '成功率' },
  { value: 'cost', label: '成本' },
]

function sortSkillRows(
  rows: ObservabilityBySkillRow[],
  key: SkillSortKey,
): ObservabilityBySkillRow[] {
  // Always render a new array (immutable) so React sees a
  // referential change and re-renders the rows.
  const copy = rows.slice()
  copy.sort((a, b) => {
    if (key === 'calls') return b.calls - a.calls
    if (key === 'success') return b.success_rate - a.success_rate
    return b.cost_cents - a.cost_cents
  })
  return copy
}

export default function ObservabilityPage() {
  const { range, setRange, data, loading, error, refresh } =
    useObservabilitySummary('7d')
  const [skillSort, setSkillSort] = useState<SkillSortKey>('calls')

  const values = buildSummaryValues(data)
  // Show the skeleton only on the first load (no cached
  // data yet). On range changes we still render the previous
  // data — keeps the page from flashing empty during a
  // 200ms round-trip.
  const showSkeleton = loading && data === null
  const trendLoading = loading && data === null

  const skillRows = data ? sortSkillRows(data.by_skill, skillSort) : []

  return (
    <>
      <TopBar title="调用观测" />
      <div className="p-6 space-y-5">
        {/* Toolbar: range selector */}
        <div className="flex items-center justify-between">
          <p className="text-sm text-claude-muted">
            按 skill × provider × 时间聚合的调用量、成功率、p50/p95 延迟与成本归集。
          </p>
          <RangeSelector value={range} onChange={setRange} />
        </div>

        {error && !loading && (
          <ErrorBanner message={error} onRetry={() => void refresh()} />
        )}

        {/* 顶部 3 张卡片 (今天) */}
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          {TODAY_CARDS.map((c) => (
            <SummaryCard
              key={c.key}
              label={c.label}
              hint={c.hint}
              value={values[c.key]}
              loading={showSkeleton}
            />
          ))}
        </div>

        {/* 中部折线图 */}
        <div className="bg-white border border-claude-hairline rounded-lg p-5 shadow-claude-soft">
          <div className="flex items-center justify-between mb-2">
            <h2 className="text-sm font-medium text-claude-ink">
              每日调用量 & 成本
            </h2>
            <span className="text-[11px] text-claude-muted-soft">
              {range === 'today'
                ? '今日按小时粒度待 Phase 4'
                : range === '30d'
                ? '近 30 天'
                : '近 7 天'}
            </span>
          </div>
          <TrendChart points={data?.last_7_days ?? []} loading={trendLoading} />
        </div>

        {/* 底部 2 个表格 */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
          <div className="bg-white border border-claude-hairline rounded-lg p-5 shadow-claude-soft">
            <div className="flex items-center justify-between mb-3">
              <h2 className="text-sm font-medium text-claude-ink">
                按 Skill 拆分
              </h2>
              <div className="flex items-center gap-1">
                <span className="text-[11px] text-claude-muted-soft mr-1">
                  排序
                </span>
                {SKILL_SORT_OPTIONS.map((opt) => {
                  const active = opt.value === skillSort
                  return (
                    <button
                      key={opt.value}
                      type="button"
                      onClick={() => setSkillSort(opt.value)}
                      className={
                        'px-2 py-0.5 text-[11px] rounded transition-colors ' +
                        (active
                          ? 'bg-claude-surface-card text-claude-ink'
                          : 'text-claude-muted hover:text-claude-ink')
                      }
                    >
                      {opt.label}
                    </button>
                  )
                })}
              </div>
            </div>
            <SkillTable rows={skillRows} loading={showSkeleton} />
          </div>

          <div className="bg-white border border-claude-hairline rounded-lg p-5 shadow-claude-soft">
            <div className="flex items-center justify-between mb-3">
              <h2 className="text-sm font-medium text-claude-ink">
                按 Provider 拆分
              </h2>
              <span className="text-[11px] text-claude-muted-soft">
                按成本倒序
              </span>
            </div>
            <ProviderTable rows={data?.by_provider ?? []} loading={showSkeleton} />
          </div>
        </div>

        {/* 底部 hint */}
        {!showSkeleton && data && (
          <p className="text-xs text-claude-muted-soft">
            数据按当前选择的时间范围 (近 {range === 'today' ? '1' : range === '30d' ? '30' : '7'} 天) 聚合 · 延迟为最近一次窗口内 nearest-rank 百分位。
          </p>
        )}
      </div>
    </>
  )
}
