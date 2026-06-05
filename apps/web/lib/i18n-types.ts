// Pure types and constants shared by server (`i18n.ts`) and client
// (`i18n-client.ts`) i18n code. This module has no `next/headers`
// and no React imports, so it is safe to pull in from either side
// without triggering the "use client / server" boundary errors.

export type Locale = 'zh' | 'en'

export const LOCALE_COOKIE = 'opc_locale'
export const DEFAULT_LOCALE: Locale = 'zh'
export const SUPPORTED_LOCALES: readonly Locale[] = ['zh', 'en'] as const

// Locale display labels for the switcher. Kept here (not in the
// catalog) because the user is selecting the locale itself; we need
// to know what to render regardless of which locale is currently
// active.
export const LOCALE_LABELS: Record<Locale, string> = {
  zh: '中',
  en: 'EN',
}
