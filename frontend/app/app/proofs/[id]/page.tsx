'use client'

import { use, useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Timeline } from '@/components/proof/timeline'
import { ProofResult } from '@/components/proof/result'
import { DiffView } from '@/components/proof/diff-view'
import { api, post, ApiError } from '@/lib/api'
import type { Proof } from '@/lib/types'
import { STATUS_LABEL } from '@/lib/format'

const TERMINAL = ['passed', 'failed', 'infra_error', 'expired']

export default function ProofPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params)
  const [proof, setProof] = useState<Proof | null>(null)
  const [error, setError] = useState<string | null>(null)
  const load = useCallback(async () => {
    try {
      setProof(await api<Proof>(`/proofs/${id}`))
    } catch (e) {
      setError((e as ApiError).status === 404 ? 'No such proof.' : (e as ApiError).message)
    }
  }, [id])
  useEffect(() => {
    void load()
    const t = setInterval(() => { if (!proof || !TERMINAL.includes(proof.status)) void load() }, 3000)
    return () => clearInterval(t)
  }, [load, proof?.status])

  async function retry() {
    try {
      setProof(await post<Proof>(`/proofs/${id}/retry`))
    } catch (e) {
      setError((e as ApiError).message)
    }
  }

  if (error) return <p role="alert" className="text-sm text-destructive">{error}</p>
  if (!proof) return <p className="text-sm text-muted-foreground">Loading…</p>
  const done = TERMINAL.includes(proof.status)
  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <p className="font-mono text-xs text-muted-foreground">{proof.task_slug}</p>
          <h1 className="text-2xl font-semibold">{done ? 'Result' : STATUS_LABEL[proof.status]}</h1>
        </div>
        <Link href="/app" className="text-sm text-muted-foreground hover:text-foreground">← Home</Link>
      </div>
      <Card className="p-4"><Timeline proof={proof} /></Card>
      {!done && <p className="text-sm text-muted-foreground">This page refreshes on its own. Your agent works alone; you can watch, but you can’t help.</p>}
      {done && <ProofResult proof={proof} />}
      {(proof.status === 'infra_error' || proof.status === 'expired') && <Button onClick={retry}>Retry</Button>}
      {proof.diff && (
        <section>
          <h3 className="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">Diff your agent produced</h3>
          <DiffView diff={proof.diff} />
        </section>
      )}
    </div>
  )
}
