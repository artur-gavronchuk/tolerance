'use client'

import { useEffect, useState } from 'react'
import { PageHeader } from '@/components/page-header'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { api, friendlyMessage } from '@/lib/api'
import { useMe } from '@/lib/use-me'
import type { OverallRow } from '@/lib/types'

export default function LeaderboardPage() {
  const { me } = useMe()
  const [rows, setRows] = useState<OverallRow[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  useEffect(() => {
    api<{ items: OverallRow[] }>('/leaderboard').then((r) => setRows(r.items)).catch((e) => setError(friendlyMessage(e)))
  }, [])
  const mine = (r: OverallRow) => r.handle === me?.user.handle
  return (
    <div className="space-y-8">
      <PageHeader title="Leaderboard">Ranked by days fully solved, then by current streak.</PageHeader>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      {!rows && !error && <Skeleton className="h-64 rounded-[14px]" />}
      {rows && rows.length === 0 && (
        <p className="rounded-[14px] border border-dashed border-input px-5 py-10 text-center text-sm text-muted-foreground">Nobody has solved a day yet.</p>
      )}
      {rows && rows.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-10">#</TableHead>
              <TableHead>Player</TableHead>
              <TableHead className="text-right">Solved days</TableHead>
              <TableHead className="text-right">Streak</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((r) => (
              <TableRow key={r.place} className={mine(r) ? 'bg-accent' : undefined}>
                <TableCell className="font-mono text-muted-foreground">{r.place}</TableCell>
                <TableCell className="max-w-[10rem] truncate font-semibold sm:max-w-none">{r.handle}</TableCell>
                <TableCell className="text-right font-mono font-bold">{r.solved_days}</TableCell>
                <TableCell className="text-right font-mono text-muted-foreground">🔥 {r.current_streak}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  )
}
