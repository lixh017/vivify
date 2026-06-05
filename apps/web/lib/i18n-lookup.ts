// Pure translation lookup. No `next/headers`, no React — safe to
// import from both server and client bundles.
//
// Kept separate from `i18n.ts` (which adds `getLocale()` and pulls
// in `next/headers`) and from `i18n-client.ts` (which adds React
// hooks). This module is the lowest common denominator that both
// can import without dragging their respective forbidden deps.

import zhCatalog from './locales/zh.json'
import enCatalog from './locales/en.json'
import { DEFAULT_LOCALE, type Locale } from './i18n-types'

const catalogs: Record<Locale, Record<string, string>> = {
  zh: zhCatalog as unknown as Record<string, string>,
  en: enCatalog as unknown as Record<string, string>,
}

/**
 * t looks up `key` in the catalog for `locale` and returns the
 * translation. If the key is missing it falls back to the default
 * locale, and then to the key string itself so the UI shows
 * something obviously unfinished rather than a blank.
 *
 * Signature: `t(key, locale)`. There is no zero-arg form because
 * the caller always knows which locale they want — server code gets
 * it from `getLocale()`, client code gets it from `useT()`. Forcing
 * the explicit argument makes the data flow obvious at call sites
 * and avoids accidentally rendering the wrong language when a
 * shared helper is called from both contexts.
 */
export function t(key: string, locale: Locale): string {
  const catalog = catalogs[locale]
  if (catalog && key in catalog) {
    return catalog[key]
  }
  // Try the default locale as a secondary fallback — useful when
  // a brand-new key has been added to zh.json but not yet to en.json.
  const fallback = catalogs[DEFAULT_LOCALE]
  if (fallback && key in fallback) {
    return fallback[key]
  }
  return key
}

/**
 * getCatalog returns the full catalog for `locale`. Used by the
 * /api/locale endpoint to ship the new strings down to client
 * components when the user toggles languages.
 */
export function getCatalog(locale: Locale): Record<string, string> {
  return catalogs[locale] ?? catalogs[DEFAULT_LOCALE]
}
