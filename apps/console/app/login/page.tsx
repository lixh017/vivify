'use client'

// Login page is intentionally outside the (console) route
// group so it renders without the sidebar/topbar. The form
// posts to /api/auth/login (rewritten to the Go backend).
// On success we set window.location to /dashboard so the
// session cookie sticks to the next request.

import { useState, FormEvent } from 'react'
import { api, ApiError } from '@opc/shared/api'

export default function LoginPage() {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [err, setErr] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setErr(null)
    setLoading(true)
    try {
      await api.auth.login({ email, password })
      window.location.href = '/dashboard'
    } catch (e: unknown) {
      if (e instanceof ApiError) {
        setErr(e.status === 401 ? '邮箱或密码错误' : `登录失败 (${e.status})`)
      } else {
        setErr('网络错误，请重试')
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <main className="min-h-screen flex items-center justify-center bg-claude-canvas p-6">
      <form
        onSubmit={onSubmit}
        className="w-full max-w-sm bg-white border border-claude-hairline rounded-lg p-6 shadow-claude-soft space-y-4"
      >
        <div>
          <h1 className="text-lg font-semibold">OPC Console</h1>
          <p className="text-sm text-claude-muted mt-1">MCN 控制台登录</p>
        </div>
        <label className="block">
          <span className="block text-sm text-claude-body mb-1">邮箱</span>
          <input
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
            autoComplete="email"
            className="w-full px-3 py-2 border border-claude-hairline rounded-md text-sm focus:outline-none focus:border-claude-coral"
          />
        </label>
        <label className="block">
          <span className="block text-sm text-claude-body mb-1">密码</span>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            autoComplete="current-password"
            className="w-full px-3 py-2 border border-claude-hairline rounded-md text-sm focus:outline-none focus:border-claude-coral"
          />
        </label>
        {err && <div className="text-sm text-claude-error">{err}</div>}
        <button
          type="submit"
          disabled={loading}
          className="w-full py-2 bg-claude-coral text-claude-on-primary rounded-md text-sm font-medium hover:bg-claude-coral-active disabled:opacity-50"
        >
          {loading ? '登录中…' : '登录'}
        </button>
      </form>
    </main>
  )
}
