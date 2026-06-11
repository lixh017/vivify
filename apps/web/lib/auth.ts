// Client-side auth helpers.
//
// These are thin wrappers over `api.auth.*` that:
//   1. Centralize the cookie-handling assumption (the server sets the
//      opc_session cookie on login/register; the browser attaches it
//      to every subsequent /api/* request via `credentials: 'include'`).
//   2. Narrow the API's `{ user: AuthUser }` envelope down to just
//      `AuthUser` for the common call sites.
//   3. Surface a stable "is logged in?" shape that React components
//      can plug into without re-implementing the try/catch dance.
//
// For richer per-call error handling (e.g. "Invalid email or password"
// vs network failure) the call site should still use `api.auth.login`
// directly and inspect the ApiError.

import { api, ApiError } from '@opc/shared/api'
import type { AuthUser } from '@opc/shared/types'

// LoginInput and RegisterInput describe the public contract for the
// helper functions. They are the same shape the Go backend expects on
// the wire, so callers can pass form values straight through.
export interface LoginInput {
  email: string
  password: string
}

export interface RegisterInput {
  email: string
  password: string
  name: string
}

// login posts the credentials and returns the AuthUser on success.
// On a 401 the underlying ApiError is propagated untouched so the
// caller can show a "邮箱或密码错误" message; we deliberately do not
// swallow or remap it here — the page is the right place to decide
// which copy to show.
export async function login(input: LoginInput): Promise<AuthUser> {
  const res = await api.auth.login(input)
  return res.user
}

// register creates a new account and signs the user in (the server
// sets the session cookie on the same response). 409 means the email
// is already taken; 400 means the payload failed server-side
// validation. Both surface as ApiError so the form can branch on
// `err.status`.
export async function register(input: RegisterInput): Promise<AuthUser> {
  const res = await api.auth.register(input)
  return res.user
}

// logout invalidates the server-side session and returns once the
// call has resolved. The server clears the cookie; we don't need to
// do anything client-side to drop it. We swallow network errors
// here because the UX outcome is always "go to /login" regardless
// of whether the server call succeeded.
export async function logout(): Promise<void> {
  try {
    await api.auth.logout()
  } catch (err: unknown) {
    // Expected: 401 if the cookie was already gone. Anything else we
    // also swallow — the navbar handles the redirect either way.
    if (!(err instanceof ApiError)) {
      // Non-HTTP failure (network, parse). Still no-op: the user
      // wanted to log out, and we are about to bounce them to /login.
    }
  }
}

// getCurrentUser resolves the current user from the session cookie.
// Returns null when there is no valid session (401) or when the API
// is unreachable; any other error is also coerced to null because
// the navbar's only job is to know whether to show a greeting.
//
// The explicit `unknown`-narrowing in the catch is intentional: the
// caller doesn't care why the call failed, only that there is no
// user. Surfacing the raw error here would force every call site to
// implement the same narrowing.
export async function getCurrentUser(): Promise<AuthUser | null> {
  try {
    const res = await api.auth.me()
    return res.user
  } catch (err: unknown) {
    if (err instanceof ApiError) {
      return null
    }
    return null
  }
}

// isAuthenticated is a convenience predicate. It runs `me()` once and
// collapses the result to a boolean. Use this when the component
// only cares about the yes/no question and not the user object.
export async function isAuthenticated(): Promise<boolean> {
  const user = await getCurrentUser()
  return user !== null
}
