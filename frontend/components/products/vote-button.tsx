import Link from 'next/link'
import { ThumbsUp } from 'lucide-react'
import { useLoginHref } from '@/components/public/return-path'
import { Button } from '@/components/ui/button'
import type { ProductEntry, ProductTask } from '@/lib/types'
import { usePT } from './phase'

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
  const t = usePT()
  const loginTo = useLoginHref()
  const count = compact || hideCount ? null : <span className="font-mono">{e.votes}</span>
  const label = (s: string) => (compact ? undefined : s)
  const name = e.voted ? t('vote.takeBack') : t('vote.vote')
  if (phase !== 'voting' || e.mine) {
    return (
      <Button size="sm" variant="ghost" disabled aria-label={hideCount ? undefined : t('vote.votes', { n: e.votes })} title={e.mine ? t('vote.yourEntry') : phase === 'final' ? t('vote.over') : undefined}>
        <ThumbsUp />{count}{e.mine && !compact && <span className="font-normal text-muted-foreground">{t('vote.yours')}</span>}
      </Button>
    )
  }
  if (!signedIn) {
    return (
      <Button size="sm" variant="outline" nativeButton={false} render={<Link href={loginTo} />} title={t('vote.signIn')}>
        <ThumbsUp />{count}{label(t('vote.signIn'))}
      </Button>
    )
  }
  return (
    <Button size="sm" variant={e.voted ? 'default' : 'outline'} disabled={busy} aria-pressed={e.voted}
      aria-label={compact ? name : undefined} title={e.voted ? t('vote.clickBack') : t('vote.forEntry')} onClick={onToggle}>
      <ThumbsUp />{count}{label(e.voted ? t('vote.voted') : t('vote.vote'))}
    </Button>
  )
}
