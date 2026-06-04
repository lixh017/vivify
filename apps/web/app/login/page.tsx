'use client'

import { Suspense, useState, FormEvent, useEffect } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { api, ApiError } from '@/lib/api'

// Login page for Phase 2's session-based auth. The form posts to
// /api/auth/login; on success the browser's opc_session cookie is
// set by the server and we navigate to `next` (or /topics by
// default). On 401 we surface a generic "邮箱或密码错误" message —
// the server intentionally does not distinguish unknown-email from
// wrong-password so the endpoint can't be used to enumerate users.
//
// Next 14 requires that any component calling useSearchParams() be
// wrapped in a <Suspense> boundary so the page can be statically
// pre-rendered at build time. We split the form into LoginForm
// (which reads searchParams) and keep the page-level export as a
// thin Suspense wrapper around it.

function LoginForm() {
  const router = useRouter()
  const searchParams = useSearchParams()
  // Where to land after a successful login. The link from a 401 on
  // a protected page can append ?next=/foo; we honour that and
  // fall back to /topics so the user sees the most useful screen.
  const next = searchParams.get('next') || '/topics'

  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // If the user is already logged in, the /api/auth/me call returns
  // 200 and we bounce straight to `next`. This keeps a refresh on
  // /login from showing the form to an already-authenticated user.
  useEffect(() => {
    let cancelled = false
    api.auth
      .me()
      .then(() => {
        if (!cancelled) router.replace(next)
      })
      .catch(() => {
        // expected when not logged in; do nothing.
      })
    return () => {
      cancelled = true
    }
  }, [next, router])

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setSubmitting(true)
    setError(null)
    try {
      await api.auth.login({ email: email.trim(), password })
      // Cookie was set by the server. Navigate to the post-login
      // destination. router.replace (not push) so the back button
      // doesn't drag the user back to /login.
      router.replace(next)
    } catch (err: unknown) {
      if (err instanceof ApiError && err.status === 401) {
        setError('邮箱或密码错误')
      } else {
        setError(err instanceof Error ? err.message : '登录失败')
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="w-full max-w-sm">
      <h1 className="text-2xl md:text-3xl font-bold text-center text-claude-ink">
        🐼 OPC 登录
      </h1>
      <p className="text-sm text-claude-muted text-center mt-1">
        熊猫 IP 创作控制台
      </p>
      <form
        onSubmit={handleSubmit}
        className="mt-6 space-y-4 p-4 md:p-6 bg-claude-surface-card rounded-lg border border-claude-hairline"
        aria-label="登录表单"
      >
        <div>
          <label
            htmlFor="email"
            className="block text-sm font-medium text-claude-ink"
          >
            邮箱
          </label>
          <input
            id="email"
            name="email"
            type="email"
            autoComplete="email"
            required
            data-testid="input-email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:outline-none focus:ring-2 focus:ring-claude-coral"
            disabled={submitting}
          />
        </div>
        <div>
          <label
            htmlFor="password"
            className="block text-sm font-medium text-claude-ink"
          >
            密码
          </label>
          <input
            id="password"
            name="password"
            type="password"
            autoComplete="current-password"
            required
            data-testid="input-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:outline-none focus:ring-2 focus:ring-claude-coral"
            disabled={submitting}
          />
        </div>

        {error && (
          <div
            role="alert"
            data-testid="login-error"
            className="p-3 bg-claude-surface-card border border-claude-error text-claude-error rounded text-sm"
          >
            {error}
          </div>
        )}

        <button
          type="submit"
          data-testid="btn-submit"
          disabled={submitting}
          className="w-full px-4 py-2 text-sm bg-claude-coral text-claude-on-primary rounded-md hover:bg-claude-coral-active disabled:opacity-50 transition-colors"
        >
          {submitting ? '登录中…' : '登录'}
        </button>
      </form>

      <p className="mt-4 text-xs text-claude-muted text-center">
        还没有账号？请联系管理员开通。
      </p>
    </div>
  )
}

function LoginFormFallback() {
  // Render-only fallback shown while Suspense is hydrating the
  // client form. Kept dependency-free (no hooks) so it is safe to
  // serve from the static prerender.
  return (
    <div className="w-full max-w-sm" aria-busy="true">
      <h1 className="text-2xl md:text-3xl font-bold text-center text-claude-ink">
        🐼 OPC 登录
      </h1>
      <p className="text-sm text-claude-muted text-center mt-1">
        熊猫 IP 创作控制台
      </p>
      <div className="mt-6 p-4 md:p-6 bg-claude-surface-card rounded-lg border border-claude-hairline text-sm text-claude-muted text-center">
        正在加载登录表单…
      </div>
    </div>
  )
}

export default function LoginPage() {
  return (
    <div className="min-h-[60vh] flex items-center justify-center px-3">
      <Suspense fallback={<LoginFormFallback />}>
        <LoginForm />
      </Suspense>
    </div>
  )
}
