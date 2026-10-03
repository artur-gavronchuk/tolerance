import { Badge } from '@/components/ui/badge'
import { ago } from '@/lib/format'
import type { ProductEntry } from '@/lib/types'

const REASONS: Record<string, string> = {
  invalid_zip: 'The zip could not be read.',
  timeout: 'The tool timed out.',
  no_results: 'The scenarios did not run (the tool crashed the runner or is missing).',
  stuck: 'The platform could not run this entry; it does not count.',
}

// One of the viewer's own uploads: status, score and per-scenario results.
export function EntryCard({ entry: e }: { entry: ProductEntry }) {
  const waiting = e.status === 'queued' || e.status === 'running'
  return (
    <div className="min-w-0 rounded-[14px] border border-border bg-card p-4">
      <div className="flex flex-wrap items-center gap-2">
        {waiting ? <Badge variant="secondary">{e.status === 'queued' ? 'Queued' : 'Running'}…</Badge>
          : e.status === 'infra_error' ? <Badge variant="destructive">Platform error</Badge>
          : <Badge variant={e.passed === e.total ? 'default' : 'outline'}>{e.passed}/{e.total} scenarios</Badge>}
        <span className="text-xs text-muted-foreground">{ago(e.created_at)}{e.made_with && ` · ${e.made_with}`}</span>
      </div>
      {e.failure_reason && <p className="mt-2 text-sm text-muted-foreground">{REASONS[e.failure_reason] ?? e.failure_reason}</p>}
      {e.status === 'done' && (
        <ul className="mt-3 space-y-1 text-sm">
          {e.results.map((r) => (
            <li key={r.name} className="flex gap-2">
              <span className={r.passed ? 'text-primary' : 'text-destructive'} aria-label={r.passed ? 'passed' : 'failed'}>{r.passed ? '✓' : '✗'}</span>
              <span className="min-w-0 break-words">{r.name}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
