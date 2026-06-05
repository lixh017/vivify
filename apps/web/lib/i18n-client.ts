'use client'

// Client-only i18n helpers. Lives in its own module so the React
// imports (useState, useEffect) stay out of the server bundle and
// don't trigger the "use client boundary" error when layout.tsx
// imports the isomorphic i18n module.
//
// `t()` is imported from `i18n-lookup.ts` (pure, no next/headers)
// rather than from `i18n.ts` (which would drag `cookies()` into the
// client bundle). The pure module is the lowest common denominator
// for both sides.

import { useEffect, useState, useCallback } from 'react'
import {
  LOCALE_COOKIE,
  DEFAULT_LOCALE,
  type Locale,
} from './i18n-types'
import { t as tImpl } from './i18n-lookup'

// parseLocale is duplicated from i18n.ts rather than imported —
// importing from i18n.ts would drag `next/headers` into the client
// bundle (Next.js hoists static analysis on the import graph). The
// duplication is two lines and the alternative is more complex than
// the cost.
function parseLocale(raw: string | null): Locale {
  if (raw === 'zh' || raw === 'en') return raw
  return DEFAULT_LOCALE
}

/**
 * useT returns a `t` function bound to the current locale. The
 * locale is read from `document.cookie` on mount and re-read on
 * window focus (the simplest way to pick up a change made by
 * another tab or by the switcher after a redirect).
 *
 * Components should call `const t = useT()` once at the top, then
 * use `t(key)` throughout the render. The returned function is
 * stable per-locale (useCallback keyed on the locale) so it is safe
 * to pass into memoized children.
 */
export function useT(): (key: string) => string {
  const [locale, setLocale] = useState<Locale>(DEFAULT_LOCALE)

  useEffect(() => {
    function read() {
      const match = document.cookie
        .split('; ')
        .find((row) => row.startsWith(`${LOCALE_COOKIE}=`))
      const raw = match ? decodeURIComponent(match.split('=')[1]) : null
      setLocale(parseLocale(raw))
    }
    read()
    window.addEventListener('focus', read)
    return () => window.removeEventListener('focus', read)
  }, [])

  return useCallback((key: string) => tImpl(key, locale), [locale])
}

/**
 * setLocaleCookie writes the locale cookie on the client. Used by
 * the locale switcher before reloading the page.
 */
export function setLocaleCookie(locale: Locale): void {
  if (typeof document === 'undefined') return
  // 1 year, path=/, SameSite=Lax. We do NOT set Secure in dev because
  // http://localhost would drop the cookie. The middleware in
  // production sits behind HTTPS so the browser will treat it as
  // secure-by-default.
  const oneYear = 60 * 60 * 24 * 365
  document.cookie = `${LOCALE_COOKIE}=${encodeURIComponent(locale)}; path=/; max-age=${oneYear}; SameSite=Lax`
}
