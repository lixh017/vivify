// Next.js middleware — runs on every request before the page or
// API handler. Our job here is tiny: ensure every visitor has an
// `opc_locale` cookie. We default to `zh` (the source language) so
// the first render is never an English-only fallback when the user
// has not expressed a preference.
//
// We deliberately do NOT redirect based on the cookie (no /en or /zh
// path segment in Phase 1). The cookie just selects which catalog
// strings render — the URL stays the same. This keeps the rest of
// the app free of locale-aware routing logic, at the cost of one
// extra cookie round-trip on the first visit.
//
// Edge runtime constraints respected:
//   - No Node APIs (fs, child_process, etc.)
//   - No `next/headers` cookies() helper (that's for server components
//     and route handlers; middleware reads from `request.cookies`).
//   - We only use `NextResponse.cookies.set`, which is allowed.

import { NextRequest, NextResponse } from 'next/server'
import { LOCALE_COOKIE, DEFAULT_LOCALE, type Locale } from './lib/i18n'

function isLocale(value: string | undefined): value is Locale {
  return value === 'zh' || value === 'en'
}

export function middleware(request: NextRequest): NextResponse {
  const existing = request.cookies.get(LOCALE_COOKIE)?.value
  if (isLocale(existing)) {
    // Visitor has already chosen a language; pass through untouched.
    return NextResponse.next()
  }

  // No cookie (or an unrecognized value). Seed it with the default
  // and pass through. Setting the cookie on the response means the
  // browser will store it for subsequent requests, so the next page
  // load doesn't go through this branch.
  const response = NextResponse.next()
  response.cookies.set({
    name: LOCALE_COOKIE,
    value: DEFAULT_LOCALE,
    path: '/',
    // Short max-age for the seed cookie: if the user picks a real
    // locale via the switcher it overrides this anyway. A year is
    // fine — it just means "until the user explicitly changes it."
    maxAge: 60 * 60 * 24 * 365,
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
