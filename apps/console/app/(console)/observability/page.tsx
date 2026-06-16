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
  ObservabilityByHourDowCell,
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

// Heatmap renders the 7×24 activity grid: rows = days of week
// (Mon first, Sun last — the Chinese dashboard convention; dow=0
// is Sunday in time.Weekday so the order is [1, 2, 3, 4, 5, 6, 0]),
// columns = hours of day 00..23. Each cell is a tiny square whose
// color intensity reflects the call count on a log2 scale so a
// 1-call cell and a 100-call cell are both visible (a linear
// scale would compress the bottom 90% into the lightest bucket).
//
// The cells are SPARSE on the wire (the server only emits cells
// with at least one call); we densify to a 7×24 grid by
// defaulting missing cells to 0. The title attribute carries
// the day + hour + count for hover inspection.
//
// Why log2: a fresh tenant might have a single high-traffic
// hour and 23 empty hours; a linear scale would render both as
// the lightest shade, hiding the pattern entirely. log2
// preserves "one cell stands out" regardless of absolute scale.
function Heatmap({
  cells,
  loading,
}: {
  cells: ObservabilityByHourDowCell[]
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
  if (cells.length === 0) {
    return (
      <div className="text-sm text-claude-muted py-6 text-center">
        该时间范围内还没有调用记录
      </div>
    )
  }

  // Densify sparse cells into a 7×24 grid. Out-of-range cells
  // (which should never appear from a correct server) are
  // silently dropped — defensive against a future bug or
  // timezone offset shift.
  const grid: number[][] = Array.from({ length: 7 }, () =>
    Array(24).fill(0),
  )
  let maxCount = 0
  for (const c of cells) {
    if (c.dow >= 0 && c.dow < 7 && c.hour >= 0 && c.hour < 24) {
      grid[c.dow][c.hour] = c.calls
      if (c.calls > maxCount) maxCount = c.calls
    }
  }

  // Mon-first display order. time.Weekday has Sun=0, so we
  // re-order to the Chinese dashboard convention.
  const dayOrder = [1, 2, 3, 4, 5, 6, 0]
  const dayLabels = ['周一', '周二', '周三', '周四', '周五', '周六', '周日']

  // Log2 bucketing. Empty cells fall to the empty bucket;
  // every other cell maps to one of four intensities based on
  // its position on the log2 scale between 1 and maxCount.
  // The exact Tailwind class strings are picked so a
  // future change to claude-coral or the opacity scale is
  // local to this function.
  const bucket = (n: number): string => {
    if (n === 0) return 'bg-claude-canvas border border-claude-hairline-soft'
    if (maxCount === 1) return 'bg-claude-coral/40'
    const ratio = Math.log2(n + 1) / Math.log2(maxCount + 1)
    if (ratio < 0.25) return 'bg-claude-coral/20'
    if (ratio < 0.5) return 'bg-claude-coral/40'
    if (ratio < 0.75) return 'bg-claude-coral/70'
    return 'bg-claude-coral'
  }

  return (
    <div className="overflow-x-auto pb-1">
      <table
        className="border-separate"
        style={{ borderSpacing: 2 }}
      >
        <thead>
          <tr>
            <th className="w-10" />
            {Array.from({ length: 24 }, (_, h) => (
              <th
                key={h}
                className="w-[18px] text-[10px] text-claude-muted-soft font-mono font-normal text-center"
              >
                {h % 3 === 0 ? h.toString().padStart(2, '0') : ''}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {dayOrder.map((dow, i) => (
            <tr key={dow}>
              <td className="text-[11px] text-claude-muted pr-2 text-right font-medium whitespace-nowrap">
                {dayLabels[i]}
              </td>
              {Array.from({ length: 24 }, (_, h) => {
                const c = grid[dow][h]
                return (
                  <td
                    key={h}
                    className={`w-[18px] h-[18px] rounded-[2px] ${bucket(c)} cursor-default`}
                    title={`${dayLabels[i]} ${h.toString().padStart(2, '0')}:00 — ${c.toLocaleString('zh-CN')} calls`}
                  />
                )
              })}
            </tr>
          ))}
        </tbody>
      </table>
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

        {/* 活跃时段热力图 — when in the week are calls concentrated?
            Shows peak hours and dead hours at a glance. Complements
            the trend chart (which shows WHEN in absolute time)
            with a WHEN-in-the-week view (which is more actionable
            for staffing + capacity planning). */}
        <div className="bg-white border border-claude-hairline rounded-lg p-5 shadow-claude-soft">
          <div className="flex items-center justify-between mb-3">
            <div>
              <h2 className="text-sm font-medium text-claude-ink">
                活跃时段热力图
              </h2>
              <p className="text-[11px] text-claude-muted-soft mt-0.5">
                按 周 × 小时 聚合的调用密度 · 颜色越深调用越多
              </p>
            </div>
            <div className="flex items-center gap-1 text-[10px] text-claude-muted-soft">
              <span>少</span>
              <span className="w-[14px] h-[14px] rounded-[2px] bg-claude-canvas border border-claude-hairline-soft" />
              <span className="w-[14px] h-[14px] rounded-[2px] bg-claude-coral/20" />
              <span className="w-[14px] h-[14px] rounded-[2px] bg-claude-coral/40" />
              <span className="w-[14px] h-[14px] rounded-[2px] bg-claude-coral/70" />
              <span className="w-[14px] h-[14px] rounded-[2px] bg-claude-coral" />
              <span>多</span>
            </div>
          </div>
          <Heatmap cells={data?.by_hour_dow ?? []} loading={trendLoading} />
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
