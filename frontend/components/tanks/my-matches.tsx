'use client'

import { useState } from 'react'
import Link from 'next/link'
import { ChevronDown } from 'lucide-react'
import { api } from '@/lib/api'
import type { MatchLog, MatchView } from '@/lib/types'
import { cn } from '@/lib/utils'
import { CopyReportButton } from './copy-report-button'
import { useT, useLocale } from '@/lib/i18n/client'
import { tanksOwnerMessages as m } from '@/lib/i18n/messages/tanks-owner'
import { intlLocale, type Locale } from '@/lib/i18n/core'

function place(mv: MatchView, botId: string): string {
  const mine = mv.players.find((p) => p.bot_id === botId)
  return mine?.place != null ? `#${mine.place}` : '—'
}

function opponents(mv: MatchView, botId: string): string {
  return mv.players.filter((p) => p.bot_id !== botId).map((p) => p.name).join(', ') || '—'
}

// "5 minutes ago" in the reader's language.
function agoIn(locale: Locale, iso: string): string {
  const s = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000))
  const rtf = new Intl.RelativeTimeFormat(intlLocale(locale), { numeric: 'auto', style: 'short' })
  if (s < 60) return rtf.format(-s, 'second')
  if (s < 3600) return rtf.format(-Math.round(s / 60), 'minute')
  if (s < 86400) return rtf.format(-Math.round(s / 3600), 'hour')
  return rtf.format(-Math.round(s / 86400), 'day')
}

type LogState = 'loading' | 'error' | { text: string }

// The owner's own recent matches: place, who they played, a link to the
// full match (replay included), and their own bot's stderr for that match
// on demand — loaded lazily, once, the first time it's opened.
export function MyMatches({ matches, botId }: { matches: MatchView[]; botId: string }) {
  const t = useT(m)
  const locale = useLocale()
  const [openId, setOpenId] = useState<string | null>(null)
  const [logs, setLogs] = useState<Record<string, LogState>>({})

  async function toggle(id: string) {
    if (openId === id) {
      setOpenId(null)
      return
    }
    setOpenId(id)
    if (logs[id]) return
    setLogs((l) => ({ ...l, [id]: 'loading' }))
    try {
      const log = await api<MatchLog>(`/me/tanks/matches/${id}/log`)
      setLogs((l) => ({ ...l, [id]: { text: log.stderr } }))
    } catch {
      setLogs((l) => ({ ...l, [id]: 'error' }))
    }
  }

  if (matches.length === 0) {
    return (
      <p className="rounded-xl border border-dashed border-strong px-5 py-8 text-center text-sm text-muted-foreground">
        {t('mm.empty')}
      </p>
    )
  }

  return (
    <ul className="divide-y divide-border rounded-xl border border-border bg-card">
      {matches.map((match) => {
        const log = logs[match.id]
        const isOpen = openId === match.id
        return (
          <li key={match.id} className="p-4">
            <div className="flex flex-wrap items-center gap-3">
              <span className="w-9 shrink-0 font-mono text-sm font-bold">{place(match, botId)}</span>
              <Link href={`/tanks/matches/${match.id}`} className="min-w-0 flex-1 truncate text-sm font-semibold hover:text-primary">
                {t('mm.vs', { names: opponents(match, botId) })}
              </Link>
              <span className="shrink-0 text-xs text-muted-foreground">{match.finished_at ? agoIn(locale, match.finished_at) : match.status}</span>
              <CopyReportButton matchIds={[match.id]} size="xs" variant="ghost" label={t('mm.copy')} className="shrink-0" />
              <button type="button" onClick={() => void toggle(match.id)} aria-expanded={isOpen} aria-controls={`match-log-${match.id}`}
                className="flex shrink-0 items-center gap-1 text-xs font-semibold text-primary hover:underline">
                {t('mm.myLog')}<ChevronDown className={cn('size-3.5 transition-transform', isOpen && 'rotate-180')} />
              </button>
            </div>
            {isOpen && (
              <pre id={`match-log-${match.id}`} className="mt-3 max-h-64 overflow-auto rounded-lg border border-border bg-muted/40 p-3 font-mono text-xs leading-5 whitespace-pre-wrap break-words">
                {log === 'loading' ? t('mm.loading') : log === 'error' ? t('mm.logError') : log?.text || t('mm.noOutput')}
              </pre>
            )}
          </li>
        )
      })}
    </ul>
  )
}
