'use client'

import { useEffect, useState } from 'react'
import { PageHeader } from '@/components/page-header'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { api } from '@/lib/api'
import { useMe } from '@/lib/use-me'
import type { OverallPage, OverallRow } from '@/lib/types'
import { HandleLink } from '@/components/daily/handle-link'
import { ReportButton } from '@/components/fairplay/report-button'
import { errorText } from '@/lib/format'
import { useT } from '@/lib/i18n/client'
import { listingMessages } from '@/lib/i18n/messages/listings'

const PAGE = 100

// Equal points, days solved and streak share a place; "2=" marks it.
const placeOf = (r: OverallRow) => `${r.place}${r.tied ? '=' : ''}`

export default function LeaderboardPage() {
  const t = useT(listingMessages)
  const { me, loading: meLoading } = useMe()
  const [rows, setRows] = useState<OverallRow[] | null>(null)
  const [total, setTotal] = useState(0)
  const [you, setYou] = useState<OverallRow | null>(null)
  const [loadingMore, setLoadingMore] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const fetchPage = (offset: number) => api<OverallPage>(`/leaderboard?offset=${offset}&limit=${PAGE}`)
  // The session decides who "you" is, so the first page waits for the session lookup.
  useEffect(() => {
    if (meLoading) return
    fetchPage(0)
      .then((r) => { setRows(r.items); setTotal(r.total); setYou(r.you) })
      .catch((e) => setError(errorText(e, t.locale)))
  }, [meLoading])
  const more = () => {
    if (!rows) return
    setLoadingMore(true)
    fetchPage(rows.length)
      .then((r) => { setRows([...rows, ...r.items]); setTotal(r.total); setYou(r.you) })
      .catch((e) => setError(errorText(e, t.locale)))
      .finally(() => setLoadingMore(false))
  }
  const mine = (r: OverallRow) => r.handle === me?.user.handle
  const line = (r: OverallRow, className?: string) => (
    <TableRow key={r.handle} className={className ?? (mine(r) ? 'bg-accent' : undefined)}>
      <TableCell className="font-mono text-muted-foreground">{placeOf(r)}</TableCell>
      <TableCell className="max-w-[10rem] truncate font-semibold sm:max-w-none">{className && <span className="mr-1.5 text-xs font-bold uppercase text-primary">{t('lbYou')}</span>}<HandleLink handle={r.handle} /> <ReportButton handle={r.handle} /></TableCell>
      <TableCell className="text-right font-mono font-bold">{r.points}</TableCell>
      <TableCell className="text-right font-mono text-muted-foreground">{r.solved_days}</TableCell>
      <TableCell className="text-right font-mono text-muted-foreground">🔥 {r.current_streak}</TableCell>
    </TableRow>
  )
  return (
    <div className="space-y-8">
      <PageHeader title={t('lbTitle')}>{t('lbIntro')}</PageHeader>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      {!rows && !error && <Skeleton className="h-64 rounded-[14px]" />}
      {rows && rows.length === 0 && (
        <p className="rounded-[14px] border border-dashed border-strong px-5 py-10 text-center text-sm text-muted-foreground">{t('lbEmpty')}</p>
      )}
      {rows && rows.length > 0 && (
        <div>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-10">#</TableHead>
                <TableHead>{t('player')}</TableHead>
                <TableHead className="text-right">{t('points')}</TableHead>
                <TableHead className="text-right">{t('solved')}</TableHead>
                <TableHead className="text-right">{t('streak')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((r) => line(r))}
              {you && (
                <>
                  <TableRow aria-hidden><TableCell colSpan={5} className="py-1 text-center text-muted-foreground">⋯</TableCell></TableRow>
                  {line(you, 'bg-accent')}
                </>
              )}
            </TableBody>
          </Table>
          <div className="mt-3 flex flex-wrap items-center justify-center gap-3">
            <span className="text-sm text-muted-foreground">{t('lbTop', { shown: rows.length, total })}</span>
            {total > rows.length && (
              <Button variant="outline" size="sm" onClick={more} disabled={loadingMore}>{t('lbShowMore')}</Button>
            )}
          </div>
          <p className="mt-2 text-center text-xs text-muted-foreground">{t('lbTieHint')}</p>
        </div>
      )}
    </div>
  )
}
