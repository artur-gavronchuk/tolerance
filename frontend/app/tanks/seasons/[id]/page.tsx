'use client'

import { use, useEffect, useState } from 'react'
import Link from 'next/link'
import { ArrowLeft, Trophy } from 'lucide-react'
import { PageHeader, SectionTitle } from '@/components/page-header'
import { Countdown } from '@/components/tanks/countdown'
import { Leaderboard } from '@/components/tanks/leaderboard'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { ApiError, tanks } from '@/lib/api'
import { cn } from '@/lib/utils'
import type { SeasonDetail, SeasonView } from '@/lib/types'

export default function SeasonPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params)
  const [d, setD] = useState<SeasonDetail | null>(null)
  const [all, setAll] = useState<SeasonView[]>([])
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    setD(null)
    void tanks
      .season(id)
      .then(setD)
      .catch((e) => setError((e as ApiError).status === 404 ? 'There is no season with this id.' : (e as ApiError).message))
    void tanks.seasons().then(setAll).catch(() => {})
  }, [id])

  if (error) {
    return (
      <div className="mx-auto max-w-3xl space-y-4 px-4 py-14 sm:px-6">
        <p role="alert" className="text-destructive">{error}</p>
        <Button variant="outline" render={<Link href="/tanks" />} nativeButton={false}>
          <ArrowLeft />Back to tanks
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
          <Link href="/tanks" className="inline-flex items-center gap-1.5 hover:text-foreground">
            <ArrowLeft className="size-4" />Tanks
          </Link>
        }
        title={
          <span className="inline-flex flex-wrap items-center gap-3">
            Season {s.name}
            <Badge variant={s.status === 'active' ? 'default' : 'secondary'}>{s.status === 'active' ? 'In progress' : 'Final standings'}</Badge>
          </span>
        }
      >
        {s.status === 'active' ? (
          <>Ends in <Countdown to={s.ends_at} serverNow={d.now} />. Every bot starts the season at a fresh rating.</>
        ) : (
          <>
            {new Date(s.starts_at).toLocaleDateString(undefined, { day: 'numeric', month: 'short', timeZone: 'UTC' })} –{' '}
            {new Date(new Date(s.ends_at).getTime() - 1).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric', timeZone: 'UTC' })}.
            These standings are frozen.
          </>
        )}
      </PageHeader>

      {s.winner && (
        <div className="mt-8 flex flex-wrap items-center gap-3 rounded-[14px] border border-warning bg-warning/10 p-5">
          <Trophy className="size-6 text-warning" />
          <div>
            <p className="text-xs font-bold uppercase tracking-wide text-muted-foreground">Season winner</p>
            <Link href={`/tanks/bots/${s.winner.bot_id}`} className="text-xl font-bold hover:text-primary">{s.winner.name}</Link>
            {s.winner.owner && <span className="ml-2 text-sm text-muted-foreground">by {s.winner.owner}</span>}
            <span className="ml-2 font-mono text-sm text-muted-foreground">{s.winner.rating}</span>
          </div>
        </div>
      )}

      <section className="mt-8">
        <SectionTitle>{s.status === 'active' ? 'Standings so far' : 'Final standings'}</SectionTitle>
        <Leaderboard entries={d.standings} />
      </section>

      {all.length > 1 && (
        <section className="mt-10">
          <SectionTitle>All seasons</SectionTitle>
          <div className="flex flex-wrap gap-2">
            {all.map((x) => (
              <Link
                key={x.id}
                href={`/tanks/seasons/${x.id}`}
                className={cn(
                  'rounded-full border border-border px-3 py-1.5 text-sm font-semibold text-muted-foreground hover:text-foreground',
                  x.id === s.id && 'bg-muted text-foreground',
                )}
              >
                {x.name}
              </Link>
            ))}
          </div>
        </section>
      )}
    </div>
  )
}
