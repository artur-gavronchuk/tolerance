import { Check, Loader2, X } from 'lucide-react'
import type { Proof } from '@/lib/types'
import { cn } from '@/lib/utils'

const STEPS: { label: string; reached: (p: Proof) => boolean }[] = [
  { label: 'Queued', reached: () => true },
  { label: 'Connector picked it up', reached: (p) => p.claimed_at != null },
  { label: 'Agent running', reached: (p) => ['running_agent', 'diff_submitted', 'running_sandbox', 'passed', 'failed'].includes(p.status) },
  { label: 'Diff received', reached: (p) => p.diff_submitted_at != null },
  { label: 'Hidden tests', reached: (p) => ['running_sandbox', 'passed', 'failed'].includes(p.status) },
  { label: 'Verdict', reached: (p) => p.finished_at != null },
]

export function Timeline({ proof }: { proof: Proof }) {
  const terminal = proof.finished_at != null
  const dead = proof.status === 'expired' || proof.status === 'infra_error'
  let activeShown = false
  return (
    <ol className="flex flex-col gap-0">
      {STEPS.map((s) => {
        const done = s.reached(proof)
        const active = !done && !activeShown && !terminal
        if (active) activeShown = true
        return (
          <li key={s.label} className="flex items-center gap-3 border-b border-border py-2.5 text-sm last:border-b-0">
            <span className={cn('flex size-5 items-center justify-center rounded-full border text-[10px]',
              done && !dead && 'border-success bg-success text-white', active && 'border-primary', dead && done && 'border-destructive text-destructive')}>
              {done && !dead && <Check className="size-3" strokeWidth={3} />}
              {done && dead && <X className="size-3" />}
              {active && <Loader2 className="size-3 animate-spin" />}
            </span>
            <span className={cn(done || active ? 'text-foreground' : 'text-muted-foreground')}>{s.label}</span>
          </li>
        )
      })}
    </ol>
  )
}
