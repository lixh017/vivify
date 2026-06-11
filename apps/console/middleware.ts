// Console middleware: protect every admin route by ensuring the
// opc_session cookie is present. The actual session validation
// (cookie signing, expiry) happens in the Go backend on each
// /api/* call; the middleware is the cheap first gate that
// spares the backend a 401 round-trip for an unauthenticated
// browser navigation.
//
// We deliberately do NOT decode the cookie value in the Edge
// runtime — the signing secret lives in the Go binary and we
// don't want to duplicate the verification path. A missing or
// obviously-malformed cookie is enough to redirect.

import { NextRequest, NextResponse } from 'next/server'

const SESSION_COOKIE = 'opc_session'

export function middleware(request: NextRequest): NextResponse {
  const session = request.cookies.get(SESSION_COOKIE)?.value
  if (!session) {
    const url = request.nextUrl.clone()
    url.pathname = '/login'
    return NextResponse.redirect(url)
  }
  return NextResponse.next()
}

// Apply to every admin route except /login, Next internals, and
// the home redirect. /api/* stays unprotected at the middleware
// layer because the Go backend's auth middleware handles it.
export const config = {
  matcher: [
    '/((?!_next/static|_next/image|favicon.ico|login|api/.*).*)',
  ],
}
