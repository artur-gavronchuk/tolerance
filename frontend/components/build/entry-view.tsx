'use client'

import { useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { ArrowLeft, ArrowRight, BadgeCheck, Bot, CheckCircle2, EyeOff, ExternalLink, Loader2, Monitor, Smartphone, Trash2, XCircle } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { ScoreBadge, ScoreBreakdown, VoteButton, entryHref, siteUrl } from '@/components/build/build-gallery'
import { api } from '@/lib/api'
import { FORMATS, tx } from '@/lib/build-kinds'
import { errorText } from '@/lib/format'
import { formatDateTime } from '@/lib/i18n/core'
import { useT } from '@/lib/i18n/client'
import { buildMessages } from '@/lib/i18n/messages/build'
import type { BuildEntryPage } from '@/lib/types'
import { useCanAdmin } from '@/lib/use-can-admin'
import { cn } from '@/lib/utils'

// One solution: live preview, who made it with which agent, the platform's score and its place.
export function EntryView({ slug, id }: { slug: string; id: string }) {
  const t = useT(buildMessages)
  const router = useRouter()
  const canAdmin = useCanAdmin()
  const [p, setP] = useState<BuildEntryPage | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [view, setView] = useState<'desktop' | 'phone' | null>(null)
  const [armed, setArmed] = useState(false)

  const load = useCallback(async () => {
    try {
      setP(await api<BuildEntryPage>(`/build-entries/${encodeURIComponent(id)}`))
      setError(null)
    } catch (e) {
      setError(errorText(e, t.locale))
    }
  }, [id, t.locale])
  useEffect(() => { void load() }, [load])

  const pending = p?.entry.status === 'queued' || p?.entry.status === 'running'
  useEffect(() => {
    if (!pending) return
    const h = setInterval(() => void load(), 3000)
    return () => clearInterval(h)
  }, [pending, load])

  if (error && !p) return <p role="alert" className="text-destructive">{error}</p>
  if (!p) return <div className="space-y-4"><Skeleton className="h-24" /><Skeleton className="h-[70vh]" /></div>

  const { entry: e, challenge: c } = p
  const k = FORMATS[c.format]
  const mobile = c.format === 'mobile'
  const mode = view ?? (mobile ? 'phone' : 'desktop')
  const failed = e.checks.failed
  const shown = e.reference ? e.test_score : e.score

  async function remove() {
    if (!armed) { setArmed(true); return }
    try {
      await api(`/build-entries/${encodeURIComponent(e.id)}`, { method: 'DELETE' })
      router.push(`/c/${encodeURIComponent(slug)}`)
    } catch {}
  }

  return (
    <div className="space-y-6">
      <nav className="flex flex-wrap items-center gap-1.5 text-sm font-semibold text-muted-foreground">
        <Link href="/" className="hover:text-foreground">{t('allChallenges')}</Link>
        <span aria-hidden>/</span>
        <Link href={`/c/${encodeURIComponent(c.slug)}`} className={cn('inline-flex items-center gap-1 hover:opacity-80', k.text)}>
          <k.icon className="size-4" />{tx(c.title, t.locale)}
        </Link>
        <span aria-hidden>/</span>
        <span className="text-foreground">{t('solution')}</span>
      </nav>

      <header className="relative overflow-hidden rounded-[20px] bg-terminal p-5 text-terminal-foreground sm:p-7">
        <div aria-hidden className={cn('pointer-events-none absolute -right-20 -top-24 size-72 rounded-full blur-3xl', k.glow)} />
        <div className="relative flex flex-wrap items-center gap-x-6 gap-y-4">
          <div className="min-w-0 flex-1">
            <p className="text-sm font-bold text-terminal-foreground/60">
              {e.place ? t('place', { place: e.place, of: p.of }) : t('notRanked')}
            </p>
            <h1 className="display mt-1 flex items-center gap-3 truncate text-[2.2rem] sm:text-[3rem]">
              {e.reference && <BadgeCheck className="size-9 shrink-0 text-terminal-accent" />}{e.reference ? t('reference') : e.handle}
            </h1>
            {e.reference ? (
              <p className="mt-2 max-w-xl text-sm text-terminal-foreground/70">{t('referenceHint')}</p>
            ) : (
              <p className="mt-2 inline-flex max-w-full items-center gap-2 rounded-full bg-white/10 px-3 py-1.5 text-sm font-bold">
                <Bot className="size-4 shrink-0 text-terminal-accent" />
                <span className="text-terminal-foreground/60">{t('agent')}:</span>
                <span className="truncate">{e.made_with || t('agentUnknown')}</span>
              </p>
            )}
          </div>
          <div className="flex items-center gap-3">
            {shown !== null && (
              <div className="text-center">
                <div className={cn('rounded-[18px] bg-gradient-to-br px-5 py-3 font-mono text-[2.6rem] font-bold leading-none text-white tabular-nums', k.gradient)}>{shown}</div>
                <p className="mt-1.5 text-xs font-bold text-terminal-foreground/60">{e.reference ? t('testsLabel') : t('score')}</p>
              </div>
            )}
            {e.status === 'done' && !e.reference && <VoteButton e={e} canVote={c.status === 'open'} onChange={() => void load()} big />}
          </div>
        </div>
      </header>

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_20rem]">
        <section className="min-w-0 space-y-3">
          <div className="flex flex-wrap items-center gap-2">
            {!mobile && (
              <div className="flex rounded-full border border-border bg-card p-1 text-sm font-semibold">
                {(['desktop', 'phone'] as const).map((m) => (
                  <button key={m} onClick={() => setView(m)} aria-pressed={mode === m}
                    className={cn('flex items-center gap-1.5 rounded-full px-3 py-1 transition-colors', mode === m ? 'bg-ink text-ink-foreground' : 'text-muted-foreground hover:text-foreground')}>
                    {m === 'desktop' ? <Monitor className="size-4" /> : <Smartphone className="size-4" />}
                    {m === 'desktop' ? t('viewDesktop') : t('viewPhone')}
                  </button>
                ))}
              </div>
            )}
            <Button variant="outline" size="sm" className="ml-auto" nativeButton={false} render={<a href={siteUrl(e)} target="_blank" rel="noreferrer" />}>
              <ExternalLink />{t('openNewTab')}
            </Button>
          </div>
          {mode === 'phone' ? (
            <div className="flex justify-center rounded-[20px] bg-muted py-8">
              <div className="overflow-hidden rounded-[44px] border-[12px] border-ink bg-ink shadow-2xl">
                <iframe key={e.version} src={siteUrl(e)} title={e.handle} sandbox="allow-scripts allow-forms allow-modals allow-popups"
                  className="block h-[720px] w-[360px] max-w-[calc(100vw-5rem)] border-0 bg-white" />
              </div>
            </div>
          ) : (
            <div className="overflow-hidden rounded-[16px] border border-border bg-card shadow-sm">
              <div className="flex items-center gap-1.5 border-b border-border bg-muted px-4 py-2.5">
                <span className="size-3 rounded-full bg-pop-2/70" /><span className="size-3 rounded-full bg-pop-6/80" /><span className="size-3 rounded-full bg-pop-5/70" />
              </div>
              <iframe key={e.version} src={siteUrl(e)} title={e.handle} sandbox="allow-scripts allow-forms allow-modals allow-popups"
                className="block h-[72vh] min-h-[520px] w-full border-0 bg-white" />
            </div>
          )}
          <p className="text-center text-sm text-muted-foreground">{t('playHint')}</p>
        </section>

        <aside className="space-y-4">
          {pending && (
            <div className="flex items-start gap-3 rounded-[14px] bg-accent p-4 text-accent-foreground">
              <Loader2 className="mt-0.5 size-5 shrink-0 animate-spin" />
              <p className="text-sm font-semibold">{e.status === 'queued' ? t('queued') : t('running')}</p>
            </div>
          )}
          {e.status === 'infra_error' && <p className="rounded-[14px] bg-destructive/10 p-4 text-sm font-semibold text-destructive">{t('infraError')}</p>}
          {e.status === 'done' && (
            <div className="rounded-[14px] border border-border bg-card p-5">
              <div className="mb-4 flex items-center justify-between gap-3">
                <h2 className="heading text-lg">{t('score')}</h2>
                <ScoreBadge score={shown} />
              </div>
              <ScoreBreakdown e={e} />
              <div className="mt-4 border-t border-border pt-4 text-sm">
                {failed === undefined ? (
                  <p className="flex items-start gap-2 text-muted-foreground"><EyeOff className="mt-0.5 size-4 shrink-0" />{t('failedHidden')}</p>
                ) : failed.length === 0 ? (
                  <p className="flex items-center gap-2 font-semibold text-success"><CheckCircle2 className="size-4" />{t('allPassed')}</p>
                ) : (
                  <>
                    <p className="mb-2 font-bold">{t('failedTests')}</p>
                    <ul className="space-y-1.5">
                      {failed.map((n) => (
                        <li key={n} className="flex items-start gap-2 text-muted-foreground"><XCircle className="mt-0.5 size-4 shrink-0 text-destructive" />{n}</li>
                      ))}
                    </ul>
                  </>
                )}
              </div>
            </div>
          )}
          <p className="text-sm text-muted-foreground">{t('uploaded', { date: formatDateTime(t.locale, e.updated_at) })}</p>
          {e.mine && e.log_tail && (
            <details className="text-sm">
              <summary className="cursor-pointer font-semibold">{t('log')}</summary>
              <pre className="mt-2 max-h-64 overflow-auto rounded-[8px] bg-terminal p-3 font-mono text-xs leading-5 whitespace-pre-wrap text-terminal-foreground">{e.log_tail}</pre>
            </details>
          )}
          {(e.mine || canAdmin) && (
            <Button variant="destructive" size="sm" onClick={() => void remove()}>
              <Trash2 />{armed ? t('deleteConfirm') : t('deleteEntry')}
            </Button>
          )}
        </aside>
      </div>

      {(p.prev || p.next) && (
        <nav className="flex justify-between gap-3 border-t border-border pt-5">
          {p.prev
            ? <Button variant="outline" nativeButton={false} render={<Link href={entryHref(c.slug, { id: p.prev })} />}><ArrowLeft />{t('prev')}</Button>
            : <span />}
          {p.next && <Button variant="outline" nativeButton={false} render={<Link href={entryHref(c.slug, { id: p.next })} />}>{t('next')}<ArrowRight /></Button>}
        </nav>
      )}
    </div>
  )
}
