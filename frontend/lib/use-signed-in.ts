'use client'

import { useEffect, useState } from 'react'
import { api } from './api'
import type { Me } from './types'

// One /me request shared by every signed-in-only control on a page (a leaderboard renders many of them).
let pending: Promise<string | null> | null = null

// The signed-in user's handle, or null when anonymous (or not loaded yet).
export function useMyHandle() {
  const [handle, setHandle] = useState<string | null>(null)
  useEffect(() => {
    pending ??= api<Me>('/me').then((m) => m.user.handle).catch(() => null)
    let live = true
    void pending.then((v) => live && setHandle(v))
    return () => { live = false }
  }, [])
  return handle
}
