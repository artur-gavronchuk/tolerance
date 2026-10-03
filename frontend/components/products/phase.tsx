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

const UTC_FMT: Intl.DateTimeFormatOptions = { timeZone: 'UTC', day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit', hour12: false }

// Every time on the product pages is shown in UTC, with the suffix, so nobody has to guess the zone.
export const utc = (iso: string) => `${new Date(iso).toLocaleString('en-GB', UTC_FMT)} UTC`
export const utcDay = (iso: string) => new Date(iso).toLocaleDateString('en-GB', { timeZone: 'UTC', day: 'numeric', month: 'short', year: 'numeric' })

// True while a site task's entries are judged blind: automated check counts and source stay hidden so they
// can't sway the votes.
export const isBlind = (task: Pick<ProductTask, 'kind' | 'phase'>) => task.kind === 'site' && task.phase === 'voting'

export const JUDGE_SENTENCE = 'Pick the better of two anonymous sites. Your picks rank the entries.'

// One line on what the ranking is, for page headers.
export function rankSummary(kind: ProductTask['kind']) {
  return kind === 'site' ? 'Ranked by blind picks between pairs of anonymous sites.' : 'Ranked by scenarios passed, then by votes.'
}

// How a task's standings are decided, in words (one rule, stated once); shown inside the disclosure.
export function rankRuleText(kind: ProductTask['kind'], hasChecks: boolean) {
  if (kind === 'site') {
    return 'Each site gets a score from everybody\'s blind picks (1000 is the average site). If scores are equal, the site with more favourite votes ranks higher, '
      + (hasChecks ? 'then the one with more automated checks passed, ' : '')
      + 'then the earlier upload. A favourite vote is one per task and never for your own entry. Your latest upload counts.'
  }
  return 'Tools are ranked by scenarios passed, then by favourite votes; if both are equal, the earlier upload ranks higher. A favourite vote is one per task and never for your own entry. Your best upload counts. A tool that fails scenarios does not win on popularity.'
}

// The collapsed method behind a ranking.
export function RankingHow({ kind, hasChecks }: { kind: ProductTask['kind']; hasChecks: boolean }) {
  return (
    <details className="max-w-2xl rounded-[10px] border border-border bg-card px-4 py-2.5 text-sm">
      <summary className="cursor-pointer font-semibold">How ranking works</summary>
      <p className="mt-2 text-muted-foreground">{rankRuleText(kind, hasChecks)}</p>
    </details>
  )
}

// The one-line state of the voting window. With `judge` it also says how site entries are judged (the compare
// panel says that itself on the task page).
export function VotingNote({ task, judge = false }: { task: ProductTask; judge?: boolean }) {
  if (task.phase === 'open') return null
  const ends = utc(task.voting_ends_at)
  return (
    <p className="rounded-[10px] border border-border bg-muted/50 px-4 py-2.5 text-sm text-muted-foreground">
      {task.phase === 'voting'
        ? <>Voting is open until <b className="text-foreground">{ends}</b> ({until(task.voting_ends_at)}).{task.kind === 'cli' ? ' Vote for the tool you like best (not your own).' : judge ? ` ${JUDGE_SENTENCE}` : ''}</>
        : <>Voting ended {ends}. These results are final.</>}
    </p>
  )
}
