// POST /api/locale — sets the locale cookie and returns the new
// catalog so the client can swap strings in place (or just reload to
// re-render server components in the new locale).
//
// The body is `{ locale: 'zh' | 'en' }`. Anything else yields a 400
// because silently coercing an unknown value would mask client bugs.
//
// We set the cookie with the same attributes the switcher uses
// client-side so a hand-rolled API call lands in the same state.

import { NextRequest, NextResponse } from 'next/server'
import {
  getCatalog,
  LOCALE_COOKIE,
  LOCALE_COOKIE_MAX_AGE,
  LOCALE_COOKIE_PATH,
  SUPPORTED_LOCALES,
  DEFAULT_LOCALE,
  type Locale,
} from '@opc/shared/i18n'

interface SetLocaleRequest {
  locale: string
}

function isLocale(value: unknown): value is Locale {
  return typeof value === 'string' && (SUPPORTED_LOCALES as readonly string[]).includes(value)
}

export async function POST(request: NextRequest): Promise<NextResponse> {
  let body: SetLocaleRequest
  try {
    body = (await request.json()) as SetLocaleRequest
  } catch {
    return NextResponse.json(
      { error: 'invalid_json', message: 'Request body must be JSON.' },
      { status: 400 },
    )
  }

  if (!isLocale(body.locale)) {
    return NextResponse.json(
      {
        error: 'invalid_locale',
        message: `locale must be one of: ${SUPPORTED_LOCALES.join(', ')}`,
      },
      { status: 400 },
    )
  }

  const response = NextResponse.json({
    success: true,
    data: {
      locale: body.locale,
      catalog: getCatalog(body.locale),
    },
  })

  // Mirror the cookie attributes used by the client-side helper so
  // both paths produce the same Set-Cookie header.
  response.cookies.set({
    name: LOCALE_COOKIE,
    value: body.locale,
    path: LOCALE_COOKIE_PATH,
    maxAge: LOCALE_COOKIE_MAX_AGE,
    sameSite: 'lax',
  })

  return response
}

// GET returns the current catalog. Useful for client components that
// need to resolve a key without having the catalog in their bundle
// (e.g. dynamically-injected widgets). It also doubles as a "what
// locale is the server seeing right now?" probe for debugging.
export async function GET(request: NextRequest): Promise<NextResponse> {
  const cookieValue = request.cookies.get(LOCALE_COOKIE)?.value
  const locale: Locale = isLocale(cookieValue) ? cookieValue : DEFAULT_LOCALE
  return NextResponse.json({
    success: true,
    data: {
      locale,
      catalog: getCatalog(locale),
    },
  })
}
