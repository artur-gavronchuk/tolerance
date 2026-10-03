'use client'

import { useState } from 'react'
import { tanks } from '@/lib/api'
import type { SeasonDetail } from '@/lib/types'

// "Show more" for a season's standings: fetches the next slice and appends it.
export function useMoreStandings(id: string, d: SeasonDetail | null, setD: (d: SeasonDetail) => void) {
  const [loading, setLoading] = useState(false)
  const more = async () => {
    if (!d) return
    setLoading(true)
    try {
      const r = await tanks.season(id, d.standings.length)
      setD({ ...d, standings: [...d.standings, ...r.standings], total: r.total, you: r.you })
    } catch {
      // the button stays; the reader can retry
    } finally {
      setLoading(false)
    }
  }
  return { more, loading }
}
