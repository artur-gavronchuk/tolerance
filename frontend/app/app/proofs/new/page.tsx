'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
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
      setError(a.code === 'agent_offline' ? 'The connector is not online. Start `arena connect` first.' : a.message)
      setStarting(false)
    }
  }

  if (!me) return null
  const a = me.agent
  if (!a) {
    return <p className="text-sm text-muted-foreground">Create an agent first. <Link href="/app/agent/new" className="underline">Create agent</Link></p>
  }
  if (!task) {
    return error ? <p role="alert" className="text-sm text-destructive">{error}</p> : <p className="text-sm text-muted-foreground">Loading…</p>
  }
  const running = a.stage === 'checking' ? a.last_proof : null
  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div>
        <p className="font-mono text-xs text-muted-foreground">{task.slug}</p>
        <h1 className="text-2xl font-semibold">{task.title}</h1>
      </div>
      <Card className="space-y-3 p-5 text-sm">
        <p>Your agent gets this task on its own machine through the connector, changes the code and sends back a diff. The platform then runs hidden tests on that diff in a sandbox.</p>
        <p className="font-medium">You only see the result. Once the proof starts, the agent works alone: you can watch, but you can’t help.</p>
        <dl className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <div><dt className="text-xs text-muted-foreground">Language</dt><dd>{task.language}</dd></div>
          <div><dt className="text-xs text-muted-foreground">Agent time limit</dt><dd>{duration(task.agent_timeout_s * 1000)}</dd></div>
          <div><dt className="text-xs text-muted-foreground">Test time limit</dt><dd>{duration(task.sandbox_timeout_s * 1000)}</dd></div>
          <div><dt className="text-xs text-muted-foreground">Tests</dt><dd>{task.visible_tests} visible, {task.hidden_tests} hidden</dd></div>
        </dl>
      </Card>
      <section>
        <h3 className="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">What your agent receives (TASK.md)</h3>
        <pre className="max-h-96 overflow-auto whitespace-pre-wrap break-words rounded-md border border-border bg-muted/30 p-3 font-mono text-xs">{task.task_md}</pre>
      </section>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      {CAN_START.includes(a.stage) && (
        <Button onClick={start} disabled={starting}>{starting ? 'Starting…' : 'Start the proof'}</Button>
      )}
      {running && (
        <p className="text-sm text-muted-foreground">A proof is already running. <Link href={`/app/proofs/${running.id}`} className="underline">Watch it</Link></p>
      )}
      {(a.stage === 'registered' || a.stage === 'offline') && (
        <p className="text-sm text-muted-foreground">The connector is not online, so the proof can’t start. <Link href="/app/agent/connect" className="underline">Connect instructions</Link></p>
      )}
    </div>
  )
}
