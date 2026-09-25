'use client'

import { useState } from 'react'
import Link from 'next/link'
import { ChevronDown } from 'lucide-react'
import { api } from '@/lib/api'
import { ago } from '@/lib/format'
import type { MatchLog, MatchView } from '@/lib/types'
import { cn } from '@/lib/utils'

function place(mv: MatchView, botId: string): string {
  const mine = mv.players.find((p) => p.bot_id === botId)
  return mine?.place != null ? `#${mine.place}` : '—'
}

function opponents(mv: MatchView, botId: string): string {
  return mv.players.filter((p) => p.bot_id !== botId).map((p) => p.name).join(', ') || '—'
}

type LogState = 'loading' | 'error' | { text: string }

// The owner's own recent matches: place, who they played, a link to the
// full match (replay included), and their own bot's stderr for that match
// on demand — loaded lazily, once, the first time it's opened.
export function MyMatches({ matches, botId }: { matches: MatchView[]; botId: string }) {
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
      <p className="rounded-[14px] border border-dashed border-input px-5 py-8 text-center text-sm text-muted-foreground">
        No matches yet.
      </p>
    )
  }

  return (
    <ul className="divide-y divide-border rounded-[14px] border border-border bg-card">
      {matches.map((m) => {
        const log = logs[m.id]
        const isOpen = openId === m.id
        return (
          <li key={m.id} className="p-4">
            <div className="flex flex-wrap items-center gap-3">
              <span className="w-9 shrink-0 font-mono text-sm font-bold">{place(m, botId)}</span>
              <Link href={`/tanks/matches/${m.id}`} className="min-w-0 flex-1 truncate text-sm font-semibold hover:text-primary">
                vs {opponents(m, botId)}
              </Link>
              <span className="shrink-0 text-xs text-muted-foreground">{m.finished_at ? ago(m.finished_at) : m.status}</span>
              <button type="button" onClick={() => void toggle(m.id)} aria-expanded={isOpen} aria-controls={`match-log-${m.id}`}
                className="flex shrink-0 items-center gap-1 text-xs font-semibold text-primary hover:underline">
                My bot&apos;s log<ChevronDown className={cn('size-3.5 transition-transform', isOpen && 'rotate-180')} />
              </button>
            </div>
            {isOpen && (
              <pre id={`match-log-${m.id}`} className="mt-3 max-h-64 overflow-auto rounded-[10px] border border-border bg-muted/40 p-3 font-mono text-xs leading-5 whitespace-pre-wrap break-words">
                {log === 'loading' ? 'Loading…' : log === 'error' ? 'Could not load the log.' : log?.text || 'No output.'}
              </pre>
            )}
          </li>
        )
      })}
    </ul>
  )
}
