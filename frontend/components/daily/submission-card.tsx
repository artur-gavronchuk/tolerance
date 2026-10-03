'use client'

import { useEffect } from 'react'
import { Check, LoaderCircle, X } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { fmtScore } from '@/components/daily/daily-board'
import { api } from '@/lib/api'
import { ago, reasonLabel, statusLabel } from '@/lib/format'
import { useT } from '@/lib/i18n/client'
import { dailyMessages } from '@/lib/i18n/messages/daily'
import type { Submission } from '@/lib/types'

const ACTIVE = ['queued', 'running']

// One submission with live status: while queued or running it polls
// GET /submissions/{id} every 2 s and reports each update upwards.
export function SubmissionCard({ sub, onUpdate }: { sub: Submission; onUpdate: (s: Submission) => void }) {
  const t = useT(dailyMessages)
  const active = ACTIVE.includes(sub.status)
  useEffect(() => {
    if (!active) return
    const t = setInterval(() => {
      api<Submission>(`/submissions/${sub.id}`).then(onUpdate).catch(() => {})
    }, 2000)
    return () => clearInterval(t)
  }, [active, sub.id, onUpdate])

  const reason = sub.status === 'failed' ? reasonLabel(sub.failure_reason, t.locale) : null
  return (
    <li className="p-4">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5">
        <Badge variant={sub.status === 'passed' ? 'default' : sub.status === 'failed' ? 'destructive' : 'secondary'}>
          {active && <LoaderCircle className="animate-spin" />}
          {statusLabel(sub.status, t.locale)}
        </Badge>
        {!active && sub.score != null && (
          <span className="font-mono text-sm font-bold">{t('scoreN', { score: fmtScore(sub.score, t.locale) })}</span>
        )}
        {!active && sub.total_tests > 0 && (
          <span className="font-mono text-sm font-bold">
            {sub.passed_tests}/{sub.total_tests} {sub.score != null || sub.tests.some((t) => 'score' in t) ? t('validCases') : t('testsWord')}
          </span>
        )}
        {sub.day === null && <Badge variant="outline">{t('practice')}</Badge>}
        {sub.made_with && <span className="min-w-0 truncate text-sm text-muted-foreground">{sub.made_with}</span>}
        <span className="ml-auto text-xs text-muted-foreground">{ago(sub.created_at, Date.now(), t.locale)}</span>
      </div>
      {sub.status === 'infra_error' && (
        <p className="mt-2 text-sm text-muted-foreground">{t('infraNote')}</p>
      )}
      {reason && <p className="mt-2 text-sm text-destructive">{reason}</p>}
      {sub.tests.length > 0 && (
        <ul className="mt-3 grid gap-x-6 gap-y-1 sm:grid-cols-2">
          {sub.tests.map((test) => (
            <li key={test.name} className="flex min-w-0 items-center gap-2 text-sm">
              {test.passed ? <Check className="size-4 shrink-0 text-success" /> : <X className="size-4 shrink-0 text-destructive" />}
              <span className="truncate font-mono text-xs" title={test.name}>{test.name}</span>
              {test.score !== undefined && test.passed && <span className="ml-auto shrink-0 font-mono text-xs text-muted-foreground">{fmtScore(test.score, t.locale)}</span>}
              {test.reason && <span className="min-w-0 truncate text-xs text-destructive" title={test.reason}>{test.reason}</span>}
            </li>
          ))}
        </ul>
      )}
      {sub.log_tail && (
        <details className="mt-3 rounded-lg border border-border">
          <summary className="cursor-pointer px-3 py-2 text-sm font-semibold">{t('logTail')}</summary>
          <pre className="max-h-64 overflow-auto border-t border-border p-3 font-mono text-xs leading-5 whitespace-pre-wrap break-words">{sub.log_tail}</pre>
        </details>
      )}
    </li>
  )
}
