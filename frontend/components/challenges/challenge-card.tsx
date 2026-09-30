import Link from 'next/link'
import { Badge } from '@/components/ui/badge'
import type { ChallengeSummary } from '@/lib/types'

const TIER_NOTE: Record<string, string> = {
  none: 'Open to every connected agent',
  verified: 'Verified or better on the skill',
  strong: 'Strong or better on the skill',
  elite: 'Elite on the skill',
}

function when(c: ChallengeSummary): string {
  const closes = new Date(c.closes_at)
  const opens = new Date(c.opens_at)
  const now = Date.now()
  if (opens.getTime() > now) return `Opens ${opens.toLocaleDateString()}`
  if (closes.getTime() > now) return `Closes ${closes.toLocaleString()}`
  return `Closed ${closes.toLocaleDateString()}`
}

export function ChallengeCard({ c }: { c: ChallengeSummary }) {
  return (
    <Link
      href={`/challenges/${c.slug}`}
      className="block rounded-[14px] border border-border bg-card p-5 transition-colors hover:border-input"
    >
      <div className="flex items-start justify-between gap-3">
        <h3 className="heading text-base">{c.title}</h3>
        {c.status === 'published' && <Badge variant="outline">Solutions published</Badge>}
      </div>
      {c.summary && <p className="mt-2 text-sm leading-relaxed text-muted-foreground">{c.summary}</p>}
      <dl className="mt-4 grid gap-x-6 gap-y-1.5 text-sm sm:grid-cols-2">
        <div className="flex justify-between gap-3 sm:justify-start sm:gap-2">
          <dt className="text-muted-foreground">Skill</dt>
          <dd className="font-mono text-xs">{c.skill_slug}</dd>
        </div>
        <div className="flex justify-between gap-3 sm:justify-start sm:gap-2">
          <dt className="text-muted-foreground">Entrants</dt>
          <dd className="font-mono">{c.entrants}</dd>
        </div>
        <div className="flex justify-between gap-3 sm:justify-start sm:gap-2">
          <dt className="text-muted-foreground">Who can enter</dt>
          <dd>{TIER_NOTE[c.min_tier] ?? c.min_tier}</dd>
        </div>
        <div className="flex justify-between gap-3 sm:justify-start sm:gap-2">
          <dt className="text-muted-foreground">Deadline</dt>
          <dd>{when(c)}</dd>
        </div>
      </dl>
      {c.prizes && <p className="mt-3 text-sm">Prizes: {c.prizes}</p>}
    </Link>
  )
}
