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
import { Skeleton } from '@/components/ui/skeleton'
import { tanks } from '@/lib/api'
import { useMe } from '@/lib/use-me'
import type { Showcase } from '@/lib/types'

const TILE = 'rounded-[14px] border border-border bg-card p-5'

export default function TanksHome() {
  const { me } = useMe()
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
        <Live />
        <div>
          <h1 className="display text-[2rem] sm:text-[2.5rem]">Coding agents write tank bots. Bots fight. You watch.</h1>
          <p className="mt-4 leading-relaxed text-muted-foreground">
            Every bot is just a process talking JSON over stdin and stdout. Have your coding agent write one, or write
            one by hand — the ladder plays them all, around the clock, every month is a season, and every Saturday the
            best eight play for the title.
          </p>
          <div className="mt-6 flex flex-wrap gap-3">
            <Button size="lg" render={<Link href={me ? '/app/tanks' : '/login'} />} nativeButton={false}>
              Enter your bot
            </Button>
            <Button size="lg" variant="outline" render={<Link href="/tanks/docs" />} nativeButton={false}>
              How it works
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
                <p className="text-xs font-bold uppercase tracking-wide text-muted-foreground">Current season</p>
                <Link href={`/tanks/seasons/${show.season.id}`} className="text-sm font-semibold text-primary hover:underline">
                  Standings
                </Link>
              </div>
              <p className="heading mt-2 text-2xl">{show.season.name}</p>
              <p className="mt-3 text-sm text-muted-foreground">
                Ends in <span className="font-bold text-foreground"><Countdown to={show.season.ends_at} serverNow={show.now} /></span>
              </p>
              <p className="mt-1 text-xs text-muted-foreground">Everyone starts the month at a fresh rating.</p>
            </div>

            <div className={TILE}>
              <div className="flex items-center justify-between gap-3">
                <p className="text-xs font-bold uppercase tracking-wide text-muted-foreground">
                  {tour ? (tour.status === 'running' ? 'Tournament, live' : 'Latest tournament') : 'Next tournament'}
                </p>
                <Link href="/tanks/tournaments" className="text-sm font-semibold text-primary hover:underline">All tournaments</Link>
              </div>
              {tour ? (
                <>
                  <p className="heading mt-2 flex flex-wrap items-center gap-2 text-2xl">
                    <Link href={`/tanks/tournaments/${tour.id}`} className="hover:text-primary">{tour.name}</Link>
                    {tour.status === 'running' && <Badge>Live</Badge>}
                  </p>
                  {tour.champion ? (
                    <p className="mt-3 flex items-center gap-1.5 text-sm">
                      <Trophy className="size-4 text-warning" />
                      Champion: <Link href={`/tanks/bots/${tour.champion.bot_id}`} className="font-bold hover:text-primary">{tour.champion.name}</Link>
                      {tour.champion.owner && <span className="text-muted-foreground">by <HandleLink handle={tour.champion.owner} /></span>}
                    </p>
                  ) : (
                    <p className="mt-3 text-sm text-muted-foreground">{tour.entry_count} bots, best of {tour.best_of}. The bracket is below.</p>
                  )}
                  {next && (
                    <p className="mt-1 text-xs text-muted-foreground">
                      Next one in <Countdown to={next.starts_at} serverNow={show.now} />
                    </p>
                  )}
                </>
              ) : next ? (
                <>
                  <p className="heading mt-2 text-2xl">{next.name}</p>
                  <p className="mt-3 flex items-center gap-1.5 text-sm text-muted-foreground">
                    <CalendarClock className="size-4" />
                    Starts in <span className="font-bold text-foreground"><Countdown to={next.starts_at} serverNow={show.now} /></span>
                  </p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    The top {next.size} of the season ladder, single elimination, every pairing a best of {next.best_of}.
                  </p>
                </>
              ) : (
                <p className="mt-3 text-sm text-muted-foreground">Nothing scheduled yet.</p>
              )}
            </div>
          </div>

          {tour && (tour.pairings?.length ?? 0) > 0 && (
            <section className="mt-10">
              <SectionTitle aside={<Link href={`/tanks/tournaments/${tour.id}`} className="font-semibold text-primary hover:underline">Open bracket</Link>}>
                {tour.status === 'running' ? 'Live bracket' : tour.name}
              </SectionTitle>
              <Bracket t={tour} />
            </section>
          )}

          <section className="mt-12">
            <SectionTitle aside={<Link href="/tanks/leaderboard" className="font-semibold text-primary hover:underline">Full ladder</Link>}>
              Season ladder, top 10
            </SectionTitle>
            <Leaderboard entries={show.ladder} />
          </section>

          <section className="mt-12">
            <SectionTitle>Notable matches</SectionTitle>
            <MatchList matches={show.notable} />
          </section>

          <section className="mt-12 grid gap-10 md:grid-cols-2">
            <div>
              <SectionTitle>Past champions</SectionTitle>
              <TournamentList items={show.champions} empty="No tournament has finished yet." />
            </div>
            <div>
              <SectionTitle>Past seasons</SectionTitle>
              {show.past_seasons.length === 0 ? (
                <p className="rounded-[14px] border border-dashed border-input px-5 py-10 text-center text-sm text-muted-foreground">
                  The first season is still running.
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
