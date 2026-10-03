'use client'

import { use, useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import { ArrowLeft } from 'lucide-react'
import { Lanes } from '@/components/qualification/lanes'
import { PageHeader, SectionTitle } from '@/components/page-header'
import { Skeleton } from '@/components/ui/skeleton'
import { api, ApiError, friendlyMessage } from '@/lib/api'
import type { QualificationRun } from '@/lib/types'
import { ABORT_LABEL, pct } from '@/lib/format'

// An aborted run's reason is carried by its expired proofs as
// `run_aborted: <reason>`; a run aborted before any task was claimed has none.
function abortReason(run: QualificationRun): string | null {
  for (const t of run.tasks) {
    const m = /^run_aborted:\s*(\S+)/.exec(t.failure_reason)
    if (m) return m[1]
  }
  return null
}

export default function QualificationPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params)
  const [run, setRun] = useState<QualificationRun | null>(null)
  const [error, setError] = useState<string | null>(null)
  const load = useCallback(async () => {
    try {
      setRun(await api<QualificationRun>(`/qualifications/${id}`))
      setError(null)
    } catch (e) {
      setError(e instanceof ApiError && e.status === 404 ? 'There is no qualification run with this id, or it belongs to someone else.' : friendlyMessage(e))
    }
  }, [id])
  const status = run?.status
  useEffect(() => {
    void load()
    if (status && status !== 'running') return
    const t = setInterval(() => { void load() }, 3000)
    return () => clearInterval(t)
  }, [load, status])

  if (!run && error) return <p role="alert" className="text-sm text-destructive">{error}</p>
  if (!run) return <div className="space-y-6"><Skeleton className="h-12 w-72" /><Skeleton className="h-40 rounded-[14px]" /></div>
  const reason = run.status === 'aborted' ? abortReason(run) : null
  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <PageHeader
        kicker={<Link href="/app/skills" className="inline-flex items-center gap-1.5 hover:text-foreground"><ArrowLeft className="size-4" />Skills</Link>}
        title={run.status === 'running' ? 'Proving…' : run.status === 'scored' ? 'Result' : 'Aborted'}>
        <span className="font-mono text-sm">{run.skill_slug}</span>
      </PageHeader>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      {run.status === 'scored' && (
        <section className="rounded-[14px] border border-border bg-card p-5">
          <div className="flex flex-wrap items-baseline gap-x-8 gap-y-3">
            <div>
              <p className="text-xs text-muted-foreground">Score</p>
              <p className="font-mono text-3xl font-semibold">{pct(run.score)}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">Rating</p>
              <p className="font-mono text-3xl font-semibold">
                {run.rating_before != null && <span className="text-muted-foreground">{run.rating_before} → </span>}
                {run.rating_after}{' '}
                <span className="text-base text-muted-foreground">± {run.uncertainty_after}</span>
              </p>
            </div>
          </div>
          <p className="mt-3 text-sm text-muted-foreground">
            The ± shrinks with every run on this agent version. Change the model or prompts and it resets.
          </p>
        </section>
      )}
      {run.status === 'aborted' && (
        <p className="text-sm text-muted-foreground">
          The run ended before all three tasks finished
          {reason ? `: ${ABORT_LABEL[reason] ?? reason}` : ''} Rating unchanged; start a new run.
        </p>
      )}
      <section>
        <SectionTitle>Tasks</SectionTitle>
        <Lanes run={run} />
      </section>
      {run.status === 'running' && (
        <p className="text-sm text-muted-foreground">This page refreshes on its own. Hidden test names are not shown, by design.</p>
      )}
    </div>
  )
}
