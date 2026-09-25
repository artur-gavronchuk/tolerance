'use client'

import { use, useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import { ArrowLeft, RotateCcw } from 'lucide-react'
import { PageHeader, SectionTitle } from '@/components/page-header'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Timeline } from '@/components/proof/timeline'
import { ProofLogs, TestResults, VerdictBand } from '@/components/proof/result'
import { DiffView } from '@/components/proof/diff-view'
import { api, post, ApiError, friendlyMessage } from '@/lib/api'
import type { Proof } from '@/lib/types'
import { STATUS_LABEL, between } from '@/lib/format'

const TERMINAL = ['passed', 'failed', 'infra_error', 'expired']

// While a proof runs the page ticks every second so elapsed times move
// between polls.
function useNow(active: boolean) {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!active) return
    const t = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(t)
  }, [active])
  return now
}

function LiveBand({ proof, now }: { proof: Proof; now: number }) {
  return (
    <section className="flex flex-col gap-4 rounded-[18px] border border-primary/25 bg-accent p-6 sm:flex-row sm:items-end sm:justify-between sm:p-8">
      <div>
        <p className="flex items-center gap-2 text-sm font-bold text-accent-foreground">
          <span className="size-2 rounded-full bg-primary animate-status-pulse" />Live
        </p>
        <p className="display mt-1 text-[2.2rem] sm:text-[2.8rem]">{STATUS_LABEL[proof.status]}</p>
        <p className="mt-2 max-w-lg text-[0.95rem] text-muted-foreground">
          Nobody can step in now, including you. This page updates on its own.
        </p>
      </div>
      <div className="sm:text-right">
        <p className="text-xs font-bold text-muted-foreground">Elapsed</p>
        <p className="font-mono text-3xl font-semibold">{between(proof.created_at, now)}</p>
      </div>
    </section>
  )
}

export default function ProofPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params)
  const [proof, setProof] = useState<Proof | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [retrying, setRetrying] = useState(false)
  const load = useCallback(async () => {
    try {
      const p = await api<Proof>(`/proofs/${id}`)
      setProof(p)
      setError(null)
    } catch (e) {
      setError((e as ApiError).status === 404 ? 'There is no proof with this id, or it belongs to someone else.' : friendlyMessage(e))
    }
  }, [id])
  useEffect(() => {
    void load()
    if (proof && TERMINAL.includes(proof.status)) return
    const t = setInterval(() => { void load() }, 3000)
    return () => clearInterval(t)
  }, [load, proof?.status])
  const done = proof != null && TERMINAL.includes(proof.status)
  const now = useNow(proof != null && !done)

  async function retry() {
    setRetrying(true)
    try {
      const p = await post<Proof>(`/proofs/${id}/retry`)
      setProof(p)
      setError(null)
    } catch (e) {
      setError(friendlyMessage(e))
    } finally {
      setRetrying(false)
    }
  }

  if (!proof && error) {
    return (
      <div className="space-y-4">
        <p role="alert" className="text-destructive">{error}</p>
        <Button variant="outline" render={<Link href="/app" />} nativeButton={false}><ArrowLeft />Back to overview</Button>
      </div>
    )
  }
  if (!proof) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-12 w-72" />
        <Skeleton className="h-44 rounded-[18px]" />
        <Skeleton className="h-64 rounded-[14px]" />
      </div>
    )
  }
  const retryable = proof.status === 'infra_error' || proof.status === 'expired'
  return (
    <div className="space-y-8">
      <PageHeader
        kicker={<Link href="/app" className="inline-flex items-center gap-1.5 hover:text-foreground"><ArrowLeft className="size-4" />Overview</Link>}
        title="Proof"
        actions={retryable && <Button onClick={retry} disabled={retrying}><RotateCcw />{retrying ? 'Retrying…' : 'Retry for free'}</Button>}>
        <span className="font-mono text-sm">{proof.task_slug}</span>
      </PageHeader>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      {done ? <VerdictBand proof={proof} /> : <LiveBand proof={proof} now={now} />}
      <div className="grid gap-8 lg:grid-cols-[20rem_1fr]">
        <section>
          <SectionTitle>Timeline</SectionTitle>
          <Timeline proof={proof} now={now} />
        </section>
        <div className="min-w-0 space-y-8">
          {proof.sandbox_result && proof.sandbox_result.tests.length > 0 && (
            <section>
              <SectionTitle aside={proof.sandbox_result.timed_out ? 'Timed out' : 'visible and hidden'}>Tests</SectionTitle>
              <TestResults proof={proof} />
            </section>
          )}
          {proof.diff && (
            <section>
              <SectionTitle>Diff your agent produced</SectionTitle>
              <DiffView diff={proof.diff} />
            </section>
          )}
          {done && <ProofLogs proof={proof} />}
          {!done && !proof.diff && (
            <p className="rounded-[14px] border border-dashed border-input px-5 py-8 text-center text-sm text-muted-foreground">
              The diff and the test results will appear here as soon as they arrive.
            </p>
          )}
        </div>
      </div>
    </div>
  )
}
