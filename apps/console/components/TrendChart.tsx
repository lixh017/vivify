'use client'

// TrendChart is a tiny dependency-free SVG line chart for the
// /observability page. It renders two series (calls + cost) on
// a dual Y-axis layout — left axis carries the calls count,
// right axis carries the cost. The lines use teal/coral from
// the existing Tailwind palette to stay consistent with the
// provider chips on /credentials.
//
// Why not a chart library: the dashboard only needs ONE chart
// shape (two-line, dual-axis, fixed-N points). Adding
// Recharts/Visx would be 80kb+ of JS for a 200-line
// component. The 7 / 30 points we render fit in a single
// <path d="..."> string and look identical to a hand-rolled
// Recharts output at this scale.
//
// Layout: the chart sits in a fixed-height viewBox (320x160).
// We pad the left/right edges so the first/last data point
// does not sit on the axis line, and we leave headroom above
// the highest point so the line never touches the top edge.

import type { ObservabilityDayPoint } from '@opc/shared/api'

type TrendChartProps = {
  points: ObservabilityDayPoint[]
  loading?: boolean
}

const WIDTH = 720
const HEIGHT = 260
const PAD_L = 48
const PAD_R = 56
const PAD_T = 16
const PAD_B = 32

// Color tokens — kept inline as raw hex (rather than Tailwind
// classes on stroke="currentColor") because SVG strokes inside
// an inline <svg> cannot reach parent classes without an
// intermediate <style> tag. These two values mirror
// tailwind.config.ts's claude-accent-teal / claude-coral so a
// palette change stays a two-place edit.
const COLOR_CALLS = '#5db8a6'
const COLOR_COST = '#cc785c'
const COLOR_GRID = '#e6dfd8'
const COLOR_AXIS = '#8e8b82'

