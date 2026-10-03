'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { notFound } from 'next/navigation'
import { Check, X } from 'lucide-react'
import { SectionTitle } from '@/components/page-header'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { api, ApiError, friendlyMessage } from '@/lib/api'
import type { Profile, ProfileDay } from '@/lib/types'

// The last 12 weeks as a grid of days, GitHub-style: solved, tried, or empty.
function Calendar({ days }: { days: ProfileDay[] }) {
  const byDay = new Map(days.map((d) => [d.day, d]))
  const today = new Date()
  const cells: { day: string; d?: ProfileDay }[] = []
  for (let i = 83; i >= 0; i--) {
    const t = new Date(Date.UTC(today.getUTCFullYear(), today.getUTCMonth(), today.getUTCDate() - i))
    const day = t.toISOString().slice(0, 10)
    cells.push({ day, d: byDay.get(day) })
  }
  return (
    <div className="grid grid-flow-col grid-rows-7 justify-start gap-1">
      {cells.map(({ day, d }) => (
        <div key={day} title={d ? `${day}: ${d.passed_tests}/${d.total_tests}` : day}
          className={`size-3 rounded-[3px] ${!d ? 'bg-muted' : d.status === 'passed' ? 'bg-success' : 'bg-primary/40'}`} />
      ))}
    </div>
  )
}

function Stat({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="rounded-[14px] border border-border bg-card p-4">
      <div className="font-mono text-2xl font-bold">{value}</div>
      <div className="text-xs text-muted-foreground">{label}</div>
    </div>
  )
}

export function ProfileView({ handle }: { handle: string }) {
  const [p, setP] = useState<Profile | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [missing, setMissing] = useState(false)
  useEffect(() => {
    api<Profile>(`/users/${encodeURIComponent(handle)}`).then(setP).catch((e) => {
      if (e instanceof ApiError && e.status === 404) setMissing(true)
      else setError(friendlyMessage(e))
    })
  }, [handle])

  if (missing) notFound()
  if (error) return <p role="alert" className="text-sm text-destructive">{error}</p>
  if (!p) return <Skeleton className="h-64 rounded-[14px]" />

  return (
    <div className="space-y-10">
      <header className="space-y-2">
        <h1 className="display text-[2.1rem] break-words sm:text-[2.75rem]">{p.handle}</h1>
        <p className="text-sm text-muted-foreground">
          Joined {p.joined_at.slice(0, 10)}
          {p.tools.length > 0 && <> · works with {p.tools.join(', ')}</>}
        </p>
      </header>

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <Stat label="Place overall" value={p.place ? `#${p.place}` : '—'} />
        <Stat label="Current streak" value={`🔥 ${p.streak.current}`} />
        <Stat label="Best streak" value={p.streak.best} />
        <Stat label="Days solved" value={`${p.solved_days}/${p.played_days}`} />
      </div>

      <section>
        <SectionTitle>Last 12 weeks</SectionTitle>
        <div className="overflow-x-auto rounded-[14px] border border-border bg-card p-4"><Calendar days={p.days} /></div>
      </section>

      <section>
        <SectionTitle>History</SectionTitle>
        {p.days.length === 0 ? (
          <p className="rounded-[14px] border border-dashed border-input px-5 py-8 text-center text-sm text-muted-foreground">No daily tasks yet.</p>
        ) : (
          <ul className="divide-y divide-border rounded-[14px] border border-border bg-card">
            {p.days.map((d) => (
              <li key={d.day} className="flex flex-wrap items-center gap-x-3 gap-y-1 p-4">
                {d.status === 'passed' ? <Check className="size-4 shrink-0 text-success" /> : <X className="size-4 shrink-0 text-destructive" />}
                <span className="font-mono text-xs text-muted-foreground">{d.day}</span>
                <Link href={`/day/${d.day}`} className="min-w-0 flex-1 truncate font-semibold hover:text-primary hover:underline">{d.task.title}</Link>
                <span className="font-mono text-sm font-bold">
                  {d.score != null ? d.score : `${d.passed_tests}/${d.total_tests}`}
                </span>
                {d.made_with && <Badge variant="outline">{d.made_with}</Badge>}
                <span className="text-xs text-muted-foreground">{d.attempts} attempt{d.attempts === 1 ? '' : 's'}</span>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  )
}
