'use client'

import { use, useEffect, useState } from 'react'
import Link from 'next/link'
import { ArrowLeft, Trophy } from 'lucide-react'
import { PageHeader, SectionTitle } from '@/components/page-header'
import { Bracket } from '@/components/tanks/bracket'
import { Countdown } from '@/components/tanks/countdown'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { ApiError, tanks } from '@/lib/api'
import type { TournamentView } from '@/lib/types'

export default function TournamentPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params)
  const [t, setT] = useState<TournamentView | null>(null)
  const [error, setError] = useState<string | null>(null)
  const live = t == null || t.status === 'scheduled' || t.status === 'running'

  useEffect(() => {
    let alive = true
    const load = () =>
      void tanks
        .tournament(id)
        .then((r) => alive && setT(r))
        .catch((e) => alive && setError((e as ApiError).status === 404 ? 'There is no tournament with this id.' : (e as ApiError).message))
    load()
    if (!live) return () => { alive = false }
    const timer = setInterval(load, 4000)
    return () => {
      alive = false
      clearInterval(timer)
    }
  }, [id, live])

  if (error) {
    return (
      <div className="mx-auto max-w-3xl space-y-4 px-4 py-14 sm:px-6">
        <p role="alert" className="text-destructive">{error}</p>
        <Button variant="outline" render={<Link href="/tanks/tournaments" />} nativeButton={false}>
          <ArrowLeft />All tournaments
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
            <ArrowLeft className="size-4" />Tournaments
          </Link>
        }
        title={t.name}
        actions={
          <Badge variant={t.status === 'running' ? 'default' : 'secondary'} className="self-start">
            {t.status === 'scheduled' ? 'Upcoming' : t.status === 'running' ? 'Live' : t.status === 'finished' ? 'Finished' : 'Cancelled'}
          </Badge>
        }
      >
        Single elimination among the top {t.size} bots of the season ladder, every pairing a best of {t.best_of} on different maps.
        {t.season_id && (
          <>
            {' '}Season <Link href={`/tanks/seasons/${t.season_id}`} className="font-semibold text-primary hover:underline">{t.season_id}</Link>.
          </>
        )}
      </PageHeader>

      {t.status === 'scheduled' && (
        <p className="mt-8 rounded-[14px] border border-border bg-card p-6 text-center">
          <span className="block text-sm text-muted-foreground">The bracket is drawn from the season ladder in</span>
          <span className="mt-1 block text-3xl font-bold"><Countdown to={t.starts_at} serverNow={t.now} done="any moment" /></span>
        </p>
      )}

      {t.status === 'cancelled' && (
        <p className="mt-8 rounded-[14px] border border-dashed border-input p-6 text-center text-sm text-muted-foreground">
          Cancelled: fewer than two ranked bots.
        </p>
      )}

      {t.champion && (
        <div className="mt-8 flex flex-wrap items-center gap-3 rounded-[14px] border border-warning bg-warning/10 p-5">
          <Trophy className="size-6 text-warning" />
          <div>
            <p className="text-xs font-bold uppercase tracking-wide text-muted-foreground">Champion</p>
            <Link href={`/tanks/bots/${t.champion.bot_id}`} className="text-xl font-bold hover:text-primary">
              {t.champion.name}
            </Link>
            {t.champion.owner && <span className="ml-2 text-sm text-muted-foreground">by {t.champion.owner}</span>}
          </div>
        </div>
      )}

      {(t.pairings?.length ?? 0) > 0 && (
        <section className="mt-8">
          <SectionTitle aside={t.status === 'running' ? 'Updates live' : undefined}>Bracket</SectionTitle>
          <Bracket t={t} />
        </section>
      )}

      {(t.entries?.length ?? 0) > 0 && (
        <section className="mt-10">
          <SectionTitle>Entrants</SectionTitle>
          <ul className="divide-y divide-border rounded-[14px] border border-border">
            {t.entries!.map((e) => (
              <li key={e.bot_id} className="flex items-center gap-3 p-3 text-sm">
                <span className="w-6 shrink-0 font-mono text-muted-foreground">#{e.seed}</span>
                <Link href={`/tanks/bots/${e.bot_id}`} className="min-w-0 flex-1 truncate font-semibold hover:text-primary">
                  {e.name}
                </Link>
                {e.owner && <span className="text-xs text-muted-foreground">{e.owner}</span>}
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  )
}
