// Server-side i18n. Adds `getLocale()` (which reads the cookie via
// `next/headers`) on top of the pure `t()` / `getCatalog()` from
// `i18n-lookup.ts`. Client code uses `i18n-client.ts` instead.
//
// The split exists so that:
//   - server bundles don't pull in React (`i18n-client.ts`);
//   - client bundles don't pull in `next/headers` (this file);
//   - both sides can share the catalog lookup logic
//     (`i18n-lookup.ts`).
//
// `t(key)` (no locale arg) is provided here for server-side call
// sites that want to resolve the locale from the cookie implicitly.
// The pure `t(key, locale)` lives in `i18n-lookup.ts` and is the
// recommended shape everywhere else.

import { cookies } from 'next/headers'
import { t as tWithLocale, getCatalog as getCatalogImpl } from './i18n-lookup'
import {
  LOCALE_COOKIE,
  DEFAULT_LOCALE,
  SUPPORTED_LOCALES,
  LOCALE_LABELS,
  type Locale,
} from './i18n-types'

// Re-export the public surface so existing server-side call sites
// (`import { ... } from '@/lib/i18n'`) keep working.
export {
  LOCALE_COOKIE,
  DEFAULT_LOCALE,
  SUPPORTED_LOCALES,
  LOCALE_LABELS,
  type Locale,
} from './i18n-types'
export { getCatalog } from './i18n-lookup'

// isLocale narrows a string to a Locale. Used after reading the cookie
// or parsing the Accept-Language header, both of which are untyped.
function isLocale(value: string | undefined | null): value is Locale {
  return value === 'zh' || value === 'en'
}

// parseLocale normalizes a raw cookie value (or any string) into a
// Locale. Anything unrecognized falls back to the default — we never
// throw here because locale resolution is on the hot path for every
// request.
function parseLocale(raw: string | undefined | null): Locale {
  if (isLocale(raw)) return raw
  return DEFAULT_LOCALE
}

/**
 * getLocale reads the active locale from the cookie. Server-only —
 * uses next/headers. Pass through `cookies()` from a server component
 * or route handler, or omit it to read the current request's cookies
 * implicitly.
 *
 * If called outside a request scope (build, tests) the underlying
 * `cookies()` call throws. We catch and fall back to the default so
 * importing this module at build time does not crash.
 */
export function getLocale(): Locale {
  try {
    const store = cookies()
    const value = store.get(LOCALE_COOKIE)?.value
    return parseLocale(value)
  } catch {
    return DEFAULT_LOCALE
  }
}

/**
 * t looks up `key` in the catalog for the active locale (read from
 * the cookie) and returns the translation. Server-side convenience
 * that delegates to `i18n-lookup.ts` after resolving the locale.
 */
export function t(key: string, locale?: Locale): string {
  const effective = locale ?? getLocale()
  return tWithLocale(key, effective)
}

// Touch getCatalogImpl so the re-export above doesn't get tree-shaken
// in case downstream code imports it from this module too.
void getCatalogImpl
