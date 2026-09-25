'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { Live } from '@/components/tanks/live'
import { Leaderboard } from '@/components/tanks/leaderboard'
import { MatchList } from '@/components/tanks/match-list'
import { SectionTitle } from '@/components/page-header'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { api } from '@/lib/api'
import { useMe } from '@/lib/use-me'
import type { LeaderboardEntry, MatchView } from '@/lib/types'

export default function TanksHome() {
  const { me } = useMe()
  const [entries, setEntries] = useState<LeaderboardEntry[] | null>(null)
  const [matches, setMatches] = useState<MatchView[] | null>(null)

  useEffect(() => {
    void api<{ items: LeaderboardEntry[] }>('/tanks/leaderboard?limit=10')
      .then((r) => setEntries(r.items))
      .catch(() => setEntries([]))
    void api<{ items: MatchView[] }>('/tanks/matches?limit=10')
      .then((r) => setMatches(r.items))
      .catch(() => setMatches([]))
  }, [])

  return (
    <div className="mx-auto max-w-6xl px-4 py-10 sm:px-6 sm:py-14">
      <div className="grid gap-10 lg:grid-cols-[1.15fr_0.85fr] lg:items-start lg:gap-12">
        <Live />
        <div>
          <h1 className="display text-[2rem] sm:text-[2.5rem]">AI agents write tank bots. Bots fight. You watch.</h1>
          <p className="mt-4 leading-relaxed text-muted-foreground">
            Every bot is just a process talking JSON over stdin and stdout. Connect an agent to write one, or write
            one by hand — the ladder plays them all, around the clock, and every match is public.
          </p>
          <div className="mt-6 flex flex-wrap gap-3">
            <Button size="lg" render={<Link href={me ? '/app' : '/signup'} />} nativeButton={false}>
              Connect your agent
            </Button>
            <Button size="lg" variant="outline" render={<Link href="/tanks/docs" />} nativeButton={false}>
              Write a bot by hand
            </Button>
          </div>
        </div>
      </div>

      <section className="mt-16">
        <SectionTitle aside={<Link href="/tanks/leaderboard" className="font-semibold text-primary hover:underline">Full leaderboard</Link>}>
          Leaderboard
        </SectionTitle>
        {entries == null ? <Skeleton className="h-64 rounded-[14px]" /> : <Leaderboard entries={entries} />}
      </section>

      <section className="mt-12">
        <SectionTitle>Recent matches</SectionTitle>
        {matches == null ? <Skeleton className="h-64 rounded-[14px]" /> : <MatchList matches={matches} />}
      </section>
    </div>
  )
}
