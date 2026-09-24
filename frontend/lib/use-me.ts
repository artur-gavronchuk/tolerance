'use client'

import { useCallback, useEffect, useState } from 'react'
import { api, ApiError } from './api'
import type { Me } from './types'

export function useMe(pollMs = 0) {
  const [me, setMe] = useState<Me | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<ApiError | null>(null)
  const refresh = useCallback(async () => {
    try {
      setMe(await api<Me>('/me'))
      setError(null)
    } catch (e) {
      setError(e as ApiError)
    } finally {
      setLoading(false)
    }
  }, [])
  useEffect(() => {
    void refresh()
    if (!pollMs) return
    const t = setInterval(() => void refresh(), pollMs)
    return () => clearInterval(t)
  }, [refresh, pollMs])
  return { me, loading, error, refresh }
}