function formatCompact(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`
  return String(n)
}

function formatDateLabel(iso: string): string {
  // iso is YYYY-MM-DD; we render MM-DD for the axis ticks so
  // a 30-day chart does not get cluttered with the year.
  return iso.slice(5)
}

function buildPath(
  xs: number[],
  ys: number[],
  x: (i: number) => number,
  y: (v: number) => number,
): string {
  if (xs.length === 0) return ''
  return xs
    .map((_, i) => `${i === 0 ? 'M' : 'L'} ${x(i).toFixed(1)} ${y(ys[i]).toFixed(1)}`)
    .join(' ')
}

export function TrendChart({ points, loading }: TrendChartProps) {
  const safePoints = points ?? []
  const n = safePoints.length
  const innerW = WIDTH - PAD_L - PAD_R
  const innerH = HEIGHT - PAD_T - PAD_B

  const maxCalls = Math.max(1, ...safePoints.map((p) => p.calls))
  const maxCost = Math.max(1, ...safePoints.map((p) => p.cost_cents))

  const xFor = (i: number) =>
    n <= 1 ? PAD_L + innerW / 2 : PAD_L + (innerW * i) / (n - 1)
  const yForCalls = (v: number) =>
    PAD_T + innerH - (innerH * v) / maxCalls
  const yForCost = (v: number) =>
    PAD_T + innerH - (innerH * v) / maxCost

  const callsPath = buildPath(
    safePoints.map((_, i) => i),
    safePoints.map((p) => p.calls),
    xFor,
    yForCalls,
  )
  const costPath = buildPath(
    safePoints.map((_, i) => i),
    safePoints.map((p) => p.cost_cents),
    xFor,
    yForCost,
  )

  // Y axis tick values — 4 evenly spaced, integer-rounded.
  const leftTicks = [0, 0.25, 0.5, 0.75, 1].map((t) =>
    Math.round(maxCalls * t),
  )
  const rightTicks = [0, 0.25, 0.5, 0.75, 1].map((t) =>
    Math.round(maxCost * t),
  )

  // X axis tick positions: show every label when n<=7, else
  // every other label so 30 dates do not collide.
  const xTickEvery = n > 14 ? Math.ceil(n / 10) : 1

  if (loading) {
    return (
      <div className="h-64 flex items-center justify-center text-sm text-claude-muted">
        正在加载趋势…
      </div>
    )
  }

  if (n === 0) {
    return (
      <div className="h-64 flex flex-col items-center justify-center text-sm text-claude-muted">
        <span>该时间范围内还没有调用数据</span>
        <span className="text-xs text-claude-muted-soft mt-1">
          接入凭据并跑一次 pipeline 后会出现在这里
        </span>
      </div>
    )
  }

  return (
    <div className="w-full">
      <svg
        viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
        className="w-full h-auto"
        role="img"
        aria-label="调用量与成本趋势"
      >
        {/* Horizontal grid lines (5 rows) */}
        {leftTicks.map((_, i) => {
          const y = PAD_T + (innerH * i) / 4
          return (
            <line
              key={`grid-${i}`}
              x1={PAD_L}
              x2={WIDTH - PAD_R}
              y1={y}
              y2={y}
              stroke={COLOR_GRID}
              strokeWidth={1}
            />
          )
        })}

        {/* Left Y axis labels (calls) */}
        {leftTicks.map((v, i) => {
          const y = PAD_T + innerH - (innerH * i) / 4
          return (
            <text
              key={`yl-${i}`}
              x={PAD_L - 8}
              y={y + 4}
              textAnchor="end"
              fontSize={10}
              fill={COLOR_AXIS}
              fontFamily="JetBrains Mono, ui-monospace, monospace"
            >
              {formatCompact(v)}
            </text>
          )
        })}

        {/* Right Y axis labels (cost cents) */}
        {rightTicks.map((v, i) => {
          const y = PAD_T + innerH - (innerH * i) / 4
          return (
            <text
              key={`yr-${i}`}
              x={WIDTH - PAD_R + 8}
              y={y + 4}
              textAnchor="start"
              fontSize={10}
              fill={COLOR_AXIS}
              fontFamily="JetBrains Mono, ui-monospace, monospace"
            >
              {formatCompact(v)}
            </text>
          )
        })}

        {/* X axis labels */}
        {safePoints.map((p, i) => {
          if (i % xTickEvery !== 0) return null
          return (
            <text
              key={`xt-${p.date}`}
              x={xFor(i)}
              y={HEIGHT - PAD_B + 18}
              textAnchor="middle"
              fontSize={10}
              fill={COLOR_AXIS}
              fontFamily="JetBrains Mono, ui-monospace, monospace"
            >
              {formatDateLabel(p.date)}
            </text>
          )
        })}

        {/* Cost line (coral) — drawn first so the teal line sits on top */}
        <path
          d={costPath}
          fill="none"
          stroke={COLOR_COST}
          strokeWidth={2}
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        {/* Calls line (teal) */}
        <path
          d={callsPath}
          fill="none"
          stroke={COLOR_CALLS}
          strokeWidth={2}
          strokeLinecap="round"
          strokeLinejoin="round"
        />

        {/* Data point dots — small for density, slightly larger
            on first/last so the eye can anchor the series ends. */}
        {safePoints.map((p, i) => {
          const cx = xFor(i)
          const r = i === 0 || i === n - 1 ? 3.5 : 2.5
          return (
            <g key={`dot-${p.date}`}>
              <circle
                cx={cx}
                cy={yForCalls(p.calls)}
                r={r}
                fill={COLOR_CALLS}
              />
              <circle
                cx={cx}
                cy={yForCost(p.cost_cents)}
                r={r}
                fill={COLOR_COST}
              />
            </g>
          )
        })}
      </svg>

      {/* Legend */}
      <div className="flex items-center gap-5 mt-2 text-xs text-claude-muted">
        <span className="inline-flex items-center gap-1.5">
          <span
            className="inline-block w-3 h-0.5"
            style={{ backgroundColor: COLOR_CALLS }}
          />
          调用量
        </span>
        <span className="inline-flex items-center gap-1.5">
          <span
            className="inline-block w-3 h-0.5"
            style={{ backgroundColor: COLOR_COST }}
          />
          成本 (分)
        </span>
      </div>
    </div>
  )
}
