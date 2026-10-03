import Link from 'next/link'
import { Loader2, Check, X } from 'lucide-react'
import type { QualificationRun, Proof } from '@/lib/types'
import { STATUS_LABEL, REASON_LABEL } from '@/lib/format'
import { cn } from '@/lib/utils'

const DONE = ['passed', 'failed', 'infra_error', 'expired']

// A position can hold several proofs (an infra_error is requeued once); the
// latest one is the one that counts.
function latest(run: QualificationRun, position: number): Proof | undefined {
  return [...run.tasks].reverse().find((t) => t.position === position)
}

function reasonText(reason: string) {
  if (REASON_LABEL[reason]) return REASON_LABEL[reason]
  if (reason.startsWith('run_aborted:')) return REASON_LABEL.run_aborted
  return reason
}

export function Lanes({ run }: { run: QualificationRun }) {
  return (
    <ol className="flex flex-col gap-2">
      {[1, 2, 3].map((pos) => {
        const p = latest(run, pos)
        const done = !!p && DONE.includes(p.status)
        const tests = p?.sandbox_result?.tests
        const passed = tests ? tests.filter((t) => t.passed).length : null
        return (
          <li key={pos} className="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-md border border-border p-3 text-sm">
            <span className={cn('flex size-6 shrink-0 items-center justify-center rounded-full border text-xs',
              done && p!.status === 'passed' && 'border-success text-success',
              done && p!.status !== 'passed' && 'border-destructive text-destructive',
              !done && p && 'border-primary')}>
              {!p ? pos : done ? (p.status === 'passed' ? <Check className="size-3" /> : <X className="size-3" />) : <Loader2 className="size-3 animate-spin" />}
            </span>
            <div className="min-w-0 flex-1">
              <p className="font-medium">Task {pos}{p?.skill_task_slug ? <span className="ml-2 font-mono text-xs break-all text-muted-foreground">{p.skill_task_slug}</span> : null}</p>
              <p className="text-xs text-muted-foreground">
                {!p ? (run.status === 'aborted' ? 'Not started' : 'Waiting for the previous task') : STATUS_LABEL[p.status]}
                {p?.failure_reason ? ` · ${reasonText(p.failure_reason)}` : ''}
              </p>
            </div>
            {passed != null && tests && <span className="font-mono text-xs">{passed}/{tests.length} hidden</span>}
            {p && <Link href={`/app/proofs/${p.id}`} className="text-xs text-muted-foreground hover:text-foreground">details</Link>}
          </li>
        )
      })}
    </ol>
  )
}
