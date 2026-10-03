'use client'

import { useCallback, useEffect, useState } from 'react'
import { api, post } from './api'

export interface UploadLinkStatus { active: boolean; created_at: string | null; last_used_at: string | null }

const KEY = 'upload-link-token'

// The token is shown by the server once; this browser keeps a copy so the prompt can be shown again.
const readToken = () => { try { return localStorage.getItem(KEY) } catch { return null } }
const writeToken = (t: string | null) => { try { t ? localStorage.setItem(KEY, t) : localStorage.removeItem(KEY) } catch {} }

export function useUploadLink(enabled = true) {
  const [status, setStatus] = useState<UploadLinkStatus | null>(null)
  const [token, setToken] = useState<string | null>(null)
  const [error, setError] = useState<unknown>(null)
  const load = useCallback(async () => {
    try {
      const s = await api<UploadLinkStatus>('/me/upload-link')
      setStatus(s)
      if (!s.active) writeToken(null)
      setToken(s.active ? readToken() : null)
    } catch (e) { setError(e) }
  }, [])
  useEffect(() => { if (enabled) void load() }, [enabled, load])
  const rotate = useCallback(async () => {
    setError(null)
    try {
      const r = await post<UploadLinkStatus & { token: string }>('/me/upload-link')
      writeToken(r.token)
      setToken(r.token)
      setStatus({ active: r.active, created_at: r.created_at, last_used_at: r.last_used_at })
    } catch (e) { setError(e) }
  }, [])
  const revoke = useCallback(async () => {
    setError(null)
    try {
      await api('/me/upload-link', { method: 'DELETE' })
      writeToken(null)
      setToken(null)
      setStatus({ active: false, created_at: null, last_used_at: null })
    } catch (e) { setError(e) }
  }, [])
  return { status, token, error, rotate, revoke }
}

export const linkBase = (token: string) => `${window.location.origin}/api/v1/u/${token}`
