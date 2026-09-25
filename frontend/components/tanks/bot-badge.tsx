import { Badge } from '@/components/ui/badge'

const LABEL: Record<string, string> = { agent: 'Agent', upload: 'Upload', house: 'House' }

// A bot's provenance mark — the one thing the ladder promises to show
// honestly, since there's no other way to tell a human-written bot from
// an agent-written one.
export function BotBadge({ source, house, className }: { source: string; house?: boolean; className?: string }) {
  const key = house ? 'house' : source
  return (
    <Badge variant={key === 'agent' ? 'default' : key === 'house' ? 'secondary' : 'outline'} className={className}>
      {LABEL[key] ?? source}
    </Badge>
  )
}
