'use client'

import { useCallback, useEffect, useState } from 'react'
import { StageCard } from '@/components/stage-card'
import { ProofList } from '@/components/proof-list'
import { api, ApiError } from '@/lib/api'
import { useMe } from '@/lib/use-me'
import type { Proof } from '@/lib/types'

export default function HomePage() {
  const { me } = useMe(5000)
  const [proofs, setProofs] = useState<Proof[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const hasAgent = me?.agent != null
  const last = me?.agent?.last_proof
  // /me is polled; reload the history when the latest proof appears or moves.
  const lastKey = last ? `${last.id}:${last.status}` : ''

  const load = useCallback(async () => {
    if (!hasAgent) return setProofs([])
    try {
      setProofs((await api<{ items: Proof[] }>('/proofs')).items)
      setError(null)
    } catch (e) {
      setError((e as ApiError).message)
    }
  }, [hasAgent])
  useEffect(() => { void load() }, [load, lastKey])

  if (!me) return null
  return (
    <div className="flex flex-col gap-6">
      <StageCard me={me} />
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      <section>
        <h3 className="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">Proof history</h3>
        {proofs ? <ProofList items={proofs} /> : <p className="text-sm text-muted-foreground">Loading…</p>}
      </section>
    </div>
  )
}
