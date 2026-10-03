'use client'

import { Badge } from '@/components/ui/badge'
import { useT } from '@/lib/i18n/client'
import { tanksHomeMessages as m } from '@/lib/i18n/messages/tanks-home'

// A bot's provenance mark — the one thing the ladder promises to show
// honestly, since there's no other way to tell a human-written bot from
// an agent-written one.
export function BotBadge({ source, house, className }: { source: string; house?: boolean; className?: string }) {
  const t = useT(m)
  const key = house ? 'house' : source
  const label = key === 'agent' ? t('badge.agent') : key === 'upload' ? t('badge.upload') : key === 'house' ? t('badge.house') : source
  return (
    <Badge variant={key === 'agent' ? 'default' : key === 'house' ? 'secondary' : 'outline'} className={className}>
      {label}
    </Badge>
  )
}
