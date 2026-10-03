'use client'

import Link from 'next/link'
import { Trophy } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useT } from '@/lib/i18n/client'
import { tanksHomeMessages as m } from '@/lib/i18n/messages/tanks-home'
import type { TournamentBot, TournamentPairing, TournamentView } from '@/lib/types'

type BracketT = ReturnType<typeof useT<typeof m.en>>

export function roundName(t: BracketT, round: number, rounds: number): string {
  if (round === rounds) return t('bracket.final')
  if (round === rounds - 1) return t('bracket.semis')
  if (round === rounds - 2) return t('bracket.quarters')
  return t('bracket.round', { n: round })
}

function BotRow({ t, bot, wins, won, lost, bye }: { t: BracketT; bot: TournamentBot | null; wins: number; won: boolean; lost: boolean; bye?: boolean }) {
  return (
    <div className={cn('flex items-center gap-2 px-3 py-2', won && 'bg-success/10', lost && 'text-muted-foreground')}>
      <span className="w-4 shrink-0 text-right font-mono text-[0.7rem] text-muted-foreground">{bot?.seed ?? ''}</span>
      {bot ? (
        <Link href={`/tanks/bots/${bot.bot_id}`} className={cn('min-w-0 flex-1 truncate text-sm hover:text-primary', won ? 'font-bold' : 'font-semibold')}>
          {bot.name}
        </Link>
      ) : (
        <span className="min-w-0 flex-1 truncate text-sm italic text-muted-foreground">{bye ? t('bracket.bye') : t('bracket.tbd')}</span>
      )}
      <span className={cn('w-4 shrink-0 text-right font-mono text-sm', won ? 'font-bold' : 'text-muted-foreground')}>
        {bot && !bye ? wins : ''}
      </span>
    </div>
  )
}

function PairingCard({ t, p }: { t: BracketT; p: TournamentPairing }) {
  const aWon = p.status === 'finished' && p.winner_bot_id != null && p.winner_bot_id === p.a?.bot_id
  const bWon = p.status === 'finished' && p.winner_bot_id != null && p.winner_bot_id === p.b?.bot_id
  return (
    <div
      className={cn(
        'overflow-hidden rounded-[10px] border bg-card',
        p.status === 'running' ? 'border-primary shadow-sm' : 'border-border',
      )}
    >
      <BotRow t={t} bot={p.a} wins={p.wins_a} won={aWon} lost={bWon} />
      <div className="border-t border-border" />
      <BotRow t={t} bot={p.b} wins={p.wins_b} won={bWon} lost={aWon} bye={p.bye} />
      {p.games.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5 border-t border-border bg-muted/40 px-3 py-1.5">
          {p.games.map((g) => {
            const w = g.winner_bot_id
            const label = w ? (w === p.a?.bot_id ? p.a?.name : p.b?.name) : g.status === 'running' ? t('bracket.playing') : t('bracket.queued')
            return (
              <Link
                key={g.game}
                href={`/tanks/matches/${g.match_id}`}
                title={`${t('bracket.gameTitle', { game: g.game, map: g.map })}${w ? t('bracket.gameWon', { name: label ?? '' }) : ''}`}
                className="inline-flex max-w-full items-center gap-1 rounded-full border border-border bg-card px-2 py-0.5 text-[0.7rem] hover:border-primary hover:text-primary"
              >
                <span className="font-mono text-muted-foreground">G{g.game}</span>
                <span className="max-w-[5.5rem] truncate">{label}</span>
              </Link>
            )
          })}
        </div>
      )}
    </div>
  )
}

// The single-elimination bracket: one column per round, scrolling sideways inside its own container so the
// page itself never does (it must work at 375px).
export function Bracket({ t: tour }: { t: TournamentView }) {
  const t = useT(m)
  const pairings = tour.pairings ?? []
  if (pairings.length === 0) return null
  const rounds = Array.from({ length: tour.rounds }, (_, i) => i + 1)
  return (
    <div className="overflow-x-auto rounded-[14px] border border-border bg-muted/20 p-4">
      <div className="flex min-w-max items-stretch gap-5">
        {rounds.map((r) => (
          <div key={r} className="flex w-56 shrink-0 flex-col">
            <p className="mb-3 flex items-center gap-1.5 text-xs font-bold uppercase tracking-wide text-muted-foreground">
              {r === tour.rounds && <Trophy className="size-3.5 text-warning" />}
              {roundName(t, r, tour.rounds)}
            </p>
            <div className="flex flex-1 flex-col justify-around gap-3">
              {pairings
                .filter((p) => p.round === r)
                .map((p) => (
                  <PairingCard key={p.id} t={t} p={p} />
                ))}
            </div>
          </div>
        ))}
        <div className="flex w-44 shrink-0 flex-col">
          <p className="mb-3 text-xs font-bold uppercase tracking-wide text-muted-foreground">{t('tour.champion')}</p>
          <div className="flex flex-1 items-center">
            <div
              className={cn(
                'w-full rounded-[10px] border px-3 py-3 text-center',
                tour.champion ? 'border-warning bg-warning/10' : 'border-dashed border-border text-muted-foreground',
              )}
            >
              {tour.champion ? (
                <Link href={`/tanks/bots/${tour.champion.bot_id}`} className="text-sm font-bold hover:text-primary">
                  {tour.champion.name}
                </Link>
              ) : (
                <span className="text-sm italic">{t('bracket.tbd')}</span>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
