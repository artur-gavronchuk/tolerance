import { Card } from '@/components/ui/card'
import type { Proof } from '@/lib/types'
import { REASON_LABEL, duration } from '@/lib/format'
import { cn } from '@/lib/utils'

export function ProofResult({ proof }: { proof: Proof }) {
  const r = proof.sandbox_result
  const passed = proof.status === 'passed'
  return (
    <div className="space-y-4">
      <div>
        <h2 className={cn('text-xl font-semibold', passed ? 'text-success' : 'text-destructive')}>
          {passed ? 'Verified: your agent works on its own' : proof.status === 'failed' ? 'Not verified yet' : proof.status === 'expired' ? 'Expired' : 'Platform error'}
        </h2>
        {proof.failure_reason && <p className="mt-1 text-sm text-muted-foreground">{REASON_LABEL[proof.failure_reason] ?? proof.failure_reason}</p>}
        {proof.status === 'infra_error' && <p className="mt-1 text-sm text-muted-foreground">This one is on us, not on your agent. Retry costs you nothing.</p>}
      </div>
      <dl className="grid grid-cols-2 gap-2 text-sm sm:grid-cols-4">
        <div><dt className="text-xs text-muted-foreground">Agent time</dt><dd>{duration(proof.agent_duration_ms)}</dd></div>
        <div><dt className="text-xs text-muted-foreground">Agent exit code</dt><dd>{proof.agent_exit_code ?? '—'}</dd></div>
        <div><dt className="text-xs text-muted-foreground">Tests</dt><dd>{r ? `${r.tests.filter((t) => t.passed).length}/${r.tests.length}` : '—'}</dd></div>
        <div><dt className="text-xs text-muted-foreground">Diff</dt><dd>{proof.diff.length} bytes</dd></div>
      </dl>
      {r && r.tests.length > 0 && (
        <Card className="overflow-hidden p-0">
          <table className="w-full text-sm">
            <tbody className="divide-y divide-border">
              {r.tests.map((t) => (
                <tr key={t.name}>
                  <td className="px-4 py-2 font-mono text-xs">{t.name}</td>
                  <td className={cn('px-4 py-2 text-right text-xs', t.passed ? 'text-success' : 'text-destructive')}>{t.passed ? 'pass' : 'FAIL'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </Card>
      )}
      {r && !passed && r.output && (
        <details><summary className="cursor-pointer text-sm text-muted-foreground">Sandbox output</summary>
          <pre className="mt-2 max-h-72 overflow-auto rounded-md border border-border bg-muted/30 p-3 font-mono text-xs">{r.output}</pre></details>
      )}
      {proof.agent_log_tail && (
        <details><summary className="cursor-pointer text-sm text-muted-foreground">Agent log (redacted tail)</summary>
          <pre className="mt-2 max-h-72 overflow-auto rounded-md border border-border bg-muted/30 p-3 font-mono text-xs">{proof.agent_log_tail}</pre></details>
      )}
    </div>
  )
}
