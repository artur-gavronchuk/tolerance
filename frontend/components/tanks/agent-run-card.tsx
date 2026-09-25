'use client'

import { useState } from 'react'
import Link from 'next/link'
import { Bot } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { StatusPill } from '@/components/status-pill'
import { post, ApiError, friendlyMessage } from '@/lib/api'
import { ago } from '@/lib/format'
import { CLI } from '@/lib/brand'
import type { AgentOverview, Proof } from '@/lib/types'

const OPEN_STATUSES = ['queued', 'claimed', 'running_agent', 'diff_submitted', 'running_sandbox']

// Fresh presence, whatever the last check's outcome was — the same "online
// enough to start something" bar the proof-task page uses, plus `checking`
// (a run may already be open, which the block below handles separately).
const ONLINE_STAGES = ['connected', 'operational', 'checking', 'check_failed']

// The "let the agent do it" path: it gets the bot's current version (or the
// starter kit), GAME.md and its match history as a repo, same shape as a
// proof task, and its diff is judged as a new bot version instead of
// against hidden tests.
export function AgentRunCard({ agent, runs, onStarted }: {
  agent: AgentOverview | null
  runs: Proof[]
  onStarted: () => void
}) {
  const [error, setError] = useState<string | null>(null)
  const [starting, setStarting] = useState(false)
  const [justStarted, setJustStarted] = useState<Proof | null>(null)

  const open = runs.find((p) => OPEN_STATUSES.includes(p.status)) ?? justStarted
  const canStart = agent != null && ONLINE_STAGES.includes(agent.stage)

  async function start() {
    setStarting(true)
    setError(null)
    try {
      const p = await post<Proof>('/me/tanks/agent-runs')
      setJustStarted(p)
      onStarted()
    } catch (e) {
      const a = e as ApiError
      setError(a.code === 'agent_offline' ? `The connector is not online. Run \`${CLI} connect\` first.` : friendlyMessage(a))
    } finally {
      setStarting(false)
    }
  }

  return (
    <section className="rounded-[14px] border border-border bg-card p-5">
      <h2 className="heading">Agent</h2>
      <p className="mt-2 text-sm text-muted-foreground">
        Your agent gets the current bot, the rules and its match history, and sends back a diff — the same
        as a proof task, but judged as a new bot version instead of against hidden tests.
      </p>

      {!agent && (
        <p className="mt-4 text-sm">
          Create an agent first. <Link href="/app/agent/new" className="font-semibold text-primary hover:underline">Create agent</Link>
        </p>
      )}
      {agent && !canStart && (
        <p className="mt-4 text-sm">
          The connector is not online, so it can&apos;t start. Run <code className="font-mono">{CLI} connect</code> on
          your machine, or <Link href="/app/agent/connect" className="font-semibold text-primary hover:underline">see connect instructions</Link>.
        </p>
      )}
      {agent && canStart && open && (
        <p className="mt-4 text-sm">
          A run is already in progress. <Link href={`/app/proofs/${open.id}`} className="font-semibold text-primary hover:underline">Watch it</Link>
        </p>
      )}
      {agent && canStart && !open && (
        <Button className="mt-4" onClick={() => void start()} disabled={starting}>
          <Bot />{starting ? 'Starting…' : 'Let my agent write the bot'}
        </Button>
      )}
      {error && <p role="alert" className="mt-3 text-sm text-destructive">{error}</p>}

      {runs.length > 0 && (
        <div className="mt-5 border-t border-border pt-4">
          <p className="mb-2 text-xs font-bold text-muted-foreground">Recent runs</p>
          <ul className="divide-y divide-border">
            {runs.map((p) => (
              <li key={p.id}>
                <Link href={`/app/proofs/${p.id}`} className="flex items-center justify-between gap-3 py-2 text-sm hover:text-primary">
                  <StatusPill status={p.status} />
                  <span className="text-xs text-muted-foreground">{ago(p.created_at)}</span>
                </Link>
              </li>
            ))}
          </ul>
        </div>
      )}
    </section>
  )
}
