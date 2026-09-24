import Link from 'next/link'
import type { Proof } from '@/lib/types'
import { STATUS_LABEL, ago } from '@/lib/format'
import { cn } from '@/lib/utils'

export function ProofList({ items }: { items: Proof[] }) {
  if (items.length === 0) return <p className="text-sm text-muted-foreground">No proofs yet.</p>
  return (
    <ul className="divide-y divide-border rounded-md border border-border">
      {items.map((p) => (
        <li key={p.id}>
          <Link href={`/app/proofs/${p.id}`} className="flex items-center justify-between gap-3 px-4 py-3 text-sm hover:bg-muted/40">
            <span className="font-mono text-xs">{p.task_slug}</span>
            <span className={cn('text-xs', p.status === 'passed' && 'text-success', p.status === 'failed' && 'text-destructive')}>{STATUS_LABEL[p.status]}</span>
            <span className="text-xs text-muted-foreground">{ago(p.created_at)}</span>
          </Link>
        </li>
      ))}
    </ul>
  )
}
