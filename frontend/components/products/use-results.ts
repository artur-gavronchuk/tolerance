'use client'

import { useCallback, useEffect, useRef, useState } from 'react'
import { products } from '@/lib/api'
import { friendly, usePT } from './phase'
import type { ProductEntry, ProductResults } from '@/lib/types'

// A task's standings plus the vote actions. With `stable` the entries keep the order they first loaded in, so
// a gallery does not reshuffle under the pointer while people vote; the numbers still update. The server sends a
// page of the standings at a time; `more` asks for the next page.
const PAGE = 40

export function useResults(slug: string, viewer: string | undefined, stable = false) {
  const t = usePT()
  const [res, setRes] = useState<ProductResults | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [voteError, setVoteError] = useState<string | null>(null)
  const [busy, setBusy] = useState<string | null>(null)
  const [limit, setLimit] = useState(PAGE)
  const order = useRef<string[] | null>(null)

  const refresh = useCallback(async () => {
    try {
      const r = await products.results(slug, limit)
      if (stable) {
        if (!order.current) order.current = r.entries.map((e) => e.id)
        const pos = new Map(order.current.map((id, i) => [id, i]))
        r.entries.sort((a, b) => (pos.get(a.id) ?? 1e9) - (pos.get(b.id) ?? 1e9))
      }
      setRes(r)
    } catch (e) {
      setError(friendly(t, e))
    }
  }, [slug, stable, t, limit])
  useEffect(() => { void refresh() }, [refresh, viewer])

  // Clicking your vote again takes it back; clicking another entry moves it.
  async function toggleVote(e: ProductEntry) {
    setVoteError(null)
    setBusy(e.id)
    try {
      if (e.voted) await products.unvote(e.id)
      else await products.vote(e.id)
      await refresh()
    } catch (err) {
      setVoteError(friendly(t, err))
    } finally {
      setBusy(null)
    }
  }

  const more = () => setLimit((l) => l + PAGE)
  return { res, error, voteError, busy, refresh, toggleVote, more }
}
