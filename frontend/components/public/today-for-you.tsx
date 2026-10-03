'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { Code2, Swords, Trophy } from 'lucide-react'
import { Countdown } from '@/components/daily/countdown'
import { Countdown as TanksCountdown } from '@/components/tanks/countdown'
import { RecapCard } from '@/components/public/recap-card'
import { api, products, retention, tanks } from '@/lib/api'
import { fmtScore } from '@/components/daily/daily-board'
import { useMe } from '@/lib/use-me'
import { useT } from '@/lib/i18n/client'
import { formatDate, type T } from '@/lib/i18n/core'
import { shellMessages } from '@/lib/i18n/messages/shell'
import type { Daily, MyTanks, Recap, ProductDetail, Showcase, SeasonDetail } from '@/lib/types'

type TT = T<typeof shellMessages.en>
type Tile = { key: string; icon: typeof Code2; label: string; href: string; headline: React.ReactNode; sub: React.ReactNode }

const shortDate = (t: TT, iso: string) => formatDate(t.locale, iso, { day: 'numeric', month: 'short' })

async function dailyTile(t: TT): Promise<Tile> {
  const d = await api<Daily>('/daily')
  const best = d.my?.best
  const optimize = d.task.kind === 'optimize'
  let headline: React.ReactNode = t('notSubmitted')
  if (best) headline = optimize ? t('bestScore', { score: fmtScore(best.score) }) : t('bestTests', { passed: best.passed_tests, total: best.total_tests })
  else if (d.my && d.my.attempts_used > 0) headline = t.plural('uploadsNoResult', d.my.attempts_used)
  return {
    key: 'daily', icon: Code2, label: t('taskOfDay'), href: '/#today', headline: <span title={d.task.title}>{headline}</span>,
    sub: d.is_open ? <>{t('closesIn')}<Countdown closesAt={d.closes_at} /></> : t('closedToday'),
  }
}

async function productTile(t: TT): Promise<Tile> {
  const list = await products.list()
  const cur = list.items.find((t) => t.phase === 'open' || t.phase === 'voting')
  if (!cur) {
    const next = list.upcoming?.next_opens_at
    return {
      key: 'products', icon: Trophy, label: t('productOfWeek'), href: '/products',
      headline: t('nothingOpen'), sub: next ? t('nextOpens', { date: shortDate(t, next) }) : t('pastWinners'),
    }
  }
  const detail: ProductDetail = await products.get(cur.slug)
  const entered = detail.mine.length > 0
  const entry = entered ? t.plural('uploadsSent', detail.mine.length) : t('notEntered')
  if (cur.phase === 'open') {
    return {
      key: 'products', icon: Trophy, label: t('productOfWeek'), href: `/products/${cur.slug}`,
      headline: cur.title, sub: <>{entry}{t('closesInLower')}<Countdown closesAt={cur.deadline} /></>,
    }
  }
  let todo: React.ReactNode = <>{t('votingEnds')}<Countdown closesAt={cur.voting_ends_at} /></>
  if (cur.kind === 'site') {
    try {
      const c = await products.compareNext(cur.slug)
      const left = Math.max(0, c.target - c.judged)
      todo = c.pair && left > 0 ? t.plural('pairs', left, { judged: c.judged, target: c.target }) : t('allJudged', { judged: c.judged, target: c.target })
    } catch {}
  }
  return {
    key: 'products', icon: Trophy, label: t('productOfWeek'), href: `/products/${cur.slug}`,
    headline: todo, sub: <span title={cur.title}>{entry}</span>,
  }
}

async function tanksTile(t: TT): Promise<Tile> {
  // The showcase ladder is cut to the top 10, so the rank comes from the full season standings.
  const [mine, show, season] = await Promise.allSettled([api<MyTanks>('/me/tanks'), api<Showcase>('/tanks/showcase'), tanks.season('current')])
  if (mine.status === 'rejected' && show.status === 'rejected') throw mine.reason
  const bot = mine.status === 'fulfilled' ? mine.value.bot : null
  const s = show.status === 'fulfilled' ? show.value : null
  let headline: React.ReactNode
  if (mine.status === 'rejected') headline = t('tanksArena')
  else if (!bot) headline = t('noBot')
  else {
    const standings: SeasonDetail['standings'] = season.status === 'fulfilled' ? season.value.standings : (s?.ladder ?? [])
    const rank = standings.find((l) => l.bot_id === bot.id)?.rank
    headline = <>{rank ? `#${rank} · ` : ''}{bot.name} <span className="font-mono">{Math.round(bot.rating)}</span></>
  }
  let sub: React.ReactNode = !bot && mine.status === 'fulfilled' ? t('buildOne') : t('seeLadder')
  if (s?.next_tournament) sub = <>{t('nextTournament')}<TanksCountdown to={s.next_tournament.starts_at} serverNow={s.now} /></>
  else if (s?.tournament && s.tournament.status === 'running') sub = t('tournamentRunning')
  return { key: 'tanks', icon: Swords, label: t('tanksArena'), href: bot || mine.status === 'rejected' ? '/tanks' : '/app/tanks', headline, sub }
}

// A compact row for signed-in people on the home page: where they stand in each of the three modes.
// Every tile loads on its own and is dropped when its requests fail.
export function TodayForYou() {
  const { me } = useMe()
  const t = useT(shellMessages)
  const [tiles, setTiles] = useState<Record<string, Tile | null>>({})
  const [recap, setRecap] = useState<Recap | null>(null)
  const handle = me?.user.handle

  useEffect(() => {
    if (!handle) return
    let live = true
    for (const load of [dailyTile, productTile, tanksTile]) {
      load(t)
        .then((t) => live && setTiles((cur) => ({ ...cur, [t.key]: t })))
        .catch(() => {})
    }
    retention.recap().then((r) => live && setRecap(r)).catch(() => {})
    return () => { live = false }
  }, [handle, t])

  const shown = ['daily', 'products', 'tanks'].map((k) => tiles[k]).filter((t): t is Tile => !!t)
  if (!me || (shown.length === 0 && !recap)) return null
  return (
    <section aria-label={t('forYou')} className="mb-8">
      <h2 className="mb-2 text-xs font-bold uppercase tracking-wider text-muted-foreground">{t('forYou')}</h2>
      {recap && <RecapCard recap={recap} />}
      <ul className="grid gap-2 sm:grid-cols-3">
        {shown.map((t) => (
          <li key={t.key} className="min-w-0">
            <Link href={t.href} className="group flex h-full flex-col gap-0.5 rounded-[12px] border border-border bg-card px-3.5 py-2.5 transition-colors hover:border-primary/50">
              <span className="flex items-center gap-1.5 text-xs font-semibold text-muted-foreground">
                <t.icon className="size-3.5 text-primary" />{t.label}
              </span>
              <span className="truncate text-sm font-bold group-hover:text-primary">{t.headline}</span>
              <span className="truncate text-xs text-muted-foreground">{t.sub}</span>
            </Link>
          </li>
        ))}
      </ul>
    </section>
  )
}
