import type { Metadata } from 'next'
import { PRODUCT } from '@/lib/brand'
import { clip, fmtDay, type OgCard, type OgStat } from '@/lib/og'
import { serverApi } from '@/lib/server-api'
import type {
  BotProfile, Daily, DailyRow, DayStats, MatchView, Profile, ProductResults, SeasonDetail, TournamentView,
} from '@/lib/types'

// What a shared link says about one entity: the page's title and description
// (for generateMetadata) and the card drawn in its opengraph-image. Every
// loader returns null when the entity is missing or the API is down.
export interface Share { title: string; description: string; card: OgCard }

// Metadata for a page whose image comes from a colocated opengraph-image file
// (or the site-wide default one).
export function shareMeta(title: string, description: string, path?: string): Metadata {
  const full = `${title} · ${PRODUCT}`
  return {
    title,
    description,
    alternates: path ? { canonical: path } : undefined,
    openGraph: { title: full, description, siteName: PRODUCT, type: 'website', url: path },
    twitter: { card: 'summary_large_image', title: full, description },
  }
}

export const DEFAULT_DESCRIPTION = 'One coding task every day. Give it to your own coding agent, upload the result, and hidden tests decide.'

const plural = (n: number, one: string, many = `${one}s`) => `${n} ${n === 1 ? one : many}`
const LANG: Record<string, string> = { go: 'Go', python: 'Python' }

export async function dayShare(day: string): Promise<Share | null> {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(day)) return null
  const d = await serverApi<Daily>(`/daily/${day}`)
  if (!d) return null
  const [stats, board] = await Promise.all([
    serverApi<DayStats>(`/daily/${day}/stats`),
    d.task.kind === 'optimize' ? serverApi<{ items: DailyRow[] }>(`/daily/${day}/leaderboard`) : Promise.resolve(null),
  ])
  const when = fmtDay(d.day)
  const lang = LANG[d.task.language] ?? d.task.language
  const optimize = d.task.kind === 'optimize'
  const players = stats?.participants ?? 0
  const attempts = stats?.submissions ?? 0
  let best: string | number = '–'
  if (optimize) {
    const scores = (board?.items ?? []).map((r) => r.score).filter((s): s is number => s != null)
    if (scores.length > 0) best = d.task.direction === 'min' ? Math.min(...scores) : Math.max(...scores)
  }
  const stat = stats == null
    ? []
    : optimize
      ? [{ label: 'Players', value: players }, { label: d.task.direction === 'min' ? 'Best (lowest)' : 'Best score', value: best }, { label: 'Attempts', value: attempts }]
      : [{ label: 'Players', value: players }, { label: 'Solved', value: stats.solvers }, { label: 'Attempts', value: attempts }]
  const summary = stats == null
    ? `${lang} ${optimize ? 'optimization' : 'bugfix'} task.`
    : optimize
      ? `${plural(players, 'player')} tried it with their own agents${best === '–' ? '' : `, best score ${best}`}.`
      : `${plural(players, 'player')} tried it with their own agents, ${stats.solvers} solved it.`
  return {
    title: `${d.task.title} · ${when}`,
    description: `The coding task for ${when}: ${d.task.title}. ${summary} Can your agent beat the hidden tests?`,
    card: {
      kicker: `Task of the day · ${when}`,
      title: d.task.title,
      subtitle: `${lang} · ${optimize ? 'optimize' : 'fix the bug'} · ${d.is_open ? 'open now' : 'closed'}`,
      stats: stat,
      badge: { text: d.is_open ? 'Open now' : 'Closed', live: d.is_open },
    },
  }
}

export async function profileShare(handle: string): Promise<Share | null> {
  const p = await serverApi<Profile>(`/users/${encodeURIComponent(handle)}`)
  if (!p) return null
  const place = p.place == null ? '–' : `#${p.place}`
  const bits = [
    p.place != null ? `ranked ${place}` : null,
    p.streak.current > 0 ? `${p.streak.current}-day streak` : null,
    `${plural(p.solved_days, 'day')} solved`,
  ].filter(Boolean)
  return {
    title: p.handle,
    description: `${p.handle} on ${PRODUCT}: ${bits.join(', ')}. One coding task a day, solved by their own agent.`,
    card: {
      kicker: 'Player',
      title: p.handle,
      subtitle: p.tools.length > 0 ? `Plays with ${p.tools.slice(0, 3).join(', ')}` : `${plural(p.played_days, 'day')} played`,
      stats: [
        { label: 'Place', value: place },
        { label: 'Streak', value: p.streak.current },
        { label: 'Days solved', value: p.solved_days },
        { label: 'Best streak', value: p.streak.best },
      ],
    },
  }
}

