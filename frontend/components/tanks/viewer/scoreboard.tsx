import { SLOT_COLORS, type TankState } from '@/lib/tanks/playback'
import type { ReplayPlayer } from '@/lib/tanks/replay'
import { cn } from '@/lib/utils'

// selectedSlot/onSelect are optional and only meaningful to the 3D viewer's
// Follow camera (task 15): clicking a row picks which tank it chases.
// Passed or not, the 2D view renders and behaves exactly as before.
export function Scoreboard({ players, tanks, killsBySlot, selectedSlot, onSelect }: {
  players: ReplayPlayer[]
  tanks: TankState[]
  killsBySlot: number[]
  selectedSlot?: number | null
  onSelect?: (slot: number | null) => void
}) {
  const sorted = [...players].sort((a, b) => a.slot - b.slot)
  return (
    <div className="rounded-[12px] border border-border bg-card p-3">
      <p className="mb-2 flex items-baseline justify-between gap-2 text-xs font-bold text-muted-foreground">
        <span>Tanks</span>
        <span className="font-normal">health bar · kills</span>
      </p>
      <ul className="space-y-2.5">
        {sorted.map((p) => {
          const tank = tanks[p.slot]
          const color = SLOT_COLORS[p.slot % SLOT_COLORS.length]
          const hp = tank ? Math.max(0, Math.min(100, Math.round(tank.hp))) : 0
          const alive = tank?.alive ?? false
          const selected = selectedSlot === p.slot
          const row = (
            <>
              <span className="size-2.5 shrink-0 rounded-full" style={{ backgroundColor: color }} />
              <span className={cn('min-w-0 flex-1 truncate text-sm font-semibold', !alive && 'text-muted-foreground line-through')}>
                {p.name}
              </span>
              <span className="block h-1.5 w-14 shrink-0 overflow-hidden rounded-full bg-muted">
                <span
                  className="block h-full rounded-full transition-[width] duration-150"
                  style={{ width: `${hp}%`, backgroundColor: alive ? color : 'var(--muted-foreground)' }}
                />
              </span>
              <span title="Kills" className="w-6 shrink-0 text-right font-mono text-xs tabular-mono text-muted-foreground">
                {killsBySlot[p.slot] ?? 0}
              </span>
            </>
          )
          if (!onSelect) {
            return (
              <li key={p.slot} className="flex items-center gap-2.5">
                {row}
              </li>
            )
          }
          return (
            <li key={p.slot}>
              <button
                type="button"
                onClick={() => onSelect(selected ? null : p.slot)}
                aria-pressed={selected}
                className={cn(
                  'flex w-full items-center gap-2.5 rounded-md px-1.5 py-0.5 -mx-1.5 text-left transition-colors',
                  selected ? 'bg-accent' : 'hover:bg-muted'
                )}
              >
                {row}
              </button>
            </li>
          )
        })}
      </ul>
      <p className="mt-2.5 text-[0.7rem] text-muted-foreground">Number on the right: kills (tanks destroyed). Struck-through name: tank destroyed.</p>
    </div>
  )
}
