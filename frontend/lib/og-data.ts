import type { Metadata } from 'next'
import { PRODUCT } from '@/lib/brand'
import { clip, fmtDay, type OgCard, type OgStat } from '@/lib/og'
import { serverApi } from '@/lib/server-api'
import { getT } from '@/lib/i18n/server'
import { ogMessages } from '@/lib/i18n/messages/og'
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

const LANG: Record<string, string> = { go: 'Go', python: 'Python' }

export async function dayShare(day: string): Promise<Share | null> {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(day)) return null
  const d = await serverApi<Daily>(`/daily/${day}`)
  if (!d) return null
  const t = await getT(ogMessages)
  const [stats, board] = await Promise.all([
    serverApi<DayStats>(`/daily/${day}/stats`),
    d.task.kind === 'optimize' ? serverApi<{ items: DailyRow[] }>(`/daily/${day}/leaderboard`) : Promise.resolve(null),
  ])
  const when = fmtDay(d.day, t.locale)
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
      ? [{ label: t('statPlayers'), value: players }, { label: d.task.direction === 'min' ? t('statBestLowest') : t('statBestScore'), value: best }, { label: t('statAttempts'), value: attempts }]
      : [{ label: t('statPlayers'), value: players }, { label: t('statSolved'), value: stats.solvers }, { label: t('statAttempts'), value: attempts }]
  const summary = stats == null
    ? t(optimize ? 'langOptimizationTask' : 'langBugfixTask', { lang })
    : optimize
      ? t('triedBest', { n: players, players: t.plural('players', players), best: best === '–' ? '' : t('triedBestPart', { best }) })
      : t('triedSolved', { n: players, players: t.plural('players', players), solved: stats.solvers })
  return {
    title: `${d.task.title} · ${when}`,
    description: t('dayDescription', { when, title: d.task.title, summary }),
    card: {
      kicker: t('kickerDay', { when }),
      title: d.task.title,
      subtitle: `${lang} · ${optimize ? t('optimize') : t('fixBug')} · ${d.is_open ? t('openNow') : t('closed')}`,
      stats: stat,
      badge: { text: d.is_open ? t('badgeOpenNow') : t('badgeClosed'), live: d.is_open },
    },
  }
}

export async function profileShare(handle: string): Promise<Share | null> {
  const p = await serverApi<Profile>(`/users/${encodeURIComponent(handle)}`)
  if (!p) return null
  const t = await getT(ogMessages)
  const place = p.place == null ? '–' : `#${p.place}`
  const bits = [
    p.place != null ? t('ranked', { place }) : null,
    p.streak.current > 0 ? t('streakBit', { n: p.streak.current }) : null,
    t('solvedBit', { days: t.plural('days', p.solved_days) }),
  ].filter(Boolean)
  return {
    title: p.handle,
    description: t('profileDescription', { handle: p.handle, product: PRODUCT, bits: bits.join(', ') }),
    card: {
      kicker: t('kickerPlayer'),
      title: p.handle,
      subtitle: p.tools.length > 0 ? t('playsWith', { tools: p.tools.slice(0, 3).join(', ') }) : t('daysPlayed', { days: t.plural('days', p.played_days) }),
      stats: [
        { label: t('statPlace'), value: place },
        { label: t('statStreak'), value: p.streak.current },
        { label: t('statDaysSolved'), value: p.solved_days },
        { label: t('statBestStreak'), value: p.streak.best },
      ],
    },
  }
}

export async function tournamentShare(id: string): Promise<Share | null> {
  const tv = await serverApi<TournamentView>(`/tanks/tournaments/${encodeURIComponent(id)}`)
  if (!tv) return null
  const t = await getT(ogMessages)
  const live = tv.status === 'running'
  const state = tv.status === 'finished' ? t('stFinished') : tv.status === 'cancelled' ? t('stCancelled') : live ? t('stLive') : t('stScheduled')
  const champ = tv.champion ? (tv.champion.owner ? t('byOwner', { name: tv.champion.name, owner: tv.champion.owner }) : tv.champion.name) : null
  return {
    title: tv.name,
    description: champ
      ? t('tourWon', { name: tv.name, champ, size: tv.size, bestOf: tv.best_of })
      : t('tourPlain', { name: tv.name, size: tv.size }) + (live ? t('tourLive') : ''),
    card: {
      kicker: t('kickerTournament'),
      title: tv.name,
      subtitle: champ ? t('champion', { champ }) : live ? t('tourFighting') : t('tourSub'),
      stats: [
        { label: t('statBracket'), value: tv.size },
        ...(tv.rounds > 0 ? [{ label: t('statRounds'), value: tv.rounds }] : []),
        { label: t('statBestOf'), value: tv.best_of },
      ],
      badge: { text: state, live },
    },
  }
}

export async function productShare(slug: string): Promise<Share | null> {
  const r = await serverApi<ProductResults>(`/products/${encodeURIComponent(slug)}/results`)
  if (!r) return null
  const t = await getT(ogMessages)
  const { task, entries } = r
  const votes = entries.reduce((n, e) => n + e.votes, 0)
  const leader = entries[0] // the results come ranked by the task kind's rule
  const phase = task.phase === 'open' ? t('phaseOpen') : task.phase === 'voting' ? t('phaseVoting') : t('phaseFinal')
  const stats: OgStat[] = [{ label: t('statEntries'), value: task.entry_count }]
  if (task.phase !== 'open' && !(task.kind === 'site' && task.phase === 'voting')) stats.push({ label: t('statVotes'), value: votes }) // site votes are blind until voting ends
  if (task.phase !== 'open' && leader?.handle) stats.push({ label: task.phase === 'final' ? t('statWinner') : t('statLeading'), value: clip(leader.handle, 14) })
  return {
    title: task.title,
    description: t('productDescription', {
      summary: task.summary || task.title,
      entries: t.plural('entries', task.entry_count),
      state: task.phase === 'voting' ? t('stateVoting') : task.phase === 'final' ? t('stateFinal') : t('stateOpen'),
    }),
    card: {
      kicker: t('kickerProduct', { kind: task.kind === 'site' ? t('kindSite') : t('kindCli') }),
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
  const t = await getT(ogMessages)
  return {
    title: b.name,
    description: t('botDescription', { name: b.name, owner: b.owner || t('theHouse'), rating: b.rating, wins: b.wins, matches: t.plural('matches', b.matches), n: b.matches }),
  }
}

export async function matchShare(id: string): Promise<{ title: string; description: string } | null> {
  const m = await serverApi<MatchView>(`/tanks/matches/${encodeURIComponent(id)}`)
  if (!m) return null
  const t = await getT(ogMessages)
  const names = [...m.players].sort((a, b) => (a.place ?? 9) - (b.place ?? 9)).map((p) => p.name)
  return {
    title: names.slice(0, 2).join(' vs '),
    description: t('matchDescription', { names: names.join(', ') }),
  }
}

export async function seasonShare(id: string): Promise<{ title: string; description: string } | null> {
  const s = await serverApi<SeasonDetail>(`/tanks/seasons/${encodeURIComponent(id)}`)
  if (!s) return null
  const t = await getT(ogMessages)
  const top = s.standings[0]
  return {
    title: s.season.name,
    description: t('seasonDescription', { name: s.season.name, top: top ? t('seasonTop', { name: top.name, rating: top.rating }) : '' }),
  }
}
