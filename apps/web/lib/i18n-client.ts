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
  LOCALE_COOKIE_MAX_AGE,
  LOCALE_COOKIE_PATH,
  type Locale,
  type TranslationKey,
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
 * The returned function supports a small `{var}` interpolation
 * template, e.g. `t('register.password_hint', { n: 8 })` returns
 * "At least 8 characters" (en) / "至少 8 位" (zh). Callers that
 * need the plain key back should use the underlying `t` from
 * `i18n-lookup.ts` directly.
 *
 * The translator is stable per-locale (useCallback keyed on the
 * locale) so it is safe to pass into memoized children.
 *
 * Contract: the locale switcher writes the cookie and then calls
 * `window.location.reload()`. Because we re-read on mount, a hard
 * reload is the canonical refresh path. The `focus` listener is
 * belt-and-suspenders for cross-tab edits where no reload happens.
 */
export function useT(): (
  key: TranslationKey | string,
  vars?: Record<string, string | number>,
) => string {
  const [locale, setLocale] = useState<Locale>(DEFAULT_LOCALE)

  useEffect(() => {
    function read() {
      setLocale(readLocaleFromCookie())
    }
    read()
    window.addEventListener('focus', read)
    return () => window.removeEventListener('focus', read)
  }, [])

  return useCallback(
    (key, vars) => {
      const raw = tImpl(key, locale)
      if (!vars) return raw
      return interpolate(raw, vars)
    },
    [locale],
  )
}

/**
 * setLocaleCookie writes the locale cookie on the client. Used by
 * the locale switcher before reloading the page. Centralised here
 * (and the server-side mirror lives in /api/locale) so the
 * attributes (maxAge, path, SameSite) stay in lockstep.
 */
export function setLocaleCookie(locale: Locale): void {
  if (typeof document === 'undefined') return
  document.cookie = `${LOCALE_COOKIE}=${encodeURIComponent(locale)}; path=${LOCALE_COOKIE_PATH}; max-age=${LOCALE_COOKIE_MAX_AGE}; SameSite=Lax`
}

/**
 * readLocaleFromCookie is a no-side-effect helper that scans
 * `document.cookie` for the locale cookie and parses it. Useful
 * for components that need to know the active locale without
 * subscribing to focus changes (e.g. the switcher's active highlight).
 */
export function readLocaleFromCookie(): Locale {
  if (typeof document === 'undefined') return DEFAULT_LOCALE
  const match = document.cookie
    .split('; ')
    .find((row) => row.startsWith(`${LOCALE_COOKIE}=`))
  const raw = match ? decodeURIComponent(match.split('=')[1]) : null
  return parseLocale(raw)
}

// interpolate replaces `{name}` tokens in `template` with values
// from `vars`. Unknown placeholders are left untouched so a typo
// in the catalog is obvious in the rendered output.
function interpolate(
  template: string,
  vars: Record<string, string | number>,
): string {
  return template.replace(/\{(\w+)\}/g, (match, name: string) => {
    if (Object.prototype.hasOwnProperty.call(vars, name)) {
      return String(vars[name])
    }
    return match
  })
}
