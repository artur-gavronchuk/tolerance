'use client'

import { useCallback, useEffect, useRef, useState } from 'react'
import Link from 'next/link'
import { Bell } from 'lucide-react'
import { StreakStrip } from '@/components/profile/streak-strip'
import { fmtScore } from '@/components/daily/daily-board'
import { retention } from '@/lib/api'
import { useT } from '@/lib/i18n/client'
import { formatDateTime, type T } from '@/lib/i18n/core'
import { notifyMessages } from '@/lib/i18n/messages/notify'
import type { AppNotification, Recap } from '@/lib/types'

type TT = T<typeof notifyMessages.en>

const POLL_MS = 60_000
const today = () => new Date().toISOString().slice(0, 10)

// Wording and target of one notification, from its type and stored params.
function render(t: TT, n: AppNotification): { text: string; href: string } {
  const p = n.params as Record<string, string | number>
  const num = (k: string) => Number(p[k] ?? 0)
  switch (n.type) {
    case 'daily_verdict': {
      const href = p.day === today() ? '/' : `/day/${p.day}`
      const vars = { title: String(p.title), passed: num('passed'), total: num('total'), score: fmtScore(p.score == null ? null : Number(p.score), t.locale) }
      if (p.status === 'infra_error') return { text: t('verdictInfra', vars), href }
      if (p.kind === 'optimize' && p.score != null) return { text: t('verdictScore', vars), href }
      return { text: t(p.status === 'passed' ? 'verdictPassed' : 'verdictFailed', vars), href }
    }
    case 'daily_final':
      return { text: t('dailyFinal', { title: String(p.title), place: num('place'), of: num('of') }), href: `/day/${p.day}` }
    case 'tournament_soon':
      return {
        text: t('tournamentSoon', { name: String(p.name), time: formatDateTime(t.locale, String(p.starts_at), { hour: '2-digit', minute: '2-digit', timeZone: 'UTC' }) }),
        href: `/tanks/tournaments/${p.id}`,
      }
    case 'tournament_entered':
      return { text: t('tournamentEntered', { bot: String(p.bot), name: String(p.name) }), href: `/tanks/tournaments/${p.id}` }
    case 'tournament_won':
      return { text: t('tournamentWon', { bot: String(p.bot), name: String(p.name) }), href: `/tanks/tournaments/${p.id}` }
    case 'tournament_lost':
      return { text: t('tournamentLost', { bot: String(p.bot), name: String(p.name), round: num('round'), rounds: num('rounds') }), href: `/tanks/tournaments/${p.id}` }
    case 'rank_drop':
      return { text: t('rankDrop', { bot: String(p.bot), from: num('from'), to: num('to'), season: String(p.season) }), href: '/tanks/leaderboard' }
  }
}

// The bell in the site header: unread count and a dropdown list. Opening it marks everything read (the rows that were
// unread stay highlighted until it is closed). The footer repeats the streak.
export function NotificationBell() {
  const t = useT(notifyMessages)
  const [items, setItems] = useState<AppNotification[]>([])
  const [unread, setUnread] = useState(0)
  const [open, setOpen] = useState(false)
  const [fresh, setFresh] = useState<Set<string>>(new Set())
  const [recap, setRecap] = useState<Recap | null>(null)
  const box = useRef<HTMLDivElement>(null)

  const load = useCallback(async () => {
    try {
      const l = await retention.notifications()
      setItems(l.items)
      setUnread(l.unread)
      return l
    } catch {}
  }, [])

  useEffect(() => {
    void load()
    const id = setInterval(() => { if (!document.hidden) void load() }, POLL_MS)
    return () => clearInterval(id)
  }, [load])

  useEffect(() => {
    if (!open) return
    const away = (e: MouseEvent) => { if (!box.current?.contains(e.target as Node)) setOpen(false) }
    const esc = (e: KeyboardEvent) => { if (e.key === 'Escape') setOpen(false) }
    document.addEventListener('mousedown', away)
    document.addEventListener('keydown', esc)
    return () => { document.removeEventListener('mousedown', away); document.removeEventListener('keydown', esc) }
  }, [open])

  async function toggle() {
    if (open) { setOpen(false); return }
    setOpen(true)
    retention.recap().then(setRecap).catch(() => {})
    const l = await load()
    if (l && l.unread > 0) {
      setFresh(new Set(l.items.filter((n) => !n.read).map((n) => n.id)))
      setUnread(0)
      retention.markNotificationsRead().catch(() => {})
    } else setFresh(new Set())
  }

  return (
    <div ref={box} className="relative">
      <button onClick={() => void toggle()} aria-haspopup="true" aria-expanded={open}
        aria-label={unread > 0 ? t('bellUnread', { n: unread }) : t('bell')} title={t('bell')}
        className="relative flex size-8 items-center sm:size-9 justify-center rounded-full text-muted-foreground hover:bg-muted hover:text-foreground">
        <Bell className="size-4" />
        {unread > 0 && (
          <span className="absolute right-0.5 top-0.5 flex min-w-4 items-center justify-center rounded-full bg-primary px-1 text-[10px] font-bold leading-4 text-primary-foreground">
            {unread > 9 ? '9+' : unread}
          </span>
        )}
      </button>
      {open && (
        <div className="fixed inset-x-3 top-[4.25rem] z-50 rounded-xl border border-border bg-card shadow-lg sm:absolute sm:inset-x-auto sm:right-0 sm:top-full sm:mt-2 sm:w-96">
          <ul className="max-h-[60vh] divide-y divide-border overflow-y-auto">
            {items.length === 0 && <li className="px-4 py-6 text-center text-sm text-muted-foreground">{t('empty')}</li>}
            {items.map((n) => {
              const r = render(t, n)
              return (
                <li key={n.id}>
                  <Link href={r.href} onClick={() => setOpen(false)}
                    className={`block px-4 py-2.5 text-sm hover:bg-muted ${fresh.has(n.id) ? 'bg-primary/5' : ''}`}>
                    <span className="block">{r.text}</span>
                    <span className="text-xs text-muted-foreground">{formatDateTime(t.locale, n.created_at)}</span>
                  </Link>
                </li>
              )
            })}
          </ul>
          {recap && <div className="border-t border-border px-4 py-3"><StreakStrip solved={recap.solved_days} streak={recap.streak} /></div>}
        </div>
      )}
    </div>
  )
}
