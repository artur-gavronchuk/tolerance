'use client'

import Link from 'next/link'
import { useCallback, useEffect, useState } from 'react'
import { PageHeader, SectionTitle } from '@/components/page-header'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { admin, ApiError } from '@/lib/api'
import { errorText } from '@/lib/i18n/messages/errors'
import { useT } from '@/lib/i18n/client'
import type { T } from '@/lib/i18n/core'
import { adminMessages } from '@/lib/i18n/messages/admin'
import { FunnelSection } from '@/components/admin/funnel'
import { ModerationSection } from '@/components/admin/moderation'
import { FairplaySection } from '@/components/admin/fairplay'
import { useMe } from '@/lib/use-me'
import { cn } from '@/lib/utils'
import type { AdminEvent, AdminPoint, AdminPulse } from '@/lib/types'

const REFRESH_MS = 15_000

type AT = T<typeof adminMessages.en>

function ago(t: AT, iso: string, now = Date.now()) {
  const s = Math.max(0, Math.round((now - new Date(iso).getTime()) / 1000))
  if (s < 60) return t('ago.s', { n: s })
  if (s < 3600) return t('ago.m', { n: Math.round(s / 60) })
  if (s < 86400) return t('ago.h', { n: Math.round(s / 3600) })
  return t('ago.d', { n: Math.round(s / 86400) })
}

const stLabel = (t: AT, k: string) => (`st.${k}` in adminMessages.en ? t(`st.${k}` as 'st.queued') : k.replace('_', ' '))

function useAdminData(enabled: boolean) {
  const [pulse, setPulse] = useState<AdminPulse | null>(null)
  const [recent, setRecent] = useState<AdminEvent[] | null>(null)
  const [error, setError] = useState<ApiError | Error | null>(null)
  const [at, setAt] = useState<number | null>(null)
  const load = useCallback(async () => {
    try {
      const [p, r] = await Promise.all([admin.pulse(), admin.recent()])
      setPulse(p)
      setRecent(r)
      setError(null)
      setAt(Date.now())
    } catch (e) {
      setError(e as Error)
    }
  }, [])
  useEffect(() => {
    if (!enabled) return
    void load()
    const t = setInterval(() => void load(), REFRESH_MS)
    return () => clearInterval(t)
  }, [enabled, load])
  return { pulse, recent, error, at }
}

export default function AdminPage() {
  const t = useT(adminMessages)
  const { me, loading } = useMe()
  const allowed = !!me?.can_admin
  const { pulse, recent, error, at } = useAdminData(allowed)

  if (loading) return <Skeleton className="h-40 rounded-xl" />
  if (!allowed) return <NotFoundLike />

  return (
    <div className="space-y-10">
      <PageHeader title={t('pulse')} kicker={t('admin')}
        actions={<span className="self-end text-xs text-muted-foreground">{at ? t('updated', { ago: ago(t, new Date(at).toISOString()) }) : t('loading')}</span>}>
        {t('lead')}
      </PageHeader>
      {error && <p role="alert" className="text-sm text-destructive">{errorText(error, t.locale)}</p>}
      {!pulse && !error && <Skeleton className="h-64 rounded-xl" />}
      {pulse && (
        <>
          <UsersSection p={pulse} />
          <DailySection p={pulse} />
          <TanksSection p={pulse} />
          <HealthSection p={pulse} />
        </>
      )}
      {recent && <FeedSection items={recent} />}
      <FunnelSection />
      <FairplaySection />
      <ModerationSection />
    </div>
  )
}

function NotFoundLike() {
  const t = useT(adminMessages)
  return (
    <div className="py-16">
      <p className="font-mono text-sm text-muted-foreground">404</p>
      <h1 className="display mt-2 text-title-sm sm:text-title">{t('nfTitle')}</h1>
      <p className="mt-3 text-muted-foreground">{t('nfText')}</p>
      <Link href="/" className="mt-6 inline-block text-sm font-semibold text-primary hover:underline">{t('nfLink')}</Link>
    </div>
  )
}

// ---- building blocks -----------------------------------------------------

