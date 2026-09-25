import { Check, X } from 'lucide-react'
import type { Proof } from '@/lib/types'
import { REASON_LABEL, duration, tone } from '@/lib/format'
import { cn } from '@/lib/utils'

const BAND = {
  pass: 'bg-success text-success-foreground dark:bg-success/15 dark:text-success dark:ring-1 dark:ring-success/30',
  fail: 'bg-destructive text-white dark:bg-destructive/15 dark:text-destructive dark:ring-1 dark:ring-destructive/30',
  error: 'bg-warning text-warning-foreground dark:bg-warning/15 dark:text-warning dark:ring-1 dark:ring-warning/30',
  live: 'bg-accent text-accent-foreground',
}

function headline(p: Proof) {
  if (p.kind === 'game_bot') {
    if (p.status === 'passed') return "Your agent's bot is in the arena"
    if (p.status === 'failed') return 'The bot did not pass the check'
  }
  switch (p.status) {
    case 'passed': return 'Verified'
    case 'failed': return 'Not verified'
    case 'expired': return 'Expired'
    case 'infra_error': return 'Platform error'
    default: return ''
  }
}

function explain(p: Proof) {
  if (p.status === 'passed') {
    return p.kind === 'game_bot'
      ? 'Every check passed. This version is now qualified and live on the ladder.'
      : 'Every hidden test ran and passed on the diff your agent produced. It works on its own.'
  }
  if (p.status === 'infra_error') return 'This one is on us, not on your agent. It does not count against it, and a retry is free.'
  if (p.failure_reason) return REASON_LABEL[p.failure_reason] ?? p.failure_reason
  return p.status === 'expired' ? 'Nobody finished this proof in time.' : ''
}

// The verdict is the loudest thing on the page: a full-colour band with the
// numbers that decided it.
export function VerdictBand({ proof }: { proof: Proof }) {
  const r = proof.sandbox_result
  const t = tone(proof.status)
  const stats = [
    { label: 'Tests passed', value: r && r.tests.length ? `${r.tests.filter((x) => x.passed).length}/${r.tests.length}` : '—' },
    { label: 'Agent time', value: duration(proof.agent_duration_ms) },
    { label: 'Exit code', value: proof.agent_exit_code ?? '—' },
    { label: 'Diff', value: proof.diff ? `${(proof.diff.length / 1024).toFixed(1)} KB` : '—' },
  ]
  return (
    <section className={cn('overflow-hidden rounded-[18px]', BAND[t])}>
      <div className="grid gap-6 p-6 sm:p-8 lg:grid-cols-[1fr_auto] lg:items-end">
        <div>
          <p className="text-sm font-bold opacity-80">Verdict</p>
          <p className="display mt-1 text-[2.8rem] sm:text-[3.6rem]">{headline(proof)}</p>
          <p className="mt-2 max-w-xl text-[0.95rem] leading-relaxed font-medium opacity-90">{explain(proof)}</p>
        </div>
        <dl className="grid grid-cols-2 gap-x-8 gap-y-4 sm:grid-cols-4">
          {stats.map((s) => (
            <div key={s.label}>
              <dt className="text-xs font-bold opacity-75">{s.label}</dt>
              <dd className="mt-0.5 font-mono text-lg font-semibold">{s.value}</dd>
            </div>
          ))}
        </dl>
      </div>
    </section>
  )
}

export function TestResults({ proof }: { proof: Proof }) {
  const r = proof.sandbox_result
  if (!r || r.tests.length === 0) return null
  return (
    <ul className="overflow-hidden rounded-[14px] border border-border bg-card">
      {r.tests.map((t) => (
        <li key={t.name} className="flex items-center gap-3 border-b border-border px-4 py-2.5 last:border-b-0">
          <span className={cn('flex size-5 shrink-0 items-center justify-center rounded-full', t.passed ? 'bg-success/12 text-success' : 'bg-destructive/10 text-destructive')}>
            {t.passed ? <Check className="size-3" strokeWidth={3} /> : <X className="size-3" strokeWidth={3} />}
          </span>
          <span className="min-w-0 flex-1 truncate font-mono text-xs">{t.name}</span>
          <span className={cn('text-xs font-bold', t.passed ? 'text-success' : 'text-destructive')}>{t.passed ? 'Passed' : 'Failed'}</span>
        </li>
      ))}
    </ul>
  )
}

function Log({ title, text }: { title: string; text: string }) {
  return (
    <details className="group rounded-[14px] border border-border bg-card">
      <summary className="cursor-pointer list-none px-4 py-3 text-sm font-bold select-none marker:hidden">
        <span className="mr-2 inline-block text-muted-foreground transition-transform group-open:rotate-90">›</span>{title}
      </summary>
      <pre className="max-h-80 overflow-auto border-t border-border bg-muted/40 p-4 font-mono text-xs leading-5 whitespace-pre-wrap break-words">{text}</pre>
    </details>
  )
}

export function ProofLogs({ proof }: { proof: Proof }) {
  const r = proof.sandbox_result
  const sandbox = r && proof.status !== 'passed' && r.output ? r.output : null
  if (!sandbox && !proof.agent_log_tail) return null
  return (
    <div className="space-y-2">
      {sandbox && <Log title="Sandbox output" text={sandbox} />}
      {proof.agent_log_tail && <Log title="Agent log, redacted tail" text={proof.agent_log_tail} />}
    </div>
  )
}
