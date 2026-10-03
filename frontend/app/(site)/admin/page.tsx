'use client'

import Link from 'next/link'
import { useCallback, useEffect, useState } from 'react'
import { PageHeader, SectionTitle } from '@/components/page-header'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { admin, ApiError, friendlyMessage } from '@/lib/api'
import { ago } from '@/lib/format'
import { useMe } from '@/lib/use-me'
import { cn } from '@/lib/utils'
import type { AdminEvent, AdminPoint, AdminPulse } from '@/lib/types'

const REFRESH_MS = 15_000

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
  const { me, loading } = useMe()
  const allowed = !!me?.can_admin
  const { pulse, recent, error, at } = useAdminData(allowed)

  if (loading) return <Skeleton className="h-40 rounded-[14px]" />
  if (!allowed) return <NotFoundLike />

  return (
    <div className="space-y-10">
      <PageHeader title="Pulse" kicker="Admin"
        actions={<span className="self-end text-xs text-muted-foreground">{at ? `Updated ${ago(new Date(at).toISOString())}, refreshes every 15s` : 'Loading'}</span>}>
        What is happening across the daily task, the weekly products and tanks, and whether the machinery is healthy.
      </PageHeader>
      {error && <p role="alert" className="text-sm text-destructive">{friendlyMessage(error)}</p>}
      {!pulse && !error && <Skeleton className="h-64 rounded-[14px]" />}
      {pulse && (
        <>
          <UsersSection p={pulse} />
          <DailySection p={pulse} />
          <ProductsSection p={pulse} />
          <TanksSection p={pulse} />
          <HealthSection p={pulse} />
        </>
      )}
      {recent && <FeedSection items={recent} />}
    </div>
  )
}