function Tile({ label, value, hint, tone, children, className }: {
  label: string
  value: React.ReactNode
  hint?: React.ReactNode
  tone?: 'bad' | 'warn' | 'good'
  children?: React.ReactNode
  className?: string
}) {
  return (
    <div className={cn('min-w-0 rounded-xl border border-border bg-card p-4', className)}>
      <div className="text-xs font-semibold text-muted-foreground">{label}</div>
      <div className={cn('mt-1 font-mono text-2xl font-bold break-words',
        tone === 'bad' && 'text-destructive', tone === 'warn' && 'text-warning', tone === 'good' && 'text-success')}>{value}</div>
      {hint && <div className="mt-1 text-xs text-muted-foreground">{hint}</div>}
      {children}
    </div>
  )
}

// Fourteen small bars, scaled to the series' own maximum. The last bar is today and is drawn solid.
function Bars({ points, label }: { points: AdminPoint[]; label: string }) {
  const max = Math.max(1, ...points.map((p) => p.value))
  const W = 140
  const H = 36
  const step = W / points.length
  return (
    <svg viewBox={`0 0 ${W} ${H}`} className="mt-3 h-9 w-full" role="img" aria-label={`${label}: ${points.map((p) => p.value).join(', ')}`} preserveAspectRatio="none">
      {points.map((p, i) => {
        const h = p.value === 0 ? 1.5 : Math.max(3, (p.value / max) * (H - 2))
        return (
          <rect key={p.day} x={i * step + 1} y={H - h} width={step - 2} height={h} rx={1.5}
            className={i === points.length - 1 ? 'fill-primary' : 'fill-primary/35'}>
            <title>{`${p.day}: ${p.value}`}</title>
          </rect>
        )
      })}
    </svg>
  )
}

const sum = (m: Record<string, number> | undefined) => Object.values(m ?? {}).reduce((a, b) => a + b, 0)
const pct = (x: number) => `${(x * 100).toFixed(x > 0 && x < 0.1 ? 1 : 0)}%`
const dur = (t: AT, s: number) => (s < 90 ? `${Math.round(s)} ${t('unitS')}` : s < 5400 ? `${Math.round(s / 60)} ${t('unitM')}` : `${(s / 3600).toFixed(1)} ${t('unitH')}`)

function Statuses({ m, order }: { m: Record<string, number>; order: string[] }) {
  const t = useT(adminMessages)
  const keys = [...order, ...Object.keys(m).filter((k) => !order.includes(k))]
  return (
    <div className="mt-2 flex flex-wrap gap-1.5">
      {keys.map((k) => (
        <Badge key={k} variant={k === 'infra_error' && (m[k] ?? 0) > 0 ? 'destructive' : (m[k] ?? 0) > 0 ? 'secondary' : 'outline'}>{stLabel(t, k)} {m[k] ?? 0}</Badge>
      ))}
    </div>
  )
}

const grid = 'grid grid-cols-2 gap-3 lg:grid-cols-4'

// ---- sections ------------------------------------------------------------

function UsersSection({ p }: { p: AdminPulse }) {
  const t = useT(adminMessages)
  const u = p.users
  return (
    <section>
      <SectionTitle>{t('users')}</SectionTitle>
      <div className={grid}>
        <Tile label={t('total')} value={u.total} />
        <Tile label={t('newToday')} value={u.new_today} hint={t('new7d', { n: u.new_7d })} />
        <Tile label={t('activeToday')} value={u.active_today} hint={t('activeHint')} />
        <Tile label={t('signups14')} value={u.signup_series.reduce((a, b) => a + b.value, 0)}>
          <Bars points={u.signup_series} label={t('signupsPerDay')} />
        </Tile>
      </div>
    </section>
  )
}

function DailySection({ p }: { p: AdminPulse }) {
  const t = useT(adminMessages)
  const d = p.daily
  const total = sum(d.today)
  const infra = d.infra_rate_7d
  return (
    <section>
      <SectionTitle aside={d.task_slug ? <Link href="/" className="hover:text-foreground">{d.task_kind}</Link> : undefined}>{t('dailyTask')}</SectionTitle>
      <div className={grid}>
        <Tile label={t('todaysTask')} value={<span className="text-lg">{d.task_slug || t('notPicked')}</span>} hint={d.task_title || t('assignedHint')} />
        <Tile label={t('subsToday')} value={total} hint={t.plural('solvers', d.unique_solvers)}>
          <Statuses m={d.today} order={['queued', 'running', 'passed', 'failed', 'infra_error']} />
        </Tile>
        <Tile label={t('infraRate')} value={pct(infra)} tone={infra >= 0.2 ? 'bad' : infra >= 0.05 ? 'warn' : 'good'}
          hint={t('infraHint', { n: d.submissions_7d, dur: d.median_seconds == null ? '-' : dur(t, d.median_seconds) })} />
        <Tile label={t('subs14')} value={d.submission_series.reduce((a, b) => a + b.value, 0)}>
          <Bars points={d.submission_series} label={t('subsPerDay')} />
        </Tile>
        <Tile label={t('usersPerDay14')} value={d.user_series[d.user_series.length - 1]?.value ?? 0} hint={t('todayHint')} className="col-span-2 lg:col-span-4">
          <Bars points={d.user_series} label={t('uniquePerDay')} />
        </Tile>
      </div>
    </section>
  )
}

