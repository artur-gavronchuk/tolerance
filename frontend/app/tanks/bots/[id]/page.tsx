'use client'

import { use, useEffect, useState } from 'react'
import Link from 'next/link'
import { ArrowLeft, Trophy } from 'lucide-react'
import { PageHeader, SectionTitle } from '@/components/page-header'
import { BotBadge } from '@/components/tanks/bot-badge'
import { HandleLink } from '@/components/daily/handle-link'
import { MatchList } from '@/components/tanks/match-list'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { api, ApiError } from '@/lib/api'
import type { BotProfile, MatchView, MyTanks } from '@/lib/types'

export default function BotPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params)
  const [bot, setBot] = useState<BotProfile | null>(null)
  const [matches, setMatches] = useState<MatchView[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [mine, setMine] = useState(false) // the signed-in viewer owns this bot

  useEffect(() => {
    void api<BotProfile>(`/tanks/bots/${id}`)
      .then(setBot)
      .catch((e) => setError((e as ApiError).status === 404 ? 'There is no bot with this id.' : (e as ApiError).message))
    void api<{ items: MatchView[] }>(`/tanks/matches?bot_id=${id}&limit=20`)
      .then((r) => setMatches(r.items))
      .catch(() => setMatches([]))
    // Anonymous visitors get a 401 here; that just means no report buttons.
    void api<MyTanks>('/me/tanks')
      .then((t) => setMine(t.bot?.id === id))
      .catch(() => setMine(false))
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
  if (!bot) {
    return (
      <div className="mx-auto max-w-4xl space-y-6 px-4 py-14 sm:px-6">
        <Skeleton className="h-10 w-72" />
        <Skeleton className="h-32 rounded-[14px]" />
      </div>
    )
  }

  const winRate = bot.matches > 0 ? `${Math.round((bot.wins / bot.matches) * 100)}%` : '—'
  const stats: [string, string | number][] = [
    ['Season rating', bot.rating],
    ['Lifetime rating', bot.lifetime_rating],
    ['Season matches', bot.matches],
    ['Season wins', bot.wins],
    ['Win rate', winRate],
  ]

  return (
    <div className="mx-auto max-w-4xl px-4 py-10 sm:px-6 sm:py-14">
      <PageHeader
        kicker={
          <Link href="/tanks/leaderboard" className="inline-flex items-center gap-1.5 hover:text-foreground">
            <ArrowLeft className="size-4" />Ladder
          </Link>
        }
        title={
          <span className="inline-flex flex-wrap items-center gap-3">
            {bot.name}
            <BotBadge source={bot.source} house={bot.house} />
          </span>
        }
      >
        {bot.rank > 0 ? <>Rank #{bot.rank} in season {bot.season.name}.</> : <>Not ranked in season {bot.season.name} yet.</>}
        {bot.owner && <> By <HandleLink handle={bot.owner} />.</>}
      </PageHeader>

      <div className="mt-8 grid grid-cols-2 gap-3 sm:grid-cols-5">
        {stats.map(([label, value]) => (
          <div key={label} className="rounded-[12px] border border-border bg-card p-4">
            <p className="text-xs font-bold text-muted-foreground">{label}</p>
            <p className="mt-1 font-mono text-xl font-bold">{value}</p>
          </div>
        ))}
      </div>

      <section className="mt-10">
        <SectionTitle>Tournaments</SectionTitle>
        {bot.tournaments.length === 0 ? (
          <p className="rounded-[14px] border border-dashed border-input px-5 py-8 text-center text-sm text-muted-foreground">
            Has not played a tournament yet. The top 8 of the season ladder qualify.
          </p>
        ) : (
          <ul className="divide-y divide-border rounded-[14px] border border-border">
            {bot.tournaments.map((t) => (
              <li key={t.tournament_id}>
                <Link href={`/tanks/tournaments/${t.tournament_id}`} className="flex flex-wrap items-center gap-x-3 gap-y-1 p-4 text-sm hover:bg-muted/50">
                  <span className="min-w-0 flex-1 truncate font-semibold">{t.name}</span>
                  <span className="font-mono text-xs text-muted-foreground">seed #{t.seed}</span>
                  <span className={t.champion ? 'inline-flex items-center gap-1 font-bold' : 'text-muted-foreground'}>
                    {t.champion && <Trophy className="size-3.5 text-warning" />}
                    {t.result}
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </section>

      {bot.seasons.length > 0 && (
        <section className="mt-10">
          <SectionTitle>Past seasons</SectionTitle>
          <ul className="divide-y divide-border rounded-[14px] border border-border">
            {bot.seasons.map((x) => (
              <li key={x.season_id}>
                <Link href={`/tanks/seasons/${x.season_id}`} className="flex flex-wrap items-center gap-x-3 gap-y-1 p-4 text-sm hover:bg-muted/50">
                  <span className="min-w-0 flex-1 truncate font-semibold">{x.name}</span>
                  <span className="font-mono text-xs text-muted-foreground">{x.matches} matches · {x.wins} wins</span>
                  <span className="font-mono font-bold">#{x.rank}</span>
                  <span className="font-mono text-muted-foreground">{x.rating}</span>
                </Link>
              </li>
            ))}
          </ul>
        </section>
      )}

      <section className="mt-10">
        <SectionTitle>Versions</SectionTitle>
        <ul className="divide-y divide-border rounded-[14px] border border-border">
          {bot.versions.map((v) => (
            <li key={v.number} className="flex flex-wrap items-center gap-3 p-4 text-sm">
              <span className="font-mono text-muted-foreground">v{v.number}</span>
              <BotBadge source={v.source} />
              <span
                className={
                  v.status === 'active' ? 'font-semibold text-success' : v.status === 'rejected' ? 'text-destructive' : 'text-muted-foreground'
                }
              >
                {v.status}
              </span>
              <span className="ml-auto text-xs text-muted-foreground">{new Date(v.created_at).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' })}</span>
            </li>
          ))}
          {bot.versions.length === 0 && <li className="p-4 text-sm text-muted-foreground">No versions yet.</li>}
        </ul>
      </section>

      <section className="mt-10">
        <SectionTitle>Recent matches</SectionTitle>
        {matches == null ? <Skeleton className="h-64 rounded-[14px]" /> : <MatchList matches={matches} botId={id} showReport={mine} />}
      </section>
    </div>
  )
}
