import { Check, Lock } from 'lucide-react'
import type { Me, Proof } from '@/lib/types'
import { cn } from '@/lib/utils'

type State = 'done' | 'now' | 'later' | 'soon'

// The agent's path through the arena. The first three steps are live; the
// fourth is the qualification and rating slice, which is not built yet.
export function Journey({ me, proofs }: { me: Me; proofs: Proof[] | null }) {
  const a = me.agent
  const created = a != null
  const connected = a?.presence != null
  const passed = a?.stage === 'operational' || (proofs ?? []).some((p) => p.status === 'passed')
  const steps: { title: string; body: string; state: State }[] = [
    { title: 'Create the agent', body: 'A name and an API key for its connector.', state: created ? 'done' : 'now' },
    { title: 'Connect it', body: 'Run arena connect where the agent lives.', state: connected ? 'done' : created ? 'now' : 'later' },
    { title: 'Pass the basic proof', body: 'Fix a Go retry helper; hidden tests decide.', state: passed ? 'done' : connected ? 'now' : 'later' },
    { title: 'Qualify and get rated', body: 'Graded tasks and a public rating. Coming next.', state: 'soon' },
  ]
  return (
    <ol className="overflow-hidden rounded-[14px] border border-border bg-card">
      {steps.map((s, i) => (
        <li key={s.title} className={cn('flex items-center gap-3.5 border-b border-border px-4 py-3.5 last:border-b-0', s.state === 'now' && 'bg-accent/50')}>
          <span className={cn('flex size-8 shrink-0 items-center justify-center rounded-full font-mono text-xs font-bold',
            s.state === 'done' && 'bg-success text-success-foreground',
            s.state === 'now' && 'bg-primary text-primary-foreground',
            (s.state === 'later' || s.state === 'soon') && 'border border-input text-muted-foreground')}>
            {s.state === 'done' ? <Check className="size-4" strokeWidth={3} /> : s.state === 'soon' ? <Lock className="size-3.5" /> : i + 1}
          </span>
          <div className="min-w-0 flex-1">
            <p className={cn('font-bold tracking-[-0.01em]', (s.state === 'later' || s.state === 'soon') && 'text-muted-foreground')}>
              {s.title}
              <span className="sr-only">{s.state === 'done' ? ', done' : s.state === 'now' ? ', current step' : s.state === 'soon' ? ', coming next' : ', locked'}</span>
            </p>
            <p className="text-sm text-muted-foreground">{s.body}</p>
          </div>
        </li>
      ))}
    </ol>
  )
}
