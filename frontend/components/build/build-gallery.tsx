'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { ExternalLink, Heart, Maximize2, Sparkles, Trash2, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useLoginHref } from '@/components/public/return-path'
import { api } from '@/lib/api'
import { errorText } from '@/lib/format'
import { useT } from '@/lib/i18n/client'
import { buildMessages } from '@/lib/i18n/messages/build'
import type { BuildEntry } from '@/lib/types'
import { useCanAdmin } from '@/lib/use-can-admin'
import { useMyHandle } from '@/lib/use-signed-in'
import { cn } from '@/lib/utils'

export const siteUrl = (e: BuildEntry) => `/api/v1/build-entries/${encodeURIComponent(e.id)}/site/index.html`
export const shotUrl = (e: BuildEntry) => `/api/v1/build-entries/${encodeURIComponent(e.id)}/shot.jpg?v=${e.version}`

type Sort = 'votes' | 'score'

export function BuildGallery({ entries, onChange, onRemoved }: {
  entries: BuildEntry[]
  onChange: (e: BuildEntry) => void
  onRemoved: () => void
}) {
  const t = useT(buildMessages)
  const [sort, setSort] = useState<Sort>('votes')
  const [open, setOpen] = useState<string | null>(null)
  const sorted = sort === 'votes' ? entries : [...entries].sort((a, b) => (b.score ?? 0) - (a.score ?? 0) || b.votes - a.votes)
  const opened = entries.find((e) => e.id === open) ?? null

  return (
    <section>
      <div className="mb-4 flex flex-wrap items-end justify-between gap-3">
        <h2 className="heading flex items-center gap-2 text-2xl">
          <Sparkles className="size-6 text-pop-2" />{t('gallery')}
          <span className="font-mono text-lg text-muted-foreground">{entries.length}</span>
        </h2>
        {entries.length > 1 && (
          <div className="flex rounded-full border border-border bg-card p-1 text-sm font-semibold">
            {(['votes', 'score'] as const).map((s) => (
              <button key={s} onClick={() => setSort(s)} aria-pressed={sort === s}
                className={cn('rounded-full px-3 py-1 transition-colors', sort === s ? 'bg-ink text-ink-foreground' : 'text-muted-foreground hover:text-foreground')}>
                {s === 'votes' ? t('sortVotes') : t('sortScore')}
              </button>
            ))}
          </div>
        )}
      </div>
      {entries.length === 0 ? (
        <p className="rounded-[14px] border-2 border-dashed border-strong px-6 py-12 text-center text-muted-foreground">{t('galleryEmpty')}</p>
      ) : (
        <ul className="grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
          {sorted.map((e, i) => (
            <li key={e.id}>
              <EntryCard e={e} place={i + 1} onOpen={() => setOpen(e.id)} onChange={onChange} />
            </li>
          ))}
        </ul>
      )}
      {opened && <Preview e={opened} onClose={() => setOpen(null)} onChange={onChange} onRemoved={() => { setOpen(null); onRemoved() }} />}
    </section>
  )
}

function EntryCard({ e, place, onOpen, onChange }: { e: BuildEntry; place: number; onOpen: () => void; onChange: (e: BuildEntry) => void }) {
  const t = useT(buildMessages)
  return (
    <article className={cn('group overflow-hidden rounded-[16px] border bg-card transition-shadow hover:shadow-lg',
      e.mine ? 'border-primary' : 'border-border')}>
      <button onClick={onOpen} className="relative block aspect-[16/10] w-full overflow-hidden bg-muted text-left" aria-label={`${t('open')}: ${e.handle}`}>
        <Thumb e={e} />
        <span className={cn('absolute left-3 top-3 flex size-8 items-center justify-center rounded-full font-mono text-sm font-bold shadow',
          place <= 3 ? 'bg-gradient-to-br from-pop-1 via-pop-2 to-pop-3 text-white' : 'bg-card text-foreground')}>{place}</span>
        <span className="absolute inset-0 flex items-center justify-center bg-terminal/0 opacity-0 transition-all group-hover:bg-terminal/40 group-hover:opacity-100">
          <span className="flex items-center gap-1.5 rounded-full bg-card px-3 py-1.5 text-sm font-bold text-foreground"><Maximize2 className="size-4" />{t('open')}</span>
        </span>
      </button>
      <div className="flex items-center gap-3 p-4">
        <div className="min-w-0 flex-1">
          <p className="truncate font-bold">{e.handle}</p>
          <p className="truncate text-xs text-muted-foreground">{e.made_with || ' '}</p>
        </div>
        <ScoreBadge score={e.score} />
        <VoteButton e={e} onChange={onChange} />
      </div>
    </article>
  )
}