function NotFoundLike() {
  return (
    <div className="py-16">
      <p className="font-mono text-sm text-muted-foreground">404</p>
      <h1 className="display mt-2 text-[2.6rem]">Nothing here</h1>
      <p className="mt-3 text-muted-foreground">This page doesn’t exist.</p>
      <Link href="/" className="mt-6 inline-block text-sm font-semibold text-primary hover:underline">Today’s task</Link>
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
    <div className={cn('min-w-0 rounded-[14px] border border-border bg-card p-4', className)}>
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
const dur = (s: number) => (s < 90 ? `${Math.round(s)}s` : s < 5400 ? `${Math.round(s / 60)}m` : `${(s / 3600).toFixed(1)}h`)
const when = (iso: string | null) => (iso ? new Date(iso).toLocaleString('en-GB', { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit', timeZone: 'UTC' }) + ' UTC' : '-')

function Statuses({ m, order }: { m: Record<string, number>; order: string[] }) {
  const keys = [...order, ...Object.keys(m).filter((k) => !order.includes(k))]
  return (
    <div className="mt-2 flex flex-wrap gap-1.5">
      {keys.map((k) => (
        <Badge key={k} variant={k === 'infra_error' && (m[k] ?? 0) > 0 ? 'destructive' : (m[k] ?? 0) > 0 ? 'secondary' : 'outline'}>{k.replace('_', ' ')} {m[k] ?? 0}</Badge>
      ))}
    </div>
  )
}

const grid = 'grid grid-cols-2 gap-3 lg:grid-cols-4'

// ---- sections ------------------------------------------------------------

function UsersSection({ p }: { p: AdminPulse }) {
  const u = p.users
  return (
    <section>
      <SectionTitle>Users</SectionTitle>
      <div className={grid}>
        <Tile label="Total" value={u.total} />
        <Tile label="New today" value={u.new_today} hint={`${u.new_7d} in 7 days`} />
        <Tile label="Active today" value={u.active_today} hint="submitted, uploaded or pushed a bot" />
        <Tile label="Signups, 14 days" value={u.signup_series.reduce((a, b) => a + b.value, 0)}>
          <Bars points={u.signup_series} label="Signups per day" />
        </Tile>
      </div>
    </section>
  )
}

function DailySection({ p }: { p: AdminPulse }) {
  const d = p.daily
  const total = sum(d.today)
  const infra = d.infra_rate_7d
  return (
    <section>
      <SectionTitle aside={d.task_slug ? <Link href="/" className="hover:text-foreground">{d.task_kind}</Link> : undefined}>Daily task</SectionTitle>
      <div className={grid}>
        <Tile label="Today’s task" value={<span className="text-lg">{d.task_slug || 'not picked yet'}</span>} hint={d.task_title || 'Assigned on the first visit of the day'} />
        <Tile label="Submissions today" value={total} hint={`${d.unique_solvers} unique ${d.unique_solvers === 1 ? 'solver' : 'solvers'}`}>
          <Statuses m={d.today} order={['queued', 'running', 'passed', 'failed', 'infra_error']} />
        </Tile>
        <Tile label="Infra error rate, 7d" value={pct(infra)} tone={infra >= 0.2 ? 'bad' : infra >= 0.05 ? 'warn' : 'good'}
          hint={`${d.submissions_7d} submissions; median verdict in ${d.median_seconds == null ? '-' : dur(d.median_seconds)}`} />
        <Tile label="Submissions, 14 days" value={d.submission_series.reduce((a, b) => a + b.value, 0)}>
          <Bars points={d.submission_series} label="Submissions per day" />
        </Tile>
        <Tile label="Unique users per day, 14 days" value={d.user_series[d.user_series.length - 1]?.value ?? 0} hint="today" className="col-span-2 lg:col-span-4">
          <Bars points={d.user_series} label="Unique submitters per day" />
        </Tile>
      </div>
    </section>
  )
}

function ProductsSection({ p }: { p: AdminPulse }) {
  const x = p.products
  return (
    <section>
      <SectionTitle aside={<Link href="/products" className="hover:text-foreground">Open products</Link>}>Product of the week</SectionTitle>
      <div className={grid}>
        <Tile label="This week" value={<span className="text-lg">{x.task_slug || 'none open'}</span>}
          hint={x.task_slug ? `${x.task_kind}, ${x.deadline ? `deadline ${when(x.deadline)}` : 'no deadline'}` : undefined} />
        <Tile label="Entries" value={x.entries}>
          <Statuses m={x.by_status} order={['queued', 'running', 'done', 'infra_error']} />
        </Tile>
        <Tile label="Votes so far" value={x.votes} />
        <Tile label="Up next" value={<span className="text-lg">{x.next_kind || 'nothing queued'}</span>} hint={`${x.upcoming} fresh ${x.upcoming === 1 ? 'task' : 'tasks'} not played yet`} />
      </div>
    </section>
  )
}

function TanksSection({ p }: { p: AdminPulse }) {
  const t = p.tanks
  return (
    <section>
      <SectionTitle aside={<Link href="/tanks" className="hover:text-foreground">Open tanks</Link>}>Tanks</SectionTitle>
      <div className={grid}>
        <Tile label="Bots" value={t.bots_total} hint={`${t.bots_active} active (passed the check)`} />
        <Tile label="Uploads today" value={t.uploads_today} hint={`${t.rejected_today} rejected today`} tone={t.rejected_today > 0 && t.rejected_today >= t.uploads_today ? 'warn' : undefined} />
        <Tile label="Matches, last hour" value={sum(t.matches_last_hour)}>
          <Statuses m={t.matches_last_hour} order={['queued', 'running', 'finished', 'infra_error']} />
        </Tile>
        <Tile label="Tournament" value={<span className="text-lg">{t.tournament ? t.tournament.status : 'none'}</span>}
          hint={t.tournament ? <Link href={`/tanks/tournaments/${t.tournament.id}`} className="text-primary hover:underline">{t.tournament.name}</Link> : undefined} />
      </div>
      <div className="mt-3 rounded-[14px] border border-border bg-card">
        <Table>
          <TableHeader>
            <TableRow><TableHead>#</TableHead><TableHead>Bot</TableHead><TableHead>Owner</TableHead><TableHead className="text-right">Rating</TableHead><TableHead className="text-right">Matches</TableHead></TableRow>
          </TableHeader>
          <TableBody>
            {t.ladder.length === 0 && <TableRow><TableCell colSpan={5} className="text-muted-foreground">No bots on the ladder yet.</TableCell></TableRow>}
            {t.ladder.map((r) => (
              <TableRow key={r.bot_id}>
                <TableCell className="font-mono">{r.rank}</TableCell>
                <TableCell><Link href={`/tanks/bots/${r.bot_id}`} className="font-semibold hover:underline">{r.name}</Link></TableCell>
                <TableCell className="text-muted-foreground">{r.owner || 'house'}</TableCell>
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
  const h = p.health
  const queued = h.jobs.filter((j) => j.state === 'queued').reduce((a, b) => a + b.count, 0)
  const oldest = h.oldest_queued_seconds
  return (
    <section>
      <SectionTitle>Health</SectionTitle>
      <div className={grid}>
        <Tile label="Jobs queued" value={queued} tone={queued > 20 ? 'warn' : undefined}
          hint={oldest == null ? 'queue is empty' : `oldest waiting ${dur(oldest)}`} />
        <Tile label="Jobs retried, 7d" value={h.retried_jobs} hint="attempts above one" tone={h.retried_jobs > 0 ? 'warn' : undefined} />
        <Tile label="Jobs failed, 7d" value={h.failed_jobs} tone={h.failed_jobs > 0 ? 'bad' : 'good'} />
        <Tile label="Stuck over 15 min" value={h.stuck.length} tone={h.stuck.length > 0 ? 'bad' : 'good'} hint="submissions, entries, matches" />
      </div>

      <div className="mt-3 grid gap-3 lg:grid-cols-2">
        <div className="min-w-0 rounded-[14px] border border-border bg-card">
          <Table>
            <TableHeader><TableRow><TableHead>Job kind</TableHead><TableHead>State</TableHead><TableHead className="text-right">Count</TableHead></TableRow></TableHeader>
            <TableBody>
              {h.jobs.length === 0 && <TableRow><TableCell colSpan={3} className="text-muted-foreground">No jobs in the last day.</TableCell></TableRow>}
              {h.jobs.map((j) => (
                <TableRow key={j.kind + j.state}>
                  <TableCell className="font-mono">{j.kind}</TableCell>
                  <TableCell><Badge variant={j.state === 'failed' ? 'destructive' : j.state === 'done' ? 'outline' : 'secondary'}>{j.state}</Badge></TableCell>
                  <TableCell className="text-right font-mono">{j.count}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
        <div className="min-w-0 rounded-[14px] border border-border bg-card">
          <Table>
            <TableHeader><TableRow><TableHead>Stuck</TableHead><TableHead>Id</TableHead><TableHead className="text-right">Running for</TableHead></TableRow></TableHeader>
            <TableBody>
              {h.stuck.length === 0 && <TableRow><TableCell colSpan={3} className="text-muted-foreground">Nothing is stuck.</TableCell></TableRow>}
              {h.stuck.map((s) => (
                <TableRow key={s.id}>
                  <TableCell>{s.kind}</TableCell>
                  <TableCell><Link href={s.href} className="font-mono text-xs hover:underline">{s.id}</Link></TableCell>
                  <TableCell className="text-right font-mono">{s.age_minutes}m</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </div>

      <h3 className="heading mt-6 mb-2 text-base">Last infra errors</h3>
      <div className="rounded-[14px] border border-border bg-card">
        <Table>
          <TableHeader><TableRow><TableHead>When</TableHead><TableHead>Kind</TableHead><TableHead>Id</TableHead><TableHead>Reason</TableHead></TableRow></TableHeader>
          <TableBody>
            {h.infra_errors.length === 0 && <TableRow><TableCell colSpan={4} className="text-muted-foreground">No infra errors recorded.</TableCell></TableRow>}
            {h.infra_errors.map((e) => (
              <TableRow key={e.kind + e.id}>
                <TableCell className="text-muted-foreground">{ago(e.at)}</TableCell>
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

const TYPE_LABEL: Record<AdminEvent['type'], string> = {
  signup: 'Signup', submission: 'Submission', product_entry: 'Product entry', bot_version: 'Bot version', tournament: 'Tournament',
}

function statusVariant(s: string) {
  if (s === 'passed' || s === 'done' || s === 'active' || s === 'finished') return 'default' as const
  if (s === 'failed' || s === 'rejected' || s === 'infra_error') return 'destructive' as const
  return 'outline' as const
}

function FeedSection({ items }: { items: AdminEvent[] }) {
  return (
    <section>
      <SectionTitle aside="last 50 events">Recent activity</SectionTitle>
      <div className="rounded-[14px] border border-border bg-card">
        <Table>
          <TableHeader><TableRow><TableHead>When</TableHead><TableHead>Event</TableHead><TableHead>Who</TableHead><TableHead>Detail</TableHead><TableHead>Status</TableHead></TableRow></TableHeader>
          <TableBody>
            {items.length === 0 && <TableRow><TableCell colSpan={5} className="text-muted-foreground">Nothing has happened yet.</TableCell></TableRow>}
            {items.map((e, i) => (
              <TableRow key={`${e.type}-${e.at}-${i}`}>
                <TableCell className="text-muted-foreground">{ago(e.at)}</TableCell>
                <TableCell>{TYPE_LABEL[e.type]}</TableCell>
                <TableCell><Link href={e.href} className="font-semibold hover:underline">{e.title}</Link></TableCell>
                <TableCell className="max-w-[22rem] truncate text-muted-foreground" title={e.detail}>{e.detail}</TableCell>
                <TableCell>{e.status && <Badge variant={statusVariant(e.status)}>{e.status.replace('_', ' ')}</Badge>}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </section>
  )
}
