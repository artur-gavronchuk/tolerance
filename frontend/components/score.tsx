import { cn } from "@/lib/utils"
import type { CriterionScore, Criterion } from "@/lib/data"

function scoreTone(value: number) {
  if (value >= 90) return "text-success"
  if (value >= 75) return "text-brand"
  if (value >= 60) return "text-amber-600"
  return "text-rose-600"
}

function barTone(value: number) {
  if (value >= 90) return "bg-success"
  if (value >= 75) return "bg-brand"
  if (value >= 60) return "bg-amber-500"
  return "bg-rose-500"
}

export function ScoreRing({ value, size = 96 }: { value: number; size?: number }) {
  const stroke = size >= 80 ? 7 : 5
  const r = (size - stroke) / 2
  const c = 2 * Math.PI * r
  const offset = c - (value / 100) * c
  return (
    <div className="relative inline-flex items-center justify-center" style={{ width: size, height: size }}>
      <svg width={size} height={size} className="-rotate-90">
        <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke="var(--muted)" strokeWidth={stroke} />
        <circle
          cx={size / 2}
          cy={size / 2}
          r={r}
          fill="none"
          stroke="currentColor"
          className={scoreTone(value)}
          strokeWidth={stroke}
          strokeDasharray={c}
          strokeDashoffset={offset}
          strokeLinecap="round"
        />
      </svg>
      <div className="absolute flex flex-col items-center">
        <span className={cn("font-semibold tabular-nums", size >= 80 ? "text-2xl" : "text-lg")}>{value}</span>
        {size >= 80 && <span className="text-[10px] uppercase tracking-wide text-muted-foreground">score</span>}
      </div>
    </div>
  )
}

export function ScoreBadge({ value }: { value: number }) {
  return (
    <span className={cn("tabular-nums text-sm font-semibold", scoreTone(value))}>{value}</span>
  )
}

export function CriteriaBreakdown({
  scores,
  criteria,
}: {
  scores: CriterionScore[]
  criteria?: Criterion[]
}) {
  const weightFor = (name: string) => criteria?.find((c) => c.name === name)?.weight
  return (
    <div className="space-y-4">
      {scores.map((s) => {
        const weight = weightFor(s.name)
        return (
          <div key={s.name}>
            <div className="mb-1.5 flex items-baseline justify-between gap-4">
              <span className="text-sm font-medium">{s.name}</span>
              <span className="flex items-baseline gap-2">
                {weight != null && (
                  <span className="text-xs text-muted-foreground">{weight}% weight</span>
                )}
                <span className={cn("tabular-nums text-sm font-semibold", scoreTone(s.score))}>{s.score}</span>
              </span>
            </div>
            <div className="h-1.5 w-full overflow-hidden rounded-full bg-muted">
              <div
                className={cn("h-full rounded-full transition-all", barTone(s.score))}
                style={{ width: `${s.score}%` }}
              />
            </div>
          </div>
        )
      })}
    </div>
  )
}
