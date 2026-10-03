import Link from 'next/link'
import { Trophy } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Countdown } from './countdown'
import type { TournamentView } from '@/lib/types'

const STATUS_LABEL: Record<string, string> = { scheduled: 'Upcoming', running: 'Live', finished: 'Finished', cancelled: 'Cancelled' }

function when(t: TournamentView): string {
  return new Date(t.finished_at ?? t.starts_at).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' })
}

// One row per tournament: name, state, and who won. Used on /tanks (past champions) and /tanks/tournaments.
export function TournamentList({ items, empty = 'No tournaments yet.' }: { items: TournamentView[]; empty?: string }) {
  if (items.length === 0) {
    return (
      <p className="rounded-[14px] border border-dashed border-input px-5 py-10 text-center text-sm text-muted-foreground">{empty}</p>
    )
  }
  return (
    <ul className="divide-y divide-border rounded-[14px] border border-border">
      {items.map((t) => (
        <li key={t.id}>
          <Link href={`/tanks/tournaments/${t.id}`} className="flex flex-wrap items-center gap-x-3 gap-y-1 p-4 hover:bg-muted/50">
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-semibold">{t.name}</p>
              <p className="mt-0.5 text-xs text-muted-foreground">
                {t.status === 'scheduled' ? (
                  <>Starts in <Countdown to={t.starts_at} serverNow={t.now} /></>
                ) : (
                  <>{t.entry_count} bots · best of {t.best_of} · {when(t)}</>
                )}
              </p>
            </div>
            {t.champion && (
              <span className="inline-flex items-center gap-1.5 text-sm font-bold">
                <Trophy className="size-3.5 text-warning" />
                {t.champion.name}
              </span>
            )}
            <Badge variant={t.status === 'running' ? 'default' : t.status === 'scheduled' ? 'secondary' : 'outline'}>
              {STATUS_LABEL[t.status] ?? t.status}
            </Badge>
          </Link>
        </li>
      ))}
    </ul>
  )
}
