// Next.js middleware — runs on every request before the page or
// API handler. Two responsibilities:
//
//   1. Auth gate: any path under the protected surfaces
//      (/dashboard, /topics, /scripts, /calendar, /knowledge,
//      /content-items) requires an `opc_session` cookie. A logged-out
//      visitor is bounced to /login?next=<original-path> so the
//      login page can send them back after auth. Without this gate
//      the page renders a perpetual "加载中..." spinner (it waits
//      on /api/auth/me → 401 → null → still no redirect).
//
//   2. Locale seed: ensure every visitor has an `opc_locale` cookie.
//      We default to `zh` (the source language) so the first render
//      is never an English-only fallback when the user has not
//      expressed a preference. We deliberately do NOT redirect based
//      on the cookie (no /en or /zh path segment in Phase 1). The
//      cookie just selects which catalog strings render — the URL
//      stays the same.
//
// Edge runtime constraints respected:
//   - No Node APIs (fs, child_process, etc.)
//   - No `next/headers` cookies() helper (that's for server components
//     and route handlers; middleware reads from `request.cookies`).
//   - We only use `NextResponse.cookies.set` and `NextResponse.redirect`,
//     both of which are allowed.
//
// Note on auth accuracy: middleware runs in the edge runtime and
// cannot call the Go API to validate the session. We trust the
// presence of the `opc_session` cookie as a "probably logged in"
// signal. If the cookie is stale/expired, the protected page's own
// data fetch will 401 and surface the error state to the user —
// the page is not silently blank anymore, which was the original
// problem.

import { NextRequest, NextResponse } from 'next/server'
import {
  LOCALE_COOKIE,
  DEFAULT_LOCALE,
  LOCALE_COOKIE_MAX_AGE,
  LOCALE_COOKIE_PATH,
  type Locale,
} from '@opc/shared/i18n-types'

// SESSION_COOKIE is the cookie the Go backend sets on login/register.
// It must stay in sync with apps/web/lib/auth.ts (and the Go
// session middleware). We declare it as a string literal here so the
// middleware stays self-contained — it can't import the auth lib,
// which uses fetch() and wouldn't run in the edge runtime.
const SESSION_COOKIE = 'opc_session'

// PROTECTED_PREFIXES is the closed list of auth-gated surfaces. The
// trailing-slash prefix match (e.g. "/dashboard" covers
// "/dashboard", "/dashboard/foo", "/dashboard?x=1") is handled by
// `pathname === p || pathname.startsWith(p + '/')` below, so we do
// not need the matcher config to enumerate them.
const PROTECTED_PREFIXES = [
  '/dashboard',
  '/topics',
  '/scripts',
  '/calendar',
  '/knowledge',
  '/content-items',
] as const

// PUBLIC_AUTH_PATHS are always reachable without a session, even
// though they share the same matcher as the rest of the site. The
// login page in particular must be reachable from the auth-gate
// redirect itself — adding it to the protected list would create
// a redirect loop.
const PUBLIC_AUTH_PATHS = new Set<string>(['/login', '/register'])

function isLocale(value: string | undefined): value is Locale {
  return value === 'zh' || value === 'en'
}

function isProtectedPath(pathname: string): boolean {
  return PROTECTED_PREFIXES.some(
    (p) => pathname === p || pathname.startsWith(p + '/'),
  )
}

export function middleware(request: NextRequest): NextResponse {
  const { pathname } = request.nextUrl

  // 1. Auth gate — runs first so a logged-out visitor never even
  //    reaches the locale seed on a protected page. (The locale
  //    seed is a no-op response on the way out, so order doesn't
  //    matter functionally, but doing auth first keeps the redirect
  //    response free of unnecessary cookie writes.)
  if (isProtectedPath(pathname) && !PUBLIC_AUTH_PATHS.has(pathname)) {
    const session = request.cookies.get(SESSION_COOKIE)?.value
    if (!session || session.length === 0) {
      // Build /login?next=<original path + search>. We carry the
      // search string so query-driven deep links (e.g. /topics?tab=foo)
      // round-trip back to the same view after login.
      const loginUrl = request.nextUrl.clone()
      loginUrl.pathname = '/login'
      loginUrl.search = ''
      const next = pathname + (request.nextUrl.search || '')
      loginUrl.searchParams.set('next', next)
      return NextResponse.redirect(loginUrl)
    }
  }

  // 2. Locale seed — same behavior as before the gate was added.
  const existing = request.cookies.get(LOCALE_COOKIE)?.value
  if (isLocale(existing)) {
    return NextResponse.next()
  }

  const response = NextResponse.next()
  response.cookies.set({
    name: LOCALE_COOKIE,
    value: DEFAULT_LOCALE,
    path: LOCALE_COOKIE_PATH,
    maxAge: LOCALE_COOKIE_MAX_AGE,
    sameSite: 'lax',
  })
  return response
}

// Skip middleware on Next's internal asset routes and the public
// files. We only want to seed the cookie on actual page/API hits.
export const config = {
  matcher: [
    // Run on everything except: _next/*, favicon, and files with
    // common static extensions. The negative-lookahead is the
    // idiomatic way to express this in Next's matcher syntax.
    '/((?!_next/static|_next/image|favicon.ico|.*\\.(?:svg|png|jpg|jpeg|gif|webp|css|js)$).*)',
  ],
}
