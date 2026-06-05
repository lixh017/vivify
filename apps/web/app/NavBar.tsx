'use client'

import { useState, useEffect, useTransition } from 'react'
import { usePathname, useRouter } from 'next/navigation'
import { api, ApiError } from '@/lib/api'
import type { AuthUser } from '@/lib/types'
import {
  LOCALE_LABELS,
  SUPPORTED_LOCALES,
  type Locale,
} from '@/lib/i18n-types'

interface NavLink {
  href: string
  label: string
  icon: string
}

const NAV_LINKS: NavLink[] = [
  { href: '/topics', label: '选题', icon: '📋' },
  { href: '/scripts', label: '脚本', icon: '📝' },
  { href: '/calendar', label: '日历', icon: '📅' },
  { href: '/dashboard', label: '表现', icon: '📊' },
  { href: '/knowledge', label: '知识库', icon: '📚' },
]

// The /login page renders inside this layout but never wants a
// "loading user" placeholder — the user is by definition unauthenticated
// there. We bail out of the /api/auth/me probe on that path so the
// navbar slot stays empty.
const PUBLIC_PATHS = new Set<string>(['/login'])

// Tri-state user resolution:
//   - 'loading' — /api/auth/me in flight; render a placeholder so the
//     header height does not shift when the response arrives;
//   - AuthUser  — authenticated; render greeting + logout;
//   - null      — explicit unauthenticated; render nothing (the
//     protected pages handle their own redirect-to-login on 401).
//
// We model loading explicitly (instead of `user | null` plus a separate
// boolean) so the JSX has a single discriminator and we cannot
// accidentally render the unauthenticated state for the few hundred ms
// it takes /api/auth/me to come back after a hard refresh — that was
// the UX gap flagged in QA.
type UserState = 'loading' | AuthUser | null

