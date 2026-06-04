'use client'

import { useState, useEffect } from 'react'
import { usePathname, useRouter } from 'next/navigation'
import { api, ApiError } from '@/lib/api'
import type { AuthUser } from '@/lib/types'

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
                className="bg-claude-coral text-white px-4 py-2 rounded-md text-sm font-medium hover:bg-claude-coral-active transition-colors"
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
                className="block px-3 py-2 rounded-md text-base font-medium bg-claude-coral text-white hover:bg-claude-coral-active transition-colors"
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
