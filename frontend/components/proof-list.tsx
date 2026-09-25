import Link from 'next/link'
import { ChevronRight } from 'lucide-react'
import { StatusPill } from '@/components/status-pill'
import type { Proof } from '@/lib/types'
import { ago, duration } from '@/lib/format'

// The narrow-screen summary line: only the facts this proof already has.
function summary(p: Proof) {
  const parts = [p.task_slug]
  if (p.sandbox_result?.tests.length) parts.push(`${tests(p)} tests`)
  if (p.agent_duration_ms != null) parts.push(`agent ${duration(p.agent_duration_ms)}`)
  return parts.join(', ')
}

function tests(p: Proof) {
  const t = p.sandbox_result?.tests
  return t && t.length ? `${t.filter((x) => x.passed).length}/${t.length}` : '—'
}

export function ProofList({ items }: { items: Proof[] }) {
  if (items.length === 0) {
    return (
      <div className="rounded-[14px] border border-dashed border-input px-5 py-8 text-center text-sm text-muted-foreground">
        No proofs yet. Your first one will show up here with its verdict.
      </div>
    )
  }
  return (
    <div className="overflow-hidden rounded-[14px] border border-border bg-card">
      <div className="hidden grid-cols-[8.5rem_1fr_4.5rem_5.5rem_5.5rem_1rem] gap-4 border-b border-border px-5 py-2.5 text-xs font-bold text-muted-foreground sm:grid">
        <span>Result</span><span>Task</span><span className="text-right">Tests</span><span className="text-right">Agent time</span><span className="text-right">Started</span><span />
      </div>
      <ul className="divide-y divide-border">
        {items.map((p) => (
          <li key={p.id}>
            <Link href={`/app/proofs/${p.id}`}
              className="grid grid-cols-[1fr_auto] items-center gap-x-4 gap-y-1.5 px-5 py-3.5 text-sm transition-colors hover:bg-muted/60 sm:grid-cols-[8.5rem_1fr_4.5rem_5.5rem_5.5rem_1rem]">
              <span><StatusPill status={p.status} /></span>
              <span className="order-3 col-span-2 truncate font-mono text-xs text-muted-foreground sm:order-none sm:col-span-1">
                <span className="hidden sm:inline">{p.task_slug}</span>
                <span className="sm:hidden">{summary(p)}</span>
              </span>
              <span className="hidden text-right font-mono text-xs sm:block">{tests(p)}</span>
              <span className="hidden text-right font-mono text-xs sm:block">{duration(p.agent_duration_ms)}</span>
              <span className="text-right text-xs text-muted-foreground">{ago(p.created_at)}</span>
              <ChevronRight className="hidden size-4 text-muted-foreground sm:block" />
            </Link>
          </li>
        ))}
      </ul>
    </div>
  )
}
