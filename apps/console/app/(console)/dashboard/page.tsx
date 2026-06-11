'use client'

// /dashboard is the MCN operator's daily landing page. We
// pull the same ObservabilitySummary that /observability uses
// and surface a "today" digest: today's calls, success rate,
// cost, plus the top-3 skills by call count. The numbers come
// from the call_log table that the call_log middleware fills
// on every protected request, so what the operator sees here
// is exactly what /api/logs would return.

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { TopBar } from '@/components/TopBar'
import { useObservabilitySummary } from '@/lib/use-observability'
import { api, ApiError } from '@opc/shared/api'
import type { CallLog } from '@opc/shared/types'

// "Today" pre-formatted copy for the three KPI tiles. Kept
// in the page (not the hook) because future KPI tiles
// (e.g. "MTD cost", "skill count") will have their own
// labels and we don't want the hook to grow a render surface.
const TODAY_CARDS: { key: 'calls' | 'success' | 'cost'; label: string; hint: string }[] = [
  { key: 'calls', label: '今日调用', hint: '今日 00:00 起 (UTC)' },
  { key: 'success', label: '成功率', hint: '成功 / 全部' },
  { key: 'cost', label: '总成本', hint: '单位: 分 (CNY)' },
]

export default function DashboardPage() {
  const { data, loading, error, range, setRange, refresh } = useObservabilitySummary('7d')
  const [latest, setLatest] = useState<CallLog[]>([])
  const [latestErr, setLatestErr] = useState<string | null>(null)

  useEffect(() => {
    // Pull the 3 most recent call_log rows so the operator
    // sees the most recent activity in one glance without
    // jumping to /logs.
    let cancelled = false
    api.logs
      .list({ limit: 3, offset: 0 })
      .then((r) => {
        if (!cancelled) setLatest(r.items || [])
      })
      .catch((e: unknown) => {
        if (cancelled) return
        if (e instanceof ApiError) setLatestErr(e.message)
        else setLatestErr(String(e))
      })
    return () => {
      cancelled = true
    }
  }, [range])

  const today = data?.today
  const cards = {
    calls: today ? today.calls.toLocaleString('zh-CN') : '—',
    success: today
      ? `${(today.success_rate * 100).toFixed(1)}%`
      : '—',
    cost: today
      ? `¥${(today.cost_cents / 100).toFixed(2)}`
      : '—',
  }

  // Top 3 skills by call count. by_skill is already sorted
  // server-side; we just take the first 3.
  const topSkills = (data?.by_skill || []).slice(0, 3)

  return (
    <>
      <TopBar title="总览" />
      <div className="p-6 space-y-4 max-w-5xl">
        {/* Range selector (mirrors /observability) + refresh */}
        <div className="flex items-center justify-between">
          <p className="text-sm text-claude-muted">
            MCN 操作员的今日视图。详细数据见{' '}
            <Link href="/observability" className="text-claude-coral hover:underline">
              调用观测
            </Link>
            {' '}和{' '}
            <Link href="/logs" className="text-claude-coral hover:underline">
              调用日志
            </Link>
            。
          </p>
          <RangeSelector value={range} onChange={setRange} />
        </div>

        {/* KPI tiles — 3 cards, mirrors /observability */}
        <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
          {TODAY_CARDS.map((c) => (
            <SummaryCard
              key={c.key}
              label={c.label}
              hint={c.hint}
              value={cards[c.key]}
              loading={loading}
            />
          ))}
        </div>

        {/* Top skills + recent activity — two columns side by side */}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          <Card title="Top skills (按调用次数)">
            {topSkills.length === 0 ? (
              <div className="text-sm text-claude-muted-soft py-2">
                {loading ? '加载中…' : '该时间范围内还没有 skill 调用'}
              </div>
            ) : (
              <table className="w-full text-sm">
                <thead className="text-[11px] uppercase tracking-eyebrow text-claude-muted">
                  <tr className="border-b border-claude-hairline">
                    <th className="py-2 text-left font-medium">Skill</th>
                    <th className="py-2 text-right font-medium">调用</th>
                    <th className="py-2 text-right font-medium">成功率</th>
                    <th className="py-2 text-right font-medium">成本</th>
                  </tr>
                </thead>
                <tbody>
                  {topSkills.map((r) => (
                    <tr key={r.skill} className="border-b border-claude-hairline-soft last:border-0">
                      <td className="py-2">
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
                      <td className="py-2 text-right font-mono tabular-nums text-claude-ink">
                        ¥{(r.cost_cents / 100).toFixed(2)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </Card>

          <Card title="最近 3 条调用">
            {latestErr ? (
              <div className="text-sm text-claude-error py-2">{latestErr}</div>
            ) : latest.length === 0 ? (
              <div className="text-sm text-claude-muted-soft py-2">还没有调用记录</div>
            ) : (
              <ul className="space-y-2">
                {latest.map((r) => (
                  <li
                    key={r.id}
                    className="flex items-center justify-between text-sm border-b border-claude-hairline-soft last:border-0 pb-2 last:pb-0"
                  >
                    <div className="min-w-0 flex-1">
                      <div className="font-mono text-xs text-claude-ink">{r.skill}</div>
                      <div className="text-[11px] text-claude-muted-soft">
                        {new Date(r.created_at).toLocaleString('zh-CN')}
                      </div>
                    </div>
                    <div className="text-right ml-3">
                      <div className="font-mono text-xs text-claude-ink">
                        ¥{(r.cost_cents / 100).toFixed(2)}
                      </div>
                      <div className="text-[10px] text-claude-muted-soft">
                        {r.status === 0 ? 'OK' : r.status === 1 ? '4xx' : r.status === 2 ? '5xx' : 'timeout'}
                      </div>
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </Card>
        </div>

        {/* Quick links */}
        <Card title="快速入口">
          <div className="grid grid-cols-2 md:grid-cols-3 gap-2 text-sm">
            <QuickLink href="/credentials" label="凭据管理" desc="API keys / 加密存储" />
            <QuickLink href="/observability" label="调用观测" desc="按 skill × 租户 × 时间" />
            <QuickLink href="/logs" label="调用日志" desc="全量 + CSV 导出" />
            <QuickLink href="/billing" label="计费" desc="按月汇总 + CSV 导出" />
            <QuickLink href="/sandbox" label="沙盒" desc="实时调一次 skill" />
            <QuickLink href="/docs" label="文档" desc="skill schema + 示例" />
          </div>
        </Card>

        {error && (
          <div className="text-sm text-claude-error">observability 加载失败: {error}</div>
        )}
        {!loading && (
          <div className="text-right">
            <button
              type="button"
              onClick={refresh}
              className="text-xs text-claude-muted hover:text-claude-ink"
            >
              刷新
            </button>
          </div>
        )}
      </div>
    </>
  )
}

function RangeSelector({
  value,
  onChange,
}: {
  value: '7d' | '30d' | 'today'
  onChange: (next: '7d' | '30d' | 'today') => void
}) {
  const opts: { value: '7d' | '30d' | 'today'; label: string }[] = [
    { value: '7d', label: '近 7 天' },
    { value: '30d', label: '近 30 天' },
    { value: 'today', label: '今天' },
  ]
  return (
    <div className="inline-flex p-0.5 bg-claude-surface-card rounded-md border border-claude-hairline-soft">
      {opts.map((o) => {
        const active = o.value === value
        return (
          <button
            key={o.value}
            type="button"
            onClick={() => onChange(o.value)}
            className={
              'px-3 py-1 text-xs rounded transition-colors ' +
              (active
                ? 'bg-white text-claude-ink shadow-claude-soft'
                : 'text-claude-muted hover:text-claude-ink')
            }
          >
            {o.label}
          </button>
        )
      })}
    </div>
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
      <div className="text-xs uppercase tracking-eyebrow text-claude-muted">{label}</div>
      <div className="mt-2 h-7 flex items-center">
        {loading ? (
          <div className="h-3 bg-claude-surface-card rounded animate-pulse" style={{ width: '60%' }} />
        ) : (
          <div className="font-mono text-2xl text-claude-ink tabular-nums">{value}</div>
        )}
      </div>
      <div className="mt-1 text-[11px] text-claude-muted-soft">{hint}</div>
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

function QuickLink({ href, label, desc }: { href: string; label: string; desc: string }) {
  return (
    <Link
      href={href}
      className="block px-3 py-2 rounded-md border border-claude-hairline-soft hover:border-claude-coral/40 hover:bg-claude-surface-soft transition-colors"
    >
      <div className="text-sm text-claude-ink font-medium">{label}</div>
      <div className="text-[11px] text-claude-muted-soft">{desc}</div>
    </Link>
  )
}
