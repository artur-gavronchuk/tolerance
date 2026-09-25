import { SLOT_COLORS, type TankState } from '@/lib/tanks/playback'
import type { ReplayPlayer } from '@/lib/tanks/replay'
import { cn } from '@/lib/utils'

export function Scoreboard({ players, tanks, killsBySlot }: {
  players: ReplayPlayer[]
  tanks: TankState[]
  killsBySlot: number[]
}) {
  const sorted = [...players].sort((a, b) => a.slot - b.slot)
  return (
    <div className="rounded-[12px] border border-border bg-card p-3">
      <p className="mb-2 text-xs font-bold text-muted-foreground">Tanks</p>
      <ul className="space-y-2.5">
        {sorted.map((p) => {
          const tank = tanks[p.slot]
          const color = SLOT_COLORS[p.slot % SLOT_COLORS.length]
          const hp = tank ? Math.max(0, Math.min(100, Math.round(tank.hp))) : 0
          const alive = tank?.alive ?? false
          return (
            <li key={p.slot} className="flex items-center gap-2.5">
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
              <span className="w-6 shrink-0 text-right font-mono text-xs tabular-mono text-muted-foreground">
                {killsBySlot[p.slot] ?? 0}
              </span>
            </li>
          )
        })}
      </ul>
    </div>
  )
}
