'use client'

// /billing is the MCN operator's monthly cost / usage
// surface. The page calls GET /api/billing/summary?month=
// for the headline KPIs and per-skill / per-provider
// breakdowns, plus a CSV export link for the underlying
// row data. We deliberately do NOT recompute cost from
// today's pricing — the historical call_log.cost_cents is
// the source of truth (the user's invoice is on historical
// cents, not on a recompute).

import { useEffect, useState } from 'react'
import { TopBar } from '@/components/TopBar'
import { api, ApiError } from '@opc/shared/api'
import Link from 'next/link'

interface SkillRow {
  skill: string
  calls: number
  cost_cents: number
}
interface ProviderRow {
  provider: string
  calls: number
  cost_cents: number
}
interface BillingSummary {
  month: string
  calls: number
  cost_cents: number
  days_elapsed: number
  days_in_month: number
  projection_cents: number
  by_skill: SkillRow[]
  by_provider: ProviderRow[]
}

function thisMonth(): string {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
}

function monthCents(cents: number): string {
  return `¥${(cents / 100).toFixed(2)}`
}

export default function BillingPage() {
  const [month, setMonth] = useState<string>(thisMonth())
  const [data, setData] = useState<BillingSummary | null>(null)
  const [loading, setLoading] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setErr(null)
    api.billing
      .summary(month)
      .then((d) => {
        if (!cancelled) setData(d as unknown as BillingSummary)
      })
      .catch((e: unknown) => {
        if (cancelled) return
        setErr(e instanceof ApiError ? e.message : String(e))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [month])

  return (
    <>
      <TopBar title="计费" />
      <div className="p-6 space-y-4 max-w-5xl">
        <div className="flex items-center justify-between">
          <p className="text-sm text-claude-muted">
            按月汇总。CSV 导出包含逐行明细。
          </p>
          <div className="flex items-center gap-3">
            <input
              type="month"
              value={month}
              onChange={(e) => setMonth(e.target.value)}
              className="px-3 py-1.5 border border-claude-hairline rounded-md text-sm focus:outline-none focus:border-claude-coral"
            />
            <a
              href={`/api/billing/export.csv?month=${month}`}
              className="text-sm text-claude-coral hover:underline"
            >
              导出 CSV
            </a>
          </div>
        </div>

        {err && (
          <div className="text-sm text-claude-error bg-claude-error/10 border border-claude-error/20 rounded-md p-3">
            {err}
          </div>
        )}

        {/* 4 KPI cards */}
        <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
          <Kpi
            label="本月调用"
            value={data ? data.calls.toLocaleString('zh-CN') : '—'}
            loading={loading}
          />
          <Kpi
            label="本月成本"
            value={data ? monthCents(data.cost_cents) : '—'}
            loading={loading}
            accent="coral"
          />
          <Kpi
            label="已过天数"
            value={data ? `${data.days_elapsed} / ${data.days_in_month}` : '—'}
            loading={loading}
            hint="用于预测"
          />
          <Kpi
            label="预测本月总成本"
            value={data ? monthCents(data.projection_cents) : '—'}
            loading={loading}
            hint="线性外推"
          />
        </div>

        {/* Two tables: by_skill + by_provider */}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          <Card title="按 skill 拆分">
            {!data || data.by_skill.length === 0 ? (
              <Empty loading={loading} />
            ) : (
              <Table
                headers={['Skill', '调用', '成本']}
                rows={data.by_skill.map((r) => [
                  <SkillCell skill={r.skill} />,
                  <span className="font-mono tabular-nums">{r.calls.toLocaleString('zh-CN')}</span>,
                  <span className="font-mono tabular-nums text-claude-ink">
                    {monthCents(r.cost_cents)}
                  </span>,
                ])}
              />
            )}
          </Card>
          <Card title="按 provider 拆分">
            {!data || data.by_provider.length === 0 ? (
              <Empty loading={loading} />
            ) : (
              <Table
                headers={['Provider', '调用', '成本']}
                rows={data.by_provider.map((r) => [
                  <span className="font-mono text-xs px-1.5 py-0.5 rounded bg-claude-surface-soft text-claude-body">
                    {r.provider}
                  </span>,
                  <span className="font-mono tabular-nums">{r.calls.toLocaleString('zh-CN')}</span>,
                  <span className="font-mono tabular-nums text-claude-ink">
                    {monthCents(r.cost_cents)}
                  </span>,
                ])}
              />
            )}
          </Card>
        </div>

        <p className="text-[11px] text-claude-muted-soft">
          注：成本以 call_log 表里 stamped 的 cost_cents 为准（不可重算）。
          即使配置表里调整了价格，历史数据保持不变；当前价格只影响未来调用。
          {' '}
          <Link href="/logs" className="text-claude-coral hover:underline">
            查看逐行日志
          </Link>
        </p>
      </div>
    </>
  )
}

function Kpi({
  label,
  value,
  loading,
  hint,
  accent,
}: {
  label: string
  value: string
  loading: boolean
  hint?: string
  accent?: 'coral' | 'plain'
}) {
  const valueClass =
    accent === 'coral' ? 'text-claude-coral' : 'text-claude-ink'
  return (
    <div className="bg-white border border-claude-hairline rounded-lg p-5 shadow-claude-soft">
      <div className="text-xs uppercase tracking-eyebrow text-claude-muted">{label}</div>
      <div className="mt-2 h-7 flex items-center">
        {loading ? (
          <div
            className="h-3 bg-claude-surface-card rounded animate-pulse"
            style={{ width: '60%' }}
          />
        ) : (
          <div className={`font-mono text-2xl tabular-nums ${valueClass}`}>{value}</div>
        )}
      </div>
      {hint && <div className="mt-1 text-[11px] text-claude-muted-soft">{hint}</div>}
    </div>
  )
}

function Card({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="bg-white border border-claude-hairline rounded-lg p-5 shadow-claude-soft">
      <h3 className="text-sm font-medium text-claude-ink mb-3">{title}</h3>
      {children}
    </div>
  )
}

function Empty({ loading }: { loading: boolean }) {
  return (
    <div className="text-sm text-claude-muted-soft py-2">
      {loading ? '加载中…' : '该月还没有调用记录'}
    </div>
  )
}

function Table({
  headers,
  rows,
}: {
  headers: string[]
  rows: React.ReactNode[][]
}) {
  return (
    <table className="w-full text-sm">
      <thead className="text-[11px] uppercase tracking-eyebrow text-claude-muted">
        <tr className="border-b border-claude-hairline">
          {headers.map((h) => (
            <th key={h} className="py-2 text-left font-medium">
              {h}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {rows.map((r, i) => (
          <tr key={i} className="border-b border-claude-hairline-soft last:border-0">
            {r.map((c, j) => (
              <td key={j} className="py-2 text-claude-body">
                {c}
              </td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  )
}

function SkillCell({ skill }: { skill: string }) {
  return (
    <span className="font-mono text-xs px-1.5 py-0.5 rounded bg-claude-surface-soft text-claude-body">
      {skill}
    </span>
  )
}
