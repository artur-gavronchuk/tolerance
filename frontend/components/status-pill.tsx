import { Check, Loader2, TriangleAlert, X } from 'lucide-react'
import { SHORT_STATUS, tone, type Tone } from '@/lib/format'
import { cn } from '@/lib/utils'

const TONE: Record<Tone, string> = {
  pass: 'bg-success/12 text-success',
  fail: 'bg-destructive/10 text-destructive',
  error: 'bg-warning/12 text-warning',
  live: 'bg-accent text-accent-foreground',
}

export function StatusPill({ status, className }: { status: string; className?: string }) {
  const t = tone(status)
  const Icon = t === 'pass' ? Check : t === 'fail' ? X : t === 'error' ? TriangleAlert : Loader2
  return (
    <span className={cn('inline-flex h-6 items-center gap-1 rounded-full px-2.5 text-xs font-bold whitespace-nowrap', TONE[t], className)}>
      <Icon className={cn('size-3.5', t === 'live' && 'animate-spin')} strokeWidth={2.75} />
      {SHORT_STATUS[status] ?? status}
    </span>
  )
}
