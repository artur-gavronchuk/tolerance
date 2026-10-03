import { Badge } from '@/components/ui/badge'
import type { ProductTask } from '@/lib/types'

export const PHASE_LABEL = { open: 'Open', voting: 'Voting', final: 'Final' } as const

export function PhaseBadge({ phase }: { phase: ProductTask['phase'] }) {
  return <Badge variant={phase === 'open' ? 'default' : phase === 'voting' ? 'outline' : 'secondary'}>{PHASE_LABEL[phase]}</Badge>
}

// "in 2d 5h" / "in 3h 10m" / "in 12m" until an instant.
export function until(iso: string, now = Date.now()) {
  const m = Math.max(0, Math.round((new Date(iso).getTime() - now) / 60000))
  if (m >= 1440) return `in ${Math.floor(m / 1440)}d ${Math.floor((m % 1440) / 60)}h`
  if (m >= 60) return `in ${Math.floor(m / 60)}h ${m % 60}m`
  return `in ${Math.max(1, m)}m`
}

// How a task's standings are decided, in words; shown wherever people see a ranking.
export function rankRuleText(kind: ProductTask['kind'], hasChecks: boolean) {
  if (kind === 'site') {
    return hasChecks
      ? 'Ranked by votes; ties go to the higher automated-check score, then the earlier upload. Your latest upload counts.'
      : 'Ranked by votes; ties go to the earlier upload. Your latest upload counts.'
  }
  return 'Ranked by scenarios passed, then by votes; ties go to the earlier upload. Your best upload counts. A tool that fails scenarios does not win on popularity.'
}

// The one-line state of the voting window.
export function VotingNote({ task }: { task: ProductTask }) {
  if (task.phase === 'open') return null
  const ends = new Date(task.voting_ends_at).toLocaleString('en-GB', { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })
  return (
    <p className="rounded-[10px] border border-border bg-muted/50 px-4 py-2.5 text-sm text-muted-foreground">
      {task.phase === 'voting'
        ? <>Voting is open until <b className="text-foreground">{ends}</b> ({until(task.voting_ends_at)}). One vote per task, not for your own entry; you can move or take back your vote until then.</>
        : <>Voting ended {ends}. These results are final.</>}
    </p>
  )
}
