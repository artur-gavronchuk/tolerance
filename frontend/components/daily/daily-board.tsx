import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import type { DailyRow } from '@/lib/types'
import { HandleLink } from '@/components/daily/handle-link'

export function DailyBoard({ rows, me, optimize }: { rows: DailyRow[]; me?: string; optimize?: boolean }) {
  if (rows.length === 0) {
    return (
      <p className="rounded-[14px] border border-dashed border-input px-5 py-10 text-center text-sm text-muted-foreground">
        No results yet. Be the first.
      </p>
    )
  }
  const mine = (r: DailyRow) => r.handle === me
  const result = (r: DailyRow) => (optimize ? fmtScore(r.score) : `${r.passed_tests}/${r.total_tests}`)
  return (
    <>
      <Table className="hidden sm:table">
        <TableHeader>
          <TableRow>
            <TableHead className="w-10">#</TableHead>
            <TableHead>Player</TableHead>
            <TableHead>Made with</TableHead>
            <TableHead className="text-right">{optimize ? 'Score' : 'Tests'}</TableHead>
            <TableHead className="text-right">Time (UTC)</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((r) => (
            <TableRow key={r.place} className={mine(r) ? 'bg-accent' : undefined}>
              <TableCell className="font-mono text-muted-foreground">{r.place}</TableCell>
              <TableCell className="font-semibold"><HandleLink handle={r.handle} /></TableCell>
              <TableCell className="text-muted-foreground">{r.made_with || '—'}</TableCell>
              <TableCell className="text-right font-mono font-bold">{result(r)}</TableCell>
              <TableCell className="text-right font-mono text-muted-foreground">{time(r.submitted_at)}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <ul className="divide-y divide-border rounded-[14px] border border-border sm:hidden">
        {rows.map((r) => (
          <li key={r.place} className={`flex items-center gap-3 p-3 ${mine(r) ? 'bg-accent' : ''}`}>
            <span className="w-5 shrink-0 font-mono text-sm text-muted-foreground">{r.place}</span>
            <div className="min-w-0 flex-1">
              <p className="truncate font-semibold"><HandleLink handle={r.handle} /></p>
              <p className="truncate text-xs text-muted-foreground">{r.made_with || '—'} · {time(r.submitted_at)}</p>
            </div>
            <span className="shrink-0 font-mono text-lg font-bold">{result(r)}</span>
          </li>
        ))}
      </ul>
    </>
  )
}

export function fmtScore(v: number | null | undefined) {
  return v == null ? '—' : v.toLocaleString('en-US', { maximumFractionDigits: 2 })
}

function time(iso: string) {
  return new Date(iso).toISOString().slice(11, 16)
}
