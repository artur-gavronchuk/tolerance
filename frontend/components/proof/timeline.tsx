import { Check, Loader2, X } from 'lucide-react'
import type { Proof } from '@/lib/types'
import { between, clock, duration } from '@/lib/format'
import { cn } from '@/lib/utils'

type Step = {
  label: string
  reached: (p: Proof) => boolean
  // Shown on the right: when the step happened, how long it took, or how
  // long it has been running. null shows nothing.
  note: (p: Proof, now: number) => string | null
}

const STEPS: Step[] = [
  { label: 'Queued', reached: () => true, note: (p) => clock(p.created_at) },
  {
    label: 'Connector picked it up',
    reached: (p) => p.claimed_at != null,
    note: (p) => (p.claimed_at ? clock(p.claimed_at) : null),
  },
  {
    label: 'Agent running',
    reached: (p) => ['running_agent', 'diff_submitted', 'running_sandbox', 'passed', 'failed'].includes(p.status),
    note: (p, now) =>
      p.agent_duration_ms != null ? `took ${duration(p.agent_duration_ms)}`
        : p.status === 'running_agent' && p.claimed_at ? `${between(p.claimed_at, now)} so far`
          : p.finished_at && p.claimed_at ? `took ${between(p.claimed_at, p.finished_at)}`
            : null,
  },
  {
    label: 'Diff received',
    reached: (p) => p.diff_submitted_at != null,
    note: (p) => (p.diff_submitted_at ? clock(p.diff_submitted_at) : null),
  },
  {
    label: 'Hidden tests',
    reached: (p) => p.diff_submitted_at != null && ['running_sandbox', 'passed', 'failed'].includes(p.status),
    note: (p, now) =>
      !p.diff_submitted_at ? null
        : p.finished_at ? `took ${between(p.diff_submitted_at, p.finished_at)}`
          : p.status === 'running_sandbox' ? `${between(p.diff_submitted_at, now)} so far`
            : null,
  },
  {
    label: 'Verdict',
    reached: (p) => p.finished_at != null,
    note: (p) => (p.finished_at ? clock(p.finished_at) : null),
  },
]

export function Timeline({ proof }: { proof: Proof }) {
  const terminal = proof.finished_at != null
  const dead = proof.status === 'expired' || proof.status === 'infra_error'
  const now = Date.now()
  let activeShown = false
  return (
    <ol className="flex flex-col gap-0">
      {STEPS.map((s) => {
        const done = s.reached(proof)
        const active = !done && !activeShown && !terminal
        if (active) activeShown = true
        const note = done || active ? s.note(proof, now) : null
        return (
          <li key={s.label} className="flex items-center gap-3 border-b border-border py-2.5 text-sm last:border-b-0">
            <span className={cn('flex size-5 shrink-0 items-center justify-center rounded-full border text-[10px]',
              done && !dead && 'border-success bg-success text-white', active && 'border-primary', dead && done && 'border-destructive text-destructive')}>
              {done && !dead && <Check className="size-3" strokeWidth={3} />}
              {done && dead && <X className="size-3" />}
              {active && <Loader2 className="size-3 animate-spin" />}
            </span>
            <span className={cn(done || active ? 'text-foreground' : 'text-muted-foreground')}>{s.label}</span>
            {note && <span className="ml-auto shrink-0 font-mono text-xs text-muted-foreground">{note}</span>}
          </li>
        )
      })}
    </ol>
  )
}
