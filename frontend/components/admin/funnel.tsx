'use client'

import { useEffect, useState } from 'react'
import { SectionTitle } from '@/components/page-header'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { admin, ApiError } from '@/lib/api'
import { errorText } from '@/lib/i18n/messages/errors'
import { useT } from '@/lib/i18n/client'
import { funnelMessages } from '@/lib/i18n/messages/admin-funnel'
import type { AdminFunnel, AdminPoint } from '@/lib/types'

function Bars({ points, label }: { points: AdminPoint[]; label: string }) {
  const max = Math.max(1, ...points.map((p) => p.value))
  const W = 140
  const H = 28
  const step = W / points.length
  return (
    <svg viewBox={`0 0 ${W} ${H}`} className="h-7 w-full" role="img" aria-label={`${label}: ${points.map((p) => p.value).join(', ')}`} preserveAspectRatio="none">
      {points.map((p, i) => {
        const h = p.value === 0 ? 1.5 : Math.max(3, (p.value / max) * (H - 2))
        return (
          <rect key={p.day} x={i * step + 1} y={H - h} width={step - 2} height={h} rx={1.5} className={i === points.length - 1 ? 'fill-primary' : 'fill-primary/35'}>
            <title>{`${p.day}: ${p.value}`}</title>
          </rect>
        )
      })}
    </svg>
  )
}

export function FunnelSection() {
  const t = useT(funnelMessages)
  const [f, setF] = useState<AdminFunnel | null>(null)
  const [error, setError] = useState<ApiError | Error | null>(null)
  useEffect(() => {
    let live = true
    const load = () => admin.funnel().then((x) => live && (setF(x), setError(null))).catch((e) => live && setError(e as Error))
    void load()
    const i = setInterval(() => void load(), 60_000)
    return () => { live = false; clearInterval(i) }
  }, [])

  if (error) return <p role="alert" className="text-sm text-destructive">{errorText(error, t.locale)}</p>
  if (!f) return <Skeleton className="h-64 rounded-xl" />

  const steps = [
    { key: 'visit', label: t('visit') }, { key: 'signin', label: t('signin') }, { key: 'download', label: t('download') },
    { key: 'upload', label: t('upload') }, { key: 'passed', label: t('passed') },
  ] as const
  const sums = Object.fromEntries(steps.map((s) => [s.key, f.days.reduce((a, d) => a + d[s.key], 0)])) as Record<(typeof steps)[number]['key'], number>
  const base = Math.max(1, sums.visit)
  const done = f.days.filter((d) => d.returned != null && d.day < f.days[f.days.length - 1].day)
  const visits = done.reduce((a, d) => a + d.visit, 0)
  const back = done.reduce((a, d) => a + (d.returned ?? 0), 0)

  return (
    <section>
      <SectionTitle aside={t('aside')}>{t('title')}</SectionTitle>
      <div className="grid gap-3 lg:grid-cols-3">
        <div className="min-w-0 rounded-xl border border-border bg-card p-4 lg:col-span-2">
          <div className="text-xs font-semibold text-muted-foreground">{t('steps')}</div>
          <ul className="mt-3 space-y-3">
            {steps.map((s) => (
              <li key={s.key} className="grid grid-cols-[1fr_auto] items-center gap-x-3 gap-y-1 sm:grid-cols-[11rem_1fr_6.5rem]">
                <span className="text-sm">{s.label}</span>
                <span className="text-right font-mono text-sm sm:order-3">{sums[s.key]} · {Math.round((sums[s.key] / base) * 100)}%</span>
                <div className="col-span-2 min-w-0 sm:order-2 sm:col-span-1">
                  <div className="h-2 rounded-full bg-muted"><div className="h-2 rounded-full bg-primary" style={{ width: `${(sums[s.key] / base) * 100}%` }} /></div>
                  <Bars points={f.days.map((d) => ({ day: d.day, value: d[s.key] }))} label={t('perDay', { label: s.label })} />
                </div>
              </li>
            ))}
          </ul>
          <p className="mt-3 text-xs text-muted-foreground">{t('stepsHint')}</p>
        </div>
        <div className="min-w-0 rounded-xl border border-border bg-card p-4">
          <div className="text-xs font-semibold text-muted-foreground">{t('retention')}</div>
          <div className="mt-1 font-mono text-2xl font-bold">{visits ? `${Math.round((back / visits) * 100)}%` : '-'}</div>
          <div className="mt-1 text-xs text-muted-foreground">{t('retentionHint')}</div>
          <Bars points={f.days.map((d) => ({ day: d.day, value: d.returned ?? 0 }))} label={t('retention')} />
        </div>
      </div>

      <h3 className="heading mt-6 mb-2 text-base">{t('activity')}</h3>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        {f.modes.map((m) => (
          <div key={m.mode} className="min-w-0 rounded-xl border border-border bg-card p-4">
            <div className="text-xs font-semibold text-muted-foreground">{t(m.mode)}</div>
            <div className="mt-1 font-mono text-2xl font-bold">{t('people', { n: m.people })}</div>
            <div className="mt-1 text-xs text-muted-foreground">{t('events', { n: m.events })}</div>
            <div className="mt-2"><Bars points={m.series} label={t(m.mode)} /></div>
          </div>
        ))}
      </div>

      <div className="mt-6 grid gap-3 lg:grid-cols-2">
        <div className="min-w-0">
          <h3 className="heading mb-2 text-base">{t('pages')}</h3>
          <div className="rounded-xl border border-border bg-card">
            <Table>
              <TableHeader><TableRow><TableHead>{t('colPage')}</TableHead><TableHead className="text-right">{t('colViews')}</TableHead><TableHead className="text-right">{t('colVisitors')}</TableHead></TableRow></TableHeader>
              <TableBody>
                {f.pages.length === 0 && <TableRow><TableCell colSpan={3} className="text-muted-foreground">{t('none')}</TableCell></TableRow>}
                {f.pages.map((p) => (
                  <TableRow key={p.path}><TableCell className="break-all font-mono text-xs">{p.path}</TableCell><TableCell className="text-right font-mono">{p.views}</TableCell><TableCell className="text-right font-mono">{p.visitors}</TableCell></TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </div>
        <div className="min-w-0">
          <h3 className="heading mb-2 text-base">{t('sources')}</h3>
          <div className="rounded-xl border border-border bg-card">
            <Table>
              <TableHeader><TableRow><TableHead>{t('colSource')}</TableHead><TableHead className="text-right">{t('colVisits')}</TableHead></TableRow></TableHeader>
              <TableBody>
                {f.sources.length === 0 && <TableRow><TableCell colSpan={2} className="text-muted-foreground">{t('none')}</TableCell></TableRow>}
                {f.sources.map((s) => (
                  <TableRow key={s.source}><TableCell className="break-all">{s.source === '(direct)' ? t('direct') : s.source}</TableCell><TableCell className="text-right font-mono">{s.visits}</TableCell></TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
          <p className="mt-2 text-xs text-muted-foreground">{t('sourcesHint')}</p>
        </div>
      </div>
    </section>
  )
}
