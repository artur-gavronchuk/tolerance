import Link from 'next/link'
import { BotBadge } from './bot-badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import type { LeaderboardEntry } from '@/lib/types'

function winRate(e: LeaderboardEntry): string {
  return e.matches > 0 ? `${Math.round((e.wins / e.matches) * 100)}%` : '—'
}

// The ranking table, shared by /tanks (top 10) and /tanks/leaderboard (the
// full ladder). Renders as a real table from sm up; a stacked row list
// below it, so it stays readable at 375px instead of scrolling sideways.
export function Leaderboard({ entries }: { entries: LeaderboardEntry[] }) {
  if (entries.length === 0) {
    return (
      <p className="rounded-[14px] border border-dashed border-input px-5 py-10 text-center text-sm text-muted-foreground">
        No ranked bots yet.
      </p>
    )
  }
  return (
    <>
      <Table className="hidden sm:table">
        <TableHeader>
          <TableRow>
            <TableHead className="w-10">#</TableHead>
            <TableHead>Bot</TableHead>
            <TableHead className="text-right">Rating</TableHead>
            <TableHead className="text-right">Matches</TableHead>
            <TableHead className="text-right">Wins</TableHead>
            <TableHead className="text-right">Win rate</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {entries.map((e) => (
            <TableRow key={e.bot_id}>
              <TableCell className="font-mono text-muted-foreground">{e.rank}</TableCell>
              <TableCell>
                <Link href={`/tanks/bots/${e.bot_id}`} className="inline-flex items-center gap-2 font-semibold hover:text-primary">
                  {e.name}
                  <BotBadge source={e.source} house={e.house} />
                </Link>
              </TableCell>
              <TableCell className="text-right font-mono font-bold">{e.rating}</TableCell>
              <TableCell className="text-right font-mono text-muted-foreground">{e.matches}</TableCell>
              <TableCell className="text-right font-mono text-muted-foreground">{e.wins}</TableCell>
              <TableCell className="text-right font-mono text-muted-foreground">{winRate(e)}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <ul className="divide-y divide-border rounded-[14px] border border-border sm:hidden">
        {entries.map((e) => (
          <li key={e.bot_id} className="flex items-center gap-3 p-3">
            <span className="w-5 shrink-0 font-mono text-sm text-muted-foreground">{e.rank}</span>
            <div className="min-w-0 flex-1">
              <Link href={`/tanks/bots/${e.bot_id}`} className="flex items-center gap-2 font-semibold">
                <span className="truncate">{e.name}</span>
                <BotBadge source={e.source} house={e.house} />
              </Link>
              <p className="mt-0.5 text-xs text-muted-foreground">
                {e.matches} matches · {e.wins} wins · {winRate(e)}
              </p>
            </div>
            <span className="shrink-0 font-mono text-lg font-bold">{e.rating}</span>
          </li>
        ))}
      </ul>
    </>
  )
}
