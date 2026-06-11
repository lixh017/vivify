'use client'

// useLogs is the console-side data hook for the /logs page. It
// wraps api.logs.list() and exposes a uniform { items, total,
// loading, error, refresh, setFilter } surface that mirrors the
// shape of the other console hooks (useCredentials /
// useObservabilitySummary) so the page component can stay
// declarative.
//
// State model:
//   - filter is the source of truth for the request. The page
//     updates filter via setFilter (range, skill, status,
//     provider, search, page). Every change goes through a
//     debounce (only the search field — range/skill/status/etc.
//     fire immediately, since they are picker-style inputs that
//     should land on the next fetch without a typing delay).
//   - searchInput is the immediate value bound to the text
//     input. We keep it separate from filter.search so the
//     operator sees their keystrokes appear instantly while the
//     network round-trip is debounced.
//   - pagination is tracked as `page` inside the hook; we
//     translate to/from the wire's limit/offset so the page
//     only ever thinks in pages.

import { useCallback, useEffect, useState } from 'react'
import { api, ApiError } from '@opc/shared/api'
import type { CallLog, LogsListParams } from '@opc/shared/types'

// PAGE_SIZE matches the server's default limit. Keeping the
// client page size in sync with the server default means a
// first-load request hits the cache hot path on the server.
const PAGE_SIZE = 50
// SEARCH_DEBOUNCE_MS is the quiet period before a search-input
// change is committed to the filter. 250ms is short enough to
// feel snappy on a fast typist and long enough to coalesce the
// bulk of a paste into one request.
const SEARCH_DEBOUNCE_MS = 250

// Range is the closed set of time-window presets the page
// exposes. The hook stores the raw ISO [from, to) pair so the
// wire call does not have to re-derive it; the page passes in
// the resolved pair.
export type LogsFilter = Omit<LogsListParams, 'limit' | 'offset'> & {
  from: string
  to: string
  // page is 1-indexed for the UI; the hook translates to
  // limit/offset on the wire.
  page: number
}

export type UseLogs = {
  items: CallLog[]
  total: number
  loading: boolean
  error: string | null
  filter: LogsFilter
  // setFilter is a partial updater — the page can pass only the
  // keys it wants to change (e.g. { status: 1 }) and the hook
  // merges. A status/skill/range change resets the page back to
  // 1 so the operator does not land on page 5 of a freshly-
  // filtered set.
  setFilter: (patch: Partial<Omit<LogsFilter, 'page'>>) => void
  setPage: (page: number) => void
  setSearchInput: (value: string) => void
  refresh: () => Promise<void>
}

export function useLogs(initial: LogsFilter): UseLogs {
  const [filter, setFilterState] = useState<LogsFilter>(initial)
  const [searchInput, setSearchInputState] = useState<string>(
    initial.search ?? '',
  )
  const [items, setItems] = useState<CallLog[]>([])
  const [total, setTotal] = useState<number>(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Debounce the search input into filter.search. We snapshot
  // searchInput via a ref-less closure — the effect re-subscribes
  // on every change, so the latest value is always live.
  useEffect(() => {
    const t = setTimeout(() => {
      setFilterState((prev) => {
        if (prev.search === searchInput) return prev
        return { ...prev, search: searchInput, page: 1 }
      })
    }, SEARCH_DEBOUNCE_MS)
    return () => clearTimeout(t)
  }, [searchInput])

  const refresh = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const offset = (filter.page - 1) * PAGE_SIZE
      const res = await api.logs.list({
        from: filter.from,
        to: filter.to,
        skill: filter.skill,
        status: filter.status,
        provider: filter.provider,
        search: filter.search,
        limit: PAGE_SIZE,
        offset,
      })
      setItems(res.items ?? [])
      setTotal(res.total ?? 0)
    } catch (err: unknown) {
      setItems([])
      setTotal(0)
      setError(
        err instanceof ApiError
          ? err.message
          : err instanceof Error
          ? err.message
          : '加载调用日志失败',
      )
    } finally {
      setLoading(false)
    }
  }, [filter])

  useEffect(() => {
    void refresh()
  }, [refresh])

  const setFilter = useCallback(
    (patch: Partial<Omit<LogsFilter, 'page'>>) => {
      setFilterState((prev) => {
        // If any of the filter-meaningful fields change, drop
        // the page back to 1. search is handled by the
        // debounce path above (it also resets to page 1).
        const meaningful =
          patch.from !== undefined ||
          patch.to !== undefined ||
          patch.skill !== undefined ||
          patch.status !== undefined ||
          patch.provider !== undefined
        return { ...prev, ...patch, page: meaningful ? 1 : prev.page }
      })
    },
    [],
  )

  const setPage = useCallback((page: number) => {
    setFilterState((prev) => ({ ...prev, page: Math.max(1, page) }))
  }, [])

  const setSearchInput = useCallback((value: string) => {
    setSearchInputState(value)
  }, [])

  return {
    items,
    total,
    loading,
    error,
    filter,
    setFilter,
    setPage,
    setSearchInput,
    refresh,
  }
}
