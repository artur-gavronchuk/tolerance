'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { PageHeader } from '@/components/page-header'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { api, stacks } from '@/lib/api'
import { cn } from '@/lib/utils'
import type { DayListItem, StackRow } from '@/lib/types'
import { localizeStack } from '@/components/daily/stack-label'
import { errorText } from '@/lib/format'
import { useT } from '@/lib/i18n/client'
import { listingMessages } from '@/lib/i18n/messages/listings'

type Range = 'all' | '30' | '7' | 'day'

const RANGES: { id: Range; label: 'rangeAll' | 'range30' | 'range7' | 'rangeDay' }[] = [
  { id: 'all', label: 'rangeAll' },
  { id: '30', label: 'range30' },
  { id: '7', label: 'range7' },
  { id: 'day', label: 'rangeDay' },
]

const pct = (v: number | null) => (v === null ? '–' : `${Math.round(v * 100)}%`)

export default function AgentsPage() {
  const t = useT(listingMessages)
  const [range, setRange] = useState<Range>('all')
  const [days, setDays] = useState<DayListItem[]>([])
  const [daysLoaded, setDaysLoaded] = useState(false)
  const [day, setDay] = useState('')
  const [rows, setRows] = useState<StackRow[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    api<{ items: DayListItem[] }>('/days').then((r) => { setDays(r.items); setDaysLoaded(true) }).catch(() => {})
  }, [])

  useEffect(() => {
    if (range === 'day' && !day) return
    let stale = false
    setRows(null)
    setError(null)
    const req = range === 'day' ? stacks.forDay(day) : stacks.overall(range === 'all' ? undefined : Number(range))
    req.then((r) => { if (!stale) setRows(r) }).catch((e) => { if (!stale) setError(errorText(e, t.locale)) })
    return () => { stale = true }
  }, [range, day])

  // One day is unavailable until the list has loaded; only say why once we know it is empty.
  const noPastDays = daysLoaded && days.length === 0

  const pickDay = (d: string) => { setDay(d); setRange('day') }

  return (
    <div className="space-y-8">
      <PageHeader title={t('agentsTitle')}>{t('agentsIntro')}</PageHeader>

      <details className="group max-w-2xl rounded-[14px] border border-border bg-card px-4 py-3 text-sm text-muted-foreground">
        <summary className="cursor-pointer font-semibold text-foreground">{t('howScoring')}</summary>
        <ul className="mt-3 list-disc space-y-1.5 pl-5 leading-relaxed">
          <li>{t('scoring1')}</li>
          <li>{t('scoring2')}</li>
          <li>{t('scoring3')}</li>
          <li>{t('scoring4')}</li>
        </ul>
      </details>

      <div className="flex flex-wrap items-center gap-2">
        {RANGES.map((r) => (
          <button key={r.id} type="button" onClick={() => (r.id === 'day' ? pickDay(day || days[0]?.day || '') : setRange(r.id))}
            disabled={r.id === 'day' && (!daysLoaded || days.length === 0)}
            title={r.id === 'day' && noPastDays ? t('dayLocked') : undefined}
            className={cn(
              'rounded-full px-3 py-1.5 text-sm font-semibold text-muted-foreground transition-colors hover:text-foreground disabled:opacity-50',
              range === r.id && 'bg-muted text-foreground'
            )}>
            {t(r.label)}
          </button>
        ))}
        {noPastDays && <span className="text-xs text-muted-foreground">{t('dayUnlocks')}</span>}
        {range === 'day' && day && (
          <>
            <select aria-label={t('day')} value={day} onChange={(e) => pickDay(e.target.value)}
              className="h-9 max-w-full min-w-0 rounded-full border border-input bg-card px-3 font-mono text-sm">
              {days.map((d) => <option key={d.day} value={d.day}>{d.day} · {d.task.title}</option>)}
            </select>
            <Link href={`/day/${day}`} className="text-sm font-semibold text-primary hover:underline">{t('openDay')}</Link>
          </>
        )}
      </div>

      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      {!rows && !error && <Skeleton className="h-64 rounded-[14px]" />}
      {rows && rows.length === 0 && (
        <p className="rounded-[14px] border border-dashed border-strong px-5 py-10 text-center text-sm text-muted-foreground">{t('noSubs')}</p>
      )}
      {rows && rows.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-10">#</TableHead>
              <TableHead>{t('stack')}</TableHead>
              <TableHead className="text-right">{t('avgPoints')}</TableHead>
              <TableHead className="text-right">{t('solveRate')}</TableHead>
              <TableHead className="text-right">{t('testsPassed')}</TableHead>
              <TableHead className="text-right">{t('attemptsToPass')}</TableHead>
              <TableHead className="text-right">{t('people')}</TableHead>
              <TableHead className="text-right">{t('days')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((r, i) => (
              <TableRow key={r.label}>
                <TableCell className="font-mono text-muted-foreground">{i + 1}</TableCell>
                <TableCell className="min-w-[10rem] font-semibold">{localizeStack(r.label, t.locale)}</TableCell>
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
