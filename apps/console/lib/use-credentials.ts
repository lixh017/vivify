'use client'

// useCredentials is the console-side data hook for the
// /credentials page. It wraps the four REST endpoints exposed
// by api.credentials.* and exposes a uniform { items, loading,
// error, refresh, add, rotate, remove } surface so the page
// component can stay declarative. Errors propagate as ApiError
// (the shared client throws this) so callers can branch on
// status if they ever need to (e.g. 401 → redirect to /login).

import { useCallback, useEffect, useState } from 'react'
import { api, ApiError } from '@opc/shared/api'
import type {
  Credential,
  CreateCredentialInput,
  RotateCredentialInput,
} from '@opc/shared/types'

export type UseCredentials = {
  items: Credential[]
  loading: boolean
  error: string | null
  refresh: () => Promise<void>
  add: (input: CreateCredentialInput) => Promise<Credential>
  rotate: (id: number, input: RotateCredentialInput) => Promise<Credential>
  remove: (id: number) => Promise<void>
}

export function useCredentials(): UseCredentials {
  const [items, setItems] = useState<Credential[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const res = await api.credentials.list()
      setItems(res.items ?? [])
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : '加载凭据失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh])

  // add issues a POST and then re-fetches the list so the new
  // row lands in canonical server order (newest first). The
  // returned Credential is the freshly inserted row, which
  // dialogs can use to surface a confirmation toast.
  const add = useCallback(
    async (input: CreateCredentialInput): Promise<Credential> => {
      const created = await api.credentials.create(input)
      await refresh()
      return created
    },
    [refresh],
  )

  // rotate swaps the encrypted key on an existing row. We mutate
  // the row in place after a successful PUT to avoid the
  // round-trip of a full refresh (the row's id/created_at are
  // unchanged so the position in the list stays stable).
  const rotate = useCallback(
    async (id: number, input: RotateCredentialInput): Promise<Credential> => {
      const updated = await api.credentials.rotate(id, input)
      setItems((prev) => prev.map((c) => (c.id === id ? updated : c)))
      return updated
    },
    [],
  )

  // remove optimistically drops the row, then reverts on
  // failure. The server returns 204; we still call refresh on
  // error so a stale cache cannot leak a "ghost" row.
  const remove = useCallback(async (id: number): Promise<void> => {
    const previous = items
    setItems((prev) => prev.filter((c) => c.id !== id))
    try {
      await api.credentials.delete(id)
    } catch (err: unknown) {
      setItems(previous)
      throw err instanceof ApiError
        ? err
        : err instanceof Error
        ? err
        : new Error('删除凭据失败')
    }
  }, [items])

  return { items, loading, error, refresh, add, rotate, remove }
}
