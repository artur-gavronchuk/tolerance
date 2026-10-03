import Link from 'next/link'
import { ThumbsUp } from 'lucide-react'
import { useLoginHref } from '@/components/public/return-path'
import { Button } from '@/components/ui/button'
import type { ProductEntry, ProductTask } from '@/lib/types'

// The vote control of one entry: the count always shows; what clicking does depends on who you are and
// whether voting is still open.
export function VoteButton({ entry: e, phase, signedIn, busy, onToggle, compact = false, hideCount = false }: {
  entry: ProductEntry
  phase: ProductTask['phase']
  signedIn: boolean
  busy: boolean
  onToggle: () => void
  compact?: boolean
  hideCount?: boolean
}) {
  const loginTo = useLoginHref()
  const count = compact || hideCount ? null : <span className="font-mono">{e.votes}</span>
  const label = (t: string) => (compact ? undefined : t)
  const name = e.voted ? 'Take back your vote' : 'Vote'
  if (phase !== 'voting' || e.mine) {
    return (
      <Button size="sm" variant="ghost" disabled aria-label={hideCount ? undefined : `${e.votes} votes`} title={e.mine ? 'Your entry' : phase === 'final' ? 'Voting is over' : undefined}>
        <ThumbsUp />{count}{e.mine && !compact && <span className="font-normal text-muted-foreground">yours</span>}
      </Button>
    )
  }
  if (!signedIn) {
    return (
      <Button size="sm" variant="outline" nativeButton={false} render={<Link href={loginTo} />} title="Sign in to vote">
        <ThumbsUp />{count}{label('Sign in to vote')}
      </Button>
    )
  }
  return (
    <Button size="sm" variant={e.voted ? 'default' : 'outline'} disabled={busy} aria-pressed={e.voted}
      aria-label={compact ? name : undefined} title={e.voted ? 'Click to take your vote back' : 'Vote for this entry'} onClick={onToggle}>
      <ThumbsUp />{count}{label(e.voted ? 'Voted' : 'Vote')}
    </Button>
  )
}
