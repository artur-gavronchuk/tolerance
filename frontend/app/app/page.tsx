'use client'

import { useCallback, useEffect, useState } from 'react'
import { useRouter } from 'next/navigation'
import { StageCard } from '@/components/stage-card'
import { ProofList } from '@/components/proof-list'
import { api, post, ApiError } from '@/lib/api'
import { useMe } from '@/lib/use-me'
import type { Proof } from '@/lib/types'

export default function HomePage() {
  const { me, refresh } = useMe(5000)
  const router = useRouter()
  const [proofs, setProofs] = useState<Proof[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [starting, setStarting] = useState(false)

  const load = useCallback(async () => {
    if (!me?.agent) return setProofs([])
    try {
      setProofs((await api<{ items: Proof[] }>('/proofs')).items)
    } catch (e) {
      setError((e as ApiError).message)
    }
  }, [me?.agent])
  useEffect(() => { void load() }, [load, me?.agent?.stage])

  async function start() {
    setStarting(true)
    setError(null)
    try {
      const p = await post<Proof>('/proofs', { task_slug: 'go-fix-retry' })
      router.push(`/app/proofs/${p.id}`)
    } catch (e) {
      const a = e as ApiError
      setError(a.code === 'agent_offline' ? 'The connector is not online. Start `arena connect` first.' : a.message)
      await refresh()
    } finally {
      setStarting(false)
    }
  }

  if (!me) return null
  return (
    <div className="flex flex-col gap-6">
      <StageCard me={me} last={proofs?.[0] ?? null} onStart={start} starting={starting} />
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      <section>
        <h3 className="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">Proof history</h3>
        {proofs ? <ProofList items={proofs} /> : <p className="text-sm text-muted-foreground">Loading…</p>}
      </section>
    </div>
  )
}
