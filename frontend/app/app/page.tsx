'use client'

import { useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import { Journey } from '@/components/journey'
import { PageHeader, SectionTitle } from '@/components/page-header'
import { ProofList } from '@/components/proof-list'
import { StageCard } from '@/components/stage-card'
import { PRESENCE_LABEL, StatusDot } from '@/components/status-dot'
import { Skeleton } from '@/components/ui/skeleton'
import { api, ApiError } from '@/lib/api'
import { ago } from '@/lib/format'
import { useMe } from '@/lib/use-me'
import type { AgentOverview, Proof } from '@/lib/types'

function ConnectorCard({ a }: { a: AgentOverview }) {
  const p = a.presence
  return (
    <section className="rounded-[14px] border border-border bg-card p-5">
      <div className="flex items-center justify-between gap-3">
        <h2 className="heading">Connector</h2>
        <span className="flex items-center gap-2 text-sm font-semibold"><StatusDot stage={a.stage} />{PRESENCE_LABEL[a.stage]}</span>
      </div>
      {p ? (
        <dl className="mt-4 space-y-2.5 text-sm">
          <div className="flex justify-between gap-3"><dt className="text-muted-foreground">Host</dt><dd className="truncate font-mono text-xs">{p.hostname}</dd></div>
          <div className="flex justify-between gap-3"><dt className="text-muted-foreground">Version</dt><dd className="font-mono text-xs">{p.connector_version}</dd></div>
          <div className="flex justify-between gap-3"><dt className="text-muted-foreground">Last seen</dt><dd>{ago(p.last_seen_at)}</dd></div>
        </dl>
      ) : (
        <p className="mt-3 text-sm text-muted-foreground">It has never connected. Setup takes three commands.</p>
      )}
      <Link href="/app/agent/connect" className="mt-4 inline-block text-sm font-bold text-primary hover:underline">
        {p ? 'Setup and API keys' : 'Show connect instructions'}
      </Link>
    </section>
  )
}

function Record({ proofs }: { proofs: Proof[] }) {
  const done = proofs.filter((p) => p.status === 'passed' || p.status === 'failed')
  const passed = done.filter((p) => p.status === 'passed').length
  return (
    <section className="rounded-[14px] border border-border bg-card p-5">
      <h2 className="heading">Record</h2>
      <div className="mt-3 flex items-baseline gap-2">
        <span className="text-[2.4rem] leading-none font-extrabold tracking-[-0.04em]">{passed}</span>
        <span className="text-muted-foreground">of {done.length} judged proofs passed</span>
      </div>
      {done.length > 0 && (
        <div className="mt-4 flex h-2 overflow-hidden rounded-full bg-muted" aria-hidden>
          <span className="bg-success" style={{ width: `${(passed / done.length) * 100}%` }} />
          <span className="flex-1 bg-destructive/60" />
        </div>
      )}
      <p className="mt-3 text-xs text-muted-foreground">Platform errors and expired proofs are not counted.</p>
    </section>
  )
}

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
  const a = me.agent
  return (
    <div className="space-y-10">
      <PageHeader
        kicker={a ? <span className="flex items-center gap-2"><StatusDot stage={a.stage} />{PRESENCE_LABEL[a.stage]}{a.presence && `, seen ${ago(a.presence.last_seen_at)}`}</span> : 'Welcome'}
        title={a ? a.name : 'Your agent starts here'}>
        {a ? a.description || 'Your agent’s proofs and connection, updated live.' : 'Create an agent, connect it from your machine and let it prove it can fix code on its own.'}
      </PageHeader>
      <div className="grid gap-8 lg:grid-cols-[1fr_20rem]">
        <div className="min-w-0 space-y-10">
          <StageCard me={me} />
          {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
          {a && (
            <section>
              <SectionTitle aside={proofs && proofs.length > 0 ? `${proofs.length} total` : undefined}>Proof history</SectionTitle>
              {proofs ? <ProofList items={proofs} /> : <Skeleton className="h-40 rounded-[14px]" />}
            </section>
          )}
        </div>
        <aside className="space-y-4">
          <div>
            <SectionTitle>Path</SectionTitle>
            <Journey me={me} proofs={proofs} />
          </div>
          {a && <ConnectorCard a={a} />}
          {a && proofs && <Record proofs={proofs} />}
        </aside>
      </div>
    </div>
  )
}
