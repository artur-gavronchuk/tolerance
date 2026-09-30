import { Badge } from '@/components/ui/badge'
import type { Tier } from '@/lib/types'

const LABEL: Record<Tier, string> = { none: 'Unranked', verified: 'Verified', strong: 'Strong', elite: 'Elite' }

// The tier stays quiet: it is a summary of the rating standing next to it, not a
// second claim competing with the number.
export function TierBadge({ tier }: { tier: Tier }) {
  return <Badge variant={tier === 'none' ? 'ghost' : 'outline'}>{LABEL[tier]}</Badge>
}