function Thumb({ e }: { e: BuildEntry }) {
  if (e.has_shot) {
    // eslint-disable-next-line @next/next/no-img-element
    return <img src={shotUrl(e)} alt="" loading="lazy" className="size-full object-cover object-top transition-transform duration-300 group-hover:scale-[1.03]" />
  }
  // No screenshot (fake sandbox locally): a live, inert, scaled-down frame.
  return (
    <iframe src={siteUrl(e)} title={e.handle} sandbox="allow-scripts" loading="lazy" tabIndex={-1}
      className="pointer-events-none h-[250%] w-[250%] origin-top-left scale-[0.4] border-0 bg-white" />
  )
}

export function ScoreBadge({ score, large }: { score: number | null; large?: boolean }) {
  if (score === null) return null
  const tone = score >= 80 ? 'bg-success text-success-foreground' : score >= 50 ? 'bg-warning text-warning-foreground' : 'bg-destructive/15 text-destructive'
  return (
    <span title="/100" className={cn('shrink-0 rounded-full font-mono font-bold tabular-nums', tone, large ? 'px-3.5 py-1 text-2xl' : 'px-2.5 py-0.5 text-sm')}>
      {score}
    </span>
  )
}

export function ScoreBreakdown({ e }: { e: BuildEntry }) {
  const t = useT(buildMessages)
  const p = e.checks.points ?? {}
  const rows = [
    { k: 'scenarios', label: t('ptsScenarios'), v: p.scenarios ?? 0, max: 70, extra: e.checks.total ? t('scenarios', { passed: e.checks.passed ?? 0, total: e.checks.total }) : '' },
    { k: 'a11y', label: t('ptsA11y'), v: p.a11y ?? 0, max: 10, extra: '' },
    { k: 'mobile', label: t('ptsMobile'), v: p.mobile ?? 0, max: 10, extra: '' },
    { k: 'perf', label: t('ptsPerf'), v: p.perf ?? 0, max: 10, extra: '' },
  ]
  return (
    <ul className="space-y-2 text-sm">
      {rows.map((r) => (
        <li key={r.k}>
          <div className="flex justify-between gap-2">
            <span className="font-semibold">{r.label} <span className="font-normal text-muted-foreground">{r.extra}</span></span>
            <span className="font-mono tabular-nums">{r.v}/{r.max}</span>
          </div>
          <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-muted">
            <div className="h-full rounded-full bg-gradient-to-r from-pop-1 via-pop-2 to-pop-3" style={{ width: `${(100 * r.v) / r.max}%` }} />
          </div>
        </li>
      ))}
    </ul>
  )
}

