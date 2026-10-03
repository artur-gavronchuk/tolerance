'use client'

import { errorText } from '@/lib/i18n/messages/errors'
import { use, useEffect, useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { ArrowLeft, Trophy } from 'lucide-react'
import { PageHeader, SectionTitle } from '@/components/page-header'
import { Countdown } from '@/components/tanks/countdown'
import { Leaderboard } from '@/components/tanks/leaderboard'
import { HandleLink } from '@/components/daily/handle-link'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { ApiError, tanks } from '@/lib/api'
import { cn } from '@/lib/utils'
import { seasonName } from '@/lib/i18n/messages/names'
import { useT } from '@/lib/i18n/client'
import { tanksHomeMessages as m } from '@/lib/i18n/messages/tanks-home'
import { formatDate } from '@/lib/i18n/core'
import type { SeasonDetail, SeasonView } from '@/lib/types'

export default function SeasonPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params)
  const tr = useT(m)
  const router = useRouter()
  const [d, setD] = useState<SeasonDetail | null>(null)
  const [all, setAll] = useState<SeasonView[]>([])
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    setD(null)
    // The current season is the Ladder page; this page is for archived seasons.
    if (id === 'current') {
      router.replace('/tanks/leaderboard')
      return
    }
    void tanks
      .season(id)
      .then((r) => {
        if (r.season.status === 'active') router.replace('/tanks/leaderboard')
        else setD(r)
      })
      .catch((e) => setError((e as ApiError).status === 404 ? tr('season.notFound') : errorText(e, tr.locale)))
    void tanks.seasons().then(setAll).catch(() => {})
  }, [id, router, tr])

  if (error) {
    return (
      <div className="mx-auto max-w-3xl space-y-4 px-4 py-14 sm:px-6">
        <p role="alert" className="text-destructive">{error}</p>
        <Button variant="outline" render={<Link href="/tanks/leaderboard" />} nativeButton={false}>
          <ArrowLeft />{tr('season.back')}
        </Button>
      </div>
    )
  }
  if (!d) {
    return (
      <div className="mx-auto max-w-4xl space-y-6 px-4 py-14 sm:px-6">
        <Skeleton className="h-10 w-72" />
        <Skeleton className="h-64 rounded-[14px]" />
      </div>
    )
  }

  const s = d.season
  return (
    <div className="mx-auto max-w-4xl px-4 py-10 sm:px-6 sm:py-14">
      <PageHeader
        kicker={
          <Link href="/tanks/leaderboard" className="inline-flex items-center gap-1.5 hover:text-foreground">
            <ArrowLeft className="size-4" />{tr('season.ladder')}
          </Link>
        }
        title={
          <span className="inline-flex flex-wrap items-center gap-3">
            {tr('season.title', { name: seasonName(tr.locale, s.starts_at) })}
            <Badge variant={s.status === 'active' ? 'default' : 'secondary'}>{s.status === 'active' ? tr('season.inProgress') : tr('season.final')}</Badge>
          </span>
        }
      >
        {s.status === 'active' ? (
          <>{tr('season.endsIn')} <Countdown to={s.ends_at} serverNow={d.now} />. {tr('season.freshRating')}</>
        ) : (
          <>
            {formatDate(tr.locale, s.starts_at, { day: 'numeric', month: 'short' })} –{' '}
            {formatDate(tr.locale, new Date(new Date(s.ends_at).getTime() - 1).toISOString())}.{' '}
            {tr('season.frozen')}
          </>
        )}
      </PageHeader>

      {s.winner && (
        <div className="mt-8 flex flex-wrap items-center gap-3 rounded-[14px] border border-warning bg-warning/10 p-5">
          <Trophy className="size-6 text-warning" />
          <div>
            <p className="text-xs font-bold uppercase tracking-wide text-muted-foreground">{tr('season.winner')}</p>
            <Link href={`/tanks/bots/${s.winner.bot_id}`} className="text-xl font-bold hover:text-primary">{s.winner.name}</Link>
            {s.winner.owner && <span className="ml-2 text-sm text-muted-foreground">{tr('home.by')} <HandleLink handle={s.winner.owner} /></span>}
            <span className="ml-2 font-mono text-sm text-muted-foreground">{s.winner.rating}</span>
          </div>
        </div>
      )}

      <section className="mt-8">
        <SectionTitle>{s.status === 'active' ? tr('season.standingsSoFar') : tr('season.final')}</SectionTitle>
        <Leaderboard entries={d.standings} />
        {d.total > d.standings.length && (
          <p className="mt-3 text-center text-sm text-muted-foreground">{tr('season.topOf', { shown: d.standings.length, total: d.total })}</p>
        )}
      </section>

      {all.length > 1 && (
        <section className="mt-10">
          <SectionTitle>{tr('season.all')}</SectionTitle>
          <nav aria-label={tr('season.seasons')} className="flex flex-wrap gap-2">
            {all.map((x) => (
              <Link
                key={x.id}
                href={x.status === 'active' ? '/tanks/leaderboard' : `/tanks/seasons/${x.id}`}
                className={cn(
                  'rounded-full border border-border px-3 py-1.5 text-sm font-semibold text-muted-foreground hover:text-foreground',
                  x.id === s.id && 'bg-muted text-foreground',
                )}
              >
                {seasonName(tr.locale, x.starts_at)}
              </Link>
            ))}
          </nav>
        </section>
      )}
    </div>
  )
}
