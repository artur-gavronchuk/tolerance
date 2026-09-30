import { Button } from '@/components/ui/button'
import { RatingPill } from '@/components/rating-pill'
import type { SkillView } from '@/lib/types'
import { BLOCKED_LABEL } from '@/lib/format'

const DAILY_LIMIT = 3

export function SkillCard({ skill, onStart, starting }: { skill: SkillView; onStart: (slug: string) => void; starting: boolean }) {
  const left = Math.max(0, DAILY_LIMIT - skill.runs_today)
  return (
    <section className="flex min-w-0 flex-col gap-3 rounded-[14px] border border-border bg-card p-5">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h2 className="heading text-base">{skill.title}</h2>
          <p className="mt-0.5 text-sm text-muted-foreground">{skill.description}</p>
        </div>
        <span className="shrink-0 font-mono text-xs text-muted-foreground">{skill.pool_size} tasks</span>
      </div>
      {skill.rating
        ? <RatingPill r={skill.rating} />
        : <p className="text-sm text-muted-foreground">Not proven yet. Three hidden tasks, about 30 minutes of agent time.</p>}
      <div className="mt-auto flex flex-wrap items-center justify-between gap-x-3 gap-y-2">
        <span className="text-xs text-muted-foreground">{left} left today</span>
        {skill.can_start ? (
          <Button size="sm" onClick={() => onStart(skill.slug)} disabled={starting}>
            {skill.rating ? 'Prove again' : `Prove ${skill.title}`}
          </Button>
        ) : (
          <span className="min-w-0 text-xs text-muted-foreground">{BLOCKED_LABEL[skill.blocked_reason] ?? skill.blocked_reason}</span>
        )}
      </div>
    </section>
  )
}
