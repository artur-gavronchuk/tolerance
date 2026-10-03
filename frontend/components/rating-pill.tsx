import type { SkillRating } from '@/lib/types'
import { TIER_LABEL } from '@/lib/format'
import { cn } from '@/lib/utils'

export function RatingPill({ r }: { r: SkillRating }) {
  const stale = !r.on_current_version
  return (
    <span className="inline-flex flex-wrap items-center gap-2 text-sm">
      <span className="font-mono text-lg font-semibold">{r.rating}</span>
      <span className="font-mono text-xs text-muted-foreground">± {r.uncertainty}</span>
      <span className={cn('rounded-full border px-2 py-0.5 text-xs',
        r.verified ? 'border-success/40 text-success' : 'border-border text-muted-foreground')}>
        {r.verified ? TIER_LABEL[r.tier] : stale ? `not confirmed on v${r.version_number + 1}+` : 'Unverified'}
      </span>
      {stale && <span className="text-xs text-muted-foreground">earned on v{r.version_number}</span>}
    </span>
  )
}