function VoteButton({ e, onChange, big }: { e: BuildEntry; onChange: (e: BuildEntry) => void; big?: boolean }) {
  const t = useT(buildMessages)
  const handle = useMyHandle()
  const loginHref = useLoginHref()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const label = e.mine ? t('ownEntry') : e.voted ? t('voted') : t('vote')
  const cls = cn('flex shrink-0 items-center gap-1.5 rounded-full border font-bold tabular-nums transition-colors disabled:opacity-60',
    big ? 'h-11 px-4' : 'h-9 px-3 text-sm',
    e.voted ? 'border-transparent bg-pop-2 text-white' : 'border-border bg-card hover:border-pop-2 hover:text-pop-2')
  const inner = <><Heart className={cn('size-4', e.voted && 'fill-current')} />{e.votes}{big && <span className="ml-1">{label}</span>}</>

  if (!handle) {
    return <Link href={loginHref} className={cls} title={t('signInToVote')} aria-label={t('signInToVote')}>{inner}</Link>
  }
  async function toggle() {
    setBusy(true)
    setError(null)
    try {
      onChange(await api<BuildEntry>(`/build-entries/${encodeURIComponent(e.id)}/vote`, { method: e.voted ? 'DELETE' : 'POST' }))
    } catch (err) {
      setError(errorText(err, t.locale))
    } finally {
      setBusy(false)
    }
  }
  return (
    <span className="relative">
      <button onClick={() => void toggle()} disabled={busy || e.mine} aria-pressed={e.voted}
        title={error ?? label} aria-label={`${label}. ${t('votesLabel', { n: e.votes })}`} className={cls}>{inner}</button>
    </span>
  )
}

function Preview({ e, onClose, onChange, onRemoved }: { e: BuildEntry; onClose: () => void; onChange: (e: BuildEntry) => void; onRemoved: () => void }) {
  const t = useT(buildMessages)
  const canAdmin = useCanAdmin()
  const [armed, setArmed] = useState(false)

  useEffect(() => {
    const onKey = (ev: KeyboardEvent) => { if (ev.key === 'Escape') onClose() }
    document.addEventListener('keydown', onKey)
    const overflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => { document.removeEventListener('keydown', onKey); document.body.style.overflow = overflow }
  }, [onClose])

  async function remove() {
    if (!armed) { setArmed(true); return }
    try {
      await api(`/build-entries/${encodeURIComponent(e.id)}`, { method: 'DELETE' })
      onRemoved()
    } catch {}
  }

  return (
    <div role="dialog" aria-modal="true" aria-label={e.handle} className="fixed inset-0 z-50 flex flex-col bg-terminal/90 p-2 backdrop-blur-sm sm:p-5" onClick={onClose}>
      <div className="mx-auto flex min-h-0 w-full max-w-7xl flex-1 flex-col overflow-hidden rounded-[16px] bg-card shadow-2xl lg:flex-row" onClick={(ev) => ev.stopPropagation()}>
        <iframe src={siteUrl(e)} title={e.handle} sandbox="allow-scripts allow-forms allow-modals allow-popups"
          className="min-h-[55vh] w-full flex-1 border-0 bg-white lg:min-h-0" />
        <aside className="flex w-full shrink-0 flex-col gap-4 overflow-y-auto border-t border-border p-5 lg:w-80 lg:border-l lg:border-t-0">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <p className="truncate text-lg font-bold">{e.handle}</p>
              {e.made_with && <p className="truncate text-sm text-muted-foreground">{e.made_with}</p>}
            </div>
            <button onClick={onClose} aria-label={t('close')} className="flex size-9 shrink-0 items-center justify-center rounded-full hover:bg-muted"><X className="size-5" /></button>
          </div>
          <div className="flex items-center justify-between gap-3">
            <span className="text-sm font-bold text-muted-foreground">{t('yourScore')}</span>
            <ScoreBadge score={e.score} large />
          </div>
          <ScoreBreakdown e={e} />
          <VoteButton e={e} onChange={onChange} big />
          <Button variant="outline" nativeButton={false} render={<a href={siteUrl(e)} target="_blank" rel="noreferrer" />}>
            <ExternalLink />{t('openNewTab')}
          </Button>
          {(canAdmin || e.mine) && (
            <Button variant="destructive" size="sm" onClick={() => void remove()} className="mt-auto">
              <Trash2 />{armed ? t('deleteConfirm') : t('deleteEntry')}
            </Button>
          )}
        </aside>
      </div>
    </div>
  )
}
