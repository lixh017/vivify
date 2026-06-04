'use client'

import { Suspense, useState, FormEvent, useEffect } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { api, ApiError } from '@/lib/api'

// Register page for Phase 2's session-based auth. The form posts to
// /api/auth/register; on success the browser's opc_session cookie is
// set by the server and we navigate to `next` (or /topics by default).
//
// Error handling:
//   - 409: email already registered → 邮箱已被注册
//   - 400: payload validation (server-side) → map a few common shapes
//     to a friendly message, fall back to the raw body
//   - anything else: keep the generic "注册失败" so the form does not
//     leak server stack traces into the UI
//
// Client-side validation:
//   - email matches a basic email regex (the server is the source of
//     truth, but a JS-level check stops us from posting obviously
//     invalid payloads)
//   - password is at least 8 characters
//   - name is non-empty after trim
//
// Next 14 requires that any component calling useSearchParams() be
// wrapped in a <Suspense> boundary so the page can be statically
// pre-rendered at build time. We split the form into RegisterForm
// (which reads searchParams) and keep the page-level export as a
// thin Suspense wrapper around it.

// Minimal email check. The server is the source of truth and runs a
// proper validation; this regex is only here to keep the form from
// posting obvious garbage.
const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
const MIN_PASSWORD_LEN = 8

function RegisterForm() {
  const router = useRouter()
  const searchParams = useSearchParams()
  // Where to land after a successful registration. The link from a
  // 401 on a protected page can append ?next=/foo; we honour that and
  // fall back to /topics so the user sees the most useful screen.
  const next = searchParams.get('next') || '/topics'

  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // If the user is already logged in, the /api/auth/me call returns
  // 200 and we bounce straight to `next`. This keeps a refresh on
  // /register from showing the form to an already-authenticated user.
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

  function validate(): string | null {
    const trimmedEmail = email.trim()
    const trimmedName = name.trim()
    if (!trimmedName) {
      return '请填写昵称'
    }
    if (!EMAIL_RE.test(trimmedEmail)) {
      return '邮箱格式不正确'
    }
    if (password.length < MIN_PASSWORD_LEN) {
      return `密码至少 ${MIN_PASSWORD_LEN} 位`
    }
    return null
  }

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setError(null)
    const clientError = validate()
    if (clientError) {
      setError(clientError)
      return
    }
    setSubmitting(true)
    try {
      await api.auth.register({
        name: name.trim(),
        email: email.trim(),
        password,
      })
      // Cookie was set by the server. Navigate to the post-register
      // destination. router.replace (not push) so the back button
      // doesn't drag the user back to /register.
      router.replace(next)
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        if (err.status === 409) {
          setError('该邮箱已被注册')
        } else if (err.status === 400) {
          setError(err.body || '注册信息有误')
        } else {
          setError(err.body || `注册失败 (${err.status})`)
        }
      } else {
        setError(err instanceof Error ? err.message : '注册失败')
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="w-full max-w-sm">
      <h1 className="font-serif text-2xl text-center text-claude-ink tracking-tight">
        🐼 创建账号
      </h1>
      <p className="text-sm text-claude-muted text-center mt-1">
        熊猫 IP 创作控制台
      </p>
      <form
        onSubmit={handleSubmit}
        className="mt-6 space-y-4 p-4 md:p-6 bg-claude-surface-card rounded-lg border border-claude-hairline"
        aria-label="注册表单"
      >
        <div>
          <label
            htmlFor="name"
            className="block text-sm font-medium text-claude-ink"
          >
            昵称
          </label>
          <input
            id="name"
            name="name"
            type="text"
            autoComplete="name"
            required
            data-testid="input-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
            disabled={submitting}
          />
        </div>
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
            className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
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
            autoComplete="new-password"
            minLength={MIN_PASSWORD_LEN}
            required
            data-testid="input-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="mt-1 w-full px-3 py-2 text-sm border border-claude-hairline rounded bg-claude-canvas text-claude-ink focus:border-claude-coral focus:outline-none focus:ring-1 focus:ring-claude-coral"
            disabled={submitting}
          />
          <p className="mt-1 text-xs text-claude-muted">
            至少 {MIN_PASSWORD_LEN} 位
          </p>
        </div>

        {error && (
          <div
            role="alert"
            data-testid="register-error"
            className="p-3 bg-claude-error/10 border border-claude-error text-claude-error rounded text-sm"
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
          {submitting ? '注册中…' : '注册'}
        </button>
      </form>

      <p className="mt-4 text-xs text-claude-muted text-center">
        已有账号？
        <a
          href="/login"
          data-testid="link-login"
          className="ml-1 text-claude-coral hover:text-claude-coral-active focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-claude-coral focus-visible:ring-offset-2 rounded-sm transition-colors"
        >
          去登录
        </a>
      </p>
    </div>
  )
}

function RegisterFormFallback() {
  // Render-only fallback shown while Suspense is hydrating the
  // client form. Kept dependency-free (no hooks) so it is safe to
  // serve from the static prerender.
  return (
    <div className="w-full max-w-sm" aria-busy="true">
      <h1 className="font-serif text-2xl text-center text-claude-ink tracking-tight">
        🐼 创建账号
      </h1>
      <p className="text-sm text-claude-muted text-center mt-1">
        熊猫 IP 创作控制台
      </p>
      <div className="mt-6 p-4 md:p-6 bg-claude-surface-card rounded-lg border border-claude-hairline text-sm text-claude-muted text-center">
        正在加载注册表单…
      </div>
    </div>
  )
}

export default function RegisterPage() {
  return (
    <div className="min-h-[60vh] flex items-center justify-center px-3">
      <Suspense fallback={<RegisterFormFallback />}>
        <RegisterForm />
      </Suspense>
    </div>
  )
}
