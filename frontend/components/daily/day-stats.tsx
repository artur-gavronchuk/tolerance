import type { DayStats } from '@/lib/types'

// How the day went: who tried, who solved it, and which agents did best.
export function DayStatsPanel({ stats }: { stats: DayStats }) {
  if (stats.participants === 0) return null
  const rate = Math.round((100 * stats.solvers) / stats.participants)
  return (
    <div className="rounded-[14px] border border-border bg-card p-5">
      <dl className="grid grid-cols-3 gap-3 text-center">
        <Stat label="Tried" value={stats.participants} />
        <Stat label="Solved" value={stats.solvers} />
        <Stat label="Solve rate" value={`${rate}%`} />
      </dl>
      {stats.by_tool.length > 0 && (
        <>
          <h3 className="mt-5 text-sm font-semibold">By tool</h3>
          <ul className="mt-2 space-y-2">
            {stats.by_tool.map((t) => {
              const pct = Math.round((100 * t.solvers) / t.participants)
              return (
                <li key={t.made_with} className="text-sm">
                  <div className="flex items-baseline gap-2">
                    <span className="min-w-0 flex-1 truncate">{t.made_with || <span className="text-muted-foreground">not stated</span>}</span>
                    <span className="shrink-0 font-mono text-xs text-muted-foreground">{t.solvers}/{t.participants}</span>
                  </div>
                  <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-muted">
                    <div className="h-full rounded-full bg-primary" style={{ width: `${pct}%` }} />
                  </div>
                </li>
              )
            })}
          </ul>
        </>
      )}
    </div>
  )
}

function Stat({ label, value }: { label: string; value: number | string }) {
  return (
    <div>
      <dd className="font-mono text-2xl font-bold">{value}</dd>
      <dt className="text-xs text-muted-foreground">{label}</dt>
    </div>
  )
}
