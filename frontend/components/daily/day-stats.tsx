import { localizeStack, stackLabel } from '@/components/daily/stack-label'
import { useT } from '@/lib/i18n/client'
import { dailyMessages } from '@/lib/i18n/messages/daily'
import type { DayStats } from '@/lib/types'

// by_tool is keyed by the raw free text; fold it into normalized labels so one tool is one row.
function groupByStack(by: DayStats['by_tool'], notStated: string) {
  const groups = new Map<string, { label: string; raw: string[]; participants: number; solvers: number }>()
  for (const t of by) {
    const label = t.made_with.trim() ? stackLabel(t.made_with) : notStated
    const g = groups.get(label) ?? { label, raw: [], participants: 0, solvers: 0 }
    g.participants += t.participants
    g.solvers += t.solvers
    if (t.made_with.trim() && !g.raw.includes(t.made_with.trim())) g.raw.push(t.made_with.trim())
    groups.set(label, g)
  }
  return [...groups.values()].sort((a, b) => b.participants - a.participants || b.solvers - a.solvers)
}

// How the day went: who tried, who solved it, and which agents did best.
export function DayStatsPanel({ stats }: { stats: DayStats }) {
  const t = useT(dailyMessages)
  if (stats.participants === 0) return null
  const rate = Math.round((100 * stats.solvers) / stats.participants)
  return (
    <div className="rounded-[14px] border border-border bg-card p-5">
      <dl className="grid grid-cols-3 gap-3 text-center">
        <Stat label={t('tried')} value={stats.participants} />
        <Stat label={t('solved')} value={stats.solvers} />
        <Stat label={t('solveRate')} value={`${rate}%`} />
      </dl>
      {stats.by_tool.length > 0 && (
        <>
          <h3 className="mt-5 text-sm font-semibold">{t('byTool')}</h3>
          <ul className="mt-2 space-y-2">
            {groupByStack(stats.by_tool, t('notStated')).map((g) => {
              const pct = Math.round((100 * g.solvers) / g.participants)
              return (
                <li key={g.label} className="text-sm">
                  <div className="flex items-baseline gap-2">
                    <span className={`min-w-0 flex-1 truncate ${g.label === t('notStated') ? 'text-muted-foreground' : ''}`} title={g.raw.length > 0 ? g.raw.join(', ') : undefined}>{localizeStack(g.label, t.locale)}</span>
                    <span className="shrink-0 font-mono text-xs text-muted-foreground">{g.solvers}/{g.participants}</span>
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
