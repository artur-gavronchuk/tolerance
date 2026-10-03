import { ExternalLink } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ago } from '@/lib/format'
import type { ProductEntry } from '@/lib/types'

const REASONS: Record<string, string> = {
  invalid_zip: 'The zip could not be read.',
  timeout: 'The tool timed out.',
  no_results: 'The scenarios did not run (the tool crashed the runner or is missing).',
  stuck: 'The platform could not run this entry; it does not count.',
}

export const siteURL = (id: string) => `/api/v1/product-entries/${id}/site/index.html`

// An uploaded site in a sandboxed frame (the server also sends CSP sandbox), with a link to open it full size.
export function SitePreview({ id, title }: { id: string; title: string }) {
  return (
    <div className="overflow-hidden rounded-[10px] border border-border">
      <iframe src={siteURL(id)} title={title} loading="lazy" sandbox="allow-scripts allow-forms allow-modals"
        className="block h-80 w-full bg-white" />
      <div className="flex justify-end border-t border-border p-2">
        <Button size="sm" variant="ghost" nativeButton={false} render={<a href={siteURL(id)} target="_blank" rel="noreferrer" />}>
          <ExternalLink />Open full size
        </Button>
      </div>
    </div>
  )
}

// One of the viewer's own uploads: status, score and per-scenario results (or the site itself).
export function EntryCard({ entry: e, site = false }: { entry: ProductEntry; site?: boolean }) {
  if (site) {
    return (
      <div className="min-w-0 space-y-3 rounded-[14px] border border-border bg-card p-4">
        <div className="flex flex-wrap items-center gap-2">
          <Badge>Uploaded</Badge>
          <span className="text-xs text-muted-foreground">{ago(e.created_at)}{e.made_with && ` · ${e.made_with}`}</span>
        </div>
        <SitePreview id={e.id} title="Your site" />
      </div>
    )
  }
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
