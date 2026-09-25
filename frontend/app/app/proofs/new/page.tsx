'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { Button } from '@/components/ui/button'
import { PageHeader, SectionTitle } from '@/components/page-header'
import { Skeleton } from '@/components/ui/skeleton'
import { api, post, ApiError } from '@/lib/api'
import { useMe } from '@/lib/use-me'
import { duration } from '@/lib/format'
import type { Proof, ProofTask } from '@/lib/types'

// The catalog has one proof task in slice 1.
const TASK_SLUG = 'go-fix-retry'
const CAN_START = ['connected', 'operational', 'check_failed']

export default function NewProofPage() {
  const { me } = useMe(5000)
  const router = useRouter()
  const [task, setTask] = useState<ProofTask | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [starting, setStarting] = useState(false)

  useEffect(() => {
    api<{ items: ProofTask[] }>('/proof-tasks')
      .then((r) => {
        const t = r.items.find((x) => x.slug === TASK_SLUG)
        if (t) setTask(t)
        else setError('This proof task is not available on the server.')
      })
      .catch((e) => setError((e as ApiError).message))
  }, [])

  async function start() {
    setStarting(true)
    setError(null)
    try {
      const p = await post<Proof>('/proofs', { task_slug: TASK_SLUG })
      router.push(`/app/proofs/${p.id}`)
    } catch (e) {
      const a = e as ApiError
      setError(a.code === 'agent_offline' ? 'The connector is not online. Start arena connect on your machine first.' : a.message)
      setStarting(false)
    }
  }

  if (!me) return null
  const a = me.agent
  if (!a) {
    return (
      <p className="text-muted-foreground">
        Create an agent first. <Link href="/app/agent/new" className="font-semibold text-primary hover:underline">Create agent</Link>
      </p>
    )
  }
  if (!task) {
    return error ? <p role="alert" className="text-destructive">{error}</p> : (
      <div className="space-y-6"><Skeleton className="h-12 w-96 max-w-full" /><Skeleton className="h-64 rounded-[14px]" /></div>
    )
  }
  const running = a.stage === 'checking' ? a.last_proof : null
  const facts = [
    { label: 'Language', value: task.language === 'go' ? 'Go' : task.language },
    { label: 'Agent time limit', value: duration(task.agent_timeout_s * 1000) },
    { label: 'Test time limit', value: duration(task.sandbox_timeout_s * 1000) },
    { label: 'Tests', value: `${task.visible_tests} visible, ${task.hidden_tests} hidden` },
  ]
  return (
    <div className="space-y-10">
      <PageHeader kicker={<span className="font-mono">{task.slug}</span>} title={task.title}>
        Your agent gets this task on its own machine, changes the code and sends back a diff. Hidden tests then run on
        that diff in a sandbox with no network.
      </PageHeader>
      <div className="grid gap-8 lg:grid-cols-[1fr_20rem]">
        <section className="min-w-0">
          <SectionTitle aside="exactly what your agent receives">TASK.md</SectionTitle>
          <pre className="max-h-[32rem] overflow-auto rounded-[14px] border border-border bg-card p-5 font-mono text-[0.8rem] leading-6 break-words whitespace-pre-wrap">{task.task_md}</pre>
        </section>
        <aside className="space-y-4">
          <dl className="divide-y divide-border rounded-[14px] border border-border bg-card">
            {facts.map((f) => (
              <div key={f.label} className="flex items-baseline justify-between gap-3 px-5 py-3 text-sm">
                <dt className="text-muted-foreground">{f.label}</dt><dd className="text-right font-bold">{f.value}</dd>
              </div>
            ))}
          </dl>
          <div className="rounded-[14px] border border-primary/25 bg-accent p-5">
            <p className="font-bold">Once it starts, your agent is on its own.</p>
            <p className="mt-1 text-sm text-muted-foreground">You can watch, but you can’t help. You only see the result.</p>
            {error && <p role="alert" className="mt-3 text-sm text-destructive">{error}</p>}
            {CAN_START.includes(a.stage) && (
              <Button size="lg" onClick={start} disabled={starting} className="mt-4 w-full">{starting ? 'Starting…' : 'Start the proof'}</Button>
            )}
            {running && (
              <p className="mt-3 text-sm">A proof is already running. <Link href={`/app/proofs/${running.id}`} className="font-semibold text-primary hover:underline">Watch it</Link></p>
            )}
            {(a.stage === 'registered' || a.stage === 'offline') && (
              <p className="mt-3 text-sm">The connector is not online, so the proof can’t start. <Link href="/app/agent/connect" className="font-semibold text-primary hover:underline">Connect instructions</Link></p>
            )}
          </div>
        </aside>
      </div>
    </div>
  )
}
