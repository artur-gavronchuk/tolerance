import { Check, Loader2 } from 'lucide-react'
import { cn } from '@/lib/utils'

// An illustration of the basic proof running, drawn from the real task
// (go-fix-retry, 5 hidden tests). It plays once on load and holds on the verdict.
const ROWS = [
  { label: 'Queued', note: '14:02:07' },
  { label: 'Connector picked it up', note: '14:02:09' },
  { label: 'Agent working', note: 'took 3m 41s' },
  { label: 'Diff received', note: 'retry.go +9 −4' },
]
const HIDDEN_TESTS = 5
const STEP = 0.32

export function ProofTicket({ className, glow = false }: { className?: string; glow?: boolean }) {
  const testsAt = ROWS.length * STEP + 0.15
  const verdictAt = testsAt + HIDDEN_TESTS * 0.16 + 0.25
  return (
    <figure className={cn('relative', className)}>
      {glow && <span aria-hidden className="glow-primary absolute -inset-10 -z-10" />}
      <div className="relative overflow-hidden rounded-[18px] border border-border bg-card shadow-[0_24px_48px_-28px] shadow-ink/35">
        <div className="flex items-start justify-between gap-3 border-b border-border px-5 py-4">
          <div className="min-w-0">
            <p className="font-mono text-xs text-muted-foreground">go-fix-retry</p>
            <p className="mt-1 font-bold leading-snug tracking-[-0.015em]">Fix exponential backoff in a Go retry helper</p>
          </div>
          <span className="mt-0.5 shrink-0 rounded-full bg-muted px-2.5 py-1 text-xs font-bold text-muted-foreground">Basic proof</span>
        </div>
        <ol className="px-5 py-2">
          {ROWS.map((r, i) => (
            <li key={r.label} className="ticket-row flex items-center gap-3 border-b border-border/70 py-2.5 text-sm" style={{ animationDelay: `${i * STEP}s` }}>
              <span className="flex size-5 shrink-0 items-center justify-center rounded-full bg-success text-success-foreground">
                <Check className="size-3" strokeWidth={3.25} />
              </span>
              <span className="font-medium">{r.label}</span>
              <span className="ml-auto font-mono text-xs text-muted-foreground">{r.note}</span>
            </li>
          ))}
          <li className="ticket-row flex flex-wrap items-center gap-3 py-3 text-sm" style={{ animationDelay: `${testsAt}s` }}>
            <span className="relative flex size-5 shrink-0">
              {/* Spins while the squares fill, then settles into a check. */}
              <span className="ticket-hide absolute inset-0 flex items-center justify-center rounded-full border border-primary text-primary" style={{ animationDelay: `${verdictAt}s` }}>
                <Loader2 className="size-3 animate-spin" />
              </span>
              <span className="ticket-row absolute inset-0 flex items-center justify-center rounded-full bg-success text-success-foreground" style={{ animationDelay: `${verdictAt}s` }}>
                <Check className="size-3" strokeWidth={3.25} />
              </span>
            </span>
            <span className="font-medium">Hidden tests</span>
            <span className="ml-auto flex gap-1" aria-label={`${HIDDEN_TESTS} of ${HIDDEN_TESTS} hidden tests passed`}>
              {Array.from({ length: HIDDEN_TESTS }, (_, i) => (
                <span key={i} className="ticket-row size-4 rounded-[4px] bg-success" style={{ animationDelay: `${testsAt + 0.2 + i * 0.16}s` }} />
              ))}
            </span>
          </li>
        </ol>
        <div className="ticket-row flex items-center justify-between gap-3 bg-success px-5 py-4 text-success-foreground" style={{ animationDelay: `${verdictAt}s` }}>
          <div>
            <p className="text-xs font-bold opacity-80">Verdict</p>
            <p className="text-2xl font-extrabold tracking-[-0.035em]">Verified</p>
          </div>
          <p className="max-w-[12rem] text-right text-xs leading-snug font-semibold opacity-90">Every hidden test ran and passed on the agent&apos;s diff.</p>
        </div>
      </div>
      <figcaption className="mt-3 text-xs text-muted-foreground">How the basic proof looks when it passes.</figcaption>
    </figure>
  )
}
