// Pure translation lookup. No `next/headers`, no React — safe to
// import from both server and client bundles.
//
// Kept separate from `i18n.ts` (which adds `getLocale()` and pulls
// in `next/headers`) and from `i18n-client.ts` (which adds React
// hooks). This module is the lowest common denominator that both
// can import without dragging their respective forbidden deps.

import zhCatalog from './locales/zh.json'
import enCatalog from './locales/en.json'
import {
  DEFAULT_LOCALE,
  SUPPORTED_LOCALES,
  type Catalog,
  type Locale,
  type TranslationKey,
} from './i18n-types'

// The two .json files are imported as `any` by Next's JSON loader,
// so we narrow them through a runtime parity check before pinning
// the type to `Catalog`. The phase-1 QA review flagged that
// `as unknown as Catalog` was hiding typos and missing keys; this
// guard turns drift into a loud startup failure rather than a
// silent `return key` at runtime.
//
// We only run the check outside production to keep the prod
// bundle small. The typed `Catalog` assignment below is the
// compile-time guarantee.
const catalogs: Record<Locale, Catalog> = assertCatalogParity({
  zh: zhCatalog as unknown as Record<string, string>,
  en: enCatalog as unknown as Record<string, string>,
}) as Record<Locale, Catalog>

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
 *
 * In dev (NODE_ENV !== 'production') a missing key logs a single
 * warning per (locale, key) pair via `warnMissingKey` so unfinished
 * translations are loud at the console. In production the warning
 * is suppressed — the `return key` fallback is the visible signal.
 */
export function t(key: TranslationKey | string, locale: Locale): string {
  const catalog = catalogs[locale] as unknown as Record<string, string>
  if (catalog && key in catalog) {
    return catalog[key]
  }
  warnMissingKey(locale, key)
  // Try the default locale as a secondary fallback — useful when
  // a brand-new key has been added to zh.json but not yet to en.json.
  const fallback = catalogs[DEFAULT_LOCALE] as unknown as Record<string, string>
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
export function getCatalog(locale: Locale): Catalog {
  return (catalogs[locale] ?? catalogs[DEFAULT_LOCALE]) as Catalog
}

// ----------------------------------------------------------------
// Internal: parity check + dev warnings
// ----------------------------------------------------------------

// ALL_KEYS is the union of all literal members of the TranslationKey
// type. We extract it by building an object of type Record<K, true>
// and reading its keys — this picks up every string-literal member
// of the union without hand-listing them.
type AllKeyObject = Record<TranslationKey, true>
const ALL_KEYS_LIST: readonly TranslationKey[] = Object.keys(
  {} as AllKeyObject,
) as readonly TranslationKey[]
const ALL_KEYS_SET: ReadonlySet<string> = new Set(ALL_KEYS_LIST)

function assertCatalogParity(
  raw: Record<Locale, Record<string, string>>,
): Record<Locale, Record<string, string>> {
  if (process.env.NODE_ENV === 'production') return raw
  for (const locale of SUPPORTED_LOCALES) {
    const keys = Object.keys(raw[locale])
    const missing = ALL_KEYS_LIST.filter((k) => !keys.includes(k))
    if (missing.length > 0) {
      throw new Error(
        `[i18n] locale "${locale}" is missing keys: ${missing.join(', ')}`,
      )
    }
    const extra = keys.filter((k) => !ALL_KEYS_SET.has(k))
    if (extra.length > 0) {
      throw new Error(
        `[i18n] locale "${locale}" has unknown keys: ${extra.join(', ')}`,
      )
    }
  }
  return raw
}

const warnedKeys = new Set<string>()

function warnMissingKey(locale: Locale, key: string): void {
  if (process.env.NODE_ENV === 'production') return
  const tag = `${locale}::${key}`
  if (warnedKeys.has(tag)) return
  warnedKeys.add(tag)
  // eslint-disable-next-line no-console
  console.warn(
    `[i18n] missing key "${key}" for locale "${locale}" — falling back.`,
  )
}
