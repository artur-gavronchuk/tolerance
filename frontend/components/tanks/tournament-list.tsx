'use client'

import Link from 'next/link'
import { Trophy } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Countdown } from './countdown'
import { tournamentName } from '@/lib/i18n/messages/names'
import { useT } from '@/lib/i18n/client'
import { tanksHomeMessages as m } from '@/lib/i18n/messages/tanks-home'
import { formatDate } from '@/lib/i18n/core'
import type { TournamentView } from '@/lib/types'

function statusLabel(t: ReturnType<typeof useT<typeof m.en>>, s: string) {
  return s === 'scheduled' || s === 'running' || s === 'finished' || s === 'cancelled' ? t(`status.${s}`) : s
}

// One row per tournament: name, state, and who won. Used on /tanks (past champions) and /tanks/tournaments.
export function TournamentList({ items, empty }: { items: TournamentView[]; empty?: string }) {
  const t = useT(m)
  if (items.length === 0) {
    return (
      <p className="rounded-xl border border-dashed border-strong px-5 py-10 text-center text-sm text-muted-foreground">{empty ?? t('tl.empty')}</p>
    )
  }
  return (
    <ul className="divide-y divide-border rounded-xl border border-border">
      {items.map((it) => (
        <li key={it.id}>
          <Link href={`/tanks/tournaments/${it.id}`} className="flex flex-wrap items-center gap-x-3 gap-y-1 p-4 hover:bg-muted/50">
            <div className="min-w-0 flex-1">
              <p className="flex items-center gap-2 text-sm font-semibold">
                <span className="truncate">{tournamentName(t.locale, it)}</span>
                {it.open && <Badge variant="secondary" title={t('tl.openTitle')}>{t('tl.open')}</Badge>}
              </p>
              <p className="mt-0.5 text-xs text-muted-foreground">
                {it.status === 'scheduled' ? (
                  <>{t('tl.startsIn')} <Countdown to={it.starts_at} serverNow={it.now} /></>
                ) : (
                  <>{t('tl.info', { count: it.entry_count, bestOf: it.best_of, date: formatDate(t.locale, it.finished_at ?? it.starts_at) })}</>
                )}
              </p>
            </div>
            {it.champion && (
              <span className="inline-flex items-center gap-1.5 text-sm font-bold">
                <Trophy className="size-3.5 text-warning" />
                {it.champion.name}
              </span>
            )}
            <Badge variant={it.status === 'running' ? 'default' : it.status === 'scheduled' ? 'secondary' : 'outline'}>
              {statusLabel(t, it.status)}
            </Badge>
          </Link>
        </li>
      ))}
    </ul>
  )
}
