import Link from 'next/link'
import { Trophy } from 'lucide-react'
import { SLOT_COLORS } from '@/lib/tanks/playback'
import { ago } from '@/lib/format'
import type { MatchView } from '@/lib/types'
import { CopyReportButton } from './copy-report-button'

// A compact list of finished matches, reused on /tanks (recent activity)
// and on a bot's profile (its recent matches). One row per match — a table
// would either scroll sideways at 375px or drop the thing people actually
// want to click.
//
// With botId, each row also shows that bot's place and rating change in the match; with showReport (the bot's
// owner) each row gets a "Copy report" button.
export function MatchList({ matches, botId, showReport = false }: { matches: MatchView[]; botId?: string; showReport?: boolean }) {
  if (matches.length === 0) {
    return (
      <p className="rounded-[14px] border border-dashed border-input px-5 py-10 text-center text-sm text-muted-foreground">
        No matches yet.
      </p>
    )
  }
  return (
    <ul className="divide-y divide-border rounded-[14px] border border-border">
      {matches.map((m) => {
        const ranked = [...m.players].sort((a, b) => (a.place ?? 99) - (b.place ?? 99))
        const winner = ranked.find((p) => p.place === 1)
        const mine = botId ? m.players.find((p) => p.bot_id === botId) : undefined
        const delta = mine && mine.rating_before != null && mine.rating_after != null ? mine.rating_after - mine.rating_before : null
        const row = (
          <>
              <div className="flex min-w-0 flex-1 items-center gap-2">
                <span
                  className="size-2.5 shrink-0 rounded-full"
                  style={{ backgroundColor: winner ? SLOT_COLORS[winner.slot % SLOT_COLORS.length] : 'var(--muted-foreground)' }}
                />
                <span className="truncate text-sm font-semibold">{ranked.map((p) => p.name).join(' vs ')}</span>
              </div>
              {m.featured && <Trophy className="size-3.5 shrink-0 text-warning" aria-label="Featured" />}
              <span className="font-mono text-xs text-muted-foreground">{m.map}</span>
              {mine && (
                <span className="font-mono text-xs font-bold">{mine.place != null ? `#${mine.place}` : '—'}</span>
              )}
              {delta != null && (
                <span className={`font-mono text-xs ${delta > 0 ? 'text-success' : delta < 0 ? 'text-destructive' : 'text-muted-foreground'}`}>
                  {delta > 0 ? `+${delta}` : delta === 0 ? '±0' : delta}
                </span>
              )}
              <span className="text-xs text-muted-foreground">{m.finished_at ? ago(m.finished_at) : m.status}</span>
          </>
        )
        return (
          <li key={m.id}>
            {showReport ? (
              <div className="flex flex-wrap items-center gap-x-3 gap-y-1 p-4 hover:bg-muted/50">
                <Link href={`/tanks/matches/${m.id}`} className="flex min-w-0 flex-1 flex-wrap items-center gap-x-3 gap-y-1">{row}</Link>
                <CopyReportButton matchIds={[m.id]} size="xs" variant="ghost" label="Copy report" className="shrink-0" />
              </div>
            ) : (
              <Link href={`/tanks/matches/${m.id}`} className="flex flex-wrap items-center gap-x-3 gap-y-1 p-4 hover:bg-muted/50">{row}</Link>
            )}
          </li>
        )
      })}
    </ul>
  )
}
