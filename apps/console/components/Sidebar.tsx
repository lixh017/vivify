'use client'

// Sidebar is the primary navigation for the console. We render
// nine items: six are active routes in the MVP, three are
// "Coming soon" placeholders that already exist as routes so
// future work (multi-tenant org, member management, billing)
// can land without a URL migration.
//
// The active item is computed from usePathname(); the active
// row gets a left coral border and a brighter label. Hovering
// an inactive item nudges the label brightness — minimal, no
// motion. Disabled (reserve) items render in muted-soft and
// are not clickable.

import Link from 'next/link'
import { usePathname } from 'next/navigation'

type Item = {
  href: string
  label: string
  reserve?: boolean
}

const ITEMS: Item[] = [
  { href: '/dashboard',       label: '总览' },
  { href: '/credentials',     label: '凭据' },
  { href: '/observability',   label: '观测' },
  { href: '/logs',            label: '日志' },
  { href: '/docs',            label: '文档' },
  { href: '/settings',        label: '设置' },
  { href: '/org',             label: '组织',     reserve: true },
  { href: '/members',         label: '成员',     reserve: true },
  { href: '/billing',         label: '计费' },
]

export function Sidebar() {
  const pathname = usePathname()
  return (
    <aside className="hidden md:flex md:flex-col w-56 shrink-0 bg-claude-surface-dark text-claude-on-dark">
      <div className="px-5 py-5 border-b border-claude-surface-dark-elevated">
        <div className="text-base font-semibold tracking-tight">OPC Console</div>
        <div className="text-xs text-claude-on-dark-soft mt-0.5">MCN 控制台</div>
      </div>
      <nav className="flex-1 py-3">
        {ITEMS.map((item) => {
          const active = pathname === item.href || pathname?.startsWith(item.href + '/')
          if (item.reserve) {
            return (
              <div
                key={item.href}
                aria-disabled
                className="block px-5 py-2 text-sm text-claude-on-dark-soft cursor-not-allowed select-none"
                title="Coming soon (reserve route)"
              >
                {item.label}
                <span className="ml-2 text-[10px] uppercase tracking-eyebrow text-claude-on-dark-soft/70">soon</span>
              </div>
            )
          }
          return (
            <Link
              key={item.href}
              href={item.href}
              className={
                'block px-5 py-2 text-sm border-l-2 transition-colors ' +
                (active
                  ? 'border-claude-coral text-claude-on-dark font-medium'
                  : 'border-transparent text-claude-on-dark-soft hover:text-claude-on-dark')
              }
            >
              {item.label}
            </Link>
          )
        })}
      </nav>
      <div className="px-5 py-3 text-[11px] text-claude-on-dark-soft border-t border-claude-surface-dark-elevated">
        v0.1.0 · single-user
      </div>
    </aside>
  )
}
