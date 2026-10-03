'use client'

import { HandleLink } from '@/components/daily/handle-link'
import { useEffect, useState } from 'react'
import Link from 'next/link'
import { CalendarClock, Trophy } from 'lucide-react'
import { Bracket } from '@/components/tanks/bracket'
import { Countdown } from '@/components/tanks/countdown'
import { Live } from '@/components/tanks/live'
import { Leaderboard } from '@/components/tanks/leaderboard'
import { MatchList } from '@/components/tanks/match-list'
import { TournamentList } from '@/components/tanks/tournament-list'
import { SectionTitle } from '@/components/page-header'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { loginHref } from '@/components/public/return-path'
import { Skeleton } from '@/components/ui/skeleton'
import { tanks } from '@/lib/api'
import { useMe } from '@/lib/use-me'
import { useT } from '@/lib/i18n/client'
import { tanksHomeMessages as m } from '@/lib/i18n/messages/tanks-home'
import type { Showcase } from '@/lib/types'

const TILE = 'rounded-[14px] border border-border bg-card p-5'

export default function TanksHome() {
  const { me } = useMe()
  const tr = useT(m)
  const [show, setShow] = useState<Showcase | null>(null)

  useEffect(() => {
    const load = () =>
      void tanks
        .showcase()
        .then(setShow)
        .catch(() => {})
    load()
    const t = setInterval(load, 8000)
    return () => clearInterval(t)
  }, [])

  const tour = show?.tournament
  const next = show?.next_tournament

  return (
    <div className="mx-auto max-w-6xl px-4 py-10 sm:px-6 sm:py-14">
      <div className="grid gap-10 lg:grid-cols-[1.15fr_0.85fr] lg:items-start lg:gap-12">
        <div className="order-2 lg:order-1">
          <Live />
        </div>
        <div className="order-1 lg:order-2">
          <h1 className="display text-[2rem] sm:text-[2.5rem]">{tr('home.title')}</h1>
          <p className="mt-4 leading-relaxed text-muted-foreground">
            {tr('home.lead')}
          </p>
          <ul className="mt-3 space-y-1 text-sm font-semibold">
            <li>{tr('home.bullet.ladder')}</li>
            <li>{tr('home.bullet.seasons')}</li>
            <li>{tr('home.bullet.tournament')}</li>
          </ul>
          <div className="mt-6 flex flex-wrap gap-3">
            <Button size="lg" render={<Link href={me ? '/app/tanks' : loginHref('/app/tanks')} />} nativeButton={false}>
              {tr('home.enter')}
            </Button>
            <Button size="lg" variant="outline" render={<Link href="/tanks/docs" />} nativeButton={false}>
              {tr('home.how')}
            </Button>
          </div>
        </div>
      </div>

      {show == null ? (
        <Skeleton className="mt-16 h-40 rounded-[14px]" />
      ) : (
        <>
          <div className="mt-16 grid gap-4 md:grid-cols-2">
            <div className={TILE}>
              <div className="flex items-center justify-between gap-3">
                <p className="text-xs font-bold uppercase tracking-wide text-muted-foreground">{tr('home.season')}</p>
                <Link href="/tanks/leaderboard" className="text-sm font-semibold text-primary hover:underline">
                  {tr('home.standings')}
                </Link>
              </div>
              <p className="heading mt-2 text-2xl">{show.season.name}</p>
              <p className="mt-3 text-sm text-muted-foreground">
                {tr('home.endsIn')} <span className="font-bold text-foreground"><Countdown to={show.season.ends_at} serverNow={show.now} /></span>
              </p>
              <p className="mt-1 text-xs text-muted-foreground">{tr('home.freshRating')}</p>
            </div>

            <div className={TILE}>
              <div className="flex items-center justify-between gap-3">
                <p className="text-xs font-bold uppercase tracking-wide text-muted-foreground">
                  {tour ? (tour.status === 'running' ? tr('home.tourLive') : tr('home.tourLatest')) : tr('home.tourNext')}
                </p>
                <Link href="/tanks/tournaments" className="text-sm font-semibold text-primary hover:underline">{tr('home.allTournaments')}</Link>
              </div>
              {tour ? (
                <>
                  <p className="heading mt-2 flex flex-wrap items-center gap-2 text-2xl">
                    <Link href={`/tanks/tournaments/${tour.id}`} className="hover:text-primary">{tour.name}</Link>
                    {tour.status === 'running' && <Badge>{tr('home.live')}</Badge>}
                  </p>
                  {tour.champion ? (
                    <p className="mt-3 flex items-center gap-1.5 text-sm">
                      <Trophy className="size-4 text-warning" />
                      {tr('home.champion')} <Link href={`/tanks/bots/${tour.champion.bot_id}`} className="font-bold hover:text-primary">{tour.champion.name}</Link>
                      {tour.champion.owner && <span className="text-muted-foreground">{tr('home.by')} <HandleLink handle={tour.champion.owner} /></span>}
                    </p>
                  ) : (
                    <p className="mt-3 text-sm text-muted-foreground">{tr('home.entries', { count: tour.entry_count, bestOf: tour.best_of })}</p>
                  )}
                  {next && (
                    <p className="mt-1 text-xs text-muted-foreground">
                      {tr('home.nextIn')} <Countdown to={next.starts_at} serverNow={show.now} />
                    </p>
                  )}
                </>
              ) : next ? (
                <>
                  <p className="heading mt-2 text-2xl">{next.name}</p>
                  <p className="mt-3 flex items-center gap-1.5 text-sm text-muted-foreground">
                    <CalendarClock className="size-4" />
                    {tr('home.startsIn')} <span className="font-bold text-foreground"><Countdown to={next.starts_at} serverNow={show.now} /></span>
                  </p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {tr('home.nextDesc', { size: next.size, bestOf: next.best_of })}
                  </p>
                </>
              ) : (
                <p className="mt-3 text-sm text-muted-foreground">{tr('home.nothing')}</p>
              )}
            </div>
          </div>

          {show.open_tournaments.length > 0 && (
            <section className="mt-10">
              <SectionTitle>{tr('home.openTournaments')}</SectionTitle>
              <p className="mb-3 text-sm text-muted-foreground">
                {tr('home.openTournamentNote')}
              </p>
              <TournamentList items={show.open_tournaments} />
            </section>
          )}

          {tour && (tour.pairings?.length ?? 0) > 0 && (
            <section className="mt-10">
              <SectionTitle aside={<Link href={`/tanks/tournaments/${tour.id}`} className="font-semibold text-primary hover:underline">{tr('home.openBracket')}</Link>}>
                {tour.status === 'running' ? tr('home.liveBracket') : tour.name}
              </SectionTitle>
              <Bracket t={tour} />
            </section>
          )}

          <section className="mt-12">
            <SectionTitle aside={<Link href="/tanks/leaderboard" className="font-semibold text-primary hover:underline">{tr('home.fullLadder')}</Link>}>
              {tr('home.ladderTop')}
            </SectionTitle>
            <Leaderboard entries={show.ladder} />
          </section>

          <section className="mt-12">
            <SectionTitle>{tr('home.notable')}</SectionTitle>
            <MatchList matches={show.notable} />
          </section>

          <section className="mt-12 grid gap-10 md:grid-cols-2">
            <div>
              <SectionTitle>{tr('home.champions')}</SectionTitle>
              <TournamentList items={show.champions} empty={tr('home.noFinished')} />
            </div>
            <div>
              <SectionTitle>{tr('home.pastSeasons')}</SectionTitle>
              {show.past_seasons.length === 0 ? (
                <p className="rounded-[14px] border border-dashed border-input px-5 py-10 text-center text-sm text-muted-foreground">
                  {tr('home.firstSeason')}
                </p>
              ) : (
                <ul className="divide-y divide-border rounded-[14px] border border-border">
                  {show.past_seasons.map((s) => (
                    <li key={s.id}>
                      <Link href={`/tanks/seasons/${s.id}`} className="flex flex-wrap items-center gap-x-3 gap-y-1 p-4 hover:bg-muted/50">
                        <span className="min-w-0 flex-1 truncate text-sm font-semibold">{s.name}</span>
                        {s.winner && (
                          <span className="inline-flex items-center gap-1.5 text-sm font-bold">
                            <Trophy className="size-3.5 text-warning" />
                            {s.winner.name}
                          </span>
                        )}
                      </Link>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </section>
        </>
      )}
    </div>
  )
}
