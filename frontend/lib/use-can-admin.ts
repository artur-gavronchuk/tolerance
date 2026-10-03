'use client'

import { useEffect, useState } from 'react'
import { api } from './api'
import type { Me } from './types'

// One /me request shared by every admin-only control on a page (a leaderboard can render hundreds of them).
let pending: Promise<boolean> | null = null

export function useCanAdmin() {
  const [can, setCan] = useState(false)
  useEffect(() => {
    pending ??= api<Me>('/me').then((m) => !!m.can_admin).catch(() => false)
    let live = true
    void pending.then((v) => live && setCan(v))
    return () => { live = false }
  }, [])
  return can
}
