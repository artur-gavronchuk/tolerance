import { ExternalLink } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { ProductEntry } from '@/lib/types'
import { BenchTime } from './bench'
import { ago, usePT } from './phase'

const REASONS = ['invalid_zip', 'timeout', 'no_results', 'stuck'] as const

export const siteURL = (id: string) => `/api/v1/product-entries/${id}/site/index.html`

// An uploaded site in a sandboxed frame (the server also sends CSP sandbox), with a link to open it full size.
export function SitePreview({ id, title }: { id: string; title: string }) {
  const t = usePT()
  return (
    <div className="overflow-hidden rounded-[10px] border border-border">
      <iframe src={siteURL(id)} title={title} loading="lazy" sandbox="allow-scripts allow-forms allow-modals"
        className="block h-80 w-full bg-white" />
      <div className="flex justify-end border-t border-border p-2">
        <Button size="sm" variant="ghost" nativeButton={false} render={<a href={siteURL(id)} target="_blank" rel="noreferrer" />}>
          <ExternalLink />{t('entry.openFull')}
        </Button>
      </div>
    </div>
  )
}

// One of the viewer's own uploads: status, score and per-scenario results (or the site itself).
export function EntryCard({ entry: e, site = false, counts = false, preview = true, bench = false, fastest = null }: { entry: ProductEntry; site?: boolean; counts?: boolean; preview?: boolean; bench?: boolean; fastest?: number | null }) {
  const t = usePT()
  const waiting = e.status === 'queued' || e.status === 'running'
  const scored = !site || e.total > 0 // a site without scenarios is just uploaded
  return (
    <div className="min-w-0 rounded-[14px] border border-border bg-card p-4">
      <div className="flex flex-wrap items-center gap-2">
        {waiting ? <Badge variant="secondary">{e.status === 'queued' ? t('entry.queued') : t('entry.running')}…</Badge>
          : e.status === 'infra_error' ? <Badge variant="destructive">{t('entry.platformError')}</Badge>
          : !scored ? <Badge>{t('entry.uploaded')}</Badge>
          : <Badge variant={e.passed === e.total ? 'default' : 'outline'}>{t('entry.scenarios', { passed: e.passed, total: e.total })}</Badge>}
        {counts && <Badge variant="secondary" title={t('entry.countsHint')}>{t('entry.counts')}</Badge>}
        <span className="text-xs text-muted-foreground">{ago(t, e.created_at)}{e.made_with && ` · ${e.made_with}`}</span>
      </div>
      {e.failure_reason && <p className="mt-2 text-sm text-muted-foreground">{(REASONS as readonly string[]).includes(e.failure_reason) ? t(`reason.${e.failure_reason}` as 'reason.timeout') : e.failure_reason}</p>}
      {e.status === 'done' && scored && (
        <ul className="mt-3 space-y-1 text-sm">
          {e.results.map((r) => (
            <li key={r.name} className="flex gap-2">
              <span className={r.passed ? 'text-primary' : 'text-destructive'} aria-label={r.passed ? t('entry.passed') : t('entry.failed')}>{r.passed ? '✓' : '✗'}</span>
              <span className="min-w-0 break-words">{r.name}</span>
            </li>
          ))}
        </ul>
      )}
      {bench && e.status === 'done' && scored && (
        <p className="mt-3 text-sm"><span className="text-muted-foreground">{t('bench.label')}: </span><BenchTime ms={e.bench_ms} fastest={fastest} className="font-mono font-semibold" /></p>
      )}
      {site && preview && e.status === 'done' && <div className="mt-3"><SitePreview id={e.id} title={t('entry.yourSite')} /></div>}
    </div>
  )
}
