import Link from 'next/link'
import { Trophy } from 'lucide-react'
import { SLOT_COLORS } from '@/lib/tanks/playback'
import { ago } from '@/lib/format'
import type { MatchView } from '@/lib/types'

// A compact list of finished matches, reused on /tanks (recent activity)
// and on a bot's profile (its recent matches). One row per match — a table
// would either scroll sideways at 375px or drop the thing people actually
// want to click.
export function MatchList({ matches }: { matches: MatchView[] }) {
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
        return (
          <li key={m.id}>
            <Link href={`/tanks/matches/${m.id}`} className="flex flex-wrap items-center gap-x-3 gap-y-1 p-4 hover:bg-muted/50">
              <div className="flex min-w-0 flex-1 items-center gap-2">
                <span
                  className="size-2.5 shrink-0 rounded-full"
                  style={{ backgroundColor: winner ? SLOT_COLORS[winner.slot % SLOT_COLORS.length] : 'var(--muted-foreground)' }}
                />
                <span className="truncate text-sm font-semibold">{ranked.map((p) => p.name).join(' vs ')}</span>
              </div>
              {m.featured && <Trophy className="size-3.5 shrink-0 text-warning" aria-label="Featured" />}
              <span className="font-mono text-xs text-muted-foreground">{m.map}</span>
              <span className="text-xs text-muted-foreground">{m.finished_at ? ago(m.finished_at) : m.status}</span>
            </Link>
          </li>
        )
      })}
    </ul>
  )
}