export async function tournamentShare(id: string): Promise<Share | null> {
  const t = await serverApi<TournamentView>(`/tanks/tournaments/${encodeURIComponent(id)}`)
  if (!t) return null
  const live = t.status === 'running'
  const state = t.status === 'finished' ? 'Finished' : t.status === 'cancelled' ? 'Cancelled' : live ? 'Live now' : 'Scheduled'
  const champ = t.champion ? `${t.champion.name}${t.champion.owner ? ` by ${t.champion.owner}` : ''}` : null
  return {
    title: t.name,
    description: champ
      ? `${t.name}: bot tournament won by ${champ}. ${t.size} bots, single elimination, best of ${t.best_of}.`
      : `${t.name}: ${t.size}-bot single elimination tournament of bots written by coding agents.${live ? ' Live now, watch the matches.' : ''}`,
    card: {
      kicker: 'Tanks tournament',
      title: t.name,
      subtitle: champ ? `Champion: ${champ}` : live ? 'Bots written by coding agents are fighting right now' : 'Bots written by coding agents, single elimination',
      stats: [
        { label: 'Bracket', value: t.size },
        ...(t.rounds > 0 ? [{ label: 'Rounds', value: t.rounds }] : []),
        { label: 'Best of', value: t.best_of },
      ],
      badge: { text: state, live },
    },
  }
}

export async function productShare(slug: string): Promise<Share | null> {
  const r = await serverApi<ProductResults>(`/products/${encodeURIComponent(slug)}/results`)
  if (!r) return null
  const { task, entries } = r
  const votes = entries.reduce((n, e) => n + e.votes, 0)
  const leader = entries[0] // the results come ranked by the task kind's rule
  const phase = task.phase === 'open' ? 'Open' : task.phase === 'voting' ? 'Voting' : 'Final'
  const stats: OgStat[] = [{ label: 'Entries', value: task.entry_count }]
  if (task.phase !== 'open' && !(task.kind === 'site' && task.phase === 'voting')) stats.push({ label: 'Votes', value: votes }) // site votes are blind until voting ends
  if (task.phase !== 'open' && leader?.handle) stats.push({ label: task.phase === 'final' ? 'Winner' : 'Leading', value: clip(leader.handle, 14) })
  return {
    title: task.title,
    description: `${task.summary || task.title} Product of the week: ${plural(task.entry_count, 'entry', 'entries')}${task.phase === 'voting' ? ', voting is open' : task.phase === 'final' ? ', final results' : ', open for entries'}.`,
    card: {
      kicker: `Product of the week · ${task.kind === 'site' ? 'website' : 'command line'}`,
      title: task.title,
      subtitle: task.summary || undefined,
      stats,
      badge: { text: phase, live: task.phase !== 'final' },
    },
  }
}

export async function botShare(id: string): Promise<{ title: string; description: string } | null> {
  const b = await serverApi<BotProfile>(`/tanks/bots/${encodeURIComponent(id)}`)
  if (!b) return null
  return {
    title: b.name,
    description: `${b.name} by ${b.owner || 'the house'}: a tank bot rated ${b.rating}, ${b.wins} wins in ${plural(b.matches, 'match', 'matches')} this season.`,
  }
}

export async function matchShare(id: string): Promise<{ title: string; description: string } | null> {
  const m = await serverApi<MatchView>(`/tanks/matches/${encodeURIComponent(id)}`)
  if (!m) return null
  const names = [...m.players].sort((a, b) => (a.place ?? 9) - (b.place ?? 9)).map((p) => p.name)
  return {
    title: names.slice(0, 2).join(' vs '),
    description: `Tank bot match: ${names.join(', ')}. Watch the replay.`,
  }
}

export async function seasonShare(id: string): Promise<{ title: string; description: string } | null> {
  const s = await serverApi<SeasonDetail>(`/tanks/seasons/${encodeURIComponent(id)}`)
  if (!s) return null
  const top = s.standings[0]
  return {
    title: s.season.name,
    description: `Tanks season ${s.season.name}${top ? `: ${top.name} leads at ${top.rating}` : ''}. Bots written by coding agents fight on a public ladder.`,
  }
}
