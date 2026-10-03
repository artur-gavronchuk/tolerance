'use client'

import Link from 'next/link'
import { useCallback, useEffect, useRef, useState } from 'react'
import { ArrowLeft, ArrowRight, ArrowDown, ExternalLink } from 'lucide-react'
import { useLoginHref } from '@/components/public/return-path'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { products } from '@/lib/api'
import type { CompareJudged, CompareNext } from '@/lib/types'
import { siteURL } from './entry-card'
import { friendly, usePT } from './phase'

type Verdict = 'a' | 'b' | 'tie'

// Blind comparison: two anonymous sites side by side (tabs on a phone), a verdict, then the authors are
// revealed and the next pair comes. The standings come from everybody's verdicts.
export function Compare({ slug, signedIn }: { slug: string; signedIn: boolean }) {
  const t = usePT()
  const loginTo = useLoginHref()
  const [next, setNext] = useState<CompareNext | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [tab, setTab] = useState<'a' | 'b'>('a')
  const [revealed, setRevealed] = useState<CompareJudged | null>(null)
  const toast = useRef<ReturnType<typeof setTimeout> | null>(null)

  const load = useCallback(async () => {
    try {
      setNext(await products.compareNext(slug))
      setTab('a')
    } catch (e) {
      setError(friendly(t, e))
    }
  }, [slug, t])
  useEffect(() => { if (signedIn) void load() }, [signedIn, load])
  useEffect(() => () => { if (toast.current) clearTimeout(toast.current) }, [])

  const pair = next?.pair ?? null
  const judge = useCallback(async (v: Verdict) => {
    if (!pair || busy) return
    setBusy(true)
    setError(null)
    try {
      const j = await products.compare(slug, pair.a.id, pair.b.id, v)
      setRevealed(j)
      if (toast.current) clearTimeout(toast.current)
      toast.current = setTimeout(() => setRevealed(null), 6000)
      await load()
    } catch (e) {
      setError(friendly(t, e))
    } finally {
      setBusy(false)
    }
  }, [pair, busy, slug, load, t])

  // ← A is better, ↓ tie, → B is better.
  useEffect(() => {
    if (!pair) return
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement | null
      if (e.metaKey || e.ctrlKey || e.altKey || t?.closest('input, textarea, select, [contenteditable]')) return
      const v = e.key === 'ArrowLeft' ? 'a' : e.key === 'ArrowRight' ? 'b' : e.key === 'ArrowDown' ? 'tie' : null
      if (!v) return
      e.preventDefault()
      void judge(v)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [pair, judge])

  return (
    <section aria-label={t('cmp.title')} className="space-y-4 rounded-[14px] border border-primary/40 bg-card p-4 sm:p-5">
      <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
        <h2 className="heading text-lg">{t('cmp.title')}</h2>
        {next && next.target > 0 && (
          <span className="font-mono text-sm text-muted-foreground" aria-live="polite">
            {pair ? t('cmp.of', { a: next.judged + 1, b: next.target }) : t('cmp.judged', { a: next.judged, b: next.target })}
          </span>
        )}
      </div>
      <p className="max-w-2xl text-sm text-muted-foreground">
        {t('judge.sentence')} {t('cmp.openBoth')}
      </p>

      {!signedIn && (
        <p className="text-sm text-muted-foreground"><Link className="font-semibold text-primary hover:underline" href={loginTo}>{t('cmp.signInLink')}</Link>{t('cmp.signInTail')}</p>
      )}
      {signedIn && !next && !error && <Skeleton className="h-80 rounded-[10px]" />}
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}

      {revealed && <RevealToast j={revealed} />}

      {signedIn && next && !pair && (
        <p className="rounded-[10px] border border-dashed border-strong px-4 py-8 text-center text-sm text-muted-foreground">
          {next.target === 0
            ? t('cmp.notEnough')
            : t('cmp.done', { pairs: t.plural('pairs', next.judged) })}
        </p>
      )}

      {pair && (
        <>
          <div role="tablist" aria-label={t('cmp.showSite')} className="grid grid-cols-2 gap-2 md:hidden">
            {(['a', 'b'] as const).map((k) => (
              <Button key={k} role="tab" aria-selected={tab === k} variant={tab === k ? 'default' : 'outline'} onClick={() => setTab(k)}>
                {t('cmp.tab', { k: k.toUpperCase() })}
              </Button>
            ))}
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            {(['a', 'b'] as const).map((k) => (
              <Frame key={pair[k].id} id={pair[k].id} label={k.toUpperCase()} className={tab === k ? '' : 'hidden md:block'} />
            ))}
          </div>
          <div className="grid grid-cols-3 gap-2">
            <Button disabled={busy} onClick={() => void judge('a')} title={t('cmp.keyLeft')}>
              <ArrowLeft /><span className="truncate">{t('cmp.aBetter')}</span>
            </Button>
            <Button disabled={busy} variant="outline" onClick={() => void judge('tie')} title={t('cmp.keyDown')}>
              <ArrowDown /><span className="truncate">{t('cmp.tie')}</span>
            </Button>
            <Button disabled={busy} onClick={() => void judge('b')} title={t('cmp.keyRight')}>
              <span className="truncate">{t('cmp.bBetter')}</span><ArrowRight />
            </Button>
          </div>
          <p className="hidden text-xs text-muted-foreground md:block">{t('cmp.keyboard')}</p>
        </>
      )}
    </section>
  )
}

function Frame({ id, label, className }: { id: string; label: string; className: string }) {
  const t = usePT()
  return (
    <div className={`min-w-0 overflow-hidden rounded-[10px] border border-border ${className}`}>
      <div className="flex items-center justify-between border-b border-border px-3 py-1.5">
        <span className="font-mono text-sm font-semibold">{label}</span>
        <Button size="sm" variant="ghost" nativeButton={false} render={<a href={siteURL(id)} target="_blank" rel="noreferrer" />}>
          <ExternalLink />{t('entry.openFull')}
        </Button>
      </div>
      <iframe src={siteURL(id)} title={t('cmp.frameTitle', { k: label })} sandbox="allow-scripts allow-forms allow-modals"
        className="block h-[26rem] w-full bg-white md:h-[34rem]" />
    </div>
  )
}

// After a verdict: whose sites those were.
function RevealToast({ j }: { j: CompareJudged }) {
  const t = usePT()
  const name = (r: CompareJudged['a']) => <b className="text-foreground">{r.handle}</b>
  const made = (r: CompareJudged['a']) => (r.made_with ? ` (${r.made_with})` : '')
  const who = (r: CompareJudged['a']) => <>{name(r)}{made(r)}</>
  return (
    <p role="status" className="rounded-[10px] border border-border bg-muted/50 px-4 py-2.5 text-sm text-muted-foreground">
      {splice(t('cmp.revealed', { a: '\u0000', b: '\u0001' }), { '\u0000': who(j.a), '\u0001': who(j.b) })}{' '}
      {j.winner === 'tie' ? t('cmp.tied') : splice(t('cmp.picked', { who: '\u0000' }), { '\u0000': name(j.winner === 'a' ? j.a : j.b) })}
    </p>
  )
}

// Fills marker characters in a translated sentence with JSX.
function splice(s: string, parts: Record<string, React.ReactNode>) {
  return s.split(/([\u0000\u0001])/).map((p, i) => (p in parts ? <span key={i}>{parts[p]}</span> : p))
}
