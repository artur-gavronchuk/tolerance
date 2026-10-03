'use client'

import Link from 'next/link'
import { Check, Loader2, X } from 'lucide-react'
import { BotBadge } from './bot-badge'
import type { VersionView } from '@/lib/types'
import { cn } from '@/lib/utils'
import { CopyReportButton } from './copy-report-button'
import { useT } from '@/lib/i18n/client'
import { tanksOwnerMessages as m, ownerError } from '@/lib/i18n/messages/tanks-owner'

const STATUS_KEY = { pending: 'ver.checking', active: 'ver.active', rejected: 'ver.rejected' } as const

function StatusBadge({ status }: { status: VersionView['status'] }) {
  const t = useT(m)
  const Icon = status === 'active' ? Check : status === 'rejected' ? X : Loader2
  const tone = status === 'active' ? 'text-success' : status === 'rejected' ? 'text-destructive' : 'text-muted-foreground'
  return (
    <span className={cn('inline-flex items-center gap-1 text-xs font-bold', tone)}>
      <Icon className={cn('size-3.5', status === 'pending' && 'animate-spin')} strokeWidth={2.75} />
      {t(STATUS_KEY[status])}
    </span>
  )
}

// The checks a version has to clear before it can become active: the
// package built, the process answered `ready`, it survived a whole match
// without crashing, and it beat the idle house tank. Each has its own
// pass/fail and a one-line detail — a "supersede" entry (bookkeeping for a
// version that passed but lost the race to a newer one) shows up here too.
function CheckList({ checks }: { checks: VersionView['checks'] }) {
  if (checks.length === 0) return null
  return (
    <ul className="mt-3 space-y-1.5">
      {checks.map((c) => (
        <li key={c.name} className="flex items-start gap-2 text-xs">
          <span className={cn('mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full',
            c.passed ? 'bg-success/12 text-success' : 'bg-destructive/10 text-destructive')}>
            {c.passed ? <Check className="size-2.5" strokeWidth={3} /> : <X className="size-2.5" strokeWidth={3} />}
          </span>
          <span className="min-w-0">
            <span className="font-mono font-semibold">{c.name}</span>
            {c.detail && <span className="text-muted-foreground">: {c.detail}</span>}
          </span>
        </li>
      ))}
    </ul>
  )
}

// What a freshly uploaded version is going through. The platform runs one trial
// match against the house idle bot; the steps below are what it will report.
function PendingNote() {
  const t = useT(m)
  return (
    <div className="mt-3 rounded-lg border border-border bg-muted/30 px-3 py-2.5 text-xs text-muted-foreground">
      <p className="font-semibold text-foreground">{t('ver.pendingTitle')}</p>
      <p className="mt-1">
        {t('ver.pendingBody1')}<span className="font-mono">ready</span>{t('ver.pendingBody2')}
      </p>
    </div>
  )
}

function CheckLog({ text }: { text: string }) {
  const t = useT(m)
  if (!text) return null
  return (
    <details className="group mt-3 rounded-lg border border-border bg-muted/30">
      <summary className="cursor-pointer list-none px-3 py-2 text-xs font-bold select-none marker:hidden">
        <span className="mr-1.5 inline-block text-muted-foreground transition-transform group-open:rotate-90">›</span>{t('ver.checkLog')}
      </summary>
      <pre className="max-h-64 overflow-auto border-t border-border p-3 font-mono text-xs leading-5 whitespace-pre-wrap break-words">{text}</pre>
    </details>
  )
}

// A bot's version history: newest first (the caller already orders them),
// each with its provenance, status and the checks that decided it.
export function VersionList({ versions }: { versions: VersionView[] }) {
  const t = useT(m)
  if (versions.length === 0) {
    return (
      <p className="rounded-xl border border-dashed border-strong px-5 py-8 text-center text-sm text-muted-foreground">
        {t('ver.empty')}
      </p>
    )
  }
  return (
    <ul className="divide-y divide-border rounded-xl border border-border bg-card">
      {versions.map((v) => (
        <li key={v.id} className="p-4">
          <div className="flex flex-wrap items-center gap-3">
            <span className="font-mono text-sm font-bold">v{v.number}</span>
            <BotBadge source={v.source} />
            <StatusBadge status={v.status} />
            {v.check_match_id && (
              <span className="ml-auto flex flex-wrap items-center gap-3">
                {v.status !== 'pending' && (
                  <CopyReportButton matchIds={[v.check_match_id]} size="xs" variant="ghost" label={t('ver.copyTrial')} />
                )}
                <Link href={`/tanks/matches/${v.check_match_id}`} className="text-xs font-semibold text-primary hover:underline">
                  {t('ver.watchTrial')}
                </Link>
              </span>
            )}
          </div>
          {v.status === 'pending' && <PendingNote />}
          <CheckList checks={v.checks} />
          <CheckLog text={v.check_log} />
        </li>
      ))}
    </ul>
  )
}
