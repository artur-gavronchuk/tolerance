'use client'

import { Download } from 'lucide-react'
import { HandleLink } from '@/components/daily/handle-link'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { ProductEntry, ProductTask } from '@/lib/types'
import { SitePreview } from './entry-card'
import { SourceViewer } from './source-viewer'
import { VoteButton } from './vote-button'

// Everything published for a task, one card per person: the live site or the scenario results, the author,
// the automated score and the vote control (checks and source stay hidden while the blind vote is on). Rank numbers are the current standings. While a site task is in
// its voting phase the gallery is blind: stable shuffled order, no ranks, authors or vote counts, so it doesn't
// undo the blind comparison above it.
export function EntryGallery({ task, entries, signedIn, busy, onToggleVote }: {
  task: ProductTask
  entries: ProductEntry[]
  signedIn: boolean
  busy: string | null
  onToggleVote: (e: ProductEntry) => void
}) {
  const site = task.kind === 'site'
  const blind = site && task.phase === 'voting'
  const shown = blind ? [...entries].sort((a, b) => hash(a.id) - hash(b.id)) : entries
  return (
    <ul className={`grid gap-5 ${site ? 'md:grid-cols-2' : ''}`}>
      {shown.map((e, i) => (
        <li key={e.id} className={`min-w-0 space-y-3 rounded-[14px] border bg-card p-4 ${e.mine ? 'border-primary' : 'border-border'}`}>
          {site && <SitePreview id={e.id} title={blind ? `Site ${i + 1}` : `${e.handle}'s site`} />}
          <div className="flex items-center gap-3">
            {blind && !e.mine ? (
              <div className="min-w-0 flex-1 truncate font-semibold">
                Site {i + 1}
                <div className="truncate text-xs font-normal text-muted-foreground">Authors are shown when voting ends</div>
              </div>
            ) : (
              <>
                {!blind && <span className="font-mono text-sm text-muted-foreground" title="Current standing">#{i + 1}</span>}
                <div className="min-w-0 flex-1">
                  <div className="truncate font-semibold">
                    {e.handle ? <HandleLink handle={e.handle} /> : 'Unknown'}
                    {e.mine && <span className="font-normal text-muted-foreground"> (you)</span>}
                  </div>
                  {e.made_with && <div className="truncate text-xs text-muted-foreground">{e.made_with}</div>}
                </div>
              </>
            )}
            {e.total > 0 && !blind && (
              <Badge variant={e.passed === e.total ? 'default' : 'outline'} title="Automated checks passed">{e.passed}/{e.total}{site ? ' checks' : ''}</Badge>
            )}
            <VoteButton entry={e} phase={task.phase} signedIn={signedIn} busy={busy === e.id} hideCount={blind} onToggle={() => onToggleVote(e)} />
          </div>
          {!site && (
            <ul className="grid gap-x-4 gap-y-1 text-sm sm:grid-cols-2">
              {e.results.map((r) => (
                <li key={r.name} className="flex min-w-0 gap-2">
                  <span className={r.passed ? 'text-primary' : 'text-destructive'} aria-label={r.passed ? 'passed' : 'failed'}>{r.passed ? '✓' : '✗'}</span>
                  <span className="min-w-0 break-words">{r.name}</span>
                </li>
              ))}
            </ul>
          )}
          {!blind && (
            <SourceViewer id={e.id}>
              <Button size="sm" variant="ghost" nativeButton={false} render={<a href={`/api/v1/product-entries/${e.id}/zip`} />}>
                <Download />Download zip
              </Button>
            </SourceViewer>
          )}
        </li>
      ))}
    </ul>
  )
}

// Stable per-entry order that doesn't follow the standings.
function hash(s: string) {
  let h = 0
  for (let i = 0; i < s.length; i++) h = (Math.imul(h, 31) + s.charCodeAt(i)) | 0
  return h
}
