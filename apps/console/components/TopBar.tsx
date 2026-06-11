'use client'

// TopBar carries the page title and the operator's identity.
// In single-user mode the user is the one who installed the
// OPC instance; the server already returns it from /api/auth/me.
// For now we just render a static "Operator" label and a
// sign-out button that delegates to the shared auth helper —
// real wiring lands in the credentials/observability tasks.

import { useState } from 'react'
import { api, ApiError } from '@opc/shared/api'

export function TopBar({ title }: { title: string }) {
  const [signingOut, setSigningOut] = useState(false)
  async function onSignOut() {
    setSigningOut(true)
    try {
      await api.auth.logout()
    } catch (err: unknown) {
      // 401 is fine here — the session was already gone. Anything
      // else is also non-fatal: the goal is just to clear the
      // cookie before we redirect to /login.
      void err
    }
    if (typeof window !== 'undefined') {
      window.location.href = '/login'
    }
  }
  return (
    <header className="h-14 shrink-0 bg-claude-canvas border-b border-claude-hairline flex items-center px-6 gap-4">
      <h1 className="text-base font-medium text-claude-ink flex-1 truncate">{title}</h1>
      <span className="text-sm text-claude-muted">Operator</span>
      <button
        onClick={onSignOut}
        disabled={signingOut}
        className="text-sm text-claude-muted hover:text-claude-ink disabled:opacity-50"
      >
        {signingOut ? '登出中…' : '登出'}
      </button>
    </header>
  )
}
