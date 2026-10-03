'use client'

import { errorText } from '@/lib/i18n/messages/errors'
import { use, useEffect, useState } from 'react'
import Link from 'next/link'
import { ArrowLeft, Trophy } from 'lucide-react'
import { PageHeader, SectionTitle } from '@/components/page-header'
import { Bracket } from '@/components/tanks/bracket'
import { Countdown } from '@/components/tanks/countdown'
import { HandleLink } from '@/components/daily/handle-link'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { ApiError, tanks } from '@/lib/api'
import { seasonNameFromId, tournamentName } from '@/lib/i18n/messages/names'
import { useT } from '@/lib/i18n/client'
import { tanksHomeMessages as m } from '@/lib/i18n/messages/tanks-home'
import type { TournamentView } from '@/lib/types'

export default function TournamentPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params)
  const tr = useT(m)
  const [t, setT] = useState<TournamentView | null>(null)
  const [error, setError] = useState<string | null>(null)
  const live = t == null || t.status === 'scheduled' || t.status === 'running'

  useEffect(() => {
    let alive = true
    const load = () =>
      void tanks
        .tournament(id)
        .then((r) => alive && setT(r))
        .catch((e) => alive && setError((e as ApiError).status === 404 ? tr('tour.notFound') : errorText(e, tr.locale)))
    load()
    if (!live) return () => { alive = false }
    const timer = setInterval(load, 4000)
    return () => {
      alive = false
      clearInterval(timer)
    }
  }, [id, live, tr])

  if (error) {
    return (
      <div className="mx-auto max-w-3xl space-y-4 px-4 py-14 sm:px-6">
        <p role="alert" className="text-destructive">{error}</p>
        <Button variant="outline" render={<Link href="/tanks/tournaments" />} nativeButton={false}>
          <ArrowLeft />{tr('tour.all')}
        </Button>
      </div>
    )
  }
  if (!t) {
    return (
      <div className="mx-auto max-w-5xl space-y-6 px-4 py-14 sm:px-6">
        <Skeleton className="h-10 w-72" />
        <Skeleton className="h-64 rounded-[14px]" />
      </div>
    )
  }

  return (
    <div className="mx-auto max-w-6xl px-4 py-10 sm:px-6 sm:py-14">
      <PageHeader
        kicker={
          <Link href="/tanks/tournaments" className="inline-flex items-center gap-1.5 hover:text-foreground">
            <ArrowLeft className="size-4" />{tr('tour.tournaments')}
          </Link>
        }
        title={tournamentName(tr.locale, t)}
        actions={
          <Badge variant={t.status === 'running' ? 'default' : 'secondary'} className="self-start">
            {tr(`status.${t.status}` as 'status.scheduled')}
          </Badge>
        }
      >
        {tr('tour.intro', { size: t.size, bestOf: t.best_of })}
        {t.season_id && (
          <>
            {' '}{tr('tour.season')} <Link href={`/tanks/seasons/${t.season_id}`} className="font-semibold text-primary hover:underline">{seasonNameFromId(tr.locale, t.season_id)}</Link>.
          </>
        )}
      </PageHeader>

      {t.status === 'scheduled' && (
        <p className="mt-8 rounded-[14px] border border-border bg-card p-6 text-center">
          <span className="block text-sm text-muted-foreground">{tr('tour.drawnIn')}</span>
          <span className="mt-1 block text-3xl font-bold"><Countdown to={t.starts_at} serverNow={t.now} done={tr('tour.anyMoment')} /></span>
        </p>
      )}

      {t.status === 'cancelled' && (
        <p className="mt-8 rounded-[14px] border border-dashed border-strong p-6 text-center text-sm text-muted-foreground">
          {tr('tour.cancelled')}
        </p>
      )}

      {t.champion && (
        <div className="mt-8 flex flex-wrap items-center gap-3 rounded-[14px] border border-warning bg-warning/10 p-5">
          <Trophy className="size-6 text-warning" />
          <div>
            <p className="text-xs font-bold uppercase tracking-wide text-muted-foreground">{tr('tour.champion')}</p>
            <Link href={`/tanks/bots/${t.champion.bot_id}`} className="text-xl font-bold hover:text-primary">
              {t.champion.name}
            </Link>
            {t.champion.owner && <span className="ml-2 text-sm text-muted-foreground">{tr('home.by')} <HandleLink handle={t.champion.owner} /></span>}
          </div>
        </div>
      )}

      {(t.pairings?.length ?? 0) > 0 && (
        <section className="mt-8">
          <SectionTitle aside={t.status === 'running' ? tr('tour.updatesLive') : undefined}>{tr('tour.bracket')}</SectionTitle>
          <Bracket t={t} />
        </section>
      )}

      {(t.entries?.length ?? 0) > 0 && (
        <section className="mt-10">
          <SectionTitle>{tr('tour.entrants')}</SectionTitle>
          <ul className="divide-y divide-border rounded-[14px] border border-border">
            {t.entries!.map((e) => (
              <li key={e.bot_id} className="flex items-center gap-3 p-3 text-sm">
                <span className="w-6 shrink-0 font-mono text-muted-foreground">#{e.seed}</span>
                <Link href={`/tanks/bots/${e.bot_id}`} className="min-w-0 flex-1 truncate font-semibold hover:text-primary">
                  {e.name}
                </Link>
                {e.owner && <span className="text-xs text-muted-foreground"><HandleLink handle={e.owner} /></span>}
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  )
}