function TanksSection({ p }: { p: AdminPulse }) {
  const t = useT(adminMessages)
  const tk = p.tanks
  return (
    <section>
      <SectionTitle aside={<Link href="/tanks" className="hover:text-foreground">{t('openTanks')}</Link>}>{t('tanks')}</SectionTitle>
      <div className={grid}>
        <Tile label={t('bots')} value={tk.bots_total} hint={t('activeBots', { n: tk.bots_active })} />
        <Tile label={t('uploadsToday')} value={tk.uploads_today} hint={t('rejectedToday', { n: tk.rejected_today })} tone={tk.rejected_today > 0 && tk.rejected_today >= tk.uploads_today ? 'warn' : undefined} />
        <Tile label={t('matchesHour')} value={sum(tk.matches_last_hour)}>
          <Statuses m={tk.matches_last_hour} order={['queued', 'running', 'finished', 'infra_error']} />
        </Tile>
        <Tile label={t('tournament')} value={<span className="text-lg">{tk.tournament ? stLabel(t, tk.tournament.status) : t('none')}</span>}
          hint={tk.tournament ? <Link href={`/tanks/tournaments/${tk.tournament.id}`} className="text-primary hover:underline">{tk.tournament.name}</Link> : undefined} />
      </div>
      <div className="mt-3 rounded-xl border border-border bg-card">
        <Table>
          <TableHeader>
            <TableRow><TableHead>#</TableHead><TableHead>{t('colBot')}</TableHead><TableHead>{t('colOwner')}</TableHead><TableHead className="text-right">{t('colRating')}</TableHead><TableHead className="text-right">{t('colMatches')}</TableHead></TableRow>
          </TableHeader>
          <TableBody>
            {tk.ladder.length === 0 && <TableRow><TableCell colSpan={5} className="text-muted-foreground">{t('noLadder')}</TableCell></TableRow>}
            {tk.ladder.map((r) => (
              <TableRow key={r.bot_id}>
                <TableCell className="font-mono">{r.rank}</TableCell>
                <TableCell><Link href={`/tanks/bots/${r.bot_id}`} className="font-semibold hover:underline">{r.name}</Link></TableCell>
                <TableCell className="text-muted-foreground">{r.owner || t('house')}</TableCell>
                <TableCell className="text-right font-mono">{r.rating}</TableCell>
                <TableCell className="text-right font-mono">{r.matches}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </section>
  )
}

function HealthSection({ p }: { p: AdminPulse }) {
  const t = useT(adminMessages)
  const h = p.health
  const queued = h.jobs.filter((j) => j.state === 'queued').reduce((a, b) => a + b.count, 0)
  const oldest = h.oldest_queued_seconds
  return (
    <section>
      <SectionTitle>{t('health')}</SectionTitle>
      <div className={grid}>
        <Tile label={t('jobsQueued')} value={queued} tone={queued > 20 ? 'warn' : undefined}
          hint={oldest == null ? t('queueEmpty') : t('oldestWaiting', { dur: dur(t, oldest) })} />
        <Tile label={t('jobsRetried')} value={h.retried_jobs} hint={t('attemptsAbove')} tone={h.retried_jobs > 0 ? 'warn' : undefined} />
        <Tile label={t('jobsFailed')} value={h.failed_jobs} tone={h.failed_jobs > 0 ? 'bad' : 'good'} />
        <Tile label={t('stuck15')} value={h.stuck.length} tone={h.stuck.length > 0 ? 'bad' : 'good'} hint={t('stuckHint')} />
      </div>

      <div className="mt-3 grid gap-3 lg:grid-cols-2">
        <div className="min-w-0 rounded-xl border border-border bg-card">
          <Table>
            <TableHeader><TableRow><TableHead>{t('colJobKind')}</TableHead><TableHead>{t('colState')}</TableHead><TableHead className="text-right">{t('colCount')}</TableHead></TableRow></TableHeader>
            <TableBody>
              {h.jobs.length === 0 && <TableRow><TableCell colSpan={3} className="text-muted-foreground">{t('noJobs')}</TableCell></TableRow>}
              {h.jobs.map((j) => (
                <TableRow key={j.kind + j.state}>
                  <TableCell className="font-mono">{j.kind}</TableCell>
                  <TableCell><Badge variant={j.state === 'failed' ? 'destructive' : j.state === 'done' ? 'outline' : 'secondary'}>{stLabel(t, j.state)}</Badge></TableCell>
                  <TableCell className="text-right font-mono">{j.count}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
        <div className="min-w-0 rounded-xl border border-border bg-card">
          <Table>
            <TableHeader><TableRow><TableHead>{t('colStuck')}</TableHead><TableHead>{t('colId')}</TableHead><TableHead className="text-right">{t('colRunningFor')}</TableHead></TableRow></TableHeader>
            <TableBody>
              {h.stuck.length === 0 && <TableRow><TableCell colSpan={3} className="text-muted-foreground">{t('nothingStuck')}</TableCell></TableRow>}
              {h.stuck.map((s) => (
                <TableRow key={s.id}>
                  <TableCell>{s.kind}</TableCell>
                  <TableCell><Link href={s.href} className="font-mono text-xs hover:underline">{s.id}</Link></TableCell>
                  <TableCell className="text-right font-mono">{s.age_minutes} {t('unitM')}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </div>

      <h3 className="heading mt-6 mb-2 text-base">{t('lastInfra')}</h3>
      <div className="rounded-xl border border-border bg-card">
        <Table>
          <TableHeader><TableRow><TableHead>{t('colWhen')}</TableHead><TableHead>{t('colKind')}</TableHead><TableHead>{t('colId')}</TableHead><TableHead>{t('colReason')}</TableHead></TableRow></TableHeader>
          <TableBody>
            {h.infra_errors.length === 0 && <TableRow><TableCell colSpan={4} className="text-muted-foreground">{t('noInfra')}</TableCell></TableRow>}
            {h.infra_errors.map((e) => (
              <TableRow key={e.kind + e.id}>
                <TableCell className="text-muted-foreground">{ago(t, e.at)}</TableCell>
                <TableCell>{e.kind}</TableCell>
                <TableCell className="font-mono text-xs">{e.id}</TableCell>
                <TableCell className="max-w-[28rem] truncate" title={e.reason}>{e.reason || '-'}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </section>
  )
}

function statusVariant(s: string) {
  if (s === 'passed' || s === 'done' || s === 'active' || s === 'finished') return 'default' as const
  if (s === 'failed' || s === 'rejected' || s === 'infra_error') return 'destructive' as const
  return 'outline' as const
}

function FeedSection({ items }: { items: AdminEvent[] }) {
  const t = useT(adminMessages)
  return (
    <section>
      <SectionTitle aside={t('last50')}>{t('recent')}</SectionTitle>
      <div className="rounded-xl border border-border bg-card">
        <Table>
          <TableHeader><TableRow><TableHead>{t('colWhen')}</TableHead><TableHead>{t('colEvent')}</TableHead><TableHead>{t('colWho')}</TableHead><TableHead>{t('colDetail')}</TableHead><TableHead>{t('colStatus')}</TableHead></TableRow></TableHeader>
          <TableBody>
            {items.length === 0 && <TableRow><TableCell colSpan={5} className="text-muted-foreground">{t('nothingYet')}</TableCell></TableRow>}
            {items.map((e, i) => (
              <TableRow key={`${e.type}-${e.at}-${i}`}>
                <TableCell className="text-muted-foreground">{ago(t, e.at)}</TableCell>
                <TableCell>{t(`type.${e.type}` as 'type.signup')}</TableCell>
                <TableCell><Link href={e.href} className="font-semibold hover:underline">{e.title}</Link></TableCell>
                <TableCell className="max-w-[22rem] truncate text-muted-foreground" title={e.detail}>{e.detail}</TableCell>
                <TableCell>{e.status && <Badge variant={statusVariant(e.status)}>{stLabel(t, e.status)}</Badge>}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </section>
  )
}