export default function NavBar() {
  const [open, setOpen] = useState(false)
  const router = useRouter()
  const pathname = usePathname()
  const [loggingOut, setLoggingOut] = useState(false)
  const [user, setUser] = useState<UserState>('loading')
  // startTransition wraps the locale switch so React can prioritize
  // a fresh render in the new language. We don't need isPending
  // here — a spinner would just flicker before the page reloads —
  // but the transition is still useful for keeping the click
  // handler low-priority during the brief moment before reload.
  const [, startTransition] = useTransition()

  // Resolve the current user once on mount, and re-resolve when the
  // route changes (so a fresh login on /login transitions the navbar
  // to the greeting without a hard reload). On the /login page itself,
  // we skip the call entirely.
  useEffect(() => {
    if (pathname && PUBLIC_PATHS.has(pathname)) {
      setUser(null)
      return
    }
    let cancelled = false
    setUser('loading')
    api.auth
      .me()
      .then((res) => {
        if (!cancelled) setUser(res.user)
      })
      .catch((err: unknown) => {
        if (cancelled) return
        // A 401 is the expected "not logged in" branch; any other
        // error is a transient API failure — collapse both to "no
        // user" so the navbar does not get stuck on "loading…".
        if (err instanceof ApiError || err instanceof Error) {
          setUser(null)
        } else {
          setUser(null)
        }
      })
    return () => {
      cancelled = true
    }
  }, [pathname])

  async function handleLogout() {
    setLoggingOut(true)
    try {
      await api.auth.logout()
    } catch {
      // Even if the server says "not authenticated" (the cookie was
      // already gone), the UX result is the same: send the user to
      // /login. We swallow the error so the button is non-blocking.
    } finally {
      setLoggingOut(false)
      setUser(null)
      router.push('/login')
    }
  }

  // Close the mobile menu when the viewport grows past the md breakpoint
  // or when the user starts navigating. The resize listener is throttled
  // by the fact that it only does work on a real width change.
  useEffect(() => {
    function handleResize() {
      if (window.innerWidth >= 768 && open) {
        setOpen(false)
      }
    }
    window.addEventListener('resize', handleResize)
    return () => window.removeEventListener('resize', handleResize)
  }, [open])

  // Read the active locale directly from the cookie at render time
  // (not via useT) so the switcher always highlights the live value
  // even before useT has re-run on focus. This is cheap: it's a
  // single document.cookie scan.
  function readActiveLocale(): Locale {
    if (typeof document === 'undefined') return 'zh'
    const match = document.cookie
      .split('; ')
      .find((row) => row.startsWith('opc_locale='))
    const raw = match ? decodeURIComponent(match.split('=')[1]) : null
    if (raw === 'zh' || raw === 'en') return raw
    return 'zh'
  }

  const [activeLocale, setActiveLocale] = useState<Locale>('zh')
  useEffect(() => {
    setActiveLocale(readActiveLocale())
  }, [])

  // handleLocaleChange persists the new locale to the cookie and
  // forces a hard reload. The reload is intentional: server
  // components re-render in the new locale, and the layout's
  // <html lang="..."> attribute gets the right value. A soft
  // client-side swap would leave server-rendered chrome in the
  // old language.
  function handleLocaleChange(next: Locale) {
    if (next === activeLocale) return
    setActiveLocale(next)
    startTransition(() => {
      const oneYear = 60 * 60 * 24 * 365
      document.cookie = `opc_locale=${encodeURIComponent(next)}; path=/; max-age=${oneYear}; SameSite=Lax`
      // Use location.reload rather than router.refresh: the entire
      // app tree (including the layout) needs to re-evaluate the
      // cookie, and a fresh load is the simplest way to do that.
      window.location.reload()
    })
  }

  const isAuthenticated = user !== 'loading' && user !== null

  return (
    <nav className="bg-claude-canvas border-b border-claude-hairline sticky top-0 z-30">
      <div className="container mx-auto px-3 md:px-6">
        <div className="flex items-center justify-between h-14 md:h-16">
          <a
            href="/"
            className="text-lg md:text-xl font-semibold text-claude-ink tracking-tight flex items-center gap-2"
          >
            <span aria-hidden="true">🐼</span>
            <span>OPC</span>
          </a>

          {/* Desktop nav — horizontal, hidden on mobile */}
          <div className="hidden md:flex gap-6 text-sm font-medium">
            {NAV_LINKS.map((link) => {
              const isActive = pathname === link.href
              return (
                <a
                  key={link.href}
                  href={link.href}
                  className={
                    isActive
                      ? 'text-claude-coral transition-colors'
                      : 'text-claude-body hover:text-claude-coral transition-colors'
                  }
                >
                  {link.icon} {link.label}
                </a>
              )
            })}
          </div>

          {/* User slot — greeting + logout, or a CTA to log in.
              Placeholder keeps the header height stable so layout
              does not jump on hydration. */}
          <div className="hidden md:flex items-center gap-3">
            <LocaleSwitcher active={activeLocale} onChange={handleLocaleChange} />
            <UserBadge state={user} />
            {user === 'loading' ? null : isAuthenticated ? (
              <button
                type="button"
                onClick={handleLogout}
                disabled={loggingOut}
                data-testid="btn-logout"
                aria-label="登出"
                className="text-sm px-3 py-1.5 rounded-md text-claude-ink hover:bg-claude-surface-card disabled:opacity-50 transition-colors"
              >
                {loggingOut ? '登出中…' : '登出'}
              </button>
            ) : (
              <a
                href="/login"
                data-testid="btn-login"
                className="bg-claude-coral text-claude-on-primary px-4 py-2 rounded-md text-sm font-medium hover:bg-claude-coral-active focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-claude-ink focus-visible:ring-offset-2 active:translate-y-px transition-colors"
              >
                登录
              </a>
            )}
          </div>

          <button
            type="button"
            aria-label="切换导航菜单"
            aria-expanded={open}
            onClick={() => setOpen((v) => !v)}
            className="md:hidden inline-flex items-center justify-center p-2 rounded-md text-claude-ink hover:bg-claude-surface-card transition-colors"
          >
            {open ? (
              <svg
                xmlns="http://www.w3.org/2000/svg"
                className="h-6 w-6"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                aria-hidden="true"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M6 18L18 6M6 6l12 12"
                />
              </svg>
            ) : (
              <svg
                xmlns="http://www.w3.org/2000/svg"
                className="h-6 w-6"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                aria-hidden="true"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M4 6h16M4 12h16M4 18h16"
                />
              </svg>
            )}
          </button>
        </div>

        {/* Mobile drawer */}
        {open && (
          <div className="md:hidden border-t border-claude-hairline py-2 space-y-1">
            <div className="px-3 py-2">
              <UserBadge state={user} />
            </div>
            {NAV_LINKS.map((link) => (
              <a
                key={link.href}
                href={link.href}
                onClick={() => setOpen(false)}
                className="block px-3 py-2 rounded-md text-base font-medium text-claude-ink hover:bg-claude-surface-card transition-colors"
              >
                {link.icon} {link.label}
              </a>
            ))}
            <div className="px-3 py-2">
              <LocaleSwitcher active={activeLocale} onChange={handleLocaleChange} />
            </div>
            {isAuthenticated ? (
              <button
                type="button"
                onClick={handleLogout}
                disabled={loggingOut}
                data-testid="btn-logout-mobile"
                className="w-full text-left px-3 py-2 rounded-md text-base font-medium text-claude-ink hover:bg-claude-surface-card disabled:opacity-50 transition-colors"
              >
                {loggingOut ? '登出中…' : '登出'}
              </button>
            ) : user !== 'loading' ? (
              <a
                href="/login"
                data-testid="btn-login-mobile"
                className="block px-3 py-2 rounded-md text-base font-medium bg-claude-coral text-claude-on-primary hover:bg-claude-coral-active focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-claude-ink focus-visible:ring-offset-2 active:translate-y-px transition-colors"
              >
                登录
              </a>
            ) : null}
          </div>
        )}
      </div>
    </nav>
  )
}

