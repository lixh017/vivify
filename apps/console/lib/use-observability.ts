'use client'

// useObservabilitySummary is the console-side data hook for the
// /observability page. It calls GET /api/observability/summary
// with the active time-range preset (7d/30d/today) and exposes
// the wire shape plus a uniform { data, loading, error, refresh }
// surface so the page component can stay declarative.
//
// We expose a setter for the range (setRange) so the page's
// segmented control can drive the data fetch without holding
// local state for the data itself. The hook re-fetches whenever
// the range changes — the server already does the right thing
// for each preset, so we do not need a "force refresh" button.

import { useCallback, useEffect, useState } from 'react'
import { api, ApiError } from '@opc/shared/api'
import type {
  ObservabilitySummary,
  ObservabilitySummaryRange,
} from '@opc/shared/api'

export type UseObservabilitySummary = {
  range: ObservabilitySummaryRange
  setRange: (next: ObservabilitySummaryRange) => void
  data: ObservabilitySummary | null
  loading: boolean
  error: string | null
  refresh: () => Promise<void>
}

export function useObservabilitySummary(
  initialRange: ObservabilitySummaryRange = '7d',
): UseObservabilitySummary {
  const [range, setRange] = useState<ObservabilitySummaryRange>(initialRange)
  const [data, setData] = useState<ObservabilitySummary | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const res = await api.observability.summary({ range })
      setData(res)
    } catch (err: unknown) {
      setData(null)
      setError(
        err instanceof ApiError
          ? err.message
          : err instanceof Error
          ? err.message
          : '加载观测数据失败',
      )
    } finally {
      setLoading(false)
    }
  }, [range])

  useEffect(() => {
    void refresh()
  }, [refresh])

  return { range, setRange, data, loading, error, refresh }
}
