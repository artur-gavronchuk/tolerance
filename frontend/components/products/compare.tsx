'use client'

import Link from 'next/link'
import { useCallback, useEffect, useRef, useState } from 'react'
import { ArrowLeft, ArrowRight, ArrowDown, ExternalLink } from 'lucide-react'
import { useLoginHref } from '@/components/public/return-path'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { friendlyMessage, products } from '@/lib/api'
import type { CompareJudged, CompareNext } from '@/lib/types'
import { siteURL } from './entry-card'
import { JUDGE_SENTENCE } from './phase'

type Verdict = 'a' | 'b' | 'tie'

// Blind comparison: two anonymous sites side by side (tabs on a phone), a verdict, then the authors are
// revealed and the next pair comes. The standings come from everybody's verdicts.
export function Compare({ slug, signedIn }: { slug: string; signedIn: boolean }) {
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
      setError(friendlyMessage(e))
    }
  }, [slug])
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
      setError(friendlyMessage(e))
    } finally {
      setBusy(false)
    }
  }, [pair, busy, slug, load])

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
    <section aria-label="Compare" className="space-y-4 rounded-[14px] border border-primary/40 bg-card p-4 sm:p-5">
      <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
        <h2 className="heading text-lg">Compare</h2>
        {next && next.target > 0 && (
          <span className="font-mono text-sm text-muted-foreground" aria-live="polite">
            {pair ? `${next.judged + 1} of ${next.target}` : `${next.judged} of ${next.target} judged`}
          </span>
        )}
      </div>
      <p className="max-w-2xl text-sm text-muted-foreground">
        {JUDGE_SENTENCE} Open both and use them before you decide.
      </p>

      {!signedIn && (
        <p className="text-sm text-muted-foreground"><Link className="font-semibold text-primary hover:underline" href={loginTo}>Sign in</Link> to judge sites.</p>
      )}
      {signedIn && !next && !error && <Skeleton className="h-80 rounded-[10px]" />}
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}

      {revealed && <RevealToast j={revealed} />}

      {signedIn && next && !pair && (
        <p className="rounded-[10px] border border-dashed border-input px-4 py-8 text-center text-sm text-muted-foreground">
          {next.target === 0
            ? 'There are not enough other sites to compare yet.'
            : `You judged ${next.judged} ${next.judged === 1 ? 'pair' : 'pairs'}. Thank you, that is all we need from you for this task.`}
        </p>
      )}

      {pair && (
        <>
          <div role="tablist" aria-label="Site to show" className="grid grid-cols-2 gap-2 md:hidden">
            {(['a', 'b'] as const).map((k) => (
              <Button key={k} role="tab" aria-selected={tab === k} variant={tab === k ? 'default' : 'outline'} onClick={() => setTab(k)}>
                Site {k.toUpperCase()}
              </Button>
            ))}
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            {(['a', 'b'] as const).map((k) => (
              <Frame key={pair[k].id} id={pair[k].id} label={k.toUpperCase()} className={tab === k ? '' : 'hidden md:block'} />
            ))}
          </div>
          <div className="grid grid-cols-3 gap-2">
            <Button disabled={busy} onClick={() => void judge('a')} title="Shortcut: left arrow">
              <ArrowLeft /><span className="truncate">A is better</span>
            </Button>
            <Button disabled={busy} variant="outline" onClick={() => void judge('tie')} title="Shortcut: down arrow">
              <ArrowDown /><span className="truncate">Tie</span>
            </Button>
            <Button disabled={busy} onClick={() => void judge('b')} title="Shortcut: right arrow">
              <span className="truncate">B is better</span><ArrowRight />
            </Button>
          </div>
          <p className="hidden text-xs text-muted-foreground md:block">Keyboard: ← A is better, ↓ tie, → B is better.</p>
        </>
      )}
    </section>
  )
}

function Frame({ id, label, className }: { id: string; label: string; className: string }) {
  return (
    <div className={`min-w-0 overflow-hidden rounded-[10px] border border-border ${className}`}>
      <div className="flex items-center justify-between border-b border-border px-3 py-1.5">
        <span className="font-mono text-sm font-semibold">{label}</span>
        <Button size="sm" variant="ghost" nativeButton={false} render={<a href={siteURL(id)} target="_blank" rel="noreferrer" />}>
          <ExternalLink />Open full size
        </Button>
      </div>
      <iframe src={siteURL(id)} title={`Site ${label}`} sandbox="allow-scripts allow-forms allow-modals"
        className="block h-[26rem] w-full bg-white md:h-[34rem]" />
    </div>
  )
}

// After a verdict: whose sites those were.
function RevealToast({ j }: { j: CompareJudged }) {
  const name = (r: CompareJudged['a']) => <b className="text-foreground">{r.handle}</b>
  const made = (r: CompareJudged['a']) => (r.made_with ? ` (${r.made_with})` : '')
  return (
    <p role="status" className="rounded-[10px] border border-border bg-muted/50 px-4 py-2.5 text-sm text-muted-foreground">
      You judged: A was {name(j.a)}{made(j.a)}, B was {name(j.b)}{made(j.b)}.{' '}
      {j.winner === 'tie' ? 'You called it a tie.' : <>You picked {name(j.winner === 'a' ? j.a : j.b)}.</>}
    </p>
  )
}
