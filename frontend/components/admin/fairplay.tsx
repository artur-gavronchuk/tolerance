'use client'

import Link from 'next/link'
import { useCallback, useEffect, useState } from 'react'
import { SectionTitle } from '@/components/page-header'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { fairplay, moderation } from '@/lib/api'
import { errorText } from '@/lib/i18n/messages/errors'
import { useT } from '@/lib/i18n/client'
import { formatDateTime, type T } from '@/lib/i18n/core'
import { fairplayMessages } from '@/lib/i18n/messages/fairplay'
import type { FairFlag, FairItem, FairOverview, FairReport } from '@/lib/types'
import { ReasonForm } from './reason-form'

type FT = T<typeof fairplayMessages.en>

function describe(t: FT, f: FairFlag) {
  const d = f.detail as Record<string, unknown>
  const users = Array.isArray(d.users) ? (d.users as string[]).join(', ') : ''
  switch (f.signal) {
    case 'fast_solve': return t('detail.fast_solve', { n: Number(d.seconds ?? 0) })
    case 'burst': return t('detail.burst', { n: Number(d.uploads ?? 0), m: Number(d.minutes ?? 0) })
    case 'shared_device': return t('detail.shared_device', { users })
    case 'shared_ip': return t('detail.shared_ip', { users })
    case 'near_duplicate': return t('detail.near_duplicate', { pct: Math.round(Number(d.similarity ?? 0) * 100), other: String(d.other ?? '') })
  }
}

// One action button that turns into a reason form (hide / ban reuse the moderation API) and, once done, closes the item.
function Action({ label, run, after }: { label: string; run: (reason: string) => Promise<unknown>; after: () => Promise<unknown> }) {
  const [open, setOpen] = useState(false)
  if (!open) return <Button type="button" size="xs" variant="outline" onClick={() => setOpen(true)}>{label}</Button>
  return <ReasonForm label={label} run={async (reason) => { await run(reason); await after() }} onDone={() => setOpen(false)} onCancel={() => setOpen(false)} />
}

function Dismiss({ run }: { run: () => Promise<unknown> }) {
  const t = useT(fairplayMessages)
  const [busy, setBusy] = useState(false)
  return <Button type="button" size="xs" variant="ghost" disabled={busy} onClick={async () => { setBusy(true); try { await run() } finally { setBusy(false) } }}>{t('dismiss')}</Button>
}

export function FairplaySection() {
  const t = useT(fairplayMessages)
  const [data, setData] = useState<FairOverview | null>(null)
  const [error, setError] = useState<Error | null>(null)
  const load = useCallback(() => fairplay.overview().then((d) => { setData(d); setError(null) }).catch((e) => setError(e as Error)), [])
  useEffect(() => { void load() }, [load])

  return (
    <section>
      <SectionTitle>{t('title')}</SectionTitle>
      <p className="-mt-1 mb-3 text-sm text-muted-foreground">{t('lead')}</p>
      {error && <p role="alert" className="text-sm text-destructive">{errorText(error, t.locale)}</p>}
      {!data && !error && <p className="text-sm text-muted-foreground">{t('loading')}</p>}
      {data && (
        <div className="space-y-6">
          <div>
            <h3 className="heading mb-2 text-base">{t('flagged')}</h3>
            {data.flags.length === 0 && <Empty text={t('noFlags')} />}
            <ul className="space-y-2">{data.flags.map((it) => <FlagRow key={it.subject_id} it={it} reload={load} />)}</ul>
          </div>
          <div>
            <h3 className="heading mb-2 text-base">{t('reports')}</h3>
            {data.reports.length === 0 && <Empty text={t('noReports')} />}
            <ul className="space-y-2">{data.reports.map((r) => <ReportRow key={r.id} r={r} reload={load} />)}</ul>
          </div>
          <div>
            <h3 className="heading mb-2 text-base">{t('clusters')}</h3>
            {data.clusters.length === 0 && <Empty text={t('noClusters')} />}
            <ul className="space-y-2">
              {data.clusters.map((c) => (
                <li key={c.kind + c.hash} className="flex flex-wrap items-center gap-2 rounded-[14px] border border-border bg-card p-3 text-sm">
                  <Badge variant={c.kind === 'device' ? 'destructive' : 'outline'}>{c.kind === 'device' ? t('kindDevice') : t('kindIp')}</Badge>
                  <span className="font-mono text-xs text-muted-foreground">{c.hash}</span>
                  {c.users.map((u) => <Link key={u.id} href={`/u/${encodeURIComponent(u.handle)}`} className="font-semibold hover:underline">{u.handle}{u.banned ? ` (${t('bannedNow')})` : ''}</Link>)}
                </li>
              ))}
            </ul>
          </div>
        </div>
      )}
    </section>
  )
}

