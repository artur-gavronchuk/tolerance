import type { Stage } from '@/lib/types'
import { cn } from '@/lib/utils'

// Presence at a glance: green when the connector is online, pulsing blue
// while a proof runs, hollow when nobody is connected.
export function StatusDot({ stage, className }: { stage: Stage; className?: string }) {
  const offline = stage === 'registered' || stage === 'offline'
  const checking = stage === 'checking'
  return (
    <span className={cn('relative inline-flex size-2.5 shrink-0', className)}>
      {checking && <span className="absolute inset-0 rounded-full bg-primary opacity-60 animate-status-pulse" />}
      <span className={cn('relative inline-flex size-2.5 rounded-full',
        offline ? 'border-2 border-muted-foreground/50' : checking ? 'bg-primary' : 'bg-success')} />
    </span>
  )
}

export const PRESENCE_LABEL: Record<Stage, string> = {
  registered: 'Never connected',
  offline: 'Offline',
  connected: 'Online',
  checking: 'Proof running',
  operational: 'Online',
  check_failed: 'Online',
}