// UserBadge owns the loading/user/anon rendering. Extracting it as a
// pure presentational component keeps the discriminator in one place
// so the desktop and mobile slots stay in lock-step.
function UserBadge({ state }: { state: UserState }) {
  if (state === 'loading') {
    return (
      <span
        aria-busy="true"
        data-testid="navbar-user-loading"
        className="inline-block h-4 w-20 rounded bg-claude-surface-card animate-pulse"
      />
    )
  }
  if (state === null) {
    // Anonymous: render nothing. The protected pages handle the
    // redirect-to-login flow themselves on a 401.
    return null
  }
  // Authenticated. Show name first (the personal handle) and tuck
  // the email into the title for tooltip-on-hover so the header
  // stays compact on narrow viewports.
  return (
    <span
      data-testid="navbar-user"
      title={state.email}
      className="text-sm text-claude-ink max-w-[12rem] truncate"
    >
      {state.name || state.email}
    </span>
  )
}

// LocaleSwitcher is a two-button segment (`中` / `EN`) that writes
// the opc_locale cookie and reloads the page. The active locale is
// highlighted in coral so the current pick is obvious at a glance.
//
// The switcher intentionally has no concept of "open" or "loading":
// the click handler does its work, then a full reload takes over.
// Showing a spinner would just flicker for a few hundred ms before
// the new page paints, which is uglier than the brief unresponsive
// state. The `aria-pressed` on each button is the right ARIA pattern
// for a mutually-exclusive toggle group.
function LocaleSwitcher({
  active,
  onChange,
}: {
  active: Locale
  onChange: (next: Locale) => void
}) {
  return (
    <div
      role="group"
      aria-label="Language"
      data-testid="locale-switcher"
      className="inline-flex rounded-md border border-claude-hairline overflow-hidden"
    >
      {SUPPORTED_LOCALES.map((loc, i) => {
        const isActive = loc === active
        return (
          <button
            key={loc}
            type="button"
            aria-pressed={isActive}
            aria-label={loc === 'zh' ? 'Switch to Chinese' : 'Switch to English'}
            onClick={() => onChange(loc)}
            data-testid={`locale-btn-${loc}`}
            className={
              'px-2 py-1 text-xs md:text-sm font-medium transition-colors ' +
              (i > 0 ? 'border-l border-claude-hairline ' : '') +
              (isActive
                ? 'bg-claude-coral text-claude-on-primary'
                : 'bg-claude-canvas text-claude-body hover:bg-claude-surface-soft')
            }
          >
            {LOCALE_LABELS[loc]}
          </button>
        )
      })}
    </div>
  )
}