function Empty({ text }: { text: string }) {
  return <p className="rounded-[14px] border border-dashed border-strong p-3 text-sm text-muted-foreground">{text}</p>
}

function FlagRow({ it, reload }: { it: FairItem; reload: () => Promise<unknown> }) {
  const t = useT(fairplayMessages)
  return (
    <li className="min-w-0 rounded-[14px] border border-border bg-card p-3">
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <Link href={`/u/${encodeURIComponent(it.handle)}`} className="font-semibold hover:underline">{it.handle}</Link>
        {it.banned && <Badge variant="destructive">{t('bannedNow')}</Badge>}
        {it.hidden && <Badge variant="outline">{t('hiddenNow')}</Badge>}
        <span className="text-xs text-muted-foreground">{t('submission')} · {it.task_slug} · {it.day ?? t('practice')} · {formatDateTime(t.locale, it.at)}</span>
      </div>
      <ul className="mt-2 space-y-1 text-sm">
        {it.flags.map((f) => (
          <li key={f.id}><Badge variant="secondary" className="mr-2">{t(`signal.${f.signal}`)}</Badge><span className="text-muted-foreground">{describe(t, f)}</span></li>
        ))}
      </ul>
      <div className="mt-3 flex flex-wrap items-center gap-2">
        {!it.hidden && <Action label={t('hideSub')} run={(reason) => moderation.hide('submission', it.subject_id, reason)} after={async () => { await fairplay.resolveFlags(it.subject_id, 'actioned'); await reload() }} />}
        {!it.banned && <Action label={t('banUser')} run={(reason) => moderation.ban(it.user_id, reason)} after={async () => { await fairplay.resolveFlags(it.subject_id, 'actioned'); await reload() }} />}
        <Dismiss run={async () => { await fairplay.resolveFlags(it.subject_id, 'dismissed'); await reload() }} />
      </div>
    </li>
  )
}

function ReportRow({ r, reload }: { r: FairReport; reload: () => Promise<unknown> }) {
  const t = useT(fairplayMessages)
  return (
    <li className="min-w-0 rounded-[14px] border border-border bg-card p-3">
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <Link href={`/u/${encodeURIComponent(r.handle)}`} className="font-semibold hover:underline">{r.handle}</Link>
        {r.banned && <Badge variant="destructive">{t('bannedNow')}</Badge>}
        <Badge variant="secondary">{t(`reason.${r.reason as 'cheating'}`)}</Badge>
        <span className="text-xs text-muted-foreground">{t('reportedBy', { who: r.reporter })} · {formatDateTime(t.locale, r.at)}{r.others > 0 ? ` · ${t('alsoReported', { n: r.others })}` : ''}</span>
      </div>
      {r.details && <p className="mt-2 text-sm break-words text-muted-foreground">{r.details}</p>}
      <div className="mt-3 flex flex-wrap items-center gap-2">
        {!r.banned && <Action label={t('banUser')} run={(reason) => moderation.ban(r.user_id, reason)} after={async () => { await fairplay.resolveReport(r.id, 'actioned'); await reload() }} />}
        <Dismiss run={async () => { await fairplay.resolveReport(r.id, 'dismissed'); await reload() }} />
      </div>
    </li>
  )
}
