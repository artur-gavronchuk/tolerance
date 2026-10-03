'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { PageHeader } from '@/components/page-header'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { api, friendlyMessage, stacks } from '@/lib/api'
import { cn } from '@/lib/utils'
import type { DayListItem, StackRow } from '@/lib/types'

type Range = 'all' | '30' | '7' | 'day'

const RANGES: { id: Range; label: string }[] = [
  { id: 'all', label: 'All time' },
  { id: '30', label: '30 days' },
  { id: '7', label: '7 days' },
  { id: 'day', label: 'One day' },
]

const pct = (v: number | null) => (v === null ? '–' : `${Math.round(v * 100)}%`)

export default function AgentsPage() {
  const [range, setRange] = useState<Range>('all')
  const [days, setDays] = useState<DayListItem[]>([])
  const [day, setDay] = useState('')
  const [rows, setRows] = useState<StackRow[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    api<{ items: DayListItem[] }>('/days').then((r) => setDays(r.items)).catch(() => {})
  }, [])

  useEffect(() => {
    if (range === 'day' && !day) return
    let stale = false
    setRows(null)
    setError(null)
    const req = range === 'day' ? stacks.forDay(day) : stacks.overall(range === 'all' ? undefined : Number(range))
    req.then((r) => { if (!stale) setRows(r) }).catch((e) => { if (!stale) setError(friendlyMessage(e)) })
    return () => { stale = true }
  }, [range, day])

  const pickDay = (d: string) => { setDay(d); setRange('day') }

  return (
    <div className="space-y-8">
      <PageHeader title="Agents">
        Real people run their own agents on the same task every day, so we can see which stacks actually deliver. Each
        submission&apos;s free-text &ldquo;made with&rdquo; is sorted into a tool and a model. A day is worth up to 100 points
        (the share of hidden tests passed on a bugfix day, the score against the day&apos;s best on an optimize day) and a
        stack is ranked by its average. Solve rate and attempts to first pass count bugfix days only. Each person-day counts
        once, under the stack of the attempt that decided it. Small samples are noisy.
      </PageHeader>

      <div className="flex flex-wrap items-center gap-2">
        {RANGES.map((r) => (
          <button key={r.id} type="button" onClick={() => (r.id === 'day' ? pickDay(day || days[0]?.day || '') : setRange(r.id))}
            disabled={r.id === 'day' && days.length === 0}
            className={cn(
              'rounded-full px-3 py-1.5 text-sm font-semibold text-muted-foreground transition-colors hover:text-foreground disabled:opacity-50',
              range === r.id && 'bg-muted text-foreground'
            )}>
            {r.label}
          </button>
        ))}
        {range === 'day' && day && (
          <>
            <select aria-label="Day" value={day} onChange={(e) => pickDay(e.target.value)}
              className="h-9 max-w-full min-w-0 rounded-full border border-input bg-card px-3 font-mono text-sm">
              {days.map((d) => <option key={d.day} value={d.day}>{d.day} · {d.task.title}</option>)}
            </select>
            <Link href={`/day/${day}`} className="text-sm font-semibold text-primary hover:underline">Open that day</Link>
          </>
        )}
      </div>

      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      {!rows && !error && <Skeleton className="h-64 rounded-[14px]" />}
      {rows && rows.length === 0 && (
        <p className="rounded-[14px] border border-dashed border-input px-5 py-10 text-center text-sm text-muted-foreground">No finished submissions yet.</p>
      )}
      {rows && rows.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-10">#</TableHead>
              <TableHead>Stack</TableHead>
              <TableHead className="text-right">Avg points</TableHead>
              <TableHead className="text-right">Solve rate</TableHead>
              <TableHead className="text-right">Tests passed</TableHead>
              <TableHead className="text-right">Attempts to pass</TableHead>
              <TableHead className="text-right">People</TableHead>
              <TableHead className="text-right">Days</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((r, i) => (
              <TableRow key={r.label}>
                <TableCell className="font-mono text-muted-foreground">{i + 1}</TableCell>
                <TableCell className="min-w-[10rem] font-semibold">{r.label}</TableCell>
                <TableCell className="text-right font-mono font-bold">{r.avg_points}</TableCell>
                <TableCell className="text-right font-mono">
                  {pct(r.solve_rate)}
                  {r.bugfix_days > 0 && <span className="text-muted-foreground"> ({r.days_solved}/{r.bugfix_days})</span>}
                </TableCell>
                <TableCell className="text-right font-mono text-muted-foreground">{pct(r.avg_tests_share)}</TableCell>
                <TableCell className="text-right font-mono text-muted-foreground">{r.avg_attempts_to_pass ?? '–'}</TableCell>
                <TableCell className="text-right font-mono text-muted-foreground">{r.users}</TableCell>
                <TableCell className="text-right font-mono text-muted-foreground">{r.days_attempted}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  )
}
