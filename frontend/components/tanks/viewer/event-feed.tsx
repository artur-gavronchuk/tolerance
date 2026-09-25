import { SLOT_COLORS } from '@/lib/tanks/playback'
import type { ReplayEvent, ReplayPlayer } from '@/lib/tanks/replay'

function describe(e: ReplayEvent, players: ReplayPlayer[]): string | null {
  const name = (slot: number) => players.find((p) => p.slot === slot)?.name ?? `tank ${slot}`
  if (e.e === 'hit' && e.b != null) return `${name(e.a)} hit ${name(e.b)} −${e.d ?? 0}`
  if (e.e === 'kill' && e.b != null) return e.a === -1 ? `The zone destroyed ${name(e.b)}` : `${name(e.a)} destroyed ${name(e.b)}`
  if (e.e === 'heal') return `${name(e.a)} picked up a repair kit${e.d != null ? ` (+${e.d})` : ''}`
  return null
}

// The last few notable events, newest first — the feel of a sports
// ticker. "shot" is deliberately excluded: it fires on every reload and
// would drown out the events people actually want to read.
export function EventFeed({ events, players }: { events: ReplayEvent[]; players: ReplayPlayer[] }) {
  const rows = events
    .map((e, i) => ({ e, text: describe(e, players), key: `${e.t}-${e.e}-${e.a}-${e.b ?? ''}-${i}` }))
    .filter((r): r is { e: ReplayEvent; text: string; key: string } => r.text != null)
  return (
    <div className="rounded-[12px] border border-border bg-card p-3">
      <p className="mb-2 text-xs font-bold text-muted-foreground">Recent events</p>
      {rows.length === 0 ? (
        <p className="text-sm text-muted-foreground">Nothing yet.</p>
      ) : (
        <ul className="space-y-1.5">
          {rows.slice(0, 8).map((r) => (
            <li key={r.key} className="flex items-start gap-2 text-sm leading-snug">
              <span
                className="mt-1.5 size-1.5 shrink-0 rounded-full"
                style={{ backgroundColor: r.e.a >= 0 ? SLOT_COLORS[r.e.a % SLOT_COLORS.length] : 'var(--muted-foreground)' }}
              />
              <span className="text-foreground/90">{r.text}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
