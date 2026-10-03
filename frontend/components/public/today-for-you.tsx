'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { Code2, Swords, Trophy } from 'lucide-react'
import { Countdown } from '@/components/daily/countdown'
import { Countdown as TanksCountdown } from '@/components/tanks/countdown'
import { api, products, tanks } from '@/lib/api'
import { fmtScore } from '@/components/daily/daily-board'
import { useMe } from '@/lib/use-me'
import type { Daily, MyTanks, ProductDetail, Showcase, SeasonDetail } from '@/lib/types'

type Tile = { key: string; icon: typeof Code2; label: string; href: string; headline: React.ReactNode; sub: React.ReactNode }

const shortDate = (iso: string) => new Date(iso).toLocaleDateString('en-GB', { day: 'numeric', month: 'short' })

async function dailyTile(): Promise<Tile> {
  const d = await api<Daily>('/daily')
  const best = d.my?.best
  const optimize = d.task.kind === 'optimize'
  let headline: React.ReactNode = 'Not submitted yet'
  if (best) headline = optimize ? `Best score ${fmtScore(best.score)}` : `Best ${best.passed_tests}/${best.total_tests} tests`
  else if (d.my && d.my.attempts_used > 0) headline = `${d.my.attempts_used} upload${d.my.attempts_used === 1 ? '' : 's'}, no result yet`
  return {
    key: 'daily', icon: Code2, label: 'Task of the day', href: '/#today', headline: <span title={d.task.title}>{headline}</span>,
    sub: d.is_open ? <>Closes in <Countdown closesAt={d.closes_at} /></> : 'Closed for today',
  }
}

async function productTile(): Promise<Tile> {
  const list = await products.list()
  const cur = list.items.find((t) => t.phase === 'open' || t.phase === 'voting')
  if (!cur) {
    const next = list.upcoming?.next_opens_at
    return {
      key: 'products', icon: Trophy, label: 'Product of the week', href: '/products',
      headline: 'Nothing open right now', sub: next ? `Next task opens ${shortDate(next)}` : 'See past winners',
    }
  }
  const detail: ProductDetail = await products.get(cur.slug)
  const entered = detail.mine.length > 0
  const entry = entered ? `${detail.mine.length} upload${detail.mine.length === 1 ? '' : 's'} sent` : 'Not entered yet'
  if (cur.phase === 'open') {
    return {
      key: 'products', icon: Trophy, label: 'Product of the week', href: `/products/${cur.slug}`,
      headline: cur.title, sub: <>{entry} · closes in <Countdown closesAt={cur.deadline} /></>,
    }
  }
  let todo: React.ReactNode = <>Voting ends in <Countdown closesAt={cur.voting_ends_at} /></>
  if (cur.kind === 'site') {
    try {
      const c = await products.compareNext(cur.slug)
      const left = Math.max(0, c.target - c.judged)
      todo = c.pair && left > 0 ? `${left} pair${left === 1 ? '' : 's'} to judge (${c.judged}/${c.target})` : `All judged (${c.judged}/${c.target})`
    } catch {}
  }
  return {
    key: 'products', icon: Trophy, label: 'Product of the week', href: `/products/${cur.slug}`,
    headline: todo, sub: <span title={cur.title}>{entry}</span>,
  }
}

async function tanksTile(): Promise<Tile> {
  // The showcase ladder is cut to the top 10, so the rank comes from the full season standings.
  const [mine, show, season] = await Promise.allSettled([api<MyTanks>('/me/tanks'), api<Showcase>('/tanks/showcase'), tanks.season('current')])
  if (mine.status === 'rejected' && show.status === 'rejected') throw mine.reason
  const bot = mine.status === 'fulfilled' ? mine.value.bot : null
  const s = show.status === 'fulfilled' ? show.value : null
  let headline: React.ReactNode
  if (mine.status === 'rejected') headline = 'Tanks arena'
  else if (!bot) headline = 'No bot yet'
  else {
    const standings: SeasonDetail['standings'] = season.status === 'fulfilled' ? season.value.standings : (s?.ladder ?? [])
    const rank = standings.find((l) => l.bot_id === bot.id)?.rank
    headline = <>{rank ? `#${rank} · ` : ''}{bot.name} <span className="font-mono">{Math.round(bot.rating)}</span></>
  }
  let sub: React.ReactNode = !bot && mine.status === 'fulfilled' ? 'Build one with your agent' : 'See the ladder'
  if (s?.next_tournament) sub = <>Next tournament in <TanksCountdown to={s.next_tournament.starts_at} serverNow={s.now} /></>
  else if (s?.tournament && s.tournament.status === 'running') sub = 'Tournament running now'
  return { key: 'tanks', icon: Swords, label: 'Tanks arena', href: bot || mine.status === 'rejected' ? '/tanks' : '/app/tanks', headline, sub }
}

// A compact row for signed-in people on the home page: where they stand in each of the three modes.
// Every tile loads on its own and is dropped when its requests fail.
export function TodayForYou() {
  const { me } = useMe()
  const [tiles, setTiles] = useState<Record<string, Tile | null>>({})
  const handle = me?.user.handle

  useEffect(() => {
    if (!handle) return
    let live = true
    for (const load of [dailyTile, productTile, tanksTile]) {
      load()
        .then((t) => live && setTiles((cur) => ({ ...cur, [t.key]: t })))
        .catch(() => {})
    }
    return () => { live = false }
  }, [handle])

  const shown = ['daily', 'products', 'tanks'].map((k) => tiles[k]).filter((t): t is Tile => !!t)
  if (!me || shown.length === 0) return null
  return (
    <section aria-label="Today for you" className="mb-8">
      <h2 className="mb-2 text-xs font-bold uppercase tracking-wider text-muted-foreground">Today for you</h2>
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
